// Kerangka impor data massal (CSV / JSON) dengan PRATINJAU sebelum eksekusi.
//
// Alur: pilih file → baca & petakan kolom → adapter memeriksa data EKSISTING dan
// mengklasifikasi tiap baris (create / update / unchanged / skip / error)
// → operator meninjau → baru dieksekusi. Tidak ada penulisan sebelum konfirmasi.
//
// Semua nilai sel diperlakukan sebagai string. Pada file JSON, angka/boolean
// dikonversi ke string dan objek/array disimpan sebagai teks JSON (adapter yang
// butuh data bersarang mem-parse-nya sendiri) — sehingga CSV dan JSON satu jalur.

import { parseCSV, unneutralize } from './csv'

export type ImportMode = 'skip' | 'update'
export type ImportAction = 'create' | 'update' | 'unchanged' | 'skip' | 'error'
export type RawRecord = Record<string, string>

export interface ImportColumn {
  /** Kunci kanonik = nama kolom di file ekspor/template (snake_case). */
  key: string
  /** Label ramah-baca untuk tabel panduan. */
  label: string
  required?: boolean
  example?: string
  hint?: string
  /** Nama lain yang diterima di header file (dibandingkan setelah normalisasi). */
  aliases?: string[]
  /** Kolom rahasia (mis. password): disamarkan di pratinjau/laporan, tidak pernah diekspor. */
  secret?: boolean
  /**
   * Kolom informasi saja (mis. status, IP): ikut diekspor, dikenali saat impor
   * tetapi DIABAIKAN — supaya file hasil ekspor bisa diimpor ulang tanpa peringatan.
   */
  readonly?: boolean
}

export interface PlanItem<P = unknown> {
  /** Nomor baris data di file (1 = baris data pertama setelah header). */
  rowNo: number
  /** Penanda yang dikenali operator, mis. serial number. */
  label: string
  action: ImportAction
  /** Nama field yang berubah (action=update). */
  changes?: string[]
  error?: string
  /** Data terurai untuk tahap eksekusi (buram bagi wizard). */
  payload?: P
}

export interface ExecResult {
  rowNo: number
  label: string
  action: ImportAction
  ok: boolean
  error?: string
}

export interface PlanContext {
  signal: AbortSignal
}

export interface ExecContext {
  signal: AbortSignal
  onProgress: (done: number, total: number) => void
}

export interface ImportAdapter {
  /** Nama entitas tunggal untuk teks UI, mis. "perangkat". */
  entity: string
  columns: ImportColumn[]
  /** Contoh baris untuk template yang diunduh. */
  template: RawRecord[]
  /** false bila modul tidak punya aksi "perbarui" (hanya tambah baru). */
  supportsUpdate: boolean
  plan(records: RawRecord[], mode: ImportMode, ctx: PlanContext): Promise<PlanItem[]>
  /** Mengeksekusi hanya item create/update; mengembalikan satu hasil per item tersebut. */
  execute(items: PlanItem[], mode: ImportMode, ctx: ExecContext): Promise<ExecResult[]>
}

/** RowError — kesalahan validasi satu baris (pesan ditampilkan apa adanya). */
export class RowError extends Error {}

export const MAX_FILE_BYTES = 5 * 1024 * 1024
export const MAX_ROWS = 5000

export interface ParsedFile {
  format: 'csv' | 'json'
  records: RawRecord[]
  headers: string[]
  recognized: string[]
  ignored: string[]
  missingRequired: string[]
}

export function normalizeHeader(h: string): string {
  return h
    .replace(/^﻿/, '')
    .trim()
    .toLowerCase()
    .replace(/[\s\-./]+/g, '_')
    .replace(/_+/g, '_')
    .replace(/^_|_$/g, '')
}

function stringify(v: unknown): string {
  if (v === null || v === undefined) return ''
  if (typeof v === 'string') return v
  if (typeof v === 'number' || typeof v === 'boolean') return String(v)
  try {
    return JSON.stringify(v)
  } catch {
    return String(v)
  }
}

export class ImportFileError extends Error {}

/** parseImportText membaca teks CSV/JSON dan memetakan header ke kolom adapter. */
export function parseImportText(text: string, filename: string, columns: ImportColumn[]): ParsedFile {
  const isJson = /\.json$/i.test(filename) || /^\s*[[{]/.test(text)
  let rawRows: Record<string, string>[] = []
  let headers: string[] = []

  if (isJson) {
    let data: unknown
    try {
      data = JSON.parse(text)
    } catch (e) {
      throw new ImportFileError(`File JSON tidak valid: ${e instanceof Error ? e.message : 'gagal diurai'}`)
    }
    // Terima array objek, atau objek pembungkus {data:[...]} / {rows:[...]} (bentuk respons API).
    const arr = Array.isArray(data)
      ? data
      : data && typeof data === 'object'
        ? ((data as Record<string, unknown>).data ?? (data as Record<string, unknown>).rows)
        : undefined
    if (!Array.isArray(arr)) throw new ImportFileError('JSON harus berupa array objek, mis. [{"kolom":"nilai"}, …]')
    const hs: string[] = []
    for (const item of arr) {
      if (!item || typeof item !== 'object' || Array.isArray(item)) {
        throw new ImportFileError('Setiap elemen JSON harus berupa objek {kolom: nilai}')
      }
      const o: Record<string, string> = {}
      for (const [k, v] of Object.entries(item as Record<string, unknown>)) {
        if (!hs.includes(k)) hs.push(k)
        o[k] = stringify(v)
      }
      rawRows.push(o)
    }
    headers = hs
  } else {
    const table = parseCSV(text)
    if (table.length === 0) throw new ImportFileError('File kosong')
    headers = table[0]
    rawRows = table.slice(1).map((cells) => {
      const o: Record<string, string> = {}
      headers.forEach((h, i) => {
        o[h] = unneutralize(cells[i] ?? '')
      })
      return o
    })
  }

  if (rawRows.length === 0) throw new ImportFileError('File tidak memuat baris data (hanya header)')
  if (rawRows.length > MAX_ROWS) {
    throw new ImportFileError(`Terlalu banyak baris (${rawRows.length.toLocaleString('id-ID')}); maksimal ${MAX_ROWS.toLocaleString('id-ID')} per file — pecah file.`)
  }

  // Peta header file -> kunci kanonik
  const lookup = new Map<string, string>()
  for (const c of columns) {
    lookup.set(normalizeHeader(c.key), c.key)
    lookup.set(normalizeHeader(c.label), c.key)
    for (const a of c.aliases ?? []) lookup.set(normalizeHeader(a), c.key)
  }
  const mapping = new Map<string, string>() // header file -> key
  const recognized: string[] = []
  const ignored: string[] = []
  const seenKeys = new Set<string>()
  for (const h of headers) {
    const key = lookup.get(normalizeHeader(h))
    if (!key) {
      ignored.push(h)
      continue
    }
    if (seenKeys.has(key)) throw new ImportFileError(`Kolom "${key}" muncul lebih dari sekali di header`)
    seenKeys.add(key)
    mapping.set(h, key)
    recognized.push(key)
  }
  const missingRequired = columns.filter((c) => c.required && !seenKeys.has(c.key)).map((c) => c.key)

  const records: RawRecord[] = rawRows.map((o) => {
    const rec: RawRecord = {}
    for (const [h, key] of mapping) rec[key] = (o[h] ?? '').trim()
    return rec
  })
  return { format: isJson ? 'json' : 'csv', records, headers, recognized, ignored, missingRequired }
}

/** readImportFile membaca File dari <input>/drag-drop (batas ukuran, UTF-8). */
export async function readImportFile(file: File, columns: ImportColumn[]): Promise<ParsedFile> {
  if (file.size > MAX_FILE_BYTES) {
    throw new ImportFileError(`File terlalu besar (${(file.size / 1024 / 1024).toFixed(1)} MB); maksimal ${MAX_FILE_BYTES / 1024 / 1024} MB.`)
  }
  if (!/\.(csv|json|txt|tsv)$/i.test(file.name)) {
    throw new ImportFileError('Format file tidak didukung — gunakan .csv atau .json (dari Excel: Simpan sebagai "CSV UTF-8").')
  }
  return parseImportText(await file.text(), file.name, columns)
}

/** templateCSV membuat isi template (header + contoh) dari kolom adapter. */
export function templateHeaders(columns: ImportColumn[]): string[] {
  return columns.filter((c) => !c.readonly).map((c) => c.key)
}

/** maskSecret menyamarkan nilai kolom rahasia untuk pratinjau/laporan. */
export function maskSecret(v: string): string {
  return v === '' ? '' : '••••••'
}

/** Parse bilangan desimal dengan koma/titik; kosong → undefined; salah → RowError. */
export function parseNumber(label: string, v: string): number | undefined {
  if (v === '') return undefined
  const n = Number(v.replace(',', '.'))
  if (!Number.isFinite(n)) throw new RowError(`${label} "${v}" bukan angka`)
  return n
}

/** Parse boolean fleksibel (true/false/ya/tidak/1/0/aktif/nonaktif); kosong → undefined. */
export function parseBool(label: string, v: string): boolean | undefined {
  if (v === '') return undefined
  const s = v.toLowerCase()
  if (['true', '1', 'ya', 'y', 'yes', 'aktif', 'active'].includes(s)) return true
  if (['false', '0', 'tidak', 'n', 'no', 'nonaktif', 'inactive'].includes(s)) return false
  throw new RowError(`${label} "${v}" harus true/false (atau ya/tidak)`)
}
