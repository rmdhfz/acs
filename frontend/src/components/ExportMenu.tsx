import { useEffect, useRef, useState } from 'react'
import { ChevronDown, Download, FileJson, FileSpreadsheet } from 'lucide-react'
import { buildCSV, buildJSON, downloadText, fileStamp, type ExportColumn } from '../lib/exporter'
import { useToast } from '../lib/toast'
import { errorMessage } from '../lib/clientAdapter'
import { Spinner } from './Spinner'

export interface ExportContext {
  signal: AbortSignal
  onProgress: (done: number, total: number) => void
}

interface ExportMenuProps<T> {
  /** Awalan nama file, mis. "perangkat" → perangkat-20261015-1608.csv */
  filename: string
  columns: ExportColumn<T>[]
  /** Mengambil baris yang akan diekspor (semua halaman / sesuai filter aktif). */
  getRows: (ctx: ExportContext) => Promise<T[]>
  disabled?: boolean
  /** Penjelasan singkat di bawah menu, mis. "Sesuai filter yang aktif". */
  hint?: string
  /** Teks tombol (default "Ekspor"). */
  label?: string
}

// ExportMenu — tombol "Ekspor" dengan pilihan CSV/JSON. Header/kunci file sama
// dengan kolom impor, jadi hasil ekspor dapat diedit dan diimpor kembali.
export function ExportMenu<T>({ filename, columns, getRows, disabled, hint, label = 'Ekspor' }: ExportMenuProps<T>) {
  const toast = useToast()
  const [open, setOpen] = useState(false)
  const [busy, setBusy] = useState(false)
  const [progress, setProgress] = useState<{ done: number; total: number } | null>(null)
  const ref = useRef<HTMLDivElement>(null)
  const abortRef = useRef<AbortController | null>(null)

  useEffect(() => {
    if (!open) return
    const onDown = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && setOpen(false)
    document.addEventListener('mousedown', onDown)
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('mousedown', onDown)
      document.removeEventListener('keydown', onKey)
    }
  }, [open])

  useEffect(() => () => abortRef.current?.abort(), [])

  async function run(format: 'csv' | 'json') {
    setOpen(false)
    const ctrl = new AbortController()
    abortRef.current = ctrl
    setBusy(true)
    setProgress(null)
    try {
      const rows = await getRows({ signal: ctrl.signal, onProgress: (done, total) => setProgress({ done, total }) })
      if (rows.length === 0) {
        toast.warning('Tidak ada data untuk diekspor.')
        return
      }
      const name = `${filename}-${fileStamp()}.${format}`
      if (format === 'csv') downloadText(name, buildCSV(columns, rows), 'text/csv')
      else downloadText(name, buildJSON(columns, rows), 'application/json')
      toast.success(`${rows.length.toLocaleString('id-ID')} baris diekspor ke ${name}`)
    } catch (e) {
      if (!(e instanceof DOMException && e.name === 'AbortError')) toast.error(errorMessage(e), 'Ekspor gagal')
    } finally {
      setBusy(false)
      setProgress(null)
      abortRef.current = null
    }
  }

  return (
    <div className="relative" ref={ref}>
      <button
        type="button"
        onClick={() => (busy ? abortRef.current?.abort() : setOpen((o) => !o))}
        disabled={disabled}
        aria-haspopup="menu"
        aria-expanded={open}
        title={busy ? 'Klik untuk membatalkan' : undefined}
        className="flex items-center gap-2 rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm font-medium text-slate-700 transition-colors hover:bg-slate-50 disabled:cursor-not-allowed disabled:opacity-60 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-300 dark:hover:bg-slate-800"
      >
        {busy ? <Spinner className="h-4 w-4" /> : <Download className="h-4 w-4" />}
        {busy ? (progress ? `Mengambil ${progress.done.toLocaleString('id-ID')}/${progress.total.toLocaleString('id-ID')}…` : 'Menyiapkan…') : label}
        {!busy && <ChevronDown className="h-3.5 w-3.5 text-slate-400" />}
      </button>
      {open && (
        <div role="menu" className="absolute right-0 z-30 mt-1.5 w-56 overflow-hidden rounded-lg border border-slate-200 bg-white py-1 shadow-lg dark:border-slate-700 dark:bg-slate-900">
          <button
            role="menuitem"
            onClick={() => void run('csv')}
            className="flex w-full items-center gap-2.5 px-3.5 py-2 text-left text-sm text-slate-700 hover:bg-slate-50 dark:text-slate-300 dark:hover:bg-slate-800"
          >
            <FileSpreadsheet className="h-4 w-4 text-emerald-600" /> CSV (Excel)
          </button>
          <button
            role="menuitem"
            onClick={() => void run('json')}
            className="flex w-full items-center gap-2.5 px-3.5 py-2 text-left text-sm text-slate-700 hover:bg-slate-50 dark:text-slate-300 dark:hover:bg-slate-800"
          >
            <FileJson className="h-4 w-4 text-amber-600" /> JSON
          </button>
          {hint && <p className="border-t border-slate-100 px-3.5 py-2 text-[11px] leading-snug text-slate-400 dark:border-slate-800 dark:text-slate-500">{hint}</p>}
        </div>
      )}
    </div>
  )
}
