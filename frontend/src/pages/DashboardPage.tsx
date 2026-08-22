import { Bar, BarChart, CartesianGrid, Cell, Legend, Pie, PieChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import { ClipboardList, Clock3, Router, Wifi, WifiOff } from 'lucide-react'
import { StatCard } from '../components/StatCard'
import { PageSpinner } from '../components/Spinner'
import { EmptyState } from '../components/EmptyState'
import { findRefById, findRefIdByCode, useDeviceStats, useRefs, useTaskStats, useVendors } from '../lib/hooks'
import { useTheme } from '../lib/theme'

// Warna selaras dengan STATUS_STYLES di components/StatusBadge.tsx supaya
// chart & badge konsisten secara visual di seluruh aplikasi.
const STATUS_COLORS: Record<string, string> = {
  ONLINE: '#10b981',
  COMPLETED: '#10b981',
  OFFLINE: '#94a3b8',
  CANCELLED: '#94a3b8',
  PROVISIONING: '#3b82f6',
  SENT: '#3b82f6',
  QUEUED: '#3b82f6',
  FAULTY: '#ef4444',
  FAILED: '#ef4444',
  UNREGISTERED: '#f59e0b',
  PENDING: '#f59e0b',
  TIMEOUT: '#f97316',
  DECOMMISSIONED: '#a1a1aa',
}
const DEFAULT_COLOR = '#94a3b8'

const cardCls = 'rounded-xl border border-slate-200 bg-white p-5 shadow-sm dark:border-slate-800 dark:bg-slate-900'

export function DashboardPage() {
  const { resolvedTheme } = useTheme()
  const isDark = resolvedTheme === 'dark'
  const gridStroke = isDark ? '#1e293b' : '#f1f5f9'
  const tickFill = isDark ? '#94a3b8' : '#64748b'
  const tooltipStyle = {
    borderRadius: 8,
    borderColor: isDark ? '#334155' : '#e2e8f0',
    backgroundColor: isDark ? '#0f172a' : '#ffffff',
    color: isDark ? '#e2e8f0' : '#0f172a',
    fontSize: 13,
  }
  const cursorFill = isDark ? '#1e293b80' : '#f8fafc'

  const { data: statusRefs } = useRefs('ref_device_status')
  const { data: taskStatusRefs } = useRefs('ref_task_status')
  const { data: vendorsResp } = useVendors()
  const vendors = vendorsResp?.data ?? []

  const { data: deviceStats, isLoading: deviceStatsLoading } = useDeviceStats()
  const { data: taskStats, isLoading: taskStatsLoading } = useTaskStats()

  const onlineId = findRefIdByCode(statusRefs, 'ONLINE')
  const offlineId = findRefIdByCode(statusRefs, 'OFFLINE')
  const pendingTaskId = findRefIdByCode(taskStatusRefs, 'PENDING')

  const byStatus = deviceStats?.by_status ?? []
  const byVendor = deviceStats?.by_vendor ?? []
  const totalDevices = byStatus.reduce((sum, s) => sum + s.count, 0)
  const onlineCount = byStatus.find((s) => s.device_status_id === onlineId)?.count ?? 0
  const offlineCount = byStatus.find((s) => s.device_status_id === offlineId)?.count ?? 0
  const pendingTaskCount = taskStats?.find((t) => t.task_status_id === pendingTaskId)?.count ?? 0

  const statusChartData = byStatus.map((s) => {
    const ref = findRefById(statusRefs, s.device_status_id)
    return { name: ref?.name ?? `#${s.device_status_id}`, code: ref?.code, value: s.count }
  })

  const vendorChartData = byVendor
    .map((v) => ({
      name: v.vendor_id ? vendors.find((vendor) => vendor.id === v.vendor_id)?.name ?? `#${v.vendor_id}` : 'Belum diketahui',
      value: v.count,
    }))
    .sort((a, b) => b.value - a.value)

  const taskChartData = (taskStats ?? []).map((t) => {
    const ref = findRefById(taskStatusRefs, t.task_status_id)
    return { name: ref?.name ?? `#${t.task_status_id}`, code: ref?.code, value: t.count }
  })

  return (
    <div className="mx-auto max-w-7xl px-6 py-8">
      <div className="mb-6">
        <h1 className="text-xl font-semibold text-slate-900 dark:text-slate-100">Dashboard</h1>
        <p className="mt-0.5 text-sm text-slate-500 dark:text-slate-400">Ringkasan status seluruh perangkat CPE dan antrean task secara real-time</p>
      </div>

      <div className="mb-6 grid grid-cols-2 gap-4 lg:grid-cols-4">
        <StatCard label="Total Device" value={totalDevices} icon={Router} tone="default" loading={deviceStatsLoading} />
        <StatCard label="Online" value={onlineCount} icon={Wifi} tone="emerald" loading={deviceStatsLoading} />
        <StatCard label="Offline" value={offlineCount} icon={WifiOff} tone="red" loading={deviceStatsLoading} />
        <StatCard label="Task Pending" value={pendingTaskCount} icon={Clock3} tone="amber" loading={taskStatsLoading} />
      </div>

      <div className="grid grid-cols-1 gap-5 lg:grid-cols-2">
        <div className={cardCls}>
          <h2 className="mb-4 text-sm font-semibold text-slate-900 dark:text-slate-100">Distribusi Status Device</h2>
          {deviceStatsLoading ? (
            <PageSpinner />
          ) : statusChartData.length === 0 || totalDevices === 0 ? (
            <EmptyState icon={Router} title="Belum ada device" />
          ) : (
            <ResponsiveContainer width="100%" height={280}>
              <PieChart>
                <Pie data={statusChartData} dataKey="value" nameKey="name" innerRadius={60} outerRadius={95} paddingAngle={2}>
                  {statusChartData.map((entry, i) => (
                    <Cell key={i} fill={STATUS_COLORS[entry.code ?? ''] ?? DEFAULT_COLOR} />
                  ))}
                </Pie>
                <Tooltip contentStyle={tooltipStyle} />
                <Legend verticalAlign="bottom" height={36} iconType="circle" iconSize={8} wrapperStyle={{ fontSize: 12 }} />
              </PieChart>
            </ResponsiveContainer>
          )}
        </div>

        <div className={cardCls}>
          <h2 className="mb-4 text-sm font-semibold text-slate-900 dark:text-slate-100">Antrean Task per Status</h2>
          {taskStatsLoading ? (
            <PageSpinner />
          ) : taskChartData.length === 0 ? (
            <EmptyState icon={ClipboardList} title="Belum ada task" />
          ) : (
            <ResponsiveContainer width="100%" height={280}>
              <BarChart data={taskChartData} layout="vertical" margin={{ left: 8, right: 16 }}>
                <CartesianGrid strokeDasharray="3 3" stroke={gridStroke} horizontal={false} />
                <XAxis type="number" allowDecimals={false} tick={{ fontSize: 12, fill: tickFill }} />
                <YAxis type="category" dataKey="name" width={110} tick={{ fontSize: 12, fill: tickFill }} />
                <Tooltip contentStyle={tooltipStyle} cursor={{ fill: cursorFill }} />
                <Bar dataKey="value" radius={[0, 4, 4, 0]}>
                  {taskChartData.map((entry, i) => (
                    <Cell key={i} fill={STATUS_COLORS[entry.code ?? ''] ?? DEFAULT_COLOR} />
                  ))}
                </Bar>
              </BarChart>
            </ResponsiveContainer>
          )}
        </div>

        <div className={`${cardCls} lg:col-span-2`}>
          <h2 className="mb-4 text-sm font-semibold text-slate-900 dark:text-slate-100">Device per Vendor</h2>
          {deviceStatsLoading ? (
            <PageSpinner />
          ) : vendorChartData.length === 0 || totalDevices === 0 ? (
            <EmptyState icon={Router} title="Belum ada device" />
          ) : (
            <ResponsiveContainer width="100%" height={260}>
              <BarChart data={vendorChartData} margin={{ top: 8, right: 8, left: -20 }}>
                <CartesianGrid strokeDasharray="3 3" stroke={gridStroke} vertical={false} />
                <XAxis dataKey="name" tick={{ fontSize: 12, fill: tickFill }} />
                <YAxis allowDecimals={false} tick={{ fontSize: 12, fill: tickFill }} />
                <Tooltip contentStyle={tooltipStyle} cursor={{ fill: cursorFill }} />
                <Bar dataKey="value" fill={isDark ? '#e2e8f0' : '#0f172a'} radius={[4, 4, 0, 0]} maxBarSize={56} />
              </BarChart>
            </ResponsiveContainer>
          )}
        </div>
      </div>
    </div>
  )
}
