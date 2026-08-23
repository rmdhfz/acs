import { useEffect, useState } from 'react'
import { NavLink, Outlet, useLocation } from 'react-router-dom'
import {
  Building2,
  Cable,
  FileSliders,
  HardDrive,
  LayoutDashboard,
  LayoutGrid,
  ListChecks,
  LogOut,
  Menu,
  Monitor,
  Moon,
  Radio,
  Search,
  Sun,
  X,
} from 'lucide-react'
import { useAuth } from '../lib/auth'
import { useTheme, type Theme } from '../lib/theme'
import { useCurrentTenant } from '../lib/hooks'
import { CommandPalette } from './CommandPalette'
import { NotificationBell } from './NotificationBell'
import { NotificationProvider } from '../lib/notifications'

const NAV_ITEMS: { to: string; label: string; icon: typeof LayoutGrid; requireRole?: string }[] = [
  { to: '/dashboard', label: 'Dashboard', icon: LayoutDashboard },
  { to: '/devices', label: 'Devices', icon: LayoutGrid },
  { to: '/tasks', label: 'Tasks', icon: ListChecks },
  { to: '/provisioning', label: 'Provisioning', icon: FileSliders },
  { to: '/firmware', label: 'Firmware', icon: HardDrive },
  { to: '/catalog', label: 'Catalog Vendor', icon: Cable, requireRole: 'SUPERADMIN' },
  { to: '/administration', label: 'Administration', icon: Building2, requireRole: 'ADMIN' },
]

const isMac = typeof navigator !== 'undefined' && /Mac|iPhone|iPad/.test(navigator.platform ?? navigator.userAgent)

// accentTextClass — warna aksen tenant (primary_color) bebas diisi admin
// tenant, termasuk warna terang (mis. #ffffff). Tanpa cek kontras, teks nav
// aktif yang di-hardcode putih akan tidak terbaca di atas latar terang.
function accentTextClass(hex?: string | null): string {
  if (!hex || !/^#[0-9a-fA-F]{6}$/.test(hex)) return 'text-white'
  const r = parseInt(hex.slice(1, 3), 16)
  const g = parseInt(hex.slice(3, 5), 16)
  const b = parseInt(hex.slice(5, 7), 16)
  const yiq = (r * 299 + g * 587 + b * 114) / 1000
  return yiq >= 150 ? 'text-slate-900' : 'text-white'
}

const THEME_CYCLE: Theme[] = ['system', 'light', 'dark']
const THEME_ICON: Record<Theme, typeof Sun> = { system: Monitor, light: Sun, dark: Moon }
const THEME_LABEL: Record<Theme, string> = { system: 'Ikuti sistem', light: 'Terang', dark: 'Gelap' }

function ThemeToggle() {
  const { theme, setTheme } = useTheme()
  const Icon = THEME_ICON[theme]
  return (
    <button
      onClick={() => setTheme(THEME_CYCLE[(THEME_CYCLE.indexOf(theme) + 1) % THEME_CYCLE.length])}
      title={`Tema: ${THEME_LABEL[theme]} (klik utk ganti)`}
      className="flex h-8 w-8 items-center justify-center rounded-lg text-slate-500 transition-colors hover:bg-slate-100 hover:text-slate-900 dark:text-slate-400 dark:hover:bg-slate-800 dark:hover:text-slate-100"
    >
      <Icon className="h-4 w-4" strokeWidth={2} />
    </button>
  )
}

export function Layout() {
  const { user, logout, hasRole } = useAuth()
  const location = useLocation()
  const visibleNavItems = NAV_ITEMS.filter((item) => !item.requireRole || hasRole(item.requireRole))
  const [paletteOpen, setPaletteOpen] = useState(false)
  const [mobileNavOpen, setMobileNavOpen] = useState(false)

  // White-labeling (ROADMAP.md Fase 2) — undefined utk superadmin global
  // (tanpa tenant) atau tenant yang belum set branding -> fallback default.
  const { data: tenant } = useCurrentTenant()
  const brandName = tenant?.brand_name || 'ACS Console'
  const logoUrl = tenant?.logo_url
  const accentStyle = tenant?.primary_color ? { backgroundColor: tenant.primary_color } : undefined
  const accentTextCls = accentTextClass(tenant?.primary_color)

  useEffect(() => {
    function onKeyDown(e: KeyboardEvent) {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault()
        setPaletteOpen((prev) => !prev)
      }
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [])

  // Tutup drawer mobile otomatis tiap kali pindah halaman.
  useEffect(() => {
    setMobileNavOpen(false)
  }, [location.pathname])

  const sidebarContent = (
    <>
      <div className="flex h-16 items-center justify-between gap-2 border-b border-slate-200 px-5 dark:border-slate-800">
        <div className="flex items-center gap-2">
          {logoUrl ? (
            <img src={logoUrl} alt={brandName} className="h-8 w-8 shrink-0 rounded-lg object-cover" />
          ) : (
            <div
              className={accentStyle ? `flex h-8 w-8 shrink-0 items-center justify-center rounded-lg ${accentTextCls}` : 'flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-slate-900 text-white dark:bg-slate-100 dark:text-slate-900'}
              style={accentStyle}
            >
              <Radio className="h-4 w-4" strokeWidth={2} />
            </div>
          )}
          <div className="min-w-0">
            <p className="truncate text-sm font-semibold leading-none text-slate-900 dark:text-slate-100">{brandName}</p>
            <p className="mt-0.5 text-[11px] leading-none text-slate-400 dark:text-slate-500">Multi-Vendor TR-069</p>
          </div>
        </div>
        <button
          onClick={() => setMobileNavOpen(false)}
          className="flex h-7 w-7 items-center justify-center rounded-md text-slate-400 hover:bg-slate-100 md:hidden dark:hover:bg-slate-800"
        >
          <X className="h-4 w-4" />
        </button>
      </div>

      <div className="px-3 pt-3">
        <button
          onClick={() => setPaletteOpen(true)}
          className="flex w-full items-center gap-2 rounded-lg border border-slate-200 bg-slate-50 px-2.5 py-1.5 text-sm text-slate-400 transition-colors hover:border-slate-300 hover:bg-white dark:border-slate-800 dark:bg-slate-900 dark:hover:border-slate-700 dark:hover:bg-slate-800"
        >
          <Search className="h-3.5 w-3.5" />
          <span className="flex-1 text-left">Cari...</span>
          <kbd className="rounded border border-slate-200 bg-white px-1.5 py-0.5 text-[10px] text-slate-400 dark:border-slate-700 dark:bg-slate-950">
            {isMac ? '⌘K' : 'Ctrl+K'}
          </kbd>
        </button>
      </div>

      <nav className="flex-1 space-y-1 px-3 py-4">
        {visibleNavItems.map((item) => (
          <NavLink
            key={item.to}
            to={item.to}
            className={({ isActive }) =>
              `flex items-center gap-2.5 rounded-lg px-3 py-2 text-sm font-medium transition-colors ${
                isActive
                  ? accentStyle
                    ? accentTextCls
                    : 'bg-slate-900 text-white dark:bg-slate-100 dark:text-slate-900'
                  : 'text-slate-600 hover:bg-slate-100 hover:text-slate-900 dark:text-slate-400 dark:hover:bg-slate-800 dark:hover:text-slate-100'
              }`
            }
            style={({ isActive }: { isActive: boolean }) => (isActive ? accentStyle : undefined)}
          >
            <item.icon className="h-4 w-4" strokeWidth={2} />
            {item.label}
          </NavLink>
        ))}
      </nav>

      <div className="border-t border-slate-200 p-3 dark:border-slate-800">
        <div className="flex items-center gap-2.5 rounded-lg px-2 py-2">
          <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-slate-100 text-xs font-semibold text-slate-600 dark:bg-slate-800 dark:text-slate-300">
            {user?.username.slice(0, 2).toUpperCase()}
          </div>
          <div className="min-w-0 flex-1">
            <p className="truncate text-sm font-medium text-slate-900 dark:text-slate-100">{user?.username}</p>
            <p className="truncate text-[11px] text-slate-400 dark:text-slate-500">{user?.roles.join(', ')}</p>
          </div>
          <button
            onClick={logout}
            title="Logout"
            className="flex h-7 w-7 items-center justify-center rounded-md text-slate-400 transition-colors hover:bg-slate-100 hover:text-slate-700 dark:hover:bg-slate-800 dark:hover:text-slate-200"
          >
            <LogOut className="h-4 w-4" strokeWidth={2} />
          </button>
        </div>
      </div>
    </>
  )

  return (
    <NotificationProvider>
      <div className="flex min-h-screen bg-slate-50 dark:bg-slate-950">
        {/* Top bar mobile — sidebar disembunyikan jadi drawer di layar sempit */}
        <div className="fixed inset-x-0 top-0 z-30 flex h-14 items-center justify-between border-b border-slate-200 bg-white px-4 md:hidden dark:border-slate-800 dark:bg-slate-900">
          <button
            onClick={() => setMobileNavOpen(true)}
            className="flex h-8 w-8 items-center justify-center rounded-lg text-slate-500 hover:bg-slate-100 dark:text-slate-400 dark:hover:bg-slate-800"
          >
            <Menu className="h-5 w-5" />
          </button>
          <span className="truncate text-sm font-semibold text-slate-900 dark:text-slate-100">{brandName}</span>
          <div className="flex items-center gap-1">
            <ThemeToggle />
            <NotificationBell />
          </div>
        </div>

        {mobileNavOpen && (
          <div className="fixed inset-0 z-40 bg-slate-900/40 md:hidden" onClick={() => setMobileNavOpen(false)} />
        )}

        <aside
          className={`fixed inset-y-0 left-0 z-50 flex w-64 shrink-0 flex-col border-r border-slate-200 bg-white transition-transform duration-200 md:static md:z-auto md:w-60 md:translate-x-0 dark:border-slate-800 dark:bg-slate-900 ${
            mobileNavOpen ? 'translate-x-0' : '-translate-x-full'
          }`}
        >
          {sidebarContent}
        </aside>

        <main className="min-w-0 flex-1 pt-14 md:pt-0">
          {/* Top bar desktop — toggle tema & notifikasi, sidebar sudah py info user */}
          <div className="hidden items-center justify-end gap-1 border-b border-slate-200 bg-white px-4 py-2 md:flex dark:border-slate-800 dark:bg-slate-900">
            <ThemeToggle />
            <NotificationBell />
          </div>
          <Outlet />
        </main>

        <CommandPalette open={paletteOpen} onClose={() => setPaletteOpen(false)} />
      </div>
    </NotificationProvider>
  )
}
