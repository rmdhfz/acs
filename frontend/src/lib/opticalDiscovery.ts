import type { DeviceParameter } from './types'

// Penemuan sumber data redaman optik dari parameter yang sudah dilaporkan ONT. Path optik TR-098 SELALU ekstensi vendor
// (X_ZTE-COM_*, X_CT-COM_*, ...) sehingga tidak boleh ditebak: kita mengenali KANDIDAT dari nama parameter yang benar-benar
// ada di perangkat, menampilkan nilai mentahnya, dan operator memilih skala satuan sebelum disimpan sebagai mapping (data).

export type OpticalRole = 'rx' | 'tx' | 'temperature' | 'voltage' | 'bias'

/** Logical key yang dipakai polling optik (usecase/task EnqueueOpticalPoll) — jangan diubah sepihak. */
export const OPTICAL_LOGICAL_KEY: Record<OpticalRole, string> = {
  rx: 'optical.rx_power',
  tx: 'optical.tx_power',
  temperature: 'optical.temperature',
  voltage: 'optical.voltage',
  bias: 'optical.bias_current',
}

export const OPTICAL_ROLE_LABEL: Record<OpticalRole, string> = {
  rx: 'RX power',
  tx: 'TX power',
  temperature: 'Suhu',
  voltage: 'Tegangan',
  bias: 'Bias current',
}

export const OPTICAL_ROLE_UNIT: Record<OpticalRole, string> = { rx: 'dBm', tx: 'dBm', temperature: '°C', voltage: 'V', bias: 'mA' }

// Rentang wajar per satuan kanonik; dipakai menyarankan skala dan memberi peringatan, bukan memvalidasi keras.
const PLAUSIBLE: Record<OpticalRole, [number, number]> = {
  rx: [-50, 10],
  tx: [-10, 15],
  temperature: [-20, 130],
  voltage: [0, 10],
  bias: [0, 200],
}

/** Pilihan skala (pengali nilai mentah → satuan kanonik) yang ditawarkan di UI. */
export const SCALE_OPTIONS = [1, 0.1, 0.01, 0.001, 0.0001] as const

// Nama terakhir parameter. Awalan vendor (X_ZTE-COM_, X_CT-COM_) boleh ada: dicocokkan pada batas "_" atau "-".
const ROLE_RE: Record<OpticalRole, RegExp> = {
  rx: /(?:^|[_-])(?:rx|receive[d]?)[a-z]*?(?:power|level)$|opticalsignallevel$/i,
  tx: /(?:^|[_-])(?:tx|transmit(?:ted)?)[a-z]*?(?:power|level)$|transmitopticallevel$/i,
  temperature: /(?:^|[_-])temperature$/i,
  voltage: /(?:^|[_-])(?:supply)?voltage$/i,
  bias: /(?:^|[_-])bias(?:current)?$/i,
}

// Cabang WiFi/LAN punya "TransmitPower" dsb. yang BUKAN optik.
const NOT_OPTICAL_PATH = /wlan|wifi|wi-fi|landevice|radio\./i
const OPTICAL_HINT_PATH = /pon|optic|fiber|fibre/i

export interface OpticalCandidate {
  path: string
  role: OpticalRole
  /** Nilai mentah apa adanya dari ONT ('' bila kosong). */
  raw: string
  /** Angka dari raw, null bila bukan angka. */
  value: number | null
  /** Skala yang disarankan; null bila tak ada skala yang membuat nilai masuk akal. */
  suggestedScale: number | null
  /** Peringatan untuk operator (mis. satuan bukan dBm). */
  warning: string | null
}

const lastSegment = (name: string) => name.split('.').filter(Boolean).pop() ?? name
const parentOf = (name: string) => name.slice(0, name.lastIndexOf('.') + 1)

/** Skala pertama (dari 1 turun) yang membuat nilai masuk rentang wajar satuan kanonik; null bila tak ada. */
export function suggestScale(role: OpticalRole, raw: number): number | null {
  const [lo, hi] = PLAUSIBLE[role]
  for (const s of SCALE_OPTIONS) {
    const v = raw * s
    if (v >= lo && v <= hi) return s
  }
  return null
}

/** Hasil konversi untuk pratinjau (2 desimal); null bila raw bukan angka. */
export function applyScale(raw: string, scale: number): number | null {
  if (raw.trim() === '') return null
  const n = Number(raw)
  return Number.isFinite(n) ? Math.round(n * scale * 1e4) / 1e4 : null
}

function warningFor(role: OpticalRole, value: number | null, scale: number | null): string | null {
  if (value === null) return 'Nilai belum terbaca atau bukan angka.'
  if ((role === 'rx' || role === 'tx') && value > 20) return 'Nilai positif besar: satuannya mungkin µW (linear), bukan dBm — konversi logaritmik belum didukung, jangan disimpan.'
  if (scale === null) return `Tidak ada skala yang membuat nilai masuk akal untuk ${OPTICAL_ROLE_LABEL[role]} (${OPTICAL_ROLE_UNIT[role]}).`
  return null
}

/**
 * Kandidat sumber data optik dari parameter yang sudah tersinkron. RX/TX dikenali dari nama; suhu/tegangan/bias hanya
 * diterima bila satu objek dengan kandidat RX/TX atau jalurnya jelas optik (pon/optic/fiber), agar suhu CPU dsb. tak ikut.
 */
export function findOpticalCandidates(params: DeviceParameter[] | undefined): OpticalCandidate[] {
  const list = (params ?? []).filter((p) => !NOT_OPTICAL_PATH.test(p.parameter_name))
  const matches = (role: OpticalRole) => list.filter((p) => ROLE_RE[role].test(lastSegment(p.parameter_name)))

  // "TransmitPower" polos (tanpa konteks optik) terlalu umum → hanya bila jalurnya jelas optik.
  const primary = (['rx', 'tx'] as const).flatMap((role) =>
    matches(role)
      .filter((p) => !/^transmitpower$/i.test(lastSegment(p.parameter_name)) || OPTICAL_HINT_PATH.test(p.parameter_name))
      .map((p) => ({ p, role })),
  )
  const opticalParents = new Set(primary.map(({ p }) => parentOf(p.parameter_name)))

  const secondary = (['temperature', 'voltage', 'bias'] as const).flatMap((role) =>
    matches(role)
      .filter((p) => opticalParents.has(parentOf(p.parameter_name)) || OPTICAL_HINT_PATH.test(p.parameter_name))
      .map((p) => ({ p, role })),
  )

  const seen = new Set<string>()
  const out: OpticalCandidate[] = []
  for (const { p, role } of [...primary, ...secondary]) {
    if (seen.has(p.parameter_name)) continue
    seen.add(p.parameter_name)
    const raw = p.parameter_value ?? ''
    const value = raw.trim() !== '' && Number.isFinite(Number(raw)) ? Number(raw) : null
    const suggested = value === null ? null : suggestScale(role, value)
    out.push({ path: p.parameter_name, role, raw, value, suggestedScale: suggested, warning: warningFor(role, value, suggested) })
  }
  const order: OpticalRole[] = ['rx', 'tx', 'temperature', 'voltage', 'bias']
  return out.sort((a, b) => order.indexOf(a.role) - order.indexOf(b.role) || a.path.localeCompare(b.path))
}

/** Cabang yang ditelusuri untuk mencari parameter optik (GetParameterNames bertingkat; kedalaman 3 mencapai parameter daun). */
export function opticalDiscoveryPath(hasTR181Root: boolean): string {
  return hasTR181Root ? 'Device.Optical.' : 'InternetGatewayDevice.WANDevice.'
}
