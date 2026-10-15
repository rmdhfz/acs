import { api } from '../lib/api'
import { clientAdapter, type ClientAdapterSpec } from '../lib/clientAdapter'
import type { ExportColumn } from '../lib/exporter'
import {
  RowError,
  parseBool,
  type ImportAdapter,
  type ImportColumn,
  type ImportMode,
  type PlanContext,
  type PlanItem,
  type RawRecord,
} from '../lib/importer'
import { LIMITS } from '../lib/limits'
import { withRetry } from '../lib/pool'
import type { DeviceModel, ListResponse, Vendor, VendorOUI, VendorParameterMapping } from '../lib/types'

// Impor/ekspor KATALOG VENDOR (khusus SUPERADMIN): vendor, OUI, device model,
// parameter mapping. Menambah vendor/model baru adalah operasi data (CLAUDE.md),
// jadi impor massal ini adalah jalur onboarding vendor.
//
// Semua entitas memakai `clientAdapter` (memanggil API create/update yang sudah
// ada per baris; RBAC & validasi tetap di backend). Data referensi (vendor, ref_*,
// model) dimuat SEGAR di awal pratinjau (bukan dari cache halaman) lalu dipakai
// untuk memetakan KODE/nama di file ke id — id numerik mentah tidak pernah ada
// di file. File ekspor memakai kunci kolom yang sama dengan kolom impor, sehingga
// ekspor → impor ulang (mode Perbarui) menghasilkan "Tidak berubah" semua.

// ------------------------------------------------------------ util umum

const norm = (s: string) => s.trim().toLowerCase()
/** Sel kosong atau kolom tidak ada → ''; dipakai untuk membandingkan "kosong" vs NULL. */
const blank = (s: string | null | undefined) => (s ?? '').trim()

interface Ref {
  id: number
  code: string
  name: string
}

class RefIndex {
  private byKey = new Map<string, Ref>()
  private byId = new Map<number, Ref>()
  readonly codes: string
  /** false bila GET /refs/:table gagal (lihat catatan ref_parameter_types). */
  readonly ok: boolean
  constructor(list: Ref[], ok = true) {
    this.ok = ok
    this.codes = list.map((r) => r.code).join(', ')
    for (const r of list) {
      this.byId.set(r.id, r)
      this.byKey.set(norm(r.name), r)
      this.byKey.set(norm(r.code), r) // kode menang atas nama bila bentrok
    }
  }
  find(raw: string): Ref | undefined {
    return this.byKey.get(norm(raw))
  }
  /** Kode untuk ekspor; "#id" bila referensi tidak diketahui. */
  codeOf(id: number | null | undefined): string {
    if (id == null) return ''
    return this.byId.get(id)?.code ?? `#${id}`
  }
}

class VendorIndex {
  private byKey = new Map<string, Vendor>()
  private byId = new Map<number, Vendor>()
  readonly list: Vendor[]
  constructor(list: Vendor[]) {
    this.list = list
    for (const v of list) {
      this.byId.set(v.id, v)
      this.byKey.set(norm(v.name), v)
      this.byKey.set(norm(v.code), v)
    }
  }
  find(raw: string): Vendor | undefined {
    return this.byKey.get(norm(raw))
  }
  codeOf(id: number): string {
    return this.byId.get(id)?.code ?? `#${id}`
  }
}

interface CatalogEnv {
  vendors: VendorIndex
  deviceTypes: RefIndex
  dataModels: RefIndex
  paramTypes: RefIndex
  /** vendor_id → model (hanya dimuat bila adapter membutuhkannya). */
  models: Map<number, DeviceModel[]>
}

interface EnvNeeds {
  refs?: boolean
  paramTypes?: boolean
  models?: boolean
}

const EMPTY_ENV: CatalogEnv = {
  vendors: new VendorIndex([]),
  deviceTypes: new RefIndex([]),
  dataModels: new RefIndex([]),
  paramTypes: new RefIndex([]),
  models: new Map(),
}

/** API membalas null (bukan []) untuk koleksi kosong. */
async function listAll<T>(path: string, signal?: AbortSignal): Promise<T[]> {
  return (await withRetry(() => api.get<T[] | null>(path), signal)) ?? []
}

async function listVendors(signal?: AbortSignal): Promise<Vendor[]> {
  const out: Vendor[] = []
  for (let page = 1; ; page++) {
    const r = await withRetry(() => api.get<ListResponse<Vendor>>(`/vendors?page=${page}&page_size=200`), signal)
    const data = r.data ?? []
    out.push(...data)
    if (data.length < 200 || out.length >= (r.total ?? 0)) break
  }
  return out
}

async function loadModels(vendors: Vendor[], signal?: AbortSignal): Promise<Map<number, DeviceModel[]>> {
  const out = new Map<number, DeviceModel[]>()
  for (const v of vendors) out.set(v.id, await listAll<DeviceModel>(`/device-models?vendor_id=${v.id}`, signal))
  return out
}

async function loadEnv(needs: EnvNeeds, signal?: AbortSignal): Promise<CatalogEnv> {
  const vendorList = await listVendors(signal)
  const env: CatalogEnv = { ...EMPTY_ENV, vendors: new VendorIndex(vendorList), models: new Map() }
  if (needs.refs) {
    env.deviceTypes = new RefIndex(await listAll<Ref>('/refs/ref_device_types', signal))
    env.dataModels = new RefIndex(await listAll<Ref>('/refs/ref_data_model_versions', signal))
  }
  if (needs.paramTypes) {
    // GET /refs/ref_parameter_types saat ini membalas 500 di backend (kolom `code AS name`
    // dipakai dua kali di SQL ref_repository.go). Jangan gagalkan seluruh pratinjau:
    // turun ke mode "#id" (lihat resolveParamType / RefIndex.codeOf).
    try {
      env.paramTypes = new RefIndex(await listAll<Ref>('/refs/ref_parameter_types', signal))
    } catch {
      env.paramTypes = new RefIndex([], false)
    }
  }
  if (needs.models) env.models = await loadModels(vendorList, signal)
  return env
}

/**
 * catalogAdapter membungkus clientAdapter: data referensi dimuat segar di awal
 * `plan` (parse bersifat sinkron sehingga membutuhkan env siap pakai), lalu
 * `post` boleh menyesuaikan hasil klasifikasi (mis. konflik OUI lintas vendor).
 * `execute` memakai spec dengan env kosong — create/update hanya memakai data
 * baris yang sudah terurai (id sudah ter-resolve), tidak pernah env.
 */
function catalogAdapter<Row, Ex>(opts: {
  entity: string
  columns: ImportColumn[]
  template: RawRecord[]
  needs: EnvNeeds
  spec: (env: CatalogEnv) => Omit<ClientAdapterSpec<Row, Ex>, 'entity' | 'columns' | 'template'>
  post?: (items: PlanItem[], env: CatalogEnv, mode: ImportMode) => PlanItem[]
}): ImportAdapter {
  const build = (env: CatalogEnv) =>
    clientAdapter<Row, Ex>({ entity: opts.entity, columns: opts.columns, template: opts.template, ...opts.spec(env) })
  const base = build(EMPTY_ENV)
  return {
    entity: base.entity,
    columns: base.columns,
    template: base.template,
    supportsUpdate: base.supportsUpdate,
    async plan(records: RawRecord[], mode: ImportMode, ctx: PlanContext) {
      const env = await loadEnv(opts.needs, ctx.signal)
      const items = await build(env).plan(records, mode, ctx)
      return opts.post ? opts.post(items, env, mode) : items
    },
    execute: base.execute,
  }
}

function asError(item: PlanItem, error: string): PlanItem {
  return { ...item, action: 'error', changes: undefined, error }
}

/** Kegagalan LANJUTAN setelah penulisan utama berhasil: BUKAN ApiError, supaya tidak di-retry (mencegah baris ganda). */
class PartialWriteError extends Error {}

function lenCheck(label: string, v: string | undefined, max: number) {
  if (v !== undefined && v.length > max) throw new RowError(`${label} maks. ${max} karakter`)
}

// ================================================================ 1. VENDORS

export const vendorImportColumns: ImportColumn[] = [
  { key: 'code', label: 'Code', required: true, example: 'VSOL', aliases: ['kode', 'vendor_code'], hint: 'Kunci pencocokan (tak peka huruf besar), maks. 32 karakter. Tidak bisa diubah setelah dibuat.' },
  { key: 'name', label: 'Nama', required: true, example: 'V-SOL Technology', aliases: ['nama'], hint: 'Maks. 128 karakter.' },
  { key: 'description', label: 'Deskripsi', example: 'Produsen ONU/OLT white-label', aliases: ['deskripsi'], hint: 'Maks. 255 karakter. Sel kosong pada vendor yang sudah ada = kosongkan deskripsi (kolom tidak ada = tidak diubah).' },
  { key: 'is_active', label: 'Aktif', example: 'true', aliases: ['aktif', 'status'], hint: 'true/false (atau ya/tidak). Kosong = aktif untuk vendor baru, tidak diubah untuk yang sudah ada.' },
]

interface VendorRow {
  code: string
  name: string
  description?: string
  isActive?: boolean
}

export function vendorAdapter(): ImportAdapter {
  return catalogAdapter<VendorRow, Vendor>({
    entity: 'vendor',
    columns: vendorImportColumns,
    template: [{ code: 'VSOL', name: 'V-SOL Technology', description: 'Produsen ONU/OLT white-label', is_active: 'true' }],
    needs: {},
    spec: () => ({
      parse(rec) {
        const code = rec.code ?? ''
        const name = rec.name ?? ''
        if (code === '') throw new RowError('code wajib diisi')
        if (name === '') throw new RowError('name wajib diisi')
        lenCheck('code', code, LIMITS.vendor.code)
        lenCheck('name', name, LIMITS.vendor.name)
        lenCheck('description', rec.description, LIMITS.description)
        return { code, name, description: rec.description, isActive: parseBool('is_active', rec.is_active ?? '') }
      },
      key: (r) => norm(r.code),
      label: (r) => r.code,
      async loadExisting(_rows, signal) {
        const m = new Map<string, Vendor>()
        for (const v of await listVendors(signal)) m.set(norm(v.code), v)
        return m
      },
      diff(r, ex) {
        const ch: string[] = []
        if (r.name !== ex.name) ch.push('name')
        if (r.description !== undefined && blank(r.description) !== blank(ex.description)) ch.push('description')
        if (r.isActive !== undefined && r.isActive !== ex.is_active) ch.push('is_active')
        return ch
      },
      async create(r) {
        const v = await api.post<Vendor>('/vendors', { code: r.code, name: r.name, description: blank(r.description) || undefined })
        if (r.isActive === false) {
          try {
            await withRetry(() => api.patch(`/vendors/${v.id}`, { is_active: false }))
          } catch (e) {
            throw new PartialWriteError(`vendor dibuat tetapi gagal dinonaktifkan: ${e instanceof Error ? e.message : String(e)}`)
          }
        }
      },
      async update(r, ex, changes) {
        const body: Record<string, unknown> = {}
        if (changes.includes('name')) body.name = r.name
        if (changes.includes('description')) body.description = blank(r.description)
        if (changes.includes('is_active')) body.is_active = r.isActive
        await api.patch(`/vendors/${ex.id}`, body)
      },
    }),
  })
}

export const vendorExportColumns: ExportColumn<Vendor>[] = [
  { key: 'code', value: (v) => v.code },
  { key: 'name', value: (v) => v.name },
  { key: 'description', value: (v) => v.description ?? '' },
  { key: 'is_active', value: (v) => v.is_active },
]

export interface CatalogExportCtx {
  signal: AbortSignal
  onProgress: (done: number, total: number) => void
}

export async function loadVendorExportRows(ctx: CatalogExportCtx): Promise<Vendor[]> {
  return listVendors(ctx.signal)
}

// ================================================================ 2. OUI

export const ouiImportColumns: ImportColumn[] = [
  { key: 'vendor', label: 'Vendor', required: true, example: 'ZTE', hint: 'Kode (atau nama) vendor yang sudah ada di katalog — impor vendor lebih dulu bila baru.' },
  { key: 'oui', label: 'OUI', required: true, example: 'AABBCC', hint: '6 karakter huruf/angka (umumnya heksadesimal). UNIK di seluruh vendor: OUI yang sudah dimiliki vendor LAIN ditolak.' },
  { key: 'notes', label: 'Catatan', example: 'Batch 2026', aliases: ['catatan'], hint: 'Maks. 255 karakter. Catatan OUI yang sudah ada belum bisa diubah lewat API.' },
]

interface OUIRow {
  vendorId: number
  vendorCode: string
  oui: string
  notes?: string
}
interface OUIEx {
  id: number
  vendorId: number
  notes: string
}

export function ouiAdapter(): ImportAdapter {
  return catalogAdapter<OUIRow, OUIEx>({
    entity: 'OUI',
    columns: ouiImportColumns,
    template: [{ vendor: 'ZTE', oui: 'AABBCC', notes: 'Batch 2026' }],
    needs: {},
    spec: (env) => ({
      parse(rec) {
        const vendorRaw = rec.vendor ?? ''
        if (vendorRaw === '') throw new RowError('vendor wajib diisi')
        const v = env.vendors.find(vendorRaw)
        if (!v) throw new RowError(`vendor "${vendorRaw}" tidak ditemukan di katalog`)
        const oui = (rec.oui ?? '').toUpperCase()
        if (!/^[0-9A-Z]{6}$/.test(oui)) throw new RowError('OUI harus 6 karakter huruf/angka, umumnya heksadesimal (contoh: 00D0D0)')
        lenCheck('notes', rec.notes, LIMITS.vendor.notes)
        return { vendorId: v.id, vendorCode: v.code, oui, notes: rec.notes }
      },
      key: (r) => r.oui,
      label: (r) => `${r.vendorCode} / ${r.oui}`,
      // Muat OUI SEMUA vendor satu kali (satu request per vendor, bukan per baris):
      // OUI unik global, jadi kepemilikan vendor lain harus terlihat untuk pratinjau.
      async loadExisting(_rows, signal) {
        const m = new Map<string, OUIEx>()
        for (const v of env.vendors.list) {
          for (const o of await listAll<VendorOUI>(`/vendors/${v.id}/ouis`, signal)) {
            m.set(o.oui.toUpperCase(), { id: o.id, vendorId: o.vendor_id, notes: o.notes ?? '' })
          }
        }
        return m
      },
      diff(r, ex) {
        return r.notes !== undefined && blank(r.notes) !== blank(ex.notes) ? ['notes'] : []
      },
      // API belum punya PATCH OUI; baris 'update' diubah menjadi error di `post`.
      update: () => Promise.reject(new Error('ubah catatan OUI belum didukung API')),
      async create(r) {
        await api.post(`/vendors/${r.vendorId}/ouis`, { oui: r.oui, notes: blank(r.notes) || undefined })
      },
    }),
    post(items, env) {
      return items.map((it) => {
        const p = it.payload as { row: OUIRow; ex?: OUIEx } | undefined
        if (!p?.ex) return it
        // Konflik lintas vendor berlaku di SEMUA mode (OUI unik global).
        if (p.ex.vendorId !== p.row.vendorId) {
          return asError(it, `OUI ${p.row.oui} sudah dimiliki vendor ${env.vendors.codeOf(p.ex.vendorId)} — tidak bisa didaftarkan ke ${p.row.vendorCode} (OUI unik di seluruh vendor)`)
        }
        if (it.action === 'update') {
          return asError(it, 'OUI sudah terdaftar pada vendor ini dengan catatan berbeda — API belum mendukung ubah catatan (hapus OUI lalu tambahkan ulang bila perlu)')
        }
        return it
      })
    },
  })
}

export interface OUIExportRow {
  vendor: string
  oui: string
  notes: string
}

export const ouiExportColumns: ExportColumn<OUIExportRow>[] = [
  { key: 'vendor', value: (r) => r.vendor },
  { key: 'oui', value: (r) => r.oui },
  { key: 'notes', value: (r) => r.notes },
]

export async function loadOUIExportRows(ctx: CatalogExportCtx): Promise<OUIExportRow[]> {
  const vendors = await listVendors(ctx.signal)
  const out: OUIExportRow[] = []
  let done = 0
  for (const v of vendors) {
    for (const o of await listAll<VendorOUI>(`/vendors/${v.id}/ouis`, ctx.signal)) out.push({ vendor: v.code, oui: o.oui, notes: o.notes ?? '' })
    ctx.onProgress(++done, vendors.length)
  }
  return out
}

// ================================================================ 3. DEVICE MODEL

export const modelImportColumns: ImportColumn[] = [
  { key: 'vendor', label: 'Vendor', required: true, example: 'ZTE', hint: 'Kode (atau nama) vendor di katalog.' },
  { key: 'model_name', label: 'Nama Model', required: true, example: 'F670L', aliases: ['model', 'nama_model'], hint: 'Kunci pencocokan bersama vendor (tak peka huruf besar), maks. 128 karakter. Nama model yang pernah dihapus tetap terkunci di database (409).' },
  { key: 'product_class', label: 'Product Class', example: 'F670L', hint: 'ProductClass dari Inform CPE, maks. 128. Sel kosong pada model yang sudah ada = kosongkan.' },
  { key: 'device_type', label: 'Device Type', required: true, example: 'ONT', aliases: ['type', 'tipe'], hint: 'Kode dari referensi: ONT, ONU, ROUTER, ACCESS_POINT, MODEM, STB.' },
  { key: 'data_model', label: 'Data Model', required: true, example: 'TR098', aliases: ['data_model_version', 'data_model_versions'], hint: 'Kode versi data model: TR098 (InternetGatewayDevice) atau TR181 (Device). Tidak bisa diubah selama model dipakai perangkat.' },
  { key: 'description', label: 'Deskripsi', example: 'ONT GPON 4 LAN + WiFi', aliases: ['deskripsi'], hint: 'Maks. 255 karakter. Sel kosong pada model yang sudah ada = kosongkan.' },
  { key: 'is_active', label: 'Aktif', example: 'true', aliases: ['aktif', 'status'], hint: 'true/false. Kosong = aktif untuk model baru, tidak diubah untuk yang sudah ada.' },
]

interface ModelRow {
  vendorId: number
  vendorCode: string
  modelName: string
  productClass?: string
  deviceTypeId: number
  deviceTypeCode: string
  dataModelId: number
  dataModelCode: string
  description?: string
  isActive?: boolean
}

export function modelAdapter(): ImportAdapter {
  return catalogAdapter<ModelRow, DeviceModel>({
    entity: 'device model',
    columns: modelImportColumns,
    template: [{ vendor: 'ZTE', model_name: 'F670L', product_class: 'F670L', device_type: 'ONT', data_model: 'TR098', description: 'ONT GPON 4 LAN + WiFi', is_active: 'true' }],
    needs: { refs: true },
    spec: (env) => ({
      parse(rec) {
        const vendorRaw = rec.vendor ?? ''
        if (vendorRaw === '') throw new RowError('vendor wajib diisi')
        const v = env.vendors.find(vendorRaw)
        if (!v) throw new RowError(`vendor "${vendorRaw}" tidak ditemukan di katalog`)
        const modelName = rec.model_name ?? ''
        if (modelName === '') throw new RowError('model_name wajib diisi')
        lenCheck('model_name', modelName, LIMITS.deviceModel.modelName)
        lenCheck('product_class', rec.product_class, LIMITS.deviceModel.productClass)
        lenCheck('description', rec.description, LIMITS.description)
        const dtRaw = rec.device_type ?? ''
        if (dtRaw === '') throw new RowError('device_type wajib diisi')
        const dt = env.deviceTypes.find(dtRaw)
        if (!dt) throw new RowError(`device_type "${dtRaw}" tidak dikenal (kode yang valid: ${env.deviceTypes.codes})`)
        const dmRaw = rec.data_model ?? ''
        if (dmRaw === '') throw new RowError('data_model wajib diisi')
        const dm = env.dataModels.find(dmRaw)
        if (!dm) throw new RowError(`data_model "${dmRaw}" tidak dikenal (kode yang valid: ${env.dataModels.codes})`)
        return {
          vendorId: v.id,
          vendorCode: v.code,
          modelName,
          productClass: rec.product_class,
          deviceTypeId: dt.id,
          deviceTypeCode: dt.code,
          dataModelId: dm.id,
          dataModelCode: dm.code,
          description: rec.description,
          isActive: parseBool('is_active', rec.is_active ?? ''),
        }
      },
      key: (r) => `${r.vendorId}|${norm(r.modelName)}`,
      label: (r) => `${r.vendorCode} / ${r.modelName}`,
      async loadExisting(rows, signal) {
        const m = new Map<string, DeviceModel>()
        for (const vid of new Set(rows.map((r) => r.vendorId))) {
          for (const x of await listAll<DeviceModel>(`/device-models?vendor_id=${vid}`, signal)) m.set(`${vid}|${norm(x.model_name)}`, x)
        }
        return m
      },
      diff(r, ex) {
        const ch: string[] = []
        if (r.productClass !== undefined && blank(r.productClass) !== blank(ex.product_class)) ch.push('product_class')
        if (r.deviceTypeId !== ex.device_type_id) ch.push('device_type')
        if (r.dataModelId !== ex.data_model_version_id) ch.push('data_model')
        if (r.description !== undefined && blank(r.description) !== blank(ex.description)) ch.push('description')
        if (r.isActive !== undefined && r.isActive !== ex.is_active) ch.push('is_active')
        return ch
      },
      async create(r) {
        const m = await api.post<DeviceModel>('/device-models', {
          vendor_id: r.vendorId,
          device_type_id: r.deviceTypeId,
          data_model_version_id: r.dataModelId,
          model_name: r.modelName,
          product_class: blank(r.productClass) || undefined,
          description: blank(r.description) || undefined,
        })
        if (r.isActive === false) {
          try {
            await withRetry(() => api.patch(`/device-models/${m.id}`, { is_active: false }))
          } catch (e) {
            throw new PartialWriteError(`model dibuat tetapi gagal dinonaktifkan: ${e instanceof Error ? e.message : String(e)}`)
          }
        }
      },
      async update(r, ex, changes) {
        const body: Record<string, unknown> = {}
        if (changes.includes('product_class')) body.product_class = blank(r.productClass)
        if (changes.includes('device_type')) body.device_type_id = r.deviceTypeId
        if (changes.includes('data_model')) body.data_model_version_id = r.dataModelId
        if (changes.includes('description')) body.description = blank(r.description)
        if (changes.includes('is_active')) body.is_active = r.isActive
        await api.patch(`/device-models/${ex.id}`, body)
      },
    }),
  })
}

export interface ModelExportRow {
  vendor: string
  model: DeviceModel
  deviceType: string
  dataModel: string
}

export const modelExportColumns: ExportColumn<ModelExportRow>[] = [
  { key: 'vendor', value: (r) => r.vendor },
  { key: 'model_name', value: (r) => r.model.model_name },
  { key: 'product_class', value: (r) => r.model.product_class ?? '' },
  { key: 'device_type', value: (r) => r.deviceType },
  { key: 'data_model', value: (r) => r.dataModel },
  { key: 'description', value: (r) => r.model.description ?? '' },
  { key: 'is_active', value: (r) => r.model.is_active },
]

export async function loadModelExportRows(ctx: CatalogExportCtx): Promise<ModelExportRow[]> {
  const env = await loadEnv({ refs: true }, ctx.signal)
  const out: ModelExportRow[] = []
  let done = 0
  for (const v of env.vendors.list) {
    for (const m of await listAll<DeviceModel>(`/device-models?vendor_id=${v.id}`, ctx.signal)) {
      out.push({ vendor: v.code, model: m, deviceType: env.deviceTypes.codeOf(m.device_type_id), dataModel: env.dataModels.codeOf(m.data_model_version_id) })
    }
    ctx.onProgress(++done, env.vendors.list.length)
  }
  return out
}

// ================================================================ 4. PARAMETER MAPPING

export const mappingImportColumns: ImportColumn[] = [
  { key: 'vendor', label: 'Vendor', required: true, example: 'ZTE', hint: 'Kode (atau nama) vendor di katalog.' },
  { key: 'data_model', label: 'Data Model', required: true, example: 'TR098', aliases: ['data_model_version', 'data_model_versions'], hint: 'TR098 (root InternetGatewayDevice) atau TR181 (root Device). Path harus diawali root yang sesuai.' },
  { key: 'logical_key', label: 'Logical Key', required: true, example: 'wifi.5g.ssid', hint: 'Nama logis (maks. 128), mis. wifi.ssid. Tidak boleh diawali InternetGatewayDevice./Device. (dianggap path mentah, tidak pernah di-resolve).' },
  { key: 'tr069_path', label: 'TR-069 Path', required: true, example: 'InternetGatewayDevice.LANDevice.1.WLANConfiguration.5.SSID', aliases: ['parameter_path', 'path'], hint: 'Path parameter TR-069 aktual vendor (maks. 512).' },
  { key: 'device_model', label: 'Device Model', example: '', aliases: ['model', 'model_name'], hint: 'Nama model (override khusus model). Kosong = berlaku untuk semua model vendor. Model harus sudah ada & memakai data model yang sama.' },
  { key: 'software_version_pattern', label: 'Pola Versi Software', example: '', aliases: ['sw_pattern', 'software_pattern'], hint: 'Pola SQL LIKE terhadap versi software (mis. V5.%), maks. 255. Kosong = lintas semua versi.' },
  { key: 'parameter_type', label: 'Tipe Parameter', example: 'string', aliases: ['parameter_type_id'], hint: 'Kode tipe (string, int, boolean, …); kosong = tanpa tipe. Bila daftar tipe tak tersedia dari server, hasil ekspor memakai bentuk #id dan tetap bisa diimpor ulang.' },
  { key: 'description', label: 'Deskripsi', example: 'SSID WiFi 5 GHz', aliases: ['deskripsi'], hint: 'Maks. 255. Sel kosong pada mapping yang sudah ada = kosongkan.' },
]

interface MappingRow {
  vendorId: number
  vendorCode: string
  dataModelId: number
  dataModelCode: string
  modelId: number | null
  modelName: string
  pattern: string | null
  logicalKey: string
  path: string
  /** undefined = kolom tidak ada di file (tidak diubah); null = kosong. */
  paramTypeId?: number | null
  description?: string
}

const ROOT_BY_DM: Record<string, string> = { TR098: 'InternetGatewayDevice.', TR181: 'Device.' }

function mappingKey(vendorId: number, dmId: number, modelId: number | null, logicalKey: string, pattern: string | null): string {
  return [vendorId, dmId, modelId ?? 0, norm(logicalKey), pattern === null ? '' : norm(pattern)].join('|')
}

function resolveParamType(env: CatalogEnv, raw: string): number | null {
  if (raw === '') return null
  const idForm = /^#(\d+)$/.exec(raw)
  if (idForm) return Number(idForm[1])
  const r = env.paramTypes.find(raw)
  if (r) return r.id
  if (!env.paramTypes.ok) throw new RowError(`parameter_type "${raw}" tidak bisa dipetakan: daftar tipe parameter tidak tersedia dari server — gunakan bentuk #id seperti pada file ekspor`)
  throw new RowError(`parameter_type "${raw}" tidak dikenal`)
}

/** Isi POST /vendor-parameter-mappings; nilai yang tak ada di file diwarisi dari baris eksisting. */
function mappingBody(r: MappingRow, ex?: VendorParameterMapping) {
  const paramType = r.paramTypeId === undefined ? (ex?.parameter_type_id ?? null) : r.paramTypeId
  const desc = r.description === undefined ? blank(ex?.description) : blank(r.description)
  return {
    vendor_id: r.vendorId,
    data_model_version_id: r.dataModelId,
    device_model_id: r.modelId ?? undefined,
    software_version_pattern: r.pattern ?? undefined,
    logical_key: r.logicalKey,
    tr069_path: r.path,
    parameter_type_id: paramType ?? undefined,
    description: desc || undefined,
  }
}

export function mappingAdapter(): ImportAdapter {
  return catalogAdapter<MappingRow, VendorParameterMapping>({
    entity: 'parameter mapping',
    columns: mappingImportColumns,
    template: [
      { vendor: 'ZTE', data_model: 'TR098', logical_key: 'wifi.5g.ssid', tr069_path: 'InternetGatewayDevice.LANDevice.1.WLANConfiguration.5.SSID', device_model: '', software_version_pattern: '', parameter_type: 'string', description: 'SSID WiFi 5 GHz' },
    ],
    needs: { refs: true, paramTypes: true, models: true },
    spec: (env) => ({
      parse(rec) {
        const vendorRaw = rec.vendor ?? ''
        if (vendorRaw === '') throw new RowError('vendor wajib diisi')
        const v = env.vendors.find(vendorRaw)
        if (!v) throw new RowError(`vendor "${vendorRaw}" tidak ditemukan di katalog`)
        const dmRaw = rec.data_model ?? ''
        if (dmRaw === '') throw new RowError('data_model wajib diisi')
        const dm = env.dataModels.find(dmRaw)
        if (!dm) throw new RowError(`data_model "${dmRaw}" tidak dikenal (kode yang valid: ${env.dataModels.codes})`)
        const logicalKey = rec.logical_key ?? ''
        if (logicalKey === '') throw new RowError('logical_key wajib diisi')
        lenCheck('logical_key', logicalKey, LIMITS.mapping.logicalKey)
        if (/\s/.test(logicalKey)) throw new RowError('logical_key tidak boleh mengandung spasi')
        if (/^(InternetGatewayDevice|Device)\./.test(logicalKey)) {
          throw new RowError('logical_key diawali root TR-069 — dianggap path mentah dan tidak pernah di-resolve lewat mapping; pakai nama logis (mis. wifi.ssid)')
        }
        const path = rec.tr069_path ?? ''
        if (path === '') throw new RowError('tr069_path wajib diisi')
        lenCheck('tr069_path', path, LIMITS.mapping.tr069Path)
        const root = ROOT_BY_DM[dm.code.toUpperCase()]
        if (root && !path.startsWith(root)) throw new RowError(`tr069_path untuk data model ${dm.code} harus diawali "${root}"`)

        const pattern = blank(rec.software_version_pattern)
        lenCheck('software_version_pattern', pattern, LIMITS.mapping.softwareVersionPattern)
        lenCheck('description', rec.description, LIMITS.description)

        let modelId: number | null = null
        const modelName = blank(rec.device_model)
        if (modelName !== '') {
          const m = (env.models.get(v.id) ?? []).find((x) => norm(x.model_name) === norm(modelName))
          if (!m) throw new RowError(`device_model "${modelName}" tidak ditemukan pada vendor ${v.code} (impor model lebih dulu)`)
          if (m.data_model_version_id !== dm.id) {
            throw new RowError(`device_model "${modelName}" memakai data model ${env.dataModels.codeOf(m.data_model_version_id)}, bukan ${dm.code} — mapping tidak akan pernah cocok`)
          }
          modelId = m.id
        }
        return {
          vendorId: v.id,
          vendorCode: v.code,
          dataModelId: dm.id,
          dataModelCode: dm.code,
          modelId,
          modelName,
          pattern: pattern === '' ? null : pattern,
          logicalKey,
          path,
          paramTypeId: rec.parameter_type === undefined ? undefined : resolveParamType(env, rec.parameter_type),
          description: rec.description,
        }
      },
      key: (r) => mappingKey(r.vendorId, r.dataModelId, r.modelId, r.logicalKey, r.pattern),
      label: (r) =>
        [r.vendorCode, r.dataModelCode, r.logicalKey, r.modelName || null, r.pattern ? `sw ${r.pattern}` : null].filter(Boolean).join(' / '),
      // Muat mapping per vendor yang disebut di file (sekali per vendor). Bila DB
      // sudah memuat kunci ganda (POST generik tidak memicu ON DUPLICATE), baris
      // dengan id terkecil yang dipakai sebagai pembanding.
      async loadExisting(rows, signal) {
        const m = new Map<string, VendorParameterMapping>()
        for (const vid of new Set(rows.map((r) => r.vendorId))) {
          for (const x of await listAll<VendorParameterMapping>(`/vendor-parameter-mappings?vendor_id=${vid}`, signal)) {
            const k = mappingKey(vid, x.data_model_version_id, x.device_model_id, x.logical_key, x.software_version_pattern)
            if (!m.has(k)) m.set(k, x)
          }
        }
        return m
      },
      diff(r, ex) {
        const ch: string[] = []
        if (r.path !== ex.tr069_path) ch.push('tr069_path')
        if (r.paramTypeId !== undefined && r.paramTypeId !== ex.parameter_type_id) ch.push('parameter_type')
        if (r.description !== undefined && blank(r.description) !== blank(ex.description)) ch.push('description')
        return ch
      },
      async create(r) {
        await api.post('/vendor-parameter-mappings', mappingBody(r))
      },
      // API tidak punya PATCH mapping, dan POST (upsert) HANYA menimpa di tempat bila
      // device_model_id DAN software_version_pattern keduanya terisi (NULL di unique key
      // tidak pernah dianggap duplikat). Karena itu pembaruan = POST dulu (menulis
      // nilai baru), lalu hapus baris lama BILA POST membuat baris baru (id berbeda).
      // Urutan ini tidak pernah menghilangkan mapping: kegagalan di tengah hanya
      // meninggalkan baris ganda, yang dilaporkan eksplisit (dan tidak di-retry).
      async update(r, ex) {
        const resp = await withRetry(() => api.post<VendorParameterMapping>('/vendor-parameter-mappings', mappingBody(r, ex)))
        if (resp.id && resp.id !== ex.id) {
          try {
            await withRetry(() => api.del(`/vendor-parameter-mappings/${ex.id}`))
          } catch (e) {
            throw new PartialWriteError(
              `nilai baru tersimpan (id ${resp.id}) tetapi baris lama (id ${ex.id}) gagal dihapus: ${e instanceof Error ? e.message : String(e)} — hapus baris lama secara manual`,
            )
          }
        }
      },
    }),
  })
}

export interface MappingExportRow {
  vendor: string
  dataModel: string
  model: string
  paramType: string
  m: VendorParameterMapping
}

export const mappingExportColumns: ExportColumn<MappingExportRow>[] = [
  { key: 'vendor', value: (r) => r.vendor },
  { key: 'data_model', value: (r) => r.dataModel },
  { key: 'logical_key', value: (r) => r.m.logical_key },
  { key: 'tr069_path', value: (r) => r.m.tr069_path },
  { key: 'device_model', value: (r) => r.model },
  { key: 'software_version_pattern', value: (r) => r.m.software_version_pattern ?? '' },
  { key: 'parameter_type', value: (r) => r.paramType },
  { key: 'description', value: (r) => r.m.description ?? '' },
]

export async function loadMappingExportRows(ctx: CatalogExportCtx): Promise<MappingExportRow[]> {
  const env = await loadEnv({ refs: true, paramTypes: true, models: true }, ctx.signal)
  const out: MappingExportRow[] = []
  let done = 0
  for (const v of env.vendors.list) {
    const models = new Map((env.models.get(v.id) ?? []).map((x) => [x.id, x.model_name]))
    for (const m of await listAll<VendorParameterMapping>(`/vendor-parameter-mappings?vendor_id=${v.id}`, ctx.signal)) {
      out.push({
        vendor: v.code,
        dataModel: env.dataModels.codeOf(m.data_model_version_id),
        model: m.device_model_id == null ? '' : (models.get(m.device_model_id) ?? `#${m.device_model_id}`),
        paramType: env.paramTypes.codeOf(m.parameter_type_id),
        m,
      })
    }
    ctx.onProgress(++done, env.vendors.list.length)
  }
  return out
}
