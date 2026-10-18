import type { ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { Languages, Monitor, Moon, Radio, Sun } from 'lucide-react'
import { useI18n } from '../lib/i18n'
import { useTheme, type Theme } from '../lib/theme'
import { CreditFooter } from './CreditFooter'

const THEME_CYCLE: Theme[] = ['system', 'light', 'dark']
const THEME_ICON = { system: Monitor, light: Sun, dark: Moon }

// AuthShell — kerangka halaman publik (Login, Lupa/Reset Password, Daftar) bergaya Google: satu kartu putih
// bersudut besar di tengah, logo di atas, tautan bantuan/bahasa di bawah kartu. Tidak memakai data tenant (belum login).
export function AuthShell({ children }: { children: ReactNode }) {
  const { t, lang, setLang } = useI18n()
  const { theme, setTheme } = useTheme()
  const ThemeIcon = THEME_ICON[theme]

  return (
    <div className="flex min-h-screen flex-col items-center bg-white sm:justify-center sm:bg-slate-50 sm:px-6 sm:py-8 dark:bg-slate-950">
      <main className="w-full max-w-[29rem] flex-1 px-6 pb-8 pt-10 sm:flex-none sm:rounded-[28px] sm:border sm:border-slate-300 sm:bg-white sm:p-11 dark:sm:border-slate-700 dark:sm:bg-slate-900">
        <div className="mb-5 flex flex-col items-center text-center">
          <Link to="/" aria-label="ACS Console — beranda" className="rounded-xl focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-indigo-600">
            <span className="flex h-12 w-12 items-center justify-center rounded-2xl bg-indigo-600 text-white">
              <Radio className="h-6 w-6" strokeWidth={1.75} aria-hidden="true" />
            </span>
          </Link>
        </div>
        <div className="motion-safe:animate-[authIn_0.3s_ease-out] [&_h1+p]:mt-2 [&_h1+p]:text-center">{children}</div>
      </main>

      <footer className="flex w-full max-w-[29rem] flex-wrap items-center justify-between gap-2 px-6 pb-4 pt-5 text-xs text-slate-600 sm:px-2 dark:text-slate-400">
        <div className="flex items-center gap-1">
          <button
            onClick={() => setLang(lang === 'id' ? 'en' : 'id')}
            aria-label={t('top.language')}
            className="flex h-9 items-center gap-1.5 rounded-full px-3 font-medium uppercase transition-colors hover:bg-slate-100 dark:hover:bg-slate-800"
          >
            <Languages className="h-4 w-4" aria-hidden="true" />
            {lang}
          </button>
          <button
            onClick={() => setTheme(THEME_CYCLE[(THEME_CYCLE.indexOf(theme) + 1) % THEME_CYCLE.length])}
            aria-label="Theme"
            className="flex h-9 w-9 items-center justify-center rounded-full transition-colors hover:bg-slate-100 dark:hover:bg-slate-800"
          >
            <ThemeIcon className="h-4 w-4" aria-hidden="true" />
          </button>
        </div>
        <nav aria-label="Tautan hukum" className="flex items-center gap-4">
          <Link to="/privacy" className="rounded px-1 py-1 hover:underline">
            Privasi
          </Link>
          <Link to="/terms" className="rounded px-1 py-1 hover:underline">
            Persyaratan
          </Link>
        </nav>
      </footer>
      <CreditFooter className="px-5 pb-6 text-center" />
    </div>
  )
}
