import { useState, type FormEvent } from 'react'
import { HardDrive, Plus } from 'lucide-react'
import { EmptyState } from '../components/EmptyState'
import { Modal } from '../components/Modal'
import { PageSpinner } from '../components/Spinner'
import { useAuth } from '../lib/auth'
import { ApiError } from '../lib/api'
import { useDeviceModels, useFirmwareList, useUploadFirmware, useVendors, type UploadFirmwareInput } from '../lib/hooks'
import { formatDateTime } from '../lib/format'

const inputCls =
  'w-full rounded-lg border border-slate-300 px-3 py-2 text-sm text-slate-900 outline-none transition-colors focus:border-slate-500 focus:ring-1 focus:ring-slate-500 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-100'
const primaryBtnCls =
  'flex items-center justify-center gap-2 rounded-lg bg-slate-900 px-3 py-2 text-sm font-medium text-white transition-colors hover:bg-slate-800 disabled:cursor-not-allowed disabled:opacity-60 dark:bg-slate-100 dark:text-slate-900 dark:hover:bg-white'

export function FirmwarePage() {
  const { hasRole } = useAuth()
  const canManage = hasRole('ADMIN')
  const { data: vendorsResp } = useVendors()
  const vendors = vendorsResp?.data ?? []
  const [vendorId, setVendorId] = useState<string>('')
  const [showUpload, setShowUpload] = useState(false)

  const { data, isLoading } = useFirmwareList(vendorId ? Number(vendorId) : undefined)
  const files = data?.data ?? []

  return (
    <div className="mx-auto max-w-7xl px-6 py-8">
      <div className="mb-6 flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold text-slate-900 dark:text-slate-100">Firmware</h1>
          <p className="mt-0.5 text-sm text-slate-500 dark:text-slate-400">Katalog firmware per vendor — penjadwalan upgrade dilakukan dari halaman detail device</p>
        </div>
        {canManage && (
          <button onClick={() => setShowUpload(true)} className={primaryBtnCls}>
            <Plus className="h-4 w-4" /> Daftarkan Firmware
          </button>
        )}
      </div>

      <div className="mb-4 max-w-xs">
        <select value={vendorId} onChange={(e) => setVendorId(e.target.value)} className={inputCls}>
          <option value="">Pilih vendor untuk melihat firmware...</option>
          {vendors.map((v) => (
            <option key={v.id} value={v.id}>
              {v.name}
            </option>
          ))}
        </select>
      </div>

      <div className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm dark:border-slate-800 dark:bg-slate-900">
        {!vendorId ? (
          <EmptyState icon={HardDrive} title="Pilih vendor" description="Firmware dikelompokkan per vendor — pilih salah satu di atas." />
        ) : isLoading ? (
          <PageSpinner />
        ) : files.length === 0 ? (
          <EmptyState icon={HardDrive} title="Belum ada firmware terdaftar" description="File firmware yang diupload di sini disimpan di object storage (MinIO)." />
        ) : (
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
        )}
      </div>

      {showUpload && <UploadFirmwareModal defaultVendorId={vendorId} onClose={() => setShowUpload(false)} />}
    </div>
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
