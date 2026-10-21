import { Fragment, useEffect, useState, type FormEvent } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { ChevronDown, ChevronRight, Eye, EyeOff, Pencil, Power, Radio, RefreshCcw, Search, Wifi } from 'lucide-react'
import { Modal } from './Modal'
import { StatusBadge } from './StatusBadge'
import { RetryTaskButton, TaskSummaryView } from './TaskParts'
import { useAuth } from '../lib/auth'
import { useToast } from '../lib/toast'
import { useConfirm } from '../lib/confirm'
import { ApiError } from '../lib/api'
import {
  findRefById,
  useDeviceParameters,
  useDeviceTasks,
  useGetParameterValues,
  useRefreshParameterTree,
  useRefs,
  useSetParameterValues,
} from '../lib/hooks'
import { formatRelativeTime } from '../lib/format'
import { useWakeDevice } from '../lib/wake'
import { parseWifi, wifiReadNames, type WifiRow } from '../lib/wifi'
import type { Device } from '../lib/types'

// Tab WiFi di Device Detail: daftar SEMUA SSID perangkat (mis. ZTE F660 punya 8) dengan band, status, keamanan,
// channel, jumlah klien, dan password; ubah SSID/password per baris. Membaca lewat satu GetParameterValues cabang
// WLAN (path parsial), menulis lewat SetParameterValues ke path yang dilaporkan perangkat sendiri — tanpa API
// baru dan tanpa percabangan vendor.

const inputCls =
  'w-full rounded-lg border border-slate-300 px-3 py-2 text-sm text-slate-900 outline-none transition-colors focus:border-indigo-600 focus:ring-1 focus:ring-indigo-600 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-100'
const primaryBtnCls =
  'flex items-center justify-center gap-2 rounded-full bg-indigo-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-indigo-700 disabled:cursor-not-allowed disabled:opacity-60 dark:bg-indigo-300 dark:text-slate-900 dark:hover:bg-indigo-200'
const secondaryBtnCls =
  'flex items-center gap-1.5 whitespace-nowrap rounded-full border border-slate-300 bg-white px-3 py-1.5 text-sm font-medium text-slate-700 transition-colors hover:bg-slate-50 disabled:opacity-50 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-300 dark:hover:bg-slate-800'

const REDACTED = '[redacted]'
const WIFI_TASK_RE = /wlanconfiguration|wifi\.|ssid|keypassphrase|presharedkey/

function PasswordCell({ value, shown, onToggle, label }: { value: string | null; shown: boolean; onToggle: () => void; label: string }) {
  if (value === null) return <span className="text-slate-400">belum terbaca</span>
  if (value === REDACTED) return <span className="text-slate-400">disembunyikan</span>
  if (value === '') return <span className="text-slate-400">kosong / tidak dikembalikan ONT</span>
  return (
    <span className="inline-flex items-center gap-1.5">
      <span className="font-mono text-xs">{shown ? value : '•'.repeat(Math.min(value.length, 12))}</span>
      <button
        type="button"
        onClick={onToggle}
        aria-label={`${shown ? 'Sembunyikan' : 'Tampilkan'} password ${label}`}
        className="text-slate-500 hover:text-slate-900 dark:hover:text-slate-100"
      >
        {shown ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
      </button>
    </span>
  )
}

const checkCls = 'h-4 w-4 rounded border-slate-300 text-indigo-600 focus:ring-indigo-600 dark:border-slate-600'

function Unreported({ children }: { children: string }) {
  return <p className="text-xs text-slate-400 dark:text-slate-500">{children}</p>
}

function EditWifiModal({ device, row, readNames, onQueued, onClose }: { device: Device; row: WifiRow; readNames: string[]; onQueued: () => void; onClose: () => void }) {
  const toast = useToast()
  const confirm = useConfirm()
  const setVals = useSetParameterValues()
  const verify = useGetParameterValues()
  const [ssid, setSsid] = useState('')
  const [pass, setPass] = useState('')
  const [showPass, setShowPass] = useState(false)
  const [hide, setHide] = useState(row.hidden === true)
  const [isolate, setIsolate] = useState(row.isolation === true)
  const [maxClients, setMaxClients] = useState(row.maxClients ?? '')
  const [priority, setPriority] = useState(row.priority ?? '')
  const [error, setError] = useState<string | null>(null)

  const onSubmit = async (e: FormEvent) => {
    e.preventDefault()
    setError(null)
    const s = ssid.trim()
    const p = pass.trim()
    if (new TextEncoder().encode(s).length > 32) return setError('SSID maksimal 32 byte.')
    if (p && (p.length < 8 || p.length > 63)) return setError('Password WiFi harus 8–63 karakter.')
    // eslint-disable-next-line no-control-regex
    if (/[\u0000-\u001f\u007f]/.test(s + p)) return setError('SSID/password tidak boleh memuat karakter kontrol.')

    // Hanya parameter yang BERUBAH yang dikirim.
    const values: Record<string, string> = {}
    const changes: string[] = []
    if (s) {
      values[row.ssidPath] = s
      changes.push(`SSID → ${s}`)
    }
    if (p) {
      values[row.passphrasePath] = p
      changes.push('password baru')
    }
    if (row.hidePath && hide !== (row.hidden === true)) {
      // nilai parameter berarti "siarkan" (SSIDAdvertisementEnabled: terbalik) atau "sembunyikan" (*SSIDHide: langsung)
      values[row.hidePath] = hide !== row.hideInverted ? '1' : '0'
      changes.push(hide ? 'SSID disembunyikan' : 'SSID ditampilkan')
    }
    if (row.isolationPath && isolate !== (row.isolation === true)) {
      values[row.isolationPath] = isolate ? '1' : '0'
      changes.push(isolate ? 'isolasi SSID diaktifkan' : 'isolasi SSID dimatikan')
    }
    const mc = maxClients.trim()
    if (row.maxClientsPath && mc !== (row.maxClients ?? '')) {
      if (!/^\d+$/.test(mc) || Number(mc) < 1 || Number(mc) > 128) return setError('Maksimum klien harus bilangan bulat 1–128.')
      values[row.maxClientsPath] = mc
      changes.push(`maks. klien → ${mc}`)
    }
    const pr = priority.trim()
    if (row.priorityPath && pr !== (row.priority ?? '')) {
      if (!/^\d+$/.test(pr) || Number(pr) > 7) return setError('Prioritas harus bilangan bulat 0–7.')
      values[row.priorityPath] = pr
      changes.push(`prioritas → ${pr}`)
    }
    if (Object.keys(values).length === 0) return setError('Tidak ada perubahan.')

    const ok = await confirm({
      title: `Ubah WiFi #${row.index}`,
      message: (
        <>
          Perangkat <b>{device.serial_number}</b>, SSID #{row.index} (<b>{row.ssid || 'tanpa nama'}</b>): {changes.join(', ')}.{' '}
          {hide && !row.hidden ? 'SSID tersembunyi tidak muncul di daftar WiFi; pelanggan harus mengetik namanya manual. ' : ''}
          Perangkat yang tersambung ke WiFi ini bisa terputus sampai disambungkan ulang.
        </>
      ),
      confirmLabel: 'Ubah WiFi',
    })
    if (!ok) return

    setVals.mutate(
      { deviceId: device.id, values },
      {
        onSuccess: () => {
          // Baca ulang cabang WLAN setelah menulis (task berurutan di sesi yang sama) agar tabel menampilkan hasil sebenarnya.
          verify.mutate({ deviceId: device.id, names: readNames })
          onQueued()
          toast.success('Perubahan WiFi diantre, lalu dibaca ulang untuk verifikasi. Diterapkan saat ONT Inform berikutnya (atau klik "Refresh ONT").', 'Berhasil')
          onClose()
        },
        onError: (err) => setError(err instanceof ApiError ? err.message : 'Gagal mengantre perubahan WiFi'),
      },
    )
  }

  return (
    <Modal title={`Pengaturan WiFi #${row.index}${row.band ? ` · ${row.band}` : ''}`} onClose={onClose}>
      <form onSubmit={onSubmit} className="space-y-4">
        <p className="text-xs text-slate-500 dark:text-slate-400">
          Saat ini: <b>{row.ssid || '(kosong)'}</b>. Hanya pengaturan yang Anda ubah yang dikirim. Catat nilai lama agar bisa dikembalikan.
        </p>
        <fieldset className="space-y-3">
          <legend className="mb-1 text-xs font-semibold uppercase tracking-wide text-slate-500 dark:text-slate-400">Dasar</legend>
          <label className="block text-sm">
            <span className="mb-1 block text-slate-600 dark:text-slate-300">SSID baru</span>
            <input value={ssid} onChange={(e) => setSsid(e.target.value)} maxLength={32} placeholder="Kosongkan bila tidak diubah" className={inputCls} />
          </label>
          <label className="block text-sm">
            <span className="mb-1 block text-slate-600 dark:text-slate-300">Password baru (8–63 karakter)</span>
            <div className="relative">
              <input
                type={showPass ? 'text' : 'password'}
                value={pass}
                onChange={(e) => setPass(e.target.value)}
                minLength={8}
                maxLength={63}
                autoComplete="new-password"
                placeholder="Kosongkan bila tidak diubah"
                className={inputCls + ' pr-10'}
              />
              <button
                type="button"
                onClick={() => setShowPass((v) => !v)}
                aria-label={showPass ? 'Sembunyikan password baru' : 'Tampilkan password baru'}
                className="absolute inset-y-0 right-2 text-slate-500 hover:text-slate-900 dark:hover:text-slate-100"
              >
                {showPass ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
              </button>
            </div>
          </label>
        </fieldset>

        <fieldset className="space-y-3">
          <legend className="mb-1 text-xs font-semibold uppercase tracking-wide text-slate-500 dark:text-slate-400">Lanjutan</legend>
          {row.hidePath ? (
            <label className="flex items-center gap-2 text-sm text-slate-700 dark:text-slate-300">
              <input type="checkbox" checked={hide} onChange={(e) => setHide(e.target.checked)} className={checkCls} />
              Sembunyikan SSID (tidak disiarkan)
            </label>
          ) : (
            <Unreported>Sembunyikan SSID: tidak dilaporkan ONT ini.</Unreported>
          )}
          {row.isolationPath ? (
            <label className="flex items-center gap-2 text-sm text-slate-700 dark:text-slate-300">
              <input type="checkbox" checked={isolate} onChange={(e) => setIsolate(e.target.checked)} className={checkCls} />
              Isolasi SSID (klien tidak saling melihat)
            </label>
          ) : (
            <Unreported>Isolasi SSID: tidak dilaporkan ONT ini. Klik “Cari pengaturan lanjutan” di tab WiFi.</Unreported>
          )}
          {row.maxClientsPath ? (
            <label className="block text-sm">
              <span className="mb-1 block text-slate-600 dark:text-slate-300">Maksimum klien (1–128)</span>
              <input type="number" min={1} max={128} value={maxClients} onChange={(e) => setMaxClients(e.target.value)} className={inputCls} />
            </label>
          ) : (
            <Unreported>Maksimum klien: tidak dilaporkan ONT ini.</Unreported>
          )}
          {row.priorityPath ? (
            <label className="block text-sm">
              <span className="mb-1 block text-slate-600 dark:text-slate-300">Prioritas (0–7)</span>
              <input type="number" min={0} max={7} value={priority} onChange={(e) => setPriority(e.target.value)} className={inputCls} />
            </label>
          ) : (
            <Unreported>Prioritas: tidak dilaporkan ONT ini. Klik “Cari pengaturan lanjutan” di tab WiFi.</Unreported>
          )}
        </fieldset>

        {error && (
          <p role="alert" className="text-sm text-red-600 dark:text-red-400">
            {error}
          </p>
        )}
        <button type="submit" disabled={setVals.isPending} className={`${primaryBtnCls} w-full`}>
          {setVals.isPending ? 'Mengantre…' : 'Terapkan ke perangkat'}
        </button>
      </form>
    </Modal>
  )
}

export function DeviceWifiTab({ device }: { device: Device }) {
  const deviceId = device.id
  const { hasRole } = useAuth()
  const canRead = hasRole('ADMIN', 'NOC')
  const canWrite = hasRole('ADMIN')
  const toast = useToast()
  const qc = useQueryClient()

  const { data: params, isLoading } = useDeviceParameters(deviceId)
  const { data: tasksResp } = useDeviceTasks(deviceId)
  const { data: statusRefs } = useRefs('ref_task_status')
  const { data: typeRefs } = useRefs('ref_task_types')
  const getVals = useGetParameterValues()
  const setVals = useSetParameterValues()
  const discover = useRefreshParameterTree()
  const wakeDevice = useWakeDevice(device)
  const confirm = useConfirm()

  const [editing, setEditing] = useState<WifiRow | null>(null)
  const [revealed, setRevealed] = useState<Set<number>>(new Set())
  const [openClients, setOpenClients] = useState<Set<number>>(new Set())

  const { model, rows } = parseWifi(params)

  const wifiTasks = (tasksResp?.data ?? [])
    .filter((t) => {
      const code = findRefById(typeRefs, t.task_type_id)?.code
      if (code !== 'SET_PARAMETER_VALUES' && code !== 'GET_PARAMETER_VALUES') return false
      return WIFI_TASK_RE.test(JSON.stringify(t.parameters ?? '').toLowerCase())
    })
    .slice(0, 5)

  // Selama ada permintaan WiFi yang belum selesai, segarkan hasil tiap 5 detik agar nilai muncul tanpa reload manual.
  const hasOpen = wifiTasks.some((t) => {
    const c = findRefById(statusRefs, t.task_status_id)?.code
    return c === 'PENDING' || c === 'SENT' || c === 'QUEUED'
  })
  useEffect(() => {
    if (!hasOpen) return
    const id = setInterval(() => {
      qc.invalidateQueries({ queryKey: ['device', deviceId, 'parameters'] })
      qc.invalidateQueries({ queryKey: ['device', deviceId, 'tasks'] })
    }, 5000)
    return () => clearInterval(id)
  }, [hasOpen, qc, deviceId])

  const onRead = () => {
    getVals.mutate(
      { deviceId, names: wifiReadNames(model) },
      {
        onSuccess: () => {
          wakeDevice.wakeQuiet()
          toast.success('Permintaan baca semua SSID diantre. Hasil muncul setelah ONT Inform berikutnya (atau klik "Refresh ONT").', 'Berhasil')
        },
        onError: (e) => toast.error(e instanceof ApiError ? e.message : 'Gagal mengantre pembacaan WiFi'),
      },
    )
  }

  // Telusuri cabang WLAN/LAN lebih dalam (GetParameterNames bertingkat + nilai) untuk menemukan pengaturan yang tak ada di
  // pembacaan biasa (mis. isolasi SSID, prioritas). Yang ditemukan otomatis muncul di dialog pengaturan.
  const onDiscover = () => {
    discover.mutate(
      { deviceId, path: model === 'TR181' ? 'Device.WiFi.' : 'InternetGatewayDevice.LANDevice.1.', depth: 3 },
      {
        onSuccess: () => {
          wakeDevice.wakeQuiet()
          toast.success('Penelusuran cabang WiFi diantre. Hasil muncul bertahap setelah ONT Inform (bisa beberapa siklus).', 'Berhasil')
        },
        onError: (e) => toast.error(e instanceof ApiError ? e.message : 'Gagal mengantre penelusuran'),
      },
    )
  }

  // Aktifkan/nonaktifkan satu SSID (parameter Enable). Tipe boolean dikirim otomatis dari tipe yang pernah dilaporkan ONT.
  const onToggleEnabled = async (r: WifiRow) => {
    const turnOn = r.enabled !== true
    const ok = await confirm({
      title: turnOn ? `Aktifkan WiFi #${r.index}` : `Nonaktifkan WiFi #${r.index}`,
      message: turnOn ? (
        <>
          SSID #{r.index} (<b>{r.ssid || 'tanpa nama'}</b>) pada <b>{device.serial_number}</b> akan diaktifkan.
        </>
      ) : (
        <>
          SSID #{r.index} (<b>{r.ssid || 'tanpa nama'}</b>) pada <b>{device.serial_number}</b> akan dimatikan.
          {r.index === 1 ? ' Ini SSID utama: semua perangkat yang tersambung akan terputus dan pelanggan bisa kehilangan WiFi sampai diaktifkan lagi.' : ' Perangkat yang tersambung ke SSID ini akan terputus.'}
        </>
      ),
      confirmLabel: turnOn ? 'Aktifkan' : 'Nonaktifkan',
      tone: turnOn ? 'default' : 'danger',
    })
    if (!ok) return
    setVals.mutate(
      { deviceId, values: { [r.enablePath]: turnOn ? '1' : '0' } },
      {
        onSuccess: () => {
          getVals.mutate({ deviceId, names: wifiReadNames(model) })
          wakeDevice.wakeQuiet()
          toast.success(`${turnOn ? 'Aktivasi' : 'Penonaktifan'} WiFi #${r.index} diantre, lalu dibaca ulang untuk verifikasi.`, 'Berhasil')
        },
        onError: (e) => toast.error(e instanceof ApiError ? e.message : 'Gagal mengantre perubahan status WiFi'),
      },
    )
  }

  const toggleClients = (i: number) =>
    setOpenClients((prev) => {
      const next = new Set(prev)
      if (next.has(i)) next.delete(i)
      else next.add(i)
      return next
    })

  const toggleReveal = (i: number) =>
    setRevealed((prev) => {
      const next = new Set(prev)
      if (next.has(i)) next.delete(i)
      else next.add(i)
      return next
    })

  if (!canRead) {
    return <p className="text-sm text-slate-500 dark:text-slate-400">Anda tidak punya akses ke pengaturan WiFi perangkat.</p>
  }

  const newest = rows.reduce<string | null>((acc, r) => (r.updatedAt && (!acc || r.updatedAt > acc) ? r.updatedAt : acc), null)

  return (
    <div className="space-y-6">
      <div className="rounded-xl border border-slate-200 bg-white shadow-sm dark:border-slate-800 dark:bg-slate-900">
        <div className="flex flex-wrap items-center justify-between gap-2 border-b border-slate-100 px-5 py-3 dark:border-slate-800">
          <div>
            <h3 className="flex items-center gap-2 text-sm font-medium text-slate-900 dark:text-slate-100">
              <Wifi className="h-4 w-4" /> WiFi perangkat
              {rows.length > 0 && <span className="rounded-full bg-slate-100 px-2 py-0.5 text-xs text-slate-600 dark:bg-slate-800 dark:text-slate-300">{rows.length} SSID</span>}
            </h3>
            {newest && <p className="mt-0.5 text-xs text-slate-500 dark:text-slate-400">Terakhir dibaca {formatRelativeTime(newest)}</p>}
          </div>
          <div className="flex flex-wrap gap-2">
            <button onClick={onRead} disabled={getVals.isPending} className={secondaryBtnCls}>
              <RefreshCcw className="h-4 w-4" /> Baca semua SSID
            </button>
            <button
              onClick={onDiscover}
              disabled={discover.isPending}
              title="Telusuri cabang WiFi lebih dalam untuk menemukan pengaturan lanjutan (isolasi SSID, prioritas, dst.) yang dilaporkan ONT"
              className={secondaryBtnCls}
            >
              <Search className="h-4 w-4" /> Cari pengaturan lanjutan
            </button>
            <button
              onClick={() => void wakeDevice.wake()}
              disabled={wakeDevice.pending || !wakeDevice.hasUrl}
              title={wakeDevice.hint}
              className={secondaryBtnCls}
            >
              <Radio className="h-4 w-4" /> Refresh ONT
            </button>
          </div>
        </div>

        {isLoading ? (
          <p className="px-5 py-4 text-sm text-slate-500 dark:text-slate-400">Memuat…</p>
        ) : rows.length === 0 ? (
          <p className="px-5 py-6 text-sm text-slate-500 dark:text-slate-400">
            Belum ada data WiFi. Klik <b>Baca semua SSID</b>; ONT akan mengirim seluruh konfigurasi WLAN pada Inform berikutnya.
          </p>
        ) : (
          <div className="table-scroll">
            <table className="w-full text-left text-sm">
              <thead>
                <tr className="border-b border-slate-200 bg-slate-50 text-xs font-medium uppercase tracking-wide text-slate-500 dark:border-slate-800 dark:bg-slate-800/50 dark:text-slate-400">
                  <th className="px-4 py-3">#</th>
                  <th className="px-4 py-3">SSID</th>
                  <th className="px-4 py-3">Band</th>
                  <th className="px-4 py-3">Status</th>
                  <th className="px-4 py-3">Keamanan</th>
                  <th className="px-4 py-3">Ch</th>
                  <th className="px-4 py-3">Klien</th>
                  <th className="px-4 py-3">Password</th>
                  {canWrite && <th className="px-4 py-3 text-right">Aksi</th>}
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
                {rows.map((r) => (
                  <Fragment key={r.index}>
                    <tr className="text-slate-700 dark:text-slate-300">
                      <td className="px-4 py-3 text-slate-500">{r.index}</td>
                      <td className="px-4 py-3 font-medium text-slate-900 dark:text-slate-100">{r.ssid || <span className="text-slate-400">(kosong)</span>}
                        {r.hidden === true && <span className="ml-2 rounded-full bg-slate-100 px-2 py-0.5 text-[10px] font-medium text-slate-600 dark:bg-slate-800 dark:text-slate-300">Tersembunyi</span>}
                        {r.isolation === true && <span className="ml-1 rounded-full bg-indigo-50 px-2 py-0.5 text-[10px] font-medium text-indigo-700 dark:bg-indigo-500/10 dark:text-indigo-300">Isolasi</span>}
                      </td>
                      <td className="px-4 py-3">{r.band ?? '-'}</td>
                      <td className="px-4 py-3">
                        <StatusBadge
                          code={r.enabled === null ? undefined : r.enabled ? 'ONLINE' : 'OFFLINE'}
                          label={r.enabled === null ? 'Belum diketahui' : r.enabled ? 'Aktif' : 'Nonaktif'}
                        />
                      </td>
                      <td className="px-4 py-3 text-xs">{r.security ?? '-'}</td>
                      <td className="px-4 py-3">{r.channel ?? '-'}</td>
                      <td className="px-4 py-3">
                        {r.clientList.length > 0 ? (
                          <button
                            type="button"
                            onClick={() => toggleClients(r.index)}
                            aria-expanded={openClients.has(r.index)}
                            aria-label={`Klien SSID ${r.index}`}
                            className="inline-flex items-center gap-0.5 font-medium text-blue-600 hover:underline dark:text-blue-400"
                          >
                            {openClients.has(r.index) ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronRight className="h-3.5 w-3.5" />}
                            {r.clientList.length}
                            {r.maxClients ? <span className="font-normal text-slate-400">/{r.maxClients}</span> : null}
                          </button>
                        ) : (
                          <>
                            {r.clients ?? '-'}
                            {r.maxClients ? <span className="text-slate-400">/{r.maxClients}</span> : null}
                          </>
                        )}
                      </td>
                      <td className="px-4 py-3">
                        <PasswordCell value={r.passphrase} shown={revealed.has(r.index)} onToggle={() => toggleReveal(r.index)} label={`SSID ${r.index}`} />
                      </td>
                      {canWrite && (
                        <td className="px-4 py-3 text-right">
                          <div className="inline-flex items-center gap-1.5">
                            <button
                              onClick={() => void onToggleEnabled(r)}
                              disabled={setVals.isPending}
                              aria-label={`${r.enabled === true ? 'Nonaktifkan' : 'Aktifkan'} WiFi ${r.index}`}
                              className={`inline-flex items-center gap-1 rounded-full border px-2.5 py-1 text-xs font-medium disabled:opacity-50 ${
                                r.enabled === true
                                  ? 'border-red-200 text-red-700 hover:bg-red-50 dark:border-red-500/40 dark:text-red-400 dark:hover:bg-red-500/10'
                                  : 'border-emerald-200 text-emerald-700 hover:bg-emerald-50 dark:border-emerald-500/40 dark:text-emerald-400 dark:hover:bg-emerald-500/10'
                              }`}
                            >
                              <Power className="h-3.5 w-3.5" /> {r.enabled === true ? 'Nonaktifkan' : 'Aktifkan'}
                            </button>
                            <button
                              onClick={() => setEditing(r)}
                              aria-label={`Ubah WiFi ${r.index}`}
                              className="inline-flex items-center gap-1 rounded-full border border-slate-300 px-2.5 py-1 text-xs font-medium text-slate-700 hover:bg-slate-50 dark:border-slate-700 dark:text-slate-300 dark:hover:bg-slate-800"
                            >
                              <Pencil className="h-3.5 w-3.5" /> Ubah
                            </button>
                          </div>
                        </td>
                      )}
                    </tr>
                    {openClients.has(r.index) && (
                      <tr className="bg-slate-50 dark:bg-slate-800/40">
                        <td colSpan={canWrite ? 9 : 8} className="px-4 py-3">
                          <p className="mb-1.5 text-xs font-medium text-slate-600 dark:text-slate-300">Klien tersambung ke SSID #{r.index}</p>
                          <table className="w-full text-left text-xs">
                            <thead>
                              <tr className="text-slate-500 dark:text-slate-400">
                                <th className="py-1 pr-4 font-medium">Nama</th>
                                <th className="py-1 pr-4 font-medium">IP</th>
                                <th className="py-1 pr-4 font-medium">MAC</th>
                                <th className="py-1 font-medium">Sinyal</th>
                              </tr>
                            </thead>
                            <tbody className="text-slate-700 dark:text-slate-300">
                              {r.clientList.map((c) => (
                                <tr key={c.mac}>
                                  <td className="py-1 pr-4">{c.name ?? <span className="text-slate-400">-</span>}</td>
                                  <td className="py-1 pr-4 font-mono">{c.ip ?? '-'}</td>
                                  <td className="py-1 pr-4 font-mono">{c.mac}</td>
                                  <td className="py-1">{c.rssi ? `${c.rssi} dBm` : '-'}</td>
                                </tr>
                              ))}
                            </tbody>
                          </table>
                        </td>
                      </tr>
                    )}
                  </Fragment>
                ))}
              </tbody>
            </table>
          </div>
        )}
        {rows.length > 0 && (
          <p className="border-t border-slate-100 px-5 py-3 text-xs text-slate-500 dark:border-slate-800 dark:text-slate-400">
            Password bisa tampil kosong: sebagian ONT tidak mengembalikannya saat dibaca. Band &amp; keamanan diambil dari parameter yang dilaporkan ONT
            (bila tidak ada, band diperkirakan dari channel). {!canWrite && 'Mengubah WiFi hanya untuk role ADMIN.'}
          </p>
        )}
      </div>

      <div className="rounded-xl border border-slate-200 bg-white shadow-sm dark:border-slate-800 dark:bg-slate-900">
        <div className="border-b border-slate-100 px-5 py-3 text-sm font-medium text-slate-900 dark:border-slate-800 dark:text-slate-100">
          Status permintaan WiFi terakhir
        </div>
        {wifiTasks.length === 0 ? (
          <p className="px-5 py-4 text-sm text-slate-500 dark:text-slate-400">Belum ada permintaan baca/ubah WiFi.</p>
        ) : (
          <ul className="divide-y divide-slate-100 dark:divide-slate-800">
            {wifiTasks.map((t) => {
              const status = findRefById(statusRefs, t.task_status_id)
              return (
                <li key={t.id} className="flex flex-wrap items-start gap-x-4 gap-y-2 px-5 py-3 text-sm">
                  <StatusBadge code={status?.code} label={status?.name ?? '-'} />
                  <div className="min-w-0 flex-1">
                    <TaskSummaryView task={t} />
                    <p className="mt-0.5 text-xs text-slate-500 dark:text-slate-400">
                      {formatRelativeTime(t.created_at)} · percobaan {t.retry_count}/{t.max_retries}
                    </p>
                    {t.error_message && status?.code !== 'COMPLETED' && <p className="mt-0.5 text-xs text-red-600">{t.error_message}</p>}
                  </div>
                  <RetryTaskButton task={t} />
                </li>
              )
            })}
          </ul>
        )}
      </div>

      {editing && <EditWifiModal device={device} row={editing} readNames={wifiReadNames(model)} onQueued={wakeDevice.wakeQuiet} onClose={() => setEditing(null)} />}
    </div>
  )
}
