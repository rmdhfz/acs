import { useMemo, useState } from 'react'
import { Navigate } from 'react-router-dom'
import { useAuth } from '../lib/auth'
import { AlertTriangle, Building2, Download, Printer, RefreshCw, Router, ShieldAlert, Users, Activity, ListX } from 'lucide-react'
import { CartesianGrid, Legend, Line, LineChart, ResponsiveContainer, Tooltip, XAxis, YAxis, Bar, BarChart } from 'recharts'
import { StatCard } from '../components/StatCard'
import { PageSpinner } from '../components/Spinner'
import { EmptyState } from '../components/EmptyState'
import { API_BASE, ApiError, getAuthToken } from '../lib/api'
import { useSaaSReport } from '../lib/hooks'
import { useTheme } from '../lib/theme'
import { useToast } from '../lib/toast'
import { formatDateTime } from '../lib/format'
import type { TenantReport } from '../lib/types'

const cardCls = 'rounded-xl border border-slate-200 bg-white p-5 shadow-sm dark:border-slate-800 dark:bg-slate-900'

const HEALTH_STYLE: Record<string, string> = {
  sehat: 'bg-emerald-100 text-emerald-800 dark:bg-emerald-500/15 dark:text-emerald-300',
  perhatian: 'bg-amber-100 text-amber-800 dark:bg-amber-500/15 dark:text-amber-300',
  nonaktif: 'bg-slate-200 text-slate-700 dark:bg-slate-700 dark:text-slate-200',
}

function pct(a: number, b: number) {
  return b > 0 ? Math.round((a / b) * 100) : 0
}

// Laporan SaaS untuk SUPERADMIN: ukuran pelanggan, pemakaian, kesehatan tenant, tren 30 hari, ekspor CSV, cetak/PDF.
export function ReportsPage() {
  const { user } = useAuth()
  // Laporan platform hanya untuk SUPERADMIN asli (sesi impersonate berperan ADMIN sehingga terlempar).
  if (!user?.roles.includes('SUPERADMIN')) return <Navigate to="/dashboard" replace />
  return <ReportsContent />
}

function ReportsContent() {
  const { data, isLoading, isError, refetch, isFetching } = useSaaSReport()
  const { resolvedTheme } = useTheme()
  const toast = useToast()
  const isDark = resolvedTheme === 'dark'
  const [filter, setFilter] = useState<'all' | 'perhatian' | 'sehat' | 'nonaktif'>('all')
  const [search, setSearch] = useState('')

  const grid = isDark ? '#1e293b' : '#e2e8f0'
  const tick = isDark ? '#94a3b8' : '#475569'
  const tooltipStyle = {
    borderRadius: 8,
    borderColor: isDark ? '#334155' : '#e2e8f0',
    backgroundColor: isDark ? '#0f172a' : '#ffffff',
    color: isDark ? '#e2e8f0' : '#0f172a',
    fontSize: 13,
  }

  const tenants = useMemo(() => {
    const term = search.trim().toLowerCase()
    return (data?.tenants ?? []).filter(
      (t) => (filter === 'all' || t.health === filter) && (!term || t.name.toLowerCase().includes(term) || t.code.toLowerCase().includes(term)),
    )
  }, [data, filter, search])

  async function downloadCsv() {
    try {
      const res = await fetch(`${API_BASE}/reports/saas/tenants.csv`, { headers: { Authorization: `Bearer ${getAuthToken() ?? ''}` } })
      if (!res.ok) throw new ApiError(res.status, 'Gagal mengunduh laporan')
      const blob = await res.blob()
      const a = document.createElement('a')
      a.href = URL.createObjectURL(blob)
      a.download = `laporan-saas-tenant-${new Date().toISOString().slice(0, 10)}.csv`
      a.click()
      URL.revokeObjectURL(a.href)
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : 'Gagal mengunduh laporan')
    }
  }

  if (isLoading) return <PageSpinner />
  if (isError || !data) {
    return (
      <div className="mx-auto max-w-6xl px-6 py-8">
        <EmptyState icon={ShieldAlert} title="Laporan tidak dapat dimuat" description="Periksa koneksi atau hak akses Anda (hanya SUPERADMIN)." />
      </div>
    )
  }
  const p = data.platform
  const attention = data.tenants.filter((t) => t.health === 'perhatian')
  const failPct = pct(p.tasks_failed_30d, p.tasks_30d)

  return (
    <div className="mx-auto max-w-7xl px-4 py-6 sm:px-6 sm:py-8">
      <div className="mb-6 flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold text-slate-900 dark:text-slate-100">Laporan SaaS</h1>
          <p className="mt-0.5 text-sm text-slate-600 dark:text-slate-400">
            Ukuran pelanggan, pemakaian, dan kesehatan tenant · dibuat {formatDateTime(data.generated_at)}
          </p>
        </div>
        <div className="flex flex-wrap gap-2 print:hidden">
          <button onClick={() => refetch()} disabled={isFetching} className="flex items-center gap-1.5 rounded-lg border border-slate-300 px-3 py-1.5 text-sm font-medium text-slate-700 hover:bg-slate-50 disabled:opacity-50 dark:border-slate-700 dark:text-slate-300 dark:hover:bg-slate-800">
            <RefreshCw className={`h-4 w-4 ${isFetching ? 'animate-spin' : ''}`} /> Muat ulang
          </button>
          <button onClick={downloadCsv} className="flex items-center gap-1.5 rounded-lg border border-slate-300 px-3 py-1.5 text-sm font-medium text-slate-700 hover:bg-slate-50 dark:border-slate-700 dark:text-slate-300 dark:hover:bg-slate-800">
            <Download className="h-4 w-4" /> Unduh CSV
          </button>
          <button onClick={() => window.print()} className="flex items-center gap-1.5 rounded-lg bg-indigo-600 px-3 py-1.5 text-sm font-semibold text-white hover:bg-indigo-500">
            <Printer className="h-4 w-4" /> Cetak / PDF
          </button>
        </div>
      </div>

      <div className="mb-6 grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
        <StatCard label="Tenant aktif" value={`${p.tenants_active} / ${p.tenants_total}`} icon={Building2} tone="blue" hint={`+${p.tenants_new_30d} baru (30 hari)`} />
        <StatCard label="Perangkat" value={p.devices_total} icon={Router} tone="emerald" hint={`${pct(p.devices_online, p.devices_total)}% online · +${p.devices_new_7d} (7 hari) · +${p.devices_new_30d} (30 hari)`} />
        <StatCard label="Pengguna aktif (30 hari)" value={`${p.users_active_30d} / ${p.users_total}`} icon={Users} hint={`${pct(p.users_active_30d, p.users_total)}% pernah login`} />
        <StatCard label="Task (30 hari)" value={p.tasks_30d} icon={ListX} tone={failPct > 20 ? 'red' : 'default'} hint={`${p.tasks_failed_30d} gagal (${failPct}%)`} />
        <StatCard label="Sesi CWMP (24 jam)" value={p.sessions_24h} icon={Activity} tone="blue" hint={`${p.webhooks_active} webhook aktif · ${p.firmware_files} berkas firmware`} />
        <StatCard label="Login gagal (24 jam)" value={p.failed_logins_24h} icon={ShieldAlert} tone={p.failed_logins_24h > 50 ? 'amber' : 'default'} hint="indikasi brute-force bila melonjak" />
      </div>

      {attention.length > 0 && (
        <section aria-label="Tenant perlu perhatian" className="mb-6 rounded-xl border border-amber-300 bg-amber-50 p-4 dark:border-amber-500/40 dark:bg-amber-500/10">
          <h2 className="flex items-center gap-2 text-sm font-semibold text-amber-900 dark:text-amber-200">
            <AlertTriangle className="h-4 w-4" /> {attention.length} tenant perlu perhatian
          </h2>
          <ul className="mt-2 space-y-1 text-sm text-amber-900 dark:text-amber-100">
            {attention.slice(0, 6).map((t) => (
              <li key={t.tenant_id}>
                <strong>{t.name}</strong> — {t.health_reasons.join('; ')}
              </li>
            ))}
          </ul>
        </section>
      )}

      <div className="mb-6 grid grid-cols-1 gap-5 lg:grid-cols-3">
        <section className={`${cardCls} lg:col-span-2`} aria-label="Tren harian">
          <h2 className="mb-3 text-sm font-semibold text-slate-900 dark:text-slate-100">Tren 30 hari</h2>
          <ResponsiveContainer width="100%" height={260}>
            <LineChart data={data.daily} margin={{ left: 0, right: 12, top: 4 }}>
              <CartesianGrid strokeDasharray="3 3" stroke={grid} />
              <XAxis dataKey="date" tick={{ fill: tick, fontSize: 11 }} tickFormatter={(d: string) => d.slice(5)} interval={4} />
              <YAxis tick={{ fill: tick, fontSize: 11 }} allowDecimals={false} width={40} />
              <Tooltip contentStyle={tooltipStyle} />
              <Legend wrapperStyle={{ fontSize: 12, color: tick }} />
              <Line type="monotone" dataKey="sessions" name="Sesi CWMP" stroke="#6366f1" strokeWidth={2} dot={false} />
              <Line type="monotone" dataKey="tasks" name="Task dibuat" stroke="#0ea5e9" strokeWidth={2} dot={false} />
              <Line type="monotone" dataKey="tasks_failed" name="Task gagal" stroke="#ef4444" strokeWidth={2} dot={false} />
              <Line type="monotone" dataKey="new_devices" name="Perangkat baru" stroke="#10b981" strokeWidth={2} dot={false} />
            </LineChart>
          </ResponsiveContainer>
        </section>
        <section className={cardCls} aria-label="Distribusi vendor">
          <h2 className="mb-3 text-sm font-semibold text-slate-900 dark:text-slate-100">Perangkat per vendor</h2>
          {data.vendors.length === 0 ? (
            <p className="text-sm text-slate-600 dark:text-slate-400">Belum ada perangkat.</p>
          ) : (
            <ResponsiveContainer width="100%" height={260}>
              <BarChart data={data.vendors.slice(0, 8)} layout="vertical" margin={{ left: 8, right: 16 }}>
                <CartesianGrid strokeDasharray="3 3" stroke={grid} horizontal={false} />
                <XAxis type="number" tick={{ fill: tick, fontSize: 11 }} allowDecimals={false} />
                <YAxis type="category" dataKey="vendor" tick={{ fill: tick, fontSize: 11 }} width={110} />
                <Tooltip contentStyle={tooltipStyle} />
                <Bar dataKey="devices" name="Perangkat" fill="#6366f1" radius={[0, 4, 4, 0]} />
              </BarChart>
            </ResponsiveContainer>
          )}
        </section>
      </div>

      <section className={cardCls} aria-label="Tabel tenant">
        <div className="mb-3 flex flex-wrap items-center justify-between gap-3">
          <h2 className="text-sm font-semibold text-slate-900 dark:text-slate-100">Tenant ({tenants.length})</h2>
          <div className="flex flex-wrap gap-2 print:hidden">
            <input
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              aria-label="Cari tenant"
              placeholder="Cari nama/kode tenant"
              maxLength={64}
              className="rounded-lg border border-slate-300 px-3 py-1.5 text-sm text-slate-900 outline-none focus:border-slate-500 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-100"
            />
            <select
              value={filter}
              onChange={(e) => setFilter(e.target.value as typeof filter)}
              aria-label="Filter kesehatan"
              className="rounded-lg border border-slate-300 bg-white px-3 py-1.5 text-sm text-slate-700 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-300"
            >
              <option value="all">Semua kesehatan</option>
              <option value="perhatian">Perlu perhatian</option>
              <option value="sehat">Sehat</option>
              <option value="nonaktif">Nonaktif</option>
            </select>
          </div>
        </div>
        <div className="overflow-x-auto" tabIndex={0} role="region" aria-label="Tabel tenant (dapat digulir)">
          <table className="w-full min-w-[900px] text-left text-sm">
            <thead>
              <tr className="border-b border-slate-200 text-xs font-medium uppercase tracking-wide text-slate-600 dark:border-slate-800 dark:text-slate-400">
                <th className="py-2 pr-3">Tenant</th>
                <th className="py-2 pr-3">Kesehatan</th>
                <th className="py-2 pr-3 text-right">Perangkat (online)</th>
                <th className="py-2 pr-3 text-right">Pengguna aktif</th>
                <th className="py-2 pr-3 text-right">Task 30h (gagal)</th>
                <th className="py-2 pr-3 text-right">Antrean</th>
                <th className="py-2 pr-3 text-right">Sesi 30h</th>
                <th className="py-2">Inform terakhir</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
              {tenants.length === 0 && (
                <tr>
                  <td colSpan={8} className="py-6 text-center text-slate-600 dark:text-slate-400">Tidak ada tenant yang cocok.</td>
                </tr>
              )}
              {tenants.map((t) => (
                <TenantRow key={t.tenant_id} t={t} />
              ))}
            </tbody>
          </table>
        </div>
      </section>
    </div>
  )
}

function TenantRow({ t }: { t: TenantReport }) {
  return (
    <tr className="text-slate-800 dark:text-slate-200">
      <td className="py-2.5 pr-3">
        <p className="font-medium">{t.name}</p>
        <p className="font-mono text-xs text-slate-600 dark:text-slate-400">{t.code}</p>
      </td>
      <td className="py-2.5 pr-3">
        <span className={`inline-block rounded-full px-2.5 py-0.5 text-xs font-semibold ${HEALTH_STYLE[t.health] ?? HEALTH_STYLE.sehat}`}>{t.health}</span>
        {t.health_reasons.length > 0 && <p className="mt-1 max-w-xs text-xs text-slate-600 dark:text-slate-400">{t.health_reasons.join('; ')}</p>}
      </td>
      <td className="py-2.5 pr-3 text-right tabular-nums">{t.devices} <span className="text-slate-600 dark:text-slate-400">({t.devices_online})</span></td>
      <td className="py-2.5 pr-3 text-right tabular-nums">{t.users_active_30d} / {t.users}</td>
      <td className="py-2.5 pr-3 text-right tabular-nums">{t.tasks_30d} <span className={t.tasks_failed_30d > 0 ? 'text-red-700 dark:text-red-400' : 'text-slate-600 dark:text-slate-400'}>({t.tasks_failed_30d})</span></td>
      <td className="py-2.5 pr-3 text-right tabular-nums">{t.pending_tasks}{t.max_pending_tasks ? ` / ${t.max_pending_tasks}` : ''}</td>
      <td className="py-2.5 pr-3 text-right tabular-nums">{t.sessions_30d}</td>
      <td className="py-2.5 text-slate-600 dark:text-slate-400">{t.last_inform_at ? formatDateTime(t.last_inform_at) : '—'}</td>
    </tr>
  )
}
