import { useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { ArrowLeft, Building2, Cable, Check, FileSliders, ShieldAlert, UserPlus } from 'lucide-react'
import { EmptyState } from '../components/EmptyState'
import { useAuth } from '../lib/auth'
import { ApiError } from '../lib/api'
import { useCreateTenant, useCreateUser, type CreateTenantInput } from '../lib/hooks'
import { LIMITS } from '../lib/limits'
import type { Tenant, User } from '../lib/types'

const inputCls =
  'w-full rounded-lg border border-slate-300 px-3 py-2 text-sm text-slate-900 outline-none transition-colors focus:border-slate-500 focus:ring-1 focus:ring-slate-500 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-100'
const primaryBtnCls =
  'flex items-center justify-center gap-2 rounded-lg bg-slate-900 px-4 py-2.5 text-sm font-medium text-white transition-colors hover:bg-slate-800 disabled:cursor-not-allowed disabled:opacity-60 dark:bg-slate-100 dark:text-slate-900 dark:hover:bg-white'
const secondaryBtnCls =
  'flex items-center justify-center gap-2 rounded-lg border border-slate-300 bg-white px-4 py-2.5 text-sm font-medium text-slate-700 transition-colors hover:bg-slate-50 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-300 dark:hover:bg-slate-800'

type WizardStep = 1 | 2 | 3

const STEPS: { key: WizardStep; label: string }[] = [
  { key: 1, label: 'Data Tenant' },
  { key: 2, label: 'Admin Pertama' },
  { key: 3, label: 'Selesai' },
]

export function TenantOnboardingWizard() {
  const navigate = useNavigate()
  const { hasRole } = useAuth()

  const [step, setStep] = useState<WizardStep>(1)
  const [tenant, setTenant] = useState<Tenant | null>(null)
  const [adminUser, setAdminUser] = useState<User | null>(null)

  if (!hasRole('SUPERADMIN')) {
    return (
      <div className="mx-auto max-w-2xl px-6 py-16">
        <EmptyState
          icon={ShieldAlert}
          title="Tidak punya akses"
          description="Onboarding tenant baru hanya bisa dilakukan superadmin — membuat tenant adalah operasi lintas-tenant global."
        />
      </div>
    )
  }

  return (
    <div className="mx-auto max-w-2xl px-6 py-8">
      <button
        onClick={() => navigate('/administration')}
        className="mb-4 flex items-center gap-1.5 text-sm text-slate-500 transition-colors hover:text-slate-900 dark:text-slate-400 dark:hover:text-slate-100"
      >
        <ArrowLeft className="h-4 w-4" /> Kembali ke Administration
      </button>

      <div className="mb-8">
        <h1 className="text-xl font-semibold text-slate-900 dark:text-slate-100">Onboarding Tenant Baru</h1>
        <p className="mt-0.5 text-sm text-slate-500 dark:text-slate-400">
          Alur terpandu utk mendaftarkan satu perusahaan baru di Group: buat tenant, admin pertamanya, lalu langkah lanjutan.
        </p>
      </div>

      <div className="mb-8 flex items-center">
        {STEPS.map((s, i) => (
          <div key={s.key} className="flex flex-1 items-center last:flex-none">
            <div className="flex flex-col items-center gap-1.5">
              <div
                className={`flex h-8 w-8 items-center justify-center rounded-full text-sm font-medium ${
                  step > s.key
                    ? 'bg-emerald-500 text-white'
                    : step === s.key
                      ? 'bg-slate-900 text-white dark:bg-slate-100 dark:text-slate-900'
                      : 'bg-slate-100 text-slate-400 dark:bg-slate-800 dark:text-slate-500'
                }`}
              >
                {step > s.key ? <Check className="h-4 w-4" /> : s.key}
              </div>
              <span className="text-[11px] font-medium text-slate-500 dark:text-slate-400">{s.label}</span>
            </div>
            {i < STEPS.length - 1 && (
              <div className={`mx-2 h-0.5 flex-1 ${step > s.key ? 'bg-emerald-500' : 'bg-slate-200 dark:bg-slate-800'}`} />
            )}
          </div>
        ))}
      </div>

      {step === 1 && (
        <TenantStep
          onCreated={(t) => {
            setTenant(t)
            setStep(2)
          }}
        />
      )}
      {step === 2 && tenant && (
        <AdminUserStep
          tenant={tenant}
          onCreated={(u) => {
            setAdminUser(u)
            setStep(3)
          }}
          onBack={() => setStep(1)}
        />
      )}
      {step === 3 && tenant && <DoneStep tenant={tenant} adminUser={adminUser} />}
    </div>
  )
}

function StepCard({ children }: { children: React.ReactNode }) {
  return <div className="rounded-xl border border-slate-200 bg-white p-6 shadow-sm dark:border-slate-800 dark:bg-slate-900">{children}</div>
}

function TenantStep({ onCreated }: { onCreated: (tenant: Tenant) => void }) {
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
      const tenant = await createMutation.mutateAsync(input)
      onCreated(tenant)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Gagal membuat tenant')
    }
  }

  return (
    <StepCard>
      <div className="mb-4 flex items-center gap-2.5">
        <Building2 className="h-5 w-5 text-slate-400" />
        <h2 className="text-sm font-semibold text-slate-900 dark:text-slate-100">Langkah 1 — Data Tenant</h2>
      </div>
      <form onSubmit={handleSubmit} className="space-y-3">
        <div>
          <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Code</label>
          <input required maxLength={LIMITS.tenant.code} value={code} onChange={(e) => setCode(e.target.value)} placeholder="mis. ISP-JKT" className={inputCls} />
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Nama Perusahaan</label>
          <input required maxLength={LIMITS.tenant.name} value={name} onChange={(e) => setName(e.target.value)} placeholder="mis. ISP Jakarta Sejahtera" className={inputCls} />
        </div>
        <div className="border-t border-slate-100 pt-3 dark:border-slate-800">
          <p className="mb-2 text-xs text-slate-500 dark:text-slate-400">
            Shared secret Inform CWMP (opsional, bisa diisi belakangan dari Administration) — dibutuhkan teknisi saat
            memprovisioning CPE pertama tenant ini.
          </p>
          <div className="grid grid-cols-2 gap-3">
            <input
              placeholder="Username Inform"
              maxLength={LIMITS.tenant.cwmpUsername}
              value={cwmpUsername}
              onChange={(e) => setCwmpUsername(e.target.value)}
              className={`${inputCls} font-mono text-xs`}
            />
            <input
              placeholder="Password Inform (min. 16 karakter)"
              type="password"
              value={cwmpPassword}
              onChange={(e) => setCwmpPassword(e.target.value)}
              className={`${inputCls} font-mono text-xs`}
            />
          </div>
        </div>
        {error && <p className="text-sm text-red-600 dark:text-red-400">{error}</p>}
        <div className="flex justify-end pt-2">
          <button type="submit" disabled={createMutation.isPending} className={primaryBtnCls}>
            Lanjut ke Admin Pertama
          </button>
        </div>
      </form>
    </StepCard>
  )
}

function AdminUserStep({ tenant, onCreated, onBack }: { tenant: Tenant; onCreated: (user: User) => void; onBack: () => void }) {
  const [username, setUsername] = useState('')
  const [fullName, setFullName] = useState('')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState<string | null>(null)
  const createUserMutation = useCreateUser()

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    try {
      const user = await createUserMutation.mutateAsync({
        tenant_id: tenant.id,
        username,
        email,
        password,
        full_name: fullName,
        role_codes: ['ADMIN'],
      })
      onCreated(user)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Gagal membuat admin')
    }
  }

  return (
    <StepCard>
      <div className="mb-4 flex items-center gap-2.5">
        <UserPlus className="h-5 w-5 text-slate-400" />
        <h2 className="text-sm font-semibold text-slate-900 dark:text-slate-100">
          Langkah 2 — Admin Pertama utk <span className="font-mono">{tenant.name}</span>
        </h2>
      </div>
      <form onSubmit={handleSubmit} className="space-y-3">
        <p className="text-xs text-slate-500 dark:text-slate-400">
          User ini akan dapat role <span className="font-medium">ADMIN</span> ter-scope ke tenant <strong>{tenant.name}</strong> —
          bisa kelola profil provisioning, aturan zero-touch, firmware, dan user lain di tenant-nya sendiri.
        </p>
        <div>
          <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Username</label>
          <input required maxLength={LIMITS.user.username} value={username} onChange={(e) => setUsername(e.target.value)} placeholder="mis. admin.jkt" className={inputCls} />
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Nama Lengkap</label>
          <input maxLength={LIMITS.user.fullName} value={fullName} onChange={(e) => setFullName(e.target.value)} placeholder="mis. Budi Santoso" className={inputCls} />
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Email</label>
          <input type="email" maxLength={LIMITS.user.email} value={email} onChange={(e) => setEmail(e.target.value)} placeholder="mis. budi@ispjakarta.co.id" className={inputCls} />
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Password</label>
          <input type="password" required value={password} onChange={(e) => setPassword(e.target.value)} className={inputCls} />
        </div>
        {error && <p className="text-sm text-red-600 dark:text-red-400">{error}</p>}
        <div className="flex justify-between pt-2">
          <button type="button" onClick={onBack} className={secondaryBtnCls}>
            Kembali
          </button>
          <button type="submit" disabled={createUserMutation.isPending} className={primaryBtnCls}>
            Buat Admin & Selesai
          </button>
        </div>
      </form>
    </StepCard>
  )
}

function DoneStep({ tenant, adminUser }: { tenant: Tenant; adminUser: User | null }) {
  const navigate = useNavigate()
  return (
    <StepCard>
      <div className="mb-4 flex items-center gap-2.5">
        <Check className="h-5 w-5 text-emerald-500" />
        <h2 className="text-sm font-semibold text-slate-900 dark:text-slate-100">Tenant Siap</h2>
      </div>
      <dl className="mb-6 space-y-2 rounded-lg bg-slate-50 p-4 text-sm dark:bg-slate-800/50">
        <div className="flex justify-between">
          <dt className="text-slate-500 dark:text-slate-400">Tenant</dt>
          <dd className="font-medium text-slate-900 dark:text-slate-100">
            {tenant.name} ({tenant.code})
          </dd>
        </div>
        {adminUser && (
          <div className="flex justify-between">
            <dt className="text-slate-500 dark:text-slate-400">Admin pertama</dt>
            <dd className="font-mono text-xs text-slate-900 dark:text-slate-100">{adminUser.username}</dd>
          </div>
        )}
        {!tenant.cwmp_inform_username && (
          <p className="pt-1 text-xs text-amber-600 dark:text-amber-400">
            Shared secret Inform CWMP belum diset — CPE tenant ini belum bisa Inform sampai kredensial ditambahkan dari Administration.
          </p>
        )}
      </dl>

      <p className="mb-3 text-sm font-medium text-slate-700 dark:text-slate-300">Langkah lanjutan yang disarankan (opsional):</p>
      <div className="space-y-2">
        <button onClick={() => navigate('/provisioning')} className={`${secondaryBtnCls} w-full justify-start gap-3`}>
          <FileSliders className="h-4 w-4 text-slate-400" />
          Buat Provisioning Profile & Zero-Touch Rule default
        </button>
        <button onClick={() => navigate('/catalog')} className={`${secondaryBtnCls} w-full justify-start gap-3`}>
          <Cable className="h-4 w-4 text-slate-400" />
          Cek Catalog Vendor (pastikan vendor CPE tenant ini sudah terdaftar)
        </button>
      </div>

      <div className="mt-6 flex justify-end border-t border-slate-100 pt-4 dark:border-slate-800">
        <button onClick={() => navigate('/administration')} className={primaryBtnCls}>
          Selesai, ke Administration
        </button>
      </div>
    </StepCard>
  )
}
