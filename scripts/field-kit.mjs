// Pengumpul bukti uji lapangan ACS (Node 20+, tanpa dependensi). Menghasilkan laporan Markdown yang aman dibagikan (nilai rahasia disamarkan, serial
// disamarkan bila tidak diminta lain). Lihat docs/pengujian/KIT_UJI_LAPANGAN.md untuk prosedur lengkap dan cara membaca hasilnya.
//
//   ACS_KIT_API=https://acs.contoh.id/api/v1  ACS_KIT_USER=<email|username> ACS_KIT_PASS=<password>   (atau ACS_KIT_TOKEN=<JWT>)
//   node scripts/field-kit.mjs --serial ELWGC09CED21 [--serial ...] [--all [--limit 20]]
//        [--queue-discovery] [--queue-values] [--try-cr [--wait 60]] [--olt-serials berkas.txt] [--show-serials] [--out laporan.md]
//
// BAWAAN HANYA-BACA. Aksi yang mengubah keadaan ACS/perangkat harus diminta eksplisit:
//   --queue-discovery  antre GetParameterNames <root>ManagementServer. (satu RPC baca; dijalankan perangkat pada Inform berikutnya)
//   --queue-values     antre GetParameterValues untuk nama STUN/NAT yang SUDAH ditemukan dan bukan rahasia
//   --try-cr           kirim satu Connection Request lalu tunggu event "6 CONNECTION REQUEST" (bukti CR); tidak mengubah konfigurasi
// Kredensial hanya dari environment dan tidak pernah dicetak. Exit code 1 bila tidak dapat login atau tidak ada perangkat yang ditemukan.
import { writeFileSync, readFileSync } from 'node:fs'
import {
  ROOTS, compareSerials, crVerdict, describeSerials, maskSerial, redactValue, rootOf, stunVerdict, table,
} from './field-kit-lib.mjs'

const API = (process.env.ACS_KIT_API || 'http://localhost:18080/api/v1').replace(/\/$/, '')

function parseArgs(argv) {
  const a = { serials: [], all: false, limit: 20, queueDiscovery: false, queueValues: false, tryCr: false, wait: 60, oltSerials: '', showSerials: false, out: 'field-kit-report.md' }
  for (let i = 0; i < argv.length; i++) {
    const k = argv[i]
    const next = () => argv[++i]
    if (k === '--serial') a.serials.push(next())
    else if (k === '--all') a.all = true
    else if (k === '--limit') a.limit = Math.max(1, Math.min(200, Number(next()) || 20))
    else if (k === '--queue-discovery') a.queueDiscovery = true
    else if (k === '--queue-values') a.queueValues = true
    else if (k === '--try-cr') a.tryCr = true
    else if (k === '--wait') a.wait = Math.max(5, Math.min(600, Number(next()) || 60))
    else if (k === '--olt-serials') a.oltSerials = next()
    else if (k === '--show-serials') a.showSerials = true
    else if (k === '--out') a.out = next()
    else throw new Error(`argumen tidak dikenal: ${k}`)
  }
  return a
}

let token = process.env.ACS_KIT_TOKEN || ''

async function api(method, path, body) {
  const res = await fetch(API + path, {
    method,
    headers: { 'Content-Type': 'application/json', ...(token ? { Authorization: `Bearer ${token}` } : {}) },
    body: body === undefined ? undefined : JSON.stringify(body),
    signal: AbortSignal.timeout(30000),
  })
  const text = await res.text()
  let json = null
  try {
    json = text ? JSON.parse(text) : null
  } catch {
    /* bukan JSON */
  }
  return { status: res.status, json, text }
}

async function login() {
  if (token) return
  const user = process.env.ACS_KIT_USER
  const pass = process.env.ACS_KIT_PASS
  if (!user || !pass) throw new Error('isi ACS_KIT_TOKEN, atau ACS_KIT_USER dan ACS_KIT_PASS')
  const r = await api('POST', '/auth/login', user.includes('@') ? { email: user, password: pass } : { username: user, password: pass })
  if (r.json?.mfa_required) throw new Error('akun meminta MFA: login lewat Console lalu pakai ACS_KIT_TOKEN')
  if (r.status !== 200 || !r.json?.access_token) throw new Error(`login gagal (HTTP ${r.status})`)
  token = r.json.access_token
}

const list = (r) => (Array.isArray(r.json) ? r.json : (r.json?.data ?? []))

async function findDevice(serial) {
  const r = await api('GET', `/devices?search=${encodeURIComponent(serial)}&page_size=20`)
  return list(r).find((d) => String(d.serial_number).toLowerCase() === serial.toLowerCase()) ?? null
}

async function allDevices(limit) {
  const out = []
  for (let page = 1; out.length < limit; page++) {
    const r = await api('GET', `/devices?page=${page}&page_size=${Math.min(200, limit - out.length)}`)
    const rows = list(r)
    out.push(...rows)
    if (rows.length === 0 || out.length >= (r.json?.total ?? 0)) break
  }
  return out.slice(0, limit)
}

async function params(id, prefix) {
  const r = await api('GET', `/devices/${id}/parameters?prefix=${encodeURIComponent(prefix)}`)
  return list(r).map((p) => ({ name: p.parameter_name, value: p.parameter_value }))
}

async function detectRoot(id) {
  for (const r of ROOTS) {
    const ps = await params(id, r + 'DeviceInfo.')
    if (rootOf(ps.map((p) => p.name))) return r
  }
  return null
}

const sleep = (ms) => new Promise((r) => setTimeout(r, ms))

async function deviceSection(d, opts, eventCodes) {
  const serial = maskSerial(d.serial_number, opts.showSerials)
  const lines = [`## Perangkat ${serial}`, '', `- Model: ${d.product_class ?? '—'} · OUI ${d.oui ?? '—'} · firmware ${d.software_version ?? '—'}`]
  const root = await detectRoot(d.id)
  lines.push(`- Model data: ${root ?? 'belum diketahui (belum ada parameter DeviceInfo yang tersimpan)'}`)
  lines.push(`- Connection Request URL: ${d.connection_request_url ?? '—'} · kredensial CR ${d.connection_request_credentials_set ? 'terisi' : 'BELUM terisi'}`)

  // ---- Langkah 0: STUN/TR-111 ----
  lines.push('', '### Langkah 0 — dukungan STUN/TR-111 (CONNECTION_REQUEST_NAT_DESIGN.md §4)', '')
  if (!root) {
    lines.push('Tidak dapat diperiksa: model data perangkat belum diketahui.')
  } else {
    if (opts.queueDiscovery) {
      const r = await api('POST', `/devices/${d.id}/tasks/get-parameter-names`, { path: root + 'ManagementServer.', next_level: true })
      lines.push(r.status < 300 ? '_Tugas penelusuran nama diantre; jalan pada Inform berikutnya (picu Connection Request atau tunggu Periodic Inform), lalu ulangi laporan._' : `_Gagal mengantre penelusuran nama: HTTP ${r.status}._`, '')
    }
    const ms = await params(d.id, root + 'ManagementServer.')
    const v = stunVerdict(ms)
    lines.push(`**Hasil: ${v.status}**`, '', ...v.notes.map((n) => `- ${n}`), '')
    if (opts.queueValues) {
      const names = ms.filter((p) => /STUN|UDPConnectionRequest|NATDetected|NATRedirected/i.test(p.name.split('.').pop() ?? '') && redactValue(p.name, 'x') !== '[disembunyikan]').map((p) => p.name)
      if (names.length === 0) lines.push('_Tidak ada nama STUN/NAT non-rahasia yang bisa dibaca nilainya (jalankan --queue-discovery dulu)._', '')
      else {
        const r = await api('POST', `/devices/${d.id}/tasks/get-parameter-values`, { names })
        lines.push(r.status < 300 ? `_Tugas baca ${names.length} nilai diantre; ulangi laporan setelah Inform berikutnya._` : `_Gagal mengantre pembacaan nilai: HTTP ${r.status}._`, '')
      }
    }
    lines.push('Parameter `ManagementServer.*` yang dikenal ACS (nilai rahasia disamarkan; periksa manusia sebelum dibagikan):', '')
    lines.push(table(['Nama', 'Nilai'], ms.sort((a, b) => a.name.localeCompare(b.name)).map((p) => [p.name.replace(root + 'ManagementServer.', ''), redactValue(p.name, p.value)])), '')
  }

  // ---- Bukti Connection Request ----
  lines.push('### Bukti Connection Request', '')
  if (!opts.tryCr) {
    lines.push('Tidak dijalankan (tambahkan `--try-cr`).', '')
  } else {
    const startedAt = Date.now()
    const send = await api('POST', `/devices/${d.id}/connection-request`)
    const sendOk = send.status < 300
    let events = []
    if (sendOk) {
      for (let waited = 0; waited < opts.wait * 1000; waited += 3000) {
        await sleep(3000)
        const r = await api('GET', `/devices/${d.id}/events?page_size=20`)
        events = list(r).map((e) => ({ code: eventCodes.get(e.event_code_id) ?? String(e.event_code_id), occurred_at: e.occurred_at }))
        if (crVerdict(events, startedAt, true).status === 'TERBUKTI') break
      }
    }
    const v = crVerdict(events, startedAt, sendOk)
    lines.push(`**Hasil: ${v.status}**`, '', `- ${v.note}`, ...(sendOk ? [] : [`- Respons ACS: HTTP ${send.status} ${String(send.json?.message ?? send.text).slice(0, 200)}`]), '')
  }

  // ---- Optik dua arah ----
  lines.push('### Redaman optik (ONT dan OLT)', '')
  const opt = list(await api('GET', `/devices/${d.id}/optical-metrics?page_size=1`))[0]
  const olt = (await api('GET', `/devices/${d.id}/olt`)).json
  const oltRx = olt?.linked ? olt.onu?.rx_power_dbm : null
  lines.push(
    table(['Sisi', 'RX (dBm)', 'Waktu / keadaan'], [
      ['ONT (downstream, via TR-069)', opt?.rx_power_dbm ?? '—', opt?.recorded_at ?? 'belum ada pembacaan'],
      ['OLT (upstream)', oltRx ?? '—', olt?.linked ? `${olt.onu.state} · polling ${olt.onu.olt_last_poll_at ?? '—'}` : olt?.tenant_has_olt ? 'ONU tidak tertaut ke OLT' : 'tenant tanpa OLT'],
    ]),
    '',
    '_Keduanya arah berbeda dan bukan besaran yang sama; bandingkan tingkatnya, jangan dikurangkan._',
    '',
  )
  return lines.join('\n')
}

async function oltSection(opts, acsSerials) {
  const lines = ['## OLT: format serial dan keadaan (butir 7–8 OLT_CONTROLLER_DESIGN.md §8)', '']
  const olts = list(await api('GET', '/olts?page_size=50'))
  const oltSerials = []
  const states = new Map()
  for (const o of olts) {
    for (let page = 1; page <= 5; page++) {
      const r = await api('GET', `/olts/${o.id}/registered-onus?page=${page}&page_size=200`)
      const rows = list(r)
      for (const x of rows) {
        oltSerials.push(x.serial_number)
        const key = `${x.vendor_state ?? '(kosong)'} → ${x.state}`
        states.set(key, (states.get(key) ?? 0) + 1)
      }
      if (rows.length < 200) break
    }
  }
  if (opts.oltSerials) {
    const extra = readFileSync(opts.oltSerials, 'utf8').split(/\r?\n/).map((s) => s.trim()).filter(Boolean)
    oltSerials.push(...extra)
    lines.push(`_${extra.length} serial tambahan dari berkas ${opts.oltSerials} (mis. ditempel dari keluaran OLT)._`, '')
  }
  if (oltSerials.length === 0) {
    lines.push('Tidak ada serial dari OLT (tidak ada OLT/ONU terdaftar di ACS dan tidak ada `--olt-serials`). Dengan driver simulator data ini palsu; jangan dipakai sebagai bukti.')
    return lines.join('\n')
  }
  const m = (s) => maskSerial(s, opts.showSerials)
  const dOlt = describeSerials(oltSerials)
  const dAcs = describeSerials(acsSerials)
  const desc = (d) => `${d.total} serial · panjang ${d.lengths.map(([l, n]) => `${l}×${n}`).join(', ')} · heksa murni: ${d.allHex ? 'ya' : 'tidak'} · huruf kecil: ${d.withLowercase} · berpemisah: ${d.withSeparator} · awalan: ${d.topPrefixes.map(([p, n]) => `${p.slice(0, 3)}*×${n}`).join(', ')}`
  lines.push(`- Sisi OLT: ${desc(dOlt)}`, `- Sisi ACS (TR-069): ${desc(dAcs)}`, '')
  const c = compareSerials(oltSerials, acsSerials)
  lines.push(`Aturan tautan sistem (sama persis, tak peka huruf besar/kecil): **${c.matched.length} cocok** dari ${oltSerials.length} serial OLT.`, '')
  if (c.candidates.length) {
    lines.push('**Kandidat yang TIDAK tertaut tetapi tampak sama setelah transformasi** (petunjuk bahwa normalisasi per driver dibutuhkan; belum tentu pasangan yang benar):', '')
    lines.push(table(['Serial OLT', 'Serial ACS', 'Transformasi'], c.candidates.slice(0, 20).map((x) => [m(x.olt), m(x.acs), x.how])), '')
  }
  if (c.unmatched.length) lines.push(`${c.unmatched.length} serial OLT tidak punya pasangan sama sekali di ACS (mis. ONU belum pernah Inform): ${c.unmatched.slice(0, 5).map(m).join(', ')}${c.unmatched.length > 5 ? ', …' : ''}`, '')
  if (states.size) lines.push('**Nilai keadaan asli dari OLT dan pemetaannya** (dasar butir 7; yang `→ UNKNOWN` belum terpetakan):', '', table(['Keadaan asli → keadaan netral', 'Jumlah'], [...states.entries()].sort((a, b) => b[1] - a[1])), '')
  return lines.join('\n')
}

async function main() {
  const opts = parseArgs(process.argv.slice(2))
  if (opts.serials.length === 0 && !opts.all) throw new Error('beri --serial <SN> (boleh berulang) atau --all')
  await login()
  const codes = new Map(list(await api('GET', '/refs/ref_event_codes')).map((r) => [r.id, r.code]))
  const devices = opts.all ? await allDevices(opts.limit) : (await Promise.all(opts.serials.map(findDevice))).filter(Boolean)
  const missing = opts.serials.filter((s) => !devices.some((d) => String(d.serial_number).toLowerCase() === s.toLowerCase()))
  if (devices.length === 0) throw new Error('tidak ada perangkat yang ditemukan')

  const acsSerials = (await allDevices(2000)).map((d) => d.serial_number)
  const parts = [
    '# Laporan bukti uji lapangan ACS',
    '',
    `Dibuat ${new Date().toISOString()} dari ${API}. Mode: ${[opts.queueDiscovery && 'antre-penelusuran', opts.queueValues && 'antre-nilai', opts.tryCr && 'coba-CR'].filter(Boolean).join(', ') || 'hanya-baca'}.`,
    '',
    '> Baca sebelum dibagikan: nilai bernama kredensial disamarkan berdasarkan NAMA, jadi nama tak lazim bisa lolos. Serial disamarkan kecuali `--show-serials`.',
    '',
  ]
  if (missing.length) parts.push(`**Serial tidak ditemukan di ACS:** ${missing.map((s) => maskSerial(s, opts.showSerials)).join(', ')}`, '')
  for (const d of devices) parts.push(await deviceSection(d, opts, codes), '')
  parts.push(await oltSection(opts, acsSerials), '')
  writeFileSync(opts.out, parts.join('\n'), 'utf8')
  console.log(`Laporan ditulis ke ${opts.out} (${devices.length} perangkat). Baca dulu sebelum dibagikan.`)
}

main().catch((e) => {
  console.error('GAGAL:', e.message)
  process.exit(1)
})
