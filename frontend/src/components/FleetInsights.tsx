import { Link } from 'react-router-dom'
import { Cell, Pie, PieChart, ResponsiveContainer, Tooltip } from 'recharts'
import { Gauge, Network, Thermometer, Timer, WifiOff } from 'lucide-react'
import { PageSpinner } from './Spinner'
import { useDeviceInsights, type BucketCount } from '../lib/hooks'

// Wawasan armada di dashboard: mode PON, uptime, kualitas optik RX, suhu ONU, dan lama offline. Semua angka datang dari
// GET /devices/insights (perangkat ONLINE untuk redaman/suhu/uptime/mode; OFFLINE untuk lama offline).

interface Seg {
  label: string
  color: string
  /** Tautan ke daftar perangkat yang sesuai keranjang ini (bila ada filter yang relevan). */
  to?: string
}

const GREY = '#94a3b8'

const PON: Record<string, Seg> = {
  gpon: { label: 'GPON', color: '#3b82f6' },
  epon: { label: 'EPON', color: '#8b5cf6' },
  unknown: { label: 'Tidak diketahui', color: GREY },
}
const UPTIME: Record<string, Seg> = {
  lt1d: { label: '< 1 hari', color: '#10b981' },
  d1_7: { label: '1 – 7 hari', color: '#3b82f6' },
  d7_30: { label: '7 – 30 hari', color: '#6366f1' },
  gt30d: { label: '> 30 hari', color: '#8b5cf6' },
  unknown: { label: 'Tidak diketahui', color: GREY },
}
const OPTICAL: Record<string, Seg> = {
  ok: { label: 'Baik', color: '#10b981', to: '/devices?optical_level=ok' },
  warn: { label: 'Peringatan', color: '#f59e0b', to: '/devices?optical_level=warn' },
  bad: { label: 'Kritis (lemah)', color: '#ef4444', to: '/devices?optical_level=bad' },
  overload: { label: 'Kelebihan beban', color: '#a855f7', to: '/devices?optical_level=bad' },
  unknown: { label: 'Tidak diketahui', color: GREY, to: '/devices?optical_level=unknown' },
}
const TEMP: Record<string, Seg> = {
  normal: { label: 'Normal', color: '#10b981' },
  warning: { label: 'Peringatan (≥ 60 °C)', color: '#f59e0b' },
  high: { label: 'Tinggi (≥ 70 °C)', color: '#f97316' },
  critical: { label: 'Kritis (≥ 80 °C)', color: '#ef4444' },
  unknown: { label: 'Tidak diketahui', color: GREY },
}
const OFFLINE: Record<string, Seg> = {
  lt1d: { label: '< 24 jam', color: '#fbbf24' },
  d1_3: { label: '1 – 3 hari', color: '#f59e0b' },
  d3_7: { label: '3 – 7 hari', color: '#f97316' },
  d7_30: { label: '7 – 30 hari', color: '#ef4444' },
  gt30d: { label: '> 30 hari', color: '#b91c1c' },
  unknown: { label: 'Tak pernah terhubung', color: GREY },
}

const cardCls = 'rounded-3xl border border-slate-200 bg-white p-6 dark:border-slate-800 dark:bg-slate-900'
const fmt = new Intl.NumberFormat('id-ID')
const pct = (n: number, total: number) => (total > 0 ? Math.round((n / total) * 100) : 0)

function Card({ title, subtitle, icon: Icon, badge, children }: { title: string; subtitle: string; icon: typeof Gauge; badge?: string; children: React.ReactNode }) {
  return (
    <section className={cardCls} aria-label={title}>
      <div className="mb-4 flex items-start justify-between gap-3">
        <div className="min-w-0">
          <h2 className="flex items-center gap-2 text-sm font-semibold uppercase tracking-wide text-slate-700 dark:text-slate-200">
            <Icon className="h-4 w-4 shrink-0 text-slate-500 dark:text-slate-400" aria-hidden="true" /> {title}
          </h2>
          <p className="mt-0.5 text-xs text-slate-500 dark:text-slate-400">{subtitle}</p>
        </div>
        {badge && <span className="shrink-0 rounded-full bg-slate-100 px-2.5 py-1 text-xs font-medium text-slate-700 dark:bg-slate-800 dark:text-slate-300">{badge}</span>}
      </div>
      {children}
    </section>
  )
}

function Row({ seg, count, total }: { seg: Seg; count: number; total: number }) {
  const inner = (
    <>
      <span className="flex min-w-0 items-center gap-2 text-slate-700 dark:text-slate-300">
        <span className="h-2.5 w-2.5 shrink-0 rounded-full" style={{ backgroundColor: seg.color }} aria-hidden="true" />
        <span className="truncate">{seg.label}</span>
      </span>
      <span className="shrink-0 tabular-nums text-slate-900 dark:text-slate-100">
        {fmt.format(count)}
        <span className="ml-2 inline-block w-9 text-right text-xs text-slate-500 dark:text-slate-400">{pct(count, total)}%</span>
      </span>
    </>
  )
  const cls = 'flex items-center justify-between gap-3 rounded-lg px-2 py-1 text-sm'
  return seg.to && count > 0 ? (
    <li>
      <Link to={seg.to} className={`${cls} hover:bg-slate-50 focus-visible:outline-2 focus-visible:outline-indigo-600 dark:hover:bg-slate-800`}>
        {inner}
      </Link>
    </li>
  ) : (
    <li className={cls}>{inner}</li>
  )
}

/** Donat + daftar angka; total di tengah. */
function DonutList({ data, defs, centerLabel, tooltipStyle }: { data: BucketCount[]; defs: Record<string, Seg>; centerLabel: string; tooltipStyle: React.CSSProperties }) {
  const total = data.reduce((n, b) => n + b.count, 0)
  const slices = data.filter((b) => b.count > 0).map((b) => ({ name: defs[b.key]?.label ?? b.key, value: b.count, color: defs[b.key]?.color ?? GREY }))
  return (
    <div className="flex flex-col items-center gap-4 sm:flex-row">
      <div className="relative h-36 w-36 shrink-0">
        {total === 0 ? (
          <div className="h-full w-full rounded-full border-[14px] border-slate-100 dark:border-slate-800" />
        ) : (
          <ResponsiveContainer width="100%" height="100%">
            <PieChart>
              <Pie data={slices} dataKey="value" nameKey="name" innerRadius={44} outerRadius={64} paddingAngle={slices.length > 1 ? 2 : 0} stroke="none">
                {slices.map((s, i) => (
                  <Cell key={i} fill={s.color} />
                ))}
              </Pie>
              <Tooltip contentStyle={tooltipStyle} />
            </PieChart>
          </ResponsiveContainer>
        )}
        <div className="pointer-events-none absolute inset-0 flex flex-col items-center justify-center">
          <span className="text-xl font-semibold tabular-nums text-slate-900 dark:text-slate-100">{fmt.format(total)}</span>
          <span className="text-[10px] font-medium uppercase tracking-wide text-slate-500 dark:text-slate-400">{centerLabel}</span>
        </div>
      </div>
      <ul className="w-full min-w-0 flex-1 space-y-0.5">
        {data.map((b) => (
          <Row key={b.key} seg={defs[b.key] ?? { label: b.key, color: GREY }} count={b.count} total={total} />
        ))}
      </ul>
    </div>
  )
}

/** Batang horizontal bertingkat (untuk distribusi berurutan seperti uptime / lama offline). */
function BarList({ data, defs }: { data: BucketCount[]; defs: Record<string, Seg> }) {
  const total = data.reduce((n, b) => n + b.count, 0)
  const max = Math.max(1, ...data.map((b) => b.count))
  return (
    <ul className="space-y-3">
      {data.map((b) => {
        const seg = defs[b.key] ?? { label: b.key, color: GREY }
        return (
          <li key={b.key}>
            <div className="mb-1 flex items-center justify-between gap-3 text-sm">
              <span className="truncate text-slate-700 dark:text-slate-300">{seg.label}</span>
              <span className="shrink-0 tabular-nums text-slate-900 dark:text-slate-100">
                {fmt.format(b.count)}
                <span className="ml-2 inline-block w-9 text-right text-xs text-slate-500 dark:text-slate-400">{pct(b.count, total)}%</span>
              </span>
            </div>
            <div className="h-2 overflow-hidden rounded-full bg-slate-100 dark:bg-slate-800" role="img" aria-label={`${seg.label}: ${b.count}`}>
              <div className="h-full rounded-full" style={{ width: `${(b.count / max) * 100}%`, backgroundColor: seg.color }} />
            </div>
          </li>
        )
      })}
    </ul>
  )
}

/** Strip Daring / Luring / Lainnya dengan persentase. */
function StatusStrip({ online, offline, other }: { online: number; offline: number; other: number }) {
  const total = online + offline + other
  const items = [
    { label: 'Daring', n: online, color: '#10b981', to: '/devices?status=ONLINE' },
    { label: 'Luring', n: offline, color: '#ef4444', to: '/devices?status=OFFLINE' },
    { label: 'Lainnya', n: other, color: GREY, to: '/devices' },
  ]
  return (
    <div className={`${cardCls} grid grid-cols-1 gap-5 py-5 sm:grid-cols-3`}>
      {items.map((i) => (
        <Link key={i.label} to={i.to} className="group block rounded-xl focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-indigo-600">
          <div className="flex items-baseline justify-between gap-3">
            <span className="text-xs font-semibold uppercase tracking-wide text-slate-600 dark:text-slate-400">{i.label}</span>
            <span className="text-xs font-medium text-slate-500 dark:text-slate-400">{pct(i.n, total)}%</span>
          </div>
          <p className="mt-1 text-3xl font-semibold tabular-nums text-slate-900 dark:text-slate-100">{fmt.format(i.n)}</p>
          <div className="mt-2 h-1.5 overflow-hidden rounded-full bg-slate-100 dark:bg-slate-800">
            <div className="h-full rounded-full transition-[width]" style={{ width: `${pct(i.n, total)}%`, backgroundColor: i.color }} />
          </div>
        </Link>
      ))}
    </div>
  )
}

export function FleetInsights({ tooltipStyle }: { tooltipStyle: React.CSSProperties }) {
  const { data, isLoading, isError } = useDeviceInsights()
  if (isLoading) return <PageSpinner />
  if (isError || !data) return null
  if (data.online + data.offline + data.other === 0) return null

  const unknownOf = (b: BucketCount[]) => b.find((x) => x.key === 'unknown')?.count ?? 0
  const tempKnown = data.online - unknownOf(data.temperature)
  const rxBad = (data.optical_rx.find((x) => x.key === 'bad')?.count ?? 0) + (data.optical_rx.find((x) => x.key === 'overload')?.count ?? 0)
  const rxOk = data.optical_rx.find((x) => x.key === 'ok')?.count ?? 0
  const rxKnown = data.online - unknownOf(data.optical_rx)

  return (
    <div className="mb-6 space-y-4" aria-label="Wawasan armada">
      <StatusStrip online={data.online} offline={data.offline} other={data.other} />
      <div className="grid grid-cols-1 gap-5 lg:grid-cols-3">
        <Card title="Mode PON" subtitle="Terdeteksi dari parameter perangkat" icon={Network} badge={data.pon_mode[0].count + data.pon_mode[1].count > 0 ? `${fmt.format(data.pon_mode[0].count + data.pon_mode[1].count)} terdeteksi` : undefined}>
          <DonutList data={data.pon_mode} defs={PON} centerLabel="Daring" tooltipStyle={tooltipStyle} />
        </Card>
        <Card title="Kualitas optik RX" subtitle={`${fmt.format(data.online)} perangkat daring`} icon={Gauge} badge={rxKnown > 0 ? `Baik ${pct(rxOk, rxKnown)}%` : 'Belum ada data'}>
          <DonutList data={data.optical_rx} defs={OPTICAL} centerLabel="Diperiksa" tooltipStyle={tooltipStyle} />
          {rxBad > 0 && (
            <p className="mt-3 text-xs text-red-700 dark:text-red-400">
              {fmt.format(rxBad)} perangkat di luar rentang aman. <Link to="/devices?optical_level=bad" className="font-medium underline">Lihat daftar</Link>
            </p>
          )}
        </Card>
        <Card title="Suhu ONU" subtitle={`${fmt.format(data.online)} perangkat daring`} icon={Thermometer} badge={tempKnown > 0 ? `${fmt.format(tempKnown)} terbaca` : 'Belum ada data'}>
          <DonutList data={data.temperature} defs={TEMP} centerLabel="Diperiksa" tooltipStyle={tooltipStyle} />
          {tempKnown === 0 && (
            <p className="mt-3 text-xs text-slate-500 dark:text-slate-400">
              Suhu belum terbaca. Petakan sumbernya per model lewat tab <b>Redaman Optik</b> pada detail perangkat.
            </p>
          )}
        </Card>
        <Card title="Uptime perangkat" subtitle="Sejak boot terakhir (perangkat daring)" icon={Timer}>
          <BarList data={data.uptime} defs={UPTIME} />
        </Card>
        <Card title="Lama offline" subtitle="Sejak Inform terakhir" icon={WifiOff} badge={data.offline > 0 ? `${fmt.format(data.offline)} luring` : undefined}>
          <BarList data={data.offline_age} defs={OFFLINE} />
        </Card>
      </div>
    </div>
  )
}
