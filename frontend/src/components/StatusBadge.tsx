interface StatusStyle {
  dot: string
  text: string
  bg: string
}

const STATUS_STYLES: Record<string, StatusStyle> = {
  ONLINE: { dot: 'bg-emerald-500', text: 'text-emerald-700', bg: 'bg-emerald-50' },
  COMPLETED: { dot: 'bg-emerald-500', text: 'text-emerald-700', bg: 'bg-emerald-50' },
  OFFLINE: { dot: 'bg-slate-400', text: 'text-slate-600', bg: 'bg-slate-100' },
  CANCELLED: { dot: 'bg-slate-400', text: 'text-slate-600', bg: 'bg-slate-100' },
  PROVISIONING: { dot: 'bg-blue-500', text: 'text-blue-700', bg: 'bg-blue-50' },
  SENT: { dot: 'bg-blue-500', text: 'text-blue-700', bg: 'bg-blue-50' },
  QUEUED: { dot: 'bg-blue-500', text: 'text-blue-700', bg: 'bg-blue-50' },
  FAULTY: { dot: 'bg-red-500', text: 'text-red-700', bg: 'bg-red-50' },
  FAILED: { dot: 'bg-red-500', text: 'text-red-700', bg: 'bg-red-50' },
  UNREGISTERED: { dot: 'bg-amber-500', text: 'text-amber-700', bg: 'bg-amber-50' },
  PENDING: { dot: 'bg-amber-500', text: 'text-amber-700', bg: 'bg-amber-50' },
  TIMEOUT: { dot: 'bg-orange-500', text: 'text-orange-700', bg: 'bg-orange-50' },
  DECOMMISSIONED: { dot: 'bg-zinc-400', text: 'text-zinc-600', bg: 'bg-zinc-100' },
}

const DEFAULT_STYLE: StatusStyle = { dot: 'bg-slate-400', text: 'text-slate-600', bg: 'bg-slate-100' }

interface StatusBadgeProps {
  code: string | undefined
  label: string
  pulse?: boolean
}

export function StatusBadge({ code, label, pulse }: StatusBadgeProps) {
  const style = (code && STATUS_STYLES[code]) || DEFAULT_STYLE
  return (
    <span
      className={`inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs font-medium ${style.bg} ${style.text}`}
    >
      <span className="relative flex h-1.5 w-1.5">
        {pulse && (
          <span className={`absolute inline-flex h-full w-full animate-ping rounded-full ${style.dot} opacity-75`} />
        )}
        <span className={`relative inline-flex h-1.5 w-1.5 rounded-full ${style.dot}`} />
      </span>
      {label}
    </span>
  )
}
