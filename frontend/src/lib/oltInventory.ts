import type { OLTONUState, PONPortSummary, RegisteredONU } from './types'

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

// Total di seluruh port, untuk judul ringkasan. problem = LOS + dying gasp (masalah fisik yang dilaporkan OLT); offline dihitung terpisah.
export function sumPorts(ports: PONPortSummary[]): { total: number; online: number; problem: number; offline: number } {
  let total = 0
  let online = 0
  let problem = 0
  let offline = 0
  for (const p of ports) {
    total += p.total
    online += p.online
    problem += p.los + p.dying_gasp
    offline += p.offline
  }
  return { total, online, problem, offline }
}

// stateSince: "Keadaan sejak" hanya bermakna bila ACS PERNAH melihat keadaan itu berubah. Baris yang baru dilihat memakai waktu penemuan sebagai
// last_state_change_at (sama dengan first_seen_at), jadi tidak boleh ditampilkan sebagai "sejak" — ONU yang sudah LOS berjam-jam sebelum
// ACS menemukannya akan tampak baru LOS beberapa detik lalu.
export function stateSince(r: Pick<RegisteredONU, 'first_seen_at' | 'last_state_change_at'>): { observed: boolean; at: string } {
  const first = new Date(r.first_seen_at).getTime()
  const change = new Date(r.last_state_change_at).getTime()
  const observed = Number.isFinite(first) && Number.isFinite(change) && change - first > 1000
  return { observed, at: observed ? r.last_state_change_at : r.first_seen_at }
}

// Korelasi per port PON: banyak ONU di SATU port yang kehilangan sinyal/daya bersamaan jarang kebetulan. Ini DUGAAN berbasis ambang, bukan
// kepastian (dua ONU di port kecil bisa kebetulan sama-sama bermasalah), jadi ambangnya konservatif dan teksnya "kemungkinan".
export const PORT_HINT_MIN_ONUS = 3
export const PORT_HINT_MIN_RATIO = 0.5

export interface PortHint {
  kind: 'FEEDER' | 'POWER'
  text: string
}

export function portHint(p: Pick<PONPortSummary, 'total' | 'los' | 'dying_gasp'>): PortHint | null {
  if (p.total <= 0) return null
  const strong = (n: number) => n >= PORT_HINT_MIN_ONUS && n / p.total >= PORT_HINT_MIN_RATIO
  if (strong(p.los) && p.los >= p.dying_gasp) {
    return { kind: 'FEEDER', text: `${p.los} dari ${p.total} ONU LOS: kemungkinan kabel feeder/distribusi atau splitter bermasalah, bukan gangguan di rumah pelanggan.` }
  }
  if (strong(p.dying_gasp)) {
    return { kind: 'POWER', text: `${p.dying_gasp} dari ${p.total} ONU dying gasp: kemungkinan listrik padam di area port ini.` }
  }
  return null
}

// flaggedPorts: port yang punya dugaan di atas, untuk banner ringkasan (terparah lebih dulu).
export function flaggedPorts(ports: PONPortSummary[]): { port: string; hint: PortHint }[] {
  const out: { port: string; hint: PortHint; weight: number }[] = []
  for (const p of ports) {
    const hint = portHint(p)
    if (hint) out.push({ port: p.pon_port, hint, weight: Math.max(p.los, p.dying_gasp) })
  }
  return out.sort((a, b) => b.weight - a.weight || a.port.localeCompare(b.port)).map(({ port, hint }) => ({ port, hint }))
}
