import { useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  Building2,
  Cable,
  FileSliders,
  HardDrive,
  LayoutDashboard,
  LayoutGrid,
  ListChecks,
  Router,
  Search,
} from 'lucide-react'
import { useAuth } from '../lib/auth'
import { useDevices } from '../lib/hooks'

interface QuickLink {
  label: string
  to: string
  icon: typeof LayoutDashboard
  requireRole?: string
}

const QUICK_LINKS: QuickLink[] = [
  { label: 'Dashboard', to: '/dashboard', icon: LayoutDashboard },
  { label: 'Devices', to: '/devices', icon: LayoutGrid },
  { label: 'Tasks', to: '/tasks', icon: ListChecks },
  { label: 'Provisioning', to: '/provisioning', icon: FileSliders },
  { label: 'Firmware', to: '/firmware', icon: HardDrive },
  { label: 'Catalog Vendor', to: '/catalog', icon: Cable, requireRole: 'SUPERADMIN' },
  { label: 'Administration', to: '/administration', icon: Building2, requireRole: 'ADMIN' },
]

// useDebouncedValue menunda update value sampai jeda ketikan berhenti —
// menghindari query /devices?search= di setiap keystroke.
function useDebouncedValue<T>(value: T, delayMs: number): T {
  const [debounced, setDebounced] = useState(value)
  useEffect(() => {
    const timer = setTimeout(() => setDebounced(value), delayMs)
    return () => clearTimeout(timer)
  }, [value, delayMs])
  return debounced
}

export function CommandPalette({ open, onClose }: { open: boolean; onClose: () => void }) {
  const navigate = useNavigate()
  const { hasRole } = useAuth()
  const [query, setQuery] = useState('')
  const [activeIndex, setActiveIndex] = useState(0)
  const inputRef = useRef<HTMLInputElement>(null)
  const debouncedQuery = useDebouncedValue(query, 250)

  const links = useMemo(
    () =>
      QUICK_LINKS.filter((l) => !l.requireRole || hasRole(l.requireRole)).filter((l) =>
        l.label.toLowerCase().includes(query.toLowerCase()),
      ),
    [query, hasRole],
  )

  const { data: devicesResp, isFetching } = useDevices({
    search: debouncedQuery.trim().length >= 2 ? debouncedQuery.trim() : undefined,
    page_size: 8,
  })
  const deviceResults = debouncedQuery.trim().length >= 2 ? devicesResp?.data ?? [] : []

  type ResultItem = { kind: 'link'; link: QuickLink } | { kind: 'device'; device: (typeof deviceResults)[number] }
  const items: ResultItem[] = [
    ...links.map((link): ResultItem => ({ kind: 'link', link })),
    ...deviceResults.map((device): ResultItem => ({ kind: 'device', device })),
  ]

  useEffect(() => {
    setActiveIndex(0)
  }, [query, deviceResults.length]) // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    if (open) {
      setQuery('')
      setActiveIndex(0)
      setTimeout(() => inputRef.current?.focus(), 0)
    }
  }, [open])

  function activate(item: ResultItem) {
    if (item.kind === 'link') navigate(item.link.to)
    else navigate(`/devices/${item.device.id}`)
    onClose()
  }

  function handleKeyDown(e: React.KeyboardEvent) {
    if (e.key === 'Escape') {
      onClose()
    } else if (e.key === 'ArrowDown') {
      e.preventDefault()
      setActiveIndex((i) => Math.min(i + 1, items.length - 1))
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      setActiveIndex((i) => Math.max(i - 1, 0))
    } else if (e.key === 'Enter') {
      e.preventDefault()
      const item = items[activeIndex]
      if (item) activate(item)
    }
  }

  if (!open) return null

  return (
    <div className="fixed inset-0 z-50 flex items-start justify-center bg-slate-900/40 pt-[15vh]" onClick={onClose}>
      <div
        className="w-full max-w-lg rounded-xl bg-white shadow-2xl"
        onClick={(e) => e.stopPropagation()}
        onKeyDown={handleKeyDown}
      >
        <div className="flex items-center gap-2.5 border-b border-slate-200 px-4 py-3">
          <Search className="h-4 w-4 shrink-0 text-slate-400 dark:text-slate-500" />
          <input
            ref={inputRef}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Cari device (serial/MAC) atau buka halaman..."
            className="w-full text-sm text-slate-900 dark:text-slate-100 outline-none placeholder:text-slate-400 dark:placeholder:text-slate-500"
          />
          {isFetching && debouncedQuery.trim().length >= 2 && (
            <span className="shrink-0 text-xs text-slate-400 dark:text-slate-500">Mencari…</span>
          )}
          <kbd className="shrink-0 rounded border border-slate-200 bg-slate-50 px-1.5 py-0.5 text-[10px] text-slate-400 dark:text-slate-500">Esc</kbd>
        </div>

        <div className="max-h-80 overflow-y-auto py-2">
          {items.length === 0 ? (
            <p className="px-4 py-6 text-center text-sm text-slate-400 dark:text-slate-500">
              {query.trim() ? 'Tidak ada hasil' : 'Ketik untuk mencari device atau halaman'}
            </p>
          ) : (
            <>
              {links.length > 0 && (
                <div className="px-2">
                  <p className="px-2 pb-1 pt-1 text-[11px] font-medium uppercase tracking-wide text-slate-400 dark:text-slate-500">Halaman</p>
                  {links.map((link, i) => (
                    <ResultRow key={link.to} active={i === activeIndex} onClick={() => activate({ kind: 'link', link })}>
                      <link.icon className="h-4 w-4 text-slate-400 dark:text-slate-500" />
                      <span>{link.label}</span>
                    </ResultRow>
                  ))}
                </div>
              )}
              {deviceResults.length > 0 && (
                <div className="px-2">
                  <p className="px-2 pb-1 pt-2 text-[11px] font-medium uppercase tracking-wide text-slate-400 dark:text-slate-500">Device</p>
                  {deviceResults.map((device, i) => {
                    const idx = links.length + i
                    return (
                      <ResultRow key={device.id} active={idx === activeIndex} onClick={() => activate({ kind: 'device', device })}>
                        <Router className="h-4 w-4 text-slate-400 dark:text-slate-500" />
                        <span className="flex-1 truncate font-mono text-xs">{device.serial_number}</span>
                        {device.mac_address && <span className="shrink-0 font-mono text-[11px] text-slate-400 dark:text-slate-500">{device.mac_address}</span>}
                      </ResultRow>
                    )
                  })}
                </div>
              )}
            </>
          )}
        </div>
      </div>
    </div>
  )
}

function ResultRow({ active, onClick, children }: { active: boolean; onClick: () => void; children: React.ReactNode }) {
  return (
    <button
      onClick={onClick}
      className={`flex w-full items-center gap-2.5 rounded-lg px-2.5 py-2 text-left text-sm transition-colors ${
        active ? 'bg-slate-900 text-white' : 'text-slate-700 dark:text-slate-300 hover:bg-slate-50'
      }`}
    >
      {children}
    </button>
  )
}
