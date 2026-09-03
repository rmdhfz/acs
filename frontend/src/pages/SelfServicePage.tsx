import { useState, type FormEvent } from 'react'
import { Wifi, RotateCw, Router as RouterIcon, CheckCircle2, AlertCircle } from 'lucide-react'
import { EmptyState } from '../components/EmptyState'
import { PageSpinner } from '../components/Spinner'
import { ApiError } from '../lib/api'
import { useConfirm } from '../lib/confirm'
import { useMyDevices, useChangeMyWiFi, useRebootMyDevice } from '../lib/hooks'
import type { SelfServiceDevice } from '../lib/types'

const inputCls =
  'w-full rounded-lg border border-slate-300 px-3 py-2 text-sm text-slate-900 outline-none transition-colors focus:border-slate-500 focus:ring-1 focus:ring-slate-500 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-100'
const primaryBtnCls =
  'flex items-center justify-center gap-2 rounded-lg bg-slate-900 px-3 py-2 text-sm font-medium text-white transition-colors hover:bg-slate-800 disabled:cursor-not-allowed disabled:opacity-60 dark:bg-slate-100 dark:text-slate-900 dark:hover:bg-white'

export default function SelfServicePage() {
  const { data, isLoading } = useMyDevices()
  const devices = data?.data ?? []

  return (
    <div className="mx-auto max-w-3xl px-6 py-8">
      <div className="mb-6">
        <h1 className="text-xl font-semibold text-slate-900 dark:text-slate-100">Perangkat Saya</h1>
        <p className="mt-0.5 text-sm text-slate-500 dark:text-slate-400">
          Ubah nama &amp; kata sandi WiFi, atau restart perangkat Anda sendiri.
        </p>
      </div>

      {isLoading ? (
        <PageSpinner />
      ) : devices.length === 0 ? (
        <EmptyState
          icon={RouterIcon}
          title="Belum ada perangkat"
          description="Perangkat yang terhubung ke akun Anda akan muncul di sini. Hubungi operator bila menurut Anda ini keliru."
        />
      ) : (
        <div className="space-y-4">
          {devices.map((d) => (
            <DeviceCard key={d.id} device={d} />
          ))}
        </div>
      )}
    </div>
  )
}

function DeviceCard({ device }: { device: SelfServiceDevice }) {
  const [showWiFi, setShowWiFi] = useState(false)
  const rebootMut = useRebootMyDevice()
  const [rebootDone, setRebootDone] = useState(false)
  const confirm = useConfirm()

  async function handleReboot() {
    const ok = await confirm({
      title: 'Restart perangkat',
      message: `Internet akan terputus sekitar 1–2 menit selagi ${device.model ?? 'perangkat'} menyala ulang.`,
      confirmLabel: 'Restart',
    })
    if (!ok) return
    rebootMut.mutate(device.id, {
      onSuccess: () => {
        setRebootDone(true)
        setTimeout(() => setRebootDone(false), 6000)
      },
    })
  }

  return (
    <div className="rounded-xl border border-slate-200 bg-white p-5 shadow-sm dark:border-slate-800 dark:bg-slate-900">
      <div className="flex items-start justify-between gap-4">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <span
              className={`h-2 w-2 shrink-0 rounded-full ${device.online ? 'bg-emerald-500' : 'bg-slate-300 dark:bg-slate-600'}`}
              title={device.online ? 'Online' : 'Offline'}
            />
            <span className="truncate font-medium text-slate-900 dark:text-slate-100">
              {device.model ?? 'Perangkat'}
            </span>
            <span className="text-xs text-slate-400">{device.online ? 'Online' : 'Offline'}</span>
          </div>
          <dl className="mt-2 grid grid-cols-2 gap-x-4 gap-y-1 text-xs text-slate-500 dark:text-slate-400">
            <div>
              <dt className="inline text-slate-400">Serial: </dt>
              <dd className="inline font-mono text-slate-600 dark:text-slate-300">{device.serial_number}</dd>
            </div>
            <div>
              <dt className="inline text-slate-400">Firmware: </dt>
              <dd className="inline text-slate-600 dark:text-slate-300">{device.software_version ?? '–'}</dd>
            </div>
          </dl>
        </div>
        <button
          onClick={handleReboot}
          disabled={rebootMut.isPending}
          className="flex shrink-0 items-center gap-1.5 rounded-lg border border-slate-300 px-3 py-1.5 text-xs font-medium text-slate-700 transition-colors hover:bg-slate-50 disabled:opacity-60 dark:border-slate-700 dark:text-slate-300 dark:hover:bg-slate-800"
        >
          <RotateCw className={`h-3.5 w-3.5 ${rebootMut.isPending ? 'animate-spin' : ''}`} />
          Restart
        </button>
      </div>

      {rebootDone && (
        <p className="mt-3 flex items-center gap-1.5 text-xs text-emerald-600 dark:text-emerald-400">
          <CheckCircle2 className="h-3.5 w-3.5" /> Perintah restart terkirim. Perangkat akan menyala ulang saat terhubung berikutnya.
        </p>
      )}
      {rebootMut.isError && (
        <p className="mt-3 flex items-center gap-1.5 text-xs text-red-600 dark:text-red-400">
          <AlertCircle className="h-3.5 w-3.5" />
          {rebootMut.error instanceof ApiError ? rebootMut.error.message : 'Gagal mengirim perintah restart'}
        </p>
      )}

      <div className="mt-4 border-t border-slate-100 pt-4 dark:border-slate-800">
        {showWiFi ? (
          <WiFiForm deviceId={device.id} onClose={() => setShowWiFi(false)} />
        ) : (
          <button
            onClick={() => setShowWiFi(true)}
            className="flex items-center gap-2 text-sm font-medium text-slate-700 hover:text-slate-900 dark:text-slate-300 dark:hover:text-slate-100"
          >
            <Wifi className="h-4 w-4" /> Ubah WiFi
          </button>
        )}
      </div>
    </div>
  )
}

function WiFiForm({ deviceId, onClose }: { deviceId: number; onClose: () => void }) {
  const [ssid, setSsid] = useState('')
  const [passphrase, setPassphrase] = useState('')
  const [band, setBand] = useState<'' | '2g' | '5g'>('')
  const [error, setError] = useState<string | null>(null)
  const [done, setDone] = useState(false)
  const mut = useChangeMyWiFi()

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    if (!ssid && !passphrase) {
      setError('Isi minimal nama WiFi (SSID) atau kata sandi baru.')
      return
    }
    if (passphrase && (passphrase.length < 8 || passphrase.length > 63)) {
      setError('Kata sandi WiFi harus 8–63 karakter.')
      return
    }
    try {
      await mut.mutateAsync({
        deviceId,
        ssid: ssid || undefined,
        passphrase: passphrase || undefined,
        band,
      })
      setDone(true)
      setSsid('')
      setPassphrase('')
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Gagal menyimpan perubahan WiFi')
    }
  }

  if (done) {
    return (
      <div className="text-sm">
        <p className="flex items-center gap-1.5 text-emerald-600 dark:text-emerald-400">
          <CheckCircle2 className="h-4 w-4" /> Perubahan diantre. Perangkat akan menerapkannya saat terhubung berikutnya (biasanya beberapa menit).
        </p>
        <button onClick={onClose} className="mt-2 text-xs text-slate-500 hover:text-slate-700 dark:hover:text-slate-300">
          Tutup
        </button>
      </div>
    )
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-3">
      <div>
        <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Nama WiFi (SSID) baru</label>
        <input value={ssid} onChange={(e) => setSsid(e.target.value)} placeholder="Kosongkan bila tidak diubah" className={inputCls} />
      </div>
      <div>
        <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Kata sandi WiFi baru</label>
        <input
          type="text"
          value={passphrase}
          onChange={(e) => setPassphrase(e.target.value)}
          placeholder="8–63 karakter, kosongkan bila tidak diubah"
          className={inputCls}
        />
      </div>
      <div>
        <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Terapkan ke</label>
        <select value={band} onChange={(e) => setBand(e.target.value as '' | '2g' | '5g')} className={inputCls}>
          <option value="">Semua band (2.4 GHz &amp; 5 GHz)</option>
          <option value="2g">Hanya 2.4 GHz</option>
          <option value="5g">Hanya 5 GHz</option>
        </select>
      </div>
      {error && <p className="text-xs text-red-600 dark:text-red-400">{error}</p>}
      <div className="flex items-center gap-2 pt-1">
        <button type="submit" disabled={mut.isPending} className={primaryBtnCls}>
          {mut.isPending ? 'Menyimpan…' : 'Simpan perubahan'}
        </button>
        <button
          type="button"
          onClick={onClose}
          className="rounded-lg px-3 py-2 text-sm font-medium text-slate-600 hover:bg-slate-100 dark:text-slate-400 dark:hover:bg-slate-800"
        >
          Batal
        </button>
      </div>
    </form>
  )
}
