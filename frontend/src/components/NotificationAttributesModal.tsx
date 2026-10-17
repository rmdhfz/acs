import { useState, type FormEvent } from 'react'
import { Modal } from './Modal'
import { ApiError } from '../lib/api'
import { useSetParameterAttributes } from '../lib/hooks'
import { useToast } from '../lib/toast'

const inputCls =
  'block w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 outline-none focus:border-indigo-500 focus:ring-2 focus:ring-indigo-500/20 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-100'

// Atur notifikasi perubahan nilai (SetParameterAttributes): CPE melaporkan event
// "4 VALUE CHANGE" untuk parameter terpilih tanpa perlu polling.
export function NotificationAttributesModal({ deviceId, initialNames, onClose }: { deviceId: number; initialNames?: string; onClose: () => void }) {
  const [names, setNames] = useState(initialNames ?? '')
  const [level, setLevel] = useState(2)
  const [error, setError] = useState<string | null>(null)
  const mut = useSetParameterAttributes()
  const toast = useToast()

  function submit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    const list = names.split('\n').map((n) => n.trim()).filter(Boolean)
    if (list.length === 0 || list.length > 200) {
      setError('Isi 1–200 nama parameter (satu per baris).')
      return
    }
    mut.mutate(
      { deviceId, attributes: list.map((name) => ({ name, notification_change: true, notification: level })) },
      {
        onSuccess: () => {
          toast.success('Task SetParameterAttributes diantrekan — berlaku setelah perangkat Inform.', 'Berhasil')
          onClose()
        },
        onError: (err) => setError(err instanceof ApiError ? err.message : 'Gagal mengantrekan task'),
      },
    )
  }

  return (
    <Modal title="Notifikasi perubahan nilai" onClose={onClose} dismissable={!mut.isPending}>
      <form onSubmit={submit} className="space-y-3">
        <div>
          <label htmlFor="attr-names" className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">
            Parameter (satu per baris; logical key atau raw path)
          </label>
          <textarea id="attr-names" required rows={5} value={names} onChange={(e) => setNames(e.target.value)} maxLength={20000}
            placeholder={'wifi.ssid\nInternetGatewayDevice.DeviceInfo.SoftwareVersion'} className={`${inputCls} font-mono text-xs`} />
        </div>
        <div>
          <label htmlFor="attr-level" className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Mode notifikasi</label>
          <select id="attr-level" value={level} onChange={(e) => setLevel(Number(e.target.value))} className={inputCls}>
            <option value={0}>Mati (0)</option>
            <option value={1}>Pasif (1) — dilaporkan pada Inform berikutnya</option>
            <option value={2}>Aktif (2) — CPE segera memulai sesi saat berubah</option>
          </select>
        </div>
        {error && <p role="alert" className="text-sm text-red-600">{error}</p>}
        <div className="flex justify-end gap-2 pt-1">
          <button type="button" onClick={onClose} className="rounded-full border border-slate-300 px-3 py-2 text-sm text-slate-700 hover:bg-slate-50 dark:border-slate-700 dark:text-slate-300 dark:hover:bg-slate-800">Batal</button>
          <button type="submit" disabled={mut.isPending} className="rounded-full bg-indigo-600 px-4 py-2 text-sm font-semibold text-white hover:bg-indigo-500 disabled:opacity-50">
            {mut.isPending ? 'Mengantrekan…' : 'Terapkan'}
          </button>
        </div>
      </form>
    </Modal>
  )
}
