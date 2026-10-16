import { api } from '../lib/api'
import type { ExportColumn } from '../lib/exporter'
import { withRetry } from '../lib/pool'
import type { FirmwareFile, GenericFile, ListResponse, RefLookup, Task, Vendor } from '../lib/types'
import { assertComplete, listAll } from './opsShared'

// Ekspor-saja untuk modul yang tidak punya impor: Task, Firmware (metadata, bukan
// berkas biner), dan File (metadata). Kolom informatif; tidak ada rahasia.

type Ctx = { signal: AbortSignal; onProgress: (done: number, total: number) => void }

// ---------------------------------------------------------------- Task

export interface TaskExportRow extends Task {
  _type: string
  _status: string
  _serial: string
}

export function taskExportColumns(): ExportColumn<TaskExportRow>[] {
  return [
    { key: 'id', value: (t) => t.id },
    { key: 'task_uuid', value: (t) => t.task_uuid },
    { key: 'device_id', value: (t) => t.device_id },
    { key: 'device_serial', value: (t) => t._serial },
    { key: 'task_type', value: (t) => t._type },
    { key: 'status', value: (t) => t._status },
    { key: 'priority', value: (t) => t.priority },
    { key: 'retry_count', value: (t) => t.retry_count },
    { key: 'max_retries', value: (t) => t.max_retries },
    { key: 'error_message', value: (t) => t.error_message },
    { key: 'scheduled_at', value: (t) => t.scheduled_at },
    { key: 'sent_at', value: (t) => t.sent_at },
    { key: 'completed_at', value: (t) => t.completed_at },
    { key: 'created_at', value: (t) => t.created_at },
  ]
}

export async function loadTaskExportRows(
  filters: { device_id?: number; task_status_id?: number; task_type?: string },
  refs: { statuses: RefLookup[]; types: RefLookup[] },
  ctx: Ctx,
): Promise<TaskExportRow[]> {
  const { rows, truncated } = await listAll<Task>('/tasks', filters, { signal: ctx.signal, onProgress: ctx.onProgress })
  assertComplete(truncated)
  // Serial perangkat: satu query per device unik (batas wajar; hasil di-cache).
  const serials = new Map<number, string>()
  for (const id of new Set(rows.map((r) => r.device_id))) {
    if (ctx.signal.aborted) break
    try {
      const d = await withRetry(() => api.get<{ serial_number: string }>(`/devices/${id}`), ctx.signal)
      serials.set(id, d.serial_number)
    } catch {
      serials.set(id, '') // perangkat sudah dihapus / tak dapat diakses
    }
  }
  return rows.map((t) => ({
    ...t,
    _type: refs.types.find((r) => r.id === t.task_type_id)?.code ?? '',
    _status: refs.statuses.find((r) => r.id === t.task_status_id)?.code ?? '',
    _serial: serials.get(t.device_id) ?? '',
  }))
}

// ---------------------------------------------------------------- Firmware

export interface FirmwareExportRow extends FirmwareFile {
  _vendor: string
}

export function firmwareExportColumns(): ExportColumn<FirmwareExportRow>[] {
  return [
    { key: 'vendor', value: (f) => f._vendor },
    { key: 'version', value: (f) => f.version },
    { key: 'file_name', value: (f) => f.file_name },
    { key: 'file_size_bytes', value: (f) => f.file_size_bytes },
    { key: 'checksum_sha256', value: (f) => f.checksum_sha256 },
    { key: 'release_notes', value: (f) => f.release_notes },
    { key: 'is_active', value: (f) => f.is_active },
    { key: 'device_model_id', value: (f) => f.device_model_id },
    { key: 'firmware_uuid', value: (f) => f.firmware_uuid },
    { key: 'created_at', value: (f) => f.created_at },
  ]
}

export async function loadFirmwareExportRows(vendors: Vendor[], ctx: Ctx): Promise<FirmwareExportRow[]> {
  const out: FirmwareExportRow[] = []
  let i = 0
  for (const v of vendors) {
    if (ctx.signal.aborted) throw new DOMException('dibatalkan', 'AbortError')
    // endpoint ini per vendor (vendor_id wajib)
    const resp = await withRetry(() => api.get<ListResponse<FirmwareFile> | null>(`/firmware?vendor_id=${v.id}&page_size=200`), ctx.signal)
    for (const f of resp?.data ?? []) out.push({ ...f, _vendor: v.code })
    ctx.onProgress(++i, vendors.length)
  }
  return out
}

// ---------------------------------------------------------------- File

export interface FileExportRow extends GenericFile {
  _vendor: string
}

export function fileExportColumns(): ExportColumn<FileExportRow>[] {
  return [
    { key: 'file_name', value: (f) => f.file_name },
    { key: 'file_type', value: (f) => f.file_type },
    { key: 'vendor', value: (f) => f._vendor },
    { key: 'version', value: (f) => f.version },
    { key: 'file_size_bytes', value: (f) => f.file_size_bytes },
    { key: 'file_uuid', value: (f) => f.file_uuid },
    { key: 'created_at', value: (f) => f.created_at },
  ]
}

export async function loadFileExportRows(vendors: Vendor[], ctx: Ctx): Promise<FileExportRow[]> {
  const { rows, truncated } = await listAll<GenericFile>('/files', {}, { signal: ctx.signal, onProgress: ctx.onProgress })
  assertComplete(truncated)
  return rows.map((f) => ({ ...f, _vendor: vendors.find((v) => v.id === f.vendor_id)?.code ?? '' }))
}
