// Galat pemuatan data (query) dilaporkan ke satu tempat. Sebelumnya hanya 2 halaman yang memeriksa
// isError, sehingga daftar yang gagal dimuat tampil seperti "belum ada data" — menyesatkan operator.
// QueryCache.onError (main.tsx) memanggil reportQueryError; QueryErrorToaster menampilkannya sebagai
// toast. Dedupe per query key supaya polling/refetch tidak membanjiri layar.
import { ApiError } from './api'

type Listener = (message: string) => void

const listeners = new Set<Listener>()
const lastShown = new Map<string, number>()
const DEDUPE_MS = 15_000

export function subscribeQueryErrors(fn: Listener): () => void {
  listeners.add(fn)
  return () => {
    listeners.delete(fn)
  }
}

export function describeQueryError(err: unknown): string | null {
  if (err instanceof ApiError) {
    if (err.status === 401) return null // sesi berakhir: AuthProvider yang menangani (logout otomatis)
    if (err.status >= 500) return `Server bermasalah (HTTP ${err.status}). Coba lagi sebentar.`
    if (err.status === 403) return 'Anda tidak punya akses ke data ini.'
    return err.message
  }
  if (err instanceof DOMException && err.name === 'AbortError') return null
  return 'Tidak dapat menjangkau server. Periksa koneksi Anda lalu muat ulang.'
}

export function reportQueryError(err: unknown, queryKey: readonly unknown[]) {
  const message = describeQueryError(err)
  if (!message) return
  const id = JSON.stringify(queryKey)
  const now = Date.now()
  if (now - (lastShown.get(id) ?? 0) < DEDUPE_MS) return
  lastShown.set(id, now)
  listeners.forEach((l) => l(message))
}
