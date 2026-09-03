import { useState } from 'react'
import { KeyRound, ShieldCheck, UserCircle2, Languages, Palette } from 'lucide-react'
import { useAuth } from '../lib/auth'
import { useI18n, type Lang } from '../lib/i18n'
import { useTheme, type Theme } from '../lib/theme'
import { ChangePasswordModal } from '../components/ChangePasswordModal'

const cardCls = 'rounded-xl border border-slate-200 bg-white p-5 shadow-sm dark:border-slate-800 dark:bg-slate-900'

export default function ProfilePage() {
  const { user } = useAuth()
  const { t, lang, setLang } = useI18n()
  const { theme, setTheme } = useTheme()
  const [pwOpen, setPwOpen] = useState(false)

  const initials = user?.username.slice(0, 2).toUpperCase() ?? '?'

  return (
    <div className="mx-auto max-w-3xl px-6 py-8">
      <div className="mb-6">
        <h1 className="text-xl font-semibold text-slate-900 dark:text-slate-100">{t('profile.title')}</h1>
        <p className="mt-0.5 text-sm text-slate-500 dark:text-slate-400">{t('profile.subtitle')}</p>
      </div>

      <div className="space-y-5">
        {/* Akun */}
        <section className={cardCls}>
          <div className="flex items-center gap-4">
            <div className="flex h-14 w-14 shrink-0 items-center justify-center rounded-full bg-slate-100 text-lg font-semibold text-slate-600 dark:bg-slate-800 dark:text-slate-200">
              {initials}
            </div>
            <div className="min-w-0">
              <p className="text-base font-semibold text-slate-900 dark:text-slate-100">{user?.username}</p>
              <div className="mt-1 flex flex-wrap gap-1.5">
                {user?.roles.map((r) => (
                  <span key={r} className="rounded-full bg-slate-100 px-2 py-0.5 text-xs font-medium text-slate-600 dark:bg-slate-800 dark:text-slate-300">
                    {r}
                  </span>
                ))}
              </div>
            </div>
          </div>
          <dl className="mt-5 grid grid-cols-2 gap-x-6 gap-y-3 border-t border-slate-100 pt-4 text-sm dark:border-slate-800">
            <div>
              <dt className="text-xs uppercase tracking-wide text-slate-400">{t('profile.userId')}</dt>
              <dd className="mt-0.5 font-mono text-slate-700 dark:text-slate-300">#{user?.id}</dd>
            </div>
            <div>
              <dt className="text-xs uppercase tracking-wide text-slate-400">{t('common.tenant')}</dt>
              <dd className="mt-0.5 text-slate-700 dark:text-slate-300">
                {user?.tenant_id ? `#${user.tenant_id}` : <span className="text-slate-400">{t('common.none')} (lintas tenant)</span>}
              </dd>
            </div>
            <div className="col-span-2">
              <dt className="text-xs uppercase tracking-wide text-slate-400">UUID</dt>
              <dd className="mt-0.5 break-all font-mono text-xs text-slate-500 dark:text-slate-400">{user?.uuid}</dd>
            </div>
          </dl>
        </section>

        {/* Keamanan */}
        <section className={cardCls}>
          <h2 className="flex items-center gap-2 text-sm font-semibold text-slate-900 dark:text-slate-100">
            <ShieldCheck className="h-4 w-4 text-slate-400" /> {t('profile.security')}
          </h2>
          <div className="mt-4 space-y-3">
            <div className="flex items-start justify-between gap-4 rounded-lg border border-slate-200 p-3.5 dark:border-slate-700">
              <div className="flex gap-3">
                <KeyRound className="mt-0.5 h-4 w-4 shrink-0 text-slate-400" />
                <div>
                  <p className="text-sm font-medium text-slate-800 dark:text-slate-200">{t('top.changePassword')}</p>
                  <p className="mt-0.5 text-xs text-slate-500 dark:text-slate-400">{t('profile.changePasswordDesc')}</p>
                </div>
              </div>
              <button
                onClick={() => setPwOpen(true)}
                className="shrink-0 rounded-lg bg-slate-900 px-3 py-1.5 text-xs font-medium text-white transition-colors hover:bg-slate-800 dark:bg-slate-100 dark:text-slate-900 dark:hover:bg-white"
              >
                {t('profile.change')}
              </button>
            </div>

            <div className="flex items-start gap-3 rounded-lg border border-dashed border-slate-300 p-3.5 dark:border-slate-700">
              <ShieldCheck className="mt-0.5 h-4 w-4 shrink-0 text-slate-400" />
              <div>
                <p className="text-sm font-medium text-slate-800 dark:text-slate-200">
                  {t('profile.2fa')}
                  <span className="ml-2 rounded bg-amber-100 px-1.5 py-0.5 text-[10px] font-semibold uppercase text-amber-700 dark:bg-amber-500/15 dark:text-amber-400">
                    Soon
                  </span>
                </p>
                <p className="mt-0.5 text-xs text-slate-500 dark:text-slate-400">{t('profile.2faDesc')}</p>
                <p className="mt-1.5 text-xs text-slate-400 dark:text-slate-500">{t('profile.2faSoon')}</p>
              </div>
            </div>
          </div>
        </section>

        {/* Preferensi */}
        <section className={cardCls}>
          <h2 className="flex items-center gap-2 text-sm font-semibold text-slate-900 dark:text-slate-100">
            <UserCircle2 className="h-4 w-4 text-slate-400" /> {lang === 'id' ? 'Preferensi' : 'Preferences'}
          </h2>
          <div className="mt-4 grid gap-4 sm:grid-cols-2">
            <div>
              <p className="mb-1.5 flex items-center gap-1.5 text-xs font-medium uppercase tracking-wide text-slate-400">
                <Languages className="h-3.5 w-3.5" /> {t('profile.language')}
              </p>
              <div className="flex gap-1.5">
                {(['id', 'en'] as Lang[]).map((l) => (
                  <button
                    key={l}
                    onClick={() => setLang(l)}
                    className={`rounded-lg border px-3 py-1.5 text-sm font-medium transition-colors ${
                      lang === l
                        ? 'border-slate-900 bg-slate-900 text-white dark:border-slate-100 dark:bg-slate-100 dark:text-slate-900'
                        : 'border-slate-200 text-slate-600 hover:bg-slate-50 dark:border-slate-700 dark:text-slate-300 dark:hover:bg-slate-800'
                    }`}
                  >
                    {l === 'id' ? 'Bahasa Indonesia' : 'English'}
                  </button>
                ))}
              </div>
            </div>
            <div>
              <p className="mb-1.5 flex items-center gap-1.5 text-xs font-medium uppercase tracking-wide text-slate-400">
                <Palette className="h-3.5 w-3.5" /> {t('profile.theme')}
              </p>
              <div className="flex gap-1.5">
                {(['system', 'light', 'dark'] as Theme[]).map((th) => (
                  <button
                    key={th}
                    onClick={() => setTheme(th)}
                    className={`rounded-lg border px-3 py-1.5 text-sm font-medium capitalize transition-colors ${
                      theme === th
                        ? 'border-slate-900 bg-slate-900 text-white dark:border-slate-100 dark:bg-slate-100 dark:text-slate-900'
                        : 'border-slate-200 text-slate-600 hover:bg-slate-50 dark:border-slate-700 dark:text-slate-300 dark:hover:bg-slate-800'
                    }`}
                  >
                    {t(`top.theme.${th}` as const)}
                  </button>
                ))}
              </div>
            </div>
          </div>
        </section>
      </div>

      {pwOpen && <ChangePasswordModal onClose={() => setPwOpen(false)} />}
    </div>
  )
}
