import { ChevronLeft, ChevronRight } from 'lucide-react'

export const PAGE_SIZES = [10, 25, 50, 100] as const

interface PagerProps {
  /** Halaman 1-based. */
  page: number
  pageSize: number
  total: number
  onPage: (p: number) => void
  onPageSize?: (n: number) => void
  /** Nama pada aria-label, mis. "tenant". */
  label?: string
}

const btn =
  'inline-flex h-8 w-8 items-center justify-center rounded-full text-slate-600 transition-colors hover:bg-slate-100 disabled:cursor-not-allowed disabled:opacity-40 dark:text-slate-300 dark:hover:bg-slate-800'

// Kontrol paginasi seragam: "a–b dari N", pemilih ukuran halaman, dan tombol sebelumnya/berikutnya.
export function Pager({ page, pageSize, total, onPage, onPageSize, label = 'data' }: PagerProps) {
  const pages = Math.max(1, Math.ceil(total / pageSize))
  const cur = Math.min(Math.max(1, page), pages)
  const from = total === 0 ? 0 : (cur - 1) * pageSize + 1
  const to = Math.min(total, cur * pageSize)
  return (
    <nav aria-label={`Paginasi ${label}`} className="flex flex-wrap items-center justify-between gap-3 border-t border-slate-200 px-4 py-2.5 text-sm text-slate-600 dark:border-slate-800 dark:text-slate-400">
      <span aria-live="polite">
        {from.toLocaleString('id-ID')}–{to.toLocaleString('id-ID')} dari {total.toLocaleString('id-ID')}
      </span>
      <div className="flex items-center gap-2">
        {onPageSize && (
          <label className="flex items-center gap-1.5">
            <span className="hidden sm:inline">Baris</span>
            <select
              aria-label={`Baris per halaman ${label}`}
              value={pageSize}
              onChange={(e) => onPageSize(Number(e.target.value))}
              className="rounded-lg border border-slate-300 bg-white px-2 py-1 text-sm text-slate-800 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-200"
            >
              {PAGE_SIZES.map((n) => (
                <option key={n} value={n}>
                  {n}
                </option>
              ))}
            </select>
          </label>
        )}
        <span className="tabular-nums">
          Hal. {cur}/{pages}
        </span>
        <button type="button" aria-label="Halaman sebelumnya" disabled={cur <= 1} onClick={() => onPage(cur - 1)} className={btn}>
          <ChevronLeft className="h-4 w-4" />
        </button>
        <button type="button" aria-label="Halaman berikutnya" disabled={cur >= pages} onClick={() => onPage(cur + 1)} className={btn}>
          <ChevronRight className="h-4 w-4" />
        </button>
      </div>
    </nav>
  )
}
