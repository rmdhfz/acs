import { useState, type FormEvent } from 'react'
import { Bug, Mail } from 'lucide-react'
import { Modal } from '../components/Modal'
import { Pager } from '../components/Pager'
import { PageSpinner } from '../components/Spinner'
import { EmptyState } from '../components/EmptyState'
import { ApiError } from '../lib/api'
import { useAuth } from '../lib/auth'
import { formatDateTime } from '../lib/format'
import { useBugReport, useBugReports, useBugScreenshot, useTenants, useUpdateBugReport, type BugReport, type BugSeverity, type BugStatus } from '../lib/hooks'
import { useToast } from '../lib/toast'

const STATUS: Record<BugStatus, { label: string; cls: string }> = {
  OPEN: { label: 'Baru', cls: 'bg-amber-100 text-amber-800 dark:bg-amber-500/15 dark:text-amber-300' },
  IN_PROGRESS: { label: 'Ditangani', cls: 'bg-indigo-100 text-indigo-800 dark:bg-indigo-500/15 dark:text-indigo-300' },
  RESOLVED: { label: 'Selesai', cls: 'bg-emerald-100 text-emerald-800 dark:bg-emerald-500/15 dark:text-emerald-300' },
  WONTFIX: { label: 'Tidak diperbaiki', cls: 'bg-slate-200 text-slate-700 dark:bg-slate-700 dark:text-slate-200' },
}
const SEVERITY: Record<BugSeverity, string> = { MINOR: 'Ringan', MAJOR: 'Sedang', BLOCKER: 'Berat' }

const field =
  'rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 outline-none focus:border-indigo-500 focus:ring-2 focus:ring-indigo-500/20 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-100'

// Laporan bug. Pengguna biasa melihat laporan MILIKNYA sendiri; SUPERADMIN melihat semua tenant dan menangani
// (ubah status + catatan). Pemisahan ditegakkan server — halaman ini hanya menampilkan apa yang diizinkan API.
export function BugReportsPage() {
  const { user } = useAuth()
  const isSuper = !!user?.roles.includes('SUPERADMIN') && !user.impersonator_id
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(25)
  const [status, setStatus] = useState('')
  const [tenantId, setTenantId] = useState<number | null>(null)
  const [openId, setOpenId] = useState<string | null>(null)
  const q = useBugReports({ page, pageSize, status, tenantId: isSuper ? tenantId : null })
  const tenants = useTenants(isSuper)

  return (
    <div className="px-6 py-6">
      <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold text-slate-900 dark:text-slate-100">Laporan bug</h1>
          <p className="mt-1 text-sm text-slate-600 dark:text-slate-400">
            {isSuper ? 'Semua laporan dari seluruh tenant. Setiap laporan juga dikirim ke email operator.' : 'Laporan yang Anda kirim lewat tombol bug di bilah atas.'}
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
          <select aria-label="Filter status" value={status} onChange={(e) => { setStatus(e.target.value); setPage(1) }} className={field}>
            <option value="">Semua status</option>
            {(Object.keys(STATUS) as BugStatus[]).map((s) => (
              <option key={s} value={s}>{STATUS[s].label}</option>
            ))}
          </select>
        </div>
      </div>

      {q.isLoading ? (
        <PageSpinner />
      ) : (q.data?.data.length ?? 0) === 0 ? (
        <EmptyState icon={Bug} title="Belum ada laporan" description="Gunakan tombol bug di bilah atas bila menemukan masalah." />
      ) : (
        <div className="overflow-hidden rounded-3xl border border-slate-200 bg-white shadow-sm dark:border-slate-800 dark:bg-slate-900">
          <div className="table-scroll">
            <table className="w-full text-left text-sm">
              <thead className="text-xs uppercase text-slate-500 dark:text-slate-400">
                <tr>
                  <th scope="col" className="px-4 py-3">Waktu</th>
                  <th scope="col" className="px-4 py-3">Judul</th>
                  {isSuper && <th scope="col" className="px-4 py-3">Pelapor</th>}
                  <th scope="col" className="px-4 py-3">Dampak</th>
                  <th scope="col" className="px-4 py-3">Status</th>
                  <th scope="col" className="px-4 py-3 text-right">Aksi</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
                {q.data?.data.map((b) => (
                  <tr key={b.id}>
                    <td className="whitespace-nowrap px-4 py-3 text-slate-600 dark:text-slate-400">{formatDateTime(b.created_at)}</td>
                    <td className="max-w-xs truncate px-4 py-3 font-medium text-slate-900 dark:text-slate-100" title={b.title}>{b.title}</td>
                    {isSuper && (
                      <td className="px-4 py-3 text-slate-700 dark:text-slate-300">
                        {b.reporter}
                        <span className="block text-xs text-slate-500 dark:text-slate-400">{b.tenant_name ?? 'Platform'}</span>
                      </td>
                    )}
                    <td className="px-4 py-3">{SEVERITY[b.severity] ?? b.severity}</td>
                    <td className="px-4 py-3">
                      <span className={`inline-flex rounded-full px-2.5 py-0.5 text-xs font-medium ${STATUS[b.status]?.cls ?? ''}`}>{STATUS[b.status]?.label ?? b.status}</span>
                    </td>
                    <td className="px-4 py-3 text-right">
                      <button type="button" onClick={() => setOpenId(b.id)} className="rounded-full px-3 py-1.5 text-sm font-medium text-indigo-700 hover:bg-indigo-50 dark:text-indigo-300 dark:hover:bg-indigo-500/10">
                        Detail
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <Pager page={page} pageSize={pageSize} total={q.data?.total ?? 0} onPage={setPage} onPageSize={(n) => { setPageSize(n); setPage(1) }} label="laporan bug" />
        </div>
      )}
      {openId && <BugDetailModal id={openId} canManage={isSuper} onClose={() => setOpenId(null)} />}
    </div>
  )
}

// Tangkapan layar dimuat lewat endpoint terautentikasi (hanya pelapor & SUPERADMIN); klik untuk membuka ukuran penuh.
function ScreenshotView({ id }: { id: string }) {
  const { data: url, isLoading, error } = useBugScreenshot(id, true)
  if (isLoading) return <p className="text-xs text-slate-500 dark:text-slate-400">Memuat tangkapan layar…</p>
  if (error || !url) return <p role="alert" className="text-xs text-red-600 dark:text-red-400">Tangkapan layar tidak dapat dimuat.</p>
  return (
    <figure>
      <figcaption className="mb-1 text-xs font-semibold uppercase tracking-wide text-slate-600 dark:text-slate-400">Tangkapan layar</figcaption>
      <a href={url} target="_blank" rel="noopener noreferrer" title="Buka ukuran penuh di tab baru">
        <img src={url} alt="Tangkapan layar yang dilampirkan pelapor" className="max-h-80 w-full rounded-2xl border border-slate-200 object-contain dark:border-slate-700" />
      </a>
    </figure>
  )
}

function BugDetailModal({ id, canManage, onClose }: { id: string; canManage: boolean; onClose: () => void }) {
  const toast = useToast()
  const { data, isLoading, error } = useBugReport(id)
  const update = useUpdateBugReport()
  const [status, setStatus] = useState<BugStatus | null>(null)
  const [note, setNote] = useState<string | null>(null)
  const [err, setErr] = useState<string | null>(null)

  function save(e: FormEvent, b: BugReport) {
    e.preventDefault()
    setErr(null)
    update.mutate(
      { id, status: status ?? b.status, note: (note ?? b.admin_note ?? '').trim() || null },
      {
        onSuccess: () => {
          toast.success('Status laporan diperbarui.', 'Laporan bug')
          onClose()
        },
        onError: (er) => setErr(er instanceof ApiError ? er.message : 'Gagal menyimpan'),
      },
    )
  }

  return (
    <Modal title="Detail laporan bug" onClose={onClose} wide dismissable={!update.isPending}>
      {isLoading ? (
        <PageSpinner />
      ) : error || !data ? (
        <p role="alert" className="text-sm text-red-600 dark:text-red-400">Laporan tidak ditemukan.</p>
      ) : (
        <form onSubmit={(e) => save(e, data)} className="space-y-3 text-sm">
          <h3 className="text-base font-semibold text-slate-900 dark:text-slate-100">{data.title}</h3>
          <dl className="grid grid-cols-2 gap-x-4 gap-y-2 text-xs text-slate-600 dark:text-slate-400">
            <div><dt className="inline">Dampak: </dt><dd className="inline text-slate-800 dark:text-slate-200">{SEVERITY[data.severity]}</dd></div>
            <div><dt className="inline">Dikirim: </dt><dd className="inline text-slate-800 dark:text-slate-200">{formatDateTime(data.created_at)}</dd></div>
            <div><dt className="inline">Halaman: </dt><dd className="inline font-mono text-slate-800 dark:text-slate-200">{data.page_path || '-'}</dd></div>
            <div><dt className="inline">Versi: </dt><dd className="inline text-slate-800 dark:text-slate-200">{data.app_version || '-'}</dd></div>
            {canManage && <div><dt className="inline">Pelapor: </dt><dd className="inline text-slate-800 dark:text-slate-200">{data.reporter} ({data.reporter_email})</dd></div>}
            {canManage && <div><dt className="inline">Tenant: </dt><dd className="inline text-slate-800 dark:text-slate-200">{data.tenant_name ?? 'Platform'}</dd></div>}
            <div className="col-span-2"><dt className="inline">Browser: </dt><dd className="inline break-all text-slate-800 dark:text-slate-200">{data.user_agent || '-'}</dd></div>
            <div className="col-span-2 flex items-center gap-1"><Mail className="h-3.5 w-3.5" aria-hidden="true" /> {data.emailed_at ? `Email ke operator terkirim ${formatDateTime(data.emailed_at)}` : 'Email ke operator tidak terkirim (SMTP nonaktif); laporan tetap tersimpan.'}</div>
          </dl>
          <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
            <section aria-label="Apa yang terjadi" className="rounded-2xl bg-slate-50 p-3 dark:bg-slate-800/50">
              <h4 className="mb-1 text-xs font-semibold uppercase tracking-wide text-red-700 dark:text-red-300">Apa yang terjadi</h4>
              <p className="whitespace-pre-wrap text-slate-800 dark:text-slate-200">{data.description}</p>
            </section>
            <section aria-label="Apa yang diharapkan" className="rounded-2xl bg-slate-50 p-3 dark:bg-slate-800/50">
              <h4 className="mb-1 text-xs font-semibold uppercase tracking-wide text-emerald-700 dark:text-emerald-300">Apa yang diharapkan</h4>
              <p className="whitespace-pre-wrap text-slate-800 dark:text-slate-200">{data.expected || <span className="text-slate-500 dark:text-slate-400">(tidak diisi — laporan lama)</span>}</p>
            </section>
          </div>
          {data.has_screenshot && <ScreenshotView id={id} />}
          {!canManage && data.admin_note && <p className="text-slate-700 dark:text-slate-300"><strong>Catatan penanganan:</strong> {data.admin_note}</p>}
          {canManage && (
            <>
              <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
                <div>
                  <label htmlFor="bug-status" className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Status</label>
                  <select id="bug-status" value={status ?? data.status} onChange={(e) => setStatus(e.target.value as BugStatus)} className={`${field} w-full`}>
                    {(Object.keys(STATUS) as BugStatus[]).map((s) => (
                      <option key={s} value={s}>{STATUS[s].label}</option>
                    ))}
                  </select>
                </div>
                <div className="sm:col-span-2">
                  <label htmlFor="bug-note" className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Catatan penanganan</label>
                  <input id="bug-note" maxLength={500} autoComplete="off" value={note ?? data.admin_note ?? ''} onChange={(e) => setNote(e.target.value)} className={`${field} w-full`} />
                </div>
              </div>
              {err && <p role="alert" className="text-sm text-red-600 dark:text-red-400">{err}</p>}
              <div className="flex justify-end gap-2">
                <button type="button" onClick={onClose} className="rounded-full border border-slate-300 px-3 py-2 text-sm text-slate-700 hover:bg-slate-50 dark:border-slate-700 dark:text-slate-300 dark:hover:bg-slate-800">Tutup</button>
                <button type="submit" disabled={update.isPending} className="rounded-full bg-indigo-600 px-4 py-2 text-sm font-semibold text-white hover:bg-indigo-500 disabled:opacity-50">{update.isPending ? 'Menyimpan…' : 'Simpan'}</button>
              </div>
            </>
          )}
        </form>
      )}
    </Modal>
  )
}
