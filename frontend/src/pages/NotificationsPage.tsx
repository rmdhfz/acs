import { useState, type FormEvent } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { BellOff, CheckCheck, Mail, Megaphone, Lock } from 'lucide-react'
import { NotificationRow, SEVERITY_ICON } from '../components/NotificationBell'
import { PageSpinner } from '../components/Spinner'
import { useAuth } from '../lib/auth'
import { useToast } from '../lib/toast'
import { ApiError } from '../lib/api'
import { LIMITS } from '../lib/limits'
import {
  useAnnounce,
  useMarkNotifications,
  useNotificationList,
  useNotificationPreferences,
  useTenants,
  useUpdateNotificationPreferences,
  type NotificationPreference,
  type ServerNotification,
} from '../lib/hooks'

type Tab = 'all' | 'unread' | 'preferences' | 'announce'
const PAGE_SIZE = 20

function Switch({ checked, onChange, disabled, label }: { checked: boolean; onChange: (v: boolean) => void; disabled?: boolean; label: string }) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label={label}
      disabled={disabled}
      onClick={() => onChange(!checked)}
      className={`relative inline-flex h-6 w-11 shrink-0 items-center rounded-full transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-600 disabled:cursor-not-allowed disabled:opacity-60 ${
        checked ? 'bg-indigo-600' : 'bg-slate-400 dark:bg-slate-600'
      }`}
    >
      <span className={`inline-block h-4 w-4 rounded-full bg-white shadow transition-transform ${checked ? 'translate-x-6' : 'translate-x-1'}`} />
    </button>
  )
}

function Preferences() {
  const { data, isLoading } = useNotificationPreferences()
  const save = useUpdateNotificationPreferences()
  const toast = useToast()
  const [draft, setDraft] = useState<Record<string, { in_app: boolean; email: boolean }>>({})

  if (isLoading) return <PageSpinner />
  const prefs = data ?? []
  const val = (p: NotificationPreference) => draft[p.type] ?? { in_app: p.in_app, email: p.email }
  const dirty = Object.keys(draft).length > 0

  async function submit() {
    try {
      await save.mutateAsync(Object.entries(draft).map(([type, v]) => ({ type, ...v })))
      setDraft({})
      toast.success('Preferensi notifikasi disimpan.')
    } catch (e) {
      toast.error(e instanceof ApiError ? e.message : 'Gagal menyimpan preferensi')
    }
  }

  return (
    <div>
      <p className="mb-5 max-w-2xl text-sm leading-relaxed text-slate-600 dark:text-slate-400">
        Pilih kejadian apa yang ingin Anda terima, di Console (ikon lonceng) dan/atau lewat email. Peringatan keamanan akun bersifat wajib dan tidak bisa dimatikan.
      </p>
      <div className="overflow-hidden rounded-2xl border border-slate-200 bg-white dark:border-slate-800 dark:bg-slate-900">
        <div className="hidden grid-cols-[1fr_6rem_6rem] gap-4 border-b border-slate-200 bg-slate-50 px-5 py-3 text-xs font-medium uppercase tracking-wide text-slate-600 sm:grid dark:border-slate-800 dark:bg-slate-800/50 dark:text-slate-400">
          <span>Jenis notifikasi</span>
          <span className="text-center">Di Console</span>
          <span className="text-center">Email</span>
        </div>
        <ul className="divide-y divide-slate-100 dark:divide-slate-800">
          {prefs.map((p) => {
            const v = val(p)
            const sev = SEVERITY_ICON[p.severity] ?? SEVERITY_ICON.INFO
            const Icon = sev.icon
            return (
              <li key={p.type} className="grid grid-cols-[1fr_auto] items-center gap-x-4 gap-y-3 px-5 py-4 sm:grid-cols-[1fr_6rem_6rem]">
                <div className="flex min-w-0 items-start gap-3">
                  <span className={`mt-0.5 flex h-9 w-9 shrink-0 items-center justify-center rounded-full ${sev.cls}`} aria-hidden="true">
                    <Icon className="h-[18px] w-[18px]" strokeWidth={1.75} />
                  </span>
                  <div className="min-w-0">
                    <p className="flex items-center gap-2 text-sm font-medium text-slate-900 dark:text-slate-100">
                      {p.name}
                      {p.mandatory && (
                        <span className="inline-flex items-center gap-1 rounded-full bg-slate-100 px-2 py-0.5 text-[11px] font-medium text-slate-600 dark:bg-slate-800 dark:text-slate-400">
                          <Lock className="h-3 w-3" aria-hidden="true" /> Wajib
                        </span>
                      )}
                    </p>
                    <p className="mt-0.5 text-[13px] text-slate-600 dark:text-slate-400">{p.description}</p>
                  </div>
                </div>
                <div className="flex items-center justify-center gap-6 sm:contents">
                  <label className="flex items-center justify-center gap-2 text-xs text-slate-600 sm:justify-center dark:text-slate-400">
                    <span className="sm:hidden">Console</span>
                    <Switch label={`${p.name} di Console`} checked={v.in_app} disabled={p.mandatory} onChange={(c) => setDraft((d) => ({ ...d, [p.type]: { ...v, in_app: c } }))} />
                  </label>
                  <label className="flex items-center justify-center gap-2 text-xs text-slate-600 sm:justify-center dark:text-slate-400">
                    <span className="sm:hidden">Email</span>
                    <Switch label={`${p.name} lewat email`} checked={v.email} disabled={p.mandatory} onChange={(c) => setDraft((d) => ({ ...d, [p.type]: { ...v, email: c } }))} />
                  </label>
                </div>
              </li>
            )
          })}
        </ul>
      </div>
      <div className="mt-5 flex items-center gap-3">
        <button
          type="button"
          onClick={submit}
          disabled={!dirty || save.isPending}
          className="inline-flex h-11 items-center rounded-full bg-indigo-600 px-7 text-sm font-medium text-white transition-colors hover:bg-indigo-700 disabled:cursor-not-allowed disabled:opacity-50"
        >
          {save.isPending ? 'Menyimpan…' : 'Simpan perubahan'}
        </button>
        {dirty && (
          <button type="button" onClick={() => setDraft({})} className="inline-flex h-11 items-center rounded-full px-5 text-sm font-medium text-indigo-700 hover:bg-indigo-50 dark:text-indigo-300 dark:hover:bg-indigo-500/10">
            Batalkan
          </button>
        )}
      </div>
    </div>
  )
}

function Announce() {
  const announce = useAnnounce()
  const { data: tenants } = useTenants()
  const toast = useToast()
  const [title, setTitle] = useState('')
  const [body, setBody] = useState('')
  const [link, setLink] = useState('')
  const [tenantId, setTenantId] = useState('')
  const [email, setEmail] = useState(true)
  const [error, setError] = useState<string | null>(null)

  async function submit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    if (link && (!link.startsWith('/') || link.startsWith('//'))) {
      setError('Tautan harus berupa path internal Console, mis. /devices (diawali satu "/").')
      return
    }
    try {
      const r = await announce.mutateAsync({ title: title.trim(), body: body.trim(), link: link.trim() || undefined, tenant_id: tenantId ? Number(tenantId) : undefined, email })
      toast.success(`Pengumuman dikirim ke ${r.recipients} penerima.`)
      setTitle('')
      setBody('')
      setLink('')
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Gagal mengirim pengumuman')
    }
  }

  const field =
    'w-full rounded-xl border border-slate-300 bg-white px-4 py-3 text-sm text-slate-900 outline-none transition focus:border-indigo-600 focus:ring-1 focus:ring-indigo-600 dark:border-slate-600 dark:bg-slate-900 dark:text-slate-100'
  return (
    <form onSubmit={submit} className="max-w-2xl space-y-5" noValidate>
      <p className="text-sm leading-relaxed text-slate-600 dark:text-slate-400">
        Kirim pengumuman ke seluruh staf (SUPERADMIN, ADMIN, NOC, VIEWER) — mis. jadwal pemeliharaan atau perubahan kebijakan. Pelanggan portal tidak menerimanya.
      </p>
      <div>
        <label htmlFor="an-title" className="mb-1.5 block text-sm font-medium text-slate-800 dark:text-slate-200">Judul</label>
        <input id="an-title" required maxLength={160} value={title} onChange={(e) => setTitle(e.target.value)} className={field} placeholder="mis. Pemeliharaan terjadwal Sabtu 22.00" />
      </div>
      <div>
        <label htmlFor="an-body" className="mb-1.5 block text-sm font-medium text-slate-800 dark:text-slate-200">Isi pesan</label>
        <textarea id="an-body" rows={4} maxLength={1000} value={body} onChange={(e) => setBody(e.target.value)} className={field} />
        <p className="mt-1 text-xs text-slate-500 dark:text-slate-400">{body.length}/1000 — teks biasa, tanpa HTML.</p>
      </div>
      <div className="grid gap-5 sm:grid-cols-2">
        <div>
          <label htmlFor="an-tenant" className="mb-1.5 block text-sm font-medium text-slate-800 dark:text-slate-200">Penerima</label>
          <select id="an-tenant" value={tenantId} onChange={(e) => setTenantId(e.target.value)} className={field}>
            <option value="">Semua tenant (seluruh platform)</option>
            {(tenants?.data ?? []).map((t) => (
              <option key={t.id} value={t.id}>
                {t.name}
              </option>
            ))}
          </select>
        </div>
        <div>
          <label htmlFor="an-link" className="mb-1.5 block text-sm font-medium text-slate-800 dark:text-slate-200">Tautan (opsional)</label>
          <input id="an-link" maxLength={LIMITS.description} value={link} onChange={(e) => setLink(e.target.value)} className={field} placeholder="/firmware" />
        </div>
      </div>
      <label className="flex items-center gap-3 text-sm text-slate-800 dark:text-slate-200">
        <Switch label="Kirim juga lewat email" checked={email} onChange={setEmail} />
        <span className="flex items-center gap-1.5">
          <Mail className="h-4 w-4 text-slate-500" aria-hidden="true" /> Kirim juga lewat email
        </span>
      </label>
      {error && (
        <p role="alert" className="rounded-xl bg-red-50 px-4 py-3 text-sm text-red-700 dark:bg-red-500/10 dark:text-red-400">
          {error}
        </p>
      )}
      <button
        type="submit"
        disabled={announce.isPending || !title.trim() || !body.trim()}
        className="inline-flex h-11 items-center gap-2 rounded-full bg-indigo-600 px-7 text-sm font-medium text-white transition-colors hover:bg-indigo-700 disabled:cursor-not-allowed disabled:opacity-50"
      >
        <Megaphone className="h-4 w-4" aria-hidden="true" /> {announce.isPending ? 'Mengirim…' : 'Kirim pengumuman'}
      </button>
    </form>
  )
}

function Inbox({ unreadOnly }: { unreadOnly: boolean }) {
  const [page, setPage] = useState(1)
  const navigate = useNavigate()
  const { data, isLoading, isFetching } = useNotificationList({ page, pageSize: PAGE_SIZE, unreadOnly })
  const mark = useMarkNotifications()
  if (isLoading) return <PageSpinner />
  const items: ServerNotification[] = data?.data ?? []
  const total = data?.meta.total ?? 0
  const pages = Math.max(1, Math.ceil(total / PAGE_SIZE))

  function open(n: ServerNotification) {
    if (!n.read_at) mark.mutate({ ids: [n.id] })
    if (n.link && n.link.startsWith('/') && !n.link.startsWith('//')) navigate(n.link)
  }

  if (items.length === 0) {
    return (
      <div className="flex flex-col items-center rounded-3xl border border-dashed border-slate-300 px-6 py-16 text-center dark:border-slate-700">
        <span className="flex h-14 w-14 items-center justify-center rounded-full bg-slate-100 text-slate-500 dark:bg-slate-800 dark:text-slate-400" aria-hidden="true">
          <BellOff className="h-7 w-7" strokeWidth={1.5} />
        </span>
        <p className="mt-4 text-base font-medium text-slate-900 dark:text-slate-100">{unreadOnly ? 'Semua notifikasi sudah dibaca' : 'Belum ada notifikasi'}</p>
        <p className="mt-1 max-w-sm text-sm text-slate-600 dark:text-slate-400">Peringatan perangkat offline massal, task gagal, rollout firmware, dan keamanan akun akan muncul di sini.</p>
      </div>
    )
  }

  return (
    <div>
      <div className="overflow-hidden rounded-2xl border border-slate-200 bg-white dark:border-slate-800 dark:bg-slate-900">
        <ul className="divide-y divide-slate-100 dark:divide-slate-800">
          {items.map((n) => (
            <li key={n.id}>
              <NotificationRow n={n} onOpen={open} />
            </li>
          ))}
        </ul>
      </div>
      <div className="mt-4 flex items-center justify-between text-sm text-slate-600 dark:text-slate-400">
        <span>
          {total} notifikasi · halaman {page}/{pages}
          {isFetching ? ' · memuat…' : ''}
        </span>
        <div className="flex gap-2">
          <button type="button" disabled={page <= 1} onClick={() => setPage((p) => p - 1)} className="inline-flex h-10 items-center rounded-full border border-slate-300 px-5 font-medium text-indigo-700 hover:bg-indigo-50 disabled:cursor-not-allowed disabled:opacity-40 dark:border-slate-600 dark:text-indigo-300 dark:hover:bg-slate-800">
            Sebelumnya
          </button>
          <button type="button" disabled={page >= pages} onClick={() => setPage((p) => p + 1)} className="inline-flex h-10 items-center rounded-full border border-slate-300 px-5 font-medium text-indigo-700 hover:bg-indigo-50 disabled:cursor-not-allowed disabled:opacity-40 dark:border-slate-600 dark:text-indigo-300 dark:hover:bg-slate-800">
            Berikutnya
          </button>
        </div>
      </div>
    </div>
  )
}

export function NotificationsPage() {
  const { hasRole } = useAuth()
  const [params, setParams] = useSearchParams()
  const mark = useMarkNotifications()
  const isSuper = hasRole('SUPERADMIN')
  const wanted = (params.get('tab') as Tab) || 'all'
  const tab: Tab = wanted === 'announce' && !isSuper ? 'all' : wanted

  const tabs: [Tab, string][] = [
    ['all', 'Semua'],
    ['unread', 'Belum dibaca'],
    ['preferences', 'Preferensi'],
    ...(isSuper ? ([['announce', 'Pengumuman']] as [Tab, string][]) : []),
  ]

  return (
    <div className="mx-auto max-w-4xl px-4 py-8 sm:px-6">
      <div className="mb-6 flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="text-2xl font-normal text-slate-900 dark:text-slate-100">Notifikasi</h1>
          <p className="mt-1 text-sm text-slate-600 dark:text-slate-400">Kejadian penting di perangkat, task, firmware, dan keamanan akun Anda.</p>
        </div>
        {(tab === 'all' || tab === 'unread') && (
          <button
            type="button"
            onClick={() => mark.mutate({ all: true })}
            className="inline-flex h-10 items-center gap-2 rounded-full border border-slate-300 px-5 text-sm font-medium text-indigo-700 hover:bg-indigo-50 dark:border-slate-600 dark:text-indigo-300 dark:hover:bg-slate-800"
          >
            <CheckCheck className="h-4 w-4" aria-hidden="true" /> Tandai semua dibaca
          </button>
        )}
      </div>

      <div role="tablist" aria-label="Bagian notifikasi" className="mb-6 flex gap-1 overflow-x-auto border-b border-slate-200 dark:border-slate-800">
        {tabs.map(([k, label]) => (
          <button
            key={k}
            role="tab"
            type="button"
            aria-selected={tab === k}
            onClick={() => setParams(k === 'all' ? {} : { tab: k })}
            className={`-mb-px shrink-0 border-b-2 px-4 py-3 text-sm font-medium transition-colors ${
              tab === k ? 'border-indigo-600 text-indigo-700 dark:border-indigo-300 dark:text-indigo-300' : 'border-transparent text-slate-600 hover:text-slate-900 dark:text-slate-400 dark:hover:text-slate-100'
            }`}
          >
            {label}
          </button>
        ))}
      </div>

      {tab === 'all' && <Inbox unreadOnly={false} />}
      {tab === 'unread' && <Inbox unreadOnly />}
      {tab === 'preferences' && <Preferences />}
      {tab === 'announce' && isSuper && <Announce />}
    </div>
  )
}
