import { useState, type FormEvent } from 'react'
import { Tag as TagIcon, Plus, AlertCircle } from 'lucide-react'
import { EmptyState } from '../components/EmptyState'
import { PageSpinner } from '../components/Spinner'
import { Modal } from '../components/Modal'
import { useAuth } from '../lib/auth'
import { ApiError } from '../lib/api'
import { useToast } from '../lib/toast'
import { useTags, useCreateTag, useDeleteTag } from '../lib/hooks'
import { formatDateTime } from '../lib/format'
import { LIMITS } from '../lib/limits'
import type { Tag } from '../lib/types'

const inputCls =
  'w-full rounded-lg border border-slate-300 px-3 py-2 text-sm text-slate-900 outline-none transition-colors focus:border-slate-500 focus:ring-1 focus:ring-slate-500 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-100'
const primaryBtnCls =
  'flex items-center justify-center gap-2 rounded-lg bg-slate-900 px-3 py-2 text-sm font-medium text-white transition-colors hover:bg-slate-800 disabled:cursor-not-allowed disabled:opacity-60 dark:bg-slate-100 dark:text-slate-900 dark:hover:bg-white'

export default function TagsPage() {
  const { hasRole } = useAuth()
  const canManage = hasRole('ADMIN')
  const { data, isLoading } = useTags({ pageSize: 50 })
  const tags = data?.data ?? []

  const [showCreate, setShowCreate] = useState(false)
  const [tagToDelete, setTagToDelete] = useState<Tag | null>(null)
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
          <h1 className="text-xl font-semibold text-slate-900 dark:text-slate-100">Tags</h1>
          <p className="mt-0.5 text-sm text-slate-500 dark:text-slate-400">
            Kelola label dinamis untuk mengelompokkan perangkat CPE.
          </p>
        </div>
        {canManage && (
          <button onClick={() => setShowCreate(true)} className={primaryBtnCls}>
            <Plus className="h-4 w-4" /> Tag Baru
          </button>
        )}
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
                    <td className="px-5 py-3.5 font-medium text-slate-900 dark:text-slate-100">{t.name}</td>
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
                      {canManage && (
                        <button
                          onClick={() => setTagToDelete(t)}
                          className="text-red-600 hover:text-red-700 dark:text-red-400 dark:hover:text-red-300 font-medium text-xs"
                        >
                          Hapus
                        </button>
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
                className="px-4 py-2 text-sm font-medium text-slate-700 hover:bg-slate-100 rounded-lg transition-colors dark:text-slate-300 dark:hover:bg-slate-800"
              >
                Batal
              </button>
              <button
                onClick={handleDelete}
                disabled={deleteMutation.isPending}
                className="px-4 py-2 text-sm font-medium text-white bg-red-600 hover:bg-red-700 rounded-lg transition-colors disabled:opacity-50"
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
