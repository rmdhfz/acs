import { api } from '../lib/api'
import { clientAdapter } from '../lib/clientAdapter'
import { fetchAllPages, type ExportColumn } from '../lib/exporter'
import { RowError, parseBool, parseNumber, type ImportAdapter, type ImportColumn, type ImportMode, type RawRecord } from '../lib/importer'
import { LIMITS } from '../lib/limits'
import type { Tenant } from '../lib/types'

// Impor/ekspor TENANT (khusus SUPERADMIN). Memakai API yang sudah ada per baris:
// POST /tenants (hanya code+name), PATCH /tenants/:id, PATCH /tenants/:id/branding
// dan PATCH /tenants/:id/task-quota.
//
// Kunci pencocokan: code (tak peka huruf besar). Pada tenant yang sudah ada hanya
// name, is_active, brand_name dan max_pending_tasks yang diperbarui (code tidak
// berubah karena itu kuncinya). Sel kosong pada tenant yang sudah ada = tidak
// diubah. Kredensial CWMP TIDAK ikut diimpor/diekspor — atur lewat tombol
// "Kredensial CWMP" pada tiap baris tenant setelah impor.

export interface TenantImportEnv {
  /** tenant_id akun yang sedang login: tenant ini tidak boleh dinonaktifkan lewat impor. */
  actorTenantId: number | null
}

export const tenantImportColumns: ImportColumn[] = [
  {
    key: 'code',
    label: 'Code',
    required: true,
    example: 'ISP-JKT',
    hint: `Kunci pencocokan (tak peka huruf besar), maks. ${LIMITS.tenant.code}. Kredensial CWMP TIDAK diimpor — atur lewat "Kredensial CWMP" di tiap tenant setelah impor.`,
  },
  { key: 'name', label: 'Nama', required: true, example: 'ISP Jakarta Sejahtera', hint: `Maks. ${LIMITS.tenant.name}.` },
  { key: 'is_active', label: 'Aktif', example: 'true', aliases: ['aktif', 'active'], hint: 'true/false. Baru: default true. false = semua user tenant tidak bisa login. Kosong pada tenant yang sudah ada = tidak diubah.' },
  { key: 'brand_name', label: 'Nama Brand', example: 'ISP Jakarta', hint: `Opsional, maks. ${LIMITS.tenant.brandName}. Kosong = tidak diubah.` },
  { key: 'max_pending_tasks', label: 'Kuota Task Pending', example: '500', aliases: ['task_quota'], hint: 'Opsional, bilangan bulat ≥ 0. Kosong = tidak diubah (tidak bisa mengosongkan kuota lewat impor).' },
  // informasi saja (ikut diekspor, diabaikan saat impor)
  { key: 'tenant_uuid', label: 'UUID', readonly: true },
  { key: 'created_at', label: 'Dibuat', readonly: true },
]

interface TenantRow {
  code: string
  name: string
  isActive?: boolean
  brandName?: string
  maxPending?: number
}

const norm = (s: string) => s.trim().toLowerCase()

function parseRow(rec: RawRecord): TenantRow {
  const code = (rec.code ?? '').trim()
  if (code === '') throw new RowError('code wajib diisi')
  if (code.length > LIMITS.tenant.code) throw new RowError(`code maks. ${LIMITS.tenant.code} karakter`)
  const name = (rec.name ?? '').trim()
  if (name === '') throw new RowError('name wajib diisi')
  if (name.length > LIMITS.tenant.name) throw new RowError(`name maks. ${LIMITS.tenant.name} karakter`)
  const brandName = rec.brand_name === undefined || rec.brand_name === '' ? undefined : rec.brand_name
  if (brandName !== undefined && brandName.length > LIMITS.tenant.brandName) throw new RowError(`brand_name maks. ${LIMITS.tenant.brandName} karakter`)
  const maxPending = parseNumber('max_pending_tasks', rec.max_pending_tasks ?? '')
  if (maxPending !== undefined && (!Number.isInteger(maxPending) || maxPending < 0 || maxPending > 4_294_967_295)) {
    throw new RowError('max_pending_tasks harus bilangan bulat ≥ 0')
  }
  return { code, name, isActive: parseBool('is_active', rec.is_active ?? ''), brandName, maxPending }
}

interface TenantPayload {
  row: TenantRow
  ex?: Tenant
  changes?: string[]
}

/** step menjalankan satu langkah lanjutan dan memberi konteks bila gagal. */
async function step(label: string, applied: string | false, fn: () => Promise<unknown>): Promise<void> {
  try {
    await fn()
  } catch (e) {
    const msg = e instanceof Error ? e.message : String(e)
    throw new Error(`${label} gagal${applied ? ` (${applied})` : ''}: ${msg}`)
  }
}

export function tenantAdapter(env: TenantImportEnv): ImportAdapter {
  const base = clientAdapter<TenantRow, Tenant>({
    entity: 'tenant',
    columns: tenantImportColumns,
    template: [{ code: 'ISP-JKT', name: 'ISP Jakarta Sejahtera', is_active: 'true', brand_name: '', max_pending_tasks: '' }],
    parse: parseRow,
    key: (r) => norm(r.code),
    label: (r) => r.code,

    async loadExisting(_rows, signal) {
      const { rows } = await fetchAllPages<Tenant>('/tenants', {}, { signal })
      const m = new Map<string, Tenant>()
      for (const t of rows) m.set(norm(t.code), t)
      return m
    },

    diff(row, ex) {
      const ch: string[] = []
      if (row.name !== ex.name) ch.push('name')
      if (row.isActive !== undefined && row.isActive !== ex.is_active) ch.push('is_active')
      if (row.brandName !== undefined && row.brandName !== (ex.brand_name ?? '')) ch.push('brand_name')
      if (row.maxPending !== undefined && row.maxPending !== ex.max_pending_tasks) ch.push('max_pending_tasks')
      return ch
    },

    async create(row) {
      const t = await api.post<Tenant>('/tenants', { code: row.code, name: row.name })
      const applied = 'tenant sudah dibuat'
      if (row.isActive === false) {
        await step('menonaktifkan tenant', applied, () => api.patch(`/tenants/${t.id}`, { is_active: false }))
      }
      if (row.brandName !== undefined) {
        await step('menyimpan brand_name', applied, () =>
          api.patch(`/tenants/${t.id}/branding`, { brand_name: row.brandName, logo_url: null, primary_color: null }),
        )
      }
      if (row.maxPending !== undefined) {
        await step('menyimpan kuota task', applied, () => api.patch(`/tenants/${t.id}/task-quota`, { max_pending_tasks: row.maxPending }))
      }
    },

    async update(row, ex, changes) {
      let applied: string | false = false
      const body: Record<string, unknown> = {}
      if (changes.includes('name')) body.name = row.name
      if (changes.includes('is_active')) body.is_active = row.isActive
      if (Object.keys(body).length > 0) {
        await step('memperbarui tenant', applied, () => api.patch(`/tenants/${ex.id}`, body))
        applied = 'langkah sebelumnya sudah diterapkan'
      }
      if (changes.includes('brand_name')) {
        // branding bersifat full-replace: logo & warna eksisting dipertahankan
        await step('menyimpan brand_name', applied, () =>
          api.patch(`/tenants/${ex.id}/branding`, { brand_name: row.brandName, logo_url: ex.logo_url, primary_color: ex.primary_color }),
        )
        applied = 'langkah sebelumnya sudah diterapkan'
      }
      if (changes.includes('max_pending_tasks')) {
        await step('menyimpan kuota task', applied, () => api.patch(`/tenants/${ex.id}/task-quota`, { max_pending_tasks: row.maxPending }))
      }
    },
  })

  return {
    ...base,
    async plan(records: RawRecord[], mode: ImportMode, ctx) {
      const items = await base.plan(records, mode, ctx)
      for (const it of items) {
        const p = it.payload as TenantPayload | undefined
        // Menonaktifkan tenant milik akun sendiri = self-lockout; tolak di pratinjau.
        if (it.action === 'update' && p?.ex && p.ex.id === env.actorTenantId && it.changes?.includes('is_active') && p.row.isActive === false) {
          it.action = 'error'
          it.error = 'tidak bisa menonaktifkan tenant milik akun sendiri'
          it.changes = undefined
        }
      }
      return items
    },
  }
}

// ---------------------------------------------------------------- ekspor

/** Kolom ekspor tenant. Kredensial CWMP (username/password) TIDAK diekspor. */
export function tenantExportColumns(): ExportColumn<Tenant>[] {
  return [
    { key: 'code', value: (t) => t.code },
    { key: 'name', value: (t) => t.name },
    { key: 'is_active', value: (t) => t.is_active },
    { key: 'brand_name', value: (t) => t.brand_name },
    { key: 'max_pending_tasks', value: (t) => t.max_pending_tasks },
    { key: 'tenant_uuid', value: (t) => t.tenant_uuid },
    { key: 'created_at', value: (t) => t.created_at },
  ]
}

export async function loadTenantExportRows(ctx: { signal: AbortSignal; onProgress: (done: number, total: number) => void }): Promise<Tenant[]> {
  const { rows, truncated } = await fetchAllPages<Tenant>('/tenants', {}, { signal: ctx.signal, onProgress: ctx.onProgress })
  if (truncated) throw new Error('Data melebihi 50.000 baris.')
  return rows
}
