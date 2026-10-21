import { useEffect, useMemo, useState, type FormEvent } from 'react'
import { useSearchParams } from 'react-router-dom'
import { AlertTriangle, CalendarClock, CheckCircle2, CreditCard, ExternalLink, Info, Pencil, Plus, RefreshCw, Router, XCircle } from 'lucide-react'
import { Modal } from '../components/Modal'
import { PageSpinner } from '../components/Spinner'
import { EmptyState } from '../components/EmptyState'
import { StatCard } from '../components/StatCard'
import { ApiError } from '../lib/api'
import { useAuth } from '../lib/auth'
import { formatDateTime } from '../lib/format'
import { useToast } from '../lib/toast'
import {
  useBillingCheckout,
  useBillingInvoices,
  useBillingPlans,
  useBillingSubscription,
  useCreateBillingPlan,
  useDeleteBillingPlan,
  useSyncInvoice,
  useTenants,
  useUpdateBillingPlan,
  type BillingInvoice,
  type BillingPlan,
  type BillingPlanInput,
  type InvoiceStatus,
} from '../lib/hooks'

// Batas masukan form paket -- sama dengan usecase/billing (validasi server tetap berlaku).
const PLAN_LIMITS = { code: 32, name: 64, description: 255, maxDevices: 10_000_000, price: 1_000_000_000, period: 366, sort: 100000 } as const
const PAGE_SIZE = 10

const idr = new Intl.NumberFormat('id-ID', { style: 'currency', currency: 'IDR', maximumFractionDigits: 0 })

const inputCls =
  'block w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 outline-none focus:border-indigo-500 focus:ring-2 focus:ring-indigo-500/20 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-100'
const labelCls = 'mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400'
const cardCls = 'rounded-3xl border border-slate-200 bg-white p-5 shadow-sm dark:border-slate-800 dark:bg-slate-900'

const STATUS_STYLE: Record<InvoiceStatus, { label: string; cls: string }> = {
  PENDING: { label: 'Menunggu pembayaran', cls: 'bg-amber-100 text-amber-800 dark:bg-amber-500/15 dark:text-amber-300' },
  PAID: { label: 'Lunas', cls: 'bg-emerald-100 text-emerald-800 dark:bg-emerald-500/15 dark:text-emerald-300' },
  EXPIRED: { label: 'Kedaluwarsa', cls: 'bg-slate-200 text-slate-700 dark:bg-slate-700 dark:text-slate-200' },
  FAILED: { label: 'Gagal', cls: 'bg-red-100 text-red-800 dark:bg-red-500/15 dark:text-red-300' },
}

function StatusChip({ status }: { status: InvoiceStatus }) {
  const s = STATUS_STYLE[status] ?? { label: status, cls: 'bg-slate-200 text-slate-700' }
  return <span className={`inline-flex rounded-full px-2.5 py-0.5 text-xs font-medium ${s.cls}`}>{s.label}</span>
}

// Hanya buka halaman bayar berskema https (payment_url berasal dari server/Xendit; ini lapisan pertahanan tambahan).
function safePaymentURL(raw: string | null | undefined): string | null {
  if (!raw) return null
  try {
    const u = new URL(raw)
    return u.protocol === 'https:' ? u.toString() : null
  } catch {
    return null
  }
}

function errMsg(err: unknown, fallback: string): string {
  return err instanceof ApiError ? err.message : fallback
}

// Langganan SaaS: status paket, pilih/ubah paket (checkout Xendit), riwayat invoice. SUPERADMIN: pilih tenant + kelola paket.
export function BillingPage() {
  const { user } = useAuth()
  const toast = useToast()
  const [params, setParams] = useSearchParams()
  const isSuper = !!user?.roles.includes('SUPERADMIN') && !user.impersonator_id
  const canManage = !!user?.roles.some((r) => r === 'ADMIN' || r === 'SUPERADMIN')

  const tenantParam = Number(params.get('tenant_id'))
  const tenantId = isSuper && Number.isInteger(tenantParam) && tenantParam > 0 ? tenantParam : null
  const returnStatus = params.get('status')

  const tenants = useTenants(isSuper)
  const sub = useBillingSubscription(tenantId, !!user && (!isSuper || tenantId !== null))
  const plans = useBillingPlans()
  const [page, setPage] = useState(1)
  const invoices = useBillingInvoices({ tenantId, page, pageSize: PAGE_SIZE, enabled: canManage && (!isSuper || tenantId !== null) })
  const checkout = useBillingCheckout()
  const sync = useSyncInvoice()
  const [planModal, setPlanModal] = useState<BillingPlan | 'new' | null>(null)
  // Paket nonaktif (mis. yang sudah ditutup penjualannya) disembunyikan; SUPERADMIN dapat menampilkannya untuk dikelola.
  const [showInactive, setShowInactive] = useState(false)

  const billingOn = sub.data?.enabled ?? false
  const pending = sub.data?.pending_invoice ?? null

  // Kembali dari Xendit (?status=success|failed): sinkronkan invoice tertunda sekali agar status terbaru tampil.
  const pendingId = pending?.id
  const [autoSynced, setAutoSynced] = useState(false)
  useEffect(() => {
    if (!autoSynced && returnStatus && pendingId && billingOn) {
      setAutoSynced(true)
      sync.mutate(pendingId)
    }
  }, [autoSynced, returnStatus, pendingId, billingOn, sync])

  function dismissReturn() {
    const next = new URLSearchParams(params)
    next.delete('status')
    setParams(next, { replace: true })
  }

  function startCheckout(plan: BillingPlan) {
    checkout.mutate(
      { plan_code: plan.code, ...(tenantId ? { tenant_id: tenantId } : {}) },
      {
        onSuccess: (res) => {
          const url = safePaymentURL(res.payment_url)
          if (url) {
            window.location.assign(url)
          } else {
            toast.error('Invoice dibuat, tetapi tautan pembayaran tidak tersedia. Coba lagi dari daftar invoice.', 'Pembayaran')
          }
        },
        onError: (err) => toast.error(errMsg(err, 'Gagal membuat invoice'), 'Checkout'),
      },
    )
  }

  function syncInvoice(inv: BillingInvoice) {
    sync.mutate(inv.id, {
      onSuccess: (r) =>
        r.status === 'PAID'
          ? toast.success('Pembayaran terkonfirmasi — langganan diperpanjang.', 'Lunas')
          : toast.success(`Status invoice: ${STATUS_STYLE[r.status]?.label ?? r.status}.`, 'Diperiksa'),
      onError: (err) => toast.error(errMsg(err, 'Gagal memeriksa status'), 'Periksa status'),
    })
  }

  const totalPages = Math.max(1, Math.ceil((invoices.data?.total ?? 0) / PAGE_SIZE))
  const allPlans = useMemo(() => plans.data ?? [], [plans.data])
  const inactiveCount = allPlans.filter((p) => !p.is_active).length
  const visiblePlans = useMemo(() => (showInactive ? allPlans : allPlans.filter((p) => p.is_active)), [allPlans, showInactive])
  const currentCode = sub.data?.plan?.code

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold text-slate-900 dark:text-slate-100">Langganan & Tagihan</h1>
          <p className="mt-1 text-sm text-slate-600 dark:text-slate-400">Paket layanan, masa aktif, dan riwayat pembayaran tenant.</p>
        </div>
        {isSuper && (
          <div className="flex items-center gap-2">
            <label htmlFor="billing-tenant" className="text-sm text-slate-600 dark:text-slate-400">Tenant</label>
            <select
              id="billing-tenant"
              value={tenantId ?? ''}
              onChange={(e) => {
                const next = new URLSearchParams()
                if (e.target.value) next.set('tenant_id', e.target.value)
                setPage(1)
                setParams(next, { replace: true })
              }}
              className={`${inputCls} w-full sm:w-64`}
            >
              <option value="">— Pilih tenant —</option>
              {(tenants.data?.data ?? []).map((t) => (
                <option key={t.id} value={t.id}>{t.name} ({t.code})</option>
              ))}
            </select>
          </div>
        )}
      </div>

      {returnStatus && (returnStatus === 'success' || returnStatus === 'failed') && (
        <div
          role="status"
          className={`flex items-start justify-between gap-3 rounded-2xl border px-4 py-3 text-sm ${
            returnStatus === 'success'
              ? 'border-emerald-200 bg-emerald-50 text-emerald-900 dark:border-emerald-500/30 dark:bg-emerald-500/10 dark:text-emerald-200'
              : 'border-red-200 bg-red-50 text-red-900 dark:border-red-500/30 dark:bg-red-500/10 dark:text-red-200'
          }`}
        >
          <span className="flex items-start gap-2">
            {returnStatus === 'success' ? <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0" /> : <XCircle className="mt-0.5 h-4 w-4 shrink-0" />}
            {returnStatus === 'success'
              ? 'Terima kasih. Kami memeriksa pembayaran Anda — status langganan diperbarui otomatis begitu pembayaran terkonfirmasi.'
              : 'Pembayaran belum selesai. Anda dapat melanjutkan dari invoice yang tertunda atau memilih paket lagi.'}
          </span>
          <button type="button" onClick={dismissReturn} className="rounded-full px-2 text-xs font-medium underline">Tutup</button>
        </div>
      )}

      {isSuper && tenantId === null ? (
        <EmptyState
          icon={CreditCard}
          title="Pilih tenant"
          description="Pilih tenant di atas untuk melihat langganan dan invoice-nya, atau kelola katalog paket di bagian bawah."
        />
      ) : sub.isLoading ? (
        <PageSpinner />
      ) : sub.isError ? (
        <div role="alert" className="rounded-2xl border border-red-200 bg-red-50 p-4 text-sm text-red-800 dark:border-red-500/30 dark:bg-red-500/10 dark:text-red-200">
          {errMsg(sub.error, 'Gagal memuat langganan')}
        </div>
      ) : (
        sub.data && (
          <>
            {!billingOn && (
              <div className="flex items-start gap-2 rounded-2xl border border-slate-200 bg-slate-50 px-4 py-3 text-sm text-slate-700 dark:border-slate-700 dark:bg-slate-800/50 dark:text-slate-300">
                <Info className="mt-0.5 h-4 w-4 shrink-0" />
                Pembayaran online belum diaktifkan oleh administrator platform. Paket dan masa aktif tetap ditampilkan; hubungi administrator untuk perubahan paket.
              </div>
            )}

            <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
              <StatCard
                label="Paket saat ini"
                value={sub.data.plan?.name ?? 'Belum ada'}
                icon={CreditCard}
                tone={sub.data.status === 'ACTIVE' ? 'emerald' : sub.data.status === 'EXPIRED' ? 'red' : 'default'}
                hint={sub.data.status === 'ACTIVE' ? 'Aktif' : sub.data.status === 'EXPIRED' ? 'Masa aktif berakhir' : 'Belum berlangganan'}
              />
              <StatCard
                label="Masa aktif"
                value={sub.data.days_left !== null ? `${Math.max(sub.data.days_left, 0)} hari` : '-'}
                icon={CalendarClock}
                tone={sub.data.days_left !== null && sub.data.days_left <= 7 ? 'amber' : 'blue'}
                hint={sub.data.expires_at ? `Berakhir ${formatDateTime(sub.data.expires_at)}` : undefined}
              />
              <StatCard
                label="Batas perangkat"
                value={sub.data.max_devices === null ? 'Tak terbatas' : sub.data.max_devices.toLocaleString('id-ID')}
                icon={Router}
                tone="default"
              />
            </div>

            {pending && (
              <div className="flex flex-wrap items-center justify-between gap-3 rounded-2xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-900 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-200">
                <span className="flex items-start gap-2">
                  <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
                  Invoice {pending.number} ({pending.plan_name}, {idr.format(pending.amount_idr)}) menunggu pembayaran hingga {formatDateTime(pending.expires_at)}.
                </span>
                <span className="flex gap-2">
                  {safePaymentURL(pending.payment_url) && (
                    <a
                      href={safePaymentURL(pending.payment_url) ?? undefined}
                      rel="noopener noreferrer"
                      className="inline-flex items-center gap-1 rounded-full bg-indigo-600 px-4 py-2 text-sm font-semibold text-white hover:bg-indigo-500"
                    >
                      Bayar sekarang <ExternalLink className="h-3.5 w-3.5" />
                    </a>
                  )}
                  {billingOn && canManage && (
                    <button
                      type="button"
                      onClick={() => syncInvoice(pending)}
                      disabled={sync.isPending}
                      className="inline-flex items-center gap-1 rounded-full border border-slate-300 bg-white px-4 py-2 text-sm font-medium text-slate-700 hover:bg-slate-50 disabled:opacity-50 dark:border-slate-600 dark:bg-slate-900 dark:text-slate-200"
                    >
                      <RefreshCw className={`h-3.5 w-3.5 ${sync.isPending ? 'animate-spin' : ''}`} /> Periksa status
                    </button>
                  )}
                </span>
              </div>
            )}
          </>
        )
      )}

      <section aria-labelledby="plans-h" className="space-y-3">
        <div className="flex items-center justify-between">
          <h2 id="plans-h" className="text-base font-semibold text-slate-900 dark:text-slate-100">Pilih paket</h2>
          {isSuper && (
            <div className="flex flex-wrap items-center gap-3">
              {inactiveCount > 0 && (
                <label className="flex items-center gap-2 text-sm text-slate-600 dark:text-slate-400">
                  <input type="checkbox" checked={showInactive} onChange={(e) => setShowInactive(e.target.checked)} />
                  Tampilkan nonaktif ({inactiveCount})
                </label>
              )}
              <button
                type="button"
                onClick={() => setPlanModal('new')}
                className="inline-flex items-center gap-1 rounded-full bg-indigo-600 px-4 py-2 text-sm font-semibold text-white hover:bg-indigo-500"
              >
                <Plus className="h-4 w-4" /> Paket baru
              </button>
            </div>
          )}
        </div>
        {plans.isLoading ? (
          <PageSpinner />
        ) : visiblePlans.length === 0 ? (
          <EmptyState icon={CreditCard} title="Belum ada paket" description="Katalog paket kosong." />
        ) : (
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {visiblePlans.map((p) => {
              const isCurrent = p.code === currentCode && sub.data?.status === 'ACTIVE'
              const canBuy = canManage && billingOn && p.is_active && p.price_idr > 0 && (!isSuper || tenantId !== null)
              // Alasan tombol beli nonaktif, ditampilkan terlihat (bukan hanya tooltip).
              const buyBlocked =
                !canManage || canBuy
                  ? null
                  : !p.is_active
                    ? 'Paket nonaktif — tidak dapat dibeli.'
                    : isSuper && tenantId === null
                      ? 'Pilih tenant di atas dulu.'
                      : !billingOn && sub.data
                        ? 'Pembayaran online belum diaktifkan.'
                        : p.price_idr === 0
                          ? 'Paket gratis — tidak perlu dibeli.'
                          : null
              return (
                <div key={p.code} className={`${cardCls} flex flex-col ${isCurrent ? 'ring-2 ring-indigo-500' : ''}`}>
                  <div className="flex items-start justify-between gap-2">
                    <h3 className="text-base font-semibold text-slate-900 dark:text-slate-100">{p.name}</h3>
                    {isCurrent && <span className="rounded-full bg-indigo-100 px-2 py-0.5 text-xs font-medium text-indigo-800 dark:bg-indigo-500/15 dark:text-indigo-300">Aktif</span>}
                    {!p.is_active && <span className="rounded-full bg-slate-200 px-2 py-0.5 text-xs font-medium text-slate-700 dark:bg-slate-700 dark:text-slate-200">Nonaktif</span>}
                  </div>
                  <p className="mt-2 text-2xl font-semibold text-slate-900 dark:text-slate-100">
                    {p.price_idr === 0 ? 'Gratis' : idr.format(p.price_idr)}
                    <span className="text-sm font-normal text-slate-500 dark:text-slate-400"> / {p.period_days} hari</span>
                  </p>
                  <p className="mt-2 text-sm text-slate-600 dark:text-slate-400">
                    {p.max_devices === null ? 'Perangkat tak terbatas' : `Hingga ${p.max_devices.toLocaleString('id-ID')} perangkat`}
                  </p>
                  {p.description && <p className="mt-1 text-sm text-slate-500 dark:text-slate-400">{p.description}</p>}
                  {buyBlocked && <p className="mt-auto pt-3 text-xs text-slate-500 dark:text-slate-400">{buyBlocked}</p>}
                  <div className={`${buyBlocked ? '' : 'mt-auto '}flex gap-2 pt-2`}>
                    {canManage && (
                      <button
                        type="button"
                        disabled={!canBuy || checkout.isPending}
                        onClick={() => startCheckout(p)}
                        title={!billingOn ? 'Pembayaran online belum diaktifkan' : undefined}
                        className="flex-1 rounded-full bg-indigo-600 px-4 py-2 text-sm font-semibold text-white hover:bg-indigo-500 disabled:cursor-not-allowed disabled:opacity-50"
                      >
                        {checkout.isPending && checkout.variables?.plan_code === p.code ? 'Menyiapkan…' : isCurrent ? 'Perpanjang' : 'Pilih paket'}
                      </button>
                    )}
                    {isSuper && (
                      <button
                        type="button"
                        onClick={() => setPlanModal(p)}
                        aria-label={`Ubah paket ${p.name}`}
                        className="rounded-full border border-slate-300 p-2 text-slate-600 hover:bg-slate-50 dark:border-slate-700 dark:text-slate-300 dark:hover:bg-slate-800"
                      >
                        <Pencil className="h-4 w-4" />
                      </button>
                    )}
                  </div>
                </div>
              )
            })}
          </div>
        )}
      </section>

      {canManage && (!isSuper || tenantId !== null) && (
        <section aria-labelledby="inv-h" className="space-y-3">
          <h2 id="inv-h" className="text-base font-semibold text-slate-900 dark:text-slate-100">Riwayat invoice</h2>
          {invoices.isLoading ? (
            <PageSpinner />
          ) : (invoices.data?.data.length ?? 0) === 0 ? (
            <EmptyState icon={CreditCard} title="Belum ada invoice" description="Invoice muncul di sini setelah Anda memilih paket." />
          ) : (
            <div className={`${cardCls} p-0`}>
              <div className="table-scroll">
              <table className="w-full text-left text-sm">
                <thead className="border-b border-slate-200 text-xs uppercase text-slate-500 dark:border-slate-800 dark:text-slate-400">
                  <tr>
                    <th scope="col" className="px-4 py-3">Nomor</th>
                    <th scope="col" className="px-4 py-3">Paket</th>
                    <th scope="col" className="px-4 py-3">Jumlah</th>
                    <th scope="col" className="px-4 py-3">Status</th>
                    <th scope="col" className="px-4 py-3">Dibuat</th>
                    <th scope="col" className="px-4 py-3 text-right">Aksi</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
                  {invoices.data?.data.map((inv) => (
                    <tr key={inv.id}>
                      <td className="px-4 py-3 font-mono text-xs">{inv.number}</td>
                      <td className="px-4 py-3">{inv.plan_name}</td>
                      <td className="px-4 py-3">{idr.format(inv.amount_idr)}</td>
                      <td className="px-4 py-3"><StatusChip status={inv.status} /></td>
                      <td className="px-4 py-3 text-slate-600 dark:text-slate-400">{formatDateTime(inv.created_at)}</td>
                      <td className="px-4 py-3 text-right">
                        {inv.status === 'PENDING' && (
                          <div className="flex justify-end gap-2">
                            {safePaymentURL(inv.payment_url) && (
                              <a href={safePaymentURL(inv.payment_url) ?? undefined} rel="noopener noreferrer" className="rounded-full px-3 py-1.5 text-sm font-medium text-indigo-700 hover:bg-indigo-50 dark:text-indigo-300 dark:hover:bg-indigo-500/10">
                                Bayar
                              </a>
                            )}
                            {billingOn && (
                              <button
                                type="button"
                                disabled={sync.isPending}
                                onClick={() => syncInvoice(inv)}
                                className="rounded-full px-3 py-1.5 text-sm font-medium text-slate-700 hover:bg-slate-100 disabled:opacity-50 dark:text-slate-300 dark:hover:bg-slate-800"
                              >
                                Periksa status
                              </button>
                            )}
                          </div>
                        )}
                        {inv.status === 'PAID' && <span className="text-xs text-slate-500 dark:text-slate-400">{formatDateTime(inv.paid_at)}</span>}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
              </div>
              {totalPages > 1 && (
                <div className="flex items-center justify-between border-t border-slate-200 px-4 py-3 text-sm dark:border-slate-800">
                  <button type="button" disabled={page <= 1} onClick={() => setPage((p) => p - 1)} className="rounded-full px-3 py-1.5 font-medium text-slate-700 hover:bg-slate-100 disabled:opacity-40 dark:text-slate-300 dark:hover:bg-slate-800">Sebelumnya</button>
                  <span className="text-slate-600 dark:text-slate-400">Halaman {page} dari {totalPages}</span>
                  <button type="button" disabled={page >= totalPages} onClick={() => setPage((p) => p + 1)} className="rounded-full px-3 py-1.5 font-medium text-slate-700 hover:bg-slate-100 disabled:opacity-40 dark:text-slate-300 dark:hover:bg-slate-800">Berikutnya</button>
                </div>
              )}
            </div>
          )}
        </section>
      )}

      {planModal && <PlanModal plan={planModal === 'new' ? null : planModal} onClose={() => setPlanModal(null)} />}
    </div>
  )
}

// Buat / ubah paket (SUPERADMIN). Harga hanya diatur di sini -- klien checkout tidak pernah mengirim nominal.
function PlanModal({ plan, onClose }: { plan: BillingPlan | null; onClose: () => void }) {
  const toast = useToast()
  const create = useCreateBillingPlan()
  const update = useUpdateBillingPlan()
  const remove = useDeleteBillingPlan()
  const [confirmDelete, setConfirmDelete] = useState(false)
  const [code, setCode] = useState(plan?.code ?? '')
  const [name, setName] = useState(plan?.name ?? '')
  const [description, setDescription] = useState(plan?.description ?? '')
  const [unlimited, setUnlimited] = useState(plan ? plan.max_devices === null : false)
  const [maxDevices, setMaxDevices] = useState(String(plan?.max_devices ?? 100))
  const [price, setPrice] = useState(String(plan?.price_idr ?? 0))
  const [period, setPeriod] = useState(String(plan?.period_days ?? 30))
  const [sortOrder, setSortOrder] = useState(String(plan?.sort_order ?? 0))
  const [active, setActive] = useState(plan?.is_active ?? true)
  const [error, setError] = useState<string | null>(null)
  const pending = create.isPending || update.isPending || remove.isPending

  function submit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    const cleanCode = code.trim().toUpperCase()
    const body: BillingPlanInput = {
      name: name.trim(),
      description: description.trim(),
      max_devices: unlimited ? null : Number(maxDevices),
      price_idr: Number(price),
      period_days: Number(period),
      sort_order: Number(sortOrder),
      is_active: active,
    }
    if (!plan && !/^[A-Z][A-Z0-9_]{1,31}$/.test(cleanCode)) return setError('Kode paket 2–32 karakter: huruf kapital, angka, garis bawah; diawali huruf.')
    if (body.name!.length < 2) return setError('Nama paket minimal 2 karakter.')
    if (!unlimited && !(Number.isInteger(body.max_devices) && body.max_devices! >= 1 && body.max_devices! <= PLAN_LIMITS.maxDevices)) return setError('Batas perangkat harus bilangan bulat 1–10.000.000, atau pilih tak terbatas.')
    if (!Number.isInteger(body.price_idr) || body.price_idr! < 0 || body.price_idr! > PLAN_LIMITS.price) return setError('Harga harus bilangan bulat rupiah 0–1.000.000.000.')
    if (!Number.isInteger(body.period_days) || body.period_days! < 1 || body.period_days! > PLAN_LIMITS.period) return setError('Masa berlaku 1–366 hari.')
    if (!Number.isInteger(body.sort_order) || Math.abs(body.sort_order!) > PLAN_LIMITS.sort) return setError('Urutan harus bilangan bulat −100000 sampai 100000.')
    const done = {
      onSuccess: () => {
        toast.success(plan ? 'Paket diperbarui.' : 'Paket dibuat.', 'Berhasil')
        onClose()
      },
      onError: (err: unknown) => setError(errMsg(err, 'Gagal menyimpan paket')),
    }
    if (plan) update.mutate({ code: plan.code, ...body }, done)
    else create.mutate({ code: cleanCode, ...body }, done)
  }

  return (
    <Modal title={plan ? `Ubah paket ${plan.name}` : 'Paket baru'} onClose={onClose} dismissable={!pending}>
      <form onSubmit={submit} className="space-y-3">
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <div>
            <label htmlFor="plan-code" className={labelCls}>Kode</label>
            <input id="plan-code" required disabled={!!plan} value={code} onChange={(e) => setCode(e.target.value)} maxLength={PLAN_LIMITS.code} minLength={2} autoComplete="off" placeholder="PRO" className={`${inputCls} font-mono uppercase disabled:opacity-60`} />
          </div>
          <div>
            <label htmlFor="plan-name" className={labelCls}>Nama</label>
            <input id="plan-name" required value={name} onChange={(e) => setName(e.target.value)} maxLength={PLAN_LIMITS.name} minLength={2} autoComplete="off" className={inputCls} />
          </div>
        </div>
        <div>
          <label htmlFor="plan-desc" className={labelCls}>Deskripsi</label>
          <input id="plan-desc" value={description} onChange={(e) => setDescription(e.target.value)} maxLength={PLAN_LIMITS.description} autoComplete="off" className={inputCls} />
        </div>
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <div>
            <label htmlFor="plan-price" className={labelCls}>Harga (Rp)</label>
            <input id="plan-price" type="number" required min={0} max={PLAN_LIMITS.price} step={1} value={price} onChange={(e) => setPrice(e.target.value)} className={inputCls} />
          </div>
          <div>
            <label htmlFor="plan-period" className={labelCls}>Masa berlaku (hari)</label>
            <input id="plan-period" type="number" required min={1} max={PLAN_LIMITS.period} step={1} value={period} onChange={(e) => setPeriod(e.target.value)} className={inputCls} />
          </div>
          <div>
            <label htmlFor="plan-devices" className={labelCls}>Batas perangkat</label>
            <input id="plan-devices" type="number" min={1} max={PLAN_LIMITS.maxDevices} step={1} disabled={unlimited} required={!unlimited} value={maxDevices} onChange={(e) => setMaxDevices(e.target.value)} className={`${inputCls} disabled:opacity-60`} />
            <label className="mt-1 flex items-center gap-2 text-xs text-slate-600 dark:text-slate-400">
              <input type="checkbox" checked={unlimited} onChange={(e) => setUnlimited(e.target.checked)} /> Tak terbatas
            </label>
          </div>
          <div>
            <label htmlFor="plan-sort" className={labelCls}>Urutan tampil</label>
            <input id="plan-sort" type="number" min={-PLAN_LIMITS.sort} max={PLAN_LIMITS.sort} step={1} value={sortOrder} onChange={(e) => setSortOrder(e.target.value)} className={inputCls} />
          </div>
        </div>
        <label className="flex items-center gap-2 text-sm text-slate-700 dark:text-slate-300">
          <input type="checkbox" checked={active} onChange={(e) => setActive(e.target.checked)} /> Paket aktif (dapat dibeli)
        </label>
        {error && <p role="alert" className="text-sm text-red-600 dark:text-red-400">{error}</p>}
        {plan && (
          <div className="rounded-xl border border-red-200 bg-red-50/50 p-3 dark:border-red-500/30 dark:bg-red-500/5">
            {!confirmDelete ? (
              <div className="flex flex-wrap items-center justify-between gap-2 text-sm text-slate-600 dark:text-slate-400">
                <span>Tidak dipakai lagi? Hapus dari katalog (riwayat invoice tetap ada).</span>
                <button type="button" disabled={pending} onClick={() => setConfirmDelete(true)} className="rounded-full border border-red-300 px-3 py-1.5 text-sm font-medium text-red-700 hover:bg-red-50 disabled:opacity-50 dark:border-red-500/40 dark:text-red-300 dark:hover:bg-red-500/10">
                  Hapus paket
                </button>
              </div>
            ) : (
              <div className="flex flex-wrap items-center justify-between gap-2 text-sm">
                <span className="font-medium text-red-800 dark:text-red-300">Hapus paket {plan.name} ({plan.code})? Tidak dapat dibatalkan.</span>
                <span className="flex gap-2">
                  <button type="button" disabled={pending} onClick={() => setConfirmDelete(false)} className="rounded-full border border-slate-300 px-3 py-1.5 text-sm text-slate-700 hover:bg-white dark:border-slate-700 dark:text-slate-300">Batal</button>
                  <button
                    type="button"
                    disabled={pending}
                    onClick={() =>
                      remove.mutate(plan.code, {
                        onSuccess: () => {
                          toast.success('Paket dihapus.', 'Berhasil')
                          onClose()
                        },
                        onError: (err) => {
                          setConfirmDelete(false)
                          setError(errMsg(err, 'Gagal menghapus paket'))
                        },
                      })
                    }
                    className="rounded-full bg-red-600 px-3 py-1.5 text-sm font-semibold text-white hover:bg-red-700 disabled:opacity-50"
                  >
                    {remove.isPending ? 'Menghapus…' : 'Ya, hapus'}
                  </button>
                </span>
              </div>
            )}
          </div>
        )}
        <div className="flex justify-end gap-2 pt-1">
          <button type="button" onClick={onClose} className="rounded-full border border-slate-300 px-3 py-2 text-sm text-slate-700 hover:bg-slate-50 dark:border-slate-700 dark:text-slate-300 dark:hover:bg-slate-800">Batal</button>
          <button type="submit" disabled={pending} className="rounded-full bg-indigo-600 px-4 py-2 text-sm font-semibold text-white hover:bg-indigo-500 disabled:opacity-50">{pending ? 'Menyimpan…' : 'Simpan'}</button>
        </div>
      </form>
    </Modal>
  )
}
