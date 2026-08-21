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
  'w-full rounded-lg border border-slate-300 px-3 py-2 text-sm text-slate-900 outline-none transition-colors focus:border-slate-500 focus:ring-1 focus:ring-slate-500'
const primaryBtnCls =
  'flex items-center justify-center gap-2 rounded-lg bg-slate-900 px-3 py-2 text-sm font-medium text-white transition-colors hover:bg-slate-800 disabled:cursor-not-allowed disabled:opacity-60'

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
          <h1 className="text-xl font-semibold text-slate-900">Firmware</h1>
          <p className="mt-0.5 text-sm text-slate-500">Katalog firmware per vendor — penjadwalan upgrade dilakukan dari halaman detail device</p>
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

      <div className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm">
        {!vendorId ? (
          <EmptyState icon={HardDrive} title="Pilih vendor" description="Firmware dikelompokkan per vendor — pilih salah satu di atas." />
        ) : isLoading ? (
          <PageSpinner />
        ) : files.length === 0 ? (
          <EmptyState icon={HardDrive} title="Belum ada firmware terdaftar" description="Metadata firmware (bukan file fisik) yang sudah ada di object storage/filesystem dicatat di sini." />
        ) : (
          <table className="w-full text-left text-sm">
            <thead>
              <tr className="border-b border-slate-200 bg-slate-50 text-xs font-medium uppercase tracking-wide text-slate-500">
                <th className="px-5 py-3">Versi</th>
                <th className="px-5 py-3">File</th>
                <th className="px-5 py-3">Checksum</th>
                <th className="px-5 py-3">Ukuran</th>
                <th className="px-5 py-3">Didaftarkan</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100">
              {files.map((f) => (
                <tr key={f.id}>
                  <td className="px-5 py-3.5 font-medium text-slate-900">{f.version}</td>
                  <td className="px-5 py-3.5 font-mono text-xs text-slate-600">{f.file_name}</td>
                  <td className="px-5 py-3.5 max-w-[220px] truncate font-mono text-xs text-slate-400" title={f.checksum_sha256 ?? undefined}>
                    {f.checksum_sha256 ?? '-'}
                  </td>
                  <td className="px-5 py-3.5 text-slate-500">{f.file_size_bytes ? `${(f.file_size_bytes / 1_000_000).toFixed(1)} MB` : '-'}</td>
                  <td className="px-5 py-3.5 text-slate-500">{formatDateTime(f.created_at)}</td>
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
  const [fileName, setFileName] = useState('')
  const [filePath, setFilePath] = useState('')
  const [checksum, setChecksum] = useState('')
  const [releaseNotes, setReleaseNotes] = useState('')
  const [error, setError] = useState<string | null>(null)
  const { data: models } = useDeviceModels(vendorId ? Number(vendorId) : undefined)
  const uploadMutation = useUploadFirmware()

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    const input: UploadFirmwareInput = {
      vendor_id: Number(vendorId),
      device_model_id: deviceModelId ? Number(deviceModelId) : undefined,
      version,
      file_name: fileName,
      file_path: filePath,
      checksum_sha256: checksum || undefined,
      release_notes: releaseNotes || undefined,
    }
    try {
      await uploadMutation.mutateAsync(input)
      onClose()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Gagal mendaftarkan firmware')
    }
  }

  return (
    <Modal title="Daftarkan Firmware" onClose={onClose}>
      <form onSubmit={handleSubmit} className="space-y-3">
        <p className="text-xs text-slate-500">
          File firmware harus sudah tersedia di object storage/filesystem — form ini hanya mencatat metadatanya (TECH.md §12).
        </p>
        <div className="grid grid-cols-2 gap-3">
          <div>
            <label className="mb-1 block text-xs font-medium text-slate-600">Vendor</label>
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
            <label className="mb-1 block text-xs font-medium text-slate-600">Device Model (opsional)</label>
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
          <label className="mb-1 block text-xs font-medium text-slate-600">Versi</label>
          <input required value={version} onChange={(e) => setVersion(e.target.value)} className={inputCls} />
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-slate-600">Nama File</label>
          <input required value={fileName} onChange={(e) => setFileName(e.target.value)} className={inputCls} />
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-slate-600">Path File (object storage / filesystem)</label>
          <input required value={filePath} onChange={(e) => setFilePath(e.target.value)} className={`${inputCls} font-mono text-xs`} />
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-slate-600">Checksum SHA-256 (opsional)</label>
          <input value={checksum} onChange={(e) => setChecksum(e.target.value)} className={`${inputCls} font-mono text-xs`} />
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-slate-600">Release Notes (opsional)</label>
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
