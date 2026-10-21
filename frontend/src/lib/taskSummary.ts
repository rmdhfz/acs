import { isSensitiveName } from '../imports/opsShared'

// Ringkasan bahasa manusia untuk sebuah task ("Ubah SSID WiFi #2 → Rumah Budi") dari kode jenis + parameters-nya,
// supaya operator tahu apa yang dikerjakan tanpa membaca JSON/path TR-069. Hanya tampilan: tidak mengubah data.
// Nilai rahasia (password/passphrase/PSK, atau yang sudah '[redacted]' dari backend) selalu ditutup.

export interface TaskSummary {
  title: string
  /** Satu baris per perubahan/parameter, mis. "SSID WiFi #2 → Rumah Budi". */
  details: string[]
}

const MASK = '••••••'

const WIFI_IDX = /(?:WLANConfiguration|WiFi\.SSID|WiFi\.AccessPoint)\.(\d+)\./

/** Label ramah untuk path TR-069 yang umum; selain itu dua segmen terakhir path. */
export function friendlyParamName(path: string): string {
  const idx = path.match(WIFI_IDX)?.[1]
  if (idx) {
    if (/KeyPassphrase$|PreSharedKey(\.\d+)?\.PreSharedKey$/.test(path)) return `Password WiFi #${idx}`
    if (/\.SSID$/.test(path)) return `SSID WiFi #${idx}`
    if (/\.Enable$/.test(path)) return `Status aktif WiFi #${idx}`
    if (/\.Channel$/.test(path)) return `Channel WiFi #${idx}`
    if (/SSIDAdvertisementEnabled$|SSIDHide$|HideSSID$/i.test(path)) return `Siaran SSID WiFi #${idx}`
    if (/(?:MaxUserNum|MaxAssociatedDevices|MaxStations|MaxClients)$/.test(path)) return `Maks. klien WiFi #${idx}`
    if (/Isolat\w*$/i.test(path)) return `Isolasi SSID WiFi #${idx}`
    if (/(?:^|[._])Priority$/.test(path)) return `Prioritas WiFi #${idx}`
  }
  if (/ManagementServer\.PeriodicInformInterval$/.test(path)) return 'Interval Periodic Inform'
  if (/ManagementServer\.PeriodicInformEnable$/.test(path)) return 'Periodic Inform aktif'
  if (/ManagementServer\.URL$/.test(path)) return 'ACS URL'
  if (/IPPingDiagnostics\.Host$/.test(path)) return 'Host ping'
  const parts = path.split('.').filter(Boolean)
  return parts.slice(-2).join('.') || path
}

function showValue(name: string, value: unknown): string {
  const v = value == null ? '' : String(value)
  if (v === '[redacted]' || isSensitiveName(name)) return MASK
  // Nilai boolean parameter WiFi dibaca manusia: terlihat/tersembunyi, aktif/nonaktif.
  const on = v === '1' || v.toLowerCase() === 'true'
  const off = v === '0' || v.toLowerCase() === 'false'
  if ((on || off) && WIFI_IDX.test(name)) {
    if (/SSIDAdvertisementEnabled$/.test(name)) return on ? 'terlihat' : 'tersembunyi'
    if (/SSIDHide$|HideSSID$/i.test(name)) return on ? 'tersembunyi' : 'terlihat'
    if (/\.Enable$|Isolat\w*$/i.test(name)) return on ? 'aktif' : 'nonaktif'
  }
  return v === '' ? '(kosong)' : v.length > 60 ? v.slice(0, 57) + '…' : v
}

function asObject(p: unknown): Record<string, unknown> {
  if (typeof p === 'string') {
    try {
      const j = JSON.parse(p)
      return j && typeof j === 'object' ? (j as Record<string, unknown>) : {}
    } catch {
      return {}
    }
  }
  return p && typeof p === 'object' ? (p as Record<string, unknown>) : {}
}

const lastSegment = (p: string) => p.replace(/\.$/, '').split('.').pop() ?? p

export function summarizeTask(typeCode: string | undefined, parameters: unknown): TaskSummary {
  const p = asObject(parameters)
  switch (typeCode) {
    case 'SET_PARAMETER_VALUES': {
      const values = asObject(p.values)
      const entries = Object.entries(values)
      if (entries.length === 0) return { title: 'Ubah parameter', details: [] }
      const details = entries.map(([name, v]) => `${friendlyParamName(name)} → ${showValue(name, v)}`)
      const host = entries.find(([n]) => /IPPingDiagnostics\.Host$/.test(n))
      if (host) return { title: `Jalankan Ping ke ${host[1]}`, details }
      const wifi = new Set(entries.map(([n]) => n.match(WIFI_IDX)?.[1]).filter(Boolean))
      if (wifi.size > 0 && entries.every(([n]) => WIFI_IDX.test(n))) {
        return { title: wifi.size === 1 ? `Ubah WiFi #${[...wifi][0]}` : `Ubah WiFi (${wifi.size} SSID)`, details }
      }
      return { title: `Ubah ${entries.length} parameter`, details }
    }
    case 'GET_PARAMETER_VALUES': {
      const names = Array.isArray(p.names) ? (p.names as unknown[]).map(String) : []
      if (names.length === 0) return { title: 'Baca parameter', details: [] }
      if (names.length === 1 && /WLANConfiguration\.$|WiFi\.SSID\.$/.test(names[0])) return { title: 'Baca semua SSID WiFi', details: [] }
      if (names.length === 1 && names[0].endsWith('.')) return { title: `Baca cabang ${lastSegment(names[0])}`, details: [] }
      return { title: `Baca ${names.length} parameter`, details: names.slice(0, 4).map(friendlyParamName) }
    }
    case 'GET_PARAMETER_NAMES':
      return { title: `Telusuri struktur ${p.path ? lastSegment(String(p.path)) : 'parameter'}`, details: [] }
    case 'REBOOT':
      return { title: 'Restart perangkat', details: [] }
    case 'FACTORY_RESET':
      return { title: 'Kembalikan ke setelan pabrik', details: [] }
    case 'DOWNLOAD':
      return { title: `Unduh ${p.file_type ? String(p.file_type) : 'file'} ke perangkat`, details: p.target_file_name ? [String(p.target_file_name)] : [] }
    case 'ADD_OBJECT':
      return { title: `Tambah objek ${p.object_name ? lastSegment(String(p.object_name)) : ''}`.trim(), details: [] }
    case 'DELETE_OBJECT':
      return { title: `Hapus objek ${p.object_name ? lastSegment(String(p.object_name)) : ''}`.trim(), details: [] }
    case 'SCHEDULE_INFORM':
      return { title: 'Jadwalkan Inform', details: [] }
    case 'SET_PARAMETER_ATTRIBUTES':
      return { title: 'Atur notifikasi perubahan nilai', details: [] }
    case 'GET_PARAMETER_ATTRIBUTES':
      return { title: 'Baca pengaturan notifikasi', details: [] }
    default:
      return { title: typeCode ?? 'Task', details: [] }
  }
}

/** Jenis task yang boleh dicoba-ulang NOC (cermin backend task.nocMayRetry); selain itu hanya ADMIN. */
export const NOC_MAY_RETRY = new Set(['GET_PARAMETER_VALUES', 'GET_PARAMETER_NAMES', 'GET_PARAMETER_ATTRIBUTES', 'REBOOT'])

export type RetryKind = 'retry' | 'send-now' | null

/** Tombol apa yang relevan: "Coba lagi" (gagal/batal/timeout) atau "Kirim sekarang" (menunggu jadwal retry). */
export function retryKind(statusCode: string | undefined, retryCount: number, scheduledAt: string | null): RetryKind {
  if (statusCode === 'FAILED' || statusCode === 'CANCELLED' || statusCode === 'TIMEOUT') return 'retry'
  if (statusCode === 'PENDING' && (retryCount > 0 || (scheduledAt != null && new Date(scheduledAt).getTime() > Date.now()))) return 'send-now'
  return null
}
