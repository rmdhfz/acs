import { useEffect, useState, type FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { AlertCircle, ArrowLeft, Clock3, KeyRound, LifeBuoy, Mail, MailCheck } from 'lucide-react'
import { useI18n } from '../lib/i18n'
import { ApiError } from '../lib/api'
import { useForgotPassword } from '../lib/hooks'
import { AuthShell } from '../components/AuthShell'
import { Spinner } from '../components/Spinner'
import { authInputCls, authPrimaryBtnCls } from '../components/authStyles'

const RESEND_SECONDS = 60

// Lupa password lewat EMAIL: backend mengirim tautan sekali-pakai (berlaku
// 30 menit). Respons backend SERAGAM baik email terdaftar atau tidak (anti
// enumerasi akun), jadi layar "terkirim" sengaja berbunyi "jika terdaftar".
export function ForgotPasswordPage() {
  const { t } = useI18n()
  const mut = useForgotPassword()
  const [email, setEmail] = useState('')
  const [sentTo, setSentTo] = useState<string | null>(null)
  const [cooldown, setCooldown] = useState(0)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (cooldown <= 0) return
    const id = window.setTimeout(() => setCooldown((c) => c - 1), 1000)
    return () => window.clearTimeout(id)
  }, [cooldown])

  async function send(address: string) {
    setError(null)
    try {
      await mut.mutateAsync(address)
      setSentTo(address)
      setCooldown(RESEND_SECONDS)
    } catch (err) {
      if (err instanceof ApiError && err.status === 429) setError(t('forgot.rate'))
      else if (err instanceof ApiError && err.status === 503) setError(t('forgot.disabled'))
      else setError(err instanceof ApiError ? err.message : t('login.errNetwork'))
    }
  }

  function handleSubmit(e: FormEvent) {
    e.preventDefault()
    const a = email.trim()
    if (a && !mut.isPending) void send(a)
  }

  return (
    <AuthShell>
      <Link to="/login" className="mb-6 inline-flex items-center gap-1.5 text-sm font-medium text-slate-500 transition-colors hover:text-slate-900 dark:text-slate-400 dark:hover:text-slate-100">
        <ArrowLeft className="h-4 w-4" /> {t('forgot.back')}
      </Link>

      {sentTo ? (
        <div>
          <div className="flex h-14 w-14 items-center justify-center rounded-full bg-emerald-50 text-emerald-600 dark:bg-emerald-500/10 dark:text-emerald-400">
            <MailCheck className="h-7 w-7" />
          </div>
          <h1 className="mt-5 text-2xl font-semibold tracking-tight text-slate-900 dark:text-slate-100">{t('forgot.sentTitle')}</h1>
          <p className="mt-2 text-sm leading-relaxed text-slate-500 dark:text-slate-400" role="status">
            {t('forgot.sentDesc', { email: sentTo })}
          </p>
          <p className="mt-3 text-sm text-slate-500 dark:text-slate-400">{t('forgot.sentSpam')}</p>

          {error && (
            <div role="alert" className="mt-4 flex items-start gap-2 rounded-lg bg-red-50 px-3.5 py-2.5 text-sm text-red-700 dark:bg-red-500/10 dark:text-red-400">
              <AlertCircle className="mt-0.5 h-4 w-4 shrink-0" />
              <span>{error}</span>
            </div>
          )}

          <div className="mt-6 flex flex-wrap items-center gap-3">
            <button
              onClick={() => void send(sentTo)}
              disabled={cooldown > 0 || mut.isPending}
              className="rounded-lg border border-slate-300 px-4 py-2 text-sm font-medium text-slate-700 transition hover:bg-slate-50 disabled:cursor-not-allowed disabled:opacity-60 dark:border-slate-700 dark:text-slate-300 dark:hover:bg-slate-800"
            >
              {cooldown > 0 ? t('forgot.resendIn', { s: cooldown }) : t('forgot.resend')}
            </button>
            <button
              onClick={() => {
                setSentTo(null)
                setError(null)
              }}
              className="text-sm font-medium text-indigo-600 hover:text-indigo-500 dark:text-indigo-400"
            >
              {t('forgot.otherEmail')}
            </button>
          </div>
        </div>
      ) : (
        <>
          <div className="mb-6 flex items-start gap-3">
            <div className="flex h-11 w-11 shrink-0 items-center justify-center rounded-xl bg-indigo-50 text-indigo-600 dark:bg-indigo-500/10 dark:text-indigo-400">
              <LifeBuoy className="h-5 w-5" />
            </div>
            <div>
              <h1 className="text-2xl font-semibold tracking-tight text-slate-900 dark:text-slate-100">{t('forgot.title')}</h1>
              <p className="mt-1 text-sm text-slate-500 dark:text-slate-400">{t('forgot.subtitle')}</p>
            </div>
          </div>

          <form onSubmit={handleSubmit} noValidate className="space-y-5">
            <div>
              <label htmlFor="fp-email" className="mb-1.5 block text-sm font-medium text-slate-700 dark:text-slate-300">
                {t('forgot.emailLabel')}
              </label>
              <div className="relative">
                <Mail className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-slate-500 dark:text-slate-400" />
                <input
                  id="fp-email"
                  type="email"
                  inputMode="email"
                  autoComplete="email"
                  autoCapitalize="none"
                  spellCheck={false}
                  autoFocus
                  placeholder={t('login.emailPh')}
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  className={`${authInputCls} pl-10`}
                />
              </div>
            </div>

            {error && (
              <div role="alert" className="flex items-start gap-2 rounded-lg bg-red-50 px-3.5 py-2.5 text-sm text-red-700 dark:bg-red-500/10 dark:text-red-400">
                <AlertCircle className="mt-0.5 h-4 w-4 shrink-0" />
                <span>{error}</span>
              </div>
            )}

            <button type="submit" disabled={mut.isPending || !email.trim()} className={authPrimaryBtnCls}>
              {mut.isPending ? (
                <>
                  <Spinner className="h-4 w-4 text-white" /> {t('forgot.sending')}
                </>
              ) : (
                t('forgot.send')
              )}
            </button>
          </form>

          <div className="mt-8 space-y-3">
            <div className="flex gap-3 rounded-lg border border-slate-200 p-3.5 dark:border-slate-800">
              <Clock3 className="mt-0.5 h-4 w-4 shrink-0 text-amber-500" />
              <div>
                <p className="text-sm font-medium text-slate-800 dark:text-slate-200">{t('forgot.locked')}</p>
                <p className="mt-0.5 text-xs text-slate-500 dark:text-slate-400">{t('forgot.lockedDesc')}</p>
              </div>
            </div>
            <div className="flex gap-3 rounded-lg border border-slate-200 p-3.5 dark:border-slate-800">
              <KeyRound className="mt-0.5 h-4 w-4 shrink-0 text-indigo-500" />
              <div>
                <p className="text-sm font-medium text-slate-800 dark:text-slate-200">{t('forgot.sso')}</p>
                <p className="mt-0.5 text-xs text-slate-500 dark:text-slate-400">{t('forgot.ssoDesc')}</p>
              </div>
            </div>
          </div>
        </>
      )}
    </AuthShell>
  )
}
