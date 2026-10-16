import { useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { AuthShell } from '../components/AuthShell'
import { useAuth } from '../lib/auth'

// Tujuan redirect login SSO: backend mengarahkan ke /sso#token=<JWT>. Token dibaca dari
// FRAGMENT (tidak ikut request/log/Referer), dibuang dari URL, lalu ditukar profil lewat /auth/me.
export function SsoCallbackPage() {
  const { loginWithToken } = useAuth()
  const navigate = useNavigate()
  const [error, setError] = useState<string | null>(null)
  const started = useRef(false)

  useEffect(() => {
    if (started.current) return
    started.current = true
    const token = new URLSearchParams(window.location.hash.replace(/^#/, '')).get('token')
    window.history.replaceState(null, '', '/sso') // hapus token dari URL & riwayat
    if (!token) {
      navigate('/login?sso=error', { replace: true })
      return
    }
    loginWithToken(token)
      .then(() => navigate('/', { replace: true }))
      .catch(() => setError('Login SSO gagal. Silakan coba lagi atau masuk dengan email dan password.'))
  }, [loginWithToken, navigate])

  return (
    <AuthShell>
      <h1 className="mb-4 text-xl font-bold text-slate-900 dark:text-slate-100">Masuk dengan SSO</h1>
      {error ? (
        <div role="alert" className="space-y-3">
          <p className="text-sm text-red-600">{error}</p>
          <a href="/login" className="text-sm font-medium text-indigo-600 hover:text-indigo-500">Kembali ke halaman login</a>
        </div>
      ) : (
        <p role="status" className="text-sm text-slate-500 dark:text-slate-400">Mohon tunggu…</p>
      )}
    </AuthShell>
  )
}
