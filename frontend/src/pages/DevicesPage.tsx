import { useI18n } from '../lib/i18n'
import { useEffect, useMemo, useState, type FormEvent } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import {
  CheckCircle2,
  ChevronLeft,
  ChevronRight,
  Clock3,
  HardDrive,
  Pencil,
  Plus,
  Power,
  Radio,
  RefreshCw,
  Router,
  Search,
  SlidersHorizontal,
  Sparkles,
  Trash2,
  Upload,
  Wifi,
  WifiOff,
  X,
  XCircle,
} from 'lucide-react'
import { StatCard } from '../components/StatCard'
import { StatusBadge } from '../components/StatusBadge'
import { EmptyState } from '../components/EmptyState'
import { Modal } from '../components/Modal'
import { DeviceFormModal } from '../components/DeviceFormModal'
import { ExportMenu } from '../components/ExportMenu'
import { OpticalCell } from '../components/OpticalCell'
import { ImportWizard } from '../components/ImportWizard'
import { deviceAdapter, deviceExportColumns, loadDeviceExportRows } from '../imports/devices'
import { useQueryClient } from '@tanstack/react-query'
import { useConfirm } from '../lib/confirm'
import { useToast } from '../lib/toast'
import { useAuth } from '../lib/auth'
import {
  findRefById,
  findRefIdByCode,
  useApplyProfile,
  useCreateTask,
  useCurrentTenant,
  useDeleteDevice,
  useDevices,
  useRefreshDeviceSummary,
  useFirmwareList,
  usePendingTaskCount,
  useProvisioningProfiles,
  useRefs,
  useScheduleFirmwareUpgrade,
  useTags,
  useTenants,
  useVendors,
  type DeviceFilters,
} from '../lib/hooks'
import { formatRelativeTime } from '../lib/format'
import { useStatusParam } from '../lib/useStatusParam'
import type { Device } from '../lib/types'

const PAGE_SIZE = 20
// Nilai khusus filter vendor: perangkat yang vendornya belum terpetakan ("Belum dikenal").
const NO_VENDOR = 'none'
const inputCls =
  'w-full rounded-lg border border-slate-300 px-3 py-2 text-sm text-slate-900 outline-none transition-colors focus:border-indigo-600 focus:ring-1 focus:ring-indigo-600 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-100'
const primaryBtnCls =
  'flex items-center justify-center gap-2 rounded-full bg-indigo-600 px-3 py-2 text-sm font-medium text-white transition-colors hover:bg-indigo-700 disabled:cursor-not-allowed disabled:opacity-60 dark:bg-indigo-300 dark:text-slate-900 dark:hover:bg-indigo-200'

export function DevicesPage() {
  const { t: tl } = useI18n()
  const navigate = useNavigate()
  const { hasRole } = useAuth()
  const canBulkAct = hasRole('ADMIN', 'NOC')
  const isAdmin = hasRole('ADMIN')
  const isSuper = hasRole('SUPERADMIN')
  const confirm = useConfirm()
  const toast = useToast()
  const queryClient = useQueryClient()
  const deleteMut = useDeleteDevice()
  const refreshSummary = useRefreshDeviceSummary()
  const [showCreate, setShowCreate] = useState(false)
  const [showImport, setShowImport] = useState(false)
  const [editDevice, setEditDevice] = useState<Device | null>(null)
  const [search, setSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState<string>('')
  const [vendorFilter, setVendorFilter] = useState<string>('')
  const [tagFilter, setTagFilter] = useState<string>('')
  // Query bebas: urutan + kondisi nilai parameter (satu per baris: nama~op~nilai; lihat GET /devices).
  const [sortKey, setSortKey] = useState('')
  const [showAdvanced, setShowAdvanced] = useState(false)
  const [paramText, setParamText] = useState('')
  // Deep-link dari dashboard: /devices?optical_level=bad (nilai tak dikenal diabaikan).
  const [urlParams] = useSearchParams()
  const [opticalFilter, setOpticalFilter] = useState<string>(() => {
    const v = urlParams.get('optical_level')
    return v === 'ok' || v === 'warn' || v === 'bad' || v === 'unknown' ? v : ''
  })
  const [page, setPage] = useState(1)
  const [selected, setSelected] = useState<Set<number>>(new Set())
  const [bulkAction, setBulkAction] = useState<'profile' | 'firmware' | 'reboot' | null>(null)

  const { data: statusRefs } = useRefs('ref_device_status')
  useStatusParam(statusRefs, setStatusFilter)
  const { data: vendorsResp } = useVendors()
  const vendors = vendorsResp?.data ?? []
  const { data: tagsResp } = useTags({ pageSize: 100 })
  const tags = tagsResp?.data ?? []
  const { data: tenantsResp } = useTenants(isSuper)
  const tenants = isSuper ? (tenantsResp?.data ?? []) : []
  const { data: currentTenant } = useCurrentTenant()

  const onlineStatusId = findRefIdByCode(statusRefs, 'ONLINE')
  const offlineStatusId = findRefIdByCode(statusRefs, 'OFFLINE')

  const { data: taskStatusRefs } = useRefs('ref_task_status')
  const pendingId = findRefIdByCode(taskStatusRefs, 'PENDING')

  // Satu kondisi per baris; baris kosong diabaikan. Server membatasi 5 kondisi dan menolak yang tak sah (muncul sebagai galat).
  const paramConds = useMemo(
    () => paramText.split('\n').map((l) => l.trim()).filter(Boolean).slice(0, 5),
    [paramText],
  )

  // Filter aktif (tanpa paginasi) — dipakai daftar & ekspor agar keduanya selalu sama.
  const activeFilters = useMemo(
    (): DeviceFilters => ({
      search: search.trim() || undefined,
      device_status_id: statusFilter ? Number(statusFilter) : undefined,
      // "none" = vendor belum dikenal (vendor_id NULL), bukan id vendor.
      vendor_id: vendorFilter && vendorFilter !== NO_VENDOR ? Number(vendorFilter) : undefined,
      no_vendor: vendorFilter === NO_VENDOR ? true : undefined,
      tag_id: tagFilter ? Number(tagFilter) : undefined,
      optical_level: (opticalFilter || undefined) as DeviceFilters['optical_level'],
      sort: sortKey || undefined,
      param: paramConds.length ? paramConds : undefined,
    }),
    [search, statusFilter, vendorFilter, tagFilter, opticalFilter, sortKey, paramConds],
  )
  const filters = useMemo(() => ({ ...activeFilters, page, page_size: PAGE_SIZE }), [activeFilters, page])

  const { data: devicesResp, isLoading, isFetching, dataUpdatedAt } = useDevices(filters)
  const { data: totalResp } = useDevices({ page_size: 1 })
  const { data: onlineResp } = useDevices({ device_status_id: onlineStatusId, page_size: 1 })
  const { data: offlineResp } = useDevices({ device_status_id: offlineStatusId, page_size: 1 })
  const pendingTasks = usePendingTaskCount(pendingId)

  const devices = devicesResp?.data ?? []
  const total = devicesResp?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))
  const selectedDevices = devices.filter((d) => selected.has(d.id))

  // Antre pembacaan PPPoE & redaman: untuk perangkat terpilih, atau seluruh halaman bila tak ada yang dipilih.
  function handleRefreshSummary() {
    const targets = (selectedDevices.length > 0 ? selectedDevices : devices).slice(0, 100)
    if (targets.length === 0) return
    refreshSummary.mutate(
      targets.map((d) => d.id),
      {
        onSuccess: (r) => {
          const why = r.skipped.length > 0 ? ` · ${r.skipped.length} dilewati (${[...new Set(r.skipped.map((x) => x.reason))].join('; ')})` : ''
          toast.success(`${r.queued} perangkat diantre dibaca. Data muncul setelah perangkat Inform${why}.`, 'Berhasil')
          // Hasil datang saat perangkat Inform; segarkan daftar beberapa kali tanpa menunggu polling 60 dtk.
          for (const ms of [15_000, 45_000, 90_000]) setTimeout(() => void queryClient.invalidateQueries({ queryKey: ['devices'] }), ms)
        },
        onError: (e) => toast.error(e instanceof Error ? e.message : 'Gagal mengantre pembacaan data'),
      },
    )
  }

  // Selection dibatasi ke halaman/filter yang sedang tampil supaya tidak ada
  // device "terpilih" secara tidak kasat mata dari filter/halaman sebelumnya.
  useEffect(() => {
    setSelected(new Set())
  }, [search, statusFilter, vendorFilter, tagFilter, opticalFilter, sortKey, paramConds, page])

  async function handleDelete(d: Device) {
    const ok = await confirm({
      title: 'Hapus perangkat',
      message: (
        <>
          Perangkat <span className="font-mono">{d.serial_number}</span> akan dihapus dari daftar (soft delete) dan task yang masih antre dibatalkan.
          Bila CPE ini mengirim Inform lagi, ia didaftarkan ulang sebagai perangkat baru.
        </>
      ),
      confirmLabel: 'Hapus perangkat',
      tone: 'danger',
    })
    if (!ok) return
    try {
      await deleteMut.mutateAsync(d.id)
      toast.success(`Perangkat ${d.serial_number} dihapus.`)
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Gagal menghapus perangkat')
    }
  }

  async function handleBulkDelete() {
    const targets = selectedDevices
    const ok = await confirm({
      title: `Hapus ${targets.length} perangkat`,
      message: 'Perangkat terpilih akan dihapus dari daftar dan task antreannya dibatalkan. Perangkat yang Inform lagi didaftarkan ulang sebagai baru.',
      confirmLabel: 'Hapus semua',
      tone: 'danger',
    })
    if (!ok) return
    let okCount = 0
    let fail = 0
    for (const d of targets) {
      try {
        await deleteMut.mutateAsync(d.id)
        okCount++
      } catch {
        fail++
      }
    }
    setSelected(new Set())
    if (fail === 0) toast.success(`${okCount} perangkat dihapus.`)
    else toast.warning(`${okCount} dihapus, ${fail} gagal (cek hak akses/koneksi).`, 'Hapus massal selesai sebagian')
  }

  function vendorName(vendorId: number | null) {
    if (vendorId == null) return 'Belum dikenal'
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
          <h1 className="text-xl font-semibold text-slate-900 dark:text-slate-100">{tl('nav.devices')}</h1>
          <p className="mt-0.5 text-sm text-slate-500 dark:text-slate-400">Monitor status koneksi perangkat CPE secara real-time</p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          {canBulkAct && (
            <button
              type="button"
              onClick={handleRefreshSummary}
              disabled={refreshSummary.isPending || devices.length === 0}
              title="Antre pembacaan username PPPoE & level optik. Untuk perangkat yang dipilih, atau seluruh halaman ini bila tak ada yang dipilih."
              className="flex items-center gap-2 rounded-full border border-slate-300 bg-white px-3 py-2 text-sm font-medium text-slate-700 transition-colors hover:bg-slate-50 disabled:opacity-50 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-300 dark:hover:bg-slate-800"
            >
              <RefreshCw className={`h-4 w-4 ${refreshSummary.isPending ? 'animate-spin' : ''}`} />
              {selectedDevices.length > 0 ? `Segarkan data (${selectedDevices.length})` : 'Segarkan data'}
            </button>
          )}
          <ExportMenu
            filename="perangkat"
            columns={deviceExportColumns()}
            hint="Mengekspor semua perangkat sesuai filter yang aktif (bukan hanya halaman ini)."
            getRows={(ctx) =>
              loadDeviceExportRows(
                { ...activeFilters, no_vendor: activeFilters.no_vendor ? 'true' : undefined },
                { vendors, tenants, tenantName: currentTenant?.name, statuses: statusRefs ?? [] },
                ctx,
              )
            }
          />
          {isAdmin && (
            <>
              <button
                type="button"
                onClick={() => setShowImport(true)}
                className="flex items-center gap-2 rounded-full border border-slate-300 bg-white px-3 py-2 text-sm font-medium text-slate-700 transition-colors hover:bg-slate-50 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-300 dark:hover:bg-slate-800"
              >
                <Upload className="h-4 w-4" /> Impor
              </button>
              <button
                type="button"
                onClick={() => setShowCreate(true)}
                className="flex items-center gap-2 rounded-full bg-indigo-600 px-3.5 py-2 text-sm font-semibold text-white transition-colors hover:bg-indigo-500"
              >
                <Plus className="h-4 w-4" /> Tambah Perangkat
              </button>
            </>
          )}
        </div>
      </div>
      <div className="-mt-4 mb-6 flex items-center justify-end gap-2 text-xs text-slate-500 dark:text-slate-400">
        <div className="flex items-center gap-2">
          <span className="relative flex h-2 w-2">
            <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-emerald-400 opacity-75" />
            <span className="relative inline-flex h-2 w-2 rounded-full bg-emerald-500" />
          </span>
          Live
          {dataUpdatedAt > 0 && <span>· diperbarui {formatRelativeTime(new Date(dataUpdatedAt).toISOString())}</span>}
          {isFetching && <RefreshCw className="h-3.5 w-3.5 animate-spin text-slate-500 dark:text-slate-400" />}
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
          <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-slate-500 dark:text-slate-400" />
          <input maxLength={100}
            type="text"
            placeholder="Cari serial, MAC, model, atau username PPPoE..."
            value={search}
            onChange={(e) => {
              setSearch(e.target.value)
              resetToFirstPage()
            }}
            aria-label="Cari device berdasarkan serial number, MAC address, model, atau username PPPoE" className="w-full rounded-lg border border-slate-300 bg-white py-2 pl-9 pr-3 text-sm text-slate-900 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-100 outline-none transition-colors focus:border-indigo-600 focus:ring-1 focus:ring-indigo-600"
          />
        </div>
        <select
          value={statusFilter}
          aria-label="Filter status"
          onChange={(e) => {
            setStatusFilter(e.target.value)
            resetToFirstPage()
          }}
          className="min-w-[140px] flex-1 rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm text-slate-700 sm:flex-none dark:border-slate-700 dark:bg-slate-900 dark:text-slate-300 outline-none focus:border-indigo-600 focus:ring-1 focus:ring-indigo-600"
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
          aria-label="Filter vendor"
          onChange={(e) => {
            setVendorFilter(e.target.value)
            resetToFirstPage()
          }}
          className="min-w-[140px] flex-1 rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm text-slate-700 sm:flex-none dark:border-slate-700 dark:bg-slate-900 dark:text-slate-300 outline-none focus:border-indigo-600 focus:ring-1 focus:ring-indigo-600"
        >
          <option value="">Semua vendor</option>
          <option value={NO_VENDOR}>Belum dikenal</option>
          {vendors.map((v) => (
            <option key={v.id} value={v.id}>
              {v.name}
            </option>
          ))}
        </select>
        <select
          value={opticalFilter}
          aria-label="Filter redaman"
          onChange={(e) => {
            setOpticalFilter(e.target.value)
            resetToFirstPage()
          }}
          className="min-w-[140px] flex-1 rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm text-slate-700 sm:flex-none dark:border-slate-700 dark:bg-slate-900 dark:text-slate-300 outline-none focus:border-indigo-600 focus:ring-1 focus:ring-indigo-600"
        >
          <option value="">Semua redaman</option>
          <option value="bad">Kritis (&lt; -27 / &gt; -8 dBm)</option>
          <option value="warn">Waspada (-27 s.d. -25 dBm)</option>
          <option value="ok">Normal</option>
          <option value="unknown">Belum terbaca</option>
        </select>
        {tags.length > 0 && (
          <select
            value={tagFilter}
            aria-label="Filter tag"
            onChange={(e) => {
              setTagFilter(e.target.value)
              resetToFirstPage()
            }}
            className="min-w-[140px] flex-1 rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm text-slate-700 sm:flex-none dark:border-slate-700 dark:bg-slate-900 dark:text-slate-300 outline-none focus:border-indigo-600 focus:ring-1 focus:ring-indigo-600"
          >
            <option value="">Semua tag</option>
            {tags.map((t) => (
              <option key={t.id} value={t.id}>
                {t.name}
              </option>
            ))}
          </select>
        )}
        <select
          value={sortKey}
          aria-label="Urutan"
          onChange={(e) => {
            setSortKey(e.target.value)
            resetToFirstPage()
          }}
          className="min-w-[140px] flex-1 rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm text-slate-700 sm:flex-none dark:border-slate-700 dark:bg-slate-900 dark:text-slate-300 outline-none focus:border-indigo-600 focus:ring-1 focus:ring-indigo-600"
        >
          <option value="">Urutan: terbaru</option>
          <option value="-last_inform_at">Inform terakhir (terbaru)</option>
          <option value="last_inform_at">Inform terakhir (terlama)</option>
          <option value="serial_number">Serial A–Z</option>
          <option value="-serial_number">Serial Z–A</option>
        </select>
        <button
          type="button"
          onClick={() => setShowAdvanced((v) => !v)}
          aria-expanded={showAdvanced}
          className={`flex items-center gap-1.5 rounded-lg border px-3 py-2 text-sm transition-colors ${
            paramConds.length > 0
              ? 'border-indigo-600 bg-indigo-50 text-indigo-700 dark:border-indigo-400 dark:bg-indigo-950/40 dark:text-indigo-300'
              : 'border-slate-300 bg-white text-slate-700 hover:bg-slate-50 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-300 dark:hover:bg-slate-800'
          }`}
        >
          <SlidersHorizontal className="h-4 w-4" />
          Filter parameter{paramConds.length > 0 ? ` (${paramConds.length})` : ''}
        </button>
      </div>

      {showAdvanced && (
        <div className="mb-4 rounded-xl border border-slate-200 bg-white p-4 dark:border-slate-800 dark:bg-slate-900">
          <label htmlFor="param-filter" className="text-sm font-medium text-slate-800 dark:text-slate-200">
            Kondisi nilai parameter (satu per baris, semuanya harus terpenuhi)
          </label>
          <textarea
            id="param-filter"
            rows={3}
            maxLength={1500}
            value={paramText}
            onChange={(e) => {
              setParamText(e.target.value)
              resetToFirstPage()
            }}
            placeholder={'InternetGatewayDevice.DeviceInfo.SoftwareVersion~prefix~V5\nInternetGatewayDevice.LANDevice.1.WLANConfiguration.1.SSID~like~kantor'}
            spellCheck={false}
            className="mt-2 w-full rounded-lg border border-slate-300 px-3 py-2 font-mono text-xs text-slate-900 outline-none focus:border-indigo-600 focus:ring-1 focus:ring-indigo-600 dark:border-slate-700 dark:bg-slate-950 dark:text-slate-100"
          />
          <p className="mt-1.5 text-xs text-slate-500 dark:text-slate-400">
            Bentuk <code>nama~operator~nilai</code>. Operator: <code>eq</code> <code>ne</code> <code>like</code> <code>prefix</code> <code>gt</code>{' '}
            <code>ge</code> <code>lt</code> <code>le</code> (angka) <code>exists</code> <code>missing</code> (tanpa nilai). Maksimal 5 kondisi; tidak
            peka huruf besar/kecil. Parameter rahasia (password/PSK) hanya untuk admin.
          </p>
        </div>
      )}

      {canBulkAct && selected.size > 0 && (
        <div className="mb-4 flex flex-wrap items-center gap-3 rounded-xl border border-slate-300 bg-slate-900 px-4 py-2.5 text-white">
          <span className="text-sm font-medium">{selected.size} device dipilih</span>
          <div className="ml-auto flex flex-wrap items-center gap-2">
            <button
              onClick={() => setBulkAction('profile')}
              className="flex items-center gap-1.5 rounded-full bg-white/10 px-3 py-1.5 text-sm font-medium transition-colors hover:bg-white/20"
            >
              <Sparkles className="h-3.5 w-3.5" /> Terapkan Profile
            </button>
            <button
              onClick={() => setBulkAction('firmware')}
              className="flex items-center gap-1.5 rounded-full bg-white/10 px-3 py-1.5 text-sm font-medium transition-colors hover:bg-white/20"
            >
              <HardDrive className="h-3.5 w-3.5" /> Firmware Upgrade
            </button>
            <button
              onClick={() => setBulkAction('reboot')}
              className="flex items-center gap-1.5 rounded-full bg-white/10 px-3 py-1.5 text-sm font-medium transition-colors hover:bg-white/20"
            >
              <Power className="h-3.5 w-3.5" /> Reboot
            </button>
            {isAdmin && (
              <button
                onClick={() => void handleBulkDelete()}
                disabled={deleteMut.isPending}
                className="flex items-center gap-1.5 rounded-full bg-red-500/90 px-3 py-1.5 text-sm font-medium transition-colors hover:bg-red-500 disabled:opacity-60"
              >
                <Trash2 className="h-3.5 w-3.5" /> Hapus
              </button>
            )}
            <button
              onClick={() => setSelected(new Set())}
              className="flex h-7 w-7 items-center justify-center rounded-full text-slate-300 transition-colors hover:bg-white/10 hover:text-white"
              title="Batal pilih"
            >
              <X className="h-4 w-4" />
            </button>
          </div>
        </div>
      )}

      <div className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm dark:border-slate-800 dark:bg-slate-900">
        {isLoading ? (
          <div className="divide-y divide-slate-100 dark:divide-slate-800">
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
            description="Device muncul otomatis begitu CPE mengirim Inform pertama ke endpoint CWMP ACS. Anda juga bisa mendaftarkannya lebih dulu (pra-registrasi) atau mengimpor daftar perangkat."
          >
            {isAdmin && (
              <div className="mt-5 flex flex-wrap justify-center gap-2.5">
                <button type="button" onClick={() => setShowCreate(true)} className="flex items-center gap-2 rounded-full bg-indigo-600 px-3.5 py-2 text-sm font-semibold text-white hover:bg-indigo-500">
                  <Plus className="h-4 w-4" /> Tambah Perangkat
                </button>
                <button type="button" onClick={() => setShowImport(true)} className="flex items-center gap-2 rounded-full border border-slate-300 px-3.5 py-2 text-sm font-medium text-slate-700 hover:bg-slate-50 dark:border-slate-700 dark:text-slate-300 dark:hover:bg-slate-800">
                  <Upload className="h-4 w-4" /> Impor dari file
                </button>
              </div>
            )}
          </EmptyState>
        ) : (
          <>
            <ul className="divide-y divide-slate-100 md:hidden dark:divide-slate-800" aria-label="Daftar perangkat">
              {devices.map((d) => {
                const status = findRefById(statusRefs, d.device_status_id)
                return (
                  <li key={d.id} className="flex items-start gap-3 px-4 py-3.5">
                    {canBulkAct && (
                      <input
                        type="checkbox"
                        checked={selected.has(d.id)}
                        aria-label={`Pilih device ${d.serial_number}`}
                        onChange={() => toggleSelected(d.id)}
                        className="mt-1 h-4 w-4 shrink-0 rounded border-slate-300"
                      />
                    )}
                    <button type="button" onClick={() => navigate(`/devices/${d.id}`)} className="min-w-0 flex-1 text-left">
                      <span className="flex items-center justify-between gap-2">
                        <span className="truncate font-mono text-xs font-medium text-slate-900 dark:text-slate-100">{d.serial_number}</span>
                        <StatusBadge code={status?.code} label={status?.name ?? '-'} pulse={status?.code === 'ONLINE'} />
                      </span>
                      <span className="mt-1 block truncate text-sm text-slate-600 dark:text-slate-400">
                        {vendorName(d.vendor_id)}
                        {d.software_version ? ` · ${d.software_version}` : ''}
                      </span>
                      <span className="mt-0.5 block truncate font-mono text-xs text-slate-500 dark:text-slate-400">
                        {d.ip_address ?? '-'}
                        {d.mac_address ? ` · ${d.mac_address}` : ''}
                      </span>
                      <span className="mt-0.5 block text-xs text-slate-500 dark:text-slate-400">Terhubung {formatRelativeTime(d.last_inform_at)}</span>
                    </button>
                  </li>
                )
              })}
            </ul>
            <div className="hidden table-scroll md:block">
            <table className="w-full text-left text-sm">
              <thead>
                <tr className="border-b border-slate-200 bg-slate-50 text-xs font-medium uppercase tracking-wide text-slate-500 dark:border-slate-800 dark:bg-slate-800/50 dark:text-slate-400">
                  {canBulkAct && (
                    <th className="w-10 px-4 py-3">
                      <input
                        type="checkbox"
                        checked={devices.length > 0 && selected.size === devices.length}
                        aria-label="Pilih semua device di halaman ini"
                        onChange={toggleSelectAll}
                        onClick={(e) => e.stopPropagation()}
                        className="h-4 w-4 rounded border-slate-300"
                      />
                    </th>
                  )}
                  <th className="px-4 py-3">Status</th>
                  <th className="px-4 py-3">Vendor</th>
                  <th className="px-4 py-3">Model</th>
                  <th className="px-4 py-3">Serial Number</th>
                  <th className="px-4 py-3">IP / MAC</th>
                  <th className="px-4 py-3">Firmware</th>
                  <th className="px-4 py-3" title="Level terima optik (RX). Hijau normal, kuning waspada (< -25 dBm), merah kritis (< -27 atau > -8 dBm).">Redaman</th>
                  <th className="px-4 py-3">Username PPPoE</th>
                  <th className="px-4 py-3">Terakhir Terhubung</th>
                  {canBulkAct && <th className="w-24 px-4 py-3 text-right">Aksi</th>}
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
                {devices.map((d) => {
                  const status = findRefById(statusRefs, d.device_status_id)
                  return (
                    <tr
                      key={d.id}
                      onClick={() => navigate(`/devices/${d.id}`)}
                      className="cursor-pointer transition-colors hover:bg-slate-50 dark:hover:bg-slate-800/50"
                    >
                      {canBulkAct && (
                        <td className="px-4 py-3.5" onClick={(e) => e.stopPropagation()}>
                          <input
                            type="checkbox"
                            checked={selected.has(d.id)}
                            aria-label={`Pilih device ${d.serial_number}`}
                            onChange={() => toggleSelected(d.id)}
                            className="h-4 w-4 rounded border-slate-300"
                          />
                        </td>
                      )}
                      <td className="px-4 py-3.5">
                        <StatusBadge code={status?.code} label={status?.name ?? '-'} pulse={status?.code === 'ONLINE'} />
                      </td>
                      <td className="max-w-[11rem] truncate px-4 py-3.5 text-slate-600 dark:text-slate-400" title={d.vendor_id ? vendorName(d.vendor_id) : `OUI ${d.oui ?? '-'} belum ada di katalog. Buka detail perangkat > Overview > Tetapkan vendor.`}>{vendorName(d.vendor_id)}</td>
                      <td className="max-w-[8rem] truncate px-4 py-3.5 text-slate-600 dark:text-slate-400" title={d.product_class ?? undefined}>{d.product_class ?? '-'}</td>
                      <td className="whitespace-nowrap px-4 py-3.5 font-mono text-xs text-slate-900 dark:text-slate-100">{d.serial_number}</td>
                      <td className="whitespace-nowrap px-4 py-3.5 font-mono text-xs text-slate-600 dark:text-slate-300">
                        <div>{d.ip_address ?? '-'}</div>
                        {d.mac_address && <div className="text-slate-400 dark:text-slate-500">{d.mac_address}</div>}
                      </td>
                      <td className="max-w-[10rem] truncate px-4 py-3.5 text-slate-500 dark:text-slate-400" title={d.software_version ?? undefined}>{d.software_version ?? '-'}</td>
                      <td className="px-4 py-3.5 text-sm">
                        <OpticalCell rx={d.rx_power_dbm} tx={d.tx_power_dbm} at={d.optical_at} />
                      </td>
                      <td className="max-w-[14rem] truncate px-4 py-3.5 font-mono text-xs text-slate-600 dark:text-slate-300" title={d.pppoe_username ?? undefined}>
                        {d.pppoe_username ?? <span className="text-slate-400 dark:text-slate-500">-</span>}
                      </td>
                      <td className="whitespace-nowrap px-4 py-3.5 text-slate-500 dark:text-slate-400">{formatRelativeTime(d.last_inform_at)}</td>
                      {canBulkAct && (
                        <td className="px-4 py-3.5" onClick={(e) => e.stopPropagation()}>
                          <div className="flex items-center justify-end gap-1">
                            <button
                              type="button"
                              onClick={() => setEditDevice(d)}
                              title="Edit perangkat"
                              aria-label={`Edit ${d.serial_number}`}
                              className="flex h-7 w-7 items-center justify-center rounded-full text-slate-500 transition-colors hover:bg-slate-100 hover:text-slate-700 dark:hover:bg-slate-800 dark:hover:text-slate-200 dark:text-slate-400"
                            >
                              <Pencil className="h-4 w-4" />
                            </button>
                            {isAdmin && (
                              <button
                                type="button"
                                onClick={() => void handleDelete(d)}
                                title="Hapus perangkat"
                                aria-label={`Hapus ${d.serial_number}`}
                                className="flex h-7 w-7 items-center justify-center rounded-full text-slate-500 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-500/10 dark:hover:text-red-400 dark:text-slate-400"
                              >
                                <Trash2 className="h-4 w-4" />
                              </button>
                            )}
                          </div>
                        </td>
                      )}
                    </tr>
                  )
                })}
              </tbody>
            </table>
            </div>

            <div className="flex items-center justify-between border-t border-slate-200 px-5 py-3 text-sm text-slate-500 dark:text-slate-400">
              <span>
                Menampilkan {devices.length} dari {total} device
              </span>
              <div className="flex items-center gap-1">
                <button
                  onClick={() => setPage((p) => Math.max(1, p - 1))}
                  disabled={page <= 1}
                  aria-label="Halaman sebelumnya"
                  className="flex h-8 w-8 items-center justify-center rounded-full border border-slate-200 text-slate-500 dark:text-slate-400 transition-colors hover:bg-slate-50 dark:hover:bg-slate-800/50 disabled:cursor-not-allowed disabled:opacity-40"
                >
                  <ChevronLeft className="h-4 w-4" />
                </button>
                <span className="px-2 text-xs tabular-nums">
                  {page} / {totalPages}
                </span>
                <button
                  onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
                  disabled={page >= totalPages}
                  aria-label="Halaman berikutnya"
                  className="flex h-8 w-8 items-center justify-center rounded-full border border-slate-200 text-slate-500 dark:text-slate-400 transition-colors hover:bg-slate-50 dark:hover:bg-slate-800/50 disabled:cursor-not-allowed disabled:opacity-40"
                >
                  <ChevronRight className="h-4 w-4" />
                </button>
              </div>
            </div>
          </>
        )}
      </div>

      {showCreate && <DeviceFormModal mode="create" onClose={() => setShowCreate(false)} />}
      {editDevice && <DeviceFormModal mode="edit" device={editDevice} onClose={() => setEditDevice(null)} />}
      {showImport && (
        <ImportWizard
          title="Impor Perangkat"
          adapter={deviceAdapter({ isSuperadmin: isSuper, vendors, tenants })}
          onClose={() => setShowImport(false)}
          onChanged={() => {
            queryClient.invalidateQueries({ queryKey: ['devices'] })
            queryClient.invalidateQueries({ queryKey: ['device-stats'] })
          }}
        />
      )}
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
    <ul className="max-h-64 divide-y divide-slate-100 dark:divide-slate-800 overflow-y-auto rounded-lg border border-slate-200">
      {items.map((item) => (
        <li key={item.device.id} className="flex items-center justify-between gap-3 px-3 py-2 text-sm">
          <div className="min-w-0">
            <p className="truncate font-mono text-xs text-slate-700 dark:text-slate-300">{item.device.serial_number}</p>
            {item.message && <p className="truncate text-xs text-slate-500 dark:text-slate-400">{item.message}</p>}
          </div>
          {item.status === 'pending' && <span className="shrink-0 text-xs text-slate-500 dark:text-slate-400">Menunggu</span>}
          {item.status === 'running' && <RefreshCw className="h-4 w-4 shrink-0 animate-spin text-slate-500 dark:text-slate-400" />}
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
          <p className="text-xs text-slate-500 dark:text-slate-400">
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
          <p className="text-xs text-slate-500 dark:text-slate-400">
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
          <p className="text-sm text-slate-600 dark:text-slate-400">
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
