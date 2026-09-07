import { useState, type FormEvent } from 'react'
import { Modal } from './Modal'
import { PasswordStrengthBar } from './PasswordStrengthBar'
import { scorePassword } from '../lib/password'
import { ApiError } from '../lib/api'
import { useToast } from '../lib/toast'
import { useChangeOwnPassword } from '../lib/hooks'

const inputCls =
  'w-full rounded-lg border border-slate-300 px-3 py-2 text-sm text-slate-900 outline-none transition-colors focus:border-slate-500 focus:ring-1 focus:ring-slate-500 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-100'

const MIN_LEN = 8

export function ChangePasswordModal({ onClose }: { onClose: () => void }) {
  const mut = useChangeOwnPassword()
  const toast = useToast()
  const [current, setCurrent] = useState('')
  const [next, setNext] = useState('')
  const [confirm, setConfirm] = useState('')
  const [error, setError] = useState<string | null>(null)

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    if (next.length < MIN_LEN) return setError(`Password baru minimal ${MIN_LEN} karakter.`)
    if (next !== confirm) return setError('Konfirmasi password tidak cocok.')
    if (next === current) return setError('Password baru harus berbeda dari yang lama.')
    if (scorePassword(next).score < 2) return setError('Password terlalu lemah — gunakan kombinasi yang lebih kuat.')
    try {
      await mut.mutateAsync({ current_password: current, new_password: next })
      toast.success('Password berhasil diganti. Gunakan password baru saat login berikutnya.')
      onClose()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Gagal mengganti password')
    }
  }

  return (
    <Modal title="Ganti Password" onClose={onClose}>
      <form onSubmit={handleSubmit} className="space-y-4">
        <div>
          <label className="mb-1 block text-sm font-medium text-slate-600 dark:text-slate-400">Password saat ini</label>
          <input type="password" autoComplete="current-password" value={current} onChange={(e) => setCurrent(e.target.value)} required className={inputCls} />
        </div>
        <div>
          <label className="mb-1 block text-sm font-medium text-slate-600 dark:text-slate-400">Password baru</label>
          <input type="password" autoComplete="new-password" value={next} onChange={(e) => setNext(e.target.value)} required minLength={MIN_LEN} placeholder={`Minimal ${MIN_LEN} karakter`} className={inputCls} />
          {next ? <PasswordStrengthBar password={next} /> : <p className="mt-1 text-xs text-slate-400">Minimal {MIN_LEN} karakter.</p>}
        </div>
        <div>
          <label className="mb-1 block text-sm font-medium text-slate-600 dark:text-slate-400">Ulangi password baru</label>
          <input type="password" autoComplete="new-password" value={confirm} onChange={(e) => setConfirm(e.target.value)} required className={inputCls} />
        </div>
        {error && <p className="text-sm text-red-600 dark:text-red-400">{error}</p>}
        <div className="flex justify-end gap-2.5 pt-1">
          <button
            type="button"
            onClick={onClose}
            className="rounded-lg px-3.5 py-2 text-sm font-medium text-slate-600 transition-colors hover:bg-slate-100 dark:text-slate-400 dark:hover:bg-slate-800"
          >
            Batal
          </button>
          <button
            type="submit"
            disabled={mut.isPending}
            className="rounded-lg bg-slate-900 px-3.5 py-2 text-sm font-medium text-white transition-colors hover:bg-slate-800 disabled:opacity-60 dark:bg-slate-100 dark:text-slate-900 dark:hover:bg-white"
          >
            {mut.isPending ? 'Menyimpan…' : 'Ganti Password'}
          </button>
        </div>
      </form>
    </Modal>
  )
}
