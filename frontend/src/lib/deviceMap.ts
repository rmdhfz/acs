import type { DeviceMapPoint, DeviceMapResponse } from './types'

// Fungsi murni untuk peta perangkat (tanpa Leaflet) supaya aturan warna/ringkasan/bbox bisa diuji sendiri.

export interface StatusStyle {
  color: string
  label: string
}

const STATUS_STYLES: Record<string, StatusStyle> = {
  ONLINE: { color: '#10b981', label: 'Online' },
  OFFLINE: { color: '#ef4444', label: 'Offline' },
  UNREGISTERED: { color: '#f59e0b', label: 'Belum terdaftar' },
}

// Status tak dikenal tidak disamarkan sebagai online: abu-abu dengan kodenya apa adanya.
export function statusStyle(code: string): StatusStyle {
  return STATUS_STYLES[code] ?? { color: '#64748b', label: code || 'Tidak diketahui' }
}

export interface StatusCounts {
  online: number
  offline: number
  other: number
}

export function countStatuses(points: Pick<DeviceMapPoint, 'status'>[]): StatusCounts {
  const c: StatusCounts = { online: 0, offline: 0, other: 0 }
  for (const p of points) {
    if (p.status === 'ONLINE') c.online++
    else if (p.status === 'OFFLINE') c.offline++
    else c.other++
  }
  return c
}

// Warna klaster mengikuti yang TERPARAH: satu perangkat offline di dalamnya cukup untuk menandai klaster merah, supaya masalah tidak
// tenggelam saat peta di-zoom keluar.
export function clusterColor(c: StatusCounts): string {
  if (c.offline > 0) return STATUS_STYLES.OFFLINE.color
  if (c.other > 0) return '#f59e0b'
  return STATUS_STYLES.ONLINE.color
}

// Catatan di atas peta bila ada perangkat yang tak bisa digambar atau hasil dipotong; null bila tidak ada yang perlu dikatakan.
export function mapNotice(res: Pick<DeviceMapResponse, 'total' | 'located' | 'truncated'> | undefined): string | null {
  if (!res) return null
  const parts: string[] = []
  const without = res.total - res.located
  if (without > 0) {
    parts.push(`${without} dari ${res.total} perangkat belum punya koordinat dan tidak tampil di peta`)
  }
  if (res.truncated) {
    parts.push('hasil dipotong pada 5000 titik — perbesar peta atau persempit filter untuk melihat sisanya')
  }
  return parts.length ? parts.join('; ') + '.' : null
}

export interface ViewBounds {
  west: number
  south: number
  east: number
  north: number
}

function wrapLng(lng: number): number {
  return ((((lng + 180) % 360) + 360) % 360) - 180
}

// bboxParam mengubah batas pandangan Leaflet menjadi query bbox "barat,selatan,timur,utara" yang diterima backend. Leaflet boleh
// memberi bujur di luar -180..180 saat peta digeser melewati salinan dunia: dibungkus kembali (barat > timur = melintasi bujur 180,
// didukung backend). Pandangan selebar 360° atau lebih = seluruh dunia = tanpa bbox (undefined). Lintang dijepit -90..90.
export function bboxParam(b: ViewBounds): string | undefined {
  if (![b.west, b.south, b.east, b.north].every(Number.isFinite)) return undefined
  if (b.east - b.west >= 360) return undefined
  const south = Math.max(-90, Math.min(90, b.south))
  const north = Math.max(-90, Math.min(90, b.north))
  const r = (n: number) => Number(n.toFixed(5))
  return [r(wrapLng(b.west)), r(south), r(wrapLng(b.east)), r(north)].join(',')
}

// Tengah peta bawaan (Indonesia) bila belum ada titik; selain itu peta menyesuaikan diri ke titik yang ada (fitBounds).
export const DEFAULT_CENTER: [number, number] = [-2.5, 118]
export const DEFAULT_ZOOM = 5
