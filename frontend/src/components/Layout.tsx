import { NavLink, Outlet } from 'react-router-dom'
import { Building2, Cable, FileSliders, HardDrive, LayoutDashboard, LayoutGrid, ListChecks, LogOut, Radio } from 'lucide-react'
import { useAuth } from '../lib/auth'

const NAV_ITEMS: { to: string; label: string; icon: typeof LayoutGrid; requireRole?: string }[] = [
  { to: '/dashboard', label: 'Dashboard', icon: LayoutDashboard },
  { to: '/devices', label: 'Devices', icon: LayoutGrid },
  { to: '/tasks', label: 'Tasks', icon: ListChecks },
  { to: '/provisioning', label: 'Provisioning', icon: FileSliders },
  { to: '/firmware', label: 'Firmware', icon: HardDrive },
  { to: '/catalog', label: 'Catalog Vendor', icon: Cable, requireRole: 'SUPERADMIN' },
  { to: '/administration', label: 'Administration', icon: Building2, requireRole: 'ADMIN' },
]

export function Layout() {
  const { user, logout, hasRole } = useAuth()
  const visibleNavItems = NAV_ITEMS.filter((item) => !item.requireRole || hasRole(item.requireRole))

  return (
    <div className="flex min-h-screen bg-slate-50">
      <aside className="flex w-60 shrink-0 flex-col border-r border-slate-200 bg-white">
        <div className="flex h-16 items-center gap-2 border-b border-slate-200 px-5">
          <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-slate-900 text-white">
            <Radio className="h-4 w-4" strokeWidth={2} />
          </div>
          <div>
            <p className="text-sm font-semibold leading-none text-slate-900">ACS Console</p>
            <p className="text-[11px] leading-none text-slate-400 mt-0.5">Multi-Vendor TR-069</p>
          </div>
        </div>

        <nav className="flex-1 space-y-1 px-3 py-4">
          {visibleNavItems.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              className={({ isActive }) =>
                `flex items-center gap-2.5 rounded-lg px-3 py-2 text-sm font-medium transition-colors ${
                  isActive ? 'bg-slate-900 text-white' : 'text-slate-600 hover:bg-slate-100 hover:text-slate-900'
                }`
              }
            >
              <item.icon className="h-4 w-4" strokeWidth={2} />
              {item.label}
            </NavLink>
          ))}
        </nav>

        <div className="border-t border-slate-200 p-3">
          <div className="flex items-center gap-2.5 rounded-lg px-2 py-2">
            <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-slate-100 text-xs font-semibold text-slate-600">
              {user?.username.slice(0, 2).toUpperCase()}
            </div>
            <div className="min-w-0 flex-1">
              <p className="truncate text-sm font-medium text-slate-900">{user?.username}</p>
              <p className="truncate text-[11px] text-slate-400">{user?.roles.join(', ')}</p>
            </div>
            <button
              onClick={logout}
              title="Logout"
              className="flex h-7 w-7 items-center justify-center rounded-md text-slate-400 transition-colors hover:bg-slate-100 hover:text-slate-700"
            >
              <LogOut className="h-4 w-4" strokeWidth={2} />
            </button>
          </div>
        </div>
      </aside>

      <main className="min-w-0 flex-1">
        <Outlet />
      </main>
    </div>
  )
}
