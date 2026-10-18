import { useEffect, useRef, useState, type ClipboardEvent, type FormEvent } from 'react'
import { useLocation } from 'react-router-dom'
import { Camera, ImagePlus, X } from 'lucide-react'
import { Modal } from './Modal'
import { ApiError } from '../lib/api'
import { useCreateBugReport, type BugSeverity } from '../lib/hooks'
import { captureScreen, prepareScreenshot, ScreenshotError } from '../lib/screenshot'
import { useToast } from '../lib/toast'

const field =
  'block w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 outline-none focus:border-indigo-500 focus:ring-2 focus:ring-indigo-500/20 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-100'
const label = 'mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400'
const ghostBtn =
  'inline-flex items-center gap-1.5 rounded-full border border-slate-300 px-3 py-1.5 text-sm font-medium text-slate-700 hover:bg-slate-50 disabled:opacity-50 dark:border-slate-700 dark:text-slate-300 dark:hover:bg-slate-800'

export const BUG_LIMITS = { titleMin: 5, titleMax: 120, descMin: 10, descMax: 4000, expMin: 5, expMax: 2000 } as const

const SEVERITIES: { value: BugSeverity; label: string }[] = [
  { value: 'MINOR', label: 'Ringan — tampilan/kenyamanan' },
  { value: 'MAJOR', label: 'Sedang — fitur tidak berjalan benar' },
  { value: 'BLOCKER', label: 'Berat — tidak bisa bekerja' },
]

// Laporan bug dari navbar: tersimpan di database dan dikirim lewat email ke operator platform. "Apa yang terjadi" dan
// "apa yang diharapkan" adalah dua isian terpisah; tangkapan layar (opsional) menunjukkan bagian mana yang bermasalah.
// Konteks (halaman, browser, versi) diisi otomatis; token/password TIDAK pernah ikut terkirim.
export function BugReportModal({ onClose }: { onClose: () => void }) {
  const location = useLocation()
  const toast = useToast()
  const create = useCreateBugReport()
  const [title, setTitle] = useState('')
  const [description, setDescription] = useState('')
  const [expected, setExpected] = useState('')
  const [severity, setSeverity] = useState<BugSeverity>('MINOR')
  const [shot, setShot] = useState<File | null>(null)
  const [preview, setPreview] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [hidden, setHidden] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const fileRef = useRef<HTMLInputElement>(null)

  // object URL pratinjau dilepas saat gambar diganti/dialog ditutup
  useEffect(() => {
    if (!shot) {
      setPreview(null)
      return
    }
    const url = URL.createObjectURL(shot)
    setPreview(url)
    return () => URL.revokeObjectURL(url)
  }, [shot])

  async function attach(src: Blob | HTMLVideoElement) {
    setError(null)
    setBusy(true)
    try {
      setShot(await prepareScreenshot(src))
    } catch (e) {
      setError(e instanceof ScreenshotError ? e.message : 'Gagal memproses gambar.')
    } finally {
      setBusy(false)
    }
  }

  async function capture() {
    setError(null)
    setBusy(true)
    try {
      setShot(await captureScreen(setHidden))
    } catch (e) {
      setError(e instanceof ScreenshotError ? e.message : 'Gagal mengambil tangkapan layar.')
    } finally {
      setBusy(false)
    }
  }

  function onPaste(e: ClipboardEvent) {
    const item = Array.from(e.clipboardData?.items ?? []).find((i) => i.kind === 'file' && i.type.startsWith('image/'))
    const f = item?.getAsFile()
    if (f) {
      e.preventDefault()
      void attach(f)
    }
  }

  function submit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    const t = title.trim()
    const d = description.trim()
    const x = expected.trim()
    if (t.length < BUG_LIMITS.titleMin) return setError(`Judul minimal ${BUG_LIMITS.titleMin} karakter.`)
    if (d.length < BUG_LIMITS.descMin) return setError(`Jelaskan apa yang terjadi, minimal ${BUG_LIMITS.descMin} karakter.`)
    if (x.length < BUG_LIMITS.expMin) return setError(`Jelaskan apa yang Anda harapkan, minimal ${BUG_LIMITS.expMin} karakter.`)
    create.mutate(
      { title: t, description: d, expected: x, severity, screenshot: shot, page_path: location.pathname, user_agent: navigator.userAgent, app_version: __APP_VERSION__ },
      {
        onSuccess: (r) => {
          toast.success(`Laporan terkirim (ID ${r.id.slice(0, 8)}). Anda akan diberi tahu saat bug ditangani.`, 'Laporan bug')
          onClose()
        },
        onError: (err) => setError(err instanceof ApiError ? err.message : 'Gagal mengirim laporan. Coba lagi.'),
      },
    )
  }

  const pending = create.isPending || busy
  return (
    <div onPaste={onPaste}>
      <Modal title="Laporkan bug" onClose={onClose} wide dismissable={!pending} hidden={hidden}>
        <form onSubmit={submit} className="space-y-3">
          <p className="text-sm text-slate-600 dark:text-slate-400">
            Laporan tersimpan dan dikirim ke tim pengembang lewat email; Anda diberi tahu saat bug sudah ditangani. Halaman saat ini ({location.pathname}), jenis
            browser, dan versi aplikasi ikut dilampirkan otomatis.
          </p>
          <div>
            <label htmlFor="bug-title" className={label}>Judul singkat</label>
            <input
              id="bug-title"
              required
              minLength={BUG_LIMITS.titleMin}
              maxLength={BUG_LIMITS.titleMax}
              pattern=".*\S.*"
              title="Tidak boleh kosong atau hanya spasi"
              autoComplete="off"
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              placeholder="mis. Tombol simpan di halaman Tag tidak merespons"
              className={field}
            />
          </div>
          <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
            <div>
              <label htmlFor="bug-desc" className={label}>Apa yang terjadi?</label>
              <textarea
                id="bug-desc"
                required
                rows={5}
                minLength={BUG_LIMITS.descMin}
                maxLength={BUG_LIMITS.descMax}
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                placeholder={'Langkah-langkah:\n1. …\n2. …\nHasil yang Anda lihat: …'}
                className={field}
              />
              <p className="mt-1 text-right text-xs text-slate-500 dark:text-slate-400">{description.length}/{BUG_LIMITS.descMax}</p>
            </div>
            <div>
              <label htmlFor="bug-exp" className={label}>Apa yang Anda harapkan?</label>
              <textarea
                id="bug-exp"
                required
                rows={5}
                minLength={BUG_LIMITS.expMin}
                maxLength={BUG_LIMITS.expMax}
                value={expected}
                onChange={(e) => setExpected(e.target.value)}
                placeholder="Seharusnya yang terjadi: …"
                className={field}
              />
              <p className="mt-1 text-right text-xs text-slate-500 dark:text-slate-400">{expected.length}/{BUG_LIMITS.expMax}</p>
            </div>
          </div>

          <div>
            <span className={label}>Tangkapan layar (opsional, membantu kami menemukan bagian yang bermasalah)</span>
            <div className="flex flex-wrap items-center gap-2">
              <button type="button" onClick={() => void capture()} disabled={pending} className={ghostBtn}>
                <Camera className="h-4 w-4" aria-hidden="true" /> Ambil tangkapan layar
              </button>
              <button type="button" onClick={() => fileRef.current?.click()} disabled={pending} className={ghostBtn}>
                <ImagePlus className="h-4 w-4" aria-hidden="true" /> Pilih gambar
              </button>
              <span className="text-xs text-slate-500 dark:text-slate-400">atau tempel gambar dengan Ctrl+V</span>
              <input
                ref={fileRef}
                type="file"
                accept="image/png,image/jpeg,image/webp"
                aria-label="Pilih berkas gambar tangkapan layar"
                className="sr-only"
                tabIndex={-1}
                onChange={(e) => {
                  const f = e.target.files?.[0]
                  e.target.value = ''
                  if (f) void attach(f)
                }}
              />
            </div>
            {shot && preview && (
              <div className="mt-2 flex items-start gap-3 rounded-2xl border border-slate-200 bg-slate-50 p-2 dark:border-slate-700 dark:bg-slate-800/50">
                <img src={preview} alt="Pratinjau tangkapan layar" className="max-h-40 rounded-lg border border-slate-200 object-contain dark:border-slate-700" />
                <div className="min-w-0 flex-1 text-xs text-slate-600 dark:text-slate-400">
                  {shot.name} · {(shot.size / 1024).toFixed(0)} KB
                </div>
                <button type="button" onClick={() => setShot(null)} aria-label="Hapus tangkapan layar" className="rounded-full p-1.5 text-slate-600 hover:bg-slate-200 dark:text-slate-300 dark:hover:bg-slate-700">
                  <X className="h-4 w-4" />
                </button>
              </div>
            )}
          </div>

          <div>
            <label htmlFor="bug-sev" className={label}>Dampak</label>
            <select id="bug-sev" value={severity} onChange={(e) => setSeverity(e.target.value as BugSeverity)} className={field}>
              {SEVERITIES.map((s) => (
                <option key={s.value} value={s.value}>
                  {s.label}
                </option>
              ))}
            </select>
          </div>
          {error && (
            <p role="alert" className="text-sm text-red-600 dark:text-red-400">
              {error}
            </p>
          )}
          <div className="flex justify-end gap-2 pt-1">
            <button type="button" onClick={onClose} disabled={pending} className="rounded-full border border-slate-300 px-3 py-2 text-sm text-slate-700 hover:bg-slate-50 dark:border-slate-700 dark:text-slate-300 dark:hover:bg-slate-800">
              Batal
            </button>
            <button type="submit" disabled={pending} className="rounded-full bg-indigo-600 px-4 py-2 text-sm font-semibold text-white hover:bg-indigo-500 disabled:opacity-50">
              {create.isPending ? 'Mengirim…' : 'Kirim laporan'}
            </button>
          </div>
        </form>
      </Modal>
    </div>
  )
}
