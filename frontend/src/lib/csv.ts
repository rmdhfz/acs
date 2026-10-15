// Parser & penulis CSV (RFC 4180) tanpa dependensi, untuk fitur impor/ekspor.
//
// - Parser: pemisah otomatis (, ; tab — Excel regional Indonesia memakai ';'),
//   sel ber-kutip dengan koma/baris baru/kutip-ganda, BOM UTF-8, CRLF/LF.
// - Penulis: BOM UTF-8 (Excel membaca UTF-8 dengan benar) dan penetralan
//   CSV/formula injection (OWASP): sel teks berawalan = + - @ TAB CR diberi
//   apostrof di depan agar Excel/Sheets tidak mengeksekusinya sebagai formula.
//   Angka murni (mis. koordinat -6.2) TIDAK diberi apostrof agar tetap angka.
//   Parser membuang apostrof penetral itu lagi, jadi ekspor → impor tidak
//   mengubah data.

export type Delimiter = ',' | ';' | '\t'

const BOM = '﻿'

/** detectDelimiter memilih pemisah dari baris pertama (di luar tanda kutip). */
export function detectDelimiter(text: string): Delimiter {
  const counts: Record<Delimiter, number> = { ',': 0, ';': 0, '\t': 0 }
  let inQuotes = false
  for (let i = 0; i < text.length; i++) {
    const ch = text[i]
    if (ch === '"') inQuotes = !inQuotes
    else if (!inQuotes) {
      if (ch === '\n' || ch === '\r') break
      if (ch === ',' || ch === ';' || ch === '\t') counts[ch]++
    }
  }
  let best: Delimiter = ','
  for (const d of [';', '\t'] as Delimiter[]) if (counts[d] > counts[best]) best = d
  return best
}

/** parseCSV mengubah teks CSV menjadi baris-baris sel (belum dipisah header). */
export function parseCSV(input: string, delimiter?: Delimiter): string[][] {
  let text = input.charCodeAt(0) === 0xfeff ? input.slice(1) : input
  if (text.length === 0) return []
  const delim = delimiter ?? detectDelimiter(text)
  const rows: string[][] = []
  let row: string[] = []
  let cell = ''
  let inQuotes = false
  let cellWasQuoted = false

  const endCell = () => {
    row.push(cellWasQuoted ? cell : cell.trim())
    cell = ''
    cellWasQuoted = false
  }
  const endRow = () => {
    endCell()
    // baris benar-benar kosong (satu sel kosong) diabaikan
    if (!(row.length === 1 && row[0] === '')) rows.push(row)
    row = []
  }

  for (let i = 0; i < text.length; i++) {
    const ch = text[i]
    if (inQuotes) {
      if (ch === '"') {
        if (text[i + 1] === '"') {
          cell += '"'
          i++
        } else inQuotes = false
      } else cell += ch
      continue
    }
    if (ch === '"' && cell.trim() === '') {
      inQuotes = true
      cellWasQuoted = true
      cell = ''
    } else if (ch === delim) endCell()
    else if (ch === '\n') endRow()
    else if (ch === '\r') {
      if (text[i + 1] === '\n') i++
      endRow()
    } else cell += ch
  }
  if (cell !== '' || cellWasQuoted || row.length > 0) endRow()
  return rows
}

const NUMERIC = /^[+-]?\d+([.,]\d+)?$/
const FORMULA_START = /^[=+\-@\t\r]/

/** neutralize memberi apostrof pada sel teks yang bisa dibaca sebagai formula. */
export function neutralize(value: string): string {
  return FORMULA_START.test(value) && !NUMERIC.test(value) ? `'${value}` : value
}

/** unneutralize membalik neutralize() saat membaca file hasil ekspor. */
export function unneutralize(value: string): string {
  return /^'[=+\-@\t\r]/.test(value) ? value.slice(1) : value
}

function escapeCell(value: string): string {
  const v = neutralize(value)
  return /[",;\n\r\t]/.test(v) || v.startsWith(' ') || v.endsWith(' ') ? `"${v.replace(/"/g, '""')}"` : v
}

export type CellValue = string | number | boolean | null | undefined

export function cellToString(v: CellValue): string {
  if (v === null || v === undefined) return ''
  if (typeof v === 'boolean') return v ? 'true' : 'false'
  return String(v)
}

/** toCSV menulis header + baris sebagai teks CSV (BOM + CRLF, pemisah koma). */
export function toCSV(headers: string[], rows: CellValue[][]): string {
  const lines = [headers.map(escapeCell).join(',')]
  for (const r of rows) lines.push(r.map((c) => escapeCell(cellToString(c))).join(','))
  return BOM + lines.join('\r\n') + '\r\n'
}
