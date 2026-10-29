export function formatRelativeTime(iso: string | null): string {
  if (!iso) return '-'
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return '-'

  const diffMs = Date.now() - date.getTime()
  const diffSec = Math.round(diffMs / 1000)

  if (diffSec < 5) return 'baru saja'
  if (diffSec < 60) return `${diffSec} dtk lalu`
  const diffMin = Math.round(diffSec / 60)
  if (diffMin < 60) return `${diffMin} mnt lalu`
  const diffHour = Math.round(diffMin / 60)
  if (diffHour < 24) return `${diffHour} jam lalu`
  const diffDay = Math.round(diffHour / 24)
  if (diffDay < 30) return `${diffDay} hari lalu`

  return date.toLocaleDateString('id-ID', { day: 'numeric', month: 'short', year: 'numeric' })
}

// formatDurationSeconds menulis durasi (detik) dalam dua satuan terbesar: "45 detik", "5 menit", "3 jam 20 menit", "2 hari 4 jam".
// Sama dengan penulisan diagnosis di backend (usecase/troubleshoot) supaya angka di kartu dan di teks temuan terbaca seragam.
export function formatDurationSeconds(totalSeconds: number | null | undefined): string {
  if (totalSeconds == null || !Number.isFinite(totalSeconds)) return '-'
  const sec = Math.max(0, Math.floor(totalSeconds))
  if (sec < 60) return `${sec} detik`
  const min = Math.floor(sec / 60)
  if (min < 60) return `${min} menit`
  const hours = Math.floor(sec / 3600)
  if (sec < 48 * 3600) {
    const m = min % 60
    return m === 0 ? `${hours} jam` : `${hours} jam ${m} menit`
  }
  const days = Math.floor(hours / 24)
  const h = hours % 24
  return h === 0 ? `${days} hari` : `${days} hari ${h} jam`
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

// Alias pendek untuk dipakai di halaman baru (WebhooksPage, dll.)
export const fmtDatetime = formatDateTime

