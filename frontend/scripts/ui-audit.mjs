// Audit UI otomatis Console: aksesibilitas (axe-core, WCAG 2 A/AA), overflow horizontal, galat konsol,
// dan keberlakuan mode gelap — untuk halaman utama (terang/gelap × desktop/mobile) dan modal/overlay.
// Exit code 1 bila ada pelanggaran, jadi bisa dipakai sebagai gerbang rilis/CI.
//
// Prasyarat: backend ACS berjalan, Console tersaji (mis. `npm run build && npx vite preview --port 5173`),
// Chrome/Chromium terpasang. Kredensial dari environment (JANGAN di-commit):
//
//   ACS_AUDIT_USER=... ACS_AUDIT_PASS=... npm run audit:ui
//
// Opsional: ACS_AUDIT_API (default http://localhost:18080/api/v1), ACS_AUDIT_APP (default
// http://localhost:5173), CHROME_PATH, ACS_AUDIT_ONLY (substring: "dark" | "mobile" | "desktop" | "light").
// Screenshot ditulis ke frontend/.ui-audit/ (tidak di-commit).
import puppeteer from 'puppeteer-core'
import fs from 'node:fs'
import path from 'node:path'
import { createRequire } from 'node:module'

const require = createRequire(import.meta.url)
const axeSrc = fs.readFileSync(require.resolve('axe-core/axe.min.js'), 'utf8')

const API = process.env.ACS_AUDIT_API || 'http://localhost:18080/api/v1'
const APP = process.env.ACS_AUDIT_APP || 'http://localhost:5173'
const USER = process.env.ACS_AUDIT_USER
const PASS = process.env.ACS_AUDIT_PASS
const ONLY = process.env.ACS_AUDIT_ONLY
if (!USER || !PASS) {
  console.error('Isi ACS_AUDIT_USER dan ACS_AUDIT_PASS (akun SUPERADMIN uji; jangan akun produksi).')
  process.exit(2)
}

function findChrome() {
  const candidates = [
    process.env.CHROME_PATH,
    'C:/Program Files/Google/Chrome/Application/chrome.exe',
    'C:/Program Files (x86)/Google/Chrome/Application/chrome.exe',
    '/usr/bin/google-chrome',
    '/usr/bin/chromium',
    '/usr/bin/chromium-browser',
    '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
  ].filter(Boolean)
  const found = candidates.find((p) => fs.existsSync(p))
  if (!found) {
    console.error('Chrome tidak ditemukan; set CHROME_PATH.')
    process.exit(2)
  }
  return found
}

const OUT = path.resolve(import.meta.dirname, '..', '.ui-audit')
fs.mkdirSync(OUT, { recursive: true })

const loginRes = await fetch(`${API}/auth/login`, {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({ username: USER, password: PASS }),
})
const login = await loginRes.json()
if (!login.access_token) {
  console.error('Login audit gagal (akun ber-MFA tidak didukung oleh skrip ini):', login.message || loginRes.status)
  process.exit(2)
}
const auth = { Authorization: `Bearer ${login.access_token}` }
const dev = (await (await fetch(`${API}/devices?page_size=1`, { headers: auth })).json()).data?.[0]

const ROUTES = [
  '/dashboard', '/devices', dev ? `/devices/${dev.id}` : null, '/tasks', '/provisioning', '/firmware',
  '/presets', '/audit', '/profile', '/webhooks', '/catalog', '/administration', '/reports',
  '/billing', '/bug-reports', '/data-transfers', '/notifications', '/notifications?tab=preferences', '/notifications?tab=announce', '/halaman-tidak-ada',
  // halaman publik (tanpa login); /signup diaudit dalam keadaan AKTIF (config dipaksa lewat intersepsi di bawah)
  '/signup', '/terms', '/privacy', '/verify-signup',
].filter(Boolean)
const VIEWPORTS = [
  { name: 'desktop', width: 1366, height: 800, mobile: false },
  { name: 'mobile', width: 390, height: 844, mobile: true },
]
const THEMES = ['light', 'dark']
const MODAL_PAGES = ['/devices', '/provisioning', '/firmware', '/files', '/tags', '/presets', '/webhooks', '/catalog', '/administration', '/profile']
const OPENERS = /^\s*(\+\s*)?(tambah|buat|upload|unggah|terbitkan|baru|aktifkan|ganti|import|impor)/i
const keep = (...tags) => !ONLY || tags.includes(ONLY)

const sleep = (ms) => new Promise((r) => setTimeout(r, ms))
const browser = await puppeteer.launch({ executablePath: findChrome(), headless: 'new', args: ['--no-sandbox'] })
const findings = [] // { where, kind, detail }
let views = 0

async function axeRun(page, selector) {
  await page.evaluate(axeSrc)
  return page.evaluate(async (sel) => {
    const root = sel ? document.querySelector(sel) : document
    if (!root) return null
    const r = await window.axe.run(root, { runOnly: { type: 'tag', values: ['wcag2a', 'wcag2aa'] } })
    return r.violations.map((v) => ({
      id: v.id,
      impact: v.impact,
      nodes: v.nodes.slice(0, 3).map((n) => n.html.slice(0, 120)),
      n: v.nodes.length,
    }))
  }, selector)
}

async function newPage(vp, theme) {
  const ctx = await browser.createBrowserContext()
  const page = await ctx.newPage()
  await page.setViewport({ width: vp.width, height: vp.height, isMobile: vp.mobile, hasTouch: vp.mobile })
  await page.evaluateOnNewDocument(
    (t, u, th) => {
      localStorage.setItem('acs_token', t)
      localStorage.setItem('acs_user', JSON.stringify(u))
      localStorage.setItem('acs_theme', th)
      localStorage.setItem('acs_lang', 'id')
    },
    login.access_token, login.user, theme,
  )
  return { ctx, page }
}

// 1) halaman utama
for (const vp of VIEWPORTS) for (const theme of THEMES) {
  if (!keep(vp.name, theme)) continue
  const { ctx, page } = await newPage(vp, theme)
  // Form pendaftaran bawaan MATI di backend; paksa tampil aktif HANYA untuk audit a11y bentuknya.
  await page.setRequestInterception(true)
  page.on('request', (req) => {
    if (req.url().endsWith('/auth/config') && req.method() === 'GET') {
      return req.respond({ status: 200, contentType: 'application/json', headers: { 'access-control-allow-origin': '*' }, body: JSON.stringify({ sso_enabled: false, mfa_available: true, self_signup_enabled: true }) })
    }
    req.continue()
  })
  const errors = []
  page.on('console', (m) => m.type() === 'error' && errors.push(m.text().slice(0, 160)))
  page.on('pageerror', (e) => errors.push(`PAGEERROR ${String(e).slice(0, 160)}`))
  for (const route of ROUTES) {
    errors.length = 0
    const where = `${vp.name}/${theme}${route}`
    try {
      await page.goto(APP + route, { waitUntil: 'networkidle2', timeout: 25000 })
    } catch (e) {
      findings.push({ where, kind: 'navigasi', detail: e.message })
    }
    await sleep(800)
    views++
    const m = await page.evaluate(() => ({
      overflow: document.documentElement.scrollWidth - document.documentElement.clientWidth,
      dark: document.documentElement.classList.contains('dark'),
      // toast galat pemuatan data (QueryErrorToaster) = ada query yang gagal/undefined meski tak ada console.error
      dataToast: /Gagal memuat data/.test(document.body.innerText),
    }))
    if (m.overflow > 1) findings.push({ where, kind: 'overflow-horizontal', detail: `+${m.overflow}px` })
    if (m.dataToast) findings.push({ where, kind: 'toast-galat-data', detail: 'ada toast "Gagal memuat data"' })
    if (theme === 'dark' && !m.dark) findings.push({ where, kind: 'mode-gelap-tidak-aktif', detail: '' })
    if (errors.length) findings.push({ where, kind: 'galat-konsol', detail: errors[0] })
    for (const v of (await axeRun(page, null)) ?? []) findings.push({ where, kind: `axe:${v.id}`, detail: `${v.n} node (${v.impact}) ${v.nodes[0]}` })
    await page.screenshot({ path: path.join(OUT, `${vp.name}-${theme}-${route.replace(/\W+/g, '_')}.png`) })
  }
  await ctx.close()
}

// 2) modal & overlay (desktop)
for (const theme of THEMES) {
  if (!keep('desktop', theme)) continue
  const { ctx, page } = await newPage(VIEWPORTS[0], theme)
  for (const route of MODAL_PAGES) {
    await page.goto(APP + route, { waitUntil: 'networkidle2', timeout: 25000 }).catch(() => {})
    await sleep(600)
    const opened = await page.evaluate((re) => {
      const rx = new RegExp(re, 'i')
      const b = [...document.querySelectorAll('button')].find((x) => !x.disabled && x.offsetParent && rx.test((x.textContent || '').trim()))
      if (!b) return null
      b.click()
      return (b.textContent || '').trim().slice(0, 30)
    }, OPENERS.source)
    if (!opened) continue
    await sleep(500)
    views++
    for (const v of (await axeRun(page, '[role="dialog"]')) ?? []) findings.push({ where: `modal/${theme}${route} [${opened}]`, kind: `axe:${v.id}`, detail: `${v.n} node (${v.impact}) ${v.nodes[0]}` })
    await page.keyboard.press('Escape')
    await sleep(250)
  }
  // menu aksi baris (RowMenu): buka pada tabel Users/Tenants & detail perangkat, lalu audit isinya
  for (const route of ['/administration', '/devices/' + (dev ? dev.id : '')]) {
    await page.goto(APP + route, { waitUntil: 'networkidle2', timeout: 25000 }).catch(() => {})
    await sleep(600)
    const opened = await page.evaluate(() => {
      const b = document.querySelector('button[aria-haspopup="menu"]')
      if (!b) return false
      b.click()
      return true
    })
    if (!opened) continue
    await sleep(300)
    views++
    for (const v of (await axeRun(page, '[role="menu"]')) ?? []) findings.push({ where: `rowmenu/${theme}${route}`, kind: `axe:${v.id}`, detail: `${v.n} node (${v.impact}) ${v.nodes[0]}` })
    await page.keyboard.press('Escape')
    await sleep(200)
  }
  // dialog global di app bar/sidebar: laporan bug & pengelola menu (Atur menu) — termasuk jebakan fokus (fokus di dalam dialog)
  for (const [label, sel] of [['bug', 'button[aria-label="Laporkan bug"]'], ['atur-menu', 'aside button']]) {
    await page.goto(APP + '/dashboard', { waitUntil: 'networkidle2', timeout: 25000 }).catch(() => {})
    await sleep(500)
    const opened = await page.evaluate((l, q) => {
      const b = l === 'bug' ? document.querySelector(q) : [...document.querySelectorAll(q)].find((x) => /atur menu/i.test(x.textContent || ''))
      if (!b) return false
      b.click()
      return true
    }, label, sel)
    if (!opened) { findings.push({ where: `dialog/${theme}/${label}`, kind: 'tidak-ada-tombol', detail: 'tombol pembuka dialog tidak ditemukan' }); continue }
    await sleep(500)
    views++
    for (const v of (await axeRun(page, '[role="dialog"]')) ?? []) findings.push({ where: `dialog/${theme}/${label}`, kind: `axe:${v.id}`, detail: `${v.n} node (${v.impact}) ${v.nodes[0]}` })
    const inside = await page.evaluate(() => !!document.activeElement?.closest('[role="dialog"]'))
    if (!inside) findings.push({ where: `dialog/${theme}/${label}`, kind: 'fokus', detail: 'fokus tidak berada di dalam dialog saat dibuka' })
    for (let i = 0; i < 12; i++) await page.keyboard.press('Tab')
    const still = await page.evaluate(() => !!document.activeElement?.closest('[role="dialog"]'))
    if (!still) findings.push({ where: `dialog/${theme}/${label}`, kind: 'fokus', detail: 'Tab berulang membuat fokus keluar dari dialog (jebakan fokus gagal)' })
    await page.keyboard.press('Escape')
    await sleep(250)
  }
  await page.goto(APP + '/dashboard', { waitUntil: 'networkidle2' }).catch(() => {})
  await sleep(500)
  await page.keyboard.down('Control')
  await page.keyboard.press('KeyK')
  await page.keyboard.up('Control')
  await sleep(400)
  views++
  for (const v of (await axeRun(page, '[class*="max-w-lg"]')) ?? []) findings.push({ where: `palette/${theme}`, kind: `axe:${v.id}`, detail: `${v.n} node (${v.impact}) ${v.nodes[0]}` })
  await ctx.close()
}
// 3) halaman publik ANONIM (tanpa token): landing, login, lupa password — terang/gelap × desktop/mobile
for (const vp of VIEWPORTS) for (const theme of THEMES) {
  if (!keep(vp.name, theme)) continue
  const ctx = await browser.createBrowserContext()
  const page = await ctx.newPage()
  await page.setViewport({ width: vp.width, height: vp.height, isMobile: vp.mobile, hasTouch: vp.mobile })
  await page.evaluateOnNewDocument((th) => {
    localStorage.setItem('acs_theme', th)
    localStorage.setItem('acs_lang', 'id')
  }, theme)
  const errors = []
  page.on('console', (m) => m.type() === 'error' && !/401|Failed to load resource/.test(m.text()) && errors.push(m.text().slice(0, 160)))
  page.on('pageerror', (e) => errors.push('PAGEERROR ' + String(e).slice(0, 160)))
  for (const route of ['/', '/login', '/forgot-password']) {
    errors.length = 0
    const where = 'anon/' + vp.name + '/' + theme + route
    await page.goto(APP + route, { waitUntil: 'networkidle2', timeout: 25000 }).catch((e) => findings.push({ where, kind: 'navigasi', detail: e.message }))
    await sleep(600)
    views++
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)
    if (overflow > 1) findings.push({ where, kind: 'overflow-horizontal', detail: '+' + overflow + 'px' })
    if (errors.length) findings.push({ where, kind: 'galat-konsol', detail: errors[0] })
    for (const v of (await axeRun(page, null)) ?? []) findings.push({ where, kind: 'axe:' + v.id, detail: v.n + ' node (' + v.impact + ') ' + v.nodes[0] })
  }
  await ctx.close()
}
await browser.close()

console.log(`Tampilan diaudit: ${views}`)
if (findings.length === 0) {
  console.log('LULUS: tanpa pelanggaran aksesibilitas (WCAG 2 A/AA), overflow, atau galat konsol.')
  process.exit(0)
}
console.log(`GAGAL: ${findings.length} temuan`)
for (const f of findings) console.log(`  [${f.kind}] ${f.where} :: ${f.detail}`)
process.exit(1)
