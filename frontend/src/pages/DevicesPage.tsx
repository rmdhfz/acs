import { useEffect, useMemo, useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  CheckCircle2,
  ChevronLeft,
  ChevronRight,
  Clock3,
  HardDrive,
  Power,
  Radio,
  RefreshCw,
  Router,
  Search,
  Sparkles,
  Wifi,
  WifiOff,
  X,
  XCircle,
} from 'lucide-react'
import { StatCard } from '../components/StatCard'
import { StatusBadge } from '../components/StatusBadge'
import { EmptyState } from '../components/EmptyState'
import { Modal } from '../components/Modal'
import { useAuth } from '../lib/auth'
import {
  findRefById,
  findRefIdByCode,
  useApplyProfile,
  useCreateTask,
  useDevices,
  useFirmwareList,
  usePendingTaskCount,
  useProvisioningProfiles,
  useRefs,
  useScheduleFirmwareUpgrade,
  useVendors,
} from '../lib/hooks'
import { formatRelativeTime } from '../lib/format'
import type { Device } from '../lib/types'

const PAGE_SIZE = 20
const inputCls =
  'w-full rounded-lg border border-slate-300 px-3 py-2 text-sm text-slate-900 outline-none transition-colors focus:border-slate-500 focus:ring-1 focus:ring-slate-500'
const primaryBtnCls =
  'flex items-center justify-center gap-2 rounded-lg bg-slate-900 px-3 py-2 text-sm font-medium text-white transition-colors hover:bg-slate-800 disabled:cursor-not-allowed disabled:opacity-60'

export function DevicesPage() {
  const navigate = useNavigate()
  const { hasRole } = useAuth()
  const canBulkAct = hasRole('ADMIN', 'NOC')
  const [search, setSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState<string>('')
  const [vendorFilter, setVendorFilter] = useState<string>('')
  const [page, setPage] = useState(1)
  const [selected, setSelected] = useState<Set<number>>(new Set())
  const [bulkAction, setBulkAction] = useState<'profile' | 'firmware' | 'reboot' | null>(null)

  const { data: statusRefs } = useRefs('ref_device_status')
  const { data: vendorsResp } = useVendors()
  const vendors = vendorsResp?.data ?? []

  const onlineStatusId = findRefIdByCode(statusRefs, 'ONLINE')
  const offlineStatusId = findRefIdByCode(statusRefs, 'OFFLINE')

  const { data: taskStatusRefs } = useRefs('ref_task_status')
  const pendingId = findRefIdByCode(taskStatusRefs, 'PENDING')

  const filters = useMemo(
    () => ({
      search: search.trim() || undefined,
      device_status_id: statusFilter ? Number(statusFilter) : undefined,
      vendor_id: vendorFilter ? Number(vendorFilter) : undefined,
      page,
      page_size: PAGE_SIZE,
    }),
    [search, statusFilter, vendorFilter, page],
  )

  const { data: devicesResp, isLoading, isFetching, dataUpdatedAt } = useDevices(filters)
  const { data: totalResp } = useDevices({ page_size: 1 })
  const { data: onlineResp } = useDevices({ device_status_id: onlineStatusId, page_size: 1 })
  const { data: offlineResp } = useDevices({ device_status_id: offlineStatusId, page_size: 1 })
  const pendingTasks = usePendingTaskCount(pendingId)

  const devices = devicesResp?.data ?? []
  const total = devicesResp?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))
  const selectedDevices = devices.filter((d) => selected.has(d.id))

  // Selection dibatasi ke halaman/filter yang sedang tampil supaya tidak ada
  // device "terpilih" secara tidak kasat mata dari filter/halaman sebelumnya.
  useEffect(() => {
    setSelected(new Set())
  }, [search, statusFilter, vendorFilter, page])

  function vendorName(vendorId: number | null) {
    if (vendorId == null) return '-'
    return vendors.find((v) => v.id === vendorId)?.name ?? `#${vendorId}`
  }

  function resetToFirstPage() {
    setPage(1)
  }

  function toggleSelected(id: number) {
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  function toggleSelectAll() {
    setSelected((prev) => (prev.size === devices.length ? new Set() : new Set(devices.map((d) => d.id))))
  }

  return (
    <div className="mx-auto max-w-7xl px-6 py-8">
      <div className="mb-6 flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold text-slate-900">Devices</h1>
          <p className="mt-0.5 text-sm text-slate-500">Monitor status koneksi perangkat CPE secara real-time</p>
        </div>
        <div className="flex items-center gap-2 text-xs text-slate-400">
          <span className="relative flex h-2 w-2">
            <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-emerald-400 opacity-75" />
            <span className="relative inline-flex h-2 w-2 rounded-full bg-emerald-500" />
          </span>
          Live
          {dataUpdatedAt > 0 && <span>· diperbarui {formatRelativeTime(new Date(dataUpdatedAt).toISOString())}</span>}
          {isFetching && <RefreshCw className="h-3.5 w-3.5 animate-spin text-slate-400" />}
        </div>
      </div>

      <div className="mb-6 grid grid-cols-2 gap-4 lg:grid-cols-4">
        <StatCard label="Total Device" value={totalResp?.total ?? 0} icon={Router} tone="default" loading={!totalResp} />
        <StatCard label="Online" value={onlineResp?.total ?? 0} icon={Wifi} tone="emerald" loading={!onlineResp} />
        <StatCard label="Offline" value={offlineResp?.total ?? 0} icon={WifiOff} tone="red" loading={!offlineResp} />
        <StatCard
          label="Task Pending"
          value={pendingTasks.data ?? 0}
          icon={Clock3}
          tone="amber"
          loading={pendingTasks.isLoading}
        />
      </div>

      <div className="mb-4 flex flex-wrap items-center gap-3">
        <div className="relative flex-1 min-w-[220px]">
          <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-slate-400" />
          <input
            type="text"
            placeholder="Cari serial number atau MAC address..."
            value={search}
            onChange={(e) => {
              setSearch(e.target.value)
              resetToFirstPage()
            }}
            className="w-full rounded-lg border border-slate-300 bg-white py-2 pl-9 pr-3 text-sm text-slate-900 outline-none transition-colors focus:border-slate-500 focus:ring-1 focus:ring-slate-500"
          />
        </div>
        <select
          value={statusFilter}
          onChange={(e) => {
            setStatusFilter(e.target.value)
            resetToFirstPage()
          }}
          className="rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm text-slate-700 outline-none focus:border-slate-500 focus:ring-1 focus:ring-slate-500"
        >
          <option value="">Semua status</option>
          {statusRefs?.map((s) => (
            <option key={s.id} value={s.id}>
              {s.name}
            </option>
          ))}
        </select>
        <select
          value={vendorFilter}
          onChange={(e) => {
            setVendorFilter(e.target.value)
            resetToFirstPage()
          }}
          className="rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm text-slate-700 outline-none focus:border-slate-500 focus:ring-1 focus:ring-slate-500"
        >
          <option value="">Semua vendor</option>
          {vendors.map((v) => (
            <option key={v.id} value={v.id}>
              {v.name}
            </option>
          ))}
        </select>
      </div>

      {canBulkAct && selected.size > 0 && (
        <div className="mb-4 flex flex-wrap items-center gap-3 rounded-xl border border-slate-300 bg-slate-900 px-4 py-2.5 text-white">
          <span className="text-sm font-medium">{selected.size} device dipilih</span>
          <div className="ml-auto flex flex-wrap items-center gap-2">
            <button
              onClick={() => setBulkAction('profile')}
              className="flex items-center gap-1.5 rounded-lg bg-white/10 px-3 py-1.5 text-sm font-medium transition-colors hover:bg-white/20"
            >
              <Sparkles className="h-3.5 w-3.5" /> Terapkan Profile
            </button>
            <button
              onClick={() => setBulkAction('firmware')}
              className="flex items-center gap-1.5 rounded-lg bg-white/10 px-3 py-1.5 text-sm font-medium transition-colors hover:bg-white/20"
            >
              <HardDrive className="h-3.5 w-3.5" /> Firmware Upgrade
            </button>
            <button
              onClick={() => setBulkAction('reboot')}
              className="flex items-center gap-1.5 rounded-lg bg-white/10 px-3 py-1.5 text-sm font-medium transition-colors hover:bg-white/20"
            >
              <Power className="h-3.5 w-3.5" /> Reboot
            </button>
            <button
              onClick={() => setSelected(new Set())}
              className="flex h-7 w-7 items-center justify-center rounded-md text-slate-300 transition-colors hover:bg-white/10 hover:text-white"
              title="Batal pilih"
            >
              <X className="h-4 w-4" />
            </button>
          </div>
        </div>
      )}

      <div className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm">
        {isLoading ? (
          <div className="divide-y divide-slate-100">
            {Array.from({ length: 6 }).map((_, i) => (
              <div key={i} className="flex items-center gap-4 px-5 py-4">
                <div className="h-5 w-20 animate-pulse rounded-full bg-slate-100" />
                <div className="h-4 w-28 animate-pulse rounded bg-slate-100" />
                <div className="h-4 flex-1 animate-pulse rounded bg-slate-100" />
              </div>
            ))}
          </div>
        ) : devices.length === 0 ? (
          <EmptyState
            icon={Radio}
            title="Belum ada device yang terhubung"
            description="Device akan otomatis muncul di sini begitu CPE mengirim Inform pertama ke endpoint CWMP ACS."
          />
        ) : (
          <>
            <table className="w-full text-left text-sm">
              <thead>
                <tr className="border-b border-slate-200 bg-slate-50 text-xs font-medium uppercase tracking-wide text-slate-500">
                  {canBulkAct && (
                    <th className="w-10 px-5 py-3">
                      <input
                        type="checkbox"
                        checked={devices.length > 0 && selected.size === devices.length}
                        onChange={toggleSelectAll}
                        onClick={(e) => e.stopPropagation()}
                        className="h-4 w-4 rounded border-slate-300"
                      />
                    </th>
                  )}
                  <th className="px-5 py-3">Status</th>
                  <th className="px-5 py-3">Vendor</th>
                  <th className="px-5 py-3">Serial Number</th>
                  <th className="px-5 py-3">MAC Address</th>
                  <th className="px-5 py-3">IP Address</th>
                  <th className="px-5 py-3">Firmware</th>
                  <th className="px-5 py-3">Terakhir Terhubung</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100">
                {devices.map((d) => {
                  const status = findRefById(statusRefs, d.device_status_id)
                  return (
                    <tr
                      key={d.id}
                      onClick={() => navigate(`/devices/${d.id}`)}
                      className="cursor-pointer transition-colors hover:bg-slate-50"
                    >
                      {canBulkAct && (
                        <td className="px-5 py-3.5" onClick={(e) => e.stopPropagation()}>
                          <input
                            type="checkbox"
                            checked={selected.has(d.id)}
                            onChange={() => toggleSelected(d.id)}
                            className="h-4 w-4 rounded border-slate-300"
                          />
                        </td>
                      )}
                      <td className="px-5 py-3.5">
                        <StatusBadge code={status?.code} label={status?.name ?? '-'} pulse={status?.code === 'ONLINE'} />
                      </td>
                      <td className="px-5 py-3.5 text-slate-600">{vendorName(d.vendor_id)}</td>
                      <td className="px-5 py-3.5 font-mono text-xs text-slate-900">{d.serial_number}</td>
                      <td className="px-5 py-3.5 font-mono text-xs text-slate-500">{d.mac_address ?? '-'}</td>
                      <td className="px-5 py-3.5 font-mono text-xs text-slate-500">{d.ip_address ?? '-'}</td>
                      <td className="px-5 py-3.5 text-slate-500">{d.software_version ?? '-'}</td>
                      <td className="px-5 py-3.5 text-slate-500">{formatRelativeTime(d.last_inform_at)}</td>
                    </tr>
                  )
                })}
              </tbody>
            </table>

            <div className="flex items-center justify-between border-t border-slate-200 px-5 py-3 text-sm text-slate-500">
              <span>
                Menampilkan {devices.length} dari {total} device
              </span>
              <div className="flex items-center gap-1">
                <button
                  onClick={() => setPage((p) => Math.max(1, p - 1))}
                  disabled={page <= 1}
                  className="flex h-8 w-8 items-center justify-center rounded-lg border border-slate-200 text-slate-500 transition-colors hover:bg-slate-50 disabled:cursor-not-allowed disabled:opacity-40"
                >
                  <ChevronLeft className="h-4 w-4" />
                </button>
                <span className="px-2 text-xs tabular-nums">
                  {page} / {totalPages}
                </span>
                <button
                  onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
                  disabled={page >= totalPages}
                  className="flex h-8 w-8 items-center justify-center rounded-lg border border-slate-200 text-slate-500 transition-colors hover:bg-slate-50 disabled:cursor-not-allowed disabled:opacity-40"
                >
                  <ChevronRight className="h-4 w-4" />
                </button>
              </div>
            </div>
          </>
        )}
      </div>

      {bulkAction === 'profile' && (
        <BulkApplyProfileModal devices={selectedDevices} onClose={() => setBulkAction(null)} onDone={() => setSelected(new Set())} />
      )}
      {bulkAction === 'firmware' && (
        <BulkFirmwareModal devices={selectedDevices} onClose={() => setBulkAction(null)} onDone={() => setSelected(new Set())} />
      )}
      {bulkAction === 'reboot' && (
        <BulkRebootModal devices={selectedDevices} onClose={() => setBulkAction(null)} onDone={() => setSelected(new Set())} />
      )}
    </div>
  )
}

// ---- Bulk actions ----

type BulkItemStatus = 'pending' | 'running' | 'success' | 'error' | 'skipped'
interface BulkItem {
  device: Device
  status: BulkItemStatus
  message?: string
}

function BulkResultList({ items }: { items: BulkItem[] }) {
  return (
    <ul className="max-h-64 divide-y divide-slate-100 overflow-y-auto rounded-lg border border-slate-200">
      {items.map((item) => (
        <li key={item.device.id} className="flex items-center justify-between gap-3 px-3 py-2 text-sm">
          <div className="min-w-0">
            <p className="truncate font-mono text-xs text-slate-700">{item.device.serial_number}</p>
            {item.message && <p className="truncate text-xs text-slate-400">{item.message}</p>}
          </div>
          {item.status === 'pending' && <span className="shrink-0 text-xs text-slate-400">Menunggu</span>}
          {item.status === 'running' && <RefreshCw className="h-4 w-4 shrink-0 animate-spin text-slate-400" />}
          {item.status === 'success' && <CheckCircle2 className="h-4 w-4 shrink-0 text-emerald-500" />}
          {item.status === 'error' && <XCircle className="h-4 w-4 shrink-0 text-red-500" />}
          {item.status === 'skipped' && <span className="shrink-0 text-xs text-amber-600">Dilewati</span>}
        </li>
      ))}
    </ul>
  )
}

function BulkApplyProfileModal({ devices, onClose, onDone }: { devices: Device[]; onClose: () => void; onDone: () => void }) {
  const { data: profilesResp } = useProvisioningProfiles()
  const profiles = (profilesResp?.data ?? []).filter((p) => p.is_active)
  const [profileId, setProfileId] = useState('')
  const [items, setItems] = useState<BulkItem[] | null>(null)
  const applyMutation = useApplyProfile()

  const selectedProfile = profiles.find((p) => p.id === Number(profileId))

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    if (!selectedProfile) return
    const initial: BulkItem[] = devices.map((device) => {
      const vendorMismatch = selectedProfile.vendor_id != null && device.vendor_id !== selectedProfile.vendor_id
      return { device, status: vendorMismatch ? 'skipped' : 'pending', message: vendorMismatch ? 'Vendor tidak cocok dengan profil' : undefined }
    })
    setItems(initial)

    for (const item of initial) {
      if (item.status === 'skipped') continue
      setItems((prev) => prev!.map((it) => (it.device.id === item.device.id ? { ...it, status: 'running' } : it)))
      try {
        await applyMutation.mutateAsync({ deviceId: item.device.id, profileId: selectedProfile.id })
        setItems((prev) => prev!.map((it) => (it.device.id === item.device.id ? { ...it, status: 'success' } : it)))
      } catch {
        setItems((prev) => prev!.map((it) => (it.device.id === item.device.id ? { ...it, status: 'error', message: 'Gagal' } : it)))
      }
    }
    onDone()
  }

  const isRunning = items !== null && items.some((i) => i.status === 'pending' || i.status === 'running')

  return (
    <Modal title={`Terapkan Provisioning Profile ke ${devices.length} Device`} onClose={onClose}>
      {items === null ? (
        <form onSubmit={handleSubmit} className="space-y-3">
          <p className="text-xs text-slate-500">
            Device dengan vendor berbeda dari profil (bila profil dibatasi ke vendor tertentu) akan otomatis dilewati.
          </p>
          <select required value={profileId} onChange={(e) => setProfileId(e.target.value)} className={inputCls}>
            <option value="">Pilih profil...</option>
            {profiles.map((p) => (
              <option key={p.id} value={p.id}>
                {p.name}
              </option>
            ))}
          </select>
          <button type="submit" disabled={!profileId} className={`${primaryBtnCls} w-full`}>
            Terapkan ke {devices.length} Device
          </button>
        </form>
      ) : (
        <div className="space-y-3">
          <BulkResultList items={items} />
          {!isRunning && (
            <button onClick={onClose} className={`${primaryBtnCls} w-full`}>
              Selesai
            </button>
          )}
        </div>
      )}
    </Modal>
  )
}

function BulkFirmwareModal({ devices, onClose, onDone }: { devices: Device[]; onClose: () => void; onDone: () => void }) {
  const { data: vendorsResp } = useVendors()
  const vendors = vendorsResp?.data ?? []
  const vendorIdsInSelection = [...new Set(devices.map((d) => d.vendor_id).filter((v): v is number => v != null))]

  const [vendorId, setVendorId] = useState(vendorIdsInSelection.length === 1 ? String(vendorIdsInSelection[0]) : '')
  const [firmwareId, setFirmwareId] = useState('')
  const [items, setItems] = useState<BulkItem[] | null>(null)
  const { data: firmwareResp } = useFirmwareList(vendorId ? Number(vendorId) : undefined)
  const firmwareOptions = firmwareResp?.data ?? []
  const scheduleMutation = useScheduleFirmwareUpgrade()

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    if (!firmwareId || !vendorId) return
    const targetVendorId = Number(vendorId)
    const initial: BulkItem[] = devices.map((device) => {
      const mismatch = device.vendor_id !== targetVendorId
      return { device, status: mismatch ? 'skipped' : 'pending', message: mismatch ? 'Vendor device tidak cocok' : undefined }
    })
    setItems(initial)

    for (const item of initial) {
      if (item.status === 'skipped') continue
      setItems((prev) => prev!.map((it) => (it.device.id === item.device.id ? { ...it, status: 'running' } : it)))
      try {
        await scheduleMutation.mutateAsync({ deviceId: item.device.id, firmwareId: Number(firmwareId) })
        setItems((prev) => prev!.map((it) => (it.device.id === item.device.id ? { ...it, status: 'success' } : it)))
      } catch {
        setItems((prev) => prev!.map((it) => (it.device.id === item.device.id ? { ...it, status: 'error', message: 'Gagal' } : it)))
      }
    }
    onDone()
  }

  const isRunning = items !== null && items.some((i) => i.status === 'pending' || i.status === 'running')

  return (
    <Modal title={`Jadwalkan Firmware Upgrade untuk ${devices.length} Device`} onClose={onClose}>
      {items === null ? (
        <form onSubmit={handleSubmit} className="space-y-3">
          <p className="text-xs text-slate-500">
            Firmware spesifik per vendor — pilih satu vendor dulu. Device dari vendor lain di seleksi ini akan dilewati.
          </p>
          <select
            required
            value={vendorId}
            onChange={(e) => {
              setVendorId(e.target.value)
              setFirmwareId('')
            }}
            className={inputCls}
          >
            <option value="">Pilih vendor...</option>
            {vendors
              .filter((v) => vendorIdsInSelection.includes(v.id))
              .map((v) => (
                <option key={v.id} value={v.id}>
                  {v.name} ({devices.filter((d) => d.vendor_id === v.id).length} device)
                </option>
              ))}
          </select>
          <select required value={firmwareId} onChange={(e) => setFirmwareId(e.target.value)} disabled={!vendorId} className={inputCls}>
            <option value="">{vendorId ? 'Pilih firmware...' : 'Pilih vendor dulu'}</option>
            {firmwareOptions.map((f) => (
              <option key={f.id} value={f.id}>
                {f.version} — {f.file_name}
              </option>
            ))}
          </select>
          <button type="submit" disabled={!firmwareId} className={`${primaryBtnCls} w-full`}>
            Jadwalkan Upgrade
          </button>
        </form>
      ) : (
        <div className="space-y-3">
          <BulkResultList items={items} />
          {!isRunning && (
            <button onClick={onClose} className={`${primaryBtnCls} w-full`}>
              Selesai
            </button>
          )}
        </div>
      )}
    </Modal>
  )
}

function BulkRebootModal({ devices, onClose, onDone }: { devices: Device[]; onClose: () => void; onDone: () => void }) {
  const [items, setItems] = useState<BulkItem[] | null>(null)
  const createTaskMutation = useCreateTask()

  async function handleConfirm() {
    const initial: BulkItem[] = devices.map((device) => ({ device, status: 'pending' }))
    setItems(initial)
    for (const item of initial) {
      setItems((prev) => prev!.map((it) => (it.device.id === item.device.id ? { ...it, status: 'running' } : it)))
      try {
        await createTaskMutation.mutateAsync({ device_id: item.device.id, task_type: 'REBOOT', priority: 2 })
        setItems((prev) => prev!.map((it) => (it.device.id === item.device.id ? { ...it, status: 'success' } : it)))
      } catch {
        setItems((prev) => prev!.map((it) => (it.device.id === item.device.id ? { ...it, status: 'error', message: 'Gagal' } : it)))
      }
    }
    onDone()
  }

  const isRunning = items !== null && items.some((i) => i.status === 'pending' || i.status === 'running')

  return (
    <Modal title={`Reboot ${devices.length} Device`} onClose={onClose}>
      {items === null ? (
        <div className="space-y-3">
          <p className="text-sm text-slate-600">
            Task reboot akan diantrekan (prioritas tinggi) ke {devices.length} device terpilih. Device yang sedang
            offline akan reboot pada sesi Inform berikutnya atau saat Connection Request berhasil.
          </p>
          <button onClick={handleConfirm} className={`${primaryBtnCls} w-full`}>
            <Power className="h-4 w-4" /> Reboot {devices.length} Device
          </button>
        </div>
      ) : (
        <div className="space-y-3">
          <BulkResultList items={items} />
          {!isRunning && (
            <button onClick={onClose} className={`${primaryBtnCls} w-full`}>
              Selesai
            </button>
          )}
        </div>
      )}
    </Modal>
  )
}
