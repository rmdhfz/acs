import { useState, type FormEvent, type ReactNode } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { Line, LineChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import { Activity, ArrowLeft, Cpu, Gauge, HardDrive, History, ListChecks, Radio, ScrollText, Sparkles, SlidersHorizontal, RefreshCcw, AlertTriangle, FileUp } from 'lucide-react'
import { StatusBadge } from '../components/StatusBadge'
import { EmptyState } from '../components/EmptyState'
import { PageSpinner } from '../components/Spinner'
import { ParameterTree } from '../components/ParameterTree'
import { ConfigHistory } from '../components/ConfigHistory'
import { Modal } from '../components/Modal'
import { useAuth } from '../lib/auth'
import { useTheme } from '../lib/theme'
import { ApiError } from '../lib/api'
import {
  findRefById,
  useApplyProfile,
  useDevice,
  useDeviceActivity,
  useDeviceDiagnostics,
  useDeviceEvents,
  useDeviceOpticalMetrics,
  useDeviceParameters,
  useDeviceTasks,
  useFirmwareJobs,
  useFirmwareList,
  useProvisioningProfiles,
  useRefs,
  useScheduleFirmwareUpgrade,
  useTriggerConnectionRequest,
  useTriggerDiagnostic,
  useUpdateDevice,
  useVendors,
  useRebootDevice,
  useFactoryResetDevice,
  usePushFileToDevice,
  useFiles,
} from '../lib/hooks'
import { formatJSONField, formatDateTime, formatRelativeTime } from '../lib/format'

type Tab = 'overview' | 'parameters' | 'optical' | 'events' | 'tasks' | 'diagnostics' | 'firmware' | 'timeline' | 'config'

const TABS: { key: Tab; label: string; icon: typeof Cpu }[] = [
  { key: 'overview', label: 'Overview', icon: Cpu },
  { key: 'parameters', label: 'Parameter', icon: SlidersHorizontal },
  { key: 'optical', label: 'Redaman Optik', icon: Gauge },
  { key: 'events', label: 'Histori Event', icon: History },
  { key: 'tasks', label: 'Task', icon: ListChecks },
  { key: 'diagnostics', label: 'Diagnostics', icon: Activity },
  { key: 'firmware', label: 'Firmware & Files', icon: HardDrive },
  { key: 'timeline', label: 'Timeline Audit', icon: ScrollText },
  { key: 'config', label: 'Config History', icon: FileUp },
]

const ACTIVITY_LABELS: Record<string, string> = {
  UPDATE_DEVICE: 'Mengubah data device',
  APPLY_PROVISIONING_PROFILE: 'Menerapkan provisioning profile',
  ZERO_TOUCH_MATCH: 'Auto-provisioning (zero-touch) match',
}

const inputCls =
  'w-full rounded-lg border border-slate-300 px-3 py-2 text-sm text-slate-900 outline-none transition-colors focus:border-slate-500 focus:ring-1 focus:ring-slate-500 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-100'
const primaryBtnCls =
  'flex items-center justify-center gap-2 rounded-lg bg-slate-900 px-3 py-2 text-sm font-medium text-white transition-colors hover:bg-slate-800 disabled:cursor-not-allowed disabled:opacity-60 dark:bg-slate-100 dark:text-slate-900 dark:hover:bg-white'

export function DeviceDetailPage() {
  const { id } = useParams<{ id: string }>()
  const deviceId = Number(id)
  const navigate = useNavigate()
  const [tab, setTab] = useState<Tab>('overview')
  const [showApplyProfile, setShowApplyProfile] = useState(false)
  const { hasRole } = useAuth()

  const { data: device, isLoading } = useDevice(deviceId)
  const { data: statusRefs } = useRefs('ref_device_status')
  const { data: vendorsResp } = useVendors()
  
  const triggerConnReq = useTriggerConnectionRequest()
  const rebootMutation = useRebootDevice()
  const factoryResetMutation = useFactoryResetDevice()
  const [connReqError, setConnReqError] = useState<string | null>(null)

  if (isLoading || !device) return <PageSpinner />

  const status = findRefById(statusRefs, device.device_status_id)
  const vendor = vendorsResp?.data?.find((v) => v.id === device.vendor_id)

  const handleConnReq = async () => {
    setConnReqError(null)
    try {
      await triggerConnReq.mutateAsync(deviceId)
      alert('Connection Request berhasil dikirim. Menunggu respons Inform dari device (bisa beberapa detik).')
    } catch (err) {
      setConnReqError(err instanceof ApiError ? err.message : 'Gagal mengirim Connection Request')
    }
  }

  return (
    <div className="mx-auto max-w-7xl px-6 py-8">
      <div className="mb-4 flex items-center justify-between">
        <button
          onClick={() => navigate('/devices')}
          className="flex items-center gap-1.5 text-sm text-slate-500 dark:text-slate-400 transition-colors hover:text-slate-900 dark:hover:text-slate-100"
        >
          <ArrowLeft className="h-4 w-4" /> Kembali ke daftar device
        </button>
        <div className="flex items-center gap-2">
          {hasRole('ADMIN', 'NOC') && (
            <button
              onClick={handleConnReq}
              disabled={triggerConnReq.isPending || !device.connection_request_url}
              title={!device.connection_request_url ? 'Device tidak memiliki connection request URL' : ''}
              className="flex items-center gap-1.5 rounded-lg border border-slate-300 bg-white px-3 py-1.5 text-sm font-medium text-slate-700 dark:text-slate-300 transition-colors hover:bg-slate-50 dark:hover:bg-slate-800/50 disabled:opacity-50"
            >
              <Radio className="h-4 w-4" /> Connection Req
            </button>
          )}
          {hasRole('ADMIN') && (
            <button
              onClick={() => setShowApplyProfile(true)}
              className="flex items-center gap-1.5 rounded-lg border border-slate-300 bg-white px-3 py-1.5 text-sm font-medium text-slate-700 dark:text-slate-300 transition-colors hover:bg-slate-50 dark:hover:bg-slate-800/50"
            >
              <Sparkles className="h-4 w-4" /> Terapkan Provisioning Profile
            </button>
          )}
          {hasRole('ADMIN', 'NOC') && (
            <button
              onClick={() => {
                if (window.confirm('Reboot device ini?')) {
                  rebootMutation.mutate(deviceId)
                }
              }}
              disabled={rebootMutation.isPending}
              className="flex items-center gap-1.5 rounded-lg border border-slate-300 bg-white px-3 py-1.5 text-sm font-medium text-slate-700 dark:text-slate-300 transition-colors hover:bg-slate-50 dark:hover:bg-slate-800/50"
            >
              <RefreshCcw className="h-4 w-4" /> Reboot
            </button>
          )}
          {hasRole('ADMIN') && (
            <button
              onClick={() => {
                if (window.confirm('PERINGATAN: Factory Reset akan mengembalikan device ke pengaturan pabrik dan memutus koneksi. Lanjutkan?')) {
                  factoryResetMutation.mutate(deviceId)
                }
              }}
              disabled={factoryResetMutation.isPending}
              className="flex items-center gap-1.5 rounded-lg border border-red-300 bg-red-50 px-3 py-1.5 text-sm font-medium text-red-700 dark:bg-red-900/30 dark:border-red-800 dark:text-red-400 transition-colors hover:bg-red-100 dark:hover:bg-red-900/50"
            >
              <AlertTriangle className="h-4 w-4" /> Factory Reset
            </button>
          )}
        </div>
      </div>
      {connReqError && <p className="mb-4 text-sm text-red-600">{connReqError}</p>}

      <div className="mb-6 flex flex-wrap items-start justify-between gap-4 rounded-xl border border-slate-200 bg-white p-5 shadow-sm">
        <div>
          <div className="flex items-center gap-3">
            <h1 className="font-mono text-lg font-semibold text-slate-900 dark:text-slate-100">{device.serial_number}</h1>
            <StatusBadge code={status?.code} label={status?.name ?? '-'} pulse={status?.code === 'ONLINE'} />
          </div>
          <p className="mt-1 text-sm text-slate-500 dark:text-slate-400">
            {vendor?.name ?? 'Vendor belum diketahui'}
            {device.product_class ? ` · ${device.product_class}` : ''}
          </p>
        </div>
        <dl className="grid grid-cols-2 gap-x-8 gap-y-1 text-sm sm:grid-cols-4">
          <InfoItem label="IP Address" value={device.ip_address ?? '-'} mono />
          <InfoItem label="MAC Address" value={device.mac_address ?? '-'} mono />
          <InfoItem label="Firmware" value={device.software_version ?? '-'} />
          <InfoItem label="Terakhir Inform" value={formatRelativeTime(device.last_inform_at)} />
        </dl>
      </div>

      {showApplyProfile && (
        <ApplyProfileModal deviceId={deviceId} vendorId={device.vendor_id} onClose={() => setShowApplyProfile(false)} />
      )}

      <div className="mb-4 flex gap-1 border-b border-slate-200">
        {TABS.map((t) => (
          <button
            key={t.key}
            onClick={() => setTab(t.key)}
            className={`flex items-center gap-1.5 border-b-2 px-3 py-2.5 text-sm font-medium transition-colors ${
              tab === t.key
                ? 'border-slate-900 text-slate-900 dark:text-slate-100'
                : 'border-transparent text-slate-500 dark:text-slate-400 hover:text-slate-800'
            }`}
          >
            <t.icon className="h-4 w-4" />
            {t.label}
          </button>
        ))}
      </div>

      <div className="mt-6">
        {tab === 'overview' && <OverviewTab device={device} />}
        {tab === 'parameters' && <ParametersTab deviceId={deviceId} />}
        {tab === 'optical' && <OpticalMetricsTab deviceId={deviceId} />}
        {tab === 'events' && <EventsTab deviceId={deviceId} />}
        {tab === 'tasks' && <TasksTab deviceId={deviceId} />}
        {tab === 'diagnostics' && <DiagnosticsTab deviceId={deviceId} canTrigger={hasRole('ADMIN', 'NOC')} />}
        {tab === 'firmware' && <FirmwareTab deviceId={deviceId} vendorId={device.vendor_id} canSchedule={hasRole('ADMIN')} />}
        {tab === 'timeline' && <TimelineTab deviceId={deviceId} />}
        {tab === 'config' && <ConfigHistory deviceId={deviceId} />}
      </div>
    </div>
  )
}

function InfoItem({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div>
      <dt className="text-xs text-slate-400 dark:text-slate-500">{label}</dt>
      <dd className={`text-slate-800 ${mono ? 'font-mono text-xs' : ''}`}>{value}</dd>
    </div>
  )
}

function Card({ children }: { children: ReactNode }) {
  return <div className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm dark:border-slate-800 dark:bg-slate-900">{children}</div>
}

function EditDeviceModal({ device, onClose }: { device: NonNullable<ReturnType<typeof useDevice>['data']>; onClose: () => void }) {
  const updateMutation = useUpdateDevice()
  const [notes, setNotes] = useState(device.notes ?? '')
  const [latitude, setLatitude] = useState(device.latitude?.toString() ?? '')
  const [longitude, setLongitude] = useState(device.longitude?.toString() ?? '')

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    await updateMutation.mutateAsync({
      id: device.id,
      notes: notes || undefined,
      latitude: latitude ? parseFloat(latitude) : undefined,
      longitude: longitude ? parseFloat(longitude) : undefined,
    })
    onClose()
  }

  return (
    <Modal title="Edit Device" onClose={onClose}>
      <form onSubmit={handleSubmit} className="space-y-4">
        <div>
          <label className="mb-1.5 block text-sm font-medium text-slate-700 dark:text-slate-300">Catatan</label>
          <textarea value={notes} onChange={e => setNotes(e.target.value)} rows={3} className={inputCls} />
        </div>
        <div className="grid grid-cols-2 gap-4">
          <div>
            <label className="mb-1.5 block text-sm font-medium text-slate-700 dark:text-slate-300">Latitude</label>
            <input type="number" step="any" value={latitude} onChange={e => setLatitude(e.target.value)} className={inputCls} />
          </div>
          <div>
            <label className="mb-1.5 block text-sm font-medium text-slate-700 dark:text-slate-300">Longitude</label>
            <input type="number" step="any" value={longitude} onChange={e => setLongitude(e.target.value)} className={inputCls} />
          </div>
        </div>
        <div className="pt-2">
          <button type="submit" disabled={updateMutation.isPending} className={`${primaryBtnCls} w-full`}>
            Simpan Perubahan
          </button>
        </div>
      </form>
    </Modal>
  )
}

function OverviewTab({ device }: { device: NonNullable<ReturnType<typeof useDevice>['data']> }) {
  const [showEdit, setShowEdit] = useState(false)
  const rows: [string, string][] = [
    ['Device UUID', device.device_uuid],
    ['OUI', device.oui ?? '-'],
    ['Product Class', device.product_class ?? '-'],
    ['Hardware Version', device.hardware_version ?? '-'],
    ['Software Version', device.software_version ?? '-'],
    ['Connection Request URL', device.connection_request_url ?? '-'],
    ['Terakhir Boot', formatDateTime(device.last_boot_event_at)],
    ['Pertama Terdaftar', formatDateTime(device.created_at)],
    ['Latitude', device.latitude?.toString() ?? '-'],
    ['Longitude', device.longitude?.toString() ?? '-'],
    ['Catatan', device.notes ?? '-'],
  ]
  return (
    <Card>
      <div className="flex justify-between items-center border-b border-slate-100 dark:border-slate-800 px-5 py-3">
        <h3 className="font-medium text-sm text-slate-900 dark:text-slate-100">Detail Perangkat</h3>
        <button onClick={() => setShowEdit(true)} className="text-sm font-medium text-blue-600 hover:underline dark:text-blue-400">
          Edit
        </button>
      </div>
      <dl className="divide-y divide-slate-100 dark:divide-slate-800">
        {rows.map(([label, value]) => (
          <div key={label} className="grid grid-cols-3 gap-4 px-5 py-3 text-sm">
            <dt className="text-slate-500 dark:text-slate-400">{label}</dt>
            <dd className="col-span-2 break-all font-mono text-xs text-slate-800 dark:text-slate-200">{value}</dd>
          </div>
        ))}
      </dl>
      {showEdit && <EditDeviceModal device={device} onClose={() => setShowEdit(false)} />}
    </Card>
  )
}

function ParametersTab({ deviceId }: { deviceId: number }) {
  const { data, isLoading, refetch } = useDeviceParameters(deviceId)

  if (isLoading) return <PageSpinner />

  if (!data || data.length === 0) {
    return (
      <EmptyState
        icon={SlidersHorizontal}
        title="Belum ada parameter tersinkron"
        description="Parameter TR-069 akan muncul setelah ACS menerima ParameterList dari Inform atau GetParameterValues."
      >
        <div className="mt-4">
          <button
            onClick={() => refetch()}
            className="text-sm font-medium text-blue-600 dark:text-blue-400 hover:underline"
          >
            Refresh Data
          </button>
        </div>
      </EmptyState>
    )
  }

  return (
    <ParameterTree
      deviceId={deviceId}
      parameters={data}
      isLoading={isLoading}
      onRefresh={() => refetch()}
    />
  )
}

function OpticalMetricsTab({ deviceId }: { deviceId: number }) {
  const { data, isLoading } = useDeviceOpticalMetrics(deviceId)
  const { resolvedTheme } = useTheme()
  const isDark = resolvedTheme === 'dark'
  const metrics = data?.data ?? []

  if (isLoading) return <PageSpinner />
  if (metrics.length === 0) {
    return (
      <EmptyState
        icon={Gauge}
        title="Belum ada data redaman optik"
        description="Metrik RX/TX power tersinkron dari parameter TR-069 saat tersedia (mis. via diagnostic OPTICAL_POWER atau parameter sync ONT GPON/EPON)."
      />
    )
  }

  // API mengembalikan urutan terbaru dulu (DESC) — balik ke kronologis utk chart.
  const chronological = [...metrics].reverse()
  const chartData = chronological.map((m) => ({
    time: formatDateTime(m.recorded_at),
    rx: m.rx_power_dbm,
    tx: m.tx_power_dbm,
  }))
  const latest = metrics[0]

  return (
    <div className="space-y-4">
      <div className="grid grid-cols-2 gap-4 sm:grid-cols-5">
        <MetricStat label="RX Power" value={latest.rx_power_dbm} unit="dBm" />
        <MetricStat label="TX Power" value={latest.tx_power_dbm} unit="dBm" />
        <MetricStat label="Voltage" value={latest.voltage} unit="V" />
        <MetricStat label="Bias Current" value={latest.bias_current_ma} unit="mA" />
        <MetricStat label="Suhu" value={latest.temperature_celsius} unit="°C" />
      </div>

      <Card>
        <div className="p-5">
          <h3 className="mb-4 text-sm font-semibold text-slate-900 dark:text-slate-100">Tren RX/TX Power</h3>
          <ResponsiveContainer width="100%" height={280}>
            <LineChart data={chartData} margin={{ left: -12, right: 8 }}>
              <CartesianGrid strokeDasharray="3 3" stroke={isDark ? '#1e293b' : '#f1f5f9'} />
              <XAxis dataKey="time" tick={{ fontSize: 11, fill: isDark ? '#94a3b8' : '#64748b' }} minTickGap={30} />
              <YAxis tick={{ fontSize: 12, fill: isDark ? '#94a3b8' : '#64748b' }} unit=" dBm" width={70} />
              <Tooltip
                contentStyle={{
                  borderRadius: 8,
                  borderColor: isDark ? '#334155' : '#e2e8f0',
                  backgroundColor: isDark ? '#0f172a' : '#ffffff',
                  color: isDark ? '#e2e8f0' : '#0f172a',
                  fontSize: 13,
                }}
              />
              <Line type="monotone" dataKey="rx" name="RX Power" stroke="#3b82f6" strokeWidth={2} dot={false} connectNulls />
              <Line type="monotone" dataKey="tx" name="TX Power" stroke="#f59e0b" strokeWidth={2} dot={false} connectNulls />
            </LineChart>
          </ResponsiveContainer>
        </div>
      </Card>
    </div>
  )
}

function MetricStat({ label, value, unit }: { label: string; value: number | null; unit: string }) {
  return (
    <div className="rounded-xl border border-slate-200 bg-white p-4 shadow-sm">
      <p className="text-xs font-medium text-slate-500 dark:text-slate-400">{label}</p>
      <p className="mt-1 text-lg font-semibold tabular-nums text-slate-900 dark:text-slate-100">
        {value ?? '-'}
        {value !== null && <span className="ml-1 text-xs font-normal text-slate-400 dark:text-slate-500">{unit}</span>}
      </p>
    </div>
  )
}

function EventsTab({ deviceId }: { deviceId: number }) {
  const { data, isLoading } = useDeviceEvents(deviceId)
  const { data: eventCodeRefs } = useRefs('ref_event_codes')

  if (isLoading) return <PageSpinner />
  const events = data?.data ?? []

  if (events.length === 0) {
    return <EmptyState icon={History} title="Belum ada histori event" description="Event BOOTSTRAP/BOOT/PERIODIC dari device akan tercatat di sini." />
  }

  return (
    <Card>
      <ul className="divide-y divide-slate-100 dark:divide-slate-800">
        {events.map((e) => {
          const code = findRefById(eventCodeRefs, e.event_code_id)
          return (
            <li key={e.id} className="flex items-center justify-between gap-4 px-5 py-3 text-sm">
              <div className="flex items-center gap-3">
                <span className="h-1.5 w-1.5 shrink-0 rounded-full bg-slate-300" />
                <span className="font-medium text-slate-800">{code?.name ?? code?.code ?? `#${e.event_code_id}`}</span>
              </div>
              <span className="shrink-0 text-xs text-slate-400 dark:text-slate-500">{formatDateTime(e.occurred_at)}</span>
            </li>
          )
        })}
      </ul>
    </Card>
  )
}

function TasksTab({ deviceId }: { deviceId: number }) {
  const { data, isLoading } = useDeviceTasks(deviceId)
  const { data: statusRefs } = useRefs('ref_task_status')
  const { data: typeRefs } = useRefs('ref_task_types')

  if (isLoading) return <PageSpinner />
  const tasks = data?.data ?? []

  if (tasks.length === 0) {
    return <EmptyState icon={ListChecks} title="Belum ada task" description="Task RPC (Reboot, SetParameterValues, dst) yang diantrekan ke device akan muncul di sini." />
  }

  return (
    <Card>
      <table className="w-full text-left text-sm">
        <thead>
          <tr className="border-b border-slate-200 bg-slate-50 text-xs font-medium uppercase tracking-wide text-slate-500 dark:border-slate-800 dark:bg-slate-800/50 dark:text-slate-400">
            <th className="px-5 py-3">Status</th>
            <th className="px-5 py-3">Tipe</th>
            <th className="px-5 py-3">Retry</th>
            <th className="px-5 py-3">Dibuat</th>
            <th className="px-5 py-3">Error</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
          {tasks.map((t) => {
            const status = findRefById(statusRefs, t.task_status_id)
            const type = findRefById(typeRefs, t.task_type_id)
            return (
              <tr key={t.id}>
                <td className="px-5 py-3">
                  <StatusBadge code={status?.code} label={status?.name ?? '-'} />
                </td>
                <td className="px-5 py-3 text-slate-700 dark:text-slate-300">{type?.name ?? type?.code ?? '-'}</td>
                <td className="px-5 py-3 text-slate-500 dark:text-slate-400">
                  {t.retry_count}/{t.max_retries}
                </td>
                <td className="px-5 py-3 text-slate-500 dark:text-slate-400">{formatRelativeTime(t.created_at)}</td>
                <td className="max-w-xs truncate px-5 py-3 text-xs text-red-600">{t.error_message ?? '-'}</td>
              </tr>
            )
          })}
        </tbody>
      </table>
    </Card>
  )
}

function ApplyProfileModal({
  deviceId,
  vendorId,
  onClose,
}: {
  deviceId: number
  vendorId: number | null
  onClose: () => void
}) {
  const { data: profilesResp } = useProvisioningProfiles()
  const profiles = (profilesResp?.data ?? []).filter((p) => p.is_active && (!p.vendor_id || p.vendor_id === vendorId))
  const [profileId, setProfileId] = useState('')
  const [error, setError] = useState<string | null>(null)
  const applyMutation = useApplyProfile()

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    try {
      await applyMutation.mutateAsync({ deviceId, profileId: Number(profileId) })
      onClose()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Gagal menerapkan profile')
    }
  }

  return (
    <Modal title="Terapkan Provisioning Profile" onClose={onClose}>
      <form onSubmit={handleSubmit} className="space-y-3">
        <p className="text-xs text-slate-500 dark:text-slate-400">
          Mengantre task SetParameterValues berisi seluruh parameter profil ke device ini sekarang — perubahan profil
          di kemudian hari tidak otomatis re-apply (FR-18).
        </p>
        <select required value={profileId} onChange={(e) => setProfileId(e.target.value)} className={inputCls}>
          <option value="">Pilih profil...</option>
          {profiles.map((p) => (
            <option key={p.id} value={p.id}>
              {p.name}
            </option>
          ))}
        </select>
        {error && <p className="text-sm text-red-600">{error}</p>}
        <button type="submit" disabled={applyMutation.isPending} className={`${primaryBtnCls} w-full`}>
          Terapkan
        </button>
      </form>
    </Modal>
  )
}

const DIAGNOSTIC_TYPES = [
  { code: 'PING', label: 'Ping' },
  { code: 'TRACEROUTE', label: 'Traceroute' },
  { code: 'WIFI_SCAN', label: 'WiFi Scan' },
  { code: 'OPTICAL_POWER', label: 'Optical Power' },
  { code: 'SPEED_TEST', label: 'Speed Test' },
]

function DiagnosticsTab({ deviceId, canTrigger }: { deviceId: number; canTrigger: boolean }) {
  const { data, isLoading } = useDeviceDiagnostics(deviceId)
  const [diagnosticType, setDiagnosticType] = useState('PING')
  const [host, setHost] = useState('')
  const triggerMutation = useTriggerDiagnostic()
  const [error, setError] = useState<string | null>(null)

  const diagnostics = data?.data ?? []

  async function handleTrigger() {
    setError(null)
    try {
      await triggerMutation.mutateAsync({
        deviceId,
        diagnosticType,
        parameters: host ? { host } : undefined,
      })
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Gagal memicu diagnostic')
    }
  }

  return (
    <div className="space-y-4">
      {canTrigger && (
        <Card>
          <div className="flex flex-wrap items-end gap-3 p-4">
            <div>
              <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Tipe Diagnostic</label>
              <select value={diagnosticType} onChange={(e) => setDiagnosticType(e.target.value)} className={`${inputCls} w-48`}>
                {DIAGNOSTIC_TYPES.map((d) => (
                  <option key={d.code} value={d.code}>
                    {d.label}
                  </option>
                ))}
              </select>
            </div>
            {(diagnosticType === 'PING' || diagnosticType === 'TRACEROUTE') && (
              <div>
                <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Host</label>
                <input value={host} onChange={(e) => setHost(e.target.value)} placeholder="8.8.8.8" className={`${inputCls} w-48`} />
              </div>
            )}
            <button onClick={handleTrigger} disabled={triggerMutation.isPending} className={primaryBtnCls}>
              <Activity className="h-4 w-4" /> Jalankan
            </button>
          </div>
          {error && <p className="px-4 pb-3 text-sm text-red-600">{error}</p>}
        </Card>
      )}

      {isLoading ? (
        <PageSpinner />
      ) : diagnostics.length === 0 ? (
        <EmptyState icon={Activity} title="Belum ada diagnostic" description="Trigger test PING/TRACEROUTE/WiFi Scan bawaan TR-069 di atas." />
      ) : (
        <Card>
          <ul className="divide-y divide-slate-100 dark:divide-slate-800">
            {diagnostics.map((d) => {
              const result = formatJSONField(d.result)
              return (
                <li key={d.id} className="px-5 py-3">
                  <div className="flex items-center justify-between gap-4 text-sm">
                    <div className="flex items-center gap-3">
                      <StatusBadge code={d.status === 'COMPLETED' ? 'COMPLETED' : d.status === 'FAILED' ? 'FAILED' : 'PENDING'} label={d.diagnostic_type} />
                    </div>
                    <span className="shrink-0 text-xs text-slate-400 dark:text-slate-500">{formatDateTime(d.executed_at ?? d.created_at)}</span>
                  </div>
                  {result && <pre className="mt-2 max-h-40 overflow-auto rounded-lg bg-slate-900 p-3 text-xs text-slate-200">{result}</pre>}
                </li>
              )
            })}
          </ul>
        </Card>
      )}
    </div>
  )
}

function FirmwareTab({ deviceId, vendorId, canSchedule }: { deviceId: number; vendorId: number | null; canSchedule: boolean }) {
  const { data: jobsResp, isLoading } = useFirmwareJobs(deviceId)
  const { data: firmwareResp } = useFirmwareList(vendorId ?? undefined)
  const { data: filesResp } = useFiles({ pageSize: 100 })
  const { data: statusRefs } = useRefs('ref_task_status')
  
  const [firmwareId, setFirmwareId] = useState('')
  const [fileId, setFileId] = useState('')
  const [error, setError] = useState<string | null>(null)
  
  const scheduleMutation = useScheduleFirmwareUpgrade()
  const pushFileMutation = usePushFileToDevice()

  const jobs = jobsResp?.data ?? []
  const firmwareOptions = firmwareResp?.data ?? []
  const filesOptions = (filesResp?.data ?? []).filter(f => f.file_type !== 'FIRMWARE' && (!f.vendor_id || f.vendor_id === vendorId))

  async function handleSchedule() {
    setError(null)
    try {
      await scheduleMutation.mutateAsync({ deviceId, firmwareId: Number(firmwareId) })
      setFirmwareId('')
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Gagal menjadwalkan upgrade')
    }
  }

  async function handlePushFile() {
    setError(null)
    try {
      await pushFileMutation.mutateAsync({ deviceId, fileId: Number(fileId) })
      setFileId('')
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Gagal mengirim file')
    }
  }

  return (
    <div className="space-y-4">
      {canSchedule && (
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
          <Card>
            <div className="flex flex-col gap-3 p-4 h-full">
              <label className="text-xs font-medium text-slate-600 dark:text-slate-400">Firmware Upgrade</label>
              <select value={firmwareId} onChange={(e) => setFirmwareId(e.target.value)} disabled={!vendorId} className={inputCls}>
                <option value="">{vendorId ? 'Pilih firmware...' : 'Vendor device belum diketahui'}</option>
                {firmwareOptions.map((f) => (
                  <option key={f.id} value={f.id}>
                    {f.version} — {f.file_name}
                  </option>
                ))}
              </select>
              <div className="mt-auto pt-2">
                <button onClick={handleSchedule} disabled={!firmwareId || scheduleMutation.isPending} className={`${primaryBtnCls} w-full`}>
                  <HardDrive className="h-4 w-4" /> Jadwalkan Upgrade
                </button>
              </div>
            </div>
          </Card>
          
          <Card>
            <div className="flex flex-col gap-3 p-4 h-full">
              <label className="text-xs font-medium text-slate-600 dark:text-slate-400">Push Configuration File</label>
              <select value={fileId} onChange={(e) => setFileId(e.target.value)} className={inputCls}>
                <option value="">Pilih file konfigurasi...</option>
                {filesOptions.map((f) => (
                  <option key={f.id} value={f.id}>
                    {f.file_name} ({f.file_type})
                  </option>
                ))}
              </select>
              <div className="mt-auto pt-2">
                <button onClick={handlePushFile} disabled={!fileId || pushFileMutation.isPending} className={`${primaryBtnCls} w-full`}>
                  <FileUp className="h-4 w-4" /> Push ke Device
                </button>
              </div>
            </div>
          </Card>
        </div>
      )}
      {error && <p className="text-sm text-red-600">{error}</p>}

      {isLoading ? (
        <PageSpinner />
      ) : jobs.length === 0 ? (
        <EmptyState icon={HardDrive} title="Belum ada histori upgrade firmware" />
      ) : (
        <Card>
          <table className="w-full text-left text-sm">
            <thead>
              <tr className="border-b border-slate-200 bg-slate-50 text-xs font-medium uppercase tracking-wide text-slate-500 dark:border-slate-800 dark:bg-slate-800/50 dark:text-slate-400">
                <th className="px-5 py-3">Status</th>
                <th className="px-5 py-3">Dari → Ke</th>
                <th className="px-5 py-3">Dijadwalkan</th>
                <th className="px-5 py-3">Selesai</th>
                <th className="px-5 py-3">Error</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
              {jobs.map((j) => {
                const status = findRefById(statusRefs, j.task_status_id)
                return (
                  <tr key={j.id}>
                    <td className="px-5 py-3">
                      <StatusBadge code={status?.code} label={status?.name ?? '-'} />
                    </td>
                    <td className="px-5 py-3 font-mono text-xs text-slate-600 dark:text-slate-400">
                      {j.from_version ?? '-'} → {j.to_version ?? '-'}
                    </td>
                    <td className="px-5 py-3 text-slate-500 dark:text-slate-400">{formatDateTime(j.scheduled_at)}</td>
                    <td className="px-5 py-3 text-slate-500 dark:text-slate-400">{formatDateTime(j.completed_at)}</td>
                    <td className="max-w-xs truncate px-5 py-3 text-xs text-red-600">{j.error_message ?? '-'}</td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </Card>
      )}
    </div>
  )
}

function TimelineTab({ deviceId }: { deviceId: number }) {
  const { data, isLoading } = useDeviceActivity(deviceId)
  const logs = data?.data ?? []

  if (isLoading) return <PageSpinner />
  if (logs.length === 0) {
    return (
      <EmptyState
        icon={ScrollText}
        title="Belum ada aktivitas tercatat"
        description="Perubahan data device, penerapan provisioning profile, dan match zero-touch akan tercatat di sini. Histori event CWMP dan task RPC ada di tab masing-masing."
      />
    )
  }

  return (
    <Card>
      <ul className="divide-y divide-slate-100 dark:divide-slate-800">
        {logs.map((log) => (
          <li key={log.id} className="flex items-start gap-3 px-5 py-3.5">
            <div className="mt-1 h-1.5 w-1.5 shrink-0 rounded-full bg-slate-300" />
            <div className="min-w-0 flex-1">
              <p className="text-sm text-slate-800">
                <span className="font-medium">{log.username ?? 'Sistem'}</span>{' '}
                {(ACTIVITY_LABELS[log.action] ?? log.action).toLowerCase()}
              </p>
              {log.description && <p className="mt-0.5 text-xs text-slate-500 dark:text-slate-400">{log.description}</p>}
            </div>
            <span className="shrink-0 text-xs text-slate-400 dark:text-slate-500">{formatDateTime(log.created_at)}</span>
          </li>
        ))}
      </ul>
    </Card>
  )
}
