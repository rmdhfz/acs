import { useCallback, useEffect, useMemo, useRef, useState, type DragEvent } from 'react'
import {
  AlertTriangle,
  ArrowLeft,
  CheckCircle2,
  CircleSlash,
  Download,
  FileUp,
  Info,
  Pencil,
  Plus,
  RefreshCw,
  SkipForward,
  Upload,
  X,
  XCircle,
} from 'lucide-react'
import { Modal } from './Modal'
import { Spinner } from './Spinner'
import { toCSV } from '../lib/csv'
import { downloadText, fileStamp } from '../lib/exporter'
import { errorMessage } from '../lib/clientAdapter'
import {
  ImportFileError,
  maskSecret,
  readImportFile,
  templateHeaders,
  type ExecResult,
  type ImportAction,
  type ImportAdapter,
  type ImportMode,
  type ParsedFile,
  type PlanItem,
} from '../lib/importer'

interface ImportWizardProps {
  adapter: ImportAdapter
  /** Judul modal, mis. "Impor Perangkat". */
  title: string
  onClose: () => void
  /** Dipanggil saat wizard ditutup SETELAH ada perubahan data (untuk me-refresh daftar). */
  onChanged?: () => void
}

type Step = 'pick' | 'preview' | 'running' | 'done'

const ACTION_META: Record<ImportAction, { label: string; icon: typeof Plus; badge: string; tile: string }> = {
  create: { label: 'Baru', icon: Plus, badge: 'bg-emerald-50 text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-400', tile: 'text-emerald-600 dark:text-emerald-400' },
  update: { label: 'Diperbarui', icon: Pencil, badge: 'bg-blue-50 text-blue-700 dark:bg-blue-500/10 dark:text-blue-400', tile: 'text-blue-600 dark:text-blue-400' },
  unchanged: { label: 'Tidak berubah', icon: CircleSlash, badge: 'bg-slate-100 text-slate-600 dark:bg-slate-700/40 dark:text-slate-300', tile: 'text-slate-500 dark:text-slate-400' },
  skip: { label: 'Dilewati', icon: SkipForward, badge: 'bg-amber-50 text-amber-700 dark:bg-amber-500/10 dark:text-amber-400', tile: 'text-amber-600 dark:text-amber-400' },
  error: { label: 'Error', icon: XCircle, badge: 'bg-red-50 text-red-700 dark:bg-red-500/10 dark:text-red-400', tile: 'text-red-600 dark:text-red-400' },
}
const ACTION_ORDER: ImportAction[] = ['create', 'update', 'unchanged', 'skip', 'error']
const PAGE = 50

const btnPrimary =
  'inline-flex items-center justify-center gap-2 rounded-full bg-indigo-600 px-4 py-2 text-sm font-semibold text-white transition-colors hover:bg-indigo-500 disabled:cursor-not-allowed disabled:opacity-50'
const btnGhost =
  'inline-flex items-center justify-center gap-2 rounded-full border border-slate-300 bg-white px-3.5 py-2 text-sm font-medium text-slate-700 transition-colors hover:bg-slate-50 disabled:cursor-not-allowed disabled:opacity-50 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-300 dark:hover:bg-slate-800'

function detail(item: PlanItem): string {
  switch (item.action) {
    case 'create':
      return 'Akan ditambahkan'
    case 'update':
      return `Ubah: ${(item.changes ?? []).join(', ')}`
    case 'unchanged':
      return 'Sudah ada, tidak ada perbedaan'
    case 'skip':
      return 'Sudah ada — dilewati'
    default:
      return item.error ?? 'Tidak valid'
  }
}

// ImportWizard — impor CSV/JSON dengan PRATINJAU: file dibaca, adapter memeriksa
// data eksisting dan mengklasifikasi setiap baris; tidak ada yang ditulis
// sebelum operator menekan "Impor".
export function ImportWizard({ adapter, title, onClose, onChanged }: ImportWizardProps) {
  const [step, setStep] = useState<Step>('pick')
  const [file, setFile] = useState<File | null>(null)
  const [parsed, setParsed] = useState<ParsedFile | null>(null)
  const [fileError, setFileError] = useState<string | null>(null)
  const [mode, setMode] = useState<ImportMode>('skip')
  const [planning, setPlanning] = useState(false)
  const [planError, setPlanError] = useState<string | null>(null)
  const [plans, setPlans] = useState<Partial<Record<ImportMode, PlanItem[]>>>({})
  const [filter, setFilter] = useState<ImportAction | 'all'>('all')
  const [page, setPage] = useState(0)
  const [progress, setProgress] = useState({ done: 0, total: 0 })
  const [results, setResults] = useState<ExecResult[]>([])
  const [dragOver, setDragOver] = useState(false)
  const abortRef = useRef<AbortController | null>(null)
  const changedRef = useRef(false)
  const inputRef = useRef<HTMLInputElement>(null)
  // Penanda permintaan plan terbaru agar hasil basi (mode/file diganti) diabaikan.
  const planSeq = useRef(0)

  const items = plans[mode]

  useEffect(() => () => abortRef.current?.abort(), [])

  const close = useCallback(() => {
    abortRef.current?.abort()
    if (changedRef.current) onChanged?.()
    onClose()
  }, [onChanged, onClose])

  const runPlan = useCallback(
    async (p: ParsedFile, m: ImportMode) => {
      const seq = ++planSeq.current
      setPlanning(true)
      setPlanError(null)
      const ctrl = new AbortController()
      abortRef.current = ctrl
      try {
        const out = await adapter.plan(p.records, m, { signal: ctrl.signal })
        if (seq !== planSeq.current) return
        setPlans((prev) => ({ ...prev, [m]: out }))
      } catch (e) {
        if (seq !== planSeq.current) return
        setPlanError(errorMessage(e))
      } finally {
        if (seq === planSeq.current) setPlanning(false)
      }
    },
    [adapter],
  )

  async function handleFile(f: File | undefined | null) {
    if (!f) return
    setFileError(null)
    setPlanError(null)
    setPlans({})
    setFilter('all')
    setPage(0)
    try {
      const p = await readImportFile(f, adapter.columns)
      if (p.missingRequired.length > 0) {
        setFile(f)
        setParsed(null)
        setFileError(`Kolom wajib tidak ada di file: ${p.missingRequired.join(', ')}. Unduh template untuk melihat format yang benar.`)
        return
      }
      setFile(f)
      setParsed(p)
      setStep('preview')
      void runPlan(p, mode)
    } catch (e) {
      setFile(f)
      setParsed(null)
      setFileError(e instanceof ImportFileError ? e.message : errorMessage(e))
    }
  }

  function changeMode(m: ImportMode) {
    if (m === mode || !parsed) return
    setMode(m)
    setFilter('all')
    setPage(0)
    if (!plans[m]) void runPlan(parsed, m)
  }

  function onDrop(e: DragEvent) {
    e.preventDefault()
    setDragOver(false)
    void handleFile(e.dataTransfer.files?.[0])
  }

  const counts = useMemo(() => {
    const c: Record<ImportAction, number> = { create: 0, update: 0, unchanged: 0, skip: 0, error: 0 }
    for (const it of items ?? []) c[it.action]++
    return c
  }, [items])
  const actionable = counts.create + counts.update
  const filtered = useMemo(() => (items ?? []).filter((it) => filter === 'all' || it.action === filter), [items, filter])
  const pageCount = Math.max(1, Math.ceil(filtered.length / PAGE))
  const pageItems = filtered.slice(page * PAGE, page * PAGE + PAGE)

  async function execute() {
    if (!items || actionable === 0) return
    const ctrl = new AbortController()
    abortRef.current = ctrl
    const todo = items.filter((i) => i.action === 'create' || i.action === 'update')
    setProgress({ done: 0, total: todo.length })
    setResults([])
    setStep('running')
    try {
      const res = await adapter.execute(items, mode, { signal: ctrl.signal, onProgress: (done, total) => setProgress({ done, total }) })
      setResults(res)
      if (res.some((r) => r.ok)) changedRef.current = true
    } catch (e) {
      setResults(todo.map((t) => ({ rowNo: t.rowNo, label: t.label, action: t.action, ok: false, error: errorMessage(e) })))
    }
    setStep('done')
  }

  function downloadTemplate() {
    const headers = templateHeaders(adapter.columns)
    const rows = adapter.template.map((r) => headers.map((h) => r[h] ?? ''))
    downloadText(`template-${adapter.entity.replace(/\s+/g, '-')}.csv`, toCSV(headers, rows), 'text/csv')
  }

  function secretKeys() {
    return new Set(adapter.columns.filter((c) => c.secret).map((c) => c.key))
  }

  function downloadPreviewReport() {
    if (!items) return
    const rows = items.map((it) => [it.rowNo, ACTION_META[it.action].label, it.label, detail(it)])
    downloadText(`pratinjau-impor-${fileStamp()}.csv`, toCSV(['baris', 'aksi', 'penanda', 'rincian'], rows), 'text/csv')
  }

  function downloadFailed() {
    if (!parsed) return
    const failed = new Map(results.filter((r) => !r.ok).map((r) => [r.rowNo, r.error ?? '']))
    // baris yang sejak pratinjau sudah error ikut disertakan agar bisa diperbaiki sekaligus
    for (const it of items ?? []) if (it.action === 'error') failed.set(it.rowNo, it.error ?? '')
    const keys = parsed.recognized
    const secrets = secretKeys()
    const rows: (string | number)[][] = []
    for (const [rowNo, err] of failed) {
      const rec = parsed.records[rowNo - 1]
      rows.push([...keys.map((k) => (secrets.has(k) ? maskSecret(rec?.[k] ?? '') : (rec?.[k] ?? ''))), err])
    }
    downloadText(`baris-gagal-${fileStamp()}.csv`, toCSV([...keys, 'error'], rows), 'text/csv')
  }

  const busy = planning || step === 'running'
  const okCount = results.filter((r) => r.ok).length
  const failCount = results.length - okCount
  const createdOk = results.filter((r) => r.ok && r.action === 'create').length
  const updatedOk = results.filter((r) => r.ok && r.action === 'update').length

  return (
    <Modal title={title} onClose={close} xl dismissable={!busy}>
      {/* ------------------------------------------------ 1. PILIH FILE */}
      {step === 'pick' && (
        <div className="space-y-5">
          <div
            onDragOver={(e) => {
              e.preventDefault()
              setDragOver(true)
            }}
            onDragLeave={() => setDragOver(false)}
            onDrop={onDrop}
            className={`flex flex-col items-center justify-center rounded-xl border-2 border-dashed px-6 py-10 text-center transition-colors ${
              dragOver ? 'border-indigo-500 bg-indigo-50 dark:bg-indigo-500/10' : 'border-slate-300 dark:border-slate-700'
            }`}
          >
            <div className="flex h-12 w-12 items-center justify-center rounded-full bg-indigo-50 text-indigo-600 dark:bg-indigo-500/10 dark:text-indigo-400">
              <FileUp className="h-6 w-6" />
            </div>
            <p className="mt-3 text-sm font-medium text-slate-800 dark:text-slate-200">Tarik file CSV / JSON ke sini</p>
            <p className="mt-1 text-xs text-slate-500 dark:text-slate-400">atau</p>
            <button type="button" onClick={() => inputRef.current?.click()} className={`${btnPrimary} mt-2`}>
              <Upload className="h-4 w-4" /> Pilih file
            </button>
            <input
              ref={inputRef}
              type="file"
              accept=".csv,.json,.txt,.tsv,text/csv,application/json"
              className="sr-only"
              aria-label="Pilih file impor"
              onChange={(e) => {
                void handleFile(e.target.files?.[0])
                e.target.value = ''
              }}
            />
            <p className="mt-3 text-[11px] text-slate-500 dark:text-slate-400">
              Maks. 5 MB / 5.000 baris. Dari Excel: Simpan sebagai &ldquo;CSV UTF-8&rdquo; (pemisah koma atau titik-koma otomatis dikenali).
            </p>
          </div>

          {fileError && (
            <div role="alert" className="flex items-start gap-2 rounded-lg bg-red-50 px-3.5 py-2.5 text-sm text-red-700 dark:bg-red-500/10 dark:text-red-400">
              <XCircle className="mt-0.5 h-4 w-4 shrink-0" />
              <div>
                {file && <p className="font-medium">{file.name}</p>}
                <p>{fileError}</p>
              </div>
            </div>
          )}

          <div className="rounded-lg border border-slate-200 dark:border-slate-800">
            <div className="flex flex-wrap items-center justify-between gap-2 border-b border-slate-200 px-4 py-2.5 dark:border-slate-800">
              <p className="text-sm font-medium text-slate-800 dark:text-slate-200">Format kolom</p>
              <button type="button" onClick={downloadTemplate} className="inline-flex items-center gap-1.5 text-sm font-medium text-indigo-600 hover:text-indigo-500 dark:text-indigo-400">
                <Download className="h-4 w-4" /> Unduh template CSV
              </button>
            </div>
            <div className="overflow-x-auto" tabIndex={0} role="region" aria-label="Pratinjau data impor (dapat digulir)">
              <table className="w-full text-left text-xs">
                <thead>
                  <tr className="border-b border-slate-100 uppercase tracking-wide text-slate-500 dark:border-slate-800 dark:text-slate-400">
                    <th className="px-4 py-2 font-medium">Kolom</th>
                    <th className="px-4 py-2 font-medium">Wajib</th>
                    <th className="px-4 py-2 font-medium">Contoh</th>
                    <th className="px-4 py-2 font-medium">Keterangan</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
                  {adapter.columns
                    .filter((c) => !c.readonly)
                    .map((c) => (
                    <tr key={c.key}>
                      <td className="px-4 py-2 font-mono text-slate-800 dark:text-slate-200">{c.key}</td>
                      <td className="px-4 py-2">{c.required ? <span className="font-semibold text-red-600 dark:text-red-400">ya</span> : <span className="text-slate-500 dark:text-slate-400">tidak</span>}</td>
                      <td className="px-4 py-2 font-mono text-slate-500 dark:text-slate-400">{c.secret && c.example ? maskSecret(c.example) : c.example}</td>
                      <td className="px-4 py-2 text-slate-500 dark:text-slate-400">{c.hint}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        </div>
      )}

      {/* ------------------------------------------------ 2. PRATINJAU */}
      {step === 'preview' && parsed && file && (
        <div className="space-y-4">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div className="min-w-0 text-sm">
              <p className="truncate font-medium text-slate-800 dark:text-slate-200">{file.name}</p>
              <p className="mt-0.5 text-xs text-slate-500 dark:text-slate-400">
                {parsed.format.toUpperCase()} · {parsed.records.length.toLocaleString('id-ID')} baris · kolom dikenali: {parsed.recognized.join(', ')}
                {parsed.ignored.length > 0 && <span className="text-amber-600 dark:text-amber-400"> · diabaikan: {parsed.ignored.join(', ')}</span>}
              </p>
            </div>
            <button
              type="button"
              disabled={planning}
              onClick={() => {
                setStep('pick')
                setParsed(null)
                setFile(null)
                setPlans({})
              }}
              className={btnGhost}
            >
              <ArrowLeft className="h-4 w-4" /> Ganti file
            </button>
          </div>

          {adapter.supportsUpdate && (
            <fieldset className="rounded-lg border border-slate-200 p-3 dark:border-slate-800" disabled={planning}>
              <legend className="px-1.5 text-xs font-medium uppercase tracking-wide text-slate-500 dark:text-slate-400">Jika data sudah ada</legend>
              <div className="flex flex-wrap gap-x-6 gap-y-2">
                {(
                  [
                    ['skip', 'Lewati — tambahkan yang baru saja'],
                    ['update', 'Perbarui — tambahkan baru & ubah yang berbeda'],
                  ] as [ImportMode, string][]
                ).map(([m, label]) => (
                  <label key={m} className="flex cursor-pointer items-center gap-2 text-sm text-slate-700 dark:text-slate-300">
                    <input type="radio" name="import-mode" checked={mode === m} onChange={() => changeMode(m)} className="h-4 w-4 border-slate-300 text-indigo-600 focus:ring-indigo-500" />
                    {label}
                  </label>
                ))}
              </div>
            </fieldset>
          )}

          {planning && (
            <div className="flex items-center gap-3 rounded-lg bg-slate-50 px-4 py-6 text-sm text-slate-600 dark:bg-slate-800/50 dark:text-slate-300">
              <Spinner className="h-5 w-5" /> Memeriksa data eksisting dan memvalidasi {parsed.records.length.toLocaleString('id-ID')} baris…
            </div>
          )}

          {planError && (
            <div role="alert" className="flex items-start justify-between gap-3 rounded-lg bg-red-50 px-3.5 py-2.5 text-sm text-red-700 dark:bg-red-500/10 dark:text-red-400">
              <span className="flex items-start gap-2">
                <XCircle className="mt-0.5 h-4 w-4 shrink-0" /> {planError}
              </span>
              <button type="button" onClick={() => void runPlan(parsed, mode)} className="inline-flex shrink-0 items-center gap-1 font-medium underline">
                <RefreshCw className="h-3.5 w-3.5" /> Coba lagi
              </button>
            </div>
          )}

          {!planning && items && (
            <>
              <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-5">
                {ACTION_ORDER.filter((a) => a !== 'skip' || adapter.supportsUpdate || counts.skip > 0).map((a) => {
                  const M = ACTION_META[a]
                  const active = filter === a
                  return (
                    <button
                      key={a}
                      type="button"
                      onClick={() => {
                        setFilter(active ? 'all' : a)
                        setPage(0)
                      }}
                      aria-pressed={active}
                      className={`rounded-full border px-3 py-2.5 text-left transition-colors ${
                        active ? 'border-indigo-500 bg-indigo-50 dark:bg-indigo-500/10' : 'border-slate-200 hover:bg-slate-50 dark:border-slate-800 dark:hover:bg-slate-800/50'
                      }`}
                    >
                      <p className="flex items-center gap-1.5 text-xs font-medium text-slate-500 dark:text-slate-400">
                        <M.icon className={`h-3.5 w-3.5 ${M.tile}`} /> {M.label}
                      </p>
                      <p className={`mt-0.5 text-xl font-semibold tabular-nums ${M.tile}`}>{counts[a].toLocaleString('id-ID')}</p>
                    </button>
                  )
                })}
              </div>

              {counts.error > 0 && (
                <p className="flex items-start gap-2 text-xs text-amber-700 dark:text-amber-400">
                  <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
                  Baris ber-error tidak akan diimpor. Perbaiki di file lalu unggah ulang, atau lanjutkan dengan baris yang valid.
                </p>
              )}

              <div className="overflow-hidden rounded-lg border border-slate-200 dark:border-slate-800">
                <div className="max-h-[38vh] overflow-auto">
                  <table className="w-full text-left text-sm">
                    <thead className="sticky top-0 bg-slate-50 text-xs uppercase tracking-wide text-slate-500 dark:bg-slate-800 dark:text-slate-400">
                      <tr>
                        <th className="w-16 px-3 py-2 font-medium">Baris</th>
                        <th className="w-36 px-3 py-2 font-medium">Aksi</th>
                        <th className="px-3 py-2 font-medium">Data</th>
                        <th className="px-3 py-2 font-medium">Rincian</th>
                      </tr>
                    </thead>
                    <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
                      {pageItems.map((it) => {
                        const M = ACTION_META[it.action]
                        return (
                          <tr key={it.rowNo}>
                            <td className="px-3 py-2 font-mono text-xs text-slate-500 dark:text-slate-400">{it.rowNo}</td>
                            <td className="px-3 py-2">
                              <span className={`inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium ${M.badge}`}>
                                <M.icon className="h-3 w-3" /> {M.label}
                              </span>
                            </td>
                            <td className="px-3 py-2 font-medium text-slate-800 dark:text-slate-200">{it.label}</td>
                            <td className={`px-3 py-2 text-xs ${it.action === 'error' ? 'text-red-600 dark:text-red-400' : 'text-slate-500 dark:text-slate-400'}`}>{detail(it)}</td>
                          </tr>
                        )
                      })}
                      {pageItems.length === 0 && (
                        <tr>
                          <td colSpan={4} className="px-3 py-8 text-center text-sm text-slate-500 dark:text-slate-400">
                            Tidak ada baris untuk filter ini.
                          </td>
                        </tr>
                      )}
                    </tbody>
                  </table>
                </div>
                {pageCount > 1 && (
                  <div className="flex items-center justify-between border-t border-slate-200 px-3 py-2 text-xs text-slate-500 dark:border-slate-800 dark:text-slate-400">
                    <span>
                      {filtered.length.toLocaleString('id-ID')} baris · halaman {page + 1}/{pageCount}
                    </span>
                    <span className="flex gap-1.5">
                      <button type="button" disabled={page === 0} onClick={() => setPage((p) => p - 1)} className="rounded border border-slate-300 px-2 py-0.5 disabled:opacity-40 dark:border-slate-700">
                        Sebelumnya
                      </button>
                      <button type="button" disabled={page >= pageCount - 1} onClick={() => setPage((p) => p + 1)} className="rounded border border-slate-300 px-2 py-0.5 disabled:opacity-40 dark:border-slate-700">
                        Berikutnya
                      </button>
                    </span>
                  </div>
                )}
              </div>

              <div className="flex flex-wrap items-center justify-between gap-3 pt-1">
                <button type="button" onClick={downloadPreviewReport} className="inline-flex items-center gap-1.5 text-sm font-medium text-slate-600 hover:text-slate-900 dark:text-slate-400 dark:hover:text-slate-100">
                  <Download className="h-4 w-4" /> Unduh laporan pratinjau
                </button>
                <div className="flex items-center gap-2.5">
                  <button type="button" onClick={close} className={btnGhost}>
                    Batal
                  </button>
                  <button type="button" onClick={() => void execute()} disabled={actionable === 0} className={btnPrimary}>
                    <Upload className="h-4 w-4" />
                    {actionable === 0 ? 'Tidak ada yang diimpor' : `Impor ${actionable.toLocaleString('id-ID')} baris`}
                  </button>
                </div>
              </div>
              {actionable === 0 && counts.error === 0 && (
                <p className="flex items-center gap-1.5 text-xs text-slate-500 dark:text-slate-400">
                  <Info className="h-3.5 w-3.5" /> Semua baris sudah ada dan tidak berbeda — tidak ada yang perlu dilakukan.
                </p>
              )}
            </>
          )}
        </div>
      )}

      {/* ------------------------------------------------ 3. BERJALAN */}
      {step === 'running' && (
        <div className="space-y-4 py-6" aria-live="polite">
          <div className="flex items-center gap-3 text-sm text-slate-700 dark:text-slate-300">
            <Spinner className="h-5 w-5" /> Mengimpor {progress.done.toLocaleString('id-ID')} / {progress.total.toLocaleString('id-ID')} baris…
          </div>
          <div className="h-2.5 overflow-hidden rounded-full bg-slate-100 dark:bg-slate-800" role="progressbar" aria-valuemin={0} aria-valuemax={progress.total} aria-valuenow={progress.done}>
            <div className="h-full rounded-full bg-indigo-500 transition-all" style={{ width: `${progress.total ? (progress.done / progress.total) * 100 : 0}%` }} />
          </div>
          <button type="button" onClick={() => abortRef.current?.abort()} className={btnGhost}>
            <X className="h-4 w-4" /> Hentikan
          </button>
          <p className="text-xs text-slate-500 dark:text-slate-400">Menghentikan tidak membatalkan baris yang sudah berhasil; sisanya tidak diproses.</p>
        </div>
      )}

      {/* ------------------------------------------------ 4. SELESAI */}
      {step === 'done' && (
        <div className="space-y-4">
          <div className="flex items-start gap-3">
            <div className={`flex h-11 w-11 shrink-0 items-center justify-center rounded-full ${failCount === 0 ? 'bg-emerald-50 text-emerald-600 dark:bg-emerald-500/10' : 'bg-amber-50 text-amber-600 dark:bg-amber-500/10'}`}>
              {failCount === 0 ? <CheckCircle2 className="h-6 w-6" /> : <AlertTriangle className="h-6 w-6" />}
            </div>
            <div>
              <p className="text-base font-semibold text-slate-900 dark:text-slate-100">{failCount === 0 ? 'Impor selesai' : 'Impor selesai dengan kegagalan'}</p>
              <p className="mt-0.5 text-sm text-slate-500 dark:text-slate-400">
                {okCount.toLocaleString('id-ID')} baris berhasil ({createdOk.toLocaleString('id-ID')} ditambahkan, {updatedOk.toLocaleString('id-ID')} diperbarui)
                {failCount > 0 && <>, <span className="font-medium text-red-600 dark:text-red-400">{failCount.toLocaleString('id-ID')} gagal</span></>}
                {counts.unchanged + counts.skip > 0 && <>, {(counts.unchanged + counts.skip).toLocaleString('id-ID')} dilewati/tidak berubah</>}.
              </p>
            </div>
          </div>
          {failCount > 0 && (
            <div className="overflow-hidden rounded-lg border border-red-200 dark:border-red-500/30">
              <div className="max-h-56 overflow-auto">
                <table className="w-full text-left text-sm">
                  <tbody className="divide-y divide-red-100 dark:divide-red-500/20">
                    {results
                      .filter((r) => !r.ok)
                      .slice(0, 100)
                      .map((r) => (
                        <tr key={r.rowNo}>
                          <td className="w-16 px-3 py-2 font-mono text-xs text-slate-500 dark:text-slate-400">{r.rowNo}</td>
                          <td className="px-3 py-2 font-medium text-slate-800 dark:text-slate-200">{r.label}</td>
                          <td className="px-3 py-2 text-xs text-red-600 dark:text-red-400">{r.error}</td>
                        </tr>
                      ))}
                  </tbody>
                </table>
              </div>
            </div>
          )}
          <div className="flex flex-wrap items-center justify-between gap-3">
            {failCount > 0 || counts.error > 0 ? (
              <button type="button" onClick={downloadFailed} className={btnGhost}>
                <Download className="h-4 w-4" /> Unduh baris gagal/error (CSV)
              </button>
            ) : (
              <span />
            )}
            <button type="button" onClick={close} className={btnPrimary}>
              Selesai
            </button>
          </div>
        </div>
      )}
    </Modal>
  )
}
