import { useEffect, useRef, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { AlertCircle, CheckCircle2 } from 'lucide-react'
import { ApiError } from '../lib/api'
import { useVerifySignup } from '../lib/hooks'
import { AuthShell } from '../components/AuthShell'
import { Spinner } from '../components/Spinner'
import { authPrimaryBtnCls } from '../components/authStyles'

// Halaman tujuan tautan di email: /verify-signup#token=... (token di FRAGMENT, tidak ikut ke server/log/Referer).
// Token sekali pakai, dikirim otomatis SEKALI (ref menjaga dari panggilan ganda StrictMode) lalu fragmen dihapus.
export function VerifySignupPage() {
  const mut = useVerifySignup()
  const navigate = useNavigate()
  const started = useRef(false)
  const [state, setState] = useState<'loading' | 'ok' | 'error'>('loading')
  const [tenant, setTenant] = useState<{ tenant_name: string; tenant_code: string; email: string } | null>(null)
  const [message, setMessage] = useState('')

  useEffect(() => {
    if (started.current) return
    started.current = true
    const token = new URLSearchParams(window.location.hash.replace(/^#/, '')).get('token') ?? ''
    window.history.replaceState(null, '', window.location.pathname) // buang token dari URL/riwayat
    if (!token) {
      setState('error')
      setMessage('Tautan verifikasi tidak lengkap. Buka tautan dari email Anda secara utuh.')
      return
    }
    mut.mutateAsync(token).then(
      (res) => {
        setTenant(res)
        setState('ok')
      },
      (err) => {
        setState('error')
        setMessage(err instanceof ApiError ? err.message.replace(/^signup:\s*/, '') : 'Tidak dapat menjangkau server. Coba muat ulang halaman.')
      },
    )
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  return (
    <AuthShell>
      {state === 'loading' && (
        <div role="status" className="flex items-center gap-3 text-sm text-slate-700 dark:text-slate-300">
          <Spinner className="h-5 w-5 text-indigo-600" /> Memverifikasi pendaftaran Anda…
        </div>
      )}
      {state === 'ok' && tenant && (
        <div>
          <div className="flex h-14 w-14 items-center justify-center rounded-full bg-emerald-50 text-emerald-600 dark:bg-emerald-500/10 dark:text-emerald-400">
            <CheckCircle2 className="h-7 w-7" />
          </div>
          <h1 className="mt-5 text-2xl font-semibold tracking-tight text-slate-900 dark:text-slate-100">Akun aktif</h1>
          <p className="mt-2 text-sm leading-relaxed text-slate-600 dark:text-slate-400" role="status">
            Tenant <strong>{tenant.tenant_name}</strong> berhasil dibuat. Masuk dengan <strong>{tenant.email}</strong> dan password yang Anda daftarkan.
            Langkah berikutnya: <em>Administrasi → Wizard Onboarding</em> untuk mengatur kredensial CWMP dan mendaftarkan perangkat pertama.
          </p>
          <button onClick={() => navigate('/login', { replace: true })} className={`${authPrimaryBtnCls} mt-6`}>
            Masuk sekarang
          </button>
        </div>
      )}
      {state === 'error' && (
        <div>
          <div role="alert" className="flex items-start gap-2 rounded-lg bg-red-50 px-3.5 py-3 text-sm text-red-700 dark:bg-red-500/10 dark:text-red-400">
            <AlertCircle className="mt-0.5 h-4 w-4 shrink-0" />
            <span>{message}</span>
          </div>
          <p className="mt-4 text-sm text-slate-600 dark:text-slate-400">
            Tautan berlaku 24 jam dan hanya sekali pakai. <Link to="/signup" className="font-medium text-indigo-700 underline dark:text-indigo-300">Daftar ulang</Link> atau{' '}
            <Link to="/login" className="font-medium text-indigo-700 underline dark:text-indigo-300">masuk</Link> bila akun sudah aktif.
          </p>
        </div>
      )}
    </AuthShell>
  )
}
