import { Link } from 'react-router-dom'
import type { LucideIcon } from 'lucide-react'

interface StatCardProps {
  label: string
  value: number | string
  icon: LucideIcon
  tone?: 'default' | 'emerald' | 'amber' | 'red' | 'blue'
  loading?: boolean
  /** Teks kecil di bawah angka (mis. "92% dari total"). */
  hint?: string
  /** Bila diisi, kartu menjadi tautan (drill-down). */
  to?: string
  /** Kartu utama: angka lebih besar (3 KPI teratas dashboard). */
  large?: boolean
}

// Gaya kartu KPI bergaya Google: garis aksen pendek berwarna di atas, label kecil, angka besar, ikon redup di kanan.
const ACCENT: Record<NonNullable<StatCardProps['tone']>, string> = {
  default: 'bg-slate-500',
  emerald: 'bg-emerald-600',
  amber: 'bg-amber-500',
  red: 'bg-red-600',
  blue: 'bg-indigo-600',
}
const ICON_TONE: Record<NonNullable<StatCardProps['tone']>, string> = {
  default: 'text-slate-500 dark:text-slate-400',
  emerald: 'text-emerald-600 dark:text-emerald-400',
  amber: 'text-amber-600 dark:text-amber-400',
  red: 'text-red-600 dark:text-red-400',
  blue: 'text-indigo-600 dark:text-indigo-400',
}

export function StatCard({ label, value, icon: Icon, tone = 'default', loading, hint, to, large }: StatCardProps) {
  const cls =
    'group relative block overflow-hidden rounded-3xl border border-slate-200 bg-white p-5 shadow-sm dark:border-slate-800 dark:bg-slate-900' +
    (to ? ' transition hover:border-slate-300 hover:shadow-md focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-600 dark:hover:border-slate-700' : '')
  const body = (
    <>
      <span className={`block h-1.5 w-9 rounded-full ${ACCENT[tone]}`} aria-hidden="true" />
      <Icon className={`absolute right-5 top-5 h-5 w-5 opacity-80 ${ICON_TONE[tone]}`} strokeWidth={1.75} aria-hidden="true" />
      <p className="mt-3 truncate pr-8 text-sm text-slate-600 dark:text-slate-400">{label}</p>
      {loading ? (
        <div className="mt-2 h-8 w-16 animate-pulse rounded-lg bg-slate-200 dark:bg-slate-700" />
      ) : (
        <p className={`mt-1 font-normal tabular-nums text-slate-900 dark:text-slate-100 ${large ? 'text-5xl leading-tight' : 'text-3xl'}`}>{value}</p>
      )}
      {hint && !loading && <p className="mt-1 truncate text-xs text-slate-600 dark:text-slate-400">{hint}</p>}
    </>
  )
  return to ? (
    <Link to={to} className={cls}>
      {body}
    </Link>
  ) : (
    <div className={cls}>{body}</div>
  )
}
