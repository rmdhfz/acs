import { useState, type FormEvent } from 'react'
import { Modal } from './Modal'
import { PasswordStrengthBar } from './PasswordStrengthBar'
import { scorePassword } from '../lib/password'
import { ApiError, setAuthToken } from '../lib/api'
import { useToast } from '../lib/toast'
import { useChangeOwnPassword } from '../lib/hooks'

const inputCls =
  'w-full rounded-lg border border-slate-300 px-3 py-2 text-sm text-slate-900 outline-none transition-colors focus:border-indigo-600 focus:ring-1 focus:ring-indigo-600 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-100'

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
      const res = await mut.mutateAsync({ current_password: current, new_password: next })
      // Sesi lama dicabut server; pakai JWT baru supaya user tidak terlempar keluar.
      setAuthToken(res.access_token)
      toast.success('Password berhasil diganti. Gunakan password baru saat login berikutnya.')
      onClose()
    } catch (err) {
      // 401 di sini = password saat ini salah (bukan sesi berakhir; lihat lib/api.ts).
      if (err instanceof ApiError && err.status === 401) setError('Password saat ini salah.')
      else setError(err instanceof ApiError ? err.message : 'Gagal mengganti password')
    }
  }

  return (
    <Modal title="Ganti Password" onClose={onClose}>
      <form onSubmit={handleSubmit} className="space-y-4">
        <div>
          <label className="mb-1 block text-sm font-medium text-slate-600 dark:text-slate-400">Password saat ini</label>
          <input maxLength={128} type="password" autoComplete="current-password" value={current} onChange={(e) => setCurrent(e.target.value)} required className={inputCls} />
        </div>
        <div>
          <label className="mb-1 block text-sm font-medium text-slate-600 dark:text-slate-400">Password baru</label>
          <input maxLength={128} type="password" autoComplete="new-password" value={next} onChange={(e) => setNext(e.target.value)} required minLength={MIN_LEN} placeholder={`Minimal ${MIN_LEN} karakter`} className={inputCls} />
          {next ? <PasswordStrengthBar password={next} /> : <p className="mt-1 text-xs text-slate-500 dark:text-slate-400">Minimal {MIN_LEN} karakter.</p>}
        </div>
        <div>
          <label className="mb-1 block text-sm font-medium text-slate-600 dark:text-slate-400">Ulangi password baru</label>
          <input minLength={8} maxLength={128} type="password" autoComplete="new-password" value={confirm} onChange={(e) => setConfirm(e.target.value)} required className={inputCls} />
        </div>
        {error && <p className="text-sm text-red-600 dark:text-red-400">{error}</p>}
        <div className="flex justify-end gap-2.5 pt-1">
          <button
            type="button"
            onClick={onClose}
            className="rounded-full px-3.5 py-2 text-sm font-medium text-slate-600 transition-colors hover:bg-slate-100 dark:text-slate-400 dark:hover:bg-slate-800"
          >
            Batal
          </button>
          <button
            type="submit"
            disabled={mut.isPending}
            className="rounded-full bg-indigo-600 px-3.5 py-2 text-sm font-medium text-white transition-colors hover:bg-indigo-700 disabled:opacity-60 dark:bg-indigo-300 dark:text-slate-900 dark:hover:bg-indigo-200"
          >
            {mut.isPending ? 'Menyimpan…' : 'Ganti Password'}
          </button>
        </div>
      </form>
    </Modal>
  )
}
