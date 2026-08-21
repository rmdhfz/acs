import type { LucideIcon } from 'lucide-react'

interface StatCardProps {
  label: string
  value: number | string
  icon: LucideIcon
  tone?: 'default' | 'emerald' | 'amber' | 'red' | 'blue'
  loading?: boolean
}

const TONE_STYLES: Record<NonNullable<StatCardProps['tone']>, string> = {
  default: 'bg-slate-100 text-slate-600',
  emerald: 'bg-emerald-50 text-emerald-600',
  amber: 'bg-amber-50 text-amber-600',
  red: 'bg-red-50 text-red-600',
  blue: 'bg-blue-50 text-blue-600',
}

export function StatCard({ label, value, icon: Icon, tone = 'default', loading }: StatCardProps) {
  return (
    <div className="flex items-center gap-4 rounded-xl border border-slate-200 bg-white p-4 shadow-sm">
      <div className={`flex h-11 w-11 shrink-0 items-center justify-center rounded-lg ${TONE_STYLES[tone]}`}>
        <Icon className="h-5 w-5" strokeWidth={2} />
      </div>
      <div className="min-w-0">
        <p className="text-xs font-medium text-slate-500">{label}</p>
        {loading ? (
          <div className="mt-1 h-6 w-12 animate-pulse rounded bg-slate-200" />
        ) : (
          <p className="text-2xl font-semibold tabular-nums text-slate-900">{value}</p>
        )}
      </div>
    </div>
  )
}
