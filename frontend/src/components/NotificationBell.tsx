import { useEffect, useRef, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { AlertTriangle, Bell, BellOff, CheckCheck, Info, Settings2, XCircle } from 'lucide-react'
import { useNotifications } from '../lib/notifications'
import type { NotificationSeverity, ServerNotification } from '../lib/hooks'
import { formatRelativeTime } from '../lib/format'

export const SEVERITY_ICON: Record<NotificationSeverity, { icon: typeof Info; cls: string; label: string }> = {
  INFO: { icon: Info, cls: 'bg-indigo-50 text-indigo-700 dark:bg-indigo-500/15 dark:text-indigo-300', label: 'Informasi' },
  WARNING: { icon: AlertTriangle, cls: 'bg-amber-50 text-amber-700 dark:bg-amber-500/15 dark:text-amber-300', label: 'Peringatan' },
  CRITICAL: { icon: XCircle, cls: 'bg-red-50 text-red-700 dark:bg-red-500/15 dark:text-red-300', label: 'Kritis' },
}

export function NotificationRow({ n, onOpen }: { n: ServerNotification; onOpen: (n: ServerNotification) => void }) {
  const sev = SEVERITY_ICON[n.severity] ?? SEVERITY_ICON.INFO
  const Icon = sev.icon
  const unread = !n.read_at
  return (
    <button
      type="button"
      onClick={() => onOpen(n)}
      className={`flex w-full items-start gap-3 px-4 py-3 text-left transition-colors hover:bg-slate-50 focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-indigo-600 dark:hover:bg-slate-800 ${
        unread ? 'bg-indigo-50/40 dark:bg-indigo-500/5' : ''
      }`}
    >
      <span className={`mt-0.5 flex h-9 w-9 shrink-0 items-center justify-center rounded-full ${sev.cls}`} aria-hidden="true">
        <Icon className="h-[18px] w-[18px]" strokeWidth={1.75} />
      </span>
      <span className="min-w-0 flex-1">
        <span className="flex items-start justify-between gap-2">
          <span className={`text-sm text-slate-900 dark:text-slate-100 ${unread ? 'font-medium' : ''}`}>
            <span className="sr-only">{sev.label}: </span>
            {n.title}
          </span>
          {unread && <span className="mt-1.5 h-2 w-2 shrink-0 rounded-full bg-indigo-600" aria-label="Belum dibaca" />}
        </span>
        {n.body && <span className="mt-0.5 line-clamp-2 block text-[13px] leading-snug text-slate-600 dark:text-slate-400">{n.body}</span>}
        <span className="mt-1 block text-xs text-slate-500 dark:text-slate-400">
          {n.type_name} · {formatRelativeTime(n.created_at)}
        </span>
      </span>
    </button>
  )
}

export function NotificationBell() {
  const { items, unreadCount, isLoading, markRead, markAllRead } = useNotifications()
  const navigate = useNavigate()
  const [open, setOpen] = useState(false)
  const [tab, setTab] = useState<'all' | 'unread'>('all')
  const btnRef = useRef<HTMLButtonElement>(null)
  const panelRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!open) return
    const onDown = (e: MouseEvent) => {
      const t = e.target as Node
      if (!panelRef.current?.contains(t) && !btnRef.current?.contains(t)) setOpen(false)
    }
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        setOpen(false)
        btnRef.current?.focus()
      }
    }
    document.addEventListener('mousedown', onDown)
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('mousedown', onDown)
      document.removeEventListener('keydown', onKey)
    }
  }, [open])

  const shown = tab === 'unread' ? items.filter((n) => !n.read_at) : items

  function openItem(n: ServerNotification) {
    if (!n.read_at) markRead([n.id])
    setOpen(false)
    // Tautan dari server hanya path internal; tetap divalidasi sebelum navigasi (defense in depth).
    if (n.link && n.link.startsWith('/') && !n.link.startsWith('//')) navigate(n.link)
  }

  return (
    <div className="relative">
      <button
        ref={btnRef}
        type="button"
        onClick={() => setOpen((v) => !v)}
        aria-haspopup="dialog"
        aria-expanded={open}
        aria-label={unreadCount > 0 ? `Notifikasi, ${unreadCount} belum dibaca` : 'Notifikasi'}
        title="Notifikasi"
        className="relative inline-flex h-10 w-10 items-center justify-center rounded-full text-slate-600 transition-colors hover:bg-slate-100 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-600 dark:text-slate-300 dark:hover:bg-slate-800"
      >
        <Bell className="h-5 w-5" strokeWidth={1.75} />
        {unreadCount > 0 && (
          <span className="absolute right-1 top-1 flex h-4 min-w-4 items-center justify-center rounded-full bg-red-600 px-1 text-[10px] font-medium text-white">
            {unreadCount > 9 ? '9+' : unreadCount}
          </span>
        )}
      </button>

      {open && (
        <div
          ref={panelRef}
          role="dialog"
          aria-label="Notifikasi"
          className="fixed inset-x-3 top-[4.25rem] z-50 overflow-hidden rounded-3xl border border-slate-200 bg-white shadow-xl sm:absolute sm:inset-x-auto sm:right-0 sm:top-12 sm:w-[26rem] dark:border-slate-700 dark:bg-slate-900"
        >
          <div className="flex items-center justify-between px-4 pb-1 pt-3">
            <h2 className="text-base font-medium text-slate-900 dark:text-slate-100">Notifikasi</h2>
            <div className="flex items-center gap-1">
              <button
                type="button"
                onClick={markAllRead}
                disabled={unreadCount === 0}
                className="inline-flex h-9 items-center gap-1.5 rounded-full px-3 text-sm font-medium text-indigo-700 hover:bg-indigo-50 disabled:cursor-not-allowed disabled:opacity-50 dark:text-indigo-300 dark:hover:bg-indigo-500/10"
              >
                <CheckCheck className="h-4 w-4" aria-hidden="true" /> Tandai dibaca
              </button>
              <Link
                to="/notifications?tab=preferences"
                onClick={() => setOpen(false)}
                aria-label="Pengaturan notifikasi"
                title="Pengaturan notifikasi"
                className="inline-flex h-9 w-9 items-center justify-center rounded-full text-slate-600 hover:bg-slate-100 dark:text-slate-300 dark:hover:bg-slate-800"
              >
                <Settings2 className="h-[18px] w-[18px]" aria-hidden="true" />
              </Link>
            </div>
          </div>

          <div role="tablist" aria-label="Filter notifikasi" className="flex gap-1 border-b border-slate-200 px-3 dark:border-slate-700">
            {(
              [
                ['all', 'Semua'],
                ['unread', `Belum dibaca${unreadCount ? ` (${unreadCount})` : ''}`],
              ] as const
            ).map(([k, label]) => (
              <button
                key={k}
                role="tab"
                type="button"
                aria-selected={tab === k}
                onClick={() => setTab(k)}
                className={`-mb-px border-b-2 px-3 py-2.5 text-sm font-medium transition-colors ${
                  tab === k ? 'border-indigo-600 text-indigo-700 dark:border-indigo-300 dark:text-indigo-300' : 'border-transparent text-slate-600 hover:text-slate-900 dark:text-slate-400 dark:hover:text-slate-100'
                }`}
              >
                {label}
              </button>
            ))}
          </div>

          <div className="max-h-[min(26rem,60vh)] overflow-y-auto">
            {isLoading ? (
              <p className="px-4 py-10 text-center text-sm text-slate-600 dark:text-slate-400">Memuat…</p>
            ) : shown.length === 0 ? (
              <div className="flex flex-col items-center px-6 py-12 text-center">
                <span className="flex h-12 w-12 items-center justify-center rounded-full bg-slate-100 text-slate-500 dark:bg-slate-800 dark:text-slate-400" aria-hidden="true">
                  <BellOff className="h-6 w-6" strokeWidth={1.5} />
                </span>
                <p className="mt-3 text-sm font-medium text-slate-900 dark:text-slate-100">{tab === 'unread' ? 'Semua sudah dibaca' : 'Belum ada notifikasi'}</p>
                <p className="mt-1 text-[13px] text-slate-600 dark:text-slate-400">Peringatan perangkat, task, dan keamanan akun akan muncul di sini.</p>
              </div>
            ) : (
              <ul className="divide-y divide-slate-100 dark:divide-slate-800">
                {shown.map((n) => (
                  <li key={n.id}>
                    <NotificationRow n={n} onOpen={openItem} />
                  </li>
                ))}
              </ul>
            )}
          </div>

          <div className="border-t border-slate-200 p-2 dark:border-slate-700">
            <Link
              to="/notifications"
              onClick={() => setOpen(false)}
              className="flex h-10 items-center justify-center rounded-full text-sm font-medium text-indigo-700 hover:bg-indigo-50 dark:text-indigo-300 dark:hover:bg-indigo-500/10"
            >
              Lihat semua notifikasi
            </Link>
          </div>
        </div>
      )}
    </div>
  )
}
