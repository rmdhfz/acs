import { useI18n } from '../lib/i18n'
import { useState, type FormEvent } from 'react'
import { Tag as TagIcon, Plus, AlertCircle } from 'lucide-react'
import { EditButton } from '../components/RecordActions'
import { EmptyState } from '../components/EmptyState'
import { PageSpinner } from '../components/Spinner'
import { Modal } from '../components/Modal'
import { useAuth } from '../lib/auth'
import { ApiError } from '../lib/api'
import { useToast } from '../lib/toast'
import { useTags, useCreateTag, useDeleteTag, useUpdateTag } from '../lib/hooks'
import { DataTools } from '../components/DataTools'
import { loadTagExportRows, tagAdapter, tagExportColumns } from '../imports/tags'
import { formatDateTime } from '../lib/format'
import { LIMITS } from '../lib/limits'
import type { Tag } from '../lib/types'

const inputCls =
  'w-full rounded-lg border border-slate-300 px-3 py-2 text-sm text-slate-900 outline-none transition-colors focus:border-indigo-600 focus:ring-1 focus:ring-indigo-600 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-100'
const primaryBtnCls =
  'flex items-center justify-center gap-2 rounded-full bg-indigo-600 px-3 py-2 text-sm font-medium text-white transition-colors hover:bg-indigo-700 disabled:cursor-not-allowed disabled:opacity-60 dark:bg-indigo-300 dark:text-slate-900 dark:hover:bg-indigo-200'

export default function TagsPage() {
  const { t: tl } = useI18n()
  const { hasRole } = useAuth()
  const canManage = hasRole('ADMIN')
  const isSuperadmin = hasRole('SUPERADMIN')
  // Tag GLOBAL (tanpa tenant) dipakai semua tenant: hanya superadmin yang boleh
  // mengubah/menghapusnya (backend menolak ADMIN tenant dengan 403).
  const canMutate = (t: Tag) => canManage && (t.tenant_id != null || isSuperadmin)
  const { data, isLoading } = useTags({ pageSize: 50 })
  const tags = data?.data ?? []

  const [showCreate, setShowCreate] = useState(false)
  const [tagToDelete, setTagToDelete] = useState<Tag | null>(null)
  const [tagToEdit, setTagToEdit] = useState<Tag | null>(null)
  const deleteMutation = useDeleteTag()
  const toast = useToast()

  const handleDelete = async () => {
    if (!tagToDelete) return
    try {
      await deleteMutation.mutateAsync(tagToDelete.id)
      toast.success(`Tag "${tagToDelete.name}" dihapus.`)
      setTagToDelete(null)
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : 'Gagal menghapus tag')
    }
  }

  return (
    <div className="mx-auto max-w-7xl px-6 py-8">
      <div className="mb-6 flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold text-slate-900 dark:text-slate-100">{tl('nav.tags')}</h1>
          <p className="mt-0.5 text-sm text-slate-500 dark:text-slate-400">
            Kelola label dinamis untuk mengelompokkan perangkat CPE.
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <DataTools
            exportCfg={{ filename: 'tag', columns: tagExportColumns(), getRows: (c) => loadTagExportRows(c) }}
            importCfg={canManage ? { title: 'Impor Tag', makeAdapter: () => tagAdapter({ isSuper: isSuperadmin }), invalidate: ['tags'] } : undefined}
          />
          {canManage && (
            <button onClick={() => setShowCreate(true)} className={primaryBtnCls}>
              <Plus className="h-4 w-4" /> Tag Baru
            </button>
          )}
        </div>
      </div>

      <div className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm dark:border-slate-800 dark:bg-slate-900">
        {isLoading ? (
          <PageSpinner />
        ) : tags.length === 0 ? (
          <EmptyState icon={TagIcon} title="Belum ada tag" description="Buat tag pertama Anda untuk mulai mengelompokkan perangkat." />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-left text-sm">
              <thead>
                <tr className="border-b border-slate-200 bg-slate-50 text-xs font-medium uppercase tracking-wide text-slate-500 dark:border-slate-800 dark:bg-slate-800/50 dark:text-slate-400">
                  <th className="px-5 py-3">ID</th>
                  <th className="px-5 py-3">Nama Tag</th>
                  <th className="px-5 py-3">Warna</th>
                  <th className="px-5 py-3">Dibuat Pada</th>
                  <th className="px-5 py-3"></th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
                {tags.map((t) => (
                  <tr key={t.id}>
                    <td className="px-5 py-3.5 font-medium text-slate-900 dark:text-slate-100">#{t.id}</td>
                    <td className="px-5 py-3.5 font-medium text-slate-900 dark:text-slate-100">
                      {t.name}
                      {t.tenant_id == null && (
                        <span className="ml-2 rounded bg-slate-100 px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide text-slate-500 dark:bg-slate-800 dark:text-slate-400" title="Tag global — dipakai semua tenant">
                          Global
                        </span>
                      )}
                    </td>
                    <td className="px-5 py-3.5">
                      {t.color ? (
                        <div className="flex items-center gap-2">
                          <span className="h-3 w-3 rounded-full" style={{ backgroundColor: t.color }}></span>
                          <span className="text-slate-600 dark:text-slate-400 font-mono text-xs">{t.color}</span>
                        </div>
                      ) : (
                        '-'
                      )}
                    </td>
                    <td className="px-5 py-3.5 text-slate-500 dark:text-slate-400">{formatDateTime(t.created_at)}</td>
                    <td className="px-5 py-3.5 text-right">
                      {canMutate(t) && (
                        <div className="flex items-center justify-end gap-3">
                          <EditButton onClick={() => setTagToEdit(t)} title={`Edit tag ${t.name}`} />
                          <button
                            onClick={() => setTagToDelete(t)}
                            className="text-red-600 hover:text-red-700 dark:text-red-400 dark:hover:text-red-300 font-medium text-xs"
                          >
                            Hapus
                          </button>
                        </div>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {showCreate && <CreateTagModal onClose={() => setShowCreate(false)} />}
      {tagToEdit && <EditTagModal tag={tagToEdit} onClose={() => setTagToEdit(null)} />}
      
      {tagToDelete && (
        <Modal onClose={() => setTagToDelete(null)} title="Hapus Tag">
          <div className="space-y-4">
            <div className="flex gap-3 text-red-600 p-4 bg-red-50 rounded-lg border border-red-100 dark:bg-red-400/10 dark:border-red-400/20 dark:text-red-400">
              <AlertCircle className="w-5 h-5 shrink-0" />
              <p className="text-sm">
                Apakah Anda yakin ingin menghapus tag <strong>{tagToDelete.name}</strong>? 
                Tag ini juga akan dihapus dari semua perangkat yang memilikinya.
              </p>
            </div>
            <div className="flex justify-end gap-3 pt-2">
              <button
                onClick={() => setTagToDelete(null)}
                className="px-4 py-2 text-sm font-medium text-slate-700 hover:bg-slate-100 rounded-full transition-colors dark:text-slate-300 dark:hover:bg-slate-800"
              >
                Batal
              </button>
              <button
                onClick={handleDelete}
                disabled={deleteMutation.isPending}
                className="px-4 py-2 text-sm font-medium text-white bg-red-600 hover:bg-red-700 rounded-full transition-colors disabled:opacity-50"
              >
                {deleteMutation.isPending ? 'Menghapus...' : 'Ya, Hapus'}
              </button>
            </div>
          </div>
        </Modal>
      )}
    </div>
  )
}

function EditTagModal({ tag, onClose }: { tag: Tag; onClose: () => void }) {
  const [name, setName] = useState(tag.name)
  const [color, setColor] = useState(tag.color ?? '')
  const [error, setError] = useState<string | null>(null)
  const mutation = useUpdateTag()
  const toast = useToast()

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    if (!name.trim()) {
      setError('Nama tag wajib diisi.')
      return
    }
    try {
      // color '' = kosongkan warna (backend: string kosong menghapus).
      await mutation.mutateAsync({ id: tag.id, input: { name, color } })
      toast.success(`Tag "${name}" diperbarui.`)
      onClose()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Gagal memperbarui tag')
    }
  }

  return (
    <Modal title={`Edit Tag — ${tag.name}`} onClose={onClose}>
      <form onSubmit={handleSubmit} className="space-y-4">
        <div>
          <label className="mb-1 block text-sm font-medium text-slate-600 dark:text-slate-400">Nama Tag</label>
          <input required maxLength={LIMITS.tag.name} value={name} onChange={(e) => setName(e.target.value)} className={inputCls} />
        </div>
        <div>
          <label className="mb-1 block text-sm font-medium text-slate-600 dark:text-slate-400">Warna (kosongkan untuk menghapus)</label>
          <div className="flex items-center gap-3">
            <input
              type="color"
              value={/^#[0-9a-fA-F]{6}$/.test(color) ? color : '#3b82f6'}
              onChange={(e) => setColor(e.target.value)}
              aria-label="Pilih warna"
              className="h-10 w-10 cursor-pointer rounded border border-slate-300 p-1"
            />
            <input type="text" maxLength={LIMITS.tag.color} value={color} onChange={(e) => setColor(e.target.value)} placeholder="#RRGGBB" className={inputCls} />
          </div>
        </div>
        {error && <p role="alert" className="text-sm text-red-600 dark:text-red-400">{error}</p>}
        <div className="pt-2">
          <button type="submit" disabled={mutation.isPending} className={`${primaryBtnCls} w-full`}>
            {mutation.isPending ? 'Menyimpan...' : 'Simpan Perubahan'}
          </button>
        </div>
      </form>
    </Modal>
  )
}

function CreateTagModal({ onClose }: { onClose: () => void }) {
  const [name, setName] = useState('')
  const [color, setColor] = useState('#3b82f6') // default blue
  const [error, setError] = useState<string | null>(null)
  const createMutation = useCreateTag()
  const toast = useToast()

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    if (!name) {
      setError('Nama tag wajib diisi.')
      return
    }
    try {
      await createMutation.mutateAsync({ name, color })
      toast.success(`Tag "${name}" dibuat.`)
      onClose()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Gagal membuat tag')
    }
  }

  return (
    <Modal title="Tag Baru" onClose={onClose}>
      <form onSubmit={handleSubmit} className="space-y-4">
        <div>
          <label className="mb-1 block text-sm font-medium text-slate-600 dark:text-slate-400">Nama Tag</label>
          <input required maxLength={LIMITS.tag.name} value={name} onChange={(e) => setName(e.target.value)} placeholder="Contoh: VIP, Suspended, Beta" className={inputCls} />
        </div>
        <div>
          <label className="mb-1 block text-sm font-medium text-slate-600 dark:text-slate-400">Warna (Opsional)</label>
          <div className="flex items-center gap-3">
            <input type="color" value={color} onChange={(e) => setColor(e.target.value)} className="h-10 w-10 cursor-pointer rounded border border-slate-300 p-1" />
            <input type="text" maxLength={LIMITS.tag.color} value={color} onChange={(e) => setColor(e.target.value)} placeholder="#RRGGBB" className={inputCls} />
          </div>
        </div>
        {error && <p className="text-sm text-red-600">{error}</p>}
        <div className="pt-2">
          <button type="submit" disabled={createMutation.isPending} className={`${primaryBtnCls} w-full`}>
            {createMutation.isPending ? 'Menyimpan...' : 'Simpan Tag'}
          </button>
        </div>
      </form>
    </Modal>
  )
}
