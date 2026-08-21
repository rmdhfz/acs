import { useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { ChevronLeft, ChevronRight, Radio, RefreshCw, Router, Search, Wifi, WifiOff, Clock3 } from 'lucide-react'
import { StatCard } from '../components/StatCard'
import { StatusBadge } from '../components/StatusBadge'
import { EmptyState } from '../components/EmptyState'
import { findRefById, findRefIdByCode, useDevices, usePendingTaskCount, useRefs, useVendors } from '../lib/hooks'
import { formatRelativeTime } from '../lib/format'

const PAGE_SIZE = 20

export function DevicesPage() {
  const navigate = useNavigate()
  const [search, setSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState<string>('')
  const [vendorFilter, setVendorFilter] = useState<string>('')
  const [page, setPage] = useState(1)

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

  function vendorName(vendorId: number | null) {
    if (vendorId == null) return '-'
    return vendors.find((v) => v.id === vendorId)?.name ?? `#${vendorId}`
  }

  function resetToFirstPage() {
    setPage(1)
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
    </div>
  )
}
