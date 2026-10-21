import { formatRelativeTime } from '../lib/format'
import { OPTICAL_LEVEL_HINT, opticalLevel } from '../lib/optical'

const TONE = {
  ok: 'text-slate-900 dark:text-slate-100',
  warn: 'font-medium text-amber-600 dark:text-amber-400',
  bad: 'font-semibold text-red-600 dark:text-red-400',
} as const

/** Sel "Redaman" daftar perangkat: RX dBm berwarna menurut level; tooltip memuat TX, status, dan umur data. */
export function OpticalCell({ rx, tx, at }: { rx: number | null | undefined; tx?: number | null; at?: string | null }) {
  const level = opticalLevel(rx)
  if (level === null || rx == null) {
    return (
      <span className="text-slate-400 dark:text-slate-500" title="Belum pernah terbaca. Pakai ‘Segarkan data’; butuh mapping optik untuk vendor/model ini.">
        -
      </span>
    )
  }
  const title = [
    `RX ${rx.toFixed(2)} dBm — ${OPTICAL_LEVEL_HINT[level]}`,
    tx != null ? `TX ${tx.toFixed(2)} dBm` : null,
    at ? `Dibaca ${formatRelativeTime(at)}` : null,
  ]
    .filter(Boolean)
    .join(' · ')
  return (
    <span className={`whitespace-nowrap tabular-nums ${TONE[level]}`} title={title}>
      {rx.toFixed(2)} dBm
    </span>
  )
}
