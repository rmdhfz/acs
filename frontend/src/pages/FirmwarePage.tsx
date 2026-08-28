import { useEffect, useState, type FormEvent } from 'react'
import { HardDrive, Plus, Rocket } from 'lucide-react'
import { EmptyState } from '../components/EmptyState'
import { Modal } from '../components/Modal'
import { PageSpinner } from '../components/Spinner'
import { StatusBadge } from '../components/StatusBadge'
import { useAuth } from '../lib/auth'
import { ApiError } from '../lib/api'
import {
  useAdvanceRolloutBatch,
  useCancelRolloutBatch,
  useCreateRolloutBatch,
  useDeviceModels,
  useFirmwareList,
  useFirmwareRolloutBatches,
  useRefs,
  useUploadFirmware,
  useVendors,
  type CreateRolloutBatchInput,
  type UploadFirmwareInput,
} from '../lib/hooks'
import { formatDateTime } from '../lib/format'

const inputCls =
  'w-full rounded-lg border border-slate-300 px-3 py-2 text-sm text-slate-900 outline-none transition-colors focus:border-slate-500 focus:ring-1 focus:ring-slate-500 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-100'
const primaryBtnCls =
  'flex items-center justify-center gap-2 rounded-lg bg-slate-900 px-3 py-2 text-sm font-medium text-white transition-colors hover:bg-slate-800 disabled:cursor-not-allowed disabled:opacity-60 dark:bg-slate-100 dark:text-slate-900 dark:hover:bg-white'

type Tab = 'catalog' | 'rollout'

export function FirmwarePage() {
  const [tab, setTab] = useState<Tab>('catalog')
  const { hasRole } = useAuth()
  const canManage = hasRole('ADMIN')

  return (
    <div className="mx-auto max-w-7xl px-6 py-8">
      <div className="mb-6">
        <h1 className="text-xl font-semibold text-slate-900 dark:text-slate-100">Firmware</h1>
        <p className="mt-0.5 text-sm text-slate-500 dark:text-slate-400">
          Katalog firmware per vendor & canary/staged rollout ke populasi device
        </p>
      </div>

      <div className="mb-4 flex gap-1 border-b border-slate-200 dark:border-slate-800">
        <TabButton active={tab === 'catalog'} onClick={() => setTab('catalog')} icon={HardDrive} label="Katalog" />
        <TabButton active={tab === 'rollout'} onClick={() => setTab('rollout')} icon={Rocket} label="Rollout Batch" />
      </div>

      {tab === 'catalog' ? <CatalogTab canManage={canManage} /> : <RolloutTab canManage={canManage} />}
    </div>
  )
}

function TabButton({ active, onClick, icon: Icon, label }: { active: boolean; onClick: () => void; icon: typeof HardDrive; label: string }) {
  return (
    <button
      onClick={onClick}
      className={`flex items-center gap-1.5 border-b-2 px-3 py-2.5 text-sm font-medium transition-colors ${
        active ? 'border-slate-900 text-slate-900 dark:border-slate-100 dark:text-slate-100' : 'border-transparent text-slate-500 dark:text-slate-400 hover:text-slate-800'
      }`}
    >
      <Icon className="h-4 w-4" />
      {label}
    </button>
  )
}

// ---- Katalog ----

function CatalogTab({ canManage }: { canManage: boolean }) {
  const { data: vendorsResp } = useVendors()
  const vendors = vendorsResp?.data ?? []
  const [vendorId, setVendorId] = useState<string>('')
  const [showUpload, setShowUpload] = useState(false)

  const { data, isLoading } = useFirmwareList(vendorId ? Number(vendorId) : undefined)
  const files = data?.data ?? []

  return (
    <div>
      <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
        <select value={vendorId} onChange={(e) => setVendorId(e.target.value)} className={`${inputCls} max-w-xs`}>
          <option value="">Pilih vendor untuk melihat firmware...</option>
          {vendors.map((v) => (
            <option key={v.id} value={v.id}>
              {v.name}
            </option>
          ))}
        </select>
        {canManage && (
          <button onClick={() => setShowUpload(true)} className={primaryBtnCls}>
            <Plus className="h-4 w-4" /> Daftarkan Firmware
          </button>
        )}
      </div>

      <div className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm dark:border-slate-800 dark:bg-slate-900">
        {!vendorId ? (
          <EmptyState icon={HardDrive} title="Pilih vendor" description="Firmware dikelompokkan per vendor — pilih salah satu di atas." />
        ) : isLoading ? (
          <PageSpinner />
        ) : files.length === 0 ? (
          <EmptyState icon={HardDrive} title="Belum ada firmware terdaftar" description="File firmware yang diupload di sini disimpan di object storage (MinIO)." />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-left text-sm">
              <thead>
                <tr className="border-b border-slate-200 bg-slate-50 text-xs font-medium uppercase tracking-wide text-slate-500 dark:border-slate-800 dark:bg-slate-800/50 dark:text-slate-400">
                  <th className="px-5 py-3">Versi</th>
                  <th className="px-5 py-3">File</th>
                  <th className="px-5 py-3">Checksum</th>
                  <th className="px-5 py-3">Ukuran</th>
                  <th className="px-5 py-3">Didaftarkan</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
                {files.map((f) => (
                  <tr key={f.id}>
                    <td className="px-5 py-3.5 font-medium text-slate-900 dark:text-slate-100">{f.version}</td>
                    <td className="px-5 py-3.5 font-mono text-xs text-slate-600 dark:text-slate-400">{f.file_name}</td>
                    <td className="px-5 py-3.5 max-w-[220px] truncate font-mono text-xs text-slate-400 dark:text-slate-500" title={f.checksum_sha256 ?? undefined}>
                      {f.checksum_sha256 ?? '-'}
                    </td>
                    <td className="px-5 py-3.5 text-slate-500 dark:text-slate-400">{f.file_size_bytes ? `${(f.file_size_bytes / 1_000_000).toFixed(1)} MB` : '-'}</td>
                    <td className="px-5 py-3.5 text-slate-500 dark:text-slate-400">{formatDateTime(f.created_at)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {showUpload && <UploadFirmwareModal defaultVendorId={vendorId} onClose={() => setShowUpload(false)} />}
    </div>
  )
}

// ---- Rollout Batch (canary/staged rollout, migrations/0011) ----

const ROLLOUT_STATUS_BADGE: Record<string, { code: string; label: string }> = {
  PENDING: { code: 'PENDING', label: 'Menunggu' },
  IN_PROGRESS: { code: 'ONLINE', label: 'Berjalan' },
  PAUSED_FAILURE_THRESHOLD: { code: 'FAILED', label: 'Dijeda — ambang gagal' },
  COMPLETED: { code: 'COMPLETED', label: 'Selesai' },
  CANCELLED: { code: 'OFFLINE', label: 'Dibatalkan' },
}

function RolloutTab({ canManage }: { canManage: boolean }) {
  const { data, isLoading } = useFirmwareRolloutBatches()
  const { data: statusRefs } = useRefs('ref_firmware_rollout_status')
  const { data: vendorsResp } = useVendors()
  const vendors = vendorsResp?.data ?? []
  const [showCreate, setShowCreate] = useState(false)
  const advanceMutation = useAdvanceRolloutBatch()
  const cancelMutation = useCancelRolloutBatch()

  const batches = data?.data ?? []
  const statusCode = (id: number) => statusRefs?.find((s) => s.id === id)?.code ?? ''

  return (
    <div>
      <div className="mb-4 flex items-center justify-between gap-3">
        <p className="text-xs text-slate-500 dark:text-slate-400">
          Upgrade firmware bertahap per wave. Wave berikutnya hanya lanjut otomatis bila failure rate wave sebelumnya di bawah ambang — jika melebihi, batch dijeda menunggu keputusan operator.
        </p>
        {canManage && (
          <button onClick={() => setShowCreate(true)} className={primaryBtnCls}>
            <Plus className="h-4 w-4" /> Rollout Baru
          </button>
        )}
      </div>

      <div className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm dark:border-slate-800 dark:bg-slate-900">
        {isLoading ? (
          <PageSpinner />
        ) : batches.length === 0 ? (
          <EmptyState icon={Rocket} title="Belum ada rollout batch" description="Rollout batch menjadwalkan upgrade firmware ke populasi device (per vendor/model) secara bertahap dengan gerbang failure-rate antar wave." />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-left text-sm">
              <thead>
                <tr className="border-b border-slate-200 bg-slate-50 text-xs font-medium uppercase tracking-wide text-slate-500 dark:border-slate-800 dark:bg-slate-800/50 dark:text-slate-400">
                  <th className="px-5 py-3">Target</th>
                  <th className="px-5 py-3">Firmware</th>
                  <th className="px-5 py-3">Wave</th>
                  <th className="px-5 py-3">Ambang Gagal</th>
                  <th className="px-5 py-3">Status</th>
                  <th className="px-5 py-3"></th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
                {batches.map((b) => {
                  const code = statusCode(b.status_id)
                  const badge = ROLLOUT_STATUS_BADGE[code] ?? { code: 'PENDING', label: code || `#${b.status_id}` }
                  const canAct = canManage && (code === 'IN_PROGRESS' || code === 'PAUSED_FAILURE_THRESHOLD' || code === 'PENDING')
                  return (
                    <tr key={b.id}>
                      <td className="px-5 py-3.5 text-xs text-slate-600 dark:text-slate-400">
                        {[
                          b.vendor_id ? vendors.find((v) => v.id === b.vendor_id)?.name ?? `vendor#${b.vendor_id}` : 'Semua vendor',
                          b.device_model_id ? `model#${b.device_model_id}` : null,
                        ]
                          .filter(Boolean)
                          .join(' · ')}
                      </td>
                      <td className="px-5 py-3.5 text-xs text-slate-600 dark:text-slate-400">#{b.firmware_file_id}</td>
                      <td className="px-5 py-3.5 tabular-nums text-slate-700 dark:text-slate-300">
                        {b.current_wave} <span className="text-slate-400">×{b.wave_percentage}%</span>
                      </td>
                      <td className="px-5 py-3.5 tabular-nums text-slate-500 dark:text-slate-400">{b.max_failure_rate_percent}%</td>
                      <td className="px-5 py-3.5">
                        <StatusBadge code={badge.code} label={badge.label} />
                        {b.scheduled_at && code === 'PENDING' && <div className="mt-1 text-[10px] text-slate-500">Jadwal: {formatDateTime(b.scheduled_at)}</div>}
                      </td>
                      <td className="px-5 py-3.5 text-right">
                        {canAct && (
                          <div className="flex justify-end gap-2">
                            <button
                              onClick={() => advanceMutation.mutate(b.id)}
                              disabled={advanceMutation.isPending}
                              className="rounded-md border border-slate-300 px-2 py-1 text-xs font-medium text-slate-700 transition-colors hover:bg-slate-50 disabled:opacity-50 dark:border-slate-700 dark:text-slate-300 dark:hover:bg-slate-800"
                            >
                              Advance
                            </button>
                            <button
                              onClick={() => {
                                if (confirm('Batalkan rollout ini? Wave berikutnya tidak akan dijalankan.')) cancelMutation.mutate(b.id)
                              }}
                              disabled={cancelMutation.isPending}
                              className="rounded-md border border-red-200 px-2 py-1 text-xs font-medium text-red-600 transition-colors hover:bg-red-50 disabled:opacity-50 dark:border-red-900"
                            >
                              Batalkan
                            </button>
                          </div>
                        )}
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {showCreate && <CreateRolloutModal onClose={() => setShowCreate(false)} />}
    </div>
  )
}

function CreateRolloutModal({ onClose }: { onClose: () => void }) {
  const { data: vendorsResp } = useVendors()
  const vendors = vendorsResp?.data ?? []
  const [vendorId, setVendorId] = useState('')
  const [deviceModelId, setDeviceModelId] = useState('')
  const [firmwareFileId, setFirmwareFileId] = useState('')
  const [wavePercentage, setWavePercentage] = useState('10')
  const [maxFailureRate, setMaxFailureRate] = useState('10')
  const [scheduledAt, setScheduledAt] = useState('')
  const [notes, setNotes] = useState('')
  const [error, setError] = useState<string | null>(null)

  const { data: models } = useDeviceModels(vendorId ? Number(vendorId) : undefined)
  const { data: firmwareResp } = useFirmwareList(vendorId ? Number(vendorId) : undefined)
  const firmwareFiles = firmwareResp?.data ?? []
  const createMutation = useCreateRolloutBatch()

  // Reset firmware pilihan bila vendor berubah (daftar firmware ikut berubah).
  useEffect(() => {
    setFirmwareFileId('')
    setDeviceModelId('')
  }, [vendorId])

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    if (!firmwareFileId) {
      setError('Pilih firmware target terlebih dahulu.')
      return
    }
    const wp = Number(wavePercentage)
    if (wp < 1 || wp > 100) {
      setError('Wave percentage harus 1–100.')
      return
    }
    const input: CreateRolloutBatchInput = {
      firmware_file_id: Number(firmwareFileId),
      vendor_id: vendorId ? Number(vendorId) : undefined,
      device_model_id: deviceModelId ? Number(deviceModelId) : undefined,
      wave_percentage: wp,
      max_failure_rate_percent: Number(maxFailureRate),
      scheduled_at: scheduledAt ? new Date(scheduledAt).toISOString() : undefined,
      notes: notes || undefined,
    }
    try {
      await createMutation.mutateAsync(input)
      onClose()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Gagal membuat rollout batch')
    }
  }

  return (
    <Modal title="Rollout Batch Baru" onClose={onClose}>
      <form onSubmit={handleSubmit} className="space-y-3">
        <p className="text-xs text-slate-500 dark:text-slate-400">
          Wave pertama dimulai otomatis saat batch dibuat. Populasi target = device yang cocok filter vendor/model di bawah.
        </p>
        <div>
          <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Vendor (opsional — filter populasi)</label>
          <select value={vendorId} onChange={(e) => setVendorId(e.target.value)} className={inputCls}>
            <option value="">Semua vendor</option>
            {vendors.map((v) => (
              <option key={v.id} value={v.id}>
                {v.name}
              </option>
            ))}
          </select>
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Device Model (opsional — filter populasi)</label>
          <select value={deviceModelId} onChange={(e) => setDeviceModelId(e.target.value)} disabled={!vendorId} className={inputCls}>
            <option value="">Semua model</option>
            {models?.map((m) => (
              <option key={m.id} value={m.id}>
                {m.model_name}
              </option>
            ))}
          </select>
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Firmware Target</label>
          <select required value={firmwareFileId} onChange={(e) => setFirmwareFileId(e.target.value)} disabled={!vendorId} className={inputCls}>
            <option value="">{vendorId ? 'Pilih firmware...' : 'Pilih vendor dulu'}</option>
            {firmwareFiles.map((f) => (
              <option key={f.id} value={f.id}>
                {f.version} ({f.file_name})
              </option>
            ))}
          </select>
        </div>
        <div className="grid grid-cols-2 gap-3">
          <div>
            <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Wave Percentage (%)</label>
            <input type="number" min={1} max={100} value={wavePercentage} onChange={(e) => setWavePercentage(e.target.value)} className={inputCls} />
          </div>
          <div>
            <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Max Failure Rate (%)</label>
            <input type="number" min={0} max={100} value={maxFailureRate} onChange={(e) => setMaxFailureRate(e.target.value)} className={inputCls} />
          </div>
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Jadwal Mulai (opsional, kosong = mulai sekarang)</label>
          <input type="datetime-local" value={scheduledAt} onChange={(e) => setScheduledAt(e.target.value)} className={inputCls} />
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Catatan (opsional)</label>
          <input value={notes} onChange={(e) => setNotes(e.target.value)} className={inputCls} />
        </div>
        {error && <p className="text-sm text-red-600">{error}</p>}
        <button type="submit" disabled={createMutation.isPending} className={`${primaryBtnCls} w-full`}>
          Buat & Mulai Wave 1
        </button>
      </form>
    </Modal>
  )
}

function UploadFirmwareModal({ defaultVendorId, onClose }: { defaultVendorId: string; onClose: () => void }) {
  const { data: vendorsResp } = useVendors()
  const vendors = vendorsResp?.data ?? []
  const [vendorId, setVendorId] = useState(defaultVendorId)
  const [deviceModelId, setDeviceModelId] = useState('')
  const [version, setVersion] = useState('')
  const [file, setFile] = useState<File | null>(null)
  const [releaseNotes, setReleaseNotes] = useState('')
  const [error, setError] = useState<string | null>(null)
  const { data: models } = useDeviceModels(vendorId ? Number(vendorId) : undefined)
  const uploadMutation = useUploadFirmware()

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    if (!file) {
      setError('Pilih file firmware terlebih dahulu')
      return
    }
    const input: UploadFirmwareInput = {
      vendor_id: Number(vendorId),
      device_model_id: deviceModelId ? Number(deviceModelId) : undefined,
      version,
      release_notes: releaseNotes || undefined,
      file,
    }
    try {
      await uploadMutation.mutateAsync(input)
      onClose()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Gagal mengupload firmware')
    }
  }

  return (
    <Modal title="Daftarkan Firmware" onClose={onClose}>
      <form onSubmit={handleSubmit} className="space-y-3">
        <p className="text-xs text-slate-500 dark:text-slate-400">
          File akan diupload langsung ke object storage (MinIO); checksum SHA-256 dihitung otomatis di server.
        </p>
        <div className="grid grid-cols-2 gap-3">
          <div>
            <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Vendor</label>
            <select
              required
              value={vendorId}
              onChange={(e) => {
                setVendorId(e.target.value)
                setDeviceModelId('')
              }}
              className={inputCls}
            >
              <option value="">Pilih vendor...</option>
              {vendors.map((v) => (
                <option key={v.id} value={v.id}>
                  {v.name}
                </option>
              ))}
            </select>
          </div>
          <div>
            <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Device Model (opsional)</label>
            <select value={deviceModelId} onChange={(e) => setDeviceModelId(e.target.value)} disabled={!vendorId} className={inputCls}>
              <option value="">Semua model vendor ini</option>
              {models?.map((m) => (
                <option key={m.id} value={m.id}>
                  {m.model_name}
                </option>
              ))}
            </select>
          </div>
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Versi</label>
          <input required value={version} onChange={(e) => setVersion(e.target.value)} className={inputCls} />
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">File Firmware</label>
          <input
            required
            type="file"
            onChange={(e) => setFile(e.target.files?.[0] ?? null)}
            className={`${inputCls} file:mr-3 file:rounded-md file:border-0 file:bg-slate-900 file:px-3 file:py-1.5 file:text-xs file:text-white dark:file:bg-slate-100 dark:file:text-slate-900`}
          />
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Release Notes (opsional)</label>
          <textarea value={releaseNotes} onChange={(e) => setReleaseNotes(e.target.value)} rows={2} className={inputCls} />
        </div>
        {error && <p className="text-sm text-red-600">{error}</p>}
        <button type="submit" disabled={uploadMutation.isPending} className={`${primaryBtnCls} w-full`}>
          Daftarkan
        </button>
      </form>
    </Modal>
  )
}
