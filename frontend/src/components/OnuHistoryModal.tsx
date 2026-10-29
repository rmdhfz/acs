import { useState } from 'react'
import { AlertTriangle, ChevronLeft, ChevronRight } from 'lucide-react'
import { Modal } from './Modal'
import { PageSpinner } from './Spinner'
import { StatusBadge } from './StatusBadge'
import { useOLTONUHistory } from '../lib/hooks'
import { formatDateTime, formatRelativeTime } from '../lib/format'
import { formatRx, onuStateBadge, onuStateLabel } from '../lib/oltInventory'
import type { ONUStateEvent } from '../lib/types'

const PAGE_SIZE = 20

// Riwayat perubahan keadaan satu ONU menurut OLT. Waktu = saat polling ACS MENGAMATI perubahan (bukan saat terjadi), dan perubahan yang pulih di
// antara dua polling tidak terlihat; panjang riwayat dibatasi retensi log. Kedua batas itu dinyatakan di footer supaya tidak dibaca sebagai bukti.
export function OnuHistoryModal({ oltId, onu, onClose }: { oltId: string; onu: { id: string; serial_number: string }; onClose: () => void }) {
  const [page, setPage] = useState(1)
  const { data, isLoading, isError } = useOLTONUHistory(oltId, onu.id, page, PAGE_SIZE)
  const events = data?.data ?? []
  const totalPages = Math.max(1, Math.ceil((data?.total ?? 0) / PAGE_SIZE))

  return (
    <Modal title={`Riwayat keadaan — ${onu.serial_number}`} onClose={onClose} wide>
      <div className="space-y-3 p-5" data-modal-body>
        {isLoading ? (
          <PageSpinner />
        ) : isError || !data ? (
          <p role="alert" className="text-sm text-red-600 dark:text-red-400">
            Riwayat tidak dapat dimuat.
          </p>
        ) : (
          <>
            {data.flapping && (
              <p role="alert" className="flex items-start gap-2 rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-900 dark:border-red-500/30 dark:bg-red-500/10 dark:text-red-200">
                <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
                ONU ini tidak stabil (flapping): berganti keadaan 4 kali atau lebih dalam 1 jam terakhir. Periksa konektor, redaman, dan catu daya di sisi pelanggan.
              </p>
            )}
            <p className="text-sm text-slate-700 dark:text-slate-300">
              {data.changes_24h} perubahan keadaan dalam 24 jam terakhir · {data.total} peristiwa tercatat
            </p>
            {events.length === 0 ? (
              <p className="py-6 text-center text-sm text-slate-500 dark:text-slate-400">Belum ada peristiwa tercatat untuk ONU ini.</p>
            ) : (
              <ol className="divide-y divide-slate-100 rounded-lg border border-slate-200 dark:divide-slate-800 dark:border-slate-800">
                {events.map((e, i) => (
                  <EventRow key={`${e.occurred_at}-${i}`} e={e} />
                ))}
              </ol>
            )}
            {data.total > PAGE_SIZE && (
              <div className="flex items-center justify-end gap-1 text-xs text-slate-500 dark:text-slate-400">
                <button type="button" onClick={() => setPage((p) => Math.max(1, p - 1))} disabled={page <= 1} aria-label="Halaman sebelumnya" className="rounded-full border border-slate-200 p-1.5 disabled:opacity-40 dark:border-slate-700">
                  <ChevronLeft className="h-4 w-4" />
                </button>
                <span className="px-2 tabular-nums">
                  {page} / {totalPages}
                </span>
                <button type="button" onClick={() => setPage((p) => Math.min(totalPages, p + 1))} disabled={page >= totalPages} aria-label="Halaman berikutnya" className="rounded-full border border-slate-200 p-1.5 disabled:opacity-40 dark:border-slate-700">
                  <ChevronRight className="h-4 w-4" />
                </button>
              </div>
            )}
            <p className="border-t border-slate-100 pt-2 text-xs text-slate-500 dark:border-slate-800 dark:text-slate-400">
              Waktu adalah saat polling ACS mengamati perubahan, bukan saat terjadi. Gangguan yang pulih di antara dua polling tidak tercatat, dan riwayat hanya disimpan selama masa
              retensi log.
            </p>
          </>
        )}
      </div>
    </Modal>
  )
}

function EventRow({ e }: { e: ONUStateEvent }) {
  return (
    <li className="flex flex-wrap items-center gap-x-3 gap-y-1 px-3 py-2 text-sm">
      <span className="w-44 shrink-0 text-xs text-slate-600 dark:text-slate-300" title={formatDateTime(e.occurred_at)}>
        {formatDateTime(e.occurred_at)}
        <span className="ml-1 text-slate-400">({formatRelativeTime(e.occurred_at)})</span>
      </span>
      <span className="flex flex-wrap items-center gap-2">
        {e.from_state === null && e.to_state !== null ? (
          <>
            <span className="text-xs text-slate-600 dark:text-slate-300">Baru terlihat sebagai</span>
            <StatusBadge code={onuStateBadge(e.to_state)} label={onuStateLabel(e.to_state)} />
          </>
        ) : e.to_state === null ? (
          <span className="text-xs font-medium text-slate-700 dark:text-slate-300">Tidak lagi dilaporkan OLT{e.from_state ? ` (sebelumnya ${onuStateLabel(e.from_state)})` : ''}</span>
        ) : (
          <>
            <StatusBadge code={onuStateBadge(e.from_state ?? 'UNKNOWN')} label={onuStateLabel(e.from_state ?? 'UNKNOWN')} />
            <span aria-hidden="true" className="text-slate-400">
              →
            </span>
            <StatusBadge code={onuStateBadge(e.to_state)} label={onuStateLabel(e.to_state)} />
          </>
        )}
      </span>
      <span className="ml-auto font-mono text-xs text-slate-500 dark:text-slate-400">
        {e.pon_port || '—'} · {formatRx(e.rx_power_dbm)}
      </span>
    </li>
  )
}
