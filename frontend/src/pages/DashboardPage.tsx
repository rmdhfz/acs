import type { ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { useQueryClient } from '@tanstack/react-query'
import { Bar, BarChart, CartesianGrid, Cell, Pie, PieChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import {
  Activity,
  AlertTriangle,
  ArrowRight,
  Building2,
  CheckCircle2,
  ClipboardList,
  Clock3,
  FileSliders,
  HardDrive,
  LayoutGrid,
  RefreshCw,
  Router,
  Search,
  Users,
  Wifi,
  WifiOff,
  Zap,
} from 'lucide-react'
import { StatCard } from '../components/StatCard'
import { FleetInsights } from '../components/FleetInsights'
import { StatusBadge } from '../components/StatusBadge'
import { PageSpinner } from '../components/Spinner'
import { EmptyState } from '../components/EmptyState'
import { useAuth } from '../lib/auth'
import { useI18n } from '../lib/i18n'
import { useTheme } from '../lib/theme'
import { useWebSocket } from '../lib/ws'
import { formatRelativeTime } from '../lib/format'
import {
  findRefById,
  findRefIdByCode,
  useActivityLog,
  useCurrentTenant,
  useCwmpSessionsCount,
  useDeviceStats,
  useDevices,
  useFirmwareRolloutBatches,
  useRefs,
  useTaskStats,
  useTasks,
  useVendors,
  useWebhookFailedCount,
} from '../lib/hooks'

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

const cardCls = 'rounded-3xl border border-slate-200 bg-white dark:border-slate-800 dark:bg-slate-900'
const linkCls = 'inline-flex items-center gap-1 rounded-full px-3 py-1.5 text-sm font-medium text-indigo-700 hover:bg-indigo-50 dark:text-indigo-300 dark:hover:bg-indigo-500/10'

function CardHeader({ title, to, toLabel }: { title: string; to?: string; toLabel?: string }) {
  return (
    <div className="mb-4 flex items-center justify-between gap-3">
      <h2 className="text-base font-medium text-slate-900 dark:text-slate-100">{title}</h2>
      {to && toLabel && (
        <Link to={to} className={linkCls}>
          {toLabel} <ArrowRight className="h-3 w-3" />
        </Link>
      )}
    </div>
  )
}

function Empty({ children }: { children: ReactNode }) {
  return <p className="py-8 text-center text-sm text-slate-500 dark:text-slate-400">{children}</p>
}

function greetingKey(hour: number) {
  if (hour < 4) return 'dash.greet.night'
  if (hour < 11) return 'dash.greet.morning'
  if (hour < 15) return 'dash.greet.afternoon'
  if (hour < 19) return 'dash.greet.evening'
  return 'dash.greet.night'
}

interface Alert {
  key: string
  text: string
  to: string
}

// Baris daftar bergaya ilustrasi beranda: latar abu lembut, titik status berwarna, teks utama + keterangan, waktu di kanan.
function ListRow({ to, color, title, sub, right }: { to: string; color: string; title: string; sub?: string; right?: string }) {
  return (
    <Link to={to} className="flex items-center gap-3 rounded-2xl bg-slate-50 px-4 py-3 transition-colors hover:bg-slate-100 focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-indigo-600 dark:bg-slate-800/50 dark:hover:bg-slate-800">
      <span className="h-2.5 w-2.5 shrink-0 rounded-full" style={{ backgroundColor: color }} aria-hidden="true" />
      <span className="min-w-0 flex-1">
        <span className="block truncate text-sm font-medium text-slate-900 dark:text-slate-100">{title}</span>
        {sub && <span className="block truncate text-xs text-slate-600 dark:text-slate-400">{sub}</span>}
      </span>
      {right && <span className="shrink-0 text-xs text-slate-600 dark:text-slate-400">{right}</span>}
    </Link>
  )
}

// Grafik batang dua-nada seperti ilustrasi: latar biru muda = seluruh task per jam, biru tua = yang berhasil.
// Dihitung di klien dari task terbaru (maks. 200) — bukan agregat server — sehingga pada tenant yang sangat sibuk
// jendela efektifnya lebih pendek dari 24 jam; keterangan di bawah grafik menyatakan hal itu.
const HOURS = 24
function TaskActivity({ tasks, completedId, failedId, loading, isDark }: { tasks: { created_at: string; task_status_id: number }[]; completedId: number | undefined; failedId: number | undefined; loading: boolean; isDark: boolean }) {
  const now = Date.now()
  const buckets = Array.from({ length: HOURS }, (_, i) => ({ start: now - (HOURS - i) * 3600_000, total: 0, ok: 0, failed: 0 }))
  for (const t of tasks) {
    const age = now - new Date(t.created_at).getTime()
    if (age < 0 || age >= HOURS * 3600_000) continue
    const b = buckets[HOURS - 1 - Math.floor(age / 3600_000)]
    b.total++
    if (t.task_status_id === completedId) b.ok++
    if (t.task_status_id === failedId) b.failed++
  }
  const max = Math.max(1, ...buckets.map((b) => b.total))
  const total = buckets.reduce((n, b) => n + b.total, 0)
  const ok = buckets.reduce((n, b) => n + b.ok, 0)
  const failed = buckets.reduce((n, b) => n + b.failed, 0)
  const truncated = tasks.length >= 200
  if (loading) return <PageSpinner />
  if (total === 0) return <Empty>Belum ada task dalam 24 jam terakhir.</Empty>
  return (
    <figure>
      <div className="flex h-44 items-end gap-1.5 sm:gap-2" role="img" aria-label={`Task per jam, 24 jam terakhir: ${total} task, ${ok} berhasil, ${failed} gagal`}>
        {buckets.map((b, i) => {
          const h = b.total === 0 ? 2 : Math.max(8, (b.total / max) * 100)
          const okPct = b.total === 0 ? 0 : (b.ok / b.total) * 100
          const hh = new Date(b.start + 3600_000).getHours().toString().padStart(2, '0')
          return (
            <div key={i} className="flex h-full min-w-0 flex-1 flex-col justify-end" title={`${hh}.00 — ${b.total} task (${b.ok} berhasil, ${b.failed} gagal)`}>
              <div className={`relative w-full overflow-hidden rounded-t-lg ${isDark ? 'bg-indigo-900/60' : 'bg-indigo-200'}`} style={{ height: `${h}%` }}>
                <div className="absolute inset-x-0 bottom-0 rounded-t-lg bg-indigo-600" style={{ height: `${okPct}%` }} />
              </div>
            </div>
          )
        })}
      </div>
      <div className="mt-2 flex justify-between text-[11px] text-slate-600 dark:text-slate-400" aria-hidden="true">
        {[0, 6, 12, 18, 23].map((i) => (
          <span key={i}>{new Date(buckets[i].start + 3600_000).getHours().toString().padStart(2, '0')}.00</span>
        ))}
      </div>
      <figcaption className="mt-3 flex flex-wrap items-center gap-x-5 gap-y-1 text-xs text-slate-600 dark:text-slate-400">
        <span className="inline-flex items-center gap-1.5"><span className={`h-2.5 w-2.5 rounded-sm ${isDark ? 'bg-indigo-900/60' : 'bg-indigo-200'}`} />Semua task ({total})</span>
        <span className="inline-flex items-center gap-1.5"><span className="h-2.5 w-2.5 rounded-sm bg-indigo-600" />Berhasil ({ok})</span>
        {failed > 0 && <span className="inline-flex items-center gap-1.5"><span className="h-2.5 w-2.5 rounded-full bg-red-600" />Gagal ({failed})</span>}
        {truncated && <span>· berdasarkan 200 task terbaru</span>}
      </figcaption>
    </figure>
  )
}

export function DashboardPage() {
  const { user, hasRole } = useAuth()
  const { t } = useI18n()
  const { resolvedTheme } = useTheme()
  const { connected } = useWebSocket()
  const queryClient = useQueryClient()
  const isDark = resolvedTheme === 'dark'
  const canOps = hasRole('ADMIN', 'NOC')
  const isAdmin = hasRole('ADMIN')
  const isSuper = hasRole('SUPERADMIN')

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

  // ---- referensi
  const { data: statusRefs } = useRefs('ref_device_status')
  const { data: taskStatusRefs } = useRefs('ref_task_status')
  const { data: taskTypeRefs } = useRefs('ref_task_types')
  const { data: rolloutStatusRefs } = useRefs('ref_firmware_rollout_status')
  const { data: vendorsResp } = useVendors()
  const vendors = vendorsResp?.data ?? []
  const { data: tenant } = useCurrentTenant()

  // ---- data
  const { data: deviceStats, isLoading: deviceStatsLoading, dataUpdatedAt } = useDeviceStats()
  const { data: taskStats, isLoading: taskStatsLoading } = useTaskStats()
  const { data: cwmpSessions } = useCwmpSessionsCount(canOps)
  const { data: failedWebhooks } = useWebhookFailedCount(isAdmin)
  const { data: rollouts } = useFirmwareRolloutBatches()

  const onlineId = findRefIdByCode(statusRefs, 'ONLINE')
  const offlineId = findRefIdByCode(statusRefs, 'OFFLINE')
  const faultyId = findRefIdByCode(statusRefs, 'FAULTY')
  const pendingTaskId = findRefIdByCode(taskStatusRefs, 'PENDING')
  const failedTaskId = findRefIdByCode(taskStatusRefs, 'FAILED')

  const { data: faultyDevices } = useDevices({ device_status_id: faultyId, page_size: 5 }, faultyId !== undefined)
  const { data: offlineDevices } = useDevices({ device_status_id: offlineId, page_size: 6 }, offlineId !== undefined)
  const { data: recentTasks, isLoading: recentTasksLoading } = useTasks({ page_size: 8 })
  const { data: activityTasks, isLoading: activityTasksLoading } = useTasks({ page_size: 200 })
  const completedTaskId = findRefIdByCode(taskStatusRefs, 'COMPLETED')
  const { data: activity } = useActivityLog({ page_size: 8 }, isAdmin)

  const byStatus = deviceStats?.by_status ?? []
  const byVendor = deviceStats?.by_vendor ?? []
  const totalDevices = byStatus.reduce((sum, s) => sum + s.count, 0)
  const countOf = (id: number | undefined) => byStatus.find((s) => s.device_status_id === id)?.count ?? 0
  const onlineCount = countOf(onlineId)
  const offlineCount = countOf(offlineId)
  const faultyCount = countOf(faultyId)
  const onlinePct = totalDevices > 0 ? Math.round((onlineCount / totalDevices) * 100) : 0
  const pendingTaskCount = taskStats?.find((s) => s.task_status_id === pendingTaskId)?.count ?? 0
  const failedTaskCount = taskStats?.find((s) => s.task_status_id === failedTaskId)?.count ?? 0
  const activeSessionsCount = cwmpSessions?.count ?? 0

  const statusChartData = byStatus.map((s) => {
    const ref = findRefById(statusRefs, s.device_status_id)
    return { name: ref?.name ?? `#${s.device_status_id}`, code: ref?.code, value: s.count }
  })

  const vendorRows = byVendor
    .map((v) => ({
      name: v.vendor_id ? (vendors.find((x) => x.id === v.vendor_id)?.name ?? `#${v.vendor_id}`) : t('dash.vendor.unknown'),
      value: v.count,
    }))
    .sort((x, y) => y.value - x.value)
  const maxVendor = vendorRows[0]?.value ?? 1

  const taskChartData = (taskStats ?? []).map((s) => {
    const ref = findRefById(taskStatusRefs, s.task_status_id)
    return { name: ref?.name ?? `#${s.task_status_id}`, code: ref?.code, value: s.count }
  })

  const activeRollouts = (rollouts?.data ?? []).filter((b) => {
    const code = findRefById(rolloutStatusRefs, b.status_id)?.code
    return code === 'IN_PROGRESS' || code === 'PENDING' || code === 'PAUSED_FAILURE_THRESHOLD'
  })
  const pausedRollouts = activeRollouts.filter((b) => findRefById(rolloutStatusRefs, b.status_id)?.code === 'PAUSED_FAILURE_THRESHOLD').length

  // Perangkat yang butuh perhatian: Faulty dulu, lalu Offline.
  const attentionDevices = [...(faultyDevices?.data ?? []), ...(offlineDevices?.data ?? [])].slice(0, 7)

  const alerts: Alert[] = []
  if (faultyCount > 0) alerts.push({ key: 'faulty', text: t('dash.attn.faulty', { n: faultyCount }), to: '/devices?status=FAULTY' })
  if (failedTaskCount > 0) alerts.push({ key: 'failed', text: t('dash.attn.failedTasks', { n: failedTaskCount }), to: '/tasks?status=FAILED' })
  if (isAdmin && (failedWebhooks?.count ?? 0) > 0) alerts.push({ key: 'wh', text: t('dash.attn.webhooks', { n: failedWebhooks?.count ?? 0 }), to: '/webhooks' })
  if (pausedRollouts > 0) alerts.push({ key: 'rollout', text: t('dash.attn.rollouts', { n: pausedRollouts }), to: '/firmware' })
  // Offline dianggap wajar bila kecil; sorot hanya bila ≥ 20% armada (dan ada device).
  if (totalDevices > 0 && offlineCount / totalDevices >= 0.2) alerts.push({ key: 'offline', text: t('dash.attn.offline', { n: offlineCount }), to: '/devices?status=OFFLINE' })

  const loadingAll = deviceStatsLoading || taskStatsLoading
  const scope = user?.tenant_id ? (tenant?.brand_name || tenant?.name || `#${user.tenant_id}`) : t('dash.scopeAll')

  const quickActions = [
    { to: '/devices', icon: LayoutGrid, label: t('dash.quick.devices'), show: true },
    { to: '/devices?status=OFFLINE', icon: WifiOff, label: t('dash.quick.offline'), show: true },
    { to: '/provisioning', icon: FileSliders, label: t('dash.quick.provision'), show: true },
    { to: '/firmware', icon: HardDrive, label: t('dash.quick.firmware'), show: true },
    { to: '/administration/onboarding', icon: Building2, label: t('dash.quick.onboarding'), show: isSuper },
    { to: '/administration', icon: Users, label: t('dash.quick.users'), show: isAdmin && !isSuper },
  ].filter((a) => a.show)

  return (
    <div className="mx-auto max-w-7xl px-6 py-8">
      {/* Header */}
      <div className="mb-6 flex flex-wrap items-start justify-between gap-4">
        <div>
          <h1 className="text-2xl font-normal text-slate-900 dark:text-slate-100">
            {t(greetingKey(new Date().getHours()))}, {user?.username}
          </h1>
          <p className="mt-0.5 text-sm text-slate-500 dark:text-slate-400">
            <span className="font-medium text-slate-700 dark:text-slate-300">{scope}</span> · {t('dash.subtitle')}
          </p>
        </div>
        <div className="flex items-center gap-2">
          <span
            title={connected ? t('dash.liveTip') : t('dash.pollingTip')}
            className={`inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs font-medium ${
              connected ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-400' : 'bg-slate-100 text-slate-500 dark:bg-slate-800 dark:text-slate-400'
            }`}
          >
            <span className="relative flex h-1.5 w-1.5">
              {connected && <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-emerald-500 opacity-75" />}
              <span className={`relative inline-flex h-1.5 w-1.5 rounded-full ${connected ? 'bg-emerald-500' : 'bg-slate-400'}`} />
            </span>
            {connected ? t('dash.live') : t('dash.polling')}
          </span>
          {dataUpdatedAt > 0 && (
            <span className="hidden text-xs text-slate-500 sm:inline dark:text-slate-400">
              {t('dash.updated', { time: formatRelativeTime(new Date(dataUpdatedAt).toISOString()) })}
            </span>
          )}
          <button
            onClick={() => queryClient.invalidateQueries()}
            title={t('dash.refresh')}
            aria-label={t('dash.refresh')}
            className="flex h-8 w-8 items-center justify-center rounded-full border border-slate-200 text-slate-500 transition-colors hover:bg-slate-50 hover:text-slate-900 dark:border-slate-700 dark:text-slate-400 dark:hover:bg-slate-800 dark:hover:text-slate-100"
          >
            <RefreshCw className="h-4 w-4" />
          </button>
        </div>
      </div>

      {/* Status kesehatan / peringatan */}
      {!loadingAll &&
        (alerts.length === 0 ? (
          <div className="mb-6 flex items-start gap-3 rounded-xl border border-emerald-200 bg-emerald-50 px-4 py-3 dark:border-emerald-500/20 dark:bg-emerald-500/10" role="status">
            <CheckCircle2 className="mt-0.5 h-5 w-5 shrink-0 text-emerald-600 dark:text-emerald-400" />
            <div>
              <p className="text-sm font-semibold text-emerald-800 dark:text-emerald-300">{t('dash.allGood')}</p>
              <p className="text-xs text-emerald-700/80 dark:text-emerald-400/80">{t('dash.allGoodDesc')}</p>
            </div>
          </div>
        ) : (
          <div className="mb-6 rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 dark:border-amber-500/20 dark:bg-amber-500/10" role="alert">
            <p className="flex items-center gap-2 text-sm font-semibold text-amber-800 dark:text-amber-300">
              <AlertTriangle className="h-4 w-4" /> {t('dash.attention')}
            </p>
            <ul className="mt-2 flex flex-wrap gap-2">
              {alerts.map((a) => (
                <li key={a.key}>
                  <Link
                    to={a.to}
                    className="inline-flex items-center gap-1.5 rounded-full bg-white px-3 py-1 text-xs font-medium text-amber-800 shadow-sm ring-1 ring-amber-200 transition-colors hover:bg-amber-100 dark:bg-slate-900 dark:text-amber-300 dark:ring-amber-500/30 dark:hover:bg-slate-800"
                  >
                    {a.text} <ArrowRight className="h-3 w-3" />
                  </Link>
                </li>
              ))}
            </ul>
          </div>
        ))}

      {/* KPI utama */}
      <div className="mb-4 grid grid-cols-1 gap-4 sm:grid-cols-3">
        <StatCard large label={t('dash.kpi.total')} value={totalDevices} icon={Router} tone="blue" loading={deviceStatsLoading} to="/devices" hint={offlineCount > 0 ? `${offlineCount} offline` : undefined} />
        <StatCard
          large
          label={t('dash.kpi.online')}
          value={onlineCount}
          icon={Wifi}
          tone="emerald"
          loading={deviceStatsLoading}
          hint={totalDevices > 0 ? t('dash.kpi.ofTotal', { pct: onlinePct }) : undefined}
          to="/devices?status=ONLINE"
        />
        <StatCard
          large
          label="Task gagal"
          value={failedTaskCount}
          icon={AlertTriangle}
          tone={failedTaskCount > 0 ? 'red' : 'default'}
          loading={taskStatsLoading}
          hint={failedTaskCount > 0 ? 'Perlu ditinjau' : 'Tidak ada yang gagal'}
          to="/tasks?status=FAILED"
        />
      </div>

      {/* Ringkasan lain: chip tautan, agar kartu utama tetap fokus */}
      <ul className="mb-6 flex flex-wrap gap-2" aria-label="Ringkasan lain">
        {[
          { to: '/devices?status=OFFLINE', icon: WifiOff, text: `${offlineCount} offline`, show: true },
          { to: '/devices?status=FAULTY', icon: AlertTriangle, text: `${faultyCount} bermasalah`, show: true },
          { to: '/tasks?status=PENDING', icon: Clock3, text: `${pendingTaskCount} task menunggu`, show: true },
          { to: '/tasks', icon: Activity, text: `${activeSessionsCount} sesi CWMP aktif`, show: canOps },
        ]
          .filter((c) => c.show)
          .map((c) => (
            <li key={c.text}>
              <Link to={c.to} className="inline-flex h-9 items-center gap-2 rounded-full border border-slate-200 bg-white px-4 text-sm text-slate-700 transition-colors hover:bg-slate-50 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-300 dark:hover:bg-slate-800">
                <c.icon className="h-4 w-4 text-slate-500 dark:text-slate-400" aria-hidden="true" /> {c.text}
              </Link>
            </li>
          ))}
      </ul>

      {/* Wawasan armada: mode PON, redaman RX, suhu ONU, uptime, lama offline */}
      <FleetInsights tooltipStyle={tooltipStyle} />

      <div className="grid grid-cols-1 gap-5 lg:grid-cols-3">
        {/* Aktivitas task 24 jam */}
        <section className={`${cardCls} p-6 lg:col-span-2`}>
          <CardHeader title="Aktivitas task 24 jam terakhir" to="/tasks" toLabel={t('dash.recentTasks.all')} />
          <TaskActivity tasks={activityTasks?.data ?? []} completedId={completedTaskId} failedId={failedTaskId} loading={activityTasksLoading} isDark={isDark} />
        </section>

        {/* Kesehatan armada */}
        <section className={`${cardCls} p-6`}>
          <CardHeader title={t('dash.health')} />
          {deviceStatsLoading ? (
            <PageSpinner />
          ) : statusChartData.length === 0 || totalDevices === 0 ? (
            <EmptyState icon={Router} title={t('dash.health.noDevices')} description={t('dash.health.noDevicesDesc')} />
          ) : (
            <>
              <div className="relative">
                <ResponsiveContainer width="100%" height={200}>
                  <PieChart>
                    <Pie data={statusChartData} dataKey="value" nameKey="name" innerRadius={62} outerRadius={90} paddingAngle={2} stroke="none">
                      {statusChartData.map((entry, i) => (
                        <Cell key={i} fill={STATUS_COLORS[entry.code ?? ''] ?? DEFAULT_COLOR} />
                      ))}
                    </Pie>
                    <Tooltip contentStyle={tooltipStyle} />
                  </PieChart>
                </ResponsiveContainer>
                <div className="pointer-events-none absolute inset-0 flex flex-col items-center justify-center">
                  <span className="text-3xl font-semibold tabular-nums text-slate-900 dark:text-slate-100">{onlinePct}%</span>
                  <span className="text-xs text-slate-500 dark:text-slate-400">{t('dash.health.online')}</span>
                </div>
              </div>
              <ul className="mt-3 space-y-1.5">
                {statusChartData.map((s) => (
                  <li key={s.name} className="flex items-center justify-between text-sm">
                    <span className="flex items-center gap-2 text-slate-600 dark:text-slate-400">
                      <span className="h-2 w-2 rounded-full" style={{ backgroundColor: STATUS_COLORS[s.code ?? ''] ?? DEFAULT_COLOR }} />
                      {s.name}
                    </span>
                    <span className="tabular-nums text-slate-900 dark:text-slate-100">
                      {s.value}
                      <span className="ml-1.5 text-xs text-slate-500 dark:text-slate-400">{Math.round((s.value / totalDevices) * 100)}%</span>
                    </span>
                  </li>
                ))}
              </ul>
            </>
          )}
        </section>

        {/* Device perlu perhatian */}
        <section className={`${cardCls} p-6 lg:col-span-2`}>
          <CardHeader title={t('dash.needAttention')} to="/devices" toLabel={t('dash.needAttention.all')} />
          {attentionDevices.length === 0 ? (
            <Empty>{deviceStatsLoading ? '…' : t('dash.needAttention.empty')}</Empty>
          ) : (
            <ul className="space-y-2">
              {attentionDevices.map((d) => {
                const st = findRefById(statusRefs, d.device_status_id)
                return (
                  <li key={d.id}>
                    <ListRow
                      to={`/devices/${d.id}`}
                      color={STATUS_COLORS[st?.code ?? ''] ?? DEFAULT_COLOR}
                      title={d.serial_number}
                      sub={`${vendors.find((v) => v.id === d.vendor_id)?.name ?? t('dash.vendor.unknown')} · ${st?.name ?? '—'}`}
                      right={d.last_inform_at ? formatRelativeTime(d.last_inform_at) : t('dash.never')}
                    />
                  </li>
                )
              })}
            </ul>
          )}
        </section>

        {/* Aksi cepat */}
        <section className={`${cardCls} p-6`}>
          <CardHeader title={t('dash.quick')} />
          <ul className="space-y-1.5">
            {quickActions.map((a) => (
              <li key={a.to + a.label}>
                <Link
                  to={a.to}
                  className="flex items-center gap-3 rounded-full px-3 py-2 text-sm font-medium text-slate-700 transition-colors hover:bg-slate-100 dark:text-slate-300 dark:hover:bg-slate-800"
                >
                  <span className="flex h-7 w-7 items-center justify-center rounded-md bg-indigo-50 text-indigo-600 dark:bg-indigo-500/10 dark:text-indigo-400">
                    <a.icon className="h-4 w-4" />
                  </span>
                  {a.label}
                  <ArrowRight className="ml-auto h-3.5 w-3.5 text-slate-300 dark:text-slate-600" />
                </Link>
              </li>
            ))}
            <li className="flex items-center gap-3 px-3 py-2 text-sm text-slate-500 dark:text-slate-400">
              <span className="flex h-7 w-7 items-center justify-center rounded-md bg-slate-100 text-slate-500 dark:bg-slate-800 dark:text-slate-400">
                <Search className="h-4 w-4" />
              </span>
              {t('dash.quick.search')}
            </li>
          </ul>
        </section>

        {/* Task terbaru */}
        <section className={`${cardCls} p-6 lg:col-span-2`}>
          <CardHeader title={t('dash.recentTasks')} to="/tasks" toLabel={t('dash.recentTasks.all')} />
          {recentTasksLoading ? (
            <PageSpinner />
          ) : (recentTasks?.data ?? []).length === 0 ? (
            <Empty>{t('dash.recentTasks.empty')}</Empty>
          ) : (
            <ul className="space-y-2">
              {(recentTasks?.data ?? []).map((task) => {
                const st = findRefById(taskStatusRefs, task.task_status_id)
                const type = findRefById(taskTypeRefs, task.task_type_id)
                return (
                  <li key={task.id}>
                    <ListRow
                      to={`/devices/${task.device_id}`}
                      color={STATUS_COLORS[st?.code ?? ''] ?? DEFAULT_COLOR}
                      title={type?.name ?? `#${task.task_type_id}`}
                      sub={`Perangkat #${task.device_id} · ${st?.name ?? '—'}`}
                      right={formatRelativeTime(task.created_at)}
                    />
                  </li>
                )
              })}
            </ul>
          )}
        </section>

        {/* Antrean task */}
        <section className={`${cardCls} p-6`}>
          <CardHeader title={t('dash.queue')} to="/tasks" toLabel={t('dash.recentTasks.all')} />
          {taskStatsLoading ? (
            <PageSpinner />
          ) : taskChartData.length === 0 ? (
            <EmptyState icon={ClipboardList} title={t('dash.noTasks')} />
          ) : (
            <ResponsiveContainer width="100%" height={220}>
              <BarChart data={taskChartData} layout="vertical" margin={{ left: 8, right: 16 }}>
                <CartesianGrid strokeDasharray="3 3" stroke={gridStroke} horizontal={false} />
                <XAxis type="number" allowDecimals={false} tick={{ fontSize: 12, fill: tickFill }} />
                <YAxis type="category" dataKey="name" width={100} tick={{ fontSize: 12, fill: tickFill }} />
                <Tooltip contentStyle={tooltipStyle} cursor={{ fill: cursorFill }} />
                <Bar dataKey="value" radius={[0, 4, 4, 0]}>
                  {taskChartData.map((entry, i) => (
                    <Cell key={i} fill={STATUS_COLORS[entry.code ?? ''] ?? DEFAULT_COLOR} />
                  ))}
                </Bar>
              </BarChart>
            </ResponsiveContainer>
          )}
        </section>

        {/* Vendor */}
        <section className={`${cardCls} p-6 lg:col-span-2`}>
          <CardHeader title={t('dash.vendor')} />
          {deviceStatsLoading ? (
            <PageSpinner />
          ) : vendorRows.length === 0 || totalDevices === 0 ? (
            <Empty>{t('dash.health.noDevices')}</Empty>
          ) : (
            <ul className="space-y-3">
              {vendorRows.slice(0, 8).map((v) => (
                <li key={v.name}>
                  <div className="mb-1 flex items-center justify-between text-sm">
                    <span className="truncate text-slate-700 dark:text-slate-300">{v.name}</span>
                    <span className="tabular-nums text-slate-900 dark:text-slate-100">
                      {v.value}
                      <span className="ml-1.5 text-xs text-slate-500 dark:text-slate-400">{Math.round((v.value / totalDevices) * 100)}%</span>
                    </span>
                  </div>
                  <div className="h-1.5 overflow-hidden rounded-full bg-slate-100 dark:bg-slate-800">
                    <div className="h-full rounded-full bg-indigo-500" style={{ width: `${Math.max(4, (v.value / maxVendor) * 100)}%` }} />
                  </div>
                </li>
              ))}
            </ul>
          )}
        </section>

        {/* Rollout firmware */}
        <section className={`${cardCls} p-6`}>
          <CardHeader title={t('dash.rollouts')} to="/firmware" toLabel={t('dash.rollouts.open')} />
          {activeRollouts.length === 0 ? (
            <Empty>{t('dash.rollouts.empty')}</Empty>
          ) : (
            <ul className="space-y-3">
              {activeRollouts.slice(0, 4).map((b) => {
                const st = findRefById(rolloutStatusRefs, b.status_id)
                return (
                  <li key={b.id} className="flex items-center justify-between gap-3 rounded-lg border border-slate-100 p-3 dark:border-slate-800">
                    <div className="min-w-0">
                      <p className="flex items-center gap-1.5 text-sm font-medium text-slate-900 dark:text-slate-100">
                        <Zap className="h-3.5 w-3.5 text-indigo-500" /> Batch #{b.id}
                      </p>
                      <p className="mt-0.5 text-xs text-slate-500 dark:text-slate-400">{t('dash.rollouts.wave', { n: b.current_wave, pct: b.wave_percentage })}</p>
                    </div>
                    <StatusBadge code={st?.code === 'PAUSED_FAILURE_THRESHOLD' ? 'FAILED' : st?.code === 'IN_PROGRESS' ? 'SENT' : st?.code} label={st?.name ?? '—'} />
                  </li>
                )
              })}
            </ul>
          )}
        </section>

        {/* Aktivitas (admin) */}
        {isAdmin && (
          <section className={`${cardCls} p-6 lg:col-span-3`}>
            <CardHeader title={t('dash.activity')} to="/audit" toLabel={t('dash.activity.all')} />
            {(activity?.data ?? []).length === 0 ? (
              <Empty>{t('dash.activity.empty')}</Empty>
            ) : (
              <ul className="grid gap-x-8 gap-y-1 md:grid-cols-2">
                {(activity?.data ?? []).map((a) => (
                  <li key={a.id} className="flex min-w-0 items-start gap-3 py-2">
                    <span className="mt-1.5 h-1.5 w-1.5 shrink-0 rounded-full bg-indigo-400" />
                    <div className="min-w-0 flex-1">
                      <p className="truncate text-sm text-slate-700 dark:text-slate-300">
                        <span className="font-medium text-slate-900 dark:text-slate-100">{a.username ?? 'system'}</span> · {a.action}
                        <span className="text-slate-500 dark:text-slate-400"> {a.entity_type}</span>
                      </p>
                      {a.description && <p className="truncate text-xs text-slate-500 dark:text-slate-400">{a.description}</p>}
                    </div>
                    <span className="shrink-0 text-xs text-slate-500 dark:text-slate-400">{formatRelativeTime(a.created_at)}</span>
                  </li>
                ))}
              </ul>
            )}
          </section>
        )}
      </div>
    </div>
  )
}
