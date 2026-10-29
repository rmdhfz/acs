// Fungsi murni untuk scripts/field-kit.mjs dan scripts/mask-olt-output.mjs (tanpa jaringan, tanpa berkas) supaya aturannya bisa diuji sendiri:
//   node --test scripts/field-kit.test.mjs
// Prinsip: tidak menebak. Yang tidak dapat disimpulkan dari data dilaporkan "belum diketahui", bukan ditebak ke satu arah.

export const ROOTS = ['InternetGatewayDevice.', 'Device.']

// rootOf: root model data (TR-098/TR-181) dari nama parameter yang sudah dikenal ACS; null bila belum ada.
export function rootOf(names) {
  for (const r of ROOTS) if (names.some((n) => n.startsWith(r))) return r
  return null
}

// Nama yang nilainya tidak boleh ikut laporan (kredensial). Berbasis nama, jadi daftar ini bisa kecolongan untuk nama tak lazim: laporan tetap harus
// dibaca sebelum dibagikan.
const SECRET_NAME = /pass(word|phrase)?|secret|psk|key|token|credential|username|community/i
export function isSecretName(name) {
  return SECRET_NAME.test(name)
}
export function redactValue(name, value) {
  if (value == null || value === '') return value ?? ''
  return isSecretName(name) ? '[disembunyikan]' : String(value)
}

// ---- Langkah 0: dukungan STUN/TR-111 (CONNECTION_REQUEST_NAT_DESIGN.md §4) ----

const STUN_NAME = /(STUN|UDPConnectionRequest|NATDetected|NATRedirected)/i
// Ambang jumlah nama ManagementServer.* di atas mana daftar dianggap hasil penelusuran GetParameterNames (bukan hanya yang ikut Inform). Heuristik:
// Inform biasa hanya memuat beberapa parameter ManagementServer.*. Itu sebabnya hasilnya selalu disertai daftar nama mentah untuk diperiksa manusia.
export const MS_DISCOVERED_MIN_NAMES = 12

// stunVerdict: params = [{name, value}] di bawah <root>ManagementServer.
export function stunVerdict(params) {
  const msNames = params.map((p) => p.name)
  const stun = params.filter((p) => STUN_NAME.test(p.name.split('.').pop() ?? ''))
  const get = (suffix) => params.find((p) => p.name.endsWith('.ManagementServer.' + suffix))
  const enable = get('STUNEnable')
  const udp = get('UDPConnectionRequestAddress')
  const nat = get('NATDetected')
  const truthy = (v) => v === '1' || String(v).toLowerCase() === 'true'
  if (stun.length > 0) {
    const notes = [`ONT mengekspos ${stun.length} parameter terkait STUN/NAT (daftar di bawah).`]
    if (enable?.value != null && enable.value !== '') notes.push(`STUNEnable = ${enable.value} (${truthy(enable.value) ? 'aktif' : 'nonaktif'}).`)
    else if (enable) notes.push('STUNEnable ada tetapi nilainya belum dibaca (jalankan GetParameterValues).')
    if (udp?.value) notes.push('UDPConnectionRequestAddress terisi: ONT melaporkan alamat UDP Connection Request.')
    if (nat?.value != null && nat.value !== '') notes.push(`NATDetected = ${nat.value}.`)
    return { status: 'DIDUKUNG', notes }
  }
  if (msNames.length >= MS_DISCOVERED_MIN_NAMES) {
    return {
      status: 'TAMPAK_TIDAK_DIDUKUNG',
      notes: [`${msNames.length} nama ManagementServer.* dikenal ACS dan tidak satu pun terkait STUN/NAT. Periksa daftar nama: bila daftarnya tampak lengkap, ONT ini tidak mendukung TR-111.`],
    }
  }
  return {
    status: 'BELUM_DIKETAHUI',
    notes: [`Hanya ${msNames.length} nama ManagementServer.* yang dikenal ACS (kemungkinan hanya yang ikut Inform). Jalankan penelusuran nama (field-kit --queue-discovery), tunggu Inform, lalu ulangi laporan.`],
  }
}

// ---- Butir 8 OLT: format serial ONU di OLT dibanding serial TR-069 ----

export const normSerial = (s) => String(s).replace(/[^A-Za-z0-9]/g, '').toUpperCase()

// hexAsciiDecode: "5A54454743..." -> "ZTEGC..." bila seluruhnya heksa genap dan hasilnya ASCII tercetak; selain itu null.
export function hexAsciiDecode(s) {
  const h = normSerial(s)
  if (h.length < 8 || h.length % 2 !== 0 || !/^[0-9A-F]+$/.test(h)) return null
  let out = ''
  for (let i = 0; i < h.length; i += 2) {
    const c = parseInt(h.slice(i, i + 2), 16)
    if (c < 0x30 || c > 0x7a) return null // hanya digit/huruf (dan sedikit tanda di antaranya): bukan teks serial bila di luar rentang
    out += String.fromCharCode(c)
  }
  return /^[0-9A-Za-z]+$/.test(out) ? out.toUpperCase() : null
}

export function describeSerials(list) {
  const lens = new Map()
  let hex = 0
  let digitsOnly = 0
  let lower = 0
  let hasSep = 0
  const prefixes = new Map()
  for (const s of list) {
    lens.set(s.length, (lens.get(s.length) ?? 0) + 1)
    if (/^[0-9A-Fa-f]+$/.test(s)) hex++
    if (/^\d+$/.test(s)) digitsOnly++
    if (/[a-z]/.test(s)) lower++
    if (/[^A-Za-z0-9]/.test(s)) hasSep++
    const p = s.slice(0, 4).toUpperCase()
    prefixes.set(p, (prefixes.get(p) ?? 0) + 1)
  }
  const top = (m, n) => [...m.entries()].sort((a, b) => b[1] - a[1]).slice(0, n)
  return { total: list.length, lengths: top(lens, 5), allHex: list.length > 0 && hex === list.length, digitsOnly, withLowercase: lower, withSeparator: hasSep, topPrefixes: top(prefixes, 5) }
}

// compareSerials: kecocokan OLT <-> ACS menurut aturan tautan yang dipakai sistem (sama persis, tak peka huruf besar/kecil) dan kandidat yang TIDAK cocok
// menurut aturan itu tetapi cocok setelah transformasi umum (membuang pemisah, awalan/akhiran, heksa<->teks). Kandidat adalah petunjuk, bukan kepastian.
export function compareSerials(oltSerials, acsSerials) {
  const acsExact = new Map(acsSerials.map((s) => [s.toLowerCase(), s]))
  const acsNorm = new Map(acsSerials.map((s) => [normSerial(s), s]))
  const matched = []
  const candidates = []
  const unmatched = []
  for (const o of oltSerials) {
    if (acsExact.has(o.toLowerCase())) {
      matched.push(o)
      continue
    }
    const n = normSerial(o)
    let how = null
    let hit = null
    if (acsNorm.has(n)) {
      how = 'sama setelah membuang pemisah/tanda baca'
      hit = acsNorm.get(n)
    }
    if (!hit) {
      const dec = hexAsciiDecode(o)
      if (dec && acsNorm.has(dec)) {
        how = 'serial OLT berupa heksa dari teks serial ACS'
        hit = acsNorm.get(dec)
      }
    }
    if (!hit) {
      for (const [an, a] of acsNorm) {
        const decA = hexAsciiDecode(a)
        if (decA && decA === n) {
          how = 'serial ACS berupa heksa dari teks serial OLT'
          hit = a
          break
        }
        if (an.length >= 8 && n.length >= 8 && an !== n && (n.endsWith(an) || an.endsWith(n) || n.startsWith(an) || an.startsWith(n))) {
          const longer = n.length > an.length ? n : an
          const shorter = n.length > an.length ? an : n
          const side = n.length > an.length ? 'OLT' : 'ACS'
          how = longer.endsWith(shorter) ? `serial ${side} memiliki awalan tambahan` : `serial ${side} memiliki akhiran tambahan`
          hit = a
          break
        }
      }
    }
    if (hit) candidates.push({ olt: o, acs: hit, how })
    else unmatched.push(o)
  }
  return { matched, candidates, unmatched }
}

// maskSerial: pertahankan panjang dan 3 karakter pertama (untuk melihat awalan vendor); sisanya "*". Serial utuh hanya bila diminta eksplisit.
export function maskSerial(s, show = false) {
  if (show) return s
  const str = String(s)
  return str.length <= 3 ? '*'.repeat(str.length) : str.slice(0, 3) + '*'.repeat(str.length - 3)
}

// ---- Pembuktian Connection Request ----

// crVerdict: events = [{code, occurred_at}] terbaru dulu; startedAt = waktu CR dikirim (ms). Bukti hanya bila event "6 CONNECTION REQUEST" muncul SETELAH CR dikirim.
export function crVerdict(events, startedAt, sendOk) {
  if (!sendOk) return { status: 'GAGAL_DIKIRIM', note: 'ACS menolak/gagal mengirim Connection Request (lihat pesan galat).' }
  const hit = events.find((e) => e.code === '6 CONNECTION REQUEST' && new Date(e.occurred_at).getTime() >= startedAt - 1000)
  if (hit) {
    const latency = Math.max(0, Math.round((new Date(hit.occurred_at).getTime() - startedAt) / 1000))
    return { status: 'TERBUKTI', note: `ONT melakukan Inform dengan event "6 CONNECTION REQUEST" ${latency} dtk setelah CR dikirim.`, latencySeconds: latency }
  }
  return { status: 'TIDAK_TERBUKTI', note: 'Tidak ada Inform dengan event "6 CONNECTION REQUEST" setelah CR dikirim dalam jendela tunggu. Bisa karena ONT tak terjangkau, kredensial CR salah, atau jendela terlalu pendek; belum tentu ONT tak mendukung.' }
}

// ---- Penyamaran keluaran OLT (mask-olt-output.mjs) ----

// maskOltOutput: menyamarkan nilai yang jelas rahasia pada keluaran CLI/SNMP yang akan ditempel. Pola umum, bukan khusus vendor: tetap WAJIB dibaca
// manusia sebelum dibagikan. Serial ONU TIDAK disamarkan (diperlukan untuk memeriksa format); gunakan --mask-serials untuk menyamarkannya.
export function maskOltOutput(text, { maskSerials = false } = {}) {
  let out = text
  // "password xxx", "community xxx", "secret xxx", "key xxx", "passwd: xxx", "pwd=xxx"
  out = out.replace(/\b(pass(?:word|wd)?|pwd|secret|community|key|token)(\s*[:=]?\s+|\s*[:=]\s*)(\S+)/gi, (_m, k, sep) => `${k}${sep}[disamarkan]`)
  // SNMPv3 / "auth ... priv ..." tidak ditebak; baris yang memuat kata kunci rahasia dan nilai panjang acak tak ditangani di sini (peringatan di dokumen).
  // alamat IPv4: pertahankan dua oktet pertama (jenis jaringan), samarkan dua terakhir
  out = out.replace(/\b(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})\b/g, (_m, a, b) => `${a}.${b}.x.x`)
  // MAC address
  out = out.replace(/\b([0-9A-Fa-f]{2})([:.-])[0-9A-Fa-f]{2}(?:\2[0-9A-Fa-f]{2}){4}\b/g, (_m, a) => `${a}:xx:xx:xx:xx:xx`)
  if (maskSerials) out = out.replace(/\b[A-Za-z]{4}[0-9A-Fa-f]{8}\b/g, (m) => m.slice(0, 4) + '********')
  return out
}

// ---- Laporan ----

export function table(headers, rows) {
  const esc = (v) => String(v ?? '').replace(/\|/g, '\\|').replace(/\n/g, ' ')
  return [`| ${headers.join(' | ')} |`, `|${headers.map(() => '---').join('|')}|`, ...rows.map((r) => `| ${r.map(esc).join(' | ')} |`)].join('\n')
}
