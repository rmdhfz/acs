// Uji asap pasca-deploy ACS (server UAT/produksi) — satu perintah, tanpa dependensi (Node 20+).
//
//   ACS_SMOKE_API=https://acs.contoh.id/api/v1  ACS_SMOKE_CWMP=https://acs.contoh.id/cwmp \
//   ACS_SMOKE_USER=<superadmin email>  ACS_SMOKE_PASS=<password> \
//   [ACS_SMOKE_CWMP_USER=<secret user tenant> ACS_SMOKE_CWMP_PASS=<secret pass tenant>] \
//   [ACS_SMOKE_APP=https://acs.contoh.id] node scripts/smoke-uat.mjs
//
// Memeriksa: health/readiness, Console, login, katalog (vendor + GENERIC), laporan SaaS, penolakan CWMP tanpa
// kredensial, dan (opsional) Inform nyata dengan secret tenant — device uji "SMOKE-…" yang dibuat dihapus lagi.
// Kredensial hanya dari environment; tidak ada yang dicetak. Exit code 1 bila ada yang GAGAL.
import { randomBytes } from 'node:crypto'

const API = (process.env.ACS_SMOKE_API || 'http://localhost:18080/api/v1').replace(/\/$/, '')
const CWMP = process.env.ACS_SMOKE_CWMP || 'http://localhost:17547/cwmp'
const APP = process.env.ACS_SMOKE_APP || ''
const USER = process.env.ACS_SMOKE_USER
const PASS = process.env.ACS_SMOKE_PASS
const CUSER = process.env.ACS_SMOKE_CWMP_USER
const CPASS = process.env.ACS_SMOKE_CWMP_PASS
const ROOT = API.replace(/\/api\/v1$/, '')

const results = []
async function check(name, fn) {
  const t0 = Date.now()
  try {
    const detail = await fn()
    results.push({ name, ok: true, detail: detail || '', ms: Date.now() - t0 })
  } catch (e) {
    results.push({ name, ok: false, detail: e.message, ms: Date.now() - t0 })
  }
}
function must(cond, msg) {
  if (!cond) throw new Error(msg)
}
async function http(url, opts = {}) {
  const res = await fetch(url, { redirect: 'manual', signal: AbortSignal.timeout(20000), ...opts })
  const text = await res.text()
  let json = null
  try {
    json = JSON.parse(text)
  } catch {
    /* bukan JSON */
  }
  return { res, text, json }
}

await check('liveness /healthz', async () => {
  const { res } = await http(`${ROOT}/healthz`)
  must(res.status === 200, `HTTP ${res.status}`)
})
await check('readiness /readyz (DB)', async () => {
  const { res, json } = await http(`${ROOT}/readyz`)
  must(res.status === 200, `HTTP ${res.status} (DB tidak siap?)`)
  return `database=${json?.checks?.database ?? '?'} redis=${json?.checks?.redis ?? '?'}`
})
if (APP) {
  await check('Console (landing)', async () => {
    const { res, text } = await http(APP + '/')
    must(res.status === 200 && /ACS Console/.test(text), `HTTP ${res.status} atau isi tak sesuai`)
  })
  await check('Console (rute SPA /login)', async () => {
    const { res } = await http(APP + '/login')
    must(res.status === 200, `HTTP ${res.status}`)
  })
  if (APP.startsWith('https://')) {
    await check('HSTS aktif', async () => {
      const { res } = await http(APP + '/')
      must(/max-age=/.test(res.headers.get('strict-transport-security') || ''), 'header Strict-Transport-Security tidak ada')
    })
  }
}
await check('metrik tidak terbuka publik', async () => {
  const { res } = await http(`${API}/metrics`)
  const closed = [401, 403, 404].includes(res.status)
  if (closed) return `HTTP ${res.status}`
  // HTTP langsung ke acsd (dev/LAN) memang terbuka; yang wajib tertutup adalah URL publik di belakang reverse proxy
  must(!API.startsWith('https://'), `HTTP ${res.status} — metrik terbuka di URL publik (blokir /api/v1/metrics di reverse proxy)`)
  return `PERINGATAN: terbuka (HTTP ${res.status}); aman hanya bila jaringan ini tertutup`
})

let token = ''
if (USER && PASS) {
  await check('login superadmin', async () => {
    const { res, json } = await http(`${API}/auth/login`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ email: USER, password: PASS }),
    })
    must(res.status === 200, `HTTP ${res.status}`)
    must(json?.access_token, json?.mfa_required ? 'akun ber-MFA: pakai akun uji tanpa MFA' : 'tanpa access_token')
    token = json.access_token
  })
} else {
  results.push({ name: 'login superadmin', ok: null, detail: 'dilewati (ACS_SMOKE_USER/PASS kosong)', ms: 0 })
}
const auth = () => ({ Authorization: `Bearer ${token}` })

if (token) {
  await check('katalog: ≥ 33 vendor & vendor GENERIC', async () => {
    const { res, json } = await http(`${API}/vendors?page_size=200`, { headers: auth() })
    must(res.status === 200, `HTTP ${res.status}`)
    const list = json?.data ?? []
    must(list.length >= 33, `hanya ${list.length} vendor (migrasi 0029 belum diterapkan?)`)
    must(list.some((v) => v.code === 'GENERIC'), 'vendor GENERIC tidak ada')
    return `${list.length} vendor`
  })
  await check('laporan SaaS', async () => {
    const { res, json } = await http(`${API}/reports/saas`, { headers: auth() })
    must(res.status === 200, `HTTP ${res.status}`)
    return `${json?.platform?.tenants_total ?? '?'} tenant, ${json?.platform?.devices_total ?? '?'} perangkat`
  })
  await check('daftar perangkat', async () => {
    const { res } = await http(`${API}/devices?page_size=1`, { headers: auth() })
    must(res.status === 200, `HTTP ${res.status}`)
  })
}

const serial = 'SMOKE-' + randomBytes(3).toString('hex').toUpperCase()
const inform = `<?xml version="1.0" encoding="UTF-8"?>
<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/" xmlns:cwmp="urn:dslforum-org:cwmp-1-0" xmlns:xsd="http://www.w3.org/2001/XMLSchema">
<soap:Header><cwmp:ID soap:mustUnderstand="1">smoke1</cwmp:ID></soap:Header>
<soap:Body><cwmp:Inform>
<DeviceId><Manufacturer>Smoke</Manufacturer><OUI>SMK001</OUI><ProductClass>SMOKE</ProductClass><SerialNumber>${serial}</SerialNumber></DeviceId>
<Event soap-enc:arrayType="cwmp:EventStruct[1]" xmlns:soap-enc="http://schemas.xmlsoap.org/soap/encoding/"><EventStruct><EventCode>0 BOOTSTRAP</EventCode><CommandKey></CommandKey></EventStruct></Event>
<MaxEnvelopes>1</MaxEnvelopes><CurrentTime>2026-01-01T00:00:00Z</CurrentTime><RetryCount>0</RetryCount>
<ParameterList soap-enc:arrayType="cwmp:ParameterValueStruct[1]" xmlns:soap-enc="http://schemas.xmlsoap.org/soap/encoding/"><ParameterValueStruct><Name>InternetGatewayDevice.DeviceInfo.SoftwareVersion</Name><Value xsi:type="xsd:string" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">smoke-1.0</Value></ParameterValueStruct></ParameterList>
</cwmp:Inform></soap:Body></soap:Envelope>`

await check('CWMP menolak Inform tanpa kredensial (401)', async () => {
  const { res } = await http(CWMP, { method: 'POST', headers: { 'Content-Type': 'text/xml' }, body: inform })
  must(res.status === 401, `HTTP ${res.status} — endpoint CWMP tak meminta autentikasi`)
})

if (CUSER && CPASS) {
await check('CWMP Inform dengan secret tenant', async () => {
    const basic = Buffer.from(`${CUSER}:${CPASS}`).toString('base64')
    const { res, text } = await http(CWMP, { method: 'POST', headers: { 'Content-Type': 'text/xml', Authorization: `Basic ${basic}` }, body: inform })
    must(res.status === 200, `HTTP ${res.status} (secret salah / tenant nonaktif / batas perangkat tercapai?)`)
    must(/InformResponse/.test(text), 'respons bukan InformResponse')
    return `device ${serial} tercatat`
  })
  if (token) {
    await check('bersihkan device uji SMOKE', async () => {
      const { json } = await http(`${API}/devices?search=${serial}&page_size=5`, { headers: auth() })
      const d = (json?.data ?? []).find((x) => x.serial_number === serial)
      must(d, 'device uji tidak ditemukan di API')
      const del = await http(`${API}/devices/${d.id}`, { method: 'DELETE', headers: auth() })
      must(del.res.status < 300, `hapus HTTP ${del.res.status}`)
    })
  }
} else {
  results.push({ name: 'CWMP Inform dengan secret tenant', ok: null, detail: 'dilewati (ACS_SMOKE_CWMP_USER/PASS kosong)', ms: 0 })
}

let fail = 0
console.log(`\nUji asap ACS — API ${API}\n`)
for (const r of results) {
  const tag = r.ok === null ? 'LEWAT' : r.ok ? ' OK  ' : 'GAGAL'
  if (r.ok === false) fail++
  console.log(`  [${tag}] ${r.name}${r.detail ? ' — ' + r.detail : ''}${r.ok === null ? '' : ` (${r.ms} ms)`}`)
}
console.log(`\n${fail === 0 ? 'SEMUA LULUS' : fail + ' PEMERIKSAAN GAGAL'}\n`)
process.exit(fail === 0 ? 0 : 1)
