import { api } from '../lib/api'
import { clientAdapter } from '../lib/clientAdapter'
import type { ExportColumn } from '../lib/exporter'
import { RowError, parseBool, parseNumber, type ImportAdapter, type ImportColumn, type PlanItem, type RawRecord } from '../lib/importer'
import type { Preset } from '../lib/types'
import { assertComplete, canon, guardScope, isSensitiveName, listAll, markAmbiguous, norm, parseJSONCell, parseScope, toError, withHooks, type Scope } from './opsShared'

// Impor/ekspor PRESET (engine drift-heal). Kunci: cakupan + nama. Kolom
// `precondition` dan `configurations` berisi teks JSON (id vendor/model di dalam
// precondition adalah id internal — portabel dalam satu instalasi). Nilai
// `set_parameter` yang kuncinya terlihat rahasia (password/psk/secret/token…) TIDAK
// diekspor (dikosongkan); saat impor ke preset yang sudah ada, nilai kosong pada
// kunci rahasia = pertahankan nilai tersimpan. Update preset adalah full-replace
// di backend, jadi baris dimerge dengan data eksisting.

export interface PresetImportEnv {
  isSuper: boolean
}

export const presetImportColumns: ImportColumn[] = [
  { key: 'name', label: 'Nama', required: true, example: 'wifi-default-korporat', hint: 'Kunci pencocokan (bersama scope).' },
  { key: 'scope', label: 'Scope', example: 'tenant', hint: 'global (hanya SUPERADMIN) atau tenant.', aliases: ['cakupan'] },
  { key: 'weight', label: 'Bobot', example: '10', hint: 'Bilangan bulat; lebih besar = dievaluasi lebih dulu. Default 0.' },
  { key: 'precondition', label: 'Precondition', example: '{"oui":"00D0D0"}', hint: 'Teks JSON; kosong = cocok semua device.' },
  { key: 'configurations', label: 'Configurations', example: '[{"op":"set_parameter","key":"wifi.ssid","value":"Kantor"}]', hint: 'Teks JSON array op set_parameter. Nilai rahasia boleh dikosongkan saat memperbarui.' },
  { key: 'enforce', label: 'Enforce', example: 'false', hint: 'true = dijaga tiap sesi CWMP saat device menyimpang.' },
  { key: 'channel', label: 'Channel', example: '', hint: 'Opsional (maks. 64).' },
  { key: 'is_active', label: 'Aktif', example: 'true', hint: 'Hanya berlaku saat memperbarui (preset baru selalu aktif).' },
  { key: 'created_at', label: 'Dibuat', readonly: true },
]

interface Op {
  op: string
  key?: string
  value?: string
}
interface PresetRow {
  name: string
  scope: Scope
  weight?: number
  precondition?: unknown
  configurations?: Op[]
  enforce?: boolean
  channel?: string
  isActive?: boolean
}
type PresetEx = Preset & { dups: number }

const scopeOf = (p: Preset): Scope => (p.tenant_id == null ? 'global' : 'tenant')
const parseJSONSafe = (s: string): unknown => {
  try {
    return JSON.parse(s)
  } catch {
    return undefined
  }
}

function parseRow(rec: RawRecord, env: PresetImportEnv): PresetRow {
  if (rec.name === '') throw new RowError('nama wajib diisi')
  if (rec.name.length > 255) throw new RowError('nama maks. 255 karakter')
  const weight = parseNumber('bobot', rec.weight ?? '')
  if (weight !== undefined && !Number.isInteger(weight)) throw new RowError('bobot harus bilangan bulat')
  const precondition = parseJSONCell('precondition', rec.precondition)
  if (precondition !== undefined && (precondition === null || typeof precondition !== 'object' || Array.isArray(precondition))) {
    throw new RowError('precondition harus objek JSON')
  }
  const configurations = parseJSONCell('configurations', rec.configurations)
  if (configurations !== undefined) {
    if (!Array.isArray(configurations)) throw new RowError('configurations harus array JSON')
    configurations.forEach((c, i) => {
      if (!c || typeof c !== 'object') throw new RowError(`configurations[${i}] harus objek`)
      if ((c as Op).op !== 'set_parameter') throw new RowError(`configurations[${i}].op harus "set_parameter"`)
      if (!(c as Op).key) throw new RowError(`configurations[${i}] wajib punya key`)
    })
  }
  if ((rec.channel ?? '').length > 64) throw new RowError('channel maks. 64 karakter')
  return {
    name: rec.name,
    scope: parseScope(rec.scope, env.isSuper),
    weight,
    precondition,
    configurations: configurations as Op[] | undefined,
    enforce: parseBool('enforce', rec.enforce ?? ''),
    channel: rec.channel === '' ? undefined : rec.channel,
    isActive: parseBool('is_active', rec.is_active ?? ''),
  }
}

/** Isi nilai rahasia yang kosong dari data eksisting (per key). */
function restoreSecrets(ops: Op[], existing: Op[]): Op[] {
  return ops.map((o) => {
    if (o.key && isSensitiveName(o.key) && (o.value ?? '') === '') {
      const old = existing.find((e) => e.key === o.key)
      if (old?.value) return { ...o, value: old.value }
    }
    return o
  })
}

function secretMissing(ops: Op[]): string | undefined {
  const bad = ops.find((o) => o.key && isSensitiveName(o.key) && (o.value ?? '') === '')
  return bad ? `nilai untuk kunci rahasia "${bad.key}" kosong — isi nilainya (tidak diekspor demi keamanan)` : undefined
}

export function presetAdapter(env: PresetImportEnv): ImportAdapter {
  const base = clientAdapter<PresetRow, PresetEx>({
    entity: 'preset',
    columns: presetImportColumns,
    template: [
      {
        name: 'wifi-default-korporat',
        scope: env.isSuper ? 'global' : 'tenant',
        weight: '10',
        precondition: '{"oui":"00D0D0"}',
        configurations: '[{"op":"set_parameter","key":"wifi.ssid","value":"Kantor"}]',
        enforce: 'false',
        channel: '',
        is_active: 'true',
      },
    ],
    parse: (rec) => parseRow(rec, env),
    key: (r) => `${r.scope}|${norm(r.name)}`,
    label: (r) => r.name,

    async loadExisting(_rows, signal) {
      const { rows } = await listAll<Preset>('/presets', {}, { signal })
      const m = new Map<string, PresetEx>()
      for (const p of rows) {
        const k = `${scopeOf(p)}|${norm(p.name)}`
        const cur = m.get(k)
        m.set(k, cur ? { ...cur, dups: cur.dups + 1 } : { ...p, dups: 1 })
      }
      return m
    },

    diff(row, ex) {
      const ch: string[] = []
      if (row.weight !== undefined && row.weight !== ex.weight) ch.push('weight')
      if (row.precondition !== undefined && canon(row.precondition) !== canon(parseJSONSafe(ex.precondition) ?? {})) ch.push('precondition')
      if (row.configurations !== undefined) {
        const merged = restoreSecrets(row.configurations, (parseJSONSafe(ex.configurations) as Op[]) ?? [])
        if (canon(merged) !== canon(parseJSONSafe(ex.configurations) ?? [])) ch.push('configurations')
      }
      if (row.enforce !== undefined && row.enforce !== ex.enforce) ch.push('enforce')
      if (row.channel !== undefined && row.channel !== (ex.channel ?? '')) ch.push('channel')
      if (row.isActive !== undefined && row.isActive !== ex.is_active) ch.push('is_active')
      return ch
    },

    async create(row) {
      const ops = row.configurations ?? []
      const miss = secretMissing(ops)
      if (miss) throw new RowError(miss)
      await api.post<Preset>('/presets', {
        name: row.name,
        weight: row.weight ?? 0,
        precondition: JSON.stringify(row.precondition ?? {}),
        configurations: JSON.stringify(ops),
        enforce: row.enforce ?? false,
        channel: row.channel ?? null,
      })
    },

    async update(row, ex) {
      const existingOps = (parseJSONSafe(ex.configurations) as Op[]) ?? []
      const ops = row.configurations !== undefined ? restoreSecrets(row.configurations, existingOps) : existingOps
      const miss = secretMissing(ops)
      if (miss) throw new RowError(miss)
      // PATCH preset = full replace: kirim semua field, nilai tak diisi dipertahankan.
      await api.patch<Preset>(`/presets/${ex.id}`, {
        name: ex.name,
        weight: row.weight ?? ex.weight,
        precondition: row.precondition !== undefined ? JSON.stringify(row.precondition) : ex.precondition,
        configurations: JSON.stringify(ops),
        is_active: row.isActive ?? ex.is_active,
        enforce: row.enforce ?? ex.enforce,
        channel: row.channel !== undefined ? row.channel : ex.channel,
      })
    },
  })

  return withHooks(base, {
    after: (items) => markAmbiguous(guardScope(items, env.isSuper, 'preset', true), 'preset').map(checkSecrets),
  })
}

/** Sama dengan validasi saat eksekusi — agar pratinjau tidak menjanjikan baris yang pasti gagal. */
function checkSecrets(it: PlanItem): PlanItem {
  if (it.action !== 'create' && it.action !== 'update') return it
  const p = it.payload as { row: PresetRow; ex?: PresetEx } | undefined
  if (!p) return it
  let ops: Op[]
  if (it.action === 'create') ops = p.row.configurations ?? []
  else {
    const existingOps = (p.ex ? (parseJSONSafe(p.ex.configurations) as Op[] | null) : null) ?? []
    ops = p.row.configurations !== undefined ? restoreSecrets(p.row.configurations, existingOps) : existingOps
  }
  const miss = secretMissing(ops)
  return miss ? toError(it, miss) : it
}

/** Kosongkan nilai op rahasia pada teks JSON configurations untuk diekspor. */
function redactConfigurations(raw: string): string {
  const ops = parseJSONSafe(raw)
  if (!Array.isArray(ops)) return raw
  return JSON.stringify(ops.map((o: Op) => (o?.key && isSensitiveName(o.key) ? { ...o, value: '' } : o)))
}

export function presetExportColumns(): ExportColumn<Preset>[] {
  return [
    { key: 'name', value: (p) => p.name },
    { key: 'scope', value: (p) => scopeOf(p) },
    { key: 'weight', value: (p) => p.weight },
    { key: 'precondition', value: (p) => p.precondition },
    { key: 'configurations', value: (p) => redactConfigurations(p.configurations) },
    { key: 'enforce', value: (p) => p.enforce },
    { key: 'channel', value: (p) => p.channel },
    { key: 'is_active', value: (p) => p.is_active },
    { key: 'created_at', value: (p) => p.created_at },
  ]
}

export async function loadPresetExportRows(ctx: { signal: AbortSignal; onProgress: (done: number, total: number) => void }): Promise<Preset[]> {
  const { rows, truncated } = await listAll<Preset>('/presets', {}, { signal: ctx.signal, onProgress: ctx.onProgress })
  assertComplete(truncated)
  return rows
}
