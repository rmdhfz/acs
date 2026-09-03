import { useState, type FormEvent } from 'react'
import { FileText, Upload, Trash2, AlertCircle, Package } from 'lucide-react'
import { useFiles, useUploadFile, useDeleteFile } from '../lib/hooks'
import { Modal } from '../components/Modal'
import { EmptyState } from '../components/EmptyState'
import { PageSpinner } from '../components/Spinner'
import { useToast } from '../lib/toast'
import { formatDateTime } from '../lib/format'
import type { GenericFile } from '../lib/types'

const inputCls =
  'w-full rounded-lg border border-slate-300 px-3 py-2 text-sm text-slate-900 outline-none transition-colors focus:border-slate-500 focus:ring-1 focus:ring-slate-500 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-100'
const primaryBtnCls =
  'flex items-center justify-center gap-2 rounded-lg bg-slate-900 px-3 py-2 text-sm font-medium text-white transition-colors hover:bg-slate-800 disabled:cursor-not-allowed disabled:opacity-60 dark:bg-slate-100 dark:text-slate-900 dark:hover:bg-white'
const ghostBtnCls =
  'rounded-lg px-3 py-2 text-sm font-medium text-slate-600 transition-colors hover:bg-slate-100 dark:text-slate-400 dark:hover:bg-slate-800'

const FILE_TYPES = [
  '1 Firmware Upgrade Image',
  '2 Web Content',
  '3 Vendor Configuration File',
  '4 Tone File',
  '5 Ringer Melody File',
]

function formatBytes(bytes: number) {
  if (!bytes) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return `${parseFloat((bytes / Math.pow(k, i)).toFixed(2))} ${sizes[i]}`
}

export default function FilesPage() {
  const { data, isLoading, isError } = useFiles({ pageSize: 50 })
  const files = data?.data ?? []
  const uploadMut = useUploadFile()
  const deleteMut = useDeleteFile()
  const toast = useToast()

  const [uploadOpen, setUploadOpen] = useState(false)
  const [toDelete, setToDelete] = useState<GenericFile | null>(null)

  const handleDelete = () => {
    if (!toDelete) return
    deleteMut.mutate(toDelete.id, {
      onSuccess: () => {
        toast.success(`"${toDelete.file_name}" dihapus.`, 'Berhasil')
        setToDelete(null)
      },
      onError: (err) => toast.error(err instanceof Error ? err.message : 'Gagal menghapus file'),
    })
  }

  return (
    <div className="mx-auto max-w-6xl px-6 py-8">
      <div className="mb-4 flex items-start justify-between gap-4">
        <div>
          <h1 className="flex items-center gap-2 text-xl font-semibold text-slate-900 dark:text-slate-100">
            <Package className="h-5 w-5 text-slate-400" /> File
          </h1>
          <p className="mt-0.5 text-sm text-slate-500 dark:text-slate-400">
            Firmware image, konfigurasi vendor, dan file lain yang bisa di-push / di-pull dari CPE.
          </p>
        </div>
        <button onClick={() => setUploadOpen(true)} className={primaryBtnCls}>
          <Upload className="h-4 w-4" /> Upload File
        </button>
      </div>

      <div className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm dark:border-slate-800 dark:bg-slate-900">
        {isLoading ? (
          <PageSpinner />
        ) : isError ? (
          <div className="flex items-center gap-2 p-6 text-sm text-red-600 dark:text-red-400">
            <AlertCircle className="h-4 w-4" /> Gagal memuat daftar file.
          </div>
        ) : files.length === 0 ? (
          <EmptyState icon={FileText} title="Belum ada file" description="Upload firmware atau file konfigurasi untuk mulai." />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-left text-sm">
              <thead>
                <tr className="border-b border-slate-200 bg-slate-50 text-xs font-medium uppercase tracking-wide text-slate-500 dark:border-slate-800 dark:bg-slate-800/50 dark:text-slate-400">
                  <th className="px-5 py-3">Nama file</th>
                  <th className="px-5 py-3">Tipe</th>
                  <th className="px-5 py-3">Ukuran</th>
                  <th className="px-5 py-3">Diupload</th>
                  <th className="px-5 py-3"></th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
                {files.map((f) => (
                  <tr key={f.id}>
                    <td className="px-5 py-3.5">
                      <span className="flex items-center gap-2.5 font-medium text-slate-900 dark:text-slate-100">
                        <FileText className="h-4 w-4 shrink-0 text-slate-400" />
                        <span className="truncate">{f.file_name}</span>
                      </span>
                    </td>
                    <td className="px-5 py-3.5">
                      <span className="rounded border border-slate-200 bg-slate-50 px-2 py-0.5 text-xs text-slate-600 dark:border-slate-700 dark:bg-slate-800 dark:text-slate-300">
                        {f.file_type}
                      </span>
                    </td>
                    <td className="px-5 py-3.5 font-mono text-xs tabular-nums text-slate-600 dark:text-slate-300">
                      {formatBytes(f.file_size_bytes)}
                    </td>
                    <td className="whitespace-nowrap px-5 py-3.5 text-slate-500 dark:text-slate-400">{formatDateTime(f.created_at)}</td>
                    <td className="px-5 py-3.5 text-right">
                      <button
                        onClick={() => setToDelete(f)}
                        className="rounded p-1.5 text-slate-400 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-500/10 dark:hover:text-red-400"
                        title="Hapus file"
                      >
                        <Trash2 className="h-4 w-4" />
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {uploadOpen && (
        <UploadModal
          onClose={() => setUploadOpen(false)}
          onDone={(name) => {
            toast.success(`"${name}" berhasil diupload.`, 'Berhasil')
            setUploadOpen(false)
          }}
          uploadMut={uploadMut}
        />
      )}

      {toDelete && (
        <Modal onClose={() => setToDelete(null)} title="Hapus File">
          <div className="space-y-4">
            <div className="flex gap-3 rounded-lg border border-red-100 bg-red-50 p-4 text-red-600 dark:border-red-400/20 dark:bg-red-400/10 dark:text-red-400">
              <AlertCircle className="h-5 w-5 shrink-0" />
              <p className="text-sm">
                Hapus <strong>{toDelete.file_name}</strong>? File akan dihapus permanen dari object storage. Rollout /
                task yang masih memakainya akan gagal.
              </p>
            </div>
            <div className="flex justify-end gap-2.5">
              <button onClick={() => setToDelete(null)} className={ghostBtnCls}>Batal</button>
              <button
                onClick={handleDelete}
                disabled={deleteMut.isPending}
                className="rounded-lg bg-red-600 px-3.5 py-2 text-sm font-medium text-white transition-colors hover:bg-red-700 disabled:opacity-50"
              >
                {deleteMut.isPending ? 'Menghapus…' : 'Ya, hapus'}
              </button>
            </div>
          </div>
        </Modal>
      )}
    </div>
  )
}

function UploadModal({
  onClose,
  onDone,
  uploadMut,
}: {
  onClose: () => void
  onDone: (name: string) => void
  uploadMut: ReturnType<typeof useUploadFile>
}) {
  const toast = useToast()
  const [fileType, setFileType] = useState(FILE_TYPES[0])
  const [fileName, setFileName] = useState('')
  const [file, setFile] = useState<globalThis.File | null>(null)

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault()
    if (!file) return
    const fd = new FormData()
    fd.append('file_type', fileType)
    fd.append('file_name', fileName || file.name)
    fd.append('file', file)
    uploadMut.mutate(fd, {
      onSuccess: () => onDone(fileName || file.name),
      onError: (err) => toast.error(err instanceof Error ? err.message : 'Gagal mengunggah file'),
    })
  }

  return (
    <Modal onClose={onClose} title="Upload File">
      <form onSubmit={handleSubmit} className="space-y-4">
        <div>
          <label className="mb-1 block text-sm font-medium text-slate-600 dark:text-slate-400">Tipe file</label>
          <select value={fileType} onChange={(e) => setFileType(e.target.value)} className={inputCls}>
            {FILE_TYPES.map((t) => (
              <option key={t} value={t}>{t}</option>
            ))}
          </select>
        </div>
        <div>
          <label className="mb-1 block text-sm font-medium text-slate-600 dark:text-slate-400">Nama file (opsional)</label>
          <input
            value={fileName}
            onChange={(e) => setFileName(e.target.value)}
            placeholder="Kosongkan untuk pakai nama asli"
            className={inputCls}
          />
        </div>
        <div>
          <label className="mb-1 block text-sm font-medium text-slate-600 dark:text-slate-400">Pilih file</label>
          <input
            type="file"
            onChange={(e) => setFile(e.target.files?.[0] ?? null)}
            required
            className="w-full rounded-lg border border-slate-300 px-3 py-2 text-sm text-slate-600 file:mr-3 file:rounded-md file:border-0 file:bg-slate-900 file:px-3 file:py-1 file:text-xs file:font-semibold file:text-white hover:file:bg-slate-800 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-300 dark:file:bg-slate-100 dark:file:text-slate-900"
          />
          <p className="mt-1 text-xs text-slate-400">Maksimum 256 MiB. Checksum SHA-256 dihitung otomatis di server.</p>
        </div>
        <div className="flex justify-end gap-2.5 pt-1">
          <button type="button" onClick={onClose} className={ghostBtnCls}>Batal</button>
          <button type="submit" disabled={uploadMut.isPending || !file} className={primaryBtnCls}>
            {uploadMut.isPending ? 'Mengunggah…' : 'Upload'}
          </button>
        </div>
      </form>
    </Modal>
  )
}
