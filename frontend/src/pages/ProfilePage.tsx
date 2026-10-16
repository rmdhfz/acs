import { useState } from 'react'
import { Link } from 'react-router-dom'
import {
  Building2,
  CheckCircle2,
  Clock3,
  Command,
  Globe2,
  KeyRound,
  Languages,
  LogOut,
  Monitor,
  Moon,
  Palette,
  ShieldCheck,
  Sun,
  Terminal,
  UserCog,
} from 'lucide-react'
import { useAuth } from '../lib/auth'
import { useI18n, type Lang } from '../lib/i18n'
import { useTheme, type Theme } from '../lib/theme'
import { useConfirm } from '../lib/confirm'
import { useApiTokens, useCurrentTenant } from '../lib/hooks'
import { formatDuration, useSessionRemaining } from '../lib/session'
import { ChangePasswordModal } from '../components/ChangePasswordModal'
import { CopyButton } from '../components/CopyButton'
import { MfaCard } from '../components/MfaCard'

const cardCls = 'rounded-xl border border-slate-200 bg-white shadow-sm dark:border-slate-800 dark:bg-slate-900'
const sectionTitleCls = 'flex items-center gap-2 text-sm font-semibold text-slate-900 dark:text-slate-100'

const ROLE_STYLES: Record<string, string> = {
  SUPERADMIN: 'bg-violet-100 text-violet-700 dark:bg-violet-500/15 dark:text-violet-300',
  ADMIN: 'bg-indigo-100 text-indigo-700 dark:bg-indigo-500/15 dark:text-indigo-300',
  NOC: 'bg-sky-100 text-sky-700 dark:bg-sky-500/15 dark:text-sky-300',
  VIEWER: 'bg-slate-100 text-slate-600 dark:bg-slate-800 dark:text-slate-300',
  ENDUSER: 'bg-emerald-100 text-emerald-700 dark:bg-emerald-500/15 dark:text-emerald-300',
}

const THEME_OPTIONS: { value: Theme; icon: typeof Sun; labelKey: string }[] = [
  { value: 'light', icon: Sun, labelKey: 'profile.themeLight' },
  { value: 'dark', icon: Moon, labelKey: 'profile.themeDark' },
  { value: 'system', icon: Monitor, labelKey: 'profile.themeSystem' },
]

const isMac = typeof navigator !== 'undefined' && /Mac|iPhone|iPad/.test(navigator.platform ?? navigator.userAgent)

function Field({ label, children, copy, className = '' }: { label: string; children: React.ReactNode; copy?: string; className?: string }) {
  return (
    <div className={`min-w-0 ${className}`}>
      <dt className="text-[11px] font-medium uppercase tracking-wide text-slate-500 dark:text-slate-400">{label}</dt>
      <dd className="mt-1 flex items-center gap-1.5 text-sm text-slate-800 dark:text-slate-200">
        <span className="min-w-0 truncate">{children}</span>
        {copy && <CopyButton value={copy} />}
      </dd>
    </div>
  )
}

function OptionButton({ active, onClick, children }: { active: boolean; onClick: () => void; children: React.ReactNode }) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-pressed={active}
      className={`flex items-center justify-center gap-2 rounded-lg border px-3 py-2 text-sm font-medium transition-colors ${
        active
          ? 'border-indigo-600 bg-indigo-50 text-indigo-700 dark:border-indigo-400 dark:bg-indigo-500/10 dark:text-indigo-300'
          : 'border-slate-200 text-slate-600 hover:bg-slate-50 dark:border-slate-700 dark:text-slate-300 dark:hover:bg-slate-800'
      }`}
    >
      {children}
    </button>
  )
}

export default function ProfilePage() {
  const { user, hasRole, logout } = useAuth()
  const { t, lang, setLang } = useI18n()
  const { theme, setTheme } = useTheme()
  const confirm = useConfirm()
  const [pwOpen, setPwOpen] = useState(false)
  const remaining = useSessionRemaining()
  const { data: tenant } = useCurrentTenant()

  const isAdmin = hasRole('ADMIN')
  const { data: tokens } = useApiTokens(user?.tenant_id ?? undefined, isAdmin)
  const activeTokens = (tokens?.data ?? []).filter(
    (tk) => !tk.revoked_at && (!tk.expires_at || new Date(tk.expires_at).getTime() > Date.now()),
  ).length

  if (!user) return null
  const initials = user.username.slice(0, 2).toUpperCase()
  const scopeLabel = user.tenant_id ? (tenant?.brand_name || tenant?.name || `#${user.tenant_id}`) : t('profile.scopeGlobal')
  const sessionTone =
    remaining === null ? 'text-slate-500' : remaining <= 0 ? 'text-red-600 dark:text-red-400' : remaining < 15 * 60_000 ? 'text-amber-600 dark:text-amber-400' : 'text-emerald-600 dark:text-emerald-400'

  async function handleSignOut() {
    const ok = await confirm({ title: t('profile.signOut'), message: t('profile.signOutDesc'), confirmLabel: t('top.logout') })
    if (ok) logout()
  }

  return (
    <div className="mx-auto max-w-6xl px-6 py-8">
      <div className="mb-6">
        <h1 className="text-xl font-semibold text-slate-900 dark:text-slate-100">{t('profile.title')}</h1>
        <p className="mt-0.5 text-sm text-slate-500 dark:text-slate-400">{t('profile.subtitle')}</p>
      </div>

      <div className="grid gap-5 lg:grid-cols-3">
        {/* ---------- Kolom utama ---------- */}
        <div className="space-y-5 lg:col-span-2">
          {/* Identitas */}
          <section className={`${cardCls} overflow-hidden`}>
            <div className="h-24 bg-gradient-to-r from-indigo-600 via-violet-600 to-indigo-500" aria-hidden />
            <div className="px-5 pb-5">
              <div className="-mt-10 flex flex-wrap items-start gap-4">
                <div className="flex h-20 w-20 shrink-0 items-center justify-center rounded-2xl bg-white text-2xl font-semibold text-indigo-600 shadow-md ring-4 ring-white dark:bg-slate-800 dark:text-indigo-300 dark:ring-slate-900">
                  {initials}
                </div>
                <div className="min-w-0 pt-12">
                  <p className="truncate text-lg font-semibold text-slate-900 dark:text-slate-100">{user.username}</p>
                  <div className="mt-1.5 flex flex-wrap gap-1.5">
                    {user.roles.map((r) => (
                      <span key={r} className={`rounded-full px-2.5 py-0.5 text-xs font-semibold ${ROLE_STYLES[r] ?? ROLE_STYLES.VIEWER}`}>
                        {r}
                      </span>
                    ))}
                  </div>
                </div>
              </div>

              <dl className="mt-6 grid gap-x-6 gap-y-4 border-t border-slate-100 pt-5 sm:grid-cols-2 dark:border-slate-800">
                <Field label={t('profile.scope')}>
                  <span className="inline-flex items-center gap-1.5">
                    {user.tenant_id ? <Building2 className="h-3.5 w-3.5 text-slate-500 dark:text-slate-400" /> : <Globe2 className="h-3.5 w-3.5 text-slate-500 dark:text-slate-400" />}
                    {scopeLabel}
                  </span>
                </Field>
                <Field label={t('profile.userId')} copy={String(user.id)}>
                  <span className="font-mono">#{user.id}</span>
                </Field>
                <Field label="UUID" copy={user.uuid} className="sm:col-span-2">
                  <span className="font-mono text-xs text-slate-500 dark:text-slate-400">{user.uuid}</span>
                </Field>
              </dl>
            </div>
          </section>

          {/* Keamanan */}
          <section className={`${cardCls} p-5`}>
            <h2 className={sectionTitleCls}>
              <ShieldCheck className="h-4 w-4 text-slate-500 dark:text-slate-400" /> {t('profile.security')}
            </h2>

            <div className="mt-4 flex flex-wrap items-center justify-between gap-3 rounded-lg border border-slate-200 p-3.5 dark:border-slate-700">
              <div className="flex min-w-0 gap-3">
                <KeyRound className="mt-0.5 h-4 w-4 shrink-0 text-slate-500 dark:text-slate-400" />
                <div>
                  <p className="text-sm font-medium text-slate-800 dark:text-slate-200">{t('top.changePassword')}</p>
                  <p className="mt-0.5 text-xs text-slate-500 dark:text-slate-400">{t('profile.changePasswordDesc')}</p>
                </div>
              </div>
              <button
                onClick={() => setPwOpen(true)}
                className="shrink-0 rounded-lg bg-indigo-600 px-3.5 py-1.5 text-sm font-semibold text-white transition-colors hover:bg-indigo-500"
              >
                {t('profile.change')}
              </button>
            </div>

            <h3 className="mb-2 mt-5 text-xs font-medium uppercase tracking-wide text-slate-500 dark:text-slate-400">{t('profile.checklist')}</h3>
            <ul className="space-y-2">
              {['profile.chk.password', 'profile.chk.lockout', 'profile.chk.session', 'profile.chk.sso'].map((k) => (
                <li key={k} className="flex items-start gap-2 text-sm text-slate-600 dark:text-slate-400">
                  <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0 text-emerald-500" />
                  {t(k)}
                </li>
              ))}
            </ul>

            <MfaCard />
          </section>

          {/* Hak akses */}
          <section className={`${cardCls} p-5`}>
            <h2 className={sectionTitleCls}>
              <UserCog className="h-4 w-4 text-slate-500 dark:text-slate-400" /> {t('profile.accessTitle')}
            </h2>
            <p className="mt-1 text-xs text-slate-500 dark:text-slate-400">{t('profile.accessDesc')}</p>
            <ul className="mt-4 space-y-3">
              {user.roles.map((r) => (
                <li key={r} className="flex items-start gap-3">
                  <span className={`mt-0.5 shrink-0 rounded-full px-2.5 py-0.5 text-xs font-semibold ${ROLE_STYLES[r] ?? ROLE_STYLES.VIEWER}`}>{r}</span>
                  <p className="text-sm text-slate-600 dark:text-slate-400">{t(`profile.role.${r}`) === `profile.role.${r}` ? '' : t(`profile.role.${r}`)}</p>
                </li>
              ))}
            </ul>
          </section>

          {/* Preferensi */}
          <section className={`${cardCls} p-5`}>
            <h2 className={sectionTitleCls}>
              <Palette className="h-4 w-4 text-slate-500 dark:text-slate-400" /> {t('profile.prefs')}
            </h2>
            <div className="mt-4 space-y-5">
              <div>
                <p className="mb-2 flex items-center gap-1.5 text-xs font-medium uppercase tracking-wide text-slate-500 dark:text-slate-400">
                  <Languages className="h-3.5 w-3.5" /> {t('profile.language')}
                </p>
                <div className="grid grid-cols-2 gap-2 sm:max-w-sm">
                  {(['id', 'en'] as Lang[]).map((l) => (
                    <OptionButton key={l} active={lang === l} onClick={() => setLang(l)}>
                      {l === 'id' ? 'Bahasa Indonesia' : 'English'}
                    </OptionButton>
                  ))}
                </div>
              </div>
              <div>
                <p className="mb-2 flex items-center gap-1.5 text-xs font-medium uppercase tracking-wide text-slate-500 dark:text-slate-400">
                  <Palette className="h-3.5 w-3.5" /> {t('profile.theme')}
                </p>
                <div className="grid grid-cols-3 gap-2 sm:max-w-md">
                  {THEME_OPTIONS.map(({ value, icon: Icon, labelKey }) => (
                    <OptionButton key={value} active={theme === value} onClick={() => setTheme(value)}>
                      <Icon className="h-4 w-4" /> {t(labelKey)}
                    </OptionButton>
                  ))}
                </div>
              </div>
            </div>
          </section>
        </div>

        {/* ---------- Kolom samping ---------- */}
        <div className="space-y-5">
          <section className={`${cardCls} p-5`}>
            <h2 className={sectionTitleCls}>
              <Clock3 className="h-4 w-4 text-slate-500 dark:text-slate-400" /> {t('profile.session')}
            </h2>
            <p className="mt-3 text-xs text-slate-500 dark:text-slate-400">{t('profile.sessionExpires')}</p>
            <p className={`text-2xl font-semibold tabular-nums ${sessionTone}`}>
              {remaining === null ? t('profile.sessionUnknown') : remaining <= 0 ? t('profile.sessionExpired') : formatDuration(remaining)}
            </p>
            <button
              onClick={handleSignOut}
              className="mt-4 flex w-full items-center justify-center gap-2 rounded-lg border border-red-200 px-3 py-2 text-sm font-medium text-red-600 transition-colors hover:bg-red-50 dark:border-red-500/30 dark:text-red-400 dark:hover:bg-red-500/10"
            >
              <LogOut className="h-4 w-4" /> {t('profile.signOut')}
            </button>
          </section>

          {isAdmin && (
            <section className={`${cardCls} p-5`}>
              <h2 className={sectionTitleCls}>
                <Terminal className="h-4 w-4 text-slate-500 dark:text-slate-400" /> {t('profile.apiTokens')}
              </h2>
              <p className="mt-2 text-xs text-slate-500 dark:text-slate-400">{t('profile.apiTokensDesc')}</p>
              <div className="mt-3 flex items-center justify-between">
                <span className="text-sm font-medium text-slate-800 dark:text-slate-200">{t('profile.apiTokensCount', { n: activeTokens })}</span>
                <Link to="/administration" className="text-sm font-medium text-indigo-600 hover:text-indigo-500 dark:text-indigo-400">
                  {t('profile.manage')} →
                </Link>
              </div>
            </section>
          )}

          <section className={`${cardCls} p-5`}>
            <h2 className={sectionTitleCls}>
              <Command className="h-4 w-4 text-slate-500 dark:text-slate-400" /> {t('profile.shortcuts')}
            </h2>
            <div className="mt-3 flex items-center justify-between gap-3 text-sm text-slate-600 dark:text-slate-400">
              <span>{t('profile.shortcut.search')}</span>
              <kbd className="shrink-0 rounded border border-slate-200 bg-slate-50 px-1.5 py-0.5 text-xs text-slate-500 dark:border-slate-700 dark:bg-slate-800 dark:text-slate-400">
                {isMac ? '⌘K' : 'Ctrl+K'}
              </kbd>
            </div>
          </section>
        </div>
      </div>

      {pwOpen && <ChangePasswordModal onClose={() => setPwOpen(false)} />}
    </div>
  )
}
