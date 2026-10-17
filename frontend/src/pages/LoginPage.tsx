import { useEffect, useRef, useState, type FormEvent, type KeyboardEvent } from 'react'
import { Link, Navigate, useLocation, useNavigate } from 'react-router-dom'
import { AlertCircle, ArrowRight, Eye, EyeOff, Info, KeyRound, Lock, Mail } from 'lucide-react'
import { useAuth } from '../lib/auth'
import { useI18n } from '../lib/i18n'
import { API_BASE, ApiError } from '../lib/api'
import { Spinner } from '../components/Spinner'
import { AuthShell } from '../components/AuthShell'
import { authInputCls, authPrimaryBtnCls } from '../components/authStyles'

const REMEMBER_KEY = 'acs_login_email'

function readRemembered(): string {
  try {
    return localStorage.getItem(REMEMBER_KEY) ?? ''
  } catch {
    return ''
  }
}

function writeRemembered(username: string | null) {
  try {
    if (username) localStorage.setItem(REMEMBER_KEY, username)
    else localStorage.removeItem(REMEMBER_KEY)
  } catch {
    // storage diblokir (mode privat) — fitur ingat username dilewati saja.
  }
}

// Tombol SSO tampil hanya bila backend melaporkan OIDC terkonfigurasi (GET /auth/config).
const SSO_URL = `${API_BASE}/auth/oidc/login`

export function LoginPage() {
  const { login, completeMfa, isAuthenticated } = useAuth()
  const { t } = useI18n()
  const navigate = useNavigate()
  const location = useLocation()
  const from = (location.state as { from?: string } | null)?.from

  const [ssoEnabled, setSsoEnabled] = useState(false)
  const [signupEnabled, setSignupEnabled] = useState(false)
  const ssoFailed = new URLSearchParams(location.search).get('sso') === 'error'
  useEffect(() => {
    let alive = true
    fetch(`${API_BASE}/auth/config`).then((r) => (r.ok ? r.json() : null)).then((c) => {
      if (!alive) return
      setSsoEnabled(Boolean(c?.sso_enabled))
      setSignupEnabled(Boolean(c?.self_signup_enabled))
    }).catch(() => undefined)
    return () => { alive = false }
  }, [])
  const [email, setEmail] = useState(readRemembered)
  const [remember, setRemember] = useState(() => readRemembered() !== '')
  const [password, setPassword] = useState('')
  const [showPassword, setShowPassword] = useState(false)
  const [capsLock, setCapsLock] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [isSubmitting, setIsSubmitting] = useState(false)
  // Langkah kedua MFA: token pendek dari backend (BUKAN sesi) + kode TOTP/pemulihan.
  const [mfaToken, setMfaToken] = useState<string | null>(null)
  const [mfaCode, setMfaCode] = useState('')
  const mfaRef = useRef<HTMLInputElement>(null)
  const emailRef = useRef<HTMLInputElement>(null)
  const passwordRef = useRef<HTMLInputElement>(null)

  // Email terisi dari "ingat saya" → langsung fokus ke password.
  useEffect(() => {
    if (email) passwordRef.current?.focus()
    else emailRef.current?.focus()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  function detectCaps(e: KeyboardEvent<HTMLInputElement>) {
    setCapsLock(e.getModifierState?.('CapsLock') ?? false)
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    if (isSubmitting) return
    setError(null)
    setIsSubmitting(true)
    const id = email.trim()
    try {
      const pending = await login(id, password)
      writeRemembered(remember ? id : null)
      if (pending) {
        setMfaToken(pending)
        setPassword('')
        setTimeout(() => mfaRef.current?.focus(), 0)
        return
      }
      navigate(from && from !== '/login' ? from : '/', { replace: true })
    } catch (err) {
      setPassword('')
      passwordRef.current?.focus()
      if (err instanceof ApiError) {
        if (err.status === 429) setError(t('login.errRate'))
        else if (err.message.includes('password salah')) setError(t('login.errCredentials'))
        // mis. "tenant tidak aktif, hubungi administrator" — tampilkan apa adanya.
        else setError(err.message.replace(/^auth:\s*/, '') || t('login.error'))
      } else {
        // fetch() melempar TypeError saat server tak terjangkau.
        setError(t('login.errNetwork'))
      }
    } finally {
      setIsSubmitting(false)
    }
  }

  async function handleMfaSubmit(e: FormEvent) {
    e.preventDefault()
    if (isSubmitting || !mfaToken) return
    setError(null)
    setIsSubmitting(true)
    try {
      await completeMfa(mfaToken, mfaCode.trim())
      navigate(from && from !== '/login' ? from : '/', { replace: true })
    } catch (err) {
      setMfaCode('')
      mfaRef.current?.focus()
      if (err instanceof ApiError) {
        if (err.status === 429) setError(t('login.errRate'))
        else if (err.message.includes('token tidak valid')) {
          // token tahap-2 kedaluwarsa (5 menit): ulangi dari awal
          setMfaToken(null)
          setError(t('login.mfaExpired'))
        } else if (err.message.includes('kode verifikasi salah')) setError(t('login.mfaWrong'))
        else setError(err.message.replace(/^auth:\s*/, '') || t('login.error'))
      } else {
        setError(t('login.errNetwork'))
      }
    } finally {
      setIsSubmitting(false)
    }
  }

  // Sudah login (mis. buka /login lewat bookmark) → tidak perlu form lagi.
  if (isAuthenticated && !isSubmitting) return <Navigate to={from && from !== '/login' ? from : '/'} replace />

  if (mfaToken) {
    return (
      <AuthShell>
        <div className="mb-8">
          <h1 className="text-2xl font-semibold tracking-tight text-slate-900 dark:text-slate-100">{t('login.mfaTitle')}</h1>
          <p className="mt-1.5 text-sm text-slate-500 dark:text-slate-400">{t('login.mfaDesc')}</p>
        </div>
        <form onSubmit={handleMfaSubmit} noValidate className="space-y-5">
          <div>
            <label htmlFor="mfa-code" className="mb-1.5 block text-sm font-medium text-slate-700 dark:text-slate-300">
              {t('login.mfaCode')}
            </label>
            <input
              id="mfa-code"
              ref={mfaRef}
              value={mfaCode}
              onChange={(e) => setMfaCode(e.target.value)}
              inputMode="text"
              autoComplete="one-time-code"
              autoCapitalize="none"
              spellCheck={false}
              placeholder="123456"
              minLength={6}
              maxLength={16}
              required
              aria-invalid={!!error}
              aria-describedby={error ? 'login-error' : undefined}
              className={`${authInputCls} text-center font-mono text-lg tracking-widest`}
            />
          </div>
          {error && (
            <div id="login-error" role="alert" className="flex items-start gap-2 rounded-lg bg-red-50 px-3.5 py-2.5 text-sm text-red-700 dark:bg-red-500/10 dark:text-red-400">
              <AlertCircle className="mt-0.5 h-4 w-4 shrink-0" strokeWidth={2} />
              <span>{error}</span>
            </div>
          )}
          <button type="submit" disabled={isSubmitting || mfaCode.trim().length < 6} className={authPrimaryBtnCls}>
            {isSubmitting ? (
              <>
                <Spinner className="h-4 w-4 text-white" /> {t('login.checking')}
              </>
            ) : (
              <>
                {t('login.mfaSubmit')} <ArrowRight className="h-4 w-4" />
              </>
            )}
          </button>
          <button
            type="button"
            onClick={() => { setMfaToken(null); setMfaCode(''); setError(null) }}
            className="w-full text-center text-sm font-medium text-indigo-600 hover:text-indigo-500 dark:text-indigo-400"
          >
            {t('login.mfaBack')}
          </button>
        </form>
      </AuthShell>
    )
  }

  return (
    <AuthShell>
      <div className="mb-8">
        <h1 className="text-2xl font-semibold tracking-tight text-slate-900 dark:text-slate-100">{t('login.welcome')}</h1>
        <p className="mt-1.5 text-sm text-slate-500 dark:text-slate-400">{t('login.welcomeSub')}</p>
      </div>

      {from && from !== '/login' && from !== '/' && (
        <div className="mb-5 flex items-start gap-2 rounded-lg bg-indigo-50 px-3.5 py-2.5 text-sm text-indigo-700 dark:bg-indigo-500/10 dark:text-indigo-300" role="status">
          <Info className="mt-0.5 h-4 w-4 shrink-0" />
          <span>{t('login.sessionNotice')}</span>
        </div>
      )}

      <form onSubmit={handleSubmit} noValidate className="space-y-5">
        <div>
          <label htmlFor="email" className="mb-1.5 block text-sm font-medium text-slate-700 dark:text-slate-300">
            {t('common.email')}
          </label>
          <div className="relative">
            <Mail className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-slate-500 dark:text-slate-400" />
            <input
              id="email"
              ref={emailRef}
              type="email"
              inputMode="email"
              autoComplete="username"
              autoCapitalize="none"
              spellCheck={false}
              placeholder={t('login.emailPh')}
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              required
              aria-invalid={!!error}
              className={`${authInputCls} pl-10`}
            />
          </div>
        </div>

        <div>
          <div className="mb-1.5 flex items-center justify-between">
            <label htmlFor="password" className="block text-sm font-medium text-slate-700 dark:text-slate-300">
              {t('common.password')}
            </label>
            <Link to="/forgot-password" className="text-xs font-medium text-indigo-600 hover:text-indigo-500 dark:text-indigo-400">
              {t('login.forgotLink')}
            </Link>
          </div>
          <div className="relative">
            <Lock className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-slate-500 dark:text-slate-400" />
            <input
              id="password"
              ref={passwordRef}
              type={showPassword ? 'text' : 'password'}
              autoComplete="current-password"
              placeholder={t('login.passwordPh')}
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              onKeyUp={detectCaps}
              onKeyDown={detectCaps}
              onBlur={() => setCapsLock(false)}
              required
              aria-invalid={!!error}
              aria-describedby={error ? 'login-error' : undefined}
              className={`${authInputCls} pl-10 pr-11`}
            />
            <button
              type="button"
              onClick={() => setShowPassword((v) => !v)}
              aria-label={showPassword ? t('login.hidePassword') : t('login.showPassword')}
              aria-pressed={showPassword}
              className="absolute right-2 top-1/2 -translate-y-1/2 rounded-md p-1.5 text-slate-500 transition-colors hover:text-slate-600 dark:hover:text-slate-300 dark:text-slate-400"
            >
              {showPassword ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
            </button>
          </div>
          {capsLock && (
            <p className="mt-1.5 flex items-center gap-1.5 text-xs font-medium text-amber-600 dark:text-amber-400" role="status">
              <KeyRound className="h-3.5 w-3.5" /> {t('login.capsLock')}
            </p>
          )}
        </div>

        <label className="flex cursor-pointer items-center gap-2 text-sm text-slate-600 dark:text-slate-400">
          <input
            type="checkbox"
            checked={remember}
            onChange={(e) => setRemember(e.target.checked)}
            className="h-4 w-4 rounded border-slate-300 text-indigo-600 focus:ring-indigo-500 dark:border-slate-600 dark:bg-slate-900"
          />
          {t('login.rememberEmail')}
        </label>

        {ssoFailed && !error && (
          <div role="alert" className="flex items-start gap-2 rounded-lg bg-red-50 px-3.5 py-2.5 text-sm text-red-700 dark:bg-red-500/10 dark:text-red-400">
            <AlertCircle className="mt-0.5 h-4 w-4 shrink-0" strokeWidth={2} />
            <span>Login SSO gagal atau akun Anda belum terdaftar. Hubungi admin atau masuk dengan email dan password.</span>
          </div>
        )}

        {error && (
          <div id="login-error" role="alert" className="flex items-start gap-2 rounded-lg bg-red-50 px-3.5 py-2.5 text-sm text-red-700 dark:bg-red-500/10 dark:text-red-400">
            <AlertCircle className="mt-0.5 h-4 w-4 shrink-0" strokeWidth={2} />
            <span>{error}</span>
          </div>
        )}

        <button type="submit" disabled={isSubmitting || !email.trim() || !password} className={authPrimaryBtnCls}>
          {isSubmitting ? (
            <>
              <Spinner className="h-4 w-4 text-white" /> {t('login.checking')}
            </>
          ) : (
            <>
              {t('login.submit')} <ArrowRight className="h-4 w-4" />
            </>
          )}
        </button>
      </form>

      {ssoEnabled && (
      <div className="relative my-6">
        <div className="absolute inset-0 flex items-center">
          <div className="w-full border-t border-slate-200 dark:border-slate-800" />
        </div>
        <div className="relative flex justify-center text-xs">
          <span className="bg-white px-3 text-slate-500 dark:bg-slate-950 dark:text-slate-400">{t('login.or')}</span>
        </div>
      </div>
      )}

      {ssoEnabled && (
      <>
      <a
        href={SSO_URL}
        className="flex w-full items-center justify-center gap-2 rounded-lg border border-slate-300 bg-white px-4 py-2.5 text-sm font-medium text-slate-700 transition hover:bg-slate-50 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-300 dark:hover:bg-slate-800"
      >
        <KeyRound className="h-4 w-4 text-slate-500 dark:text-slate-400" />
        {t('login.sso')}
      </a>
      <p className="mt-2 text-center text-xs text-slate-500 dark:text-slate-400">{t('login.ssoHint')}</p>
      </>
      )}

      {signupEnabled && (
        <p className="mt-6 text-center text-sm text-slate-600 dark:text-slate-400">
          Belum punya akun?{' '}
          <Link to="/signup" className="font-medium text-indigo-700 hover:text-indigo-600 dark:text-indigo-300">
            Daftar tenant baru
          </Link>
        </p>
      )}

      <p className="mt-8 text-center text-xs text-slate-500 dark:text-slate-400">
        {t('login.needHelp')}{' '}
        <Link to="/forgot-password" className="font-medium text-indigo-600 hover:text-indigo-500 dark:text-indigo-400">
          {t('login.forgotLink')}
        </Link>
      </p>
    </AuthShell>
  )
}
