import { createContext, useContext, useEffect, useState, type ReactNode } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { api, getAuthToken, setAuthToken, setUnauthorizedHandler } from './api'
import type { AuthUser } from './types'

interface LoginResponse {
  access_token: string
  token_type: string
  user: AuthUser
}

/** Password benar tetapi akun ber-MFA: login diselesaikan lewat completeMfa. */
interface MfaRequiredResponse {
  mfa_required: true
  mfa_token: string
}

interface AuthContextValue {
  user: AuthUser | null
  isAuthenticated: boolean
  isLoading: boolean
  hasRole: (...roles: string[]) => boolean
  /**
   * identifier = email (berisi "@") atau username lama. Mengembalikan mfa_token bila akun
   * butuh langkah kedua (lalu panggil completeMfa), atau null bila sudah login.
   */
  login: (identifier: string, password: string) => Promise<string | null>
  /** Langkah kedua login MFA: kode TOTP 6 digit atau kode pemulihan. */
  completeMfa: (mfaToken: string, code: string) => Promise<void>
  /** Login SSO: token dari redirect backend; profil diambil lewat GET /auth/me. */
  loginWithToken: (token: string) => Promise<void>
  logout: () => void
  /** SUPERADMIN: "masuk sebagai" pengguna tenant (token pendek, teraudit). */
  impersonate: (userId: number) => Promise<void>
  /** Keluar dari impersonate dan kembali ke sesi SUPERADMIN asli. */
  stopImpersonation: () => Promise<void>
}

const AuthContext = createContext<AuthContextValue | null>(null)
const USER_STORAGE_KEY = 'acs_user'
// Sesi SUPERADMIN asli selama impersonate; sessionStorage (per-tab) agar tidak bocor ke tab lain / sesi berikutnya.
const IMPERSONATION_ORIGIN_KEY = 'acs_imp_origin'

export function AuthProvider({ children }: { children: ReactNode }) {
  const queryClient = useQueryClient()
  const [user, setUser] = useState<AuthUser | null>(null)
  const [isLoading, setIsLoading] = useState(true)

  useEffect(() => {
    const raw = localStorage.getItem(USER_STORAGE_KEY)
    if (raw && getAuthToken()) {
      try {
        setUser(JSON.parse(raw) as AuthUser)
      } catch {
        localStorage.removeItem(USER_STORAGE_KEY)
      }
    }
    setIsLoading(false)

    setUnauthorizedHandler(() => {
      setAuthToken(null)
      localStorage.removeItem(USER_STORAGE_KEY)
      sessionStorage.removeItem(IMPERSONATION_ORIGIN_KEY)
      queryClient.clear() // jangan sisakan data pengguna sebelumnya di cache
      setUser(null)
    })
    return () => setUnauthorizedHandler(null)
  }, [queryClient])

  function startSession(resp: LoginResponse) {
    queryClient.clear() // cache milik sesi/pengguna sebelumnya tidak boleh terlihat
    setAuthToken(resp.access_token)
    localStorage.setItem(USER_STORAGE_KEY, JSON.stringify(resp.user))
    setUser(resp.user)
  }

  async function login(identifier: string, password: string): Promise<string | null> {
    const body = identifier.includes('@') ? { email: identifier, password } : { username: identifier, password }
    const resp = await api.post<LoginResponse | MfaRequiredResponse>('/auth/login', body)
    if ('mfa_required' in resp) return resp.mfa_token
    startSession(resp)
    return null
  }

  async function completeMfa(mfaToken: string, code: string) {
    startSession(await api.post<LoginResponse>('/auth/mfa/verify', { mfa_token: mfaToken, code }))
  }

  async function loginWithToken(token: string) {
    queryClient.clear()
    setAuthToken(token)
    try {
      const me = await api.get<AuthUser>('/auth/me')
      localStorage.setItem(USER_STORAGE_KEY, JSON.stringify(me))
      setUser(me)
    } catch (e) {
      setAuthToken(null)
      throw e
    }
  }

  async function impersonate(userId: number) {
    const resp = await api.post<LoginResponse & { impersonator_id: number }>(`/users/${userId}/impersonate`, {})
    sessionStorage.setItem(IMPERSONATION_ORIGIN_KEY, JSON.stringify({ token: getAuthToken(), user }))
    queryClient.clear()
    setAuthToken(resp.access_token)
    const impersonated = { ...resp.user, impersonator_id: resp.impersonator_id }
    localStorage.setItem(USER_STORAGE_KEY, JSON.stringify(impersonated))
    setUser(impersonated)
  }

  async function stopImpersonation() {
    try {
      await api.post('/auth/impersonation/end', {}) // catat akhir sesi di audit trail (best-effort)
    } catch {
      // token impersonate mungkin sudah kedaluwarsa — tetap pulihkan sesi asli
    }
    const raw = sessionStorage.getItem(IMPERSONATION_ORIGIN_KEY)
    sessionStorage.removeItem(IMPERSONATION_ORIGIN_KEY)
    queryClient.clear()
    try {
      const origin = raw ? (JSON.parse(raw) as { token: string | null; user: AuthUser | null }) : null
      if (origin?.token && origin.user) {
        setAuthToken(origin.token)
        localStorage.setItem(USER_STORAGE_KEY, JSON.stringify(origin.user))
        setUser(origin.user)
        return
      }
    } catch {
      // data asal rusak -> keluar bersih
    }
    logout()
  }

  function logout() {
    setAuthToken(null)
    sessionStorage.removeItem(IMPERSONATION_ORIGIN_KEY)
    localStorage.removeItem(USER_STORAGE_KEY)
    queryClient.clear()
    setUser(null)
  }

  function hasRole(...roles: string[]) {
    if (!user) return false
    return user.roles.includes('SUPERADMIN') || roles.some((r) => user.roles.includes(r))
  }

  return (
    <AuthContext.Provider value={{ user, isAuthenticated: !!user, isLoading, hasRole, login, completeMfa, loginWithToken, logout, impersonate, stopImpersonation }}>
      {children}
    </AuthContext.Provider>
  )
}

export function useAuth() {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error('useAuth harus dipakai di dalam AuthProvider')
  return ctx
}
