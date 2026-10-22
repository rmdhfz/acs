import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { AlertTriangle, ChevronLeft, ChevronRight, X } from 'lucide-react'
import { useI18n } from '../lib/i18n'
import { useAuth } from '../lib/auth'
import { useToast } from '../lib/toast'
import { useConfirm } from '../lib/confirm'
import { EmptyState } from '../components/EmptyState'
import { useDismissFault, useFaults, useRefs, type FaultFilters } from '../lib/hooks'
import { formatDateTime, formatRelativeTime } from '../lib/format'

const PAGE_SIZE = 25
const inputCls =
  'w-full rounded-lg border border-slate-300 px-3 py-2 text-sm text-slate-900 outline-none transition-colors focus:border-indigo-600 focus:ring-1 focus:ring-indigo-600 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-100'

// Arti kode fault standar TR-069 (Tabel Fault Codes); kode di luar daftar (kuirk vendor) tampil apa adanya.
const FAULT_MEANING: Record<string, string> = {
  '9000': 'Method tidak didukung',
  '9001': 'Permintaan ditolak',
  '9002': 'Kesalahan internal perangkat',
  '9003': 'Argumen tidak valid',
  '9004': 'Sumber daya perangkat habis',
  '9005': 'Nama parameter tidak valid',
  '9006': 'Tipe parameter tidak valid',
  '9007': 'Nilai parameter tidak valid',
  '9008': 'Parameter tidak dapat ditulis',
  '9009': 'Permintaan notifikasi ditolak',
  '9010': 'Download gagal',
  '9011': 'Upload gagal',
  '9012': 'Autentikasi server transfer gagal',
  '9013': 'Protokol transfer tidak didukung',
}

export function FaultsPage() {
  const { t: tl } = useI18n()
  const { hasRole } = useAuth()
  const canManage = hasRole('ADMIN', 'NOC')
  const toast = useToast()
  const confirm = useConfirm()

  const [search, setSearch] = useState('')
  const [code, setCode] = useState('')
  const [rpc, setRpc] = useState('')
  const [page, setPage] = useState(1)

  const { data: typeRefs } = useRefs('ref_task_types')
  const filters: FaultFilters = useMemo(
    () => ({ search: search.trim() || undefined, fault_code: code.trim() || undefined, rpc_method: rpc || undefined, page, page_size: PAGE_SIZE }),
    [search, code, rpc, page],
  )
  const { data, isLoading } = useFaults(filters)
  const faults = data?.data ?? []
  const total = data?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))
  const dismiss = useDismissFault()

  const handleDismiss = async (id: string, serial: string) => {
    const ok = await confirm({
      title: 'Tutup fault',
      message: `Fault pada ${serial} akan dihapus dari daftar. Muncul lagi bila perangkat mengulang fault yang sama.`,
      confirmLabel: 'Tutup fault',
      tone: 'danger',
    })
    if (!ok) return
    dismiss.mutate(id, {
      onSuccess: () => toast.success('Fault ditutup.'),
      onError: (e) => toast.error(e instanceof Error ? e.message : 'Gagal menutup fault'),
    })
  }

  return (
    <div className="mx-auto max-w-7xl px-6 py-8">
      <div className="mb-6">
        <h1 className="text-xl font-semibold text-slate-900 dark:text-slate-100">{tl('nav.faults')}</h1>
        <p className="mt-0.5 text-sm text-slate-500 dark:text-slate-400">
          Perangkat yang menolak RPC dari ACS (cwmp:Fault) — satu baris per perangkat, kode, dan jenis RPC. Hilang otomatis saat RPC yang sama berhasil.
        </p>
      </div>

      <div className="mb-4 flex flex-wrap items-center gap-3">
        <input
          type="search"
          maxLength={255}
          placeholder="Cari serial atau teks fault…"
          aria-label="Cari fault"
          value={search}
          onChange={(e) => {
            setSearch(e.target.value)
            setPage(1)
          }}
          className={`max-w-[260px] ${inputCls}`}
        />
        <input
          type="text"
          maxLength={16}
          placeholder="Kode (mis. 9005)"
          aria-label="Filter kode fault"
          value={code}
          onChange={(e) => {
            setCode(e.target.value)
            setPage(1)
          }}
          className={`max-w-[160px] ${inputCls}`}
        />
        <select
          value={rpc}
          aria-label="Filter jenis RPC"
          onChange={(e) => {
            setRpc(e.target.value)
            setPage(1)
          }}
          className={`max-w-[240px] ${inputCls}`}
        >
          <option value="">Semua jenis RPC</option>
          {typeRefs?.map((t) => (
            <option key={t.id} value={t.code}>
              {t.name}
            </option>
          ))}
        </select>
      </div>

      <div className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm dark:border-slate-800 dark:bg-slate-900">
        {isLoading ? (
          <div className="divide-y divide-slate-100 dark:divide-slate-800">
            {Array.from({ length: 5 }).map((_, i) => (
              <div key={i} className="flex items-center gap-4 px-5 py-4">
                <div className="h-5 w-20 animate-pulse rounded-full bg-slate-100 dark:bg-slate-800" />
                <div className="h-4 flex-1 animate-pulse rounded bg-slate-100 dark:bg-slate-800" />
              </div>
            ))}
          </div>
        ) : faults.length === 0 ? (
          <EmptyState icon={AlertTriangle} title="Tidak ada fault aktif" description="Perangkat yang menolak RPC dari ACS akan muncul di sini." />
        ) : (
          <>
            <div className="table-scroll">
              <table className="w-full text-left text-sm">
                <thead>
                  <tr className="border-b border-slate-200 bg-slate-50 text-xs font-medium uppercase tracking-wide text-slate-500 dark:border-slate-800 dark:bg-slate-800/50 dark:text-slate-400">
                    <th className="px-5 py-3">Perangkat</th>
                    <th className="px-5 py-3">Fault</th>
                    <th className="px-5 py-3">RPC</th>
                    <th className="px-5 py-3">Kejadian</th>
                    <th className="px-5 py-3">Terakhir</th>
                    <th className="px-5 py-3"></th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
                  {faults.map((f) => (
                    <tr key={f.id} className="align-top">
                      <td className="px-5 py-3.5">
                        <Link to={`/devices/${f.device_id}`} className="font-mono text-xs text-indigo-700 hover:underline dark:text-indigo-300">
                          {f.device_serial}
                        </Link>
                      </td>
                      <td className="px-5 py-3.5">
                        <span className="inline-flex items-center rounded-full bg-red-50 px-2 py-0.5 font-mono text-xs font-medium text-red-700 dark:bg-red-950/40 dark:text-red-300">
                          {f.fault_code}
                        </span>
                        {FAULT_MEANING[f.fault_code] && (
                          <span className="ml-2 text-xs text-slate-500 dark:text-slate-400">{FAULT_MEANING[f.fault_code]}</span>
                        )}
                        {f.fault_string && <p className="mt-1 max-w-md break-words text-xs text-slate-600 dark:text-slate-400">{f.fault_string}</p>}
                      </td>
                      <td className="px-5 py-3.5 text-xs text-slate-600 dark:text-slate-400">
                        {typeRefs?.find((t) => t.code === f.rpc_method)?.name ?? (f.rpc_method || '-')}
                      </td>
                      <td className="px-5 py-3.5 tabular-nums text-slate-700 dark:text-slate-300">{f.occurrence_count}×</td>
                      <td className="px-5 py-3.5 text-slate-500 dark:text-slate-400" title={`Pertama: ${formatDateTime(f.first_seen_at)}`}>
                        {formatRelativeTime(f.last_seen_at)}
                      </td>
                      <td className="px-5 py-3.5 text-right">
                        {canManage && (
                          <button
                            onClick={() => void handleDismiss(f.id, f.device_serial)}
                            className="rounded-md p-1.5 text-slate-500 transition-colors hover:bg-red-50 hover:text-red-600 dark:text-slate-400"
                            title="Tutup fault"
                            aria-label={`Tutup fault ${f.device_serial}`}
                          >
                            <X className="h-4 w-4" />
                          </button>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            <div className="flex items-center justify-between border-t border-slate-200 px-5 py-3 text-sm text-slate-500 dark:border-slate-800 dark:text-slate-400">
              <span>
                Menampilkan {faults.length} dari {total} fault
              </span>
              <div className="flex items-center gap-1">
                <button
                  onClick={() => setPage((p) => Math.max(1, p - 1))}
                  disabled={page <= 1}
                  aria-label="Halaman sebelumnya"
                  className="flex h-8 w-8 items-center justify-center rounded-full border border-slate-200 text-slate-500 transition-colors hover:bg-slate-50 disabled:cursor-not-allowed disabled:opacity-40 dark:border-slate-700 dark:text-slate-400 dark:hover:bg-slate-800/50"
                >
                  <ChevronLeft className="h-4 w-4" />
                </button>
                <span className="px-2 text-xs tabular-nums">
                  {page} / {totalPages}
                </span>
                <button
                  onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
                  disabled={page >= totalPages}
                  aria-label="Halaman berikutnya"
                  className="flex h-8 w-8 items-center justify-center rounded-full border border-slate-200 text-slate-500 transition-colors hover:bg-slate-50 disabled:cursor-not-allowed disabled:opacity-40 dark:border-slate-700 dark:text-slate-400 dark:hover:bg-slate-800/50"
                >
                  <ChevronRight className="h-4 w-4" />
                </button>
              </div>
            </div>
          </>
        )}
      </div>
    </div>
  )
}
