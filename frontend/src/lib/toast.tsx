import { createContext, useCallback, useContext, useEffect, useRef, useState, type ReactNode } from 'react'
import { CheckCircle2, AlertTriangle, XCircle, Info, X } from 'lucide-react'

// Toast = umpan balik singkat untuk AKSI pengguna (task diantre, item dihapus,
// dsb). Beda dari NotificationBell (lib/notifications.tsx) yang memantau kondisi
// sistem lewat polling. Gantikan pemakaian window.alert() di seluruh app —
// non-blocking, theme-aware, auto-dismiss.

type ToastTone = 'success' | 'error' | 'warning' | 'info'

interface Toast {
  id: string
  tone: ToastTone
  message: string
  /** Judul opsional; bila kosong, hanya message yang tampil. */
  title?: string
}

interface ToastApi {
  success: (message: string, title?: string) => void
  error: (message: string, title?: string) => void
  warning: (message: string, title?: string) => void
  info: (message: string, title?: string) => void
  dismiss: (id: string) => void
}

const ToastContext = createContext<ToastApi | null>(null)

const DEFAULT_TTL = 5000

const toneConfig: Record<ToastTone, { icon: typeof CheckCircle2; classes: string; iconClass: string }> = {
  success: {
    icon: CheckCircle2,
    classes: 'border-emerald-200 bg-white dark:border-emerald-500/30 dark:bg-slate-900',
    iconClass: 'text-emerald-500',
  },
  error: {
    icon: XCircle,
    classes: 'border-red-200 bg-white dark:border-red-500/30 dark:bg-slate-900',
    iconClass: 'text-red-500',
  },
  warning: {
    icon: AlertTriangle,
    classes: 'border-amber-200 bg-white dark:border-amber-500/30 dark:bg-slate-900',
    iconClass: 'text-amber-500',
  },
  info: {
    icon: Info,
    classes: 'border-slate-200 bg-white dark:border-slate-700 dark:bg-slate-900',
    iconClass: 'text-slate-400',
  },
}

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([])
  const timers = useRef<Map<string, ReturnType<typeof setTimeout>>>(new Map())

  const dismiss = useCallback((id: string) => {
    setToasts((prev) => prev.filter((t) => t.id !== id))
    const timer = timers.current.get(id)
    if (timer) {
      clearTimeout(timer)
      timers.current.delete(id)
    }
  }, [])

  const push = useCallback(
    (tone: ToastTone, message: string, title?: string) => {
      const id = `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`
      setToasts((prev) => [...prev.slice(-3), { id, tone, message, title }])
      const timer = setTimeout(() => dismiss(id), DEFAULT_TTL)
      timers.current.set(id, timer)
    },
    [dismiss],
  )

  useEffect(() => {
    const map = timers.current
    return () => {
      map.forEach((t) => clearTimeout(t))
      map.clear()
    }
  }, [])

  const api: ToastApi = {
    success: (m, t) => push('success', m, t),
    error: (m, t) => push('error', m, t),
    warning: (m, t) => push('warning', m, t),
    info: (m, t) => push('info', m, t),
    dismiss,
  }

  return (
    <ToastContext.Provider value={api}>
      {children}
      <div
        className="pointer-events-none fixed bottom-0 right-0 z-[60] flex w-full max-w-sm flex-col gap-2 p-4"
        aria-live="polite"
        aria-atomic="false"
      >
        {toasts.map((toast) => {
          const cfg = toneConfig[toast.tone]
          const Icon = cfg.icon
          return (
            <div
              key={toast.id}
              role="status"
              className={`pointer-events-auto flex items-start gap-3 rounded-lg border px-4 py-3 shadow-lg motion-safe:animate-[toastIn_.18s_ease-out] ${cfg.classes}`}
            >
              <Icon className={`mt-0.5 h-5 w-5 shrink-0 ${cfg.iconClass}`} strokeWidth={2} />
              <div className="min-w-0 flex-1 text-sm">
                {toast.title && <p className="font-medium text-slate-900 dark:text-slate-100">{toast.title}</p>}
                <p className={toast.title ? 'mt-0.5 text-slate-500 dark:text-slate-400' : 'text-slate-700 dark:text-slate-200'}>
                  {toast.message}
                </p>
              </div>
              <button
                onClick={() => dismiss(toast.id)}
                aria-label="Tutup notifikasi"
                className="-m-1 shrink-0 rounded p-1 text-slate-400 transition-colors hover:bg-slate-100 hover:text-slate-600 dark:hover:bg-slate-800 dark:hover:text-slate-300"
              >
                <X className="h-3.5 w-3.5" />
              </button>
            </div>
          )
        })}
      </div>
    </ToastContext.Provider>
  )
}

// eslint-disable-next-line react-refresh/only-export-components
export function useToast(): ToastApi {
  const ctx = useContext(ToastContext)
  if (!ctx) throw new Error('useToast harus dipakai di dalam ToastProvider')
  return ctx
}
