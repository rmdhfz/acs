import { api } from '../lib/api'
import { EXPORT_MAX_ROWS } from '../lib/exporter'
import { RowError, type ImportAdapter, type ImportMode, type PlanContext, type PlanItem } from '../lib/importer'
import { runPool, withRetry } from '../lib/pool'
import type { DeviceModel, Vendor } from '../lib/types'

// Pembantu bersama modul operasional (tag, preset, provisioning, task, firmware,
// file). Dipisah dari kerangka impor/ekspor supaya kerangka itu tidak berubah.

export const norm = (s: string) => s.trim().toLowerCase()

// ---------------------------------------------------------------- paginasi

interface AnyPage<T> {
  data?: T[] | null
  total?: number
  meta?: { total?: number }
}

/**
 * listAll mengambil SEMUA halaman sebuah endpoint list. Menangani tiga bentuk
 * respons yang dipakai backend: array polos (tanpa paginasi, mis. zero-touch
 * rules), {data,total}, dan {data,meta:{total}} (tag/preset/file). Baris diberi
 * de-duplikasi per id supaya data baru yang masuk di tengah pengambilan tidak
 * menggandakan baris (urutan list menurun menurut id/waktu).
 */
export async function listAll<T extends { id: number }>(
  path: string,
  params: Record<string, string | number | undefined> = {},
  opts: { signal?: AbortSignal; onProgress?: (done: number, total: number) => void; maxRows?: number } = {},
): Promise<{ rows: T[]; total: number; truncated: boolean }> {
  const pageSize = 200
  const max = opts.maxRows ?? EXPORT_MAX_ROWS
  const byId = new Map<number, T>()
  let total: number | undefined
  for (let page = 1; ; page++) {
    if (opts.signal?.aborted) throw new DOMException('dibatalkan', 'AbortError')
    const usp = new URLSearchParams()
    for (const [k, v] of Object.entries({ ...params, page, page_size: pageSize })) if (v !== undefined && (v as unknown) !== '') usp.set(k, String(v))
    const resp = await withRetry(() => api.get<AnyPage<T> | T[]>(`${path}?${usp.toString()}`), opts.signal)
    if (Array.isArray(resp)) {
      for (const r of resp) byId.set(r.id, r)
      opts.onProgress?.(byId.size, byId.size)
      return { rows: [...byId.values()], total: byId.size, truncated: false }
    }
    const data = resp.data ?? []
    total = resp.total ?? resp.meta?.total ?? total
    for (const r of data) byId.set(r.id, r)
    opts.onProgress?.(byId.size, Math.max(total ?? 0, byId.size))
    if (data.length === 0 || data.length < pageSize || (total !== undefined && byId.size >= total)) break
    if (byId.size >= max) return { rows: [...byId.values()].slice(0, max), total: total ?? byId.size, truncated: true }
  }
  return { rows: [...byId.values()], total: total ?? byId.size, truncated: false }
}

/** Melempar galat bila hasil listAll terpotong batas ekspor (data tidak boleh diekspor setengah-setengah). */
export function assertComplete(truncated: boolean) {
  if (truncated) throw new Error('Data melebihi 50.000 baris — persempit dengan filter lalu ekspor lagi.')
}

// ---------------------------------------------------------------- hook plan

/**
 * withHooks membungkus adapter: `before` dijalankan sebelum plan (mis. memuat
 * katalog yang dibutuhkan parse), `after` memproses hasil plan (aturan yang
 * butuh konteks peran/pencocokan yang tak bisa diungkap oleh clientAdapter).
 */
export function withHooks(
  base: ImportAdapter,
  hooks: {
    before?: (mode: ImportMode, ctx: PlanContext) => Promise<void>
    after?: (items: PlanItem[], mode: ImportMode, ctx: PlanContext) => PlanItem[] | Promise<PlanItem[]>
  },
): ImportAdapter {
  return {
    ...base,
    async plan(records, mode, ctx) {
      await hooks.before?.(mode, ctx)
      const items = await base.plan(records, mode, ctx)
      return hooks.after ? hooks.after(items, mode, ctx) : items
    },
  }
}

export function toError(item: PlanItem, error: string): PlanItem {
  return { ...item, action: 'error', changes: undefined, error }
}

/** Data eksisting dengan kunci yang sama lebih dari satu: tidak aman diperbarui otomatis. */
export interface WithDups {
  dups: number
}

export function markAmbiguous(items: PlanItem[], noun: string): PlanItem[] {
  return items.map((it) => {
    const ex = (it.payload as { ex?: WithDups } | undefined)?.ex
    if ((it.action === 'update' || it.action === 'unchanged') && ex && ex.dups > 1) {
      return toError(it, `ambigu: ada ${ex.dups} ${noun} eksisting dengan kunci yang sama — rapikan duplikatnya dulu`)
    }
    return it
  })
}

// ---------------------------------------------------------------- cakupan

export type Scope = 'global' | 'tenant'

/** Cakupan baris: kosong = tenant (ADMIN) / global (SUPERADMIN). */
export function parseScope(raw: string | undefined, isSuper: boolean, hasTenantValue = false): Scope {
  const s = norm(raw ?? '')
  if (s === '') return isSuper && !hasTenantValue ? 'global' : 'tenant'
  if (['global', 'umum', 'semua'].includes(s)) return 'global'
  if (['tenant', 'tenant sendiri'].includes(s)) return 'tenant'
  throw new RowError(`scope "${raw}" harus global atau tenant`)
}

/**
 * guardScope menolak baris yang tidak boleh ditulis peran ini. Data global
 * (tenant_id NULL) hanya boleh dimutasi SUPERADMIN; baris global yang tidak
 * berubah / dilewati tetap lolos (supaya file hasil ekspor ADMIN, yang memuat
 * data global, bisa diimpor ulang tanpa error semu). Bila `superNeedsTenant`
 * (tag/preset: pembuatnya mewarisi tenant pelaku) SUPERADMIN tak bisa membuat
 * data ber-scope tenant.
 */
export function guardScope(items: PlanItem[], isSuper: boolean, noun: string, superNeedsTenant: boolean): PlanItem[] {
  return items.map((it) => {
    if (it.action !== 'create' && it.action !== 'update') return it
    const scope = (it.payload as { row?: { scope?: Scope } } | undefined)?.row?.scope
    if (!isSuper && scope === 'global') {
      return toError(it, `${noun} global hanya dapat ${it.action === 'create' ? 'dibuat' : 'diubah'} oleh SUPERADMIN`)
    }
    if (isSuper && superNeedsTenant && scope === 'tenant' && it.action === 'create') {
      return toError(it, `SUPERADMIN tidak terikat tenant — ${noun} ber-scope tenant dibuat oleh ADMIN tenant yang bersangkutan (gunakan scope global)`)
    }
    return it
  })
}

// ---------------------------------------------------------------- JSON

/** Stringify stabil (kunci objek diurutkan) untuk perbandingan struktur. */
export function canon(v: unknown): string {
  if (Array.isArray(v)) return `[${v.map(canon).join(',')}]`
  if (v && typeof v === 'object') {
    const o = v as Record<string, unknown>
    return `{${Object.keys(o)
      .sort()
      .map((k) => `${JSON.stringify(k)}:${canon(o[k])}`)
      .join(',')}}`
  }
  return JSON.stringify(v) ?? 'null'
}

/** Uraikan sel berisi teks JSON; kosong → undefined; rusak → RowError. */
export function parseJSONCell(label: string, raw: string | undefined): unknown {
  if (raw === undefined || raw === '') return undefined
  try {
    return JSON.parse(raw)
  } catch (e) {
    throw new RowError(`${label} bukan JSON valid (${e instanceof Error ? e.message : 'gagal diurai'})`)
  }
}

/** Nama parameter/kunci yang nilainya dianggap rahasia (tidak pernah diekspor). */
const SENSITIVE_RE = /pass(word|phrase|wd)|psk|preshared|secret|wepkey|token|key$/i
export function isSensitiveName(name: string): boolean {
  return SENSITIVE_RE.test(name)
}

// ---------------------------------------------------------------- katalog

export async function loadModels(vendors: Vendor[], signal?: AbortSignal): Promise<DeviceModel[]> {
  const lists = await runPool(
    vendors,
    // API membalas null (bukan []) untuk vendor tanpa model.
    async (v) => (await withRetry(() => api.get<DeviceModel[] | null>(`/device-models?vendor_id=${v.id}`), signal)) ?? [],
    { concurrency: 4, signal },
  )
  return lists.flatMap((l) => l ?? [])
}

export function findVendor(vendors: Vendor[], raw: string): Vendor | undefined {
  const s = norm(raw)
  return vendors.find((x) => norm(x.code) === s || norm(x.name) === s || String(x.id) === raw.trim())
}

/** Model berdasar nama/product class; vendor opsional (tanpa vendor: harus unik lintas vendor). */
export function findModel(models: DeviceModel[], vendorId: number | undefined, raw: string): DeviceModel {
  const s = norm(raw)
  const hits = models.filter(
    (m) => (vendorId === undefined || m.vendor_id === vendorId) && (norm(m.model_name) === s || (m.product_class != null && norm(m.product_class) === s)),
  )
  if (hits.length === 0) throw new RowError(`model "${raw}" tidak ditemukan${vendorId !== undefined ? ' pada vendor tersebut' : ''}`)
  if (hits.length > 1) throw new RowError(`model "${raw}" ambigu (ada di beberapa vendor) — isi vendor-nya`)
  return hits[0]
}

export const orUndef = (s: string | undefined) => (s === undefined || s === '' ? undefined : s)
