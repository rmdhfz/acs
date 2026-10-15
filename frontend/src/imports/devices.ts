import { api } from '../lib/api'
import { fetchAllPages, type ExportColumn } from '../lib/exporter'
import { errorMessage } from '../lib/clientAdapter'
import {
  RowError,
  parseNumber,
  type ExecResult,
  type ImportAdapter,
  type ImportColumn,
  type ImportMode,
  type PlanItem,
  type RawRecord,
} from '../lib/importer'
import { withRetry } from '../lib/pool'
import type { Device, DeviceModel, Tenant, Vendor } from '../lib/types'

// Impor/ekspor PERANGKAT. Berbeda dari modul lain, impor perangkat memakai
// endpoint massal di server (POST /devices/import) karena volume bisa ribuan:
// server memvalidasi, memeriksa data eksisting (OUI+serial), dan mengklasifikasi
// baris (dry_run=true untuk pratinjau) — satu round trip per 500 baris.

const CHUNK = 500

export interface DeviceImportEnv {
  isSuperadmin: boolean
  vendors: Vendor[]
  /** Hanya terisi untuk superadmin (GET /tenants khusus superadmin). */
  tenants: Tenant[]
}

export const deviceImportColumns: ImportColumn[] = [
  { key: 'tenant', label: 'Tenant', hint: 'Kode/nama tenant. WAJIB untuk superadmin; diabaikan untuk ADMIN (otomatis tenant sendiri).', example: 'acme', aliases: ['tenant_code', 'tenant_id'] },
  { key: 'oui', label: 'OUI', required: true, example: '00D0D0', hint: '6 karakter (umumnya heksadesimal) — kode produsen pada DeviceId TR-069.' },
  { key: 'serial_number', label: 'Serial Number', required: true, example: 'ZTEGC8A1B2C3', aliases: ['serial', 'sn'], hint: 'Unik bersama OUI. Kunci pencocokan data eksisting.' },
  { key: 'vendor', label: 'Vendor', example: 'ZTE', hint: 'Kode/nama vendor di katalog. Kosong = dikenali otomatis dari OUI.' },
  { key: 'model', label: 'Model', example: 'F670L', hint: 'Nama model/product class di katalog vendor (butuh kolom vendor).' },
  { key: 'product_class', label: 'Product Class', example: 'F670L' },
  { key: 'mac_address', label: 'MAC Address', example: 'AA:BB:CC:DD:EE:FF', aliases: ['mac'], hint: 'Format bebas (: - . atau tanpa pemisah), disimpan AA:BB:CC:DD:EE:FF.' },
  { key: 'notes', label: 'Catatan', example: 'Pelanggan korporat, lantai 2', aliases: ['catatan'], hint: 'Maks. 255 karakter.' },
  { key: 'latitude', label: 'Latitude', example: '-6.2088', hint: 'Berpasangan dengan longitude.' },
  { key: 'longitude', label: 'Longitude', example: '106.8456' },
  { key: 'connection_request_url', label: 'Connection Request URL', example: 'http://203.0.113.7:7547/', hint: 'Opsional; biasanya terisi otomatis dari Inform.' },
  { key: 'connection_request_username', label: 'Connection Request Username', secret: true, example: 'acs', hint: 'Opsional.' },
  { key: 'connection_request_password', label: 'Connection Request Password', secret: true, example: 'rahasia', hint: 'Opsional; tersimpan terenkripsi, TIDAK pernah diekspor.' },
  // informasi saja (ikut diekspor, diabaikan saat impor)
  { key: 'status', label: 'Status', readonly: true },
  { key: 'ip_address', label: 'IP Address', readonly: true },
  { key: 'software_version', label: 'Firmware', readonly: true },
  { key: 'hardware_version', label: 'Hardware', readonly: true },
  { key: 'last_inform_at', label: 'Terakhir terhubung', readonly: true },
  { key: 'device_uuid', label: 'UUID', readonly: true },
]

const norm = (s: string) => s.trim().toLowerCase()

interface ServerRow {
  tenant_id?: number
  oui: string
  serial_number: string
  product_class?: string
  mac_address?: string
  notes?: string
  vendor_id?: number
  device_model_id?: number
  latitude?: number
  longitude?: number
  connection_request_url?: string
  connection_request_username?: string
  connection_request_password?: string
}

interface ParsedRow {
  row: ServerRow
  vendorId?: number
  modelName: string
}

interface ServerResult {
  index: number
  action: 'create' | 'update' | 'unchanged' | 'skip' | 'error'
  oui: string
  serial_number: string
  changes?: string[]
  error?: string
}
interface ServerResponse {
  results: ServerResult[]
}

const orUndef = (s: string) => (s === '' ? undefined : s)

function parseRow(rec: RawRecord, env: DeviceImportEnv): ParsedRow {
  const oui = rec.oui.toUpperCase()
  if (!/^[0-9A-Z]{6}$/.test(oui)) throw new RowError('OUI harus 6 karakter huruf/angka, umumnya heksadesimal (contoh: 00D0D0)')
  if (rec.serial_number === '') throw new RowError('serial_number wajib diisi')
  if (rec.serial_number.length > 128) throw new RowError('serial_number maks. 128 karakter')

  const lat = parseNumber('latitude', rec.latitude ?? '')
  const lon = parseNumber('longitude', rec.longitude ?? '')
  if ((lat === undefined) !== (lon === undefined)) throw new RowError('latitude dan longitude harus diisi berpasangan')
  if (lat !== undefined && (lat < -90 || lat > 90)) throw new RowError('latitude harus di antara -90 dan 90')
  if (lon !== undefined && (lon < -180 || lon > 180)) throw new RowError('longitude harus di antara -180 dan 180')
  if ((rec.notes ?? '').length > 255) throw new RowError('catatan maks. 255 karakter')

  const row: ServerRow = {
    oui,
    serial_number: rec.serial_number,
    product_class: orUndef(rec.product_class ?? ''),
    mac_address: orUndef(rec.mac_address ?? ''),
    notes: orUndef(rec.notes ?? ''),
    latitude: lat,
    longitude: lon,
    connection_request_url: orUndef(rec.connection_request_url ?? ''),
    connection_request_username: orUndef(rec.connection_request_username ?? ''),
    connection_request_password: orUndef(rec.connection_request_password ?? ''),
  }

  const tenantRaw = rec.tenant ?? ''
  if (env.isSuperadmin) {
    if (tenantRaw === '') throw new RowError('kolom tenant wajib diisi (superadmin tidak terikat tenant)')
    const t = env.tenants.find((x) => norm(x.code) === norm(tenantRaw) || norm(x.name) === norm(tenantRaw) || String(x.id) === tenantRaw.trim())
    if (!t) throw new RowError(`tenant "${tenantRaw}" tidak ditemukan`)
    row.tenant_id = t.id
  }

  let vendorId: number | undefined
  const vendorRaw = rec.vendor ?? ''
  if (vendorRaw !== '') {
    const v = env.vendors.find((x) => norm(x.code) === norm(vendorRaw) || norm(x.name) === norm(vendorRaw) || String(x.id) === vendorRaw.trim())
    if (!v) throw new RowError(`vendor "${vendorRaw}" tidak ditemukan di katalog`)
    vendorId = v.id
    row.vendor_id = v.id
  }
  const modelName = rec.model ?? ''
  if (modelName !== '' && vendorId === undefined) throw new RowError('kolom model butuh kolom vendor')
  return { row, vendorId, modelName }
}

async function resolveModels(parsed: { p: ParsedRow }[], signal: AbortSignal): Promise<Map<number, DeviceModel[]>> {
  const vendorIds = [...new Set(parsed.filter((x) => x.p.modelName !== '').map((x) => x.p.vendorId!))]
  const out = new Map<number, DeviceModel[]>()
  for (const vid of vendorIds) {
    if (signal.aborted) break
    // API membalas null (bukan []) untuk vendor tanpa model.
    out.set(vid, (await withRetry(() => api.get<DeviceModel[] | null>(`/device-models?vendor_id=${vid}`), signal)) ?? [])
  }
  return out
}

function chunks<T>(arr: T[], n: number): T[][] {
  const out: T[][] = []
  for (let i = 0; i < arr.length; i += n) out.push(arr.slice(i, i + n))
  return out
}

export function deviceAdapter(env: DeviceImportEnv): ImportAdapter {
  return {
    entity: 'perangkat',
    columns: deviceImportColumns,
    template: [
      {
        tenant: env.isSuperadmin ? (env.tenants[0]?.code ?? 'acme') : '',
        oui: '00D0D0',
        serial_number: 'ZTEGC8A1B2C3',
        vendor: env.vendors[0]?.code ?? '',
        model: '',
        product_class: 'F670L',
        mac_address: 'AA:BB:CC:DD:EE:FF',
        notes: 'Pelanggan korporat, lantai 2',
        latitude: '-6.2088',
        longitude: '106.8456',
        connection_request_url: '',
        connection_request_username: '',
        connection_request_password: '',
      },
    ],
    supportsUpdate: true,

    async plan(records, mode: ImportMode, ctx) {
      const items: PlanItem<ServerRow>[] = []
      const valid: { p: ParsedRow; rowNo: number; label: string }[] = []
      const seen = new Map<string, number>()

      records.forEach((rec, i) => {
        const rowNo = i + 1
        try {
          const p = parseRow(rec, env)
          const label = `${p.row.oui} / ${p.row.serial_number}`
          // OUI+serial unik GLOBAL di server, jadi duplikat dideteksi tanpa tenant
          const gkey = `${p.row.oui}|${p.row.serial_number.toLowerCase()}`
          const first = seen.get(gkey)
          if (first !== undefined) {
            items.push({ rowNo, label, action: 'error', error: `duplikat dalam file (sama dengan baris ${first})` })
            return
          }
          seen.set(gkey, rowNo)
          valid.push({ p, rowNo, label })
        } catch (e) {
          if (!(e instanceof RowError)) throw e
          items.push({ rowNo, label: `baris ${rowNo}`, action: 'error', error: e.message })
        }
      })

      // model → device_model_id (per vendor)
      const modelsByVendor = await resolveModels(valid, ctx.signal)
      const sendable: { rowNo: number; label: string; row: ServerRow }[] = []
      for (const v of valid) {
        if (v.p.modelName !== '') {
          const list = modelsByVendor.get(v.p.vendorId!) ?? []
          const m = list.find((x) => norm(x.model_name) === norm(v.p.modelName) || (x.product_class && norm(x.product_class) === norm(v.p.modelName)))
          if (!m) {
            items.push({ rowNo: v.rowNo, label: v.label, action: 'error', error: `model "${v.p.modelName}" tidak ditemukan pada vendor tersebut` })
            continue
          }
          v.p.row.device_model_id = m.id
        }
        sendable.push({ rowNo: v.rowNo, label: v.label, row: v.p.row })
      }

      // dry run di server, per 500 baris
      for (const part of chunks(sendable, CHUNK)) {
        if (ctx.signal.aborted) break
        const resp = await withRetry(
          () => api.post<ServerResponse>('/devices/import?dry_run=true', { mode, rows: part.map((p) => p.row) }),
          ctx.signal,
        )
        for (const r of resp.results) {
          const src = part[r.index]
          items.push({
            rowNo: src.rowNo,
            label: src.label,
            action: r.action,
            changes: r.changes,
            error: r.error,
            payload: src.row,
          })
        }
      }
      items.sort((a, b) => a.rowNo - b.rowNo)
      return items
    },

    async execute(items, mode, ctx) {
      const todo = items.filter((i) => i.action === 'create' || i.action === 'update')
      const out: ExecResult[] = []
      let done = 0
      for (const part of chunks(todo, CHUNK)) {
        if (ctx.signal.aborted) {
          for (const t of part) out.push({ rowNo: t.rowNo, label: t.label, action: t.action, ok: false, error: 'dibatalkan sebelum diproses' })
          continue
        }
        try {
          const resp = await withRetry(
            () => api.post<ServerResponse>('/devices/import', { mode, rows: part.map((p) => p.payload as ServerRow) }),
            ctx.signal,
          )
          for (const r of resp.results) {
            const src = part[r.index]
            // unchanged/skip saat eksekusi = data berubah sejak pratinjau; tidak ada yang perlu ditulis (bukan kegagalan)
            out.push({ rowNo: src.rowNo, label: src.label, action: src.action, ok: r.action !== 'error', error: r.error })
          }
        } catch (e) {
          for (const t of part) out.push({ rowNo: t.rowNo, label: t.label, action: t.action, ok: false, error: errorMessage(e) })
        }
        done += part.length
        ctx.onProgress(done, todo.length)
      }
      return out
    },
  }
}

// ---------------------------------------------------------------- ekspor

export interface DeviceExportRow extends Device {
  _tenant: string
  _vendor: string
  _model: string
  _status: string
}

export function deviceExportColumns(): ExportColumn<DeviceExportRow>[] {
  return [
    { key: 'tenant', value: (d) => d._tenant },
    { key: 'oui', value: (d) => d.oui },
    { key: 'serial_number', value: (d) => d.serial_number },
    { key: 'vendor', value: (d) => d._vendor },
    { key: 'model', value: (d) => d._model },
    { key: 'product_class', value: (d) => d.product_class },
    { key: 'mac_address', value: (d) => d.mac_address },
    { key: 'notes', value: (d) => d.notes },
    { key: 'latitude', value: (d) => d.latitude },
    { key: 'longitude', value: (d) => d.longitude },
    { key: 'connection_request_url', value: (d) => d.connection_request_url },
    // kolom rahasia (connection_request_username/password) TIDAK diekspor
    { key: 'status', value: (d) => d._status },
    { key: 'ip_address', value: (d) => d.ip_address },
    { key: 'software_version', value: (d) => d.software_version },
    { key: 'hardware_version', value: (d) => d.hardware_version },
    { key: 'last_inform_at', value: (d) => d.last_inform_at },
    { key: 'device_uuid', value: (d) => d.device_uuid },
  ]
}

export async function loadDeviceExportRows(
  filters: Record<string, string | number | undefined>,
  lookup: { vendors: Vendor[]; tenants: Tenant[]; tenantName?: string; statuses: { id: number; name: string }[] },
  ctx: { signal: AbortSignal; onProgress: (done: number, total: number) => void },
): Promise<DeviceExportRow[]> {
  const { rows, truncated } = await fetchAllPages<Device>('/devices', filters, { signal: ctx.signal, onProgress: ctx.onProgress })
  const vendorIds = [...new Set(rows.map((r) => r.vendor_id).filter((x): x is number => x != null))]
  const models = new Map<number, string>()
  for (const vid of vendorIds) {
    const list = (await api.get<DeviceModel[] | null>(`/device-models?vendor_id=${vid}`)) ?? []
    for (const m of list) models.set(m.id, m.model_name)
  }
  const out = rows.map((d) => ({
    ...d,
    _tenant: d.tenant_id == null ? '' : (lookup.tenants.find((t) => t.id === d.tenant_id)?.code ?? lookup.tenantName ?? String(d.tenant_id)),
    _vendor: d.vendor_id == null ? '' : (lookup.vendors.find((v) => v.id === d.vendor_id)?.code ?? ''),
    _model: d.device_model_id == null ? '' : (models.get(d.device_model_id) ?? ''),
    _status: lookup.statuses.find((s) => s.id === d.device_status_id)?.name ?? '',
  }))
  if (truncated) throw new Error('Data melebihi 50.000 baris — persempit dengan filter lalu ekspor lagi.')
  return out
}
