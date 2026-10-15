import { fetchAllPages, type ExportColumn } from '../lib/exporter'
import type { ActivityLog } from '../lib/types'

// Ekspor AUDIT TRAIL (GET /activity). Hanya ekspor — log audit tidak boleh
// diimpor. Mengikuti filter yang sedang aktif di halaman (action, entity_type)
// dan mengambil SEMUA halaman, bukan hanya baris yang tampil.

export function activityExportColumns(): ExportColumn<ActivityLog>[] {
  return [
    { key: 'id', value: (r) => r.id },
    { key: 'created_at', value: (r) => r.created_at },
    { key: 'username', value: (r) => r.username },
    { key: 'action', value: (r) => r.action },
    { key: 'entity_type', value: (r) => r.entity_type },
    { key: 'entity_id', value: (r) => r.entity_id },
    { key: 'description', value: (r) => r.description },
    { key: 'ip_address', value: (r) => r.ip_address },
    { key: 'tenant_id', value: (r) => r.tenant_id },
    { key: 'user_id', value: (r) => r.user_id },
  ]
}

export async function loadActivityExportRows(
  filters: { action?: string; entity_type?: string },
  ctx: { signal: AbortSignal; onProgress: (done: number, total: number) => void },
): Promise<ActivityLog[]> {
  const { rows, truncated } = await fetchAllPages<ActivityLog>('/activity', filters, { signal: ctx.signal, onProgress: ctx.onProgress })
  if (truncated) throw new Error('Data melebihi 50.000 baris — persempit dengan filter aksi/entitas lalu ekspor lagi.')
  return rows
}
