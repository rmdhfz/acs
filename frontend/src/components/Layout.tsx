import { useEffect, useRef, useState } from 'react'
import { Link, NavLink, Outlet, useLocation, useNavigate } from 'react-router-dom'
import {
  BarChart3,
  Bell,
  EyeOff,
  Building2,
  Cable,
  FileSliders,
  FolderArchive,
  HardDrive,
  LayoutDashboard,
  LayoutGrid,
  ListChecks,
  LogOut,
  KeyRound,
  Languages,
  Menu,
  Monitor,
  Moon,
  Radio,
  ScrollText,
  Search,
  ShieldCheck,
  SlidersHorizontal,
  Sun,
  Tag,
  X,
} from 'lucide-react'
import { useAuth } from '../lib/auth'
import { useTheme, type Theme } from '../lib/theme'
import { useI18n, type Lang } from '../lib/i18n'
import { useCurrentTenant } from '../lib/hooks'
import { CommandPalette } from './CommandPalette'
import { NotificationBell } from './NotificationBell'
import { NavClock } from './NavClock'
import { ChangePasswordModal } from './ChangePasswordModal'
import { NotificationProvider } from '../lib/notifications'
import { CreditFooter } from './CreditFooter'

const NAV_ITEMS: { to: string; key: string; icon: typeof LayoutGrid; requireRole?: string }[] = [
  { to: '/self-service', key: 'nav.myWifi', icon: Radio, requireRole: 'ENDUSER' },
  { to: '/dashboard', key: 'nav.dashboard', icon: LayoutDashboard, requireRole: 'VIEWER' },
  { to: '/devices', key: 'nav.devices', icon: LayoutGrid, requireRole: 'VIEWER' },
  { to: '/tasks', key: 'nav.tasks', icon: ListChecks, requireRole: 'VIEWER' },
  { to: '/provisioning', key: 'nav.provisioning', icon: FileSliders, requireRole: 'VIEWER' },
  { to: '/presets', key: 'nav.presets', icon: SlidersHorizontal, requireRole: 'ADMIN' },
  { to: '/firmware', key: 'nav.firmware', icon: HardDrive, requireRole: 'VIEWER' },
  { to: '/files', key: 'nav.files', icon: FolderArchive, requireRole: 'VIEWER' },
  { to: '/tags', key: 'nav.tags', icon: Tag, requireRole: 'ADMIN' },
  { to: '/webhooks', key: 'nav.webhooks', icon: Bell, requireRole: 'ADMIN' },
  { to: '/audit', key: 'nav.auditTrail', icon: ScrollText, requireRole: 'ADMIN' },
  { to: '/catalog', key: 'nav.catalog', icon: Cable, requireRole: 'SUPERADMIN' },
  { to: '/reports', key: 'nav.reports', icon: BarChart3, requireRole: 'SUPERADMIN' },
  { to: '/administration', key: 'nav.administration', icon: Building2, requireRole: 'ADMIN' },
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

const iconBtn =
  'inline-flex h-10 w-10 items-center justify-center rounded-full text-slate-600 transition-colors hover:bg-slate-100 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-600 dark:text-slate-300 dark:hover:bg-slate-800'

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
      aria-label={`Ganti tema (saat ini: ${THEME_LABEL[theme]})`}
      className={iconBtn}
    >
      <Icon className="h-5 w-5" strokeWidth={1.75} />
    </button>
  )
}

const LANG_CYCLE: Lang[] = ['id', 'en']

// Banner permanen selama SUPERADMIN meng-impersonate pengguna tenant: tidak bisa disembunyikan, jelas siapa yang sedang
// bertindak, dan satu klik untuk kembali. Aksi sensitif (password/MFA/token API) diblokir server selama mode ini.
function ImpersonationBanner() {
  const { user, stopImpersonation } = useAuth()
  const navigate = useNavigate()
  const [busy, setBusy] = useState(false)
  return (
    <div role="status" className="sticky top-16 z-20 flex flex-wrap items-center justify-between gap-2 border-b border-amber-300 bg-amber-100 px-4 py-2 text-sm text-amber-950 print:hidden dark:border-amber-500/50 dark:bg-amber-900/60 dark:text-amber-50">
      <span className="flex items-center gap-2">
        <EyeOff className="h-4 w-4 shrink-0" />
        <span>
          <strong>Mode impersonate:</strong> Anda bertindak sebagai <strong>{user?.username}</strong> ({user?.roles.join(', ')}). Semua aksi tercatat
          atas nama Anda; sesi berakhir otomatis ≤ 30 menit.
        </span>
      </span>
      <button
        type="button"
        disabled={busy}
        onClick={async () => {
          setBusy(true)
          await stopImpersonation()
          navigate('/administration', { replace: true })
        }}
        className="rounded-full bg-amber-900 px-4 py-1.5 text-xs font-semibold text-white hover:bg-amber-800 disabled:opacity-60 dark:bg-amber-200 dark:text-amber-950 dark:hover:bg-amber-100"
      >
        Keluar dari impersonate
      </button>
    </div>
  )
}

// Menu akun ala Google: avatar bulat di kanan atas membuka kartu berisi identitas, tautan profil, bahasa, dan keluar.
function AccountMenu({ onChangePassword }: { onChangePassword: () => void }) {
  const { user, logout } = useAuth()
  const { lang, setLang, t } = useI18n()
  const [open, setOpen] = useState(false)
  const btnRef = useRef<HTMLButtonElement>(null)
  const panelRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!open) return
    panelRef.current?.querySelector<HTMLElement>('a,button')?.focus()
    const onDown = (e: MouseEvent) => {
      const el = e.target as Node
      if (!panelRef.current?.contains(el) && !btnRef.current?.contains(el)) setOpen(false)
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

  const initials = (user?.username ?? '?').slice(0, 2).toUpperCase()
  const item =
    'flex w-full items-center gap-3 rounded-full px-4 py-2.5 text-left text-sm font-medium text-slate-700 transition-colors hover:bg-slate-100 focus-visible:outline-2 focus-visible:outline-indigo-600 dark:text-slate-200 dark:hover:bg-slate-800'

  return (
    <div className="relative">
      <button
        ref={btnRef}
        type="button"
        onClick={() => setOpen((v) => !v)}
        aria-haspopup="dialog"
        aria-expanded={open}
        aria-label={`Akun: ${user?.username ?? ''}`}
        className="ml-1 flex h-10 w-10 items-center justify-center rounded-full bg-indigo-600 text-sm font-medium text-white transition-shadow hover:shadow-md focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-600"
      >
        {initials}
      </button>
      {open && (
        <div
          ref={panelRef}
          role="dialog"
          aria-label="Menu akun"
          className="absolute right-0 top-12 z-50 w-80 rounded-3xl border border-slate-200 bg-white p-3 shadow-xl dark:border-slate-700 dark:bg-slate-900"
        >
          <div className="flex flex-col items-center px-3 pb-3 pt-4 text-center">
            <div className="flex h-16 w-16 items-center justify-center rounded-full bg-indigo-600 text-xl font-medium text-white" aria-hidden="true">
              {initials}
            </div>
            <p className="mt-3 max-w-full truncate text-base font-medium text-slate-900 dark:text-slate-100">{user?.username}</p>
            <p className="mt-0.5 max-w-full truncate text-xs text-slate-600 dark:text-slate-400">{user?.roles.join(' · ')}</p>
          </div>
          <div className="space-y-0.5 border-t border-slate-200 pt-2 dark:border-slate-700">
            <Link to="/notifications" onClick={() => setOpen(false)} className={item}>
              <Bell className="h-5 w-5 shrink-0 text-slate-600 dark:text-slate-400" strokeWidth={1.75} />
              Notifikasi &amp; preferensi
            </Link>
            <Link to="/profile" onClick={() => setOpen(false)} className={item}>
              <ShieldCheck className="h-5 w-5 shrink-0 text-slate-600 dark:text-slate-400" strokeWidth={1.75} />
              Profil &amp; keamanan
            </Link>
            <button
              type="button"
              onClick={() => {
                setOpen(false)
                onChangePassword()
              }}
              className={item}
            >
              <KeyRound className="h-5 w-5 shrink-0 text-slate-600 dark:text-slate-400" strokeWidth={1.75} />
              {t('top.changePassword')}
            </button>
            <button type="button" onClick={() => setLang(LANG_CYCLE[(LANG_CYCLE.indexOf(lang) + 1) % LANG_CYCLE.length])} className={item}>
              <Languages className="h-5 w-5 shrink-0 text-slate-600 dark:text-slate-400" strokeWidth={1.75} />
              {t('top.language')}: <span className="uppercase">{lang}</span>
            </button>
          </div>
          <div className="mt-2 border-t border-slate-200 pt-2 dark:border-slate-700">
            <button type="button" onClick={logout} className={`${item} text-red-700 hover:bg-red-50 dark:text-red-400 dark:hover:bg-red-500/10`}>
              <LogOut className="h-5 w-5 shrink-0" strokeWidth={1.75} />
              {t('top.logout')}
            </button>
          </div>
        </div>
      )}
    </div>
  )
}

export function Layout() {
  const { user, hasRole } = useAuth()
  const { t } = useI18n()
  const location = useLocation()
  const visibleNavItems = NAV_ITEMS.filter((item) => !item.requireRole || hasRole(item.requireRole))
  const [paletteOpen, setPaletteOpen] = useState(false)
  const [mobileNavOpen, setMobileNavOpen] = useState(false)
  const [pwOpen, setPwOpen] = useState(false)

  // White-labeling (ROADMAP.md Fase 2) — null/undefined utk superadmin global
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

  const brand = (
    <Link to="/" className="flex min-w-0 items-center gap-3 rounded-lg focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-600">
      {logoUrl ? (
        <img src={logoUrl} alt="" className="h-9 w-9 shrink-0 rounded-xl object-cover" />
      ) : (
        <div
          className={accentStyle ? `flex h-9 w-9 shrink-0 items-center justify-center rounded-xl ${accentTextCls}` : 'flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-indigo-600 text-white'}
          style={accentStyle}
        >
          <Radio className="h-5 w-5" strokeWidth={1.75} aria-hidden="true" />
        </div>
      )}
      <span className="truncate text-[17px] font-medium text-slate-900 dark:text-slate-100">{brandName}</span>
    </Link>
  )

  const nav = (
    <nav aria-label="Navigasi utama" className="flex-1 space-y-0.5 overflow-y-auto py-3 pr-3">
      {visibleNavItems.map((item) => (
        <NavLink
          key={item.to}
          to={item.to}
          className={({ isActive }) =>
            `flex h-11 items-center gap-4 rounded-r-full pl-6 pr-4 text-sm font-medium transition-colors focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-indigo-600 ${
              isActive
                ? accentStyle
                  ? accentTextCls
                  : 'bg-indigo-50 text-indigo-700 dark:bg-indigo-500/15 dark:text-indigo-300'
                : 'text-slate-700 hover:bg-slate-100 dark:text-slate-300 dark:hover:bg-slate-800'
            }`
          }
          style={({ isActive }: { isActive: boolean }) => (isActive ? accentStyle : undefined)}
        >
          <item.icon className="h-5 w-5 shrink-0" strokeWidth={1.75} aria-hidden="true" />
          <span className="truncate">{t(item.key)}</span>
        </NavLink>
      ))}
    </nav>
  )

  return (
    <NotificationProvider>
      <div className="min-h-screen bg-slate-50 dark:bg-slate-950">
        {/* App bar (Google): merek di kiri, pencarian pil di tengah, aksi di kanan */}
        <header className="fixed inset-x-0 top-0 z-40 flex h-16 items-center gap-2 border-b border-slate-200 bg-white px-3 print:hidden md:px-4 dark:border-slate-800 dark:bg-slate-900">
          <button type="button" onClick={() => setMobileNavOpen(true)} aria-label="Buka menu navigasi" className={`${iconBtn} md:hidden`}>
            <Menu className="h-6 w-6" strokeWidth={1.75} />
          </button>
          <div className="w-[13.5rem] shrink-0 md:pl-2">{brand}</div>
          <div className="mx-auto hidden w-full max-w-2xl md:block">
            <button
              type="button"
              onClick={() => setPaletteOpen(true)}
              className="flex h-12 w-full items-center gap-3 rounded-full bg-slate-100 px-5 text-left text-sm text-slate-600 transition-colors hover:bg-slate-200/70 hover:shadow-sm focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-600 dark:bg-slate-800 dark:text-slate-300 dark:hover:bg-slate-700"
            >
              <Search className="h-5 w-5 shrink-0" strokeWidth={1.75} aria-hidden="true" />
              <span className="flex-1 truncate">{t('common.search')} perangkat, halaman, aksi…</span>
              <kbd className="hidden rounded-md border border-slate-300 bg-white px-1.5 py-0.5 text-[11px] text-slate-600 lg:inline dark:border-slate-600 dark:bg-slate-900 dark:text-slate-300">
                {isMac ? '⌘K' : 'Ctrl+K'}
              </kbd>
            </button>
          </div>
          <div className="ml-auto flex items-center gap-0.5 md:ml-0">
            <button type="button" onClick={() => setPaletteOpen(true)} aria-label={t('common.search')} className={`${iconBtn} md:hidden`}>
              <Search className="h-5 w-5" strokeWidth={1.75} />
            </button>
            <span className="mr-2 hidden xl:block">
              <NavClock />
            </span>
            <ThemeToggle />
            <NotificationBell />
            <AccountMenu onChangePassword={() => setPwOpen(true)} />
          </div>
        </header>

        {mobileNavOpen && <div className="fixed inset-0 z-40 bg-slate-900/50 md:hidden" onClick={() => setMobileNavOpen(false)} aria-hidden="true" />}

        {/* Drawer navigasi: tetap di desktop, geser dari kiri di mobile */}
        <aside
          className={`print:hidden fixed bottom-0 left-0 z-50 flex w-72 flex-col bg-white transition-transform duration-200 md:top-16 md:z-30 md:w-64 md:translate-x-0 md:border-r md:border-slate-200 dark:bg-slate-900 md:dark:border-slate-800 top-0 ${
            mobileNavOpen ? 'translate-x-0' : '-translate-x-full'
          }`}
        >
          <div className="flex h-16 items-center justify-between border-b border-slate-200 px-4 md:hidden dark:border-slate-800">
            {brand}
            <button type="button" onClick={() => setMobileNavOpen(false)} aria-label="Tutup menu navigasi" className={iconBtn}>
              <X className="h-5 w-5" />
            </button>
          </div>
          {nav}
        </aside>

        <main className="min-w-0 pt-16 md:pl-64">
          {user?.impersonator_id ? <ImpersonationBanner /> : null}
          <Outlet />
          <CreditFooter className="px-6 pb-6 pt-2 text-center print:hidden" />
        </main>

        <CommandPalette open={paletteOpen} onClose={() => setPaletteOpen(false)} />
        {pwOpen && <ChangePasswordModal onClose={() => setPwOpen(false)} />}
      </div>
    </NotificationProvider>
  )
}
