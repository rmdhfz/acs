import { useEffect, useMemo, useRef, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { AlertTriangle, ChevronLeft, ChevronRight, Network } from 'lucide-react'
import { EmptyState } from './EmptyState'
import { StatusBadge } from './StatusBadge'
import { useOLTPorts, useOLTRegisteredONUs, type RegisteredONUFilters } from '../lib/hooks'
import { formatRelativeTime } from '../lib/format'
import { ONU_STATE_LABEL, flaggedPorts, formatRx, isOltDataStale, onuStateBadge, onuStateLabel, portHint, portTone, stateSince, sumPorts, type PortTone } from '../lib/oltInventory'
import type { OLT, OLTONUState } from '../lib/types'

const PAGE_SIZE = 50

const inputCls =
  'rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 outline-none focus:border-indigo-600 focus:ring-1 focus:ring-indigo-600 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-100'

const TONE_ROW: Record<PortTone, string> = {
  bad: 'bg-red-50/60 dark:bg-red-500/5',
  warn: 'bg-amber-50/60 dark:bg-amber-500/5',
  ok: '',
  idle: 'opacity-60',
}

// ONU terdaftar & port PON dari polling OLT (hanya baca). Keadaan berasal dari polling terakhir, bukan detik ini: usia data selalu ditampilkan
// dan data lebih tua dari 30 menit diberi catatan. Tautan "Perangkat ACS" = serial yang sama di tenant yang sama.
export function OltInventory({ olts }: { olts: OLT[] }) {
  // Tautan dalam (mis. dari notifikasi ONU LOS): /olts?olt=<uuid>&problem=1 membuka OLT itu dengan filter "bermasalah saja" sudah aktif.
  const [params] = useSearchParams()
  const [selected, setSelected] = useState(() => params.get('olt') ?? '')
  const oltId = olts.some((o) => o.id === selected) ? selected : (olts[0]?.id ?? '')
  const olt = olts.find((o) => o.id === oltId)

  const [tab, setTab] = useState<'onus' | 'ports'>('onus')
  const [state, setState] = useState('')
  const [port, setPort] = useState('')
  const [search, setSearch] = useState('')
  const [problem, setProblem] = useState(() => params.get('problem') === '1')
  const [page, setPage] = useState(1)
  // OLT berganti = filter dan halaman kembali ke awal (port milik OLT lain tidak berlaku). Render pertama tidak mereset, supaya filter dari
  // tautan dalam tidak terhapus.
  const prevOlt = useRef(oltId)
  useEffect(() => {
    if (prevOlt.current === oltId) return
    prevOlt.current = oltId
    setState('')
    setPort('')
    setSearch('')
    setProblem(false)
    setPage(1)
  }, [oltId])

  const filters: RegisteredONUFilters = useMemo(
    () => ({ pon_port: port || undefined, state: state || undefined, search: search.trim() || undefined, problem: problem || undefined, sort: 'severity', page, page_size: PAGE_SIZE }),
    [port, state, search, problem, page],
  )
  const ports = useOLTPorts(oltId)
  const onus = useOLTRegisteredONUs(oltId, filters, tab === 'onus')

  if (!olt) return null
  const portRows = ports.data?.data ?? []
  const totals = sumPorts(portRows)
  const rows = onus.data?.data ?? []
  const total = onus.data?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))
  const stale = isOltDataStale(olt.last_poll_at)
  const flagged = flaggedPorts(portRows)
  const unavailable = ports.isError || (tab === 'onus' && onus.isError)

  return (
    <section aria-label="ONU terdaftar dan port PON" className="mb-8 rounded-xl border border-slate-200 bg-white shadow-sm dark:border-slate-800 dark:bg-slate-900">
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-slate-100 px-5 py-4 dark:border-slate-800">
        <div>
          <h2 className="text-sm font-semibold text-slate-900 dark:text-slate-100">ONU terdaftar &amp; port PON</h2>
          <p className="mt-0.5 text-xs text-slate-500 dark:text-slate-400">
            Keadaan menurut OLT pada polling terakhir
            {olt.last_poll_at ? ` (${formatRelativeTime(olt.last_poll_at)})` : ' — OLT ini belum pernah dipoll'}. Hanya baca: tidak ada kontrol port atau penulisan ke OLT.
          </p>
        </div>
        {olts.length > 1 ? (
          <select value={oltId} onChange={(e) => setSelected(e.target.value)} aria-label="Pilih OLT" className={inputCls}>
            {olts.map((o) => (
              <option key={o.id} value={o.id}>
                {o.name}
              </option>
            ))}
          </select>
        ) : (
          <span className="text-sm font-medium text-slate-700 dark:text-slate-300">{olt.name}</span>
        )}
      </div>

      {stale && olt.last_poll_at && (
        <p role="status" className="border-b border-amber-100 bg-amber-50 px-5 py-2 text-xs text-amber-800 dark:border-amber-500/20 dark:bg-amber-500/10 dark:text-amber-300">
          Data OLT sudah lebih dari 30 menit: keadaan di bawah mungkin sudah berubah. Tekan Poll pada OLT di atas, atau nyalakan polling berkala (<code>ACS_OLT_POLL_MINUTES</code>).
        </p>
      )}

      <div className="flex flex-wrap items-center gap-3 px-5 pt-4">
        <div role="group" aria-label="Tampilan" className="inline-flex rounded-full border border-slate-300 bg-white p-0.5 text-sm dark:border-slate-700 dark:bg-slate-900">
          {(['onus', 'ports'] as const).map((k) => (
            <button
              key={k}
              type="button"
              aria-pressed={tab === k}
              onClick={() => setTab(k)}
              className={`rounded-full px-3.5 py-1.5 font-medium transition-colors ${tab === k ? 'bg-indigo-600 text-white' : 'text-slate-600 hover:bg-slate-50 dark:text-slate-300 dark:hover:bg-slate-800'}`}
            >
              {k === 'onus' ? 'ONU terdaftar' : 'Port PON'}
            </button>
          ))}
        </div>
        {ports.data && (
          <span className="text-xs text-slate-600 dark:text-slate-300">
            {totals.total} ONU · {totals.online} online
            {totals.problem > 0 && <strong className="ml-1 text-red-600 dark:text-red-400">· {totals.problem} LOS/dying gasp</strong>}
            {totals.offline > 0 && <strong className="ml-1 text-slate-700 dark:text-slate-300">· {totals.offline} offline</strong>}
          </span>
        )}
        {tab === 'onus' && (
          <div className="ml-auto flex flex-wrap items-center gap-2">
            <select value={port} onChange={(e) => { setPort(e.target.value); setPage(1) }} aria-label="Filter port PON" className={inputCls}>
              <option value="">Semua port</option>
              {portRows.map((p) => (
                <option key={p.pon_port} value={p.pon_port}>
                  {p.pon_port}
                </option>
              ))}
            </select>
            <select value={state} onChange={(e) => { setState(e.target.value); setPage(1) }} aria-label="Filter keadaan" className={inputCls}>
              <option value="">Semua keadaan</option>
              {(Object.keys(ONU_STATE_LABEL) as OLTONUState[]).map((s) => (
                <option key={s} value={s}>
                  {ONU_STATE_LABEL[s]}
                </option>
              ))}
            </select>
            <button
              type="button"
              aria-pressed={problem}
              onClick={() => { setProblem((v) => !v); setPage(1) }}
              className={`rounded-full border px-3 py-1.5 text-sm font-medium transition-colors ${problem ? 'border-red-600 bg-red-600 text-white' : 'border-slate-300 text-slate-700 hover:bg-slate-50 dark:border-slate-700 dark:text-slate-300 dark:hover:bg-slate-800'}`}
            >
              Bermasalah saja
            </button>
            <input
              type="text"
              maxLength={100}
              value={search}
              onChange={(e) => { setSearch(e.target.value); setPage(1) }}
              placeholder="Cari serial atau port..."
              aria-label="Cari ONU terdaftar"
              className={inputCls}
            />
          </div>
        )}
      </div>

      {flagged.length > 0 && (
        <div role="alert" aria-label="Port PON dengan dugaan gangguan" className="mx-5 mt-4 rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-900 dark:border-red-500/30 dark:bg-red-500/10 dark:text-red-200">
          <p className="flex items-center gap-2 font-medium">
            <AlertTriangle className="h-4 w-4 shrink-0" /> Dugaan gangguan per port PON
          </p>
          <ul className="mt-1 space-y-1 text-xs">
            {flagged.map((f) => (
              <li key={f.port}>
                <button
                  type="button"
                  onClick={() => { setPort(f.port); setProblem(true); setPage(1); setTab('onus') }}
                  className="font-mono font-semibold underline-offset-2 hover:underline"
                  aria-label={`Lihat ONU bermasalah di port ${f.port}`}
                >
                  {f.port}
                </button>
                : {f.hint.text}
              </li>
            ))}
          </ul>
          <p className="mt-1 text-xs opacity-80">Ini dugaan dari pola data OLT (banyak ONU di satu port), bukan kepastian.</p>
        </div>
      )}

      <div className="px-5 pb-5 pt-4">
        {unavailable ? (
          <p role="status" className="rounded-lg border border-slate-200 px-4 py-6 text-center text-sm text-slate-500 dark:border-slate-800 dark:text-slate-400">
            Inventaris ONU terdaftar tidak tersedia untuk OLT ini.
          </p>
        ) : tab === 'ports' ? (
          portRows.length === 0 ? (
            <Empty polled={!!olt.last_poll_at} />
          ) : (
            <div className="table-scroll">
              <table className="w-full text-left text-sm">
                <thead>
                  <tr className="border-b border-slate-200 text-xs font-medium uppercase tracking-wide text-slate-500 dark:border-slate-800 dark:text-slate-400">
                    <th className="px-3 py-2">Port PON</th>
                    <th className="px-3 py-2 text-right">ONU</th>
                    <th className="px-3 py-2 text-right">Online</th>
                    <th className="px-3 py-2 text-right">Offline</th>
                    <th className="px-3 py-2 text-right">LOS</th>
                    <th className="px-3 py-2 text-right">Dying gasp</th>
                    <th className="px-3 py-2 text-right">Lainnya</th>
                    <th className="px-3 py-2 text-right">Tertaut ACS</th>
                    <th className="px-3 py-2">RX OLT (min / rata-rata)</th>
                    <th className="px-3 py-2">Dugaan</th>
                    <th className="px-3 py-2" />
                  </tr>
                </thead>
                <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
                  {portRows.map((p) => (
                    <tr key={p.pon_port} className={TONE_ROW[portTone(p)]}>
                      <td className="px-3 py-2 font-mono text-xs font-medium text-slate-900 dark:text-slate-100">{p.pon_port || '—'}</td>
                      <td className="px-3 py-2 text-right tabular-nums">{p.total}</td>
                      <td className="px-3 py-2 text-right tabular-nums">{p.online}</td>
                      <td className="px-3 py-2 text-right tabular-nums">{p.offline}</td>
                      <td className={`px-3 py-2 text-right tabular-nums ${p.los > 0 ? 'font-semibold text-red-600 dark:text-red-400' : ''}`}>{p.los}</td>
                      <td className={`px-3 py-2 text-right tabular-nums ${p.dying_gasp > 0 ? 'font-semibold text-orange-600 dark:text-orange-400' : ''}`}>{p.dying_gasp}</td>
                      <td className="px-3 py-2 text-right tabular-nums">{p.unknown}</td>
                      <td className="px-3 py-2 text-right tabular-nums">{p.linked}</td>
                      <td className="px-3 py-2 text-xs text-slate-600 dark:text-slate-300">
                        {formatRx(p.min_rx_dbm)} / {formatRx(p.avg_rx_dbm)}
                      </td>
                      <td className="px-3 py-2 text-xs text-slate-600 dark:text-slate-300">{portHint(p)?.kind === 'FEEDER' ? 'Feeder/distribusi' : portHint(p)?.kind === 'POWER' ? 'Listrik area' : '—'}</td>
                      <td className="px-3 py-2 text-right">
                        <button
                          type="button"
                          onClick={() => { setPort(p.pon_port); setPage(1); setTab('onus') }}
                          className="text-xs font-medium text-indigo-600 hover:underline dark:text-indigo-400"
                          aria-label={`Lihat ONU di port ${p.pon_port}`}
                        >
                          Lihat ONU
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )
        ) : rows.length === 0 ? (
          onus.isLoading ? (
            <p className="py-6 text-center text-sm text-slate-500 dark:text-slate-400">Memuat...</p>
          ) : total === 0 && !port && !state && !search && !problem ? (
            <Empty polled={!!olt.last_poll_at} />
          ) : (
            <p className="py-6 text-center text-sm text-slate-500 dark:text-slate-400">Tidak ada ONU yang cocok dengan filter.</p>
          )
        ) : (
          <>
            <div className="table-scroll">
              <table className="w-full text-left text-sm">
                <thead>
                  <tr className="border-b border-slate-200 text-xs font-medium uppercase tracking-wide text-slate-500 dark:border-slate-800 dark:text-slate-400">
                    <th className="px-3 py-2">Port</th>
                    <th className="px-3 py-2">ONU</th>
                    <th className="px-3 py-2">Serial</th>
                    <th className="px-3 py-2">Keadaan di OLT</th>
                    <th className="px-3 py-2">RX OLT</th>
                    <th className="px-3 py-2">Keadaan sejak</th>
                    <th className="px-3 py-2">Perangkat ACS</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
                  {rows.map((r) => (
                    <tr key={r.id}>
                      <td className="px-3 py-2 font-mono text-xs">{r.pon_port || '—'}</td>
                      <td className="px-3 py-2 text-xs">{r.onu_index ?? '—'}</td>
                      <td className="px-3 py-2 font-mono text-xs font-medium text-slate-900 dark:text-slate-100">{r.serial_number}</td>
                      <td className="px-3 py-2">
                        <StatusBadge code={onuStateBadge(r.state)} label={onuStateLabel(r.state)} />
                        {r.vendor_state && r.state === 'UNKNOWN' && <span className="ml-2 text-xs text-slate-500 dark:text-slate-400">({r.vendor_state})</span>}
                      </td>
                      <td className="px-3 py-2 text-xs tabular-nums">{formatRx(r.rx_power_dbm)}</td>
                      <td className="px-3 py-2 text-xs text-slate-500 dark:text-slate-400">
                        {(() => {
                          const s = stateSince(r)
                          return s.observed ? (
                            formatRelativeTime(s.at)
                          ) : (
                            <span title="ACS belum pernah melihat keadaan ini berubah, jadi waktu mulainya tidak diketahui (bisa jauh lebih lama).">
                              terlihat sejak {formatRelativeTime(s.at)}
                            </span>
                          )
                        })()}
                      </td>
                      <td className="px-3 py-2 text-xs">
                        {r.device_id != null ? (
                          <Link to={`/devices/${r.device_id}`} className="font-medium text-indigo-600 hover:underline dark:text-indigo-400">
                            Buka perangkat{r.device_status ? ` (${r.device_status})` : ''}
                          </Link>
                        ) : (
                          <span className="text-slate-400" title="Tidak ada perangkat ACS dengan serial ini di tenant OLT">
                            Belum tertaut
                          </span>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            <div className="mt-3 flex items-center justify-between text-xs text-slate-500 dark:text-slate-400">
              <span>{total} ONU</span>
              <div className="flex items-center gap-1">
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
            </div>
          </>
        )}
      </div>
    </section>
  )
}

function Empty({ polled }: { polled: boolean }) {
  return (
    <EmptyState
      icon={Network}
      title="Belum ada ONU terdaftar"
      description={
        polled
          ? 'OLT ini sudah dipoll tetapi tidak melaporkan ONU terdaftar. Driver OLT harus mendukung pembacaan ONU terdaftar.'
          : 'Tekan Poll pada OLT di atas untuk membaca ONU terdaftar. Driver OLT harus mendukung pembacaan ONU terdaftar.'
      }
    />
  )
}
