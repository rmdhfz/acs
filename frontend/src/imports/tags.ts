import { api } from '../lib/api'
import { clientAdapter } from '../lib/clientAdapter'
import type { ExportColumn } from '../lib/exporter'
import { RowError, type ImportAdapter, type ImportColumn, type RawRecord } from '../lib/importer'
import type { Tag } from '../lib/types'
import { assertComplete, guardScope, listAll, markAmbiguous, norm, parseScope, withHooks, type Scope } from './opsShared'

// Impor/ekspor TAG. Kunci pencocokan: cakupan + nama (tak peka huruf besar).
// Tag global (tenant_id NULL) hanya dapat dibuat/diubah SUPERADMIN; ADMIN
// mengelola tag tenant sendiri (backend memberi tenant pelaku otomatis).
// Yang bisa diperbarui hanya warna (nama adalah kuncinya).

export interface TagImportEnv {
  isSuper: boolean
}

export const tagImportColumns: ImportColumn[] = [
  { key: 'name', label: 'Nama', required: true, example: 'pelanggan-vip', hint: 'Kunci pencocokan (bersama scope), tak peka huruf besar.' },
  { key: 'color', label: 'Warna', example: '#ef4444', hint: 'Heksadesimal #RGB atau #RRGGBB. Kosong = tidak diubah.' },
  { key: 'scope', label: 'Scope', example: 'tenant', hint: 'global (hanya SUPERADMIN) atau tenant. Kosong: tenant untuk ADMIN, global untuk SUPERADMIN.', aliases: ['cakupan'] },
  { key: 'created_at', label: 'Dibuat', readonly: true },
]

interface TagRow {
  name: string
  color?: string
  scope: Scope
}
type TagEx = Tag & { dups: number }

const COLOR_RE = /^#([0-9a-f]{3}|[0-9a-f]{6})$/i
const scopeOf = (t: Tag): Scope => (t.tenant_id == null ? 'global' : 'tenant')

function parseRow(rec: RawRecord, env: TagImportEnv): TagRow {
  if (rec.name === '') throw new RowError('nama wajib diisi')
  if (rec.name.length > 255) throw new RowError('nama maks. 255 karakter')
  const color = rec.color ?? ''
  if (color !== '' && !COLOR_RE.test(color)) throw new RowError(`warna "${color}" harus heksadesimal (#RGB atau #RRGGBB)`)
  return { name: rec.name, color: color === '' ? undefined : color.toLowerCase(), scope: parseScope(rec.scope, env.isSuper) }
}

export function tagAdapter(env: TagImportEnv): ImportAdapter {
  const base = clientAdapter<TagRow, TagEx>({
    entity: 'tag',
    columns: tagImportColumns,
    template: [{ name: 'pelanggan-vip', color: '#ef4444', scope: env.isSuper ? 'global' : 'tenant' }],
    parse: (rec) => parseRow(rec, env),
    key: (r) => `${r.scope}|${norm(r.name)}`,
    label: (r) => r.name,

    async loadExisting(_rows, signal) {
      const { rows } = await listAll<Tag>('/tags', {}, { signal })
      const m = new Map<string, TagEx>()
      for (const t of rows) {
        const k = `${scopeOf(t)}|${norm(t.name)}`
        const cur = m.get(k)
        m.set(k, cur ? { ...cur, dups: cur.dups + 1 } : { ...t, dups: 1 })
      }
      return m
    },

    diff: (row, ex) => (row.color !== undefined && row.color !== (ex.color ?? '').toLowerCase() ? ['color'] : []),

    async create(row) {
      await api.post<Tag>('/tags', { name: row.name, ...(row.color ? { color: row.color } : {}) })
    },
    async update(row, ex) {
      await api.patch<Tag>(`/tags/${ex.id}`, { color: row.color })
    },
  })

  return withHooks(base, {
    after: (items) => markAmbiguous(guardScope(items, env.isSuper, 'tag', true), 'tag'),
  })
}

export function tagExportColumns(): ExportColumn<Tag>[] {
  return [
    { key: 'name', value: (t) => t.name },
    { key: 'color', value: (t) => t.color },
    { key: 'scope', value: (t) => scopeOf(t) },
    { key: 'created_at', value: (t) => t.created_at },
  ]
}

export async function loadTagExportRows(ctx: { signal: AbortSignal; onProgress: (done: number, total: number) => void }): Promise<Tag[]> {
  const { rows, truncated } = await listAll<Tag>('/tags', {}, { signal: ctx.signal, onProgress: ctx.onProgress })
  assertComplete(truncated)
  return rows
}
