import { useEffect, useState } from 'react'
import { Clock } from 'lucide-react'
import { useI18n } from '../lib/i18n'

// Jam realtime di top bar. Update tiap detik. Zona waktu & format mengikuti
// locale browser + pilihan bahasa aplikasi.
export function NavClock({ compact = false }: { compact?: boolean }) {
  const { lang } = useI18n()
  const [now, setNow] = useState(() => new Date())

  useEffect(() => {
    const id = setInterval(() => setNow(new Date()), 1000)
    return () => clearInterval(id)
  }, [])

  const locale = lang === 'id' ? 'id-ID' : 'en-GB'
  const time = now.toLocaleTimeString(locale, { hour: '2-digit', minute: '2-digit', second: '2-digit' })
  const date = now.toLocaleDateString(locale, { weekday: 'short', day: 'numeric', month: 'short' })

  if (compact) {
    return (
      <span className="flex items-center gap-1.5 text-xs font-medium tabular-nums text-slate-500 dark:text-slate-400">
        <Clock className="h-3.5 w-3.5" />
        {time}
      </span>
    )
  }

  return (
    <span className="flex items-center gap-2 text-sm text-slate-500 dark:text-slate-400" title={now.toString()}>
      <Clock className="h-4 w-4 text-slate-400" />
      <span className="hidden lg:inline">{date}</span>
      <span className="font-medium tabular-nums text-slate-700 dark:text-slate-200">{time}</span>
    </span>
  )
}
