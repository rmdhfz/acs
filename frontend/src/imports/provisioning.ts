import { api } from '../lib/api'
import { clientAdapter } from '../lib/clientAdapter'
import type { ExportColumn } from '../lib/exporter'
import { RowError, parseBool, parseNumber, type ImportAdapter, type ImportColumn, type PlanItem } from '../lib/importer'
import { runPool, withRetry } from '../lib/pool'
import type { DeviceModel, FirmwareFile, ListResponse, ProvisioningProfile, ProvisioningProfileParameter, RefLookup, Vendor, ZeroTouchRule } from '../lib/types'
import {
  assertComplete,
  canon,
  findModel,
  findVendor,
  guardScope,
  isSensitiveName,
  listAll,
  loadModels,
  markAmbiguous,
  norm,
  orUndef,
  parseJSONCell,
  parseScope,
  toError,
  withHooks,
  type Scope,
} from './opsShared'

// Impor/ekspor PROVISIONING: profil (beserta parameter) dan aturan ZERO-TOUCH.
//
// Profil — kunci: cakupan + nama. Kolom `parameters` = teks JSON
// [{"name":"InternetGatewayDevice...","value":"...","order":1}]. Nilai parameter
// yang namanya terlihat rahasia (password/psk/…) TIDAK diekspor; saat memperbarui,
// nilai kosong pada nama rahasia = pertahankan nilai tersimpan. Sesuai FR-18
// (PRD), mengubah profil lewat impor TIDAK mendorong ulang ke device yang sudah
// terprovisioning.
//
// Aturan ZTP tidak punya nama; kuncinya adalah kombinasi kriteria pencocokan
// (cakupan, vendor, model, OUI, pola serial/versi, parameter pencocok, trigger).

export interface ProvisioningImportEnv {
  isSuper: boolean
  /** tenant_id akun login (dipakai untuk data ber-scope tenant). */
  tenantId: number | null
  vendors: Vendor[]
}

const scopeOfTenant = (tenantId: number | null): Scope => (tenantId == null ? 'global' : 'tenant')

// ================================================================ PROFIL

export const profileImportColumns: ImportColumn[] = [
  { key: 'name', label: 'Nama', required: true, example: 'Profil WiFi Korporat', hint: 'Kunci pencocokan (bersama scope), maks. 128.' },
  { key: 'scope', label: 'Scope', example: 'tenant', hint: 'global (hanya SUPERADMIN) atau tenant.', aliases: ['cakupan'] },
  { key: 'vendor', label: 'Vendor', example: 'ZTE', hint: 'Kode/nama vendor; kosong = semua vendor.' },
  { key: 'model', label: 'Model', example: 'F670L', hint: 'Butuh kolom vendor; kosong = semua model.' },
  { key: 'description', label: 'Deskripsi', example: 'Profil standar pelanggan korporat' },
  { key: 'is_default', label: 'Default', example: 'false', hint: 'true = dipakai untuk device yang cocok bila tak ada aturan lain.' },
  { key: 'is_active', label: 'Aktif', example: 'true', hint: 'Hanya berlaku saat memperbarui (profil baru selalu aktif).' },
  {
    key: 'parameters',
    label: 'Parameter',
    example: '[{"name":"InternetGatewayDevice.LANDevice.1.WLANConfiguration.1.SSID","value":"Kantor","order":1}]',
    hint: 'Teks JSON array {name,value,order}. Kosong = tidak diubah.',
  },
  { key: 'created_at', label: 'Dibuat', readonly: true },
]

interface ParamIn {
  name: string
  value: string
  order: number
}
interface ProfileRow {
  name: string
  scope: Scope
  vendorId?: number
  modelId?: number
  description?: string
  isDefault?: boolean
  isActive?: boolean
  params?: ParamIn[]
}
type ProfileEx = ProvisioningProfile & { dups: number; params: ProvisioningProfileParameter[] }

function parseParams(raw: string | undefined): ParamIn[] | undefined {
  const v = parseJSONCell('parameters', raw)
  if (v === undefined) return undefined
  if (!Array.isArray(v)) throw new RowError('parameters harus array JSON')
  const seen = new Set<string>()
  return v.map((p, i) => {
    if (!p || typeof p !== 'object') throw new RowError(`parameters[${i}] harus objek`)
    const o = p as Record<string, unknown>
    const name = typeof o.name === 'string' ? o.name.trim() : ''
    if (name === '') throw new RowError(`parameters[${i}].name wajib diisi`)
    if (seen.has(name)) throw new RowError(`parameters: nama ganda "${name}"`)
    seen.add(name)
    const order = o.order === undefined ? i + 1 : Number(o.order)
    if (!Number.isInteger(order) || order < 0) throw new RowError(`parameters[${i}].order harus bilangan bulat ≥ 0`)
    return { name, value: o.value === undefined || o.value === null ? '' : String(o.value), order }
  })
}

function restoreProfileSecrets(params: ParamIn[], existing: ProvisioningProfileParameter[]): ParamIn[] {
  return params.map((p) => {
    if (isSensitiveName(p.name) && p.value === '') {
      const old = existing.find((e) => e.parameter_name === p.name)
      if (old?.parameter_value) return { ...p, value: old.parameter_value }
    }
    return p
  })
}

function secretMissingIn(params: ParamIn[]): string | undefined {
  const bad = params.find((p) => isSensitiveName(p.name) && p.value === '')
  return bad ? `nilai parameter rahasia "${bad.name}" kosong — isi nilainya (tidak diekspor demi keamanan)` : undefined
}

export function profileAdapter(env: ProvisioningImportEnv): ImportAdapter {
  let models: DeviceModel[] = []

  const base = clientAdapter<ProfileRow, ProfileEx>({
    entity: 'profil provisioning',
    columns: profileImportColumns,
    template: [
      {
        name: 'Profil WiFi Korporat',
        scope: env.isSuper ? 'global' : 'tenant',
        vendor: env.vendors[0]?.code ?? '',
        model: '',
        description: 'Profil standar pelanggan korporat',
        is_default: 'false',
        is_active: 'true',
        parameters: '[{"name":"InternetGatewayDevice.LANDevice.1.WLANConfiguration.1.SSID","value":"Kantor","order":1}]',
      },
    ],
    parse(rec) {
      if (rec.name === '') throw new RowError('nama wajib diisi')
      if (rec.name.length > 128) throw new RowError('nama maks. 128 karakter')
      let vendorId: number | undefined
      if ((rec.vendor ?? '') !== '') {
        const v = findVendor(env.vendors, rec.vendor)
        if (!v) throw new RowError(`vendor "${rec.vendor}" tidak ditemukan di katalog`)
        vendorId = v.id
      }
      let modelId: number | undefined
      if ((rec.model ?? '') !== '') {
        if (vendorId === undefined) throw new RowError('kolom model butuh kolom vendor')
        modelId = findModel(models, vendorId, rec.model).id
      }
      return {
        name: rec.name,
        scope: parseScope(rec.scope, env.isSuper),
        vendorId,
        modelId,
        description: orUndef(rec.description),
        isDefault: parseBool('is_default', rec.is_default ?? ''),
        isActive: parseBool('is_active', rec.is_active ?? ''),
        params: parseParams(rec.parameters),
      }
    },
    key: (r) => `${r.scope}|${norm(r.name)}`,
    label: (r) => r.name,

    async loadExisting(rows, signal) {
      const { rows: all } = await listAll<ProvisioningProfile>('/provisioning-profiles', {}, { signal })
      const wanted = new Set(rows.map((r) => `${r.scope}|${norm(r.name)}`))
      const m = new Map<string, ProfileEx>()
      const matched: ProvisioningProfile[] = []
      for (const p of all) {
        const k = `${scopeOfTenant(p.tenant_id)}|${norm(p.name)}`
        if (!wanted.has(k)) continue
        const cur = m.get(k)
        if (cur) m.set(k, { ...cur, dups: cur.dups + 1 })
        else {
          m.set(k, { ...p, dups: 1, params: [] })
          matched.push(p)
        }
      }
      // Detail (parameter) hanya untuk profil yang cocok dengan baris file.
      await runPool(
        matched,
        async (p) => {
          const d = await withRetry(() => api.get<{ profile: ProvisioningProfile; parameters: ProvisioningProfileParameter[] | null }>(`/provisioning-profiles/${p.id}`), signal)
          const k = `${scopeOfTenant(p.tenant_id)}|${norm(p.name)}`
          m.set(k, { ...(m.get(k) as ProfileEx), params: d.parameters ?? [] })
        },
        { concurrency: 4, signal },
      )
      return m
    },

    diff(row, ex) {
      const ch: string[] = []
      if (row.vendorId !== undefined && row.vendorId !== ex.vendor_id) ch.push('vendor')
      if (row.modelId !== undefined && row.modelId !== ex.device_model_id) ch.push('model')
      if (row.description !== undefined && row.description !== (ex.description ?? '')) ch.push('description')
      if (row.isDefault !== undefined && row.isDefault !== ex.is_default) ch.push('is_default')
      if (row.isActive !== undefined && row.isActive !== ex.is_active) ch.push('is_active')
      if (row.params !== undefined) {
        const merged = restoreProfileSecrets(row.params, ex.params)
        const cur = ex.params.map((p) => ({ name: p.parameter_name, value: p.parameter_value ?? '', order: p.apply_order }))
        if (canon([...merged].sort((a, b) => a.name.localeCompare(b.name))) !== canon([...cur].sort((a, b) => a.name.localeCompare(b.name)))) ch.push('parameters')
      }
      return ch
    },

    async create(row) {
      const params = row.params ?? []
      const miss = secretMissingIn(params)
      if (miss) throw new RowError(miss)
      await api.post<ProvisioningProfile>('/provisioning-profiles', {
        ...(row.scope === 'tenant' && env.tenantId != null ? { tenant_id: env.tenantId } : {}),
        vendor_id: row.vendorId,
        device_model_id: row.modelId,
        name: row.name,
        description: row.description,
        is_default: row.isDefault ?? false,
        parameters: params.map((p) => ({ parameter_name: p.name, parameter_value: p.value, apply_order: p.order })),
      })
    },

    async update(row, ex) {
      const params = row.params !== undefined ? restoreProfileSecrets(row.params, ex.params) : ex.params.map((p) => ({ name: p.parameter_name, value: p.parameter_value ?? '', order: p.apply_order }))
      const miss = secretMissingIn(params)
      if (miss) throw new RowError(miss)
      await api.put<void>(`/provisioning-profiles/${ex.id}`, {
        vendor_id: row.vendorId !== undefined ? row.vendorId : ex.vendor_id,
        device_model_id: row.modelId !== undefined ? row.modelId : ex.device_model_id,
        name: ex.name,
        description: row.description !== undefined ? row.description : ex.description,
        is_default: row.isDefault ?? ex.is_default,
        is_active: row.isActive ?? ex.is_active,
        parameters: params.map((p) => ({ parameter_name: p.name, parameter_value: p.value, apply_order: p.order })),
      })
    },
  })

  return withHooks(base, {
    before: async (_m, ctx) => {
      models = await loadModels(env.vendors, ctx.signal)
    },
    after: (items) => markAmbiguous(guardScope(items, env.isSuper, 'profil', true), 'profil').map(checkProfileSecrets),
  })
}

/** Sama dengan validasi saat eksekusi — agar pratinjau tidak menjanjikan baris yang pasti gagal. */
function checkProfileSecrets(it: PlanItem): PlanItem {
  if (it.action !== 'create' && it.action !== 'update') return it
  const p = it.payload as { row: ProfileRow; ex?: ProfileEx } | undefined
  if (!p) return it
  let params: ParamIn[]
  if (it.action === 'create') params = p.row.params ?? []
  else if (p.row.params !== undefined) params = restoreProfileSecrets(p.row.params, p.ex?.params ?? [])
  else return it
  const miss = secretMissingIn(params)
  return miss ? toError(it, miss) : it
}

export interface ProfileExportRow extends ProvisioningProfile {
  _vendor: string
  _model: string
  _params: ProvisioningProfileParameter[]
}

export function profileExportColumns(): ExportColumn<ProfileExportRow>[] {
  return [
    { key: 'name', value: (p) => p.name },
    { key: 'scope', value: (p) => scopeOfTenant(p.tenant_id) },
    { key: 'vendor', value: (p) => p._vendor },
    { key: 'model', value: (p) => p._model },
    { key: 'description', value: (p) => p.description },
    { key: 'is_default', value: (p) => p.is_default },
    { key: 'is_active', value: (p) => p.is_active },
    {
      key: 'parameters',
      value: (p) =>
        JSON.stringify(
          [...p._params]
            .sort((a, b) => a.apply_order - b.apply_order)
            .map((x) => ({ name: x.parameter_name, value: isSensitiveName(x.parameter_name) ? '' : (x.parameter_value ?? ''), order: x.apply_order })),
        ),
    },
    { key: 'created_at', value: (p) => p.created_at },
  ]
}

export async function loadProfileExportRows(vendors: Vendor[], ctx: { signal: AbortSignal; onProgress: (done: number, total: number) => void }): Promise<ProfileExportRow[]> {
  const { rows, truncated } = await listAll<ProvisioningProfile>('/provisioning-profiles', {}, { signal: ctx.signal })
  assertComplete(truncated)
  const models = await loadModels(vendors, ctx.signal)
  const out: ProfileExportRow[] = []
  await runPool(
    rows,
    async (p, i) => {
      const d = await withRetry(() => api.get<{ parameters: ProvisioningProfileParameter[] | null }>(`/provisioning-profiles/${p.id}`), ctx.signal)
      out[i] = {
        ...p,
        _vendor: vendors.find((v) => v.id === p.vendor_id)?.code ?? '',
        _model: models.find((m) => m.id === p.device_model_id)?.model_name ?? '',
        _params: d.parameters ?? [],
      }
    },
    { concurrency: 4, signal: ctx.signal, onProgress: (done, total) => ctx.onProgress(done, total) },
  )
  return out.filter(Boolean)
}

// ================================================================ ZTP

export const ztpImportColumns: ImportColumn[] = [
  { key: 'scope', label: 'Scope', example: 'tenant', hint: 'global (hanya SUPERADMIN) atau tenant. Bagian dari kunci.', aliases: ['cakupan'] },
  { key: 'vendor', label: 'Vendor', example: 'ZTE', hint: 'Kriteria pencocokan; kosong = semua.' },
  { key: 'model', label: 'Model', example: 'F670L', hint: 'Butuh kolom vendor.' },
  { key: 'oui', label: 'OUI', example: '00D0D0', hint: 'Kriteria pencocokan (6 karakter).' },
  { key: 'serial_pattern', label: 'Pola serial', example: 'ZTEG%', hint: 'Pola SQL LIKE.' },
  { key: 'software_version_pattern', label: 'Pola versi firmware', example: 'V1.1%' },
  { key: 'match_parameter_name', label: 'Parameter pencocok', hint: 'Wajib berpasangan dengan pola nilainya.' },
  { key: 'match_parameter_value_pattern', label: 'Pola nilai parameter' },
  { key: 'trigger', label: 'Trigger', required: true, example: 'BOOTSTRAP_ONLY', hint: 'Kode ref_ztp_trigger_event (mis. BOOTSTRAP_ONLY, BOOTSTRAP_OR_BOOT, EVERY_INFORM). Bagian dari kunci.' },
  { key: 'profile', label: 'Profil', example: 'Profil WiFi Korporat', hint: 'Nama profil provisioning yang sudah ada. Kosong = tanpa profil.' },
  { key: 'post_apply_reboot', label: 'Reboot setelah apply', example: 'false' },
  { key: 'firmware_version', label: 'Versi firmware', example: '', hint: 'Versi firmware milik vendor aturan (butuh kolom vendor).' },
  { key: 'priority', label: 'Prioritas', example: '100', hint: 'Bilangan bulat ≥ 0; lebih kecil dievaluasi lebih dulu.' },
  { key: 'is_active', label: 'Aktif', example: 'true' },
]

interface ZtpRow {
  scope: Scope
  vendorId?: number
  modelId?: number
  oui?: string
  serial?: string
  swPattern?: string
  mpName?: string
  mpValue?: string
  triggerId: number
  profileId?: number
  reboot?: boolean
  firmwareId?: number
  priority?: number
  isActive?: boolean
}
type ZtpEx = ZeroTouchRule & { dups: number }

const ztpKey = (s: Scope, vendor?: number | null, model?: number | null, oui?: string | null, serial?: string | null, sw?: string | null, mpn?: string | null, mpv?: string | null, trig?: number) =>
  [s, vendor ?? '', model ?? '', (oui ?? '').toUpperCase(), serial ?? '', sw ?? '', mpn ?? '', mpv ?? '', trig ?? ''].join('|')

async function loadFirmware(vendors: Vendor[], signal?: AbortSignal): Promise<FirmwareFile[]> {
  const lists = await runPool(
    vendors,
    async (v) => (await withRetry(() => api.get<ListResponse<FirmwareFile> | null>(`/firmware?vendor_id=${v.id}&page_size=200`), signal))?.data ?? [],
    { concurrency: 4, signal },
  )
  return lists.flatMap((l) => l ?? [])
}

export function ztpAdapter(env: ProvisioningImportEnv): ImportAdapter {
  let models: DeviceModel[] = []
  let profiles: ProvisioningProfile[] = []
  let firmware: FirmwareFile[] = []
  let triggers: RefLookup[] = []

  const base = clientAdapter<ZtpRow, ZtpEx>({
    entity: 'aturan zero-touch',
    columns: ztpImportColumns,
    template: [
      {
        scope: env.isSuper ? 'global' : 'tenant',
        vendor: env.vendors[0]?.code ?? '',
        model: '',
        oui: '',
        serial_pattern: '',
        software_version_pattern: '',
        match_parameter_name: '',
        match_parameter_value_pattern: '',
        trigger: 'BOOTSTRAP_ONLY',
        profile: '',
        post_apply_reboot: 'false',
        firmware_version: '',
        priority: '100',
        is_active: 'true',
      },
    ],
    parse(rec) {
      const trig = triggers.find((t) => norm(t.code) === norm(rec.trigger ?? '') || norm(t.name) === norm(rec.trigger ?? ''))
      if (!trig) throw new RowError(`trigger "${rec.trigger}" tidak dikenal (valid: ${triggers.map((t) => t.code).join(', ')})`)
      const scope = parseScope(rec.scope, env.isSuper)
      let vendorId: number | undefined
      if ((rec.vendor ?? '') !== '') {
        const v = findVendor(env.vendors, rec.vendor)
        if (!v) throw new RowError(`vendor "${rec.vendor}" tidak ditemukan di katalog`)
        vendorId = v.id
      }
      let modelId: number | undefined
      if ((rec.model ?? '') !== '') {
        if (vendorId === undefined) throw new RowError('kolom model butuh kolom vendor')
        modelId = findModel(models, vendorId, rec.model).id
      }
      const oui = (rec.oui ?? '').toUpperCase()
      if (oui !== '' && !/^[0-9A-Z]{6}$/.test(oui)) throw new RowError('OUI harus 6 karakter huruf/angka')
      const mpName = orUndef(rec.match_parameter_name)
      const mpValue = orUndef(rec.match_parameter_value_pattern)
      if ((mpName === undefined) !== (mpValue === undefined)) throw new RowError('match_parameter_name dan match_parameter_value_pattern harus berpasangan')

      let profileId: number | undefined
      if ((rec.profile ?? '') !== '') {
        const hits = profiles.filter((p) => norm(p.name) === norm(rec.profile))
        const same = hits.filter((p) => scopeOfTenant(p.tenant_id) === scope)
        const pick = same.length > 0 ? same : hits
        if (pick.length === 0) throw new RowError(`profil "${rec.profile}" tidak ditemukan`)
        if (pick.length > 1) throw new RowError(`profil "${rec.profile}" ambigu (ada ${pick.length})`)
        profileId = pick[0].id
      }
      let firmwareId: number | undefined
      if ((rec.firmware_version ?? '') !== '') {
        if (vendorId === undefined) throw new RowError('firmware_version butuh kolom vendor')
        const hits = firmware.filter((f) => f.vendor_id === vendorId && norm(f.version) === norm(rec.firmware_version))
        if (hits.length === 0) throw new RowError(`firmware versi "${rec.firmware_version}" tidak ditemukan pada vendor tersebut`)
        if (hits.length > 1) throw new RowError(`firmware versi "${rec.firmware_version}" ambigu`)
        firmwareId = hits[0].id
      }
      const priority = parseNumber('prioritas', rec.priority ?? '')
      if (priority !== undefined && (!Number.isInteger(priority) || priority < 0)) throw new RowError('prioritas harus bilangan bulat ≥ 0')
      const row: ZtpRow = {
        scope,
        vendorId,
        modelId,
        oui: oui === '' ? undefined : oui,
        serial: orUndef(rec.serial_pattern),
        swPattern: orUndef(rec.software_version_pattern),
        mpName,
        mpValue,
        triggerId: trig.id,
        profileId,
        reboot: parseBool('post_apply_reboot', rec.post_apply_reboot ?? ''),
        firmwareId,
        priority,
        isActive: parseBool('is_active', rec.is_active ?? ''),
      }
      if (row.profileId === undefined && !row.reboot && row.firmwareId === undefined) {
        throw new RowError('aturan harus melakukan sesuatu: isi profil, firmware, atau reboot')
      }
      return row
    },
    key: (r) => ztpKey(r.scope, r.vendorId, r.modelId, r.oui, r.serial, r.swPattern, r.mpName, r.mpValue, r.triggerId),
    label: (r) => {
      const parts = [r.oui, r.serial, r.swPattern].filter(Boolean)
      const v = env.vendors.find((x) => x.id === r.vendorId)?.code
      return [v, ...parts].filter(Boolean).join(' / ') || '(semua perangkat)'
    },

    async loadExisting(_rows, signal) {
      const rules = (await withRetry(() => api.get<ZeroTouchRule[] | null>('/zero-touch-rules'), signal)) ?? []
      const m = new Map<string, ZtpEx>()
      for (const r of rules) {
        const k = ztpKey(scopeOfTenant(r.tenant_id), r.vendor_id, r.device_model_id, r.oui, r.serial_pattern, r.software_version_pattern, r.match_parameter_name, r.match_parameter_value_pattern, r.trigger_event_id)
        const cur = m.get(k)
        m.set(k, cur ? { ...cur, dups: cur.dups + 1 } : { ...r, dups: 1 })
      }
      return m
    },

    diff(row, ex) {
      const ch: string[] = []
      if (row.profileId !== undefined && row.profileId !== ex.provisioning_profile_id) ch.push('profile')
      if (row.reboot !== undefined && row.reboot !== ex.post_apply_reboot) ch.push('post_apply_reboot')
      if (row.firmwareId !== undefined && row.firmwareId !== ex.firmware_file_id) ch.push('firmware')
      if (row.priority !== undefined && row.priority !== ex.priority) ch.push('priority')
      if (row.isActive !== undefined && row.isActive !== ex.is_active) ch.push('is_active')
      return ch
    },

    async create(row) {
      await api.post<ZeroTouchRule>('/zero-touch-rules', {
        ...(row.scope === 'tenant' && env.tenantId != null ? { tenant_id: env.tenantId } : {}),
        vendor_id: row.vendorId,
        device_model_id: row.modelId,
        oui: row.oui,
        serial_pattern: row.serial,
        software_version_pattern: row.swPattern,
        match_parameter_name: row.mpName,
        match_parameter_value_pattern: row.mpValue,
        provisioning_profile_id: row.profileId,
        post_apply_reboot: row.reboot ?? false,
        firmware_file_id: row.firmwareId,
        trigger_event_id: row.triggerId,
        priority: row.priority ?? 100,
        is_active: row.isActive ?? true,
      })
    },

    async update(row, ex) {
      await api.put<void>(`/zero-touch-rules/${ex.id}`, {
        tenant_id: ex.tenant_id ?? undefined,
        vendor_id: ex.vendor_id ?? undefined,
        device_model_id: ex.device_model_id ?? undefined,
        oui: ex.oui ?? undefined,
        serial_pattern: ex.serial_pattern ?? undefined,
        software_version_pattern: ex.software_version_pattern ?? undefined,
        match_parameter_name: ex.match_parameter_name ?? undefined,
        match_parameter_value_pattern: ex.match_parameter_value_pattern ?? undefined,
        provisioning_profile_id: (row.profileId !== undefined ? row.profileId : ex.provisioning_profile_id) ?? undefined,
        post_apply_reboot: row.reboot ?? ex.post_apply_reboot,
        firmware_file_id: (row.firmwareId !== undefined ? row.firmwareId : ex.firmware_file_id) ?? undefined,
        trigger_event_id: ex.trigger_event_id,
        priority: row.priority ?? ex.priority,
        is_active: row.isActive ?? ex.is_active,
      })
    },
  })

  return withHooks(base, {
    before: async (_m, ctx) => {
      const [m, p, f, t] = await Promise.all([
        loadModels(env.vendors, ctx.signal),
        listAll<ProvisioningProfile>('/provisioning-profiles', {}, { signal: ctx.signal }),
        loadFirmware(env.vendors, ctx.signal),
        api.get<RefLookup[]>('/refs/ref_ztp_trigger_event'),
      ])
      models = m
      profiles = p.rows
      firmware = f
      triggers = t ?? []
    },
    after: (items) => markAmbiguous(guardScope(items, env.isSuper, 'aturan ZTP', true), 'aturan'),
  })
}

export interface ZtpExportRow extends ZeroTouchRule {
  _vendor: string
  _model: string
  _profile: string
  _firmware: string
  _trigger: string
}

export function ztpExportColumns(): ExportColumn<ZtpExportRow>[] {
  return [
    { key: 'scope', value: (r) => scopeOfTenant(r.tenant_id) },
    { key: 'vendor', value: (r) => r._vendor },
    { key: 'model', value: (r) => r._model },
    { key: 'oui', value: (r) => r.oui },
    { key: 'serial_pattern', value: (r) => r.serial_pattern },
    { key: 'software_version_pattern', value: (r) => r.software_version_pattern },
    { key: 'match_parameter_name', value: (r) => r.match_parameter_name },
    { key: 'match_parameter_value_pattern', value: (r) => r.match_parameter_value_pattern },
    { key: 'trigger', value: (r) => r._trigger },
    { key: 'profile', value: (r) => r._profile },
    { key: 'post_apply_reboot', value: (r) => r.post_apply_reboot },
    { key: 'firmware_version', value: (r) => r._firmware },
    { key: 'priority', value: (r) => r.priority },
    { key: 'is_active', value: (r) => r.is_active },
  ]
}

export async function loadZtpExportRows(vendors: Vendor[], ctx: { signal: AbortSignal; onProgress: (done: number, total: number) => void }): Promise<ZtpExportRow[]> {
  const [rules, models, profs, fw, triggers] = await Promise.all([
    api.get<ZeroTouchRule[] | null>('/zero-touch-rules'),
    loadModels(vendors, ctx.signal),
    listAll<ProvisioningProfile>('/provisioning-profiles', {}, { signal: ctx.signal }),
    loadFirmware(vendors, ctx.signal),
    api.get<RefLookup[]>('/refs/ref_ztp_trigger_event'),
  ])
  const out = (rules ?? []).map((r) => ({
    ...r,
    _vendor: vendors.find((v) => v.id === r.vendor_id)?.code ?? '',
    _model: models.find((m) => m.id === r.device_model_id)?.model_name ?? '',
    _profile: profs.rows.find((p) => p.id === r.provisioning_profile_id)?.name ?? '',
    _firmware: fw.find((f) => f.id === r.firmware_file_id)?.version ?? '',
    _trigger: (triggers ?? []).find((t) => t.id === r.trigger_event_id)?.code ?? '',
  }))
  ctx.onProgress(out.length, out.length)
  return out
}
