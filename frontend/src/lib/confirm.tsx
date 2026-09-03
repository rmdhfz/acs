import { createContext, useCallback, useContext, useEffect, useState, type ReactNode } from 'react'
import { AlertTriangle } from 'lucide-react'

// useConfirm() = pengganti window.confirm() yang blocking & tidak theme-aware.
// Promise-based: `if (await confirm({ title, message })) { ... }`.

interface ConfirmOptions {
  title: string
  message: ReactNode
  confirmLabel?: string
  cancelLabel?: string
  /** 'danger' = tombol merah untuk aksi destruktif (hapus, factory reset). */
  tone?: 'default' | 'danger'
}

type ConfirmFn = (opts: ConfirmOptions) => Promise<boolean>

const ConfirmContext = createContext<ConfirmFn | null>(null)

interface PendingState extends ConfirmOptions {
  resolve: (value: boolean) => void
}

export function ConfirmProvider({ children }: { children: ReactNode }) {
  const [pending, setPending] = useState<PendingState | null>(null)

  const confirm = useCallback<ConfirmFn>((opts) => {
    return new Promise<boolean>((resolve) => {
      setPending({ ...opts, resolve })
    })
  }, [])

  const settle = useCallback(
    (value: boolean) => {
      pending?.resolve(value)
      setPending(null)
    },
    [pending],
  )

  useEffect(() => {
    if (!pending) return
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') settle(false)
      if (e.key === 'Enter') settle(true)
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [pending, settle])

  const isDanger = pending?.tone === 'danger'

  return (
    <ConfirmContext.Provider value={confirm}>
      {children}
      {pending && (
        <div
          className="fixed inset-0 z-[70] flex items-center justify-center bg-slate-900/40 px-4 dark:bg-black/60"
          role="dialog"
          aria-modal="true"
          aria-labelledby="confirm-title"
          onMouseDown={(e) => {
            if (e.target === e.currentTarget) settle(false)
          }}
        >
          <div className="w-full max-w-md rounded-xl bg-white shadow-xl dark:bg-slate-900">
            <div className="flex gap-3.5 px-5 py-5">
              <div
                className={`mt-0.5 flex h-9 w-9 shrink-0 items-center justify-center rounded-full ${
                  isDanger ? 'bg-red-100 text-red-600 dark:bg-red-500/15 dark:text-red-400' : 'bg-slate-100 text-slate-500 dark:bg-slate-800 dark:text-slate-300'
                }`}
              >
                <AlertTriangle className="h-4 w-4" strokeWidth={2} />
              </div>
              <div className="min-w-0 flex-1">
                <h2 id="confirm-title" className="text-sm font-semibold text-slate-900 dark:text-slate-100">
                  {pending.title}
                </h2>
                <div className="mt-1 text-sm text-slate-500 dark:text-slate-400">{pending.message}</div>
              </div>
            </div>
            <div className="flex justify-end gap-2.5 border-t border-slate-200 px-5 py-3.5 dark:border-slate-800">
              <button
                onClick={() => settle(false)}
                className="rounded-lg px-3.5 py-2 text-sm font-medium text-slate-600 transition-colors hover:bg-slate-100 dark:text-slate-300 dark:hover:bg-slate-800"
              >
                {pending.cancelLabel ?? 'Batal'}
              </button>
              <button
                autoFocus
                onClick={() => settle(true)}
                className={`rounded-lg px-3.5 py-2 text-sm font-medium text-white transition-colors ${
                  isDanger
                    ? 'bg-red-600 hover:bg-red-700'
                    : 'bg-slate-900 hover:bg-slate-800 dark:bg-slate-100 dark:text-slate-900 dark:hover:bg-white'
                }`}
              >
                {pending.confirmLabel ?? 'Lanjutkan'}
              </button>
            </div>
          </div>
        </div>
      )}
    </ConfirmContext.Provider>
  )
}

// eslint-disable-next-line react-refresh/only-export-components
export function useConfirm(): ConfirmFn {
  const ctx = useContext(ConfirmContext)
  if (!ctx) throw new Error('useConfirm harus dipakai di dalam ConfirmProvider')
  return ctx
}
