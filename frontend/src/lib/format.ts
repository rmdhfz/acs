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

// formatJSONField pretty-print field JSON mentah dari backend seperti
// Task.parameters/response atau DeviceDiagnostic.result (domain.Task/
// DeviceDiagnostic Go: json.RawMessage, bukan lagi []byte biasa -- browser
// sudah otomatis mem-parsingnya jadi objek JS lewat fetch/response.json(),
// TIDAK ada lagi base64 yang perlu di-decode manual di sini seperti
// sebelumnya). Kembalikan null bila field kosong/null.
export function formatJSONField(value: unknown): string | null {
  if (value === null || value === undefined) return null
  if (typeof value === 'string') return value
  try {
    return JSON.stringify(value, null, 2)
  } catch {
    return String(value)
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
