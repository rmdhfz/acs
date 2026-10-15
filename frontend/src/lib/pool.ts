import { ApiError } from './api'

/** sleep yang bisa dibatalkan lewat AbortSignal. */
export function sleep(ms: number, signal?: AbortSignal): Promise<void> {
  return new Promise((resolve) => {
    if (signal?.aborted) return resolve()
    const t = setTimeout(resolve, ms)
    signal?.addEventListener(
      'abort',
      () => {
        clearTimeout(t)
        resolve()
      },
      { once: true },
    )
  })
}

/**
 * withRetry menjalankan fn dan mengulang bila server menjawab 429 (rate limit)
 * atau 503 (sibuk) dengan backoff eksponensial. Error lain langsung dilempar.
 */
export async function withRetry<T>(fn: () => Promise<T>, signal?: AbortSignal, retries = 5): Promise<T> {
  let attempt = 0
  for (;;) {
    try {
      return await fn()
    } catch (err) {
      const retryable = err instanceof ApiError && (err.status === 429 || err.status === 503)
      if (!retryable || attempt >= retries || signal?.aborted) throw err
      await sleep(Math.min(8000, 700 * 2 ** attempt), signal)
      attempt++
    }
  }
}

/**
 * runPool menjalankan worker pada tiap item dengan batas paralelisme, menjaga
 * urutan hasil, dan berhenti menjadwalkan item baru saat signal dibatalkan
 * (item yang sedang berjalan dibiarkan selesai). Item yang tak sempat berjalan
 * bernilai undefined.
 */
export async function runPool<T, R>(
  items: T[],
  worker: (item: T, index: number) => Promise<R>,
  opts: { concurrency?: number; signal?: AbortSignal; onProgress?: (done: number, total: number) => void } = {},
): Promise<(R | undefined)[]> {
  const results: (R | undefined)[] = new Array(items.length)
  let next = 0
  let done = 0
  const total = items.length
  const concurrency = Math.max(1, Math.min(opts.concurrency ?? 4, total || 1))

  async function lane() {
    for (;;) {
      if (opts.signal?.aborted) return
      const i = next++
      if (i >= total) return
      results[i] = await worker(items[i], i)
      done++
      opts.onProgress?.(done, total)
    }
  }
  await Promise.all(Array.from({ length: concurrency }, lane))
  return results
}
