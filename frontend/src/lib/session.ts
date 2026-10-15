import { useEffect, useState } from 'react'
import { getAuthToken } from './api'

// Masa berlaku sesi dibaca dari klaim `exp` JWT (hanya untuk TAMPILAN — validasi
// sebenarnya tetap di backend). Token yang bukan JWT / rusak → null.
export function tokenExpiryMs(token: string | null = getAuthToken()): number | null {
  if (!token) return null
  const part = token.split('.')[1]
  if (!part) return null
  try {
    const b64 = part.replace(/-/g, '+').replace(/_/g, '/')
    const json = decodeURIComponent(
      atob(b64.padEnd(Math.ceil(b64.length / 4) * 4, '='))
        .split('')
        .map((c) => '%' + c.charCodeAt(0).toString(16).padStart(2, '0'))
        .join(''),
    )
    const exp = (JSON.parse(json) as { exp?: unknown }).exp
    return typeof exp === 'number' ? exp * 1000 : null
  } catch {
    return null
  }
}

// useSessionRemaining — sisa waktu sesi (ms), diperbarui tiap 30 detik.
// null = tidak diketahui.
export function useSessionRemaining(): number | null {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const id = window.setInterval(() => setNow(Date.now()), 30_000)
    return () => window.clearInterval(id)
  }, [])
  const exp = tokenExpiryMs()
  return exp === null ? null : exp - now
}

// formatDuration — "2j 15m", "45m", "<1m".
export function formatDuration(ms: number): string {
  const totalMin = Math.floor(ms / 60_000)
  if (totalMin < 1) return '<1m'
  const d = Math.floor(totalMin / 1440)
  const h = Math.floor((totalMin % 1440) / 60)
  const m = totalMin % 60
  if (d > 0) return `${d}h ${h}j`
  if (h > 0) return `${h}j ${m}m`
  return `${m}m`
}
