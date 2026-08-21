export function formatRelativeTime(iso: string | null): string {
  if (!iso) return '-'
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return '-'

  const diffMs = Date.now() - date.getTime()
  const diffSec = Math.round(diffMs / 1000)

  if (diffSec < 5) return 'baru saja'
  if (diffSec < 60) return `${diffSec}d lalu`
  const diffMin = Math.round(diffSec / 60)
  if (diffMin < 60) return `${diffMin}mnt lalu`
  const diffHour = Math.round(diffMin / 60)
  if (diffHour < 24) return `${diffHour}j lalu`
  const diffDay = Math.round(diffHour / 24)
  if (diffDay < 30) return `${diffDay}h lalu`

  return date.toLocaleDateString('id-ID', { day: 'numeric', month: 'short', year: 'numeric' })
}

// decodeBytesField membaca field Go []byte (di-encode base64 oleh
// encoding/json) seperti Task.parameters/response atau DeviceDiagnostic.result,
// lalu coba pretty-print sebagai JSON. Kembalikan null bila field kosong.
export function decodeBytesField(value: string | null): string | null {
  if (!value) return null
  try {
    const decoded = atob(value)
    try {
      return JSON.stringify(JSON.parse(decoded), null, 2)
    } catch {
      return decoded
    }
  } catch {
    return value
  }
}

export function formatDateTime(iso: string | null): string {
  if (!iso) return '-'
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return '-'
  return date.toLocaleString('id-ID', {
    day: '2-digit',
    month: 'short',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}
