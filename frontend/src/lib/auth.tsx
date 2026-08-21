import { createContext, useContext, useEffect, useState, type ReactNode } from 'react'
import { api, getAuthToken, setAuthToken, setUnauthorizedHandler } from './api'
import type { AuthUser } from './types'

interface LoginResponse {
  access_token: string
  token_type: string
  user: AuthUser
}

interface AuthContextValue {
  user: AuthUser | null
  isAuthenticated: boolean
  isLoading: boolean
  hasRole: (...roles: string[]) => boolean
  login: (username: string, password: string) => Promise<void>
  logout: () => void
}

const AuthContext = createContext<AuthContextValue | null>(null)
const USER_STORAGE_KEY = 'acs_user'

export function AuthProvider({ children }: { children: ReactNode }) {
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
      setUser(null)
    })
    return () => setUnauthorizedHandler(null)
  }, [])

  async function login(username: string, password: string) {
    const resp = await api.post<LoginResponse>('/auth/login', { username, password })
    setAuthToken(resp.access_token)
    localStorage.setItem(USER_STORAGE_KEY, JSON.stringify(resp.user))
    setUser(resp.user)
  }

  function logout() {
    setAuthToken(null)
    localStorage.removeItem(USER_STORAGE_KEY)
    setUser(null)
  }

  function hasRole(...roles: string[]) {
    if (!user) return false
    return user.roles.includes('SUPERADMIN') || roles.some((r) => user.roles.includes(r))
  }

  return (
    <AuthContext.Provider value={{ user, isAuthenticated: !!user, isLoading, hasRole, login, logout }}>
      {children}
    </AuthContext.Provider>
  )
}

export function useAuth() {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error('useAuth harus dipakai di dalam AuthProvider')
  return ctx
}
