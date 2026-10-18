import { useEffect, useState, type FormEvent } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { AlertCircle, CheckCircle2, Eye, EyeOff, Lock, ShieldAlert } from 'lucide-react'
import { useI18n } from '../lib/i18n'
import { ApiError } from '../lib/api'
import { useResetPassword } from '../lib/hooks'
import { scorePassword } from '../lib/password'
import { AuthShell } from '../components/AuthShell'
import { PasswordStrengthBar } from '../components/PasswordStrengthBar'
import { Spinner } from '../components/Spinner'
import { authInputCls, authPrimaryBtnCls } from '../components/authStyles'

const MIN_LEN = 8

// Token reset ada di FRAGMENT URL (#token=…) — tidak dikirim ke server dan tidak
// ikut header Referer. Dibaca sekali lalu dihapus dari address bar/history.
function readTokenFromHash(): string {
  const m = /[#&]token=([A-Za-z0-9_-]+)/.exec(window.location.hash)
  return m ? m[1] : ''
}

export function ResetPasswordPage() {
  const { t } = useI18n()
  const navigate = useNavigate()
  const mut = useResetPassword()
  const [token] = useState(readTokenFromHash)
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [show, setShow] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [done, setDone] = useState(false)
  const [invalid, setInvalid] = useState(token === '')

  useEffect(() => {
    if (window.location.hash) window.history.replaceState(null, '', window.location.pathname)
  }, [])

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    if (password.length < MIN_LEN) return setError(t('reset.tooShort'))
    if (password !== confirm) return setError(t('reset.mismatch'))
    if (scorePassword(password).score < 2) return setError(t('reset.weak'))
    try {
      await mut.mutateAsync({ token, new_password: password })
      setDone(true)
    } catch (err) {
      if (err instanceof ApiError && err.status === 400 && /tautan|kedaluwarsa|token/i.test(err.message)) setInvalid(true)
      else setError(err instanceof ApiError ? err.message : t('login.errNetwork'))
    }
  }

  if (done) {
    return (
      <AuthShell>
        <div className="text-center">
          <div className="mx-auto mx-auto flex h-14 w-14 items-center justify-center rounded-full bg-emerald-50 text-emerald-600 dark:bg-emerald-500/10 dark:text-emerald-400">
            <CheckCircle2 className="h-7 w-7" />
          </div>
          <h1 className="mt-5 text-[1.75rem] font-normal leading-tight text-center text-slate-900 dark:text-slate-100">{t('reset.successTitle')}</h1>
          <p className="mt-2 text-sm text-slate-500 dark:text-slate-400">{t('reset.successDesc')}</p>
          <button onClick={() => navigate('/login', { replace: true })} className={`${authPrimaryBtnCls} mt-7`}>
            {t('reset.goLogin')}
          </button>
        </div>
      </AuthShell>
    )
  }

  if (invalid) {
    return (
      <AuthShell>
        <div className="text-center">
          <div className="mx-auto mx-auto flex h-14 w-14 items-center justify-center rounded-full bg-amber-50 text-amber-600 dark:bg-amber-500/10 dark:text-amber-400">
            <ShieldAlert className="h-7 w-7" />
          </div>
          <h1 className="mt-5 text-[1.75rem] font-normal leading-tight text-center text-slate-900 dark:text-slate-100">{t('reset.invalidTitle')}</h1>
          <p className="mt-2 text-sm text-slate-500 dark:text-slate-400">{t('reset.invalidDesc')}</p>
          <Link to="/forgot-password" className={`${authPrimaryBtnCls} mt-7`}>
            {t('reset.requestNew')}
          </Link>
          <Link to="/login" className="mt-4 inline-block text-sm font-medium text-slate-500 hover:text-slate-900 dark:text-slate-400 dark:hover:text-slate-100">
            {t('forgot.back')}
          </Link>
        </div>
      </AuthShell>
    )
  }

  return (
    <AuthShell>
      <div className="mb-8">
        <h1 className="text-[1.75rem] font-normal leading-tight text-center text-slate-900 dark:text-slate-100">{t('reset.title')}</h1>
        <p className="mt-1.5 text-sm text-slate-500 dark:text-slate-400">{t('reset.subtitle')}</p>
      </div>

      <form onSubmit={handleSubmit} noValidate className="space-y-5">
        <div>
          <label htmlFor="new-password" className="mb-1.5 block text-sm font-medium text-slate-700 dark:text-slate-300">
            {t('reset.newPassword')}
          </label>
          <div className="relative">
            <Lock className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-slate-500 dark:text-slate-400" />
            <input minLength={8}
              id="new-password"
              type={show ? 'text' : 'password'}
              autoComplete="new-password"
              autoFocus
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              className={`${authInputCls} pl-10 pr-11`}
            />
            <button
              type="button"
              onClick={() => setShow((v) => !v)}
              aria-label={show ? t('login.hidePassword') : t('login.showPassword')}
              aria-pressed={show}
              className="absolute right-2 top-1/2 -translate-y-1/2 rounded-md p-1.5 text-slate-500 transition-colors hover:text-slate-600 dark:hover:text-slate-300 dark:text-slate-400"
            >
              {show ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
            </button>
          </div>
          {password && <PasswordStrengthBar password={password} />}
        </div>

        <div>
          <label htmlFor="confirm-password" className="mb-1.5 block text-sm font-medium text-slate-700 dark:text-slate-300">
            {t('reset.confirm')}
          </label>
          <div className="relative">
            <Lock className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-slate-500 dark:text-slate-400" />
            <input minLength={8}
              id="confirm-password"
              type={show ? 'text' : 'password'}
              autoComplete="new-password"
              value={confirm}
              onChange={(e) => setConfirm(e.target.value)}
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

        <button type="submit" disabled={mut.isPending || !password || !confirm} className={authPrimaryBtnCls}>
          {mut.isPending ? (
            <>
              <Spinner className="h-4 w-4 text-white" /> {t('reset.saving')}
            </>
          ) : (
            t('reset.submit')
          )}
        </button>
      </form>
    </AuthShell>
  )
}
