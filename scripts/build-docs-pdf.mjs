// Membuat ulang PDF dari sumber HTML di docs/ memakai Chrome/Chromium headless (tanpa dependensi npm).
//   node scripts/build-docs-pdf.mjs            # semua
//   node scripts/build-docs-pdf.mjs UAT        # hanya berkas yang namanya memuat "UAT"
// Opsional: CHROME_PATH. HTML adalah sumber; jangan menyunting PDF langsung.
import { spawnSync } from 'node:child_process'
import fs from 'node:fs'
import path from 'node:path'
import { pathToFileURL } from 'node:url'

const root = path.resolve(import.meta.dirname, '..')
const only = process.argv[2]?.toLowerCase()
const chrome = [
  process.env.CHROME_PATH,
  'C:/Program Files/Google/Chrome/Application/chrome.exe',
  'C:/Program Files (x86)/Google/Chrome/Application/chrome.exe',
  '/usr/bin/google-chrome',
  '/usr/bin/chromium',
  '/usr/bin/chromium-browser',
  '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
].filter(Boolean).find((p) => fs.existsSync(p))
if (!chrome) {
  console.error('Chrome/Chromium tidak ditemukan; set CHROME_PATH.')
  process.exit(2)
}

// Hanya dokumen "hidup" (bukan arsip) yang punya padanan PDF.
const sources = []
for (const dir of ['panduan', 'pengujian']) {
  const d = path.join(root, 'docs', dir)
  for (const f of fs.readdirSync(d)) if (f.endsWith('.html')) sources.push(path.join(d, f))
}

let failed = 0
for (const src of sources) {
  if (only && !path.basename(src).toLowerCase().includes(only)) continue
  const out = src.replace(/\.html$/, '.pdf')
  const r = spawnSync(chrome, ['--headless=new', '--disable-gpu', '--no-pdf-header-footer', '--virtual-time-budget=25000', `--print-to-pdf=${out}`, pathToFileURL(src).href], { encoding: 'utf8' })
  const ok = r.status === 0 && fs.existsSync(out)
  console.log(`${ok ? 'OK   ' : 'GAGAL'} ${path.relative(root, out)}${ok ? ` (${Math.round(fs.statSync(out).size / 1024)} KB)` : ''}`)
  if (!ok) failed++
}
process.exit(failed ? 1 : 0)
