import { RowError, type ExecContext, type ExecResult, type ImportAdapter, type ImportColumn, type ImportMode, type PlanItem, type RawRecord } from './importer'
import { runPool, withRetry } from './pool'

// Adapter impor "sisi klien": memanggil API create/update yang SUDAH ada per baris
// (RBAC & validasi tetap ditegakkan backend), dengan pemeriksaan data eksisting
// di tahap pratinjau supaya tidak membuang waktu menjalankan baris yang pasti
// duplikat/tidak berubah. Cocok untuk modul berjumlah sedang (ratusan–ribuan);
// modul volume besar (perangkat) memakai endpoint massal di server.

export interface ClientAdapterSpec<Row, Ex> {
  entity: string
  columns: ImportColumn[]
  template: RawRecord[]
  /** Uraikan & validasi satu baris; lempar RowError bila tidak valid. */
  parse(rec: RawRecord): Row
  /** Identitas unik baris (sudah dinormalkan, mis. huruf kecil). */
  key(row: Row): string
  /** Penanda untuk operator (ditampilkan di pratinjau/laporan). */
  label(row: Row): string
  /** Muat data eksisting yang relevan; kunci peta HARUS sama dgn key(). */
  loadExisting(rows: Row[], signal: AbortSignal): Promise<Map<string, Ex>>
  /** Nama field yang berbeda antara baris dan data eksisting; kosong = tidak berubah. */
  diff?(row: Row, ex: Ex): string[]
  create(row: Row, ctx: { signal: AbortSignal }): Promise<void>
  update?(row: Row, ex: Ex, changes: string[], ctx: { signal: AbortSignal }): Promise<void>
  concurrency?: number
}

interface Payload<Row, Ex> {
  row: Row
  ex?: Ex
  changes?: string[]
}

/** Pesan galat ramah-baca: buang awalan sentinel domain dari backend. */
export function errorMessage(err: unknown): string {
  const m = err instanceof Error ? err.message : String(err)
  return m.replace(/^(input tidak valid|data sudah ada \/ konflik unique|tidak punya akses|data tidak ditemukan): /, '')
}

export function clientAdapter<Row, Ex>(spec: ClientAdapterSpec<Row, Ex>): ImportAdapter {
  const supportsUpdate = !!(spec.diff && spec.update)

  return {
    entity: spec.entity,
    columns: spec.columns,
    template: spec.template,
    supportsUpdate,

    async plan(records, mode: ImportMode, ctx) {
      const items: PlanItem<Payload<Row, Ex>>[] = []
      const firstSeen = new Map<string, number>()
      const valid: { idx: number; row: Row; key: string }[] = []

      records.forEach((rec, i) => {
        const rowNo = i + 1
        let row: Row
        try {
          row = spec.parse(rec)
        } catch (e) {
          if (!(e instanceof RowError)) throw e
          items.push({ rowNo, label: `baris ${rowNo}`, action: 'error', error: e.message })
          return
        }
        const label = spec.label(row)
        const key = spec.key(row)
        const dupOf = firstSeen.get(key)
        if (dupOf !== undefined) {
          items.push({ rowNo, label, action: 'error', error: `duplikat dalam file (sama dengan baris ${dupOf})` })
          return
        }
        firstSeen.set(key, rowNo)
        valid.push({ idx: items.length, row, key })
        items.push({ rowNo, label, action: 'create', payload: { row } })
      })

      if (valid.length > 0) {
        const existing = await spec.loadExisting(
          valid.map((v) => v.row),
          ctx.signal,
        )
        for (const v of valid) {
          const item = items[v.idx]
          const ex = existing.get(v.key)
          if (ex === undefined) continue // tetap 'create'
          item.payload = { row: v.row, ex }
          if (!supportsUpdate) {
            item.action = 'skip'
            continue
          }
          if (mode === 'skip') {
            item.action = 'skip'
            continue
          }
          const changes = spec.diff!(v.row, ex)
          if (changes.length === 0) item.action = 'unchanged'
          else {
            item.action = 'update'
            item.changes = changes
            item.payload = { row: v.row, ex, changes }
          }
        }
      }
      items.sort((a, b) => a.rowNo - b.rowNo)
      return items
    },

    async execute(items, _mode, ctx: ExecContext) {
      const todo = items.filter((i) => i.action === 'create' || i.action === 'update')
      const results = await runPool(
        todo,
        async (item): Promise<ExecResult> => {
          const p = item.payload as Payload<Row, Ex>
          try {
            if (item.action === 'create') await withRetry(() => spec.create(p.row, { signal: ctx.signal }), ctx.signal)
            else await withRetry(() => spec.update!(p.row, p.ex as Ex, p.changes ?? [], { signal: ctx.signal }), ctx.signal)
            return { rowNo: item.rowNo, label: item.label, action: item.action, ok: true }
          } catch (e) {
            return { rowNo: item.rowNo, label: item.label, action: item.action, ok: false, error: errorMessage(e) }
          }
        },
        { concurrency: spec.concurrency ?? 4, signal: ctx.signal, onProgress: ctx.onProgress },
      )
      // item yang tak sempat jalan (dibatalkan) dilaporkan eksplisit
      return results.map((r, i) =>
        r ?? { rowNo: todo[i].rowNo, label: todo[i].label, action: todo[i].action, ok: false, error: 'dibatalkan sebelum diproses' },
      )
    },
  }
}
