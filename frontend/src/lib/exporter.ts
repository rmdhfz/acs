import { api } from './api'
import { toCSV, type CellValue } from './csv'
import type { ListResponse } from './types'

// Ekspor data ke CSV/JSON. Header CSV & kunci JSON = kunci kolom IMPOR, sehingga
// file hasil ekspor bisa diedit lalu diimpor kembali (round trip).

export interface ExportColumn<T> {
  /** Nama kolom di file (sama dengan ImportColumn.key bila modul punya impor). */
  key: string
  value: (row: T) => CellValue
}

export function buildCSV<T>(columns: ExportColumn<T>[], rows: T[]): string {
  return toCSV(
    columns.map((c) => c.key),
    rows.map((r) => columns.map((c) => c.value(r))),
  )
}

export function buildJSON<T>(columns: ExportColumn<T>[], rows: T[]): string {
  const out = rows.map((r) => {
    const o: Record<string, CellValue> = {}
    for (const c of columns) {
      const v = c.value(r)
      o[c.key] = v === undefined ? null : v
    }
    return o
  })
  return JSON.stringify(out, null, 2) + '\n'
}

export function downloadText(filename: string, text: string, mime: string) {
  const blob = new Blob([text], { type: `${mime};charset=utf-8` })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  a.rel = 'noopener'
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  // beri browser waktu memulai unduhan sebelum URL dicabut
  setTimeout(() => URL.revokeObjectURL(url), 1500)
}

/** Cap waktu untuk nama file: 20261015-1608. */
export function fileStamp(d = new Date()): string {
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}${p(d.getMonth() + 1)}${p(d.getDate())}-${p(d.getHours())}${p(d.getMinutes())}`
}

export const EXPORT_MAX_ROWS = 50_000

function query(params: Record<string, string | number | boolean | string[] | undefined>): string {
  const usp = new URLSearchParams()
  for (const [k, v] of Object.entries(params)) {
    if (Array.isArray(v)) for (const x of v) usp.append(k, x) // parameter berulang (mis. param= pada /devices)
    else if (v !== undefined && v !== '') usp.set(k, String(v))
  }
  return usp.toString()
}

/**
 * fetchAllPages mengambil SEMUA halaman sebuah endpoint list (page_size 200,
 * batas server) dengan progres, pembatalan, dan batas EXPORT_MAX_ROWS.
 */
export async function fetchAllPages<T>(
  path: string,
  params: Record<string, string | number | boolean | string[] | undefined> = {},
  opts: { signal?: AbortSignal; onProgress?: (done: number, total: number) => void; maxRows?: number } = {},
): Promise<{ rows: T[]; total: number; truncated: boolean }> {
  const pageSize = 200
  const max = opts.maxRows ?? EXPORT_MAX_ROWS
  const rows: T[] = []
  let total = 0
  for (let page = 1; ; page++) {
    if (opts.signal?.aborted) throw new DOMException('dibatalkan', 'AbortError')
    const qs = query({ ...params, page, page_size: pageSize })
    const resp = await api.get<ListResponse<T>>(`${path}?${qs}`)
    const data = resp.data ?? []
    total = resp.total ?? total
    rows.push(...data)
    opts.onProgress?.(Math.min(rows.length, total), total)
    if (data.length < pageSize || rows.length >= total) break
    if (rows.length >= max) return { rows: rows.slice(0, max), total, truncated: true }
  }
  return { rows, total, truncated: false }
}
