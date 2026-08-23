import { useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { Building2, Gauge, KeyRound, Pencil, Palette, Plus, Shield, Trash2, Users as UsersIcon, Wand2 } from 'lucide-react'
import { EmptyState } from '../components/EmptyState'
import { StatusBadge } from '../components/StatusBadge'
import { Modal } from '../components/Modal'
import { PageSpinner } from '../components/Spinner'
import { useAuth } from '../lib/auth'
import { ApiError } from '../lib/api'
import {
  useCreateTenant,
  useCreateUser,
  useCurrentTenant,
  useDeleteUser,
  useRefs,
  useReplaceUserRoles,
  useResetUserPassword,
  useSetTenantCWMPCredentials,
  useSetTenantTaskQuota,
  useTenants,
  useUpdateTenantBranding,
  useUpdateUser,
  useUsers,
  type CreateTenantInput,
  type CreateUserInput,
  type UpdateTenantBrandingInput,
  type UpdateUserInput,
} from '../lib/hooks'
import { formatDateTime } from '../lib/format'
import type { Tenant, User } from '../lib/types'

const inputCls =
  'w-full rounded-lg border border-slate-300 px-3 py-2 text-sm text-slate-900 outline-none transition-colors focus:border-slate-500 focus:ring-1 focus:ring-slate-500 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-100'
const primaryBtnCls =
  'flex items-center justify-center gap-2 rounded-lg bg-slate-900 px-3 py-2 text-sm font-medium text-white transition-colors hover:bg-slate-800 disabled:cursor-not-allowed disabled:opacity-60 dark:bg-slate-100 dark:text-slate-900 dark:hover:bg-white'

type Tab = 'users' | 'tenants' | 'branding'

export function AdministrationPage() {
  const { hasRole } = useAuth()
  const isSuperadmin = hasRole('SUPERADMIN')
  const isAdmin = hasRole('ADMIN')
  const [tab, setTab] = useState<Tab>('users')

  return (
    <div className="mx-auto max-w-7xl px-6 py-8">
      <div className="mb-6">
        <h1 className="text-xl font-semibold text-slate-900 dark:text-slate-100">Administration</h1>
        <p className="mt-0.5 text-sm text-slate-500 dark:text-slate-400">Kelola user dan tenant platform</p>
      </div>

      {isAdmin && (
        <div className="mb-4 flex gap-1 border-b border-slate-200 dark:border-slate-800">
          <TabButton active={tab === 'users'} onClick={() => setTab('users')} icon={UsersIcon} label="Users" />
          {isSuperadmin && <TabButton active={tab === 'tenants'} onClick={() => setTab('tenants')} icon={Building2} label="Tenants" />}
          {!isSuperadmin && <TabButton active={tab === 'branding'} onClick={() => setTab('branding')} icon={Palette} label="Branding" />}
        </div>
      )}

      {tab === 'users' && <UsersTab isSuperadmin={isSuperadmin} />}
      {tab === 'tenants' && isSuperadmin && <TenantsTab />}
      {tab === 'branding' && !isSuperadmin && <MyBrandingTab />}
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
  const { user: currentUser } = useAuth()
  const { data: tenantsResp } = useTenants()
  const tenants = isSuperadmin ? tenantsResp?.data ?? [] : []
  const [tenantFilter, setTenantFilter] = useState('')
  const [showCreate, setShowCreate] = useState(false)
  const [editTarget, setEditTarget] = useState<User | null>(null)
  const [passwordTarget, setPasswordTarget] = useState<User | null>(null)
  const [rolesTarget, setRolesTarget] = useState<User | null>(null)
  const [deleteError, setDeleteError] = useState<string | null>(null)

  const { data, isLoading } = useUsers(tenantFilter ? Number(tenantFilter) : undefined)
  const users = data?.data ?? []
  const deleteMutation = useDeleteUser()

  async function handleDelete(u: User) {
    if (!confirm(`Hapus user "${u.username}"? Aksi ini tidak bisa dibatalkan.`)) return
    setDeleteError(null)
    try {
      await deleteMutation.mutateAsync(u.id)
    } catch (err) {
      setDeleteError(err instanceof ApiError ? err.message : 'Gagal menghapus user')
    }
  }

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

      {deleteError && (
        <div className="mb-3 rounded-lg border border-red-200 bg-red-50 px-4 py-2.5 text-sm text-red-700 dark:border-red-900/50 dark:bg-red-950/30 dark:text-red-400">
          {deleteError}
        </div>
      )}

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
                <th className="px-5 py-3"></th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
              {users.map((u) => {
                const isSelf = currentUser?.id === u.id
                return (
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
                    <td className="px-5 py-3.5 text-right">
                      <div className="flex items-center justify-end gap-3">
                        <button
                          onClick={() => setEditTarget(u)}
                          className="flex items-center gap-1 text-xs font-medium text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:hover:text-slate-100"
                          title="Edit nama/email/status aktif"
                        >
                          <Pencil className="h-3.5 w-3.5" /> Edit
                        </button>
                        <button
                          onClick={() => setPasswordTarget(u)}
                          className="flex items-center gap-1 text-xs font-medium text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:hover:text-slate-100"
                          title="Reset password user ini"
                        >
                          <KeyRound className="h-3.5 w-3.5" /> Reset Password
                        </button>
                        <button
                          onClick={() => setRolesTarget(u)}
                          className="flex items-center gap-1 text-xs font-medium text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:hover:text-slate-100"
                          title="Kelola role user ini"
                        >
                          <Shield className="h-3.5 w-3.5" /> Kelola Role
                        </button>
                        {!isSelf && (
                          <button
                            onClick={() => handleDelete(u)}
                            className="flex items-center gap-1 text-xs font-medium text-red-600 hover:text-red-800 dark:text-red-400 dark:hover:text-red-300"
                            title="Hapus user ini"
                          >
                            <Trash2 className="h-3.5 w-3.5" /> Hapus
                          </button>
                        )}
                      </div>
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        )}
      </div>

      {showCreate && <CreateUserModal isSuperadmin={isSuperadmin} tenants={tenants} onClose={() => setShowCreate(false)} />}
      {editTarget && <EditUserModal user={editTarget} isSelf={currentUser?.id === editTarget.id} onClose={() => setEditTarget(null)} />}
      {passwordTarget && <ResetPasswordModal user={passwordTarget} onClose={() => setPasswordTarget(null)} />}
      {rolesTarget && <ManageRolesModal user={rolesTarget} onClose={() => setRolesTarget(null)} />}
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

function EditUserModal({ user, isSelf, onClose }: { user: User; isSelf: boolean; onClose: () => void }) {
  const [fullName, setFullName] = useState(user.full_name)
  const [email, setEmail] = useState(user.email)
  const [isActive, setIsActive] = useState(user.is_active)
  const [error, setError] = useState<string | null>(null)
  const updateMutation = useUpdateUser()

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    const input: UpdateUserInput = {
      full_name: fullName,
      email,
      is_active: isActive,
    }
    try {
      await updateMutation.mutateAsync({ userId: user.id, input })
      onClose()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Gagal menyimpan perubahan user')
    }
  }

  return (
    <Modal title={`Edit User — ${user.username}`} onClose={onClose}>
      <form onSubmit={handleSubmit} className="space-y-3">
        <div>
          <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Nama Lengkap</label>
          <input value={fullName} onChange={(e) => setFullName(e.target.value)} className={inputCls} />
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Email</label>
          <input type="email" value={email} onChange={(e) => setEmail(e.target.value)} className={inputCls} />
        </div>
        <div>
          <label className="flex items-center gap-1.5 text-sm text-slate-700 dark:text-slate-300">
            <input type="checkbox" checked={isActive} onChange={(e) => setIsActive(e.target.checked)} />
            Akun aktif
          </label>
          {isSelf && (
            <p className="mt-1 text-xs text-slate-400 dark:text-slate-500">
              Ini akun Anda sendiri — menonaktifkannya bisa ditolak backend (self-lockout guard).
            </p>
          )}
        </div>
        {error && <p className="text-sm text-red-600">{error}</p>}
        <button type="submit" disabled={updateMutation.isPending} className={`${primaryBtnCls} w-full`}>
          Simpan Perubahan
        </button>
      </form>
    </Modal>
  )
}

function ResetPasswordModal({ user, onClose }: { user: User; onClose: () => void }) {
  const [newPassword, setNewPassword] = useState('')
  const [error, setError] = useState<string | null>(null)
  const resetMutation = useResetUserPassword()

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    try {
      await resetMutation.mutateAsync({ userId: user.id, newPassword })
      onClose()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Gagal mereset password')
    }
  }

  return (
    <Modal title={`Reset Password — ${user.username}`} onClose={onClose}>
      <form onSubmit={handleSubmit} className="space-y-3">
        <p className="text-xs text-slate-500 dark:text-slate-400">
          Password baru berlaku langsung — beri tahu pemilik akun ini melalui jalur aman di luar console.
        </p>
        <div>
          <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Password Baru</label>
          <input
            required
            type="password"
            value={newPassword}
            onChange={(e) => setNewPassword(e.target.value)}
            className={inputCls}
          />
        </div>
        {error && <p className="text-sm text-red-600">{error}</p>}
        <button type="submit" disabled={resetMutation.isPending} className={`${primaryBtnCls} w-full`}>
          Reset Password
        </button>
      </form>
    </Modal>
  )
}

function ManageRolesModal({ user, onClose }: { user: User; onClose: () => void }) {
  const { data: roleRefs } = useRefs('ref_roles')
  const [roleCodes, setRoleCodes] = useState<string[]>(user.roles)
  const [error, setError] = useState<string | null>(null)
  const replaceMutation = useReplaceUserRoles()

  function toggleRole(code: string) {
    setRoleCodes((prev) => (prev.includes(code) ? prev.filter((r) => r !== code) : [...prev, code]))
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    try {
      await replaceMutation.mutateAsync({ userId: user.id, roleCodes })
      onClose()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Gagal menyimpan role')
    }
  }

  return (
    <Modal title={`Kelola Role — ${user.username}`} onClose={onClose}>
      <form onSubmit={handleSubmit} className="space-y-3">
        <p className="text-xs text-slate-500 dark:text-slate-400">
          Menyimpan akan mengganti seluruh set role user ini sesuai pilihan di bawah (bukan tambah/hapus satu-satu).
        </p>
        <div className="flex flex-wrap gap-3">
          {roleRefs?.map((r) => (
            <label key={r.id} className="flex items-center gap-1.5 text-sm text-slate-700 dark:text-slate-300">
              <input type="checkbox" checked={roleCodes.includes(r.code)} onChange={() => toggleRole(r.code)} />
              {r.name}
            </label>
          ))}
        </div>
        {error && <p className="text-sm text-red-600">{error}</p>}
        <button type="submit" disabled={replaceMutation.isPending} className={`${primaryBtnCls} w-full`}>
          Simpan Role
        </button>
      </form>
    </Modal>
  )
}

// ---- Tenants ----

function TenantsTab() {
  const navigate = useNavigate()
  const { data, isLoading } = useTenants()
  const tenants = data?.data ?? []
  const [showCreate, setShowCreate] = useState(false)
  const [rotateTarget, setRotateTarget] = useState<Tenant | null>(null)
  const [brandingTarget, setBrandingTarget] = useState<Tenant | null>(null)
  const [quotaTarget, setQuotaTarget] = useState<Tenant | null>(null)

  return (
    <div>
      <div className="mb-4 flex justify-end gap-2">
        <button onClick={() => navigate('/administration/onboarding')} className={primaryBtnCls}>
          <Wand2 className="h-4 w-4" /> Onboarding Tenant Baru
        </button>
        <button
          onClick={() => setShowCreate(true)}
          className="flex items-center justify-center gap-2 rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm font-medium text-slate-700 transition-colors hover:bg-slate-50 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-300 dark:hover:bg-slate-800"
        >
          <Plus className="h-4 w-4" /> Tenant Cepat
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
                <th className="px-5 py-3">Kuota Task</th>
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
                  <td className="px-5 py-3.5 text-slate-600 dark:text-slate-400">
                    {t.max_pending_tasks == null ? (
                      <span className="text-xs text-slate-400 dark:text-slate-500">Tidak dibatasi</span>
                    ) : (
                      <span className="font-mono text-xs">{t.max_pending_tasks} pending</span>
                    )}
                  </td>
                  <td className="px-5 py-3.5 text-slate-500 dark:text-slate-400">{formatDateTime(t.created_at)}</td>
                  <td className="px-5 py-3.5 text-right">
                    <div className="flex items-center justify-end gap-3">
                      <button
                        onClick={() => setBrandingTarget(t)}
                        className="flex items-center gap-1 text-xs font-medium text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:hover:text-slate-100"
                        title="Edit branding (nama, logo, warna aksen)"
                      >
                        <Palette className="h-3.5 w-3.5" /> Branding
                      </button>
                      <button
                        onClick={() => setRotateTarget(t)}
                        className="flex items-center gap-1 text-xs font-medium text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:hover:text-slate-100"
                        title="Set/rotate shared secret Inform CWMP"
                      >
                        <KeyRound className="h-3.5 w-3.5" /> Kredensial CWMP
                      </button>
                      <button
                        onClick={() => setQuotaTarget(t)}
                        className="flex items-center gap-1 text-xs font-medium text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:hover:text-slate-100"
                        title="Atur kuota task queue tenant (superadmin only)"
                      >
                        <Gauge className="h-3.5 w-3.5" /> Kuota Task
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {showCreate && <CreateTenantModal onClose={() => setShowCreate(false)} />}
      {rotateTarget && <RotateCWMPCredentialsModal tenant={rotateTarget} onClose={() => setRotateTarget(null)} />}
      {brandingTarget && <EditBrandingModal tenant={brandingTarget} onClose={() => setBrandingTarget(null)} />}
      {quotaTarget && <EditTaskQuotaModal tenant={quotaTarget} onClose={() => setQuotaTarget(null)} />}
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

// EditTaskQuotaModal — kebijakan platform-level (superadmin only, RBAC di
// backend juga menolak ADMIN non-superadmin, lihat usecase/iam.SetTaskQuota).
// Kosongkan field = tidak dibatasi (null), beda dari branding di atas yang
// self-service tenant.
function EditTaskQuotaModal({ tenant, onClose }: { tenant: Tenant; onClose: () => void }) {
  const [value, setValue] = useState(tenant.max_pending_tasks != null ? String(tenant.max_pending_tasks) : '')
  const [error, setError] = useState<string | null>(null)
  const setQuotaMutation = useSetTenantTaskQuota()

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    const trimmed = value.trim()
    const maxPendingTasks = trimmed === '' ? null : Number(trimmed)
    if (maxPendingTasks !== null && (!Number.isInteger(maxPendingTasks) || maxPendingTasks < 0)) {
      setError('Kuota harus bilangan bulat non-negatif, atau kosongkan untuk tidak dibatasi')
      return
    }
    try {
      await setQuotaMutation.mutateAsync({ tenantId: tenant.id, maxPendingTasks })
      onClose()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Gagal menyimpan kuota task')
    }
  }

  return (
    <Modal title={`Kuota Task — ${tenant.name}`} onClose={onClose}>
      <form onSubmit={handleSubmit} className="space-y-3">
        <p className="text-xs text-slate-500 dark:text-slate-400">
          Batas jumlah task berstatus PENDING milik tenant ini lintas semua device-nya — mencegah satu tenant
          menghabiskan resource task queue bersama. Kosongkan untuk tidak dibatasi. Ini bukan pembatas koneksi/sesi
          CWMP itu sendiri.
        </p>
        <div>
          <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Maks. Task Pending</label>
          <input
            type="number"
            min={0}
            value={value}
            onChange={(e) => setValue(e.target.value)}
            placeholder="Tidak dibatasi"
            className={inputCls}
          />
        </div>
        {error && <p className="text-sm text-red-600">{error}</p>}
        <button type="submit" disabled={setQuotaMutation.isPending} className={`${primaryBtnCls} w-full`}>
          Simpan Kuota
        </button>
      </form>
    </Modal>
  )
}

function BrandingForm({
  tenantId,
  initialBrandName,
  initialLogoUrl,
  initialPrimaryColor,
  onSaved,
}: {
  tenantId: number
  initialBrandName: string
  initialLogoUrl: string
  initialPrimaryColor: string
  onSaved?: () => void
}) {
  const [brandName, setBrandName] = useState(initialBrandName)
  const [logoUrl, setLogoUrl] = useState(initialLogoUrl)
  const [primaryColor, setPrimaryColor] = useState(initialPrimaryColor)
  const [error, setError] = useState<string | null>(null)
  const updateMutation = useUpdateTenantBranding()

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    const input: UpdateTenantBrandingInput = {
      brand_name: brandName.trim() || null,
      logo_url: logoUrl.trim() || null,
      primary_color: primaryColor.trim() || null,
    }
    try {
      await updateMutation.mutateAsync({ tenantId, input })
      onSaved?.()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Gagal menyimpan branding')
    }
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-3">
      <p className="text-xs text-slate-500 dark:text-slate-400">
        Kosongkan field untuk kembali ke tampilan default ACS Console. Logo berupa URL eksternal (bukan upload file).
      </p>
      <div>
        <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Nama Brand</label>
        <input
          value={brandName}
          onChange={(e) => setBrandName(e.target.value)}
          placeholder="ACS Console"
          className={inputCls}
        />
      </div>
      <div>
        <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">URL Logo</label>
        <input
          value={logoUrl}
          onChange={(e) => setLogoUrl(e.target.value)}
          placeholder="https://..."
          className={`${inputCls} font-mono text-xs`}
        />
      </div>
      <div>
        <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Warna Aksen</label>
        <div className="flex items-center gap-2">
          <input
            type="color"
            value={/^#[0-9a-fA-F]{6}$/.test(primaryColor) ? primaryColor : '#0f172a'}
            onChange={(e) => setPrimaryColor(e.target.value)}
            className="h-9 w-12 shrink-0 cursor-pointer rounded border border-slate-300 dark:border-slate-700"
          />
          <input
            value={primaryColor}
            onChange={(e) => setPrimaryColor(e.target.value)}
            placeholder="#0f172a"
            className={`${inputCls} font-mono text-xs`}
          />
        </div>
      </div>
      {error && <p className="text-sm text-red-600">{error}</p>}
      <button type="submit" disabled={updateMutation.isPending} className={`${primaryBtnCls} w-full`}>
        Simpan Branding
      </button>
    </form>
  )
}

function EditBrandingModal({ tenant, onClose }: { tenant: Tenant; onClose: () => void }) {
  return (
    <Modal title={`Branding — ${tenant.name}`} onClose={onClose}>
      <BrandingForm
        tenantId={tenant.id}
        initialBrandName={tenant.brand_name ?? ''}
        initialLogoUrl={tenant.logo_url ?? ''}
        initialPrimaryColor={tenant.primary_color ?? ''}
        onSaved={onClose}
      />
    </Modal>
  )
}

function MyBrandingTab() {
  const { data: tenant, isLoading } = useCurrentTenant()

  if (isLoading) return <PageSpinner />

  if (!tenant) {
    return (
      <EmptyState
        icon={Palette}
        title="Tidak ada tenant"
        description="Akun ini tidak terhubung ke tenant manapun, branding tidak berlaku."
      />
    )
  }

  return (
    <div className="max-w-md rounded-xl border border-slate-200 bg-white p-5 shadow-sm dark:border-slate-800 dark:bg-slate-900">
      <p className="mb-4 text-sm text-slate-500 dark:text-slate-400">
        Sesuaikan tampilan console untuk tenant <span className="font-medium text-slate-700 dark:text-slate-300">{tenant.name}</span>.
        Perubahan berlaku untuk semua user di tenant ini.
      </p>
      <BrandingForm
        tenantId={tenant.id}
        initialBrandName={tenant.brand_name ?? ''}
        initialLogoUrl={tenant.logo_url ?? ''}
        initialPrimaryColor={tenant.primary_color ?? ''}
      />
    </div>
  )
}
