import type { OLTONUState, PONPortSummary } from './types'

// Fungsi murni untuk tampilan ONU terdaftar/port PON (tanpa React) supaya aturannya bisa diuji sendiri.

export const ONU_STATE_LABEL: Record<OLTONUState, string> = {
  ONLINE: 'Online',
  OFFLINE: 'Offline',
  LOS: 'LOS (sinyal hilang)',
  DYING_GASP: 'Dying gasp (daya hilang)',
  UNKNOWN: 'Tidak diketahui',
}

// Kode keadaan ONU -> kode gaya StatusBadge yang sudah ada (hijau/abu/merah/oranye/abu-tua). Kode asing diperlakukan UNKNOWN.
const BADGE: Record<OLTONUState, string> = { ONLINE: 'ONLINE', OFFLINE: 'OFFLINE', LOS: 'FAULTY', DYING_GASP: 'TIMEOUT', UNKNOWN: 'DECOMMISSIONED' }

export function onuStateLabel(state: string): string {
  return ONU_STATE_LABEL[state as OLTONUState] ?? ONU_STATE_LABEL.UNKNOWN
}

export function onuStateBadge(state: string): string {
  return BADGE[state as OLTONUState] ?? BADGE.UNKNOWN
}

// Sama dengan backend (usecase/troubleshoot oltStaleAfter): data OLT lebih tua dari ini diberi catatan "basi".
export const OLT_DATA_STALE_MINUTES = 30

// isOltDataStale: true bila waktu polling kosong/tak valid (belum pernah dipoll) atau lebih tua dari batas.
export function isOltDataStale(iso: string | null | undefined, now: number = Date.now()): boolean {
  if (!iso) return true
  const t = new Date(iso).getTime()
  if (Number.isNaN(t)) return true
  return now - t > OLT_DATA_STALE_MINUTES * 60_000
}

export type PortTone = 'bad' | 'warn' | 'ok' | 'idle'

// portTone: merah bila ada ONU LOS/dying gasp (masalah fisik yang dilaporkan OLT), kuning bila ada offline/tak diketahui, hijau bila semua online.
export function portTone(p: Pick<PONPortSummary, 'total' | 'los' | 'dying_gasp' | 'offline' | 'unknown'>): PortTone {
  if (p.total === 0) return 'idle'
  if (p.los > 0 || p.dying_gasp > 0) return 'bad'
  if (p.offline > 0 || p.unknown > 0) return 'warn'
  return 'ok'
}

export function formatRx(v: number | null | undefined): string {
  return v == null || !Number.isFinite(v) ? '—' : `${v.toFixed(1)} dBm`
}

// Total di seluruh port, untuk judul ringkasan.
export function sumPorts(ports: PONPortSummary[]): { total: number; online: number; problem: number } {
  let total = 0
  let online = 0
  let problem = 0
  for (const p of ports) {
    total += p.total
    online += p.online
    problem += p.los + p.dying_gasp
  }
  return { total, online, problem }
}
