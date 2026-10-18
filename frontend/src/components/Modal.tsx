import { useEffect, useId, useRef, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { X } from 'lucide-react'
import { focusableIn, useFocusTrap } from '../lib/focusTrap'

interface ModalProps {
  title: string
  onClose: () => void
  children: ReactNode
  wide?: boolean
  /** Lebih lebar lagi (mis. wizard impor dengan tabel pratinjau). */
  xl?: boolean
  /** Nonaktifkan tutup via Esc / klik backdrop (mis. saat ada operasi berjalan). */
  dismissable?: boolean
  /** Sembunyikan sementara (visibility:hidden) tanpa melepas dialog — mis. saat mengambil tangkapan layar. */
  hidden?: boolean
}

// Dialog aksesibel: nama dari judul (aria-labelledby), fokus pindah ke kontrol pertama di badan dialog (bukan tombol
// tutup), Tab/Shift+Tab berputar di dalamnya, Esc menutup, dan fokus kembali ke elemen pemicu saat ditutup.
export function Modal({ title, onClose, children, wide, xl, dismissable = true, hidden = false }: ModalProps) {
  const titleId = useId()
  const panelRef = useRef<HTMLDivElement>(null)

  useFocusTrap(panelRef, (panel) => {
    const body = panel.querySelector<HTMLElement>('[data-modal-body]')
    return body ? focusableIn(body)[0] : undefined
  })

  useEffect(() => {
    if (!dismissable) return
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') {
        e.stopPropagation()
        onClose()
      }
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [dismissable, onClose])

  // Cegah scroll body selama modal terbuka.
  useEffect(() => {
    const prev = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    return () => {
      document.body.style.overflow = prev
    }
  }, [])

  // Portal ke <body>: dialog yang dibuka dari dalam elemen bertransform (mis. sidebar) tidak ikut terkurung ukurannya.
  return createPortal(
    <div
      className={`fixed inset-0 z-50 flex items-center justify-center bg-slate-900/40 px-4 dark:bg-black/60 ${hidden ? 'invisible' : ''}`}
      onMouseDown={(e) => {
        // klik pada backdrop (bukan di dalam panel) menutup modal
        if (dismissable && e.target === e.currentTarget) onClose()
      }}
      role="dialog"
      aria-modal="true"
      aria-labelledby={titleId}
    >
      <div
        ref={panelRef}
        tabIndex={-1}
        className={`w-full ${xl ? 'max-w-5xl' : wide ? 'max-w-2xl' : 'max-w-md'} rounded-xl bg-white shadow-xl outline-none dark:bg-slate-900`}
      >
        <div className="flex items-center justify-between border-b border-slate-200 px-5 py-4 dark:border-slate-800">
          <h2 id={titleId} className="text-sm font-semibold text-slate-900 dark:text-slate-100">{title}</h2>
          <button
            type="button"
            onClick={onClose}
            aria-label="Tutup"
            className="flex h-7 w-7 items-center justify-center rounded-full text-slate-500 transition-colors hover:bg-slate-100 hover:text-slate-700 dark:hover:bg-slate-800 dark:hover:text-slate-200 dark:text-slate-400"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
        <div data-modal-body className="max-h-[75vh] overflow-y-auto px-5 py-4">{children}</div>
      </div>
    </div>,
    document.body,
  )
}
