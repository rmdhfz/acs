import { useEffect, useMemo, useState, type CSSProperties } from 'react'
import { NavLink, useLocation } from 'react-router-dom'
import { ChevronDown, Settings2 } from 'lucide-react'
import { MenuManagerModal } from './MenuManagerModal'
import { useAuth } from '../lib/auth'
import { useMenuLayout } from '../lib/hooks'
import { useI18n } from '../lib/i18n'
import { MENU_CATALOG, resolveLayout, type MenuItemDef } from '../lib/menu'

const COLLAPSE_KEY = 'acs_nav_collapsed'

// Status buka/tutup grup disimpan per perangkat (localStorage) — preferensi tampilan, bukan data akun.
function readCollapsed(): Record<string, boolean> {
  try {
    const v = JSON.parse(localStorage.getItem(COLLAPSE_KEY) ?? '{}') as unknown
    return v && typeof v === 'object' ? (v as Record<string, boolean>) : {}
  } catch {
    return {}
  }
}

interface Props {
  accentStyle?: CSSProperties
  accentTextCls: string
}

// Sidebar: menu (urutan/grup/sub menu sesuai susunan pengguna), tombol "Atur menu", dan footer versi + hak cipta.
export function SidebarNav({ accentStyle, accentTextCls }: Props) {
  const { hasRole, user } = useAuth()
  const { t } = useI18n()
  const location = useLocation()
  const layoutQ = useMenuLayout(!!user)
  const [managerOpen, setManagerOpen] = useState(false)
  const [collapsed, setCollapsed] = useState<Record<string, boolean>>(readCollapsed)

  const allowed: MenuItemDef[] = useMemo(() => MENU_CATALOG.filter((m) => !m.requireRole || hasRole(m.requireRole)), [hasRole])
  const nodes = useMemo(() => resolveLayout(layoutQ.data, allowed), [layoutQ.data, allowed])

  // Grup yang memuat halaman aktif selalu terbuka.
  useEffect(() => {
    const active = nodes.find((n) => n.type === 'group' && n.items.some((i) => location.pathname === i.to || location.pathname.startsWith(i.to + '/')))
    if (active && active.type === 'group') {
      setCollapsed((c) => (c[active.id] ? { ...c, [active.id]: false } : c))
    }
  }, [location.pathname, nodes])

  function toggle(id: string, defaultCollapsed: boolean) {
    setCollapsed((c) => {
      const cur = id in c ? c[id] : defaultCollapsed
      const next = { ...c, [id]: !cur }
      try {
        localStorage.setItem(COLLAPSE_KEY, JSON.stringify(next))
      } catch {
        // abaikan: hanya preferensi tampilan
      }
      return next
    })
  }

  const linkCls = (isActive: boolean, nested: boolean) =>
    `flex h-10 items-center gap-3.5 rounded-r-full pr-4 text-sm font-medium transition-colors focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-indigo-600 ${
      nested ? 'pl-11' : 'pl-6'
    } ${
      isActive
        ? accentStyle
          ? accentTextCls
          : 'bg-indigo-50 text-indigo-700 dark:bg-indigo-500/15 dark:text-indigo-300'
        : 'text-slate-700 hover:bg-slate-100 dark:text-slate-300 dark:hover:bg-slate-800'
    }`

  const link = (item: MenuItemDef, nested: boolean) => (
    <NavLink key={item.key} to={item.to} className={({ isActive }) => linkCls(isActive, nested)} style={({ isActive }: { isActive: boolean }) => (isActive ? accentStyle : undefined)}>
      <item.icon className="h-5 w-5 shrink-0" strokeWidth={1.75} aria-hidden="true" />
      <span className="truncate">{t(item.labelKey)}</span>
    </NavLink>
  )

  return (
    <>
      <nav aria-label="Navigasi utama" className="flex-1 space-y-0.5 overflow-y-auto py-3 pr-3">
        {nodes.map((n) => {
          if (n.type === 'item') return link(n.def, false)
          const isCollapsed = n.id in collapsed ? collapsed[n.id] : n.collapsed
          const panelId = `nav-group-${n.id}`
          return (
            <div key={n.id} className="pt-1">
              <button
                type="button"
                onClick={() => toggle(n.id, n.collapsed)}
                aria-expanded={!isCollapsed}
                aria-controls={panelId}
                className="flex h-9 w-full items-center justify-between rounded-r-full pl-6 pr-4 text-xs font-semibold uppercase tracking-wide text-slate-600 transition-colors hover:bg-slate-100 focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-indigo-600 dark:text-slate-400 dark:hover:bg-slate-800"
              >
                <span className="truncate">{n.label}</span>
                <ChevronDown className={`h-4 w-4 shrink-0 transition-transform ${isCollapsed ? '-rotate-90' : ''}`} aria-hidden="true" />
              </button>
              <div id={panelId} hidden={isCollapsed} className="space-y-0.5">
                {n.items.map((i) => link(i, true))}
              </div>
            </div>
          )
        })}
      </nav>

      <div className="border-t border-slate-200 px-4 py-3 dark:border-slate-800">
        <button
          type="button"
          onClick={() => setManagerOpen(true)}
          className="mb-2 flex h-9 w-full items-center gap-2.5 rounded-full px-3 text-sm font-medium text-slate-700 transition-colors hover:bg-slate-100 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-600 dark:text-slate-300 dark:hover:bg-slate-800"
        >
          <Settings2 className="h-4.5 w-4.5 shrink-0" strokeWidth={1.75} aria-hidden="true" />
          Atur menu
        </button>
        <p className="px-3 text-[11px] leading-snug text-slate-500 dark:text-slate-400">
          ACS Console v{__APP_VERSION__}
          <br />© {new Date().getFullYear()} Hafiz Ramadhan
        </p>
      </div>

      {managerOpen && <MenuManagerModal saved={layoutQ.data} allowed={allowed} onClose={() => setManagerOpen(false)} />}
    </>
  )
}
