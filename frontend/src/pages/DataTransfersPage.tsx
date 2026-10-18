import { useState } from 'react'
import { Download, History, Upload } from 'lucide-react'
import { Modal } from '../components/Modal'
import { Pager } from '../components/Pager'
import { PageSpinner } from '../components/Spinner'
import { EmptyState } from '../components/EmptyState'
import { useAuth } from '../lib/auth'
import { formatDateTime } from '../lib/format'
import { useDataTransfer, useDataTransfers, useTenants, type TransferStatus } from '../lib/hooks'

const STATUS: Record<TransferStatus, { label: string; cls: string }> = {
  COMPLETED: { label: 'Selesai', cls: 'bg-emerald-100 text-emerald-800 dark:bg-emerald-500/15 dark:text-emerald-300' },
  PARTIAL: { label: 'Sebagian berhasil', cls: 'bg-amber-100 text-amber-800 dark:bg-amber-500/15 dark:text-amber-300' },
  FAILED: { label: 'Gagal', cls: 'bg-red-100 text-red-800 dark:bg-red-500/15 dark:text-red-300' },
}

const field =
  'rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 outline-none focus:border-indigo-500 focus:ring-2 focus:ring-indigo-500/20 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-100'

// Nama modul untuk tampilan: kunci "device_model" -> "Device model".
export function moduleLabel(m: string): string {
  const s = m.replace(/_/g, ' ')
  return s.charAt(0).toUpperCase() + s.slice(1)
}

// Riwayat impor/ekspor. Staf tenant melihat riwayat tenant-nya sendiri; SUPERADMIN melihat semua tenant. Pemisahan
// ditegakkan server (parameter tenant dari klien diabaikan untuk non-SUPERADMIN).
export function DataTransfersPage() {
  const { user } = useAuth()
  const isSuper = !!user?.roles.includes('SUPERADMIN') && !user.impersonator_id
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(25)
  const [kind, setKind] = useState('')
  const [tenantId, setTenantId] = useState<number | null>(null)
  const [openId, setOpenId] = useState<string | null>(null)
  const q = useDataTransfers({ page, pageSize, kind, tenantId: isSuper ? tenantId : null })
  const tenants = useTenants(isSuper)

  return (
    <div className="px-6 py-6">
      <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold text-slate-900 dark:text-slate-100">Riwayat impor &amp; ekspor</h1>
          <p className="mt-1 text-sm text-slate-600 dark:text-slate-400">
            {isSuper ? 'Seluruh proses impor/ekspor dari semua tenant.' : 'Seluruh proses impor/ekspor data di tenant Anda. Ringkasan juga dikirim ke email pelaku.'}
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          {isSuper && (
            <select aria-label="Filter tenant" value={tenantId ?? ''} onChange={(e) => { setTenantId(e.target.value ? Number(e.target.value) : null); setPage(1) }} className={`${field} w-56`}>
              <option value="">Semua tenant</option>
              {(tenants.data?.data ?? []).map((t) => (
                <option key={t.id} value={t.id}>{t.name}</option>
              ))}
            </select>
          )}
          <select aria-label="Filter jenis" value={kind} onChange={(e) => { setKind(e.target.value); setPage(1) }} className={field}>
            <option value="">Impor &amp; ekspor</option>
            <option value="IMPORT">Impor</option>
            <option value="EXPORT">Ekspor</option>
          </select>
        </div>
      </div>

      {q.isLoading ? (
        <PageSpinner />
      ) : (q.data?.data.length ?? 0) === 0 ? (
        <EmptyState icon={History} title="Belum ada riwayat" description="Proses impor dan ekspor data akan tercatat di sini." />
      ) : (
        <div className="overflow-hidden rounded-3xl border border-slate-200 bg-white shadow-sm dark:border-slate-800 dark:bg-slate-900">
          <div className="table-scroll">
            <table className="w-full text-left text-sm">
              <thead className="text-xs uppercase text-slate-500 dark:text-slate-400">
                <tr>
                  <th scope="col" className="px-4 py-3">Waktu</th>
                  <th scope="col" className="px-4 py-3">Jenis</th>
                  <th scope="col" className="px-4 py-3">Modul</th>
                  {isSuper && <th scope="col" className="px-4 py-3">Tenant</th>}
                  <th scope="col" className="px-4 py-3">Pelaku</th>
                  <th scope="col" className="px-4 py-3 text-right">Baris (ok/gagal/total)</th>
                  <th scope="col" className="px-4 py-3">Status</th>
                  <th scope="col" className="px-4 py-3 text-right">Aksi</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
                {q.data?.data.map((r) => (
                  <tr key={r.id}>
                    <td className="whitespace-nowrap px-4 py-3 text-slate-600 dark:text-slate-400">{formatDateTime(r.created_at)}</td>
                    <td className="px-4 py-3">
                      <span className="inline-flex items-center gap-1.5">
                        {r.kind === 'IMPORT' ? <Upload className="h-4 w-4" aria-hidden="true" /> : <Download className="h-4 w-4" aria-hidden="true" />}
                        {r.kind === 'IMPORT' ? 'Impor' : 'Ekspor'}
                      </span>
                    </td>
                    <td className="px-4 py-3">{moduleLabel(r.module)}</td>
                    {isSuper && <td className="px-4 py-3">{r.tenant_name ?? 'Platform'}</td>}
                    <td className="px-4 py-3">
                      {r.user}
                      {r.impersonated && <span className="ml-1 text-xs text-amber-700 dark:text-amber-300">(impersonate)</span>}
                    </td>
                    <td className="whitespace-nowrap px-4 py-3 text-right tabular-nums">
                      {r.success_rows.toLocaleString('id-ID')} / {r.failed_rows.toLocaleString('id-ID')} / {r.total_rows.toLocaleString('id-ID')}
                    </td>
                    <td className="px-4 py-3">
                      <span className={`inline-flex rounded-full px-2.5 py-0.5 text-xs font-medium ${STATUS[r.status]?.cls ?? ''}`}>{STATUS[r.status]?.label ?? r.status}</span>
                    </td>
                    <td className="px-4 py-3 text-right">
                      <button type="button" onClick={() => setOpenId(r.id)} className="rounded-full px-3 py-1.5 text-sm font-medium text-indigo-700 hover:bg-indigo-50 dark:text-indigo-300 dark:hover:bg-indigo-500/10">
                        Detail
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <Pager page={page} pageSize={pageSize} total={q.data?.total ?? 0} onPage={setPage} onPageSize={(n) => { setPageSize(n); setPage(1) }} label="riwayat" />
        </div>
      )}
      {openId && <TransferDetailModal id={openId} onClose={() => setOpenId(null)} />}
    </div>
  )
}

function TransferDetailModal({ id, onClose }: { id: string; onClose: () => void }) {
  const { data, isLoading, error } = useDataTransfer(id)
  return (
    <Modal title="Detail proses" onClose={onClose}>
      {isLoading ? (
        <PageSpinner />
      ) : error || !data ? (
        <p role="alert" className="text-sm text-red-600 dark:text-red-400">Data tidak ditemukan.</p>
      ) : (
        <dl className="grid grid-cols-2 gap-x-4 gap-y-3 text-sm">
          {(
            [
              ['Jenis', data.kind === 'IMPORT' ? 'Impor' : 'Ekspor'],
              ['Status', STATUS[data.status]?.label ?? data.status],
              ['Modul', moduleLabel(data.module)],
              ['Format', data.format.toUpperCase()],
              ['Tenant', data.tenant_name ?? 'Platform'],
              ['Pelaku', data.user + (data.impersonated ? ' (impersonate)' : '')],
              ['Waktu', formatDateTime(data.created_at)],
              ['Sumber catatan', data.source === 'SERVER' ? 'Server' : 'Dilaporkan Console'],
              ['Total baris', data.total_rows.toLocaleString('id-ID')],
              ['Berhasil', data.success_rows.toLocaleString('id-ID')],
              ['Gagal', data.failed_rows.toLocaleString('id-ID')],
              ['Email notifikasi', data.emailed_at ? `Terkirim ${formatDateTime(data.emailed_at)}` : 'Tidak dikirim'],
            ] as const
          ).map(([k, v]) => (
            <div key={k}>
              <dt className="text-xs text-slate-600 dark:text-slate-400">{k}</dt>
              <dd className="font-medium text-slate-900 dark:text-slate-100">{v}</dd>
            </div>
          ))}
          {data.file_name && (
            <div className="col-span-2">
              <dt className="text-xs text-slate-600 dark:text-slate-400">Berkas</dt>
              <dd className="break-all font-mono text-xs text-slate-900 dark:text-slate-100">{data.file_name}</dd>
            </div>
          )}
          {data.detail && (
            <div className="col-span-2">
              <dt className="text-xs text-slate-600 dark:text-slate-400">Catatan</dt>
              <dd className="text-slate-900 dark:text-slate-100">{data.detail}</dd>
            </div>
          )}
        </dl>
      )}
    </Modal>
  )
}
