// Data DEMO untuk tinjauan manual/demo ACS: 2 tenant fiktif, pengguna tiap peran, ±50 perangkat multi-vendor yang
// benar-benar melakukan Inform (simulator cpesim; OUI diambil dari katalog supaya vendor ter-resolve), tag, profil
// provisioning, beberapa task (berhasil, gagal, menunggu), dan penautan pelanggan portal.
//
// Semua lewat REST API + CWMP (jalur nyata, termasuk RBAC) — tidak menyentuh DB langsung. Idempoten: dijalankan ulang
// memakai tenant/pengguna/perangkat yang sama (secret CWMP & password diputar ulang). DATA INI SIMULASI, bukan perangkat nyata.
//
// Prasyarat: stack ACS hidup; binari linux cpesim di ./bin (build:
//   docker run --rm -v "$PWD":/src -w /src -e CGO_ENABLED=0 golang:1.26.9 go build -o bin/cpesim ./cmd/cpesim ).
//
//   ACS_DEMO_USER=<superadmin email> ACS_DEMO_PASS=<password> node scripts/demo-seed.mjs            # seed sekali
//   ACS_DEMO_USER=... ACS_DEMO_PASS=... node scripts/demo-seed.mjs --keepalive                       # + jaga tetap online
//
// Opsional: ACS_DEMO_API (http://localhost:18080/api/v1), ACS_DEMO_CWMP (http://acsd:7547/cwmp — alamat DI DALAM jaringan
// docker), ACS_DEMO_NETWORK (acs_default). Kredensial demo ditulis ke .demo-seed.local.json (git-ignored) dan dicetak sekali.
import { spawnSync } from 'node:child_process'
import { randomBytes } from 'node:crypto'
import fs from 'node:fs'
import path from 'node:path'

const API = (process.env.ACS_DEMO_API || 'http://localhost:18080/api/v1').replace(/\/$/, '')
const CWMP = process.env.ACS_DEMO_CWMP || 'http://acsd:7547/cwmp'
const NETWORK = process.env.ACS_DEMO_NETWORK || 'acs_default'
const SU = process.env.ACS_DEMO_USER
const SP = process.env.ACS_DEMO_PASS
const KEEPALIVE = process.argv.includes('--keepalive')
const OUT = path.resolve('.demo-seed.local.json')
const BIN = path.resolve('bin')
if (!SU || !SP) {
  console.error('Isi ACS_DEMO_USER dan ACS_DEMO_PASS (superadmin).')
  process.exit(2)
}
if (!fs.existsSync(path.join(BIN, 'cpesim'))) {
  console.error('bin/cpesim (linux) belum ada. Build: docker run --rm -v "$PWD":/src -w /src -e CGO_ENABLED=0 golang:1.26.9 go build -o bin/cpesim ./cmd/cpesim')
  process.exit(2)
}

const rnd = (n) => randomBytes(n).toString('base64url').slice(0, n)
const sleep = (ms) => new Promise((r) => setTimeout(r, ms))

async function call(token, method, p, body) {
  for (let attempt = 0; attempt < 4; attempt++) {
    const res = await fetch(API + p, {
      method,
      headers: { 'Content-Type': 'application/json', ...(token ? { Authorization: `Bearer ${token}` } : {}) },
      body: body === undefined ? undefined : JSON.stringify(body),
    })
    if (res.status === 429) {
      await sleep(1500)
      continue
    }
    const text = await res.text()
    let json = null
    try {
      json = JSON.parse(text)
    } catch {
      /* bukan JSON */
    }
    return { status: res.status, json, text }
  }
  throw new Error('rate limit terus-menerus: ' + p)
}
async function ok(token, method, p, body, allow = []) {
  const r = await call(token, method, p, body)
  if (r.status >= 400 && !allow.includes(r.status)) throw new Error(`${method} ${p} -> ${r.status} ${r.text.slice(0, 200)}`)
  return r
}

// ---------- spesifikasi demo ----------
const VENDORS = {
  zte: { code: 'ZTE', profile: 'zte', models: ['F670L', 'F609', 'F660'], prefix: 'ZTEG' },
  huawei: { code: 'HUAWEI', profile: 'huawei', models: ['EG8145V5', 'HG8245H'], prefix: 'HWTC' },
  fiberhome: { code: 'FIBERHOME', profile: 'fiberhome', models: ['HG6145F', 'AN5506-04-FA'], prefix: 'FHTT' },
  nokia: { code: 'NOKIA', profile: 'nokia', models: ['G-1425G-A'], prefix: 'ALCL' },
  cdata: { code: 'CDATA', profile: 'cdata', models: ['FD704GW'], prefix: 'CDTC' },
  tplink: { code: 'TPLINK', profile: 'zte', models: ['XX230v'], prefix: 'TPLK', manufacturer: 'TP-Link' },
  vsol: { code: 'VSOL', profile: 'zte', models: ['V2802GWT'], prefix: 'VSOL', manufacturer: 'V-SOLUTION' },
}
const TENANTS = [
  {
    code: 'NUSAFIBER', name: 'Nusa Fiber', cwmpUser: 'nusafiber-acs', maxDevices: 100,
    mix: { zte: 14, huawei: 9, fiberhome: 5, nokia: 4, tplink: 3, vsol: 3 }, offline: 5, label: 'nusa',
  },
  {
    code: 'CAKRAWALA', name: 'Cakrawala Net', cwmpUser: 'cakrawala-acs', maxDevices: 30,
    mix: { cdata: 6, zte: 4, huawei: 3, nokia: 2 }, offline: 2, label: 'cakra',
  },
]
const hex = (n, w = 8) => (n * 2654435761 >>> 0).toString(16).toUpperCase().padStart(w, '0').slice(0, w)

function buildDevices(t, ouis) {
  const out = []
  let i = 0
  for (const [vk, n] of Object.entries(t.mix)) {
    const v = VENDORS[vk]
    for (let k = 0; k < n; k++, i++) {
      out.push({
        vendor: vk, profile: v.profile, oui: ouis[v.code], manufacturer: v.manufacturer || '',
        model: v.models[k % v.models.length], serial: `${v.prefix}${hex(i + TENANTS.indexOf(t) * 1000 + 17)}`, tenant: t.code,
      })
    }
  }
  // sebagian perangkat sengaja TIDAK dijaga online (menjadi offline sendiri setelah 15 menit)
  out.slice(-t.offline).forEach((d) => (d.letOffline = true))
  return out
}

// ---------- Inform lewat cpesim di jaringan docker ----------
function runInforms(devs, creds, events) {
  const lines = []
  for (const d of devs) {
    const c = creds[d.tenant]
    const args = [
      '/bin/cpesim', '-acs', CWMP, '-user', c.cwmpUser, '-pass', c.cwmpPass, '-vendor', d.profile, '-serial', d.serial,
      '-oui', d.oui, '-model', d.model, '-events', `"${events}"`,
    ]
    if (d.manufacturer) args.push('-manufacturer', `"${d.manufacturer}"`)
    if (d.faultParam) args.push('-fault-param', d.faultParam)
    // batas laju /cwmp 5 req/dtk per IP: jeda antar-perangkat + satu percobaan ulang bila ditolak (429)
    const cmd = args.join(' ') + ' >/dev/null 2>&1'
    lines.push(`${cmd} || { sleep 3; ${cmd}; } || echo "GAGAL ${d.serial}"; sleep 1`)
  }
  const r = spawnSync('docker', ['run', '--rm', '--network', NETWORK, '-v', `${BIN}:/bin_host:ro`, 'alpine:3', 'sh', '-c', 'cp /bin_host/cpesim /bin/cpesim && ' + lines.join('\n')], {
    encoding: 'utf8', env: { ...process.env, MSYS_NO_PATHCONV: '1' },
  })
  if (r.status !== 0) throw new Error('docker run gagal: ' + (r.stderr || '').slice(0, 300))
  const bad = (r.stdout || '').split('\n').filter((l) => l.startsWith('GAGAL'))
  if (bad.length) console.warn('  peringatan Inform:', bad.join(', '))
}

// ---------- main ----------
const login = await ok(null, 'POST', '/auth/login', { email: SU, password: SP })
const su = login.json?.access_token
if (!su) throw new Error('login superadmin gagal (akun ber-MFA tidak didukung)')

// OUI asli vendor dari katalog
const vendors = (await ok(su, 'GET', '/vendors?page_size=200')).json.data ?? []
const ouis = {}
for (const v of Object.values(VENDORS)) {
  const vend = vendors.find((x) => x.code === v.code)
  if (!vend) throw new Error(`vendor ${v.code} tidak ada di katalog (migrasi 0029?)`)
  const list = (await ok(su, 'GET', `/vendors/${vend.id}/ouis`)).json
  if (!list?.length) throw new Error(`vendor ${v.code} tanpa OUI`)
  ouis[v.code] = list[0].oui
}

const state = fs.existsSync(OUT) ? JSON.parse(fs.readFileSync(OUT, 'utf8')) : { tenants: {}, users: [] }

if (!KEEPALIVE) {
  const existingTenants = (await ok(su, 'GET', '/tenants?page_size=200')).json.data ?? []
  for (const t of TENANTS) {
    let row = existingTenants.find((x) => x.code === t.code && !x.is_deleted)
    const cwmpPass = rnd(24)
    if (!row) {
      row = (await ok(su, 'POST', '/tenants', { code: t.code, name: t.name, cwmp_inform_username: t.cwmpUser, cwmp_inform_password: cwmpPass })).json
    } else {
      await ok(su, 'PATCH', `/tenants/${row.id}/cwmp-credentials`, { username: t.cwmpUser, password: cwmpPass })
    }
    await ok(su, 'PATCH', `/tenants/${row.id}/device-limit`, { max_devices: t.maxDevices })
    state.tenants[t.code] = { id: row.id, cwmpUser: t.cwmpUser, cwmpPass }

    // pengguna tiap peran
    const people = [
      { role: 'ADMIN', user: `admin.${t.label}`, name: `Admin ${t.name}` },
      { role: 'NOC', user: `noc.${t.label}`, name: `Petugas NOC ${t.name}` },
      { role: 'VIEWER', user: `viewer.${t.label}`, name: `Pengamat ${t.name}` },
      { role: 'ENDUSER', user: `pelanggan.${t.label}`, name: `Pelanggan Contoh (${t.name})` },
    ]
    const users = (await ok(su, 'GET', `/users?tenant_id=${row.id}&page_size=100`)).json.data ?? []
    for (const p of people) {
      const email = `${p.user}@demo.acs.local`
      const pw = rnd(18) + 'A1!'
      let u = users.find((x) => x.email === email)
      if (!u) {
        u = (await ok(su, 'POST', '/users', { tenant_id: row.id, username: p.user, email, password: pw, full_name: p.name, role_codes: [p.role] })).json
      } else {
        await ok(su, 'PATCH', `/users/${u.id}/password`, { new_password: pw })
      }
      state.users = state.users.filter((x) => x.email !== email)
      state.users.push({ tenant: t.code, role: p.role, email, password: pw, id: u.id })
    }
  }
  fs.writeFileSync(OUT, JSON.stringify(state, null, 2))
  console.log('Tenant & pengguna siap. Inform perangkat…')

  // perangkat: BOOTSTRAP
  const all = TENANTS.flatMap((t) => buildDevices(t, ouis))
  runInforms(all, state.tenants, '0 BOOTSTRAP')

  // objek tingkat tenant lewat ADMIN tenant (RBAC nyata)
  for (const t of TENANTS) {
    const adm = state.users.find((u) => u.tenant === t.code && u.role === 'ADMIN')
    const lg = await ok(null, 'POST', '/auth/login', { email: adm.email, password: adm.password })
    const tk = lg.json.access_token
    const devs = (await ok(tk, 'GET', '/devices?page_size=200')).json.data ?? []
    const mine = buildDevices(t, ouis).map((d) => devs.find((x) => x.serial_number === d.serial)).filter(Boolean)

    // tag
    const tagDefs = [['Pelanggan korporat', '#6366f1'], ['Gedung A', '#0ea5e9'], ['Perlu dipantau', '#f59e0b']]
    const tags = []
    const have = (await ok(tk, 'GET', '/tags?page_size=100')).json.data ?? []
    for (const [name, color] of tagDefs) {
      let tg = have.find((x) => x.name === name)
      if (!tg) tg = (await ok(tk, 'POST', '/tags', { name, color })).json
      tags.push(tg)
    }
    mine.slice(0, 6).forEach(() => {})
    for (const [i, d] of mine.entries()) {
      const tg = i % 7 === 0 ? tags[0] : i % 5 === 0 ? tags[1] : i % 11 === 0 ? tags[2] : null
      if (tg) await ok(tk, 'POST', `/devices/${d.id}/tags`, { tag_id: tg.id }, [409])
    }

    // profil provisioning
    const profName = `SSID standar ${t.name}`
    const profs = (await ok(tk, 'GET', '/provisioning-profiles?page_size=100')).json.data ?? []
    let prof = profs.find((p) => p.name === profName)
    if (!prof) {
      prof = (await ok(tk, 'POST', '/provisioning-profiles', {
        tenant_id: state.tenants[t.code].id, name: profName, description: 'SSID dan interval Inform standar operator',
        parameters: [
          { parameter_name: 'wifi.ssid', parameter_value: `${t.name.replace(/\s+/g, '')}-Home`, apply_order: 1 },
          { parameter_name: 'device.periodic_inform_interval', parameter_value: '300', apply_order: 2 },
        ],
      })).json
    }

    // task: terapkan profil (2), reboot (2), ubah SSID lewat logical key (2), satu menunggu yang gagal
    const pick = (n, off = 0) => mine.slice(off, off + n)
    for (const d of pick(2, 0)) await ok(tk, 'POST', `/devices/${d.id}/apply-profile/${prof.id}`, {}, [409])
    for (const d of pick(2, 2)) await ok(tk, 'POST', `/devices/${d.id}/reboot`, {}, [409])
    for (const d of pick(2, 4)) await ok(tk, 'POST', `/devices/${d.id}/tasks/set-parameter-values`, { values: { 'wifi.ssid': `Kantor-${d.serial_number.slice(-4)}` } }, [409])
    // gagal sungguhan: CPE menjawab Fault 9005 untuk SSID
    const failing = mine[6]
    if (failing) await ok(tk, 'POST', `/devices/${failing.id}/tasks/set-parameter-values`, { values: { 'wifi.ssid': 'Gagal-Uji' } }, [409])
    // pelanggan portal: tautkan ke satu perangkat
    const cust = state.users.find((u) => u.tenant === t.code && u.role === 'ENDUSER')
    if (mine[8]) await ok(tk, 'PUT', `/users/${cust.id}/devices/${mine[8].id}`, {}, [409])
  }

  // sesi PERIODIC: perangkat mengambil task (perangkat #7 tiap tenant menjawab Fault 9005 untuk SSID)
  const round = TENANTS.flatMap((t) => buildDevices(t, ouis))
  TENANTS.forEach((t) => {
    const first = round.filter((d) => d.tenant === t.code)[6]
    if (first) first.faultParam = 'SSID'
  })
  runInforms(round, state.tenants, '2 PERIODIC')
  console.log('\nSelesai. Akun demo (disimpan di .demo-seed.local.json, jangan di-commit):')
  for (const u of state.users) console.log(`  ${u.tenant.padEnd(10)} ${u.role.padEnd(8)} ${u.email}  ${u.password}`)
  console.log('\nPerangkat menjadi OFFLINE bila tak Inform 15 menit. Jaga tetap hidup: node scripts/demo-seed.mjs --keepalive')
} else {
  if (!Object.keys(state.tenants).length) throw new Error('jalankan seed dulu (tanpa --keepalive)')
  console.log('Keepalive: Inform PERIODIC tiap 5 menit untuk perangkat demo (sebagian sengaja dibiarkan offline). Ctrl+C untuk berhenti.')
  for (;;) {
    const live = TENANTS.flatMap((t) => buildDevices(t, ouis)).filter((d) => !d.letOffline)
    runInforms(live, state.tenants, '2 PERIODIC')
    console.log(new Date().toLocaleTimeString('id-ID'), `${live.length} perangkat di-Inform`)
    await sleep(5 * 60 * 1000)
  }
}
