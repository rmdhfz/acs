import { useState, type FormEvent } from 'react'
import { Building2, KeyRound, Plus, Users as UsersIcon } from 'lucide-react'
import { EmptyState } from '../components/EmptyState'
import { StatusBadge } from '../components/StatusBadge'
import { Modal } from '../components/Modal'
import { PageSpinner } from '../components/Spinner'
import { useAuth } from '../lib/auth'
import { ApiError } from '../lib/api'
import {
  useCreateTenant,
  useCreateUser,
  useRefs,
  useSetTenantCWMPCredentials,
  useTenants,
  useUsers,
  type CreateTenantInput,
  type CreateUserInput,
} from '../lib/hooks'
import { formatDateTime } from '../lib/format'
import type { Tenant } from '../lib/types'

const inputCls =
  'w-full rounded-lg border border-slate-300 px-3 py-2 text-sm text-slate-900 outline-none transition-colors focus:border-slate-500 focus:ring-1 focus:ring-slate-500 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-100'
const primaryBtnCls =
  'flex items-center justify-center gap-2 rounded-lg bg-slate-900 px-3 py-2 text-sm font-medium text-white transition-colors hover:bg-slate-800 disabled:cursor-not-allowed disabled:opacity-60 dark:bg-slate-100 dark:text-slate-900 dark:hover:bg-white'

type Tab = 'users' | 'tenants'

export function AdministrationPage() {
  const { hasRole } = useAuth()
  const isSuperadmin = hasRole('SUPERADMIN')
  const [tab, setTab] = useState<Tab>('users')

  return (
    <div className="mx-auto max-w-7xl px-6 py-8">
      <div className="mb-6">
        <h1 className="text-xl font-semibold text-slate-900 dark:text-slate-100">Administration</h1>
        <p className="mt-0.5 text-sm text-slate-500 dark:text-slate-400">Kelola user dan tenant platform</p>
      </div>

      {isSuperadmin && (
        <div className="mb-4 flex gap-1 border-b border-slate-200">
          <TabButton active={tab === 'users'} onClick={() => setTab('users')} icon={UsersIcon} label="Users" />
          <TabButton active={tab === 'tenants'} onClick={() => setTab('tenants')} icon={Building2} label="Tenants" />
        </div>
      )}

      {tab === 'users' || !isSuperadmin ? <UsersTab isSuperadmin={isSuperadmin} /> : <TenantsTab />}
    </div>
  )
}

function TabButton({ active, onClick, icon: Icon, label }: { active: boolean; onClick: () => void; icon: typeof Building2; label: string }) {
  return (
    <button
      onClick={onClick}
      className={`flex items-center gap-1.5 border-b-2 px-3 py-2.5 text-sm font-medium transition-colors ${
        active ? 'border-slate-900 text-slate-900 dark:text-slate-100' : 'border-transparent text-slate-500 dark:text-slate-400 hover:text-slate-800'
      }`}
    >
      <Icon className="h-4 w-4" />
      {label}
    </button>
  )
}

// ---- Users ----

function UsersTab({ isSuperadmin }: { isSuperadmin: boolean }) {
  const { data: tenantsResp } = useTenants()
  const tenants = isSuperadmin ? tenantsResp?.data ?? [] : []
  const [tenantFilter, setTenantFilter] = useState('')
  const [showCreate, setShowCreate] = useState(false)

  const { data, isLoading } = useUsers(tenantFilter ? Number(tenantFilter) : undefined)
  const users = data?.data ?? []

  return (
    <div>
      <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
        {isSuperadmin ? (
          <select value={tenantFilter} onChange={(e) => setTenantFilter(e.target.value)} className={`max-w-xs ${inputCls}`}>
            <option value="">Semua tenant</option>
            {tenants.map((t) => (
              <option key={t.id} value={t.id}>
                {t.name}
              </option>
            ))}
          </select>
        ) : (
          <span />
        )}
        <button onClick={() => setShowCreate(true)} className={primaryBtnCls}>
          <Plus className="h-4 w-4" /> User Baru
        </button>
      </div>

      <div className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm dark:border-slate-800 dark:bg-slate-900">
        {isLoading ? (
          <PageSpinner />
        ) : users.length === 0 ? (
          <EmptyState icon={UsersIcon} title="Belum ada user" />
        ) : (
          <table className="w-full text-left text-sm">
            <thead>
              <tr className="border-b border-slate-200 bg-slate-50 text-xs font-medium uppercase tracking-wide text-slate-500 dark:border-slate-800 dark:bg-slate-800/50 dark:text-slate-400">
                <th className="px-5 py-3">Username</th>
                <th className="px-5 py-3">Nama</th>
                <th className="px-5 py-3">Email</th>
                <th className="px-5 py-3">Role</th>
                <th className="px-5 py-3">Status</th>
                <th className="px-5 py-3">Login Terakhir</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
              {users.map((u) => (
                <tr key={u.id}>
                  <td className="px-5 py-3.5 font-mono text-xs text-slate-900 dark:text-slate-100">{u.username}</td>
                  <td className="px-5 py-3.5 text-slate-700 dark:text-slate-300">{u.full_name || '-'}</td>
                  <td className="px-5 py-3.5 text-slate-500 dark:text-slate-400">{u.email || '-'}</td>
                  <td className="px-5 py-3.5">
                    <div className="flex flex-wrap gap-1">
                      {u.roles.length === 0 ? <span className="text-xs text-slate-400 dark:text-slate-500">-</span> : u.roles.map((r) => <StatusBadge key={r} code="PROVISIONING" label={r} />)}
                    </div>
                  </td>
                  <td className="px-5 py-3.5">
                    <StatusBadge code={u.is_active ? 'ONLINE' : 'OFFLINE'} label={u.is_active ? 'Aktif' : 'Nonaktif'} />
                  </td>
                  <td className="px-5 py-3.5 text-slate-500 dark:text-slate-400">{formatDateTime(u.last_login_at)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {showCreate && <CreateUserModal isSuperadmin={isSuperadmin} tenants={tenants} onClose={() => setShowCreate(false)} />}
    </div>
  )
}

function CreateUserModal({
  isSuperadmin,
  tenants,
  onClose,
}: {
  isSuperadmin: boolean
  tenants: { id: number; name: string }[]
  onClose: () => void
}) {
  const { data: roleRefs } = useRefs('ref_roles')
  const [tenantId, setTenantId] = useState('')
  const [username, setUsername] = useState('')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [fullName, setFullName] = useState('')
  const [roleCodes, setRoleCodes] = useState<string[]>([])
  const [error, setError] = useState<string | null>(null)
  const createMutation = useCreateUser()

  function toggleRole(code: string) {
    setRoleCodes((prev) => (prev.includes(code) ? prev.filter((r) => r !== code) : [...prev, code]))
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    const input: CreateUserInput = {
      tenant_id: tenantId ? Number(tenantId) : undefined,
      username,
      email,
      password,
      full_name: fullName,
      role_codes: roleCodes,
    }
    try {
      await createMutation.mutateAsync(input)
      onClose()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Gagal membuat user')
    }
  }

  return (
    <Modal title="User Baru" onClose={onClose}>
      <form onSubmit={handleSubmit} className="space-y-3">
        {isSuperadmin && (
          <div>
            <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Tenant</label>
            <select value={tenantId} onChange={(e) => setTenantId(e.target.value)} className={inputCls}>
              <option value="">- (lintas tenant / global)</option>
              {tenants.map((t) => (
                <option key={t.id} value={t.id}>
                  {t.name}
                </option>
              ))}
            </select>
          </div>
        )}
        <div>
          <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Username</label>
          <input required value={username} onChange={(e) => setUsername(e.target.value)} className={inputCls} />
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Nama Lengkap</label>
          <input value={fullName} onChange={(e) => setFullName(e.target.value)} className={inputCls} />
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Email</label>
          <input type="email" value={email} onChange={(e) => setEmail(e.target.value)} className={inputCls} />
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Password</label>
          <input type="password" required value={password} onChange={(e) => setPassword(e.target.value)} className={inputCls} />
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Role</label>
          <div className="flex flex-wrap gap-3">
            {roleRefs?.map((r) => (
              <label key={r.id} className="flex items-center gap-1.5 text-sm text-slate-700 dark:text-slate-300">
                <input type="checkbox" checked={roleCodes.includes(r.code)} onChange={() => toggleRole(r.code)} />
                {r.name}
              </label>
            ))}
          </div>
        </div>
        {error && <p className="text-sm text-red-600">{error}</p>}
        <button type="submit" disabled={createMutation.isPending} className={`${primaryBtnCls} w-full`}>
          Buat User
        </button>
      </form>
    </Modal>
  )
}

// ---- Tenants ----

function TenantsTab() {
  const { data, isLoading } = useTenants()
  const tenants = data?.data ?? []
  const [showCreate, setShowCreate] = useState(false)
  const [rotateTarget, setRotateTarget] = useState<Tenant | null>(null)

  return (
    <div>
      <div className="mb-4 flex justify-end">
        <button onClick={() => setShowCreate(true)} className={primaryBtnCls}>
          <Plus className="h-4 w-4" /> Tenant Baru
        </button>
      </div>

      <div className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm dark:border-slate-800 dark:bg-slate-900">
        {isLoading ? (
          <PageSpinner />
        ) : tenants.length === 0 ? (
          <EmptyState icon={Building2} title="Belum ada tenant" />
        ) : (
          <table className="w-full text-left text-sm">
            <thead>
              <tr className="border-b border-slate-200 bg-slate-50 text-xs font-medium uppercase tracking-wide text-slate-500 dark:border-slate-800 dark:bg-slate-800/50 dark:text-slate-400">
                <th className="px-5 py-3">Code</th>
                <th className="px-5 py-3">Nama</th>
                <th className="px-5 py-3">Status</th>
                <th className="px-5 py-3">Shared Secret CWMP</th>
                <th className="px-5 py-3">Dibuat</th>
                <th className="px-5 py-3"></th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
              {tenants.map((t) => (
                <tr key={t.id}>
                  <td className="px-5 py-3.5 font-mono text-xs text-slate-700 dark:text-slate-300">{t.code}</td>
                  <td className="px-5 py-3.5 font-medium text-slate-900 dark:text-slate-100">{t.name}</td>
                  <td className="px-5 py-3.5">
                    <StatusBadge code={t.is_active ? 'ONLINE' : 'OFFLINE'} label={t.is_active ? 'Aktif' : 'Nonaktif'} />
                  </td>
                  <td className="px-5 py-3.5">
                    {t.cwmp_inform_username ? (
                      <span className="font-mono text-xs text-slate-600 dark:text-slate-400">{t.cwmp_inform_username}</span>
                    ) : (
                      <StatusBadge code="FAULTY" label="Belum diset" />
                    )}
                  </td>
                  <td className="px-5 py-3.5 text-slate-500 dark:text-slate-400">{formatDateTime(t.created_at)}</td>
                  <td className="px-5 py-3.5 text-right">
                    <button
                      onClick={() => setRotateTarget(t)}
                      className="flex items-center gap-1 text-xs font-medium text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:hover:text-slate-100"
                      title="Set/rotate shared secret Inform CWMP"
                    >
                      <KeyRound className="h-3.5 w-3.5" /> Kredensial CWMP
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {showCreate && <CreateTenantModal onClose={() => setShowCreate(false)} />}
      {rotateTarget && <RotateCWMPCredentialsModal tenant={rotateTarget} onClose={() => setRotateTarget(null)} />}
    </div>
  )
}

function CreateTenantModal({ onClose }: { onClose: () => void }) {
  const [code, setCode] = useState('')
  const [name, setName] = useState('')
  const [cwmpUsername, setCwmpUsername] = useState('')
  const [cwmpPassword, setCwmpPassword] = useState('')
  const [error, setError] = useState<string | null>(null)
  const createMutation = useCreateTenant()

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    const input: CreateTenantInput = {
      code,
      name,
      cwmp_inform_username: cwmpUsername || undefined,
      cwmp_inform_password: cwmpPassword || undefined,
    }
    try {
      await createMutation.mutateAsync(input)
      onClose()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Gagal membuat tenant')
    }
  }

  return (
    <Modal title="Tenant Baru" onClose={onClose}>
      <form onSubmit={handleSubmit} className="space-y-3">
        <div>
          <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Code</label>
          <input required value={code} onChange={(e) => setCode(e.target.value)} className={inputCls} />
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Nama</label>
          <input required value={name} onChange={(e) => setName(e.target.value)} className={inputCls} />
        </div>
        <div className="border-t border-slate-100 pt-3">
          <p className="mb-2 text-xs text-slate-500 dark:text-slate-400">
            Shared secret Inform CWMP (opsional, bisa diisi belakangan) — dipakai memvalidasi device baru milik tenant
            ini sebelum CPE punya kredensial sendiri. Beri tahu username/password ini ke teknisi yang memprovisioning
            CPE tenant ini.
          </p>
          <div className="grid grid-cols-2 gap-3">
            <input
              placeholder="Username Inform"
              value={cwmpUsername}
              onChange={(e) => setCwmpUsername(e.target.value)}
              className={`${inputCls} font-mono text-xs`}
            />
            <input
              placeholder="Password Inform"
              type="password"
              value={cwmpPassword}
              onChange={(e) => setCwmpPassword(e.target.value)}
              className={`${inputCls} font-mono text-xs`}
            />
          </div>
        </div>
        {error && <p className="text-sm text-red-600">{error}</p>}
        <button type="submit" disabled={createMutation.isPending} className={`${primaryBtnCls} w-full`}>
          Buat Tenant
        </button>
      </form>
    </Modal>
  )
}

function RotateCWMPCredentialsModal({ tenant, onClose }: { tenant: Tenant; onClose: () => void }) {
  const [username, setUsername] = useState(tenant.cwmp_inform_username ?? '')
  const [password, setPassword] = useState('')
  const [error, setError] = useState<string | null>(null)
  const setCredsMutation = useSetTenantCWMPCredentials()

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    try {
      await setCredsMutation.mutateAsync({ tenantId: tenant.id, username, password })
      onClose()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Gagal menyimpan kredensial')
    }
  }

  return (
    <Modal title={`Kredensial CWMP — ${tenant.name}`} onClose={onClose}>
      <form onSubmit={handleSubmit} className="space-y-3">
        <p className="text-xs text-slate-500 dark:text-slate-400">
          Password lama tidak ditampilkan (tersimpan terenkripsi). Mengisi form ini akan menimpa/mengganti kredensial
          yang ada — pastikan teknisi lapangan mengetahui perubahan ini sebelum menyimpan.
        </p>
        <div>
          <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Username</label>
          <input required value={username} onChange={(e) => setUsername(e.target.value)} className={`${inputCls} font-mono text-xs`} />
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Password Baru</label>
          <input required type="password" value={password} onChange={(e) => setPassword(e.target.value)} className={`${inputCls} font-mono text-xs`} />
        </div>
        {error && <p className="text-sm text-red-600">{error}</p>}
        <button type="submit" disabled={setCredsMutation.isPending} className={`${primaryBtnCls} w-full`}>
          Simpan
        </button>
      </form>
    </Modal>
  )
}
