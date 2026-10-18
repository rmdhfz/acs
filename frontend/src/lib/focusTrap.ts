import { useEffect, type RefObject } from 'react'

const FOCUSABLE =
  'a[href], button:not([disabled]), input:not([disabled]):not([type="hidden"]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'

// Elemen yang bisa difokuskan di dalam root (yang tersembunyi dengan hidden/display:none/visibility:hidden dilewati).
export function focusableIn(root: HTMLElement): HTMLElement[] {
  return Array.from(root.querySelectorAll<HTMLElement>(FOCUSABLE)).filter((el) => {
    if (el.closest('[hidden]')) return false
    const st = getComputedStyle(el)
    return st.display !== 'none' && st.visibility !== 'hidden'
  })
}

// Jebakan fokus untuk dialog: saat dibuka fokus dipindah ke dalam panel (kecuali sudah di dalam, mis. autoFocus),
// Tab/Shift+Tab berputar di dalam panel, dan saat ditutup fokus dikembalikan ke elemen pemicu.
// preferred: pemilih elemen fokus awal (default: kontrol pertama di dalam panel, atau panel itu sendiri).
export function useFocusTrap(panelRef: RefObject<HTMLElement | null>, preferred?: (panel: HTMLElement) => HTMLElement | undefined) {
  useEffect(() => {
    const panel = panelRef.current
    const opener = document.activeElement as HTMLElement | null
    if (panel && !panel.contains(document.activeElement)) {
      const target = preferred?.(panel) ?? focusableIn(panel)[0] ?? panel
      target.focus()
    }

    function onKey(e: KeyboardEvent) {
      if (e.key !== 'Tab' || !panel) return
      const items = focusableIn(panel)
      if (items.length === 0) {
        e.preventDefault()
        panel.focus()
        return
      }
      const first = items[0]
      const last = items[items.length - 1]
      const active = document.activeElement
      if (e.shiftKey && (active === first || !panel.contains(active))) {
        e.preventDefault()
        last.focus()
      } else if (!e.shiftKey && (active === last || !panel.contains(active))) {
        e.preventDefault()
        first.focus()
      }
    }
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('keydown', onKey)
      if (opener && typeof opener.focus === 'function' && document.contains(opener)) opener.focus()
    }
    // sengaja hanya saat mount/unmount dialog
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])
}
