import type { ReactNode } from 'react'
import { Cable, Layers3, Languages, Monitor, Moon, Radio, ShieldCheck, Sun, Zap } from 'lucide-react'
import { useI18n } from '../lib/i18n'
import { useTheme, type Theme } from '../lib/theme'
import { CreditFooter } from './CreditFooter'

const FEATURES = [
  { icon: Cable, key: 'auth.feat.multivendor' },
  { icon: Zap, key: 'auth.feat.zerotouch' },
  { icon: Layers3, key: 'auth.feat.firmware' },
  { icon: Radio, key: 'auth.feat.realtime' },
  { icon: ShieldCheck, key: 'auth.feat.tenant' },
]

const THEME_CYCLE: Theme[] = ['system', 'light', 'dark']
const THEME_ICON = { system: Monitor, light: Sun, dark: Moon }

function Logo({ className = '' }: { className?: string }) {
  return (
    <div className={`flex h-10 w-10 items-center justify-center rounded-xl bg-indigo-600 text-white shadow-lg shadow-indigo-600/30 ${className}`}>
      <Radio className="h-5 w-5" strokeWidth={2.25} />
    </div>
  )
}

// AuthShell — kerangka halaman publik (Login, Lupa Password): panel brand di
// kiri (≥ lg) dan area form di kanan. Tidak memakai data tenant (belum login).
export function AuthShell({ children }: { children: ReactNode }) {
  const { t, lang, setLang } = useI18n()
  const { theme, setTheme } = useTheme()
  const ThemeIcon = THEME_ICON[theme]

  return (
    <div className="grid min-h-screen bg-white lg:grid-cols-[minmax(0,5fr)_minmax(0,6fr)] dark:bg-slate-950">
      {/* Panel brand */}
      <aside className="relative hidden overflow-hidden bg-slate-950 text-white lg:flex lg:flex-col lg:justify-between lg:p-12">
        <div aria-hidden className="pointer-events-none absolute inset-0">
          <div className="absolute -left-24 -top-24 h-96 w-96 rounded-full bg-indigo-600/30 blur-3xl" />
          <div className="absolute -bottom-32 right-0 h-[28rem] w-[28rem] rounded-full bg-violet-600/20 blur-3xl" />
          <svg className="absolute inset-0 h-full w-full opacity-[0.07]" xmlns="http://www.w3.org/2000/svg">
            <defs>
              <pattern id="grid" width="40" height="40" patternUnits="userSpaceOnUse">
                <path d="M 40 0 L 0 0 0 40" fill="none" stroke="white" strokeWidth="1" />
              </pattern>
            </defs>
            <rect width="100%" height="100%" fill="url(#grid)" />
          </svg>
        </div>

        <div className="relative flex items-center gap-3">
          <Logo />
          <div>
            <p className="text-base font-semibold leading-none">ACS Console</p>
            <p className="mt-1 text-xs leading-none text-slate-400">Multi-Vendor TR-069</p>
          </div>
        </div>

        <div className="relative max-w-md">
          <h2 className="text-3xl font-semibold leading-tight tracking-tight">{t('auth.brand.tagline')}</h2>
          <p className="mt-3 text-sm leading-relaxed text-slate-400">{t('auth.brand.sub')}</p>
          <ul className="mt-8 space-y-3.5">
            {FEATURES.map(({ icon: Icon, key }) => (
              <li key={key} className="flex items-start gap-3 text-sm text-slate-200">
                <span className="mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-lg bg-white/10 text-indigo-300 ring-1 ring-white/10">
                  <Icon className="h-3.5 w-3.5" strokeWidth={2} />
                </span>
                <span className="pt-0.5">{t(key)}</span>
              </li>
            ))}
          </ul>
        </div>

        <p className="relative flex items-start gap-2 text-xs leading-relaxed text-slate-500">
          <ShieldCheck className="mt-0.5 h-3.5 w-3.5 shrink-0" />
          {t('auth.secure')}
        </p>
      </aside>

      {/* Area form */}
      <main className="relative flex min-h-screen flex-col">
        <div className="flex items-center justify-between px-5 py-4 sm:px-8">
          <div className="flex items-center gap-2.5 lg:invisible">
            <Logo className="h-8 w-8 rounded-lg" />
            <span className="text-sm font-semibold text-slate-900 dark:text-slate-100">ACS Console</span>
          </div>
          <div className="flex items-center gap-1">
            <button
              onClick={() => setLang(lang === 'id' ? 'en' : 'id')}
              aria-label={t('top.language')}
              className="flex h-8 items-center gap-1 rounded-lg px-2 text-xs font-semibold uppercase text-slate-500 transition-colors hover:bg-slate-100 hover:text-slate-900 dark:text-slate-400 dark:hover:bg-slate-800 dark:hover:text-slate-100"
            >
              <Languages className="h-4 w-4" />
              {lang}
            </button>
            <button
              onClick={() => setTheme(THEME_CYCLE[(THEME_CYCLE.indexOf(theme) + 1) % THEME_CYCLE.length])}
              aria-label="Theme"
              className="flex h-8 w-8 items-center justify-center rounded-lg text-slate-500 transition-colors hover:bg-slate-100 hover:text-slate-900 dark:text-slate-400 dark:hover:bg-slate-800 dark:hover:text-slate-100"
            >
              <ThemeIcon className="h-4 w-4" />
            </button>
          </div>
        </div>

        <div className="flex flex-1 items-center justify-center px-5 pb-12 sm:px-8">
          <div className="w-full max-w-md motion-safe:animate-[authIn_0.35s_ease-out]">{children}</div>
        </div>
        <CreditFooter className="px-5 pb-6 text-center sm:px-8" />
      </main>
    </div>
  )
}
