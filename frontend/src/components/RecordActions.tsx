import type { ReactNode } from 'react'
import { Eye, Pencil, Trash2 } from 'lucide-react'
import { Modal } from './Modal'
import { PageSpinner } from './Spinner'

// Komponen kecil yang dipakai berulang di tabel CRUD: tombol aksi baris dan
// modal detail baca-saja. Disatukan di sini supaya gaya (dan label a11y) sama
// di semua halaman.

const baseBtn = 'flex items-center gap-1 whitespace-nowrap text-xs font-medium transition-colors disabled:cursor-not-allowed disabled:opacity-50'
const neutralBtn = `${baseBtn} text-slate-600 hover:text-slate-900 dark:text-slate-400 dark:hover:text-slate-100`
const dangerBtn = `${baseBtn} text-red-600 hover:text-red-800 dark:text-red-400 dark:hover:text-red-300`

interface ActionProps {
  onClick: () => void
  title?: string
  disabled?: boolean
}

export function EditButton({ onClick, title = 'Edit', disabled }: ActionProps) {
  return (
    <button type="button" onClick={onClick} title={title} aria-label={title} disabled={disabled} className={neutralBtn}>
      <Pencil className="h-3.5 w-3.5" /> Edit
    </button>
  )
}

export function DetailButton({ onClick, title = 'Lihat detail', disabled }: ActionProps) {
  return (
    <button type="button" onClick={onClick} title={title} aria-label={title} disabled={disabled} className={neutralBtn}>
      <Eye className="h-3.5 w-3.5" /> Detail
    </button>
  )
}

export function DeleteButton({ onClick, title = 'Hapus', disabled }: ActionProps) {
  return (
    <button type="button" onClick={onClick} title={title} aria-label={title} disabled={disabled} className={dangerBtn}>
      <Trash2 className="h-3.5 w-3.5" /> Hapus
    </button>
  )
}

/** Pasangan label/nilai untuk DetailGrid. Nilai null/undefined/'' tampil sebagai '-'. */
export type DetailRow = [label: string, value: ReactNode]

export function DetailGrid({ rows }: { rows: DetailRow[] }) {
  return (
    <dl className="grid grid-cols-[9rem_1fr] gap-x-4 gap-y-2.5 text-sm">
      {rows.map(([label, value]) => (
        <div key={label} className="contents">
          <dt className="text-xs font-medium uppercase tracking-wide text-slate-500 dark:text-slate-400">{label}</dt>
          <dd className="break-words text-slate-900 dark:text-slate-100">
            {value === null || value === undefined || value === '' ? <span className="text-slate-500 dark:text-slate-400">-</span> : value}
          </dd>
        </div>
      ))}
    </dl>
  )
}

interface DetailModalProps {
  title: string
  onClose: () => void
  isLoading: boolean
  /** Pesan galat dari query (mis. 403/404); bila ada, ditampilkan menggantikan isi. */
  error?: string | null
  rows: DetailRow[]
  wide?: boolean
}

/** Modal detail baca-saja: memuat satu record lewat GET by id, dengan state loading & galat. */
export function DetailModal({ title, onClose, isLoading, error, rows, wide }: DetailModalProps) {
  return (
    <Modal title={title} onClose={onClose} wide={wide}>
      {isLoading ? (
        <PageSpinner />
      ) : error ? (
        <p role="alert" className="text-sm text-red-600 dark:text-red-400">
          {error}
        </p>
      ) : (
        <DetailGrid rows={rows} />
      )}
    </Modal>
  )
}
