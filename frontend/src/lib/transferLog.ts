import { api } from './api'
import type { TransferKind, TransferStatus } from './hooks'

// Pencatatan riwayat impor/ekspor (POST /data-transfers). Dipanggil SETELAH proses selesai: kegagalan mencatat tidak
// pernah mengganggu pengguna (hanya riwayat yang hilang). Hanya hitungan dan nama berkas yang dikirim — isi baris TIDAK.

/** Mengubah nama entitas bebas ("Perangkat", "vendor-oui") menjadi kunci modul server (^[a-z][a-z0-9_]{1,47}$). */
export function moduleKey(name: string): string {
  const k = name
    .normalize('NFKD')
    .replace(/[̀-ͯ]/g, '')
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '_')
    .replace(/^_+|_+$/g, '')
    .slice(0, 47)
  if (!k) return 'data'
  return /^[a-z]/.test(k) ? (k.length < 2 ? `${k}_` : k) : `m_${k}`.slice(0, 48)
}

export interface TransferReport {
  kind: TransferKind
  module: string
  format?: 'csv' | 'json'
  fileName?: string
  total: number
  success: number
  failed: number
  detail?: string
  /** 'FAILED' = proses gagal total (mis. ekspor galat); selain itu status diturunkan dari hitungan. */
  outcome?: 'FAILED'
}

/** Menentukan status dari hitungan (konsisten dengan validasi server). */
export function transferStatus(success: number, failed: number): TransferStatus {
  if (failed === 0) return 'COMPLETED'
  return success === 0 ? 'FAILED' : 'PARTIAL'
}

// Karakter kontrol (kode < 32 atau 127) diganti spasi; server menolak teks ber-kontrol.
function oneLine(s: string): string {
  return Array.from(s, (c) => (c.charCodeAt(0) < 32 || c.charCodeAt(0) === 127 ? ' ' : c))
    .join('')
    .replace(/ {2,}/g, ' ')
    .trim()
}

export function recordTransfer(r: TransferReport): void {
  const total = Math.max(0, Math.floor(r.total))
  const success = Math.min(total, Math.max(0, Math.floor(r.success)))
  const failed = Math.min(total - success, Math.max(0, Math.floor(r.failed)))
  // pemanggilan satu baris (api.post(...)) agar terbaca skrip api-parity
  const body = {
    kind: r.kind,
    status: r.outcome === 'FAILED' ? 'FAILED' : transferStatus(success, failed),
    module: moduleKey(r.module),
    format: r.format ?? 'csv',
    file_name: r.fileName?.slice(0, 255),
    total_rows: total,
    success_rows: success,
    failed_rows: failed,
    detail: r.detail ? oneLine(r.detail).slice(0, 500) : undefined,
  }
  api.post('/data-transfers', body).catch(() => {
    // sengaja diam: pencatatan riwayat tidak boleh mengganggu alur impor/ekspor
  })
}
