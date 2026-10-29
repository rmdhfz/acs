import { useQueryClient, type Query } from '@tanstack/react-query'
import { AlertTriangle, RefreshCw } from 'lucide-react'
import { useEffect, useState } from 'react'
import { describeQueryError } from '../lib/queryErrors'

// Query yang sedang dipakai halaman (ada observer) dan berstatus galat. Query tanpa observer (halaman lain yang sudah
// ditinggalkan) tidak dihitung.
function failedActive(q: Query): boolean {
  return q.state.status === 'error' && q.getObserversCount() > 0
}

// Status galat INLINE untuk seluruh halaman: berbeda dari toast (hilang sendiri), banner ini tetap ada selama data
// halaman gagal dimuat — daftar yang gagal tidak lagi tampak seperti "belum ada data". Dipasang sekali di Layout.
export function QueryErrorBanner() {
  const qc = useQueryClient()
  const [failed, setFailed] = useState<Query[]>([])
  const [retrying, setRetrying] = useState(false)

  useEffect(() => {
    const cache = qc.getQueryCache()
    let alive = true
    const sync = () => {
      if (alive) setFailed(cache.findAll({ predicate: failedActive }))
    }
    sync()
    // Notifikasi cache bisa datang SAAT komponen lain sedang render (useQuery membuat observer di fase render). setState sinkron di
    // sini memicu peringatan React "Cannot update a component while rendering a different component"; ditunda ke microtask.
    const unsubscribe = cache.subscribe(() => queueMicrotask(sync))
    return () => {
      alive = false
      unsubscribe()
    }
  }, [qc])

  if (failed.length === 0) return null

  // pesan pertama yang bermakna (401 ditangani AuthProvider dan menghasilkan null)
  const message = failed.map((q) => describeQueryError(q.state.error)).find(Boolean)
  if (!message) return null

  async function retry() {
    setRetrying(true)
    try {
      await qc.refetchQueries({ predicate: failedActive })
    } finally {
      setRetrying(false)
    }
  }

  return (
    <div
      role="alert"
      className="mb-4 flex flex-wrap items-center justify-between gap-3 rounded-2xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-900 dark:border-red-500/30 dark:bg-red-500/10 dark:text-red-200"
    >
      <span className="flex items-start gap-2">
        <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
        <span>
          <strong className="font-semibold">{failed.length > 1 ? `${failed.length} bagian halaman gagal dimuat.` : 'Sebagian halaman gagal dimuat.'}</strong>{' '}
          {message} Data yang tampil mungkin tidak lengkap.
        </span>
      </span>
      <button
        type="button"
        onClick={() => void retry()}
        disabled={retrying}
        className="inline-flex items-center gap-1.5 rounded-full border border-red-300 bg-white px-3 py-1.5 text-sm font-medium text-red-800 hover:bg-red-100 disabled:opacity-60 dark:border-red-500/40 dark:bg-transparent dark:text-red-200 dark:hover:bg-red-500/20"
      >
        <RefreshCw className={`h-3.5 w-3.5 ${retrying ? 'animate-spin' : ''}`} aria-hidden="true" /> Coba lagi
      </button>
    </div>
  )
}
