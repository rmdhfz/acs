import { useCallback, useEffect, useId, useLayoutEffect, useRef, useState, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { MoreHorizontal, type LucideIcon } from 'lucide-react'

// Menu aksi baris ("⋯") untuk tabel dengan banyak aksi. Menggantikan deretan 5–7 tautan yang bertumpuk
// di kolom terakhir (sulit dipindai, patah baris di layar sempit). Dirender lewat portal + posisi fixed supaya
// tidak terpotong oleh container tabel yang `overflow-x-auto`.
//
// Aksesibilitas: tombol pemicu aria-haspopup/aria-expanded, daftar role="menu" dengan item role="menuitem",
// navigasi panah/Home/End, Esc menutup dan mengembalikan fokus ke pemicu, klik di luar atau scroll menutup.

export interface RowMenuItem {
  label: string
  icon?: LucideIcon
  onClick: () => void
  tone?: 'default' | 'danger' | 'warn' | 'brand'
  disabled?: boolean
  /** true = item tidak ditampilkan (praktis untuk kondisi peran/status). */
  hidden?: boolean
  title?: string
}

const TONE: Record<NonNullable<RowMenuItem['tone']>, string> = {
  default: 'text-slate-700 hover:bg-slate-100 dark:text-slate-200 dark:hover:bg-slate-800',
  brand: 'text-indigo-700 hover:bg-indigo-50 dark:text-indigo-300 dark:hover:bg-indigo-950/40',
  warn: 'text-amber-700 hover:bg-amber-50 dark:text-amber-300 dark:hover:bg-amber-950/40',
  danger: 'text-red-600 hover:bg-red-50 dark:text-red-400 dark:hover:bg-red-950/40',
}

const MENU_WIDTH = 224

export function RowMenu({ items, label = 'Aksi lainnya', trigger }: { items: RowMenuItem[]; label?: string; trigger?: ReactNode }) {
  const visible = items.filter((i) => !i.hidden)
  const [open, setOpen] = useState(false)
  const [pos, setPos] = useState<{ top: number; left: number; up: boolean } | null>(null)
  const btnRef = useRef<HTMLButtonElement>(null)
  const menuRef = useRef<HTMLDivElement>(null)
  const menuId = useId()

  const close = useCallback((restoreFocus: boolean) => {
    setOpen(false)
    if (restoreFocus) btnRef.current?.focus()
  }, [])

  // Hitung posisi saat dibuka: rata kanan pemicu, buka ke atas bila ruang bawah tak cukup.
  useLayoutEffect(() => {
    if (!open || !btnRef.current) return
    const r = btnRef.current.getBoundingClientRect()
    const estHeight = Math.min(visible.length * 36 + 12, 360)
    const up = window.innerHeight - r.bottom < estHeight + 8 && r.top > estHeight
    const left = Math.max(8, Math.min(r.right - MENU_WIDTH, window.innerWidth - MENU_WIDTH - 8))
    setPos({ top: up ? r.top - 4 : r.bottom + 4, left, up })
  }, [open, visible.length])

  // Menu baru terpasang setelah posisi dihitung (pos != null): fokuskan item pertama saat itu.
  const mounted = open && pos !== null
  useEffect(() => {
    if (mounted) menuRef.current?.querySelector<HTMLButtonElement>('button:not([disabled])')?.focus()
  }, [mounted])

  useEffect(() => {
    if (!open) return
    const onDown = (e: MouseEvent) => {
      const t = e.target as Node
      if (menuRef.current?.contains(t) || btnRef.current?.contains(t)) return
      close(false)
    }
    const onAway = () => close(false)
    document.addEventListener('mousedown', onDown)
    window.addEventListener('resize', onAway)
    window.addEventListener('scroll', onAway, true)
    return () => {
      document.removeEventListener('mousedown', onDown)
      window.removeEventListener('resize', onAway)
      window.removeEventListener('scroll', onAway, true)
    }
  }, [open, close])

  if (visible.length === 0) return null

  function onMenuKey(e: React.KeyboardEvent) {
    const btns = Array.from(menuRef.current?.querySelectorAll<HTMLButtonElement>('button:not([disabled])') ?? [])
    const i = btns.indexOf(document.activeElement as HTMLButtonElement)
    if (e.key === 'Escape') {
      e.preventDefault()
      close(true)
    } else if (e.key === 'ArrowDown') {
      e.preventDefault()
      btns[(i + 1) % btns.length]?.focus()
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      btns[(i - 1 + btns.length) % btns.length]?.focus()
    } else if (e.key === 'Home') {
      e.preventDefault()
      btns[0]?.focus()
    } else if (e.key === 'End') {
      e.preventDefault()
      btns[btns.length - 1]?.focus()
    } else if (e.key === 'Tab') {
      close(false)
    }
  }

  return (
    <>
      <button
        ref={btnRef}
        type="button"
        aria-haspopup="menu"
        aria-expanded={open}
        aria-controls={open ? menuId : undefined}
        aria-label={label}
        title={label}
        onClick={() => setOpen((v) => !v)}
        className="inline-flex h-8 w-8 items-center justify-center rounded-lg text-slate-500 transition-colors hover:bg-slate-100 hover:text-slate-900 focus-visible:outline-2 focus-visible:outline-indigo-500 aria-expanded:bg-slate-100 dark:text-slate-400 dark:hover:bg-slate-800 dark:hover:text-slate-100 dark:aria-expanded:bg-slate-800"
      >
        {trigger ?? <MoreHorizontal className="h-4 w-4" aria-hidden="true" />}
      </button>
      {open &&
        pos &&
        createPortal(
          <div
            ref={menuRef}
            id={menuId}
            role="menu"
            aria-label={label}
            onKeyDown={onMenuKey}
            style={{ position: 'fixed', top: pos.top, left: pos.left, width: MENU_WIDTH, transform: pos.up ? 'translateY(-100%)' : undefined }}
            className="z-[60] max-h-[360px] overflow-y-auto rounded-xl border border-slate-200 bg-white p-1.5 shadow-xl dark:border-slate-700 dark:bg-slate-900"
          >
            {visible.map((it, idx) => {
              const Icon = it.icon
              const sep = it.tone === 'danger' && idx > 0
              return (
                <div key={it.label} role="none">
                  {sep && <div role="separator" className="my-1 h-px bg-slate-200 dark:bg-slate-700" />}
                  <button
                    type="button"
                    role="menuitem"
                    disabled={it.disabled}
                    title={it.title}
                    onClick={() => {
                      close(false)
                      it.onClick()
                    }}
                    className={`flex w-full items-center gap-2.5 rounded-lg px-2.5 py-2 text-left text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-indigo-500 disabled:cursor-not-allowed disabled:opacity-50 ${TONE[it.tone ?? 'default']}`}
                  >
                    {Icon && <Icon className="h-4 w-4 shrink-0" aria-hidden="true" />}
                    <span className="truncate">{it.label}</span>
                  </button>
                </div>
              )
            })}
          </div>,
          document.body,
        )}
    </>
  )
}
