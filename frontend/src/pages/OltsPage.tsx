import { useMemo, useState, type FormEvent } from 'react'
import { ChevronLeft, ChevronRight, Eye, EyeOff, Pencil, PlugZap, Plus, RefreshCw, Router as RouterIcon, Trash2 } from 'lucide-react'
import { useI18n } from '../lib/i18n'
import { useAuth } from '../lib/auth'
import { useToast } from '../lib/toast'
import { useConfirm } from '../lib/confirm'
import { ApiError } from '../lib/api'
import { LIMITS } from '../lib/limits'
import { EmptyState } from '../components/EmptyState'
import { Modal } from '../components/Modal'
import { StatusBadge } from '../components/StatusBadge'
import {
  useCreateOLT,
  useDeleteOLT,
  useOLTDrivers,
  useOLTONUs,
  useOLTs,
  usePollOLT,
  useSetOLTONUIgnored,
  useTestOLT,
  useUpdateOLT,
  type OLTONUFilters,
} from '../lib/hooks'
import type { OLT, OLTInput, OLTONUStatus, OLTPollStatus } from '../lib/types'
import { formatDateTime, formatRelativeTime } from '../lib/format'

const PAGE_SIZE = 25
const inputCls =
  'w-full rounded-lg border border-slate-300 px-3 py-2 text-sm text-slate-900 outline-none transition-colors focus:border-indigo-600 focus:ring-1 focus:ring-indigo-600 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-100'
const primaryBtnCls =
  'inline-flex items-center justify-center gap-2 rounded-lg bg-indigo-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-indigo-700 disabled:cursor-not-allowed disabled:opacity-50'
const iconBtnCls =
  'rounded-md p-1.5 text-slate-500 transition-colors hover:bg-slate-100 hover:text-slate-700 disabled:cursor-not-allowed disabled:opacity-40 dark:text-slate-400 dark:hover:bg-slate-800 dark:hover:text-slate-200'

// Status poll OLT -> label dan gaya StatusBadge yang sudah ada (OK = hijau, tak terjangkau = abu, ditolak/galat = merah).
const POLL_LABEL: Record<OLTPollStatus, string> = {
  UNKNOWN: 'Belum dicoba',
  OK: 'Terhubung',
  UNREACHABLE: 'Tak terjangkau',
  AUTH_FAILED: 'Autentikasi ditolak',
  ERROR: 'Galat',
}
const POLL_BADGE: Record<OLTPollStatus, string> = { UNKNOWN: 'DECOMMISSIONED', OK: 'ONLINE', UNREACHABLE: 'OFFLINE', AUTH_FAILED: 'FAULTY', ERROR: 'FAILED' }
const ONU_LABEL: Record<OLTONUStatus, string> = { PENDING: 'Menunggu', IGNORED: 'Diabaikan', RESOLVED: 'Selesai' }
const ONU_BADGE: Record<OLTONUStatus, string> = { PENDING: 'PENDING', IGNORED: 'CANCELLED', RESOLVED: 'COMPLETED' }

const errMsg = (e: unknown, fallback: string) => (e instanceof ApiError || e instanceof Error ? e.message : fallback)

export function OltsPage() {
  const { t: tl } = useI18n()
  const { hasRole } = useAuth()
  const canManage = hasRole('ADMIN') // ADMIN (dan SUPERADMIN): tambah, ubah, hapus, uji koneksi
  const canOperate = hasRole('ADMIN', 'NOC') // polling dan abaikan ONU
  const toast = useToast()
  const confirm = useConfirm()

  const { data: oltData, isLoading: oltLoading } = useOLTs({ page_size: 100 })
  const olts = oltData?.data ?? []
  const { data: driverData } = useOLTDrivers()
  const drivers = driverData?.data ?? []

  const [form, setForm] = useState<{ olt?: OLT } | null>(null)
  const testOLT = useTestOLT()
  const pollOLT = usePollOLT()
  const deleteOLT = useDeleteOLT()
  const [busyId, setBusyId] = useState<string | null>(null)

  const [oltFilter, setOltFilter] = useState('')
  const [status, setStatus] = useState<'' | OLTONUStatus>('PENDING')
  const [search, setSearch] = useState('')
  const [page, setPage] = useState(1)
  const filters: OLTONUFilters = useMemo(
    () => ({ olt: oltFilter || undefined, status: status || undefined, search: search.trim() || undefined, page, page_size: PAGE_SIZE }),
    [oltFilter, status, search, page],
  )
  const { data: onuData, isLoading: onuLoading } = useOLTONUs(filters)
  const onus = onuData?.data ?? []
  const total = onuData?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))
  const setIgnored = useSetOLTONUIgnored()

  const handleTest = (o: OLT) => {
    setBusyId(o.id)
    testOLT.mutate(o.id, {
      onSuccess: (r) => toast.success(`Koneksi ke ${o.name} berhasil${r.reported_name ? ` (${r.reported_name})` : ''}.`),
      onError: (e) => toast.error(errMsg(e, 'Uji koneksi gagal')),
      onSettled: () => setBusyId(null),
    })
  }
  const handlePoll = (o: OLT) => {
    setBusyId(o.id)
    pollOLT.mutate(o.id, {
      onSuccess: (s) => toast.success(`Polling ${o.name} selesai: ${s.new} baru, ${s.resolved} selesai, ${s.queue_length} menunggu.`),
      onError: (e) => toast.error(errMsg(e, 'Polling gagal')),
      onSettled: () => setBusyId(null),
    })
  }
  const handleDelete = async (o: OLT) => {
    const ok = await confirm({
      title: 'Hapus OLT',
      message: `OLT "${o.name}" beserta antrean ONU-nya akan dihapus dari ACS. OLT fisiknya tidak diubah.`,
      confirmLabel: 'Hapus OLT',
      tone: 'danger',
    })
    if (!ok) return
    deleteOLT.mutate(o.id, {
      onSuccess: () => {
        toast.success('OLT dihapus.')
        if (oltFilter === o.id) setOltFilter('')
      },
      onError: (e) => toast.error(errMsg(e, 'Gagal menghapus OLT')),
    })
  }
  const handleIgnore = (id: string, ignored: boolean) =>
    setIgnored.mutate(
      { id, ignored },
      { onError: (e) => toast.error(errMsg(e, 'Gagal mengubah entri')) },
    )

  return (
    <div className="mx-auto max-w-7xl px-6 py-8">
      <div className="mb-6 flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold text-slate-900 dark:text-slate-100">{tl('nav.olts')}</h1>
          <p className="mt-0.5 max-w-3xl text-sm text-slate-500 dark:text-slate-400">
            Inventaris OLT dan antrean ONU yang dilaporkan OLT sebagai belum terdaftar. ACS hanya <strong>membaca</strong> dari OLT; belum
            ada pengesahan ONU atau penulisan konfigurasi ke OLT.
          </p>
        </div>
        {canManage && (
          <button onClick={() => setForm({})} className={primaryBtnCls}>
            <Plus className="h-4 w-4" /> Tambah OLT
          </button>
        )}
      </div>

      {canManage && drivers.length === 0 && driverData && (
        <p role="status" className="mb-4 rounded-lg border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800 dark:border-amber-900/50 dark:bg-amber-950/30 dark:text-amber-300">
          Belum ada driver OLT yang terdaftar di server, jadi OLT belum bisa ditambahkan. Driver vendor nyata menyusul; untuk uji coba
          operator dapat mengaktifkan driver simulator (<code>ACS_OLT_SIM_ENABLED</code>).
        </p>
      )}

      {/* ---- OLT ---- */}
      <div className="mb-8 overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm dark:border-slate-800 dark:bg-slate-900">
        {oltLoading ? (
          <div className="divide-y divide-slate-100 dark:divide-slate-800">
            {Array.from({ length: 3 }).map((_, i) => (
              <div key={i} className="flex items-center gap-4 px-5 py-4">
                <div className="h-4 flex-1 animate-pulse rounded bg-slate-100 dark:bg-slate-800" />
              </div>
            ))}
          </div>
        ) : olts.length === 0 ? (
          <EmptyState icon={RouterIcon} title="Belum ada OLT" description="Tambahkan OLT untuk mulai memantau ONU yang belum terdaftar." />
        ) : (
          <div className="table-scroll">
            <table className="w-full text-left text-sm">
              <thead>
                <tr className="border-b border-slate-200 bg-slate-50 text-xs font-medium uppercase tracking-wide text-slate-500 dark:border-slate-800 dark:bg-slate-800/50 dark:text-slate-400">
                  <th className="px-5 py-3">OLT</th>
                  <th className="px-5 py-3">Alamat</th>
                  <th className="px-5 py-3">Status</th>
                  <th className="px-5 py-3">Dilaporkan OLT</th>
                  <th className="px-5 py-3"></th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
                {olts.map((o) => (
                  <tr key={o.id} className="align-top">
                    <td className="px-5 py-3.5">
                      <p className="font-medium text-slate-900 dark:text-slate-100">{o.name}</p>
                      <p className="text-xs text-slate-500 dark:text-slate-400">
                        driver <span className="font-mono">{o.driver_code}</span>
                        {!o.is_active && <span className="ml-2 rounded bg-slate-100 px-1.5 py-0.5 text-slate-600 dark:bg-slate-800 dark:text-slate-300">nonaktif</span>}
                      </p>
                    </td>
                    <td className="px-5 py-3.5 font-mono text-xs text-slate-700 dark:text-slate-300">
                      {o.host}
                      <p className="text-slate-500 dark:text-slate-400">
                        SNMP {o.snmp_port}
                        {o.has_snmp_community ? ' · community tersimpan' : ''} · SSH {o.ssh_port}
                        {o.has_ssh_password ? ' · password tersimpan' : ''}
                      </p>
                    </td>
                    <td className="px-5 py-3.5">
                      <StatusBadge code={POLL_BADGE[o.poll_status]} label={POLL_LABEL[o.poll_status]} />
                      {o.last_poll_at && (
                        <p className="mt-1 text-xs text-slate-500 dark:text-slate-400" title={formatDateTime(o.last_poll_at)}>
                          {formatRelativeTime(o.last_poll_at)}
                        </p>
                      )}
                      {o.last_poll_error && <p className="mt-1 max-w-xs break-words text-xs text-red-600 dark:text-red-400">{o.last_poll_error}</p>}
                    </td>
                    <td className="px-5 py-3.5 text-xs text-slate-600 dark:text-slate-400">
                      {o.reported_name ? (
                        <>
                          <p>{o.reported_name}</p>
                          {o.reported_version && <p>{o.reported_version}</p>}
                        </>
                      ) : (
                        '-'
                      )}
                    </td>
                    <td className="px-5 py-3.5 text-right">
                      <div className="flex items-center justify-end gap-1">
                        {canOperate && (
                          <button
                            onClick={() => handlePoll(o)}
                            disabled={busyId === o.id || !o.is_active}
                            className={iconBtnCls}
                            title={o.is_active ? 'Baca ONU belum terdaftar dari OLT' : 'OLT nonaktif'}
                            aria-label={`Poll ${o.name}`}
                          >
                            <RefreshCw className={`h-4 w-4 ${busyId === o.id && pollOLT.isPending ? 'animate-spin' : ''}`} />
                          </button>
                        )}
                        {canManage && (
                          <>
                            <button onClick={() => handleTest(o)} disabled={busyId === o.id} className={iconBtnCls} title="Uji koneksi" aria-label={`Uji koneksi ${o.name}`}>
                              <PlugZap className="h-4 w-4" />
                            </button>
                            <button onClick={() => setForm({ olt: o })} className={iconBtnCls} title="Ubah" aria-label={`Ubah ${o.name}`}>
                              <Pencil className="h-4 w-4" />
                            </button>
                            <button
                              onClick={() => void handleDelete(o)}
                              className={`${iconBtnCls} hover:bg-red-50 hover:text-red-600`}
                              title="Hapus"
                              aria-label={`Hapus ${o.name}`}
                            >
                              <Trash2 className="h-4 w-4" />
                            </button>
                          </>
                        )}
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {/* ---- Antrean ONU belum terdaftar ---- */}
      <h2 className="mb-3 text-base font-semibold text-slate-900 dark:text-slate-100">ONU belum terdaftar</h2>
      <div className="mb-4 flex flex-wrap items-center gap-3">
        <select
          value={oltFilter}
          aria-label="Filter OLT"
          onChange={(e) => {
            setOltFilter(e.target.value)
            setPage(1)
          }}
          className={`max-w-[240px] ${inputCls}`}
        >
          <option value="">Semua OLT</option>
          {olts.map((o) => (
            <option key={o.id} value={o.id}>
              {o.name}
            </option>
          ))}
        </select>
        <select
          value={status}
          aria-label="Filter status"
          onChange={(e) => {
            setStatus(e.target.value as '' | OLTONUStatus)
            setPage(1)
          }}
          className={`max-w-[200px] ${inputCls}`}
        >
          <option value="">Semua status</option>
          {(Object.keys(ONU_LABEL) as OLTONUStatus[]).map((s) => (
            <option key={s} value={s}>
              {ONU_LABEL[s]}
            </option>
          ))}
        </select>
        <input
          type="search"
          maxLength={255}
          placeholder="Cari serial atau port PON…"
          aria-label="Cari ONU"
          value={search}
          onChange={(e) => {
            setSearch(e.target.value)
            setPage(1)
          }}
          className={`max-w-[260px] ${inputCls}`}
        />
      </div>

      <div className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm dark:border-slate-800 dark:bg-slate-900">
        {onuLoading ? (
          <div className="divide-y divide-slate-100 dark:divide-slate-800">
            {Array.from({ length: 4 }).map((_, i) => (
              <div key={i} className="flex items-center gap-4 px-5 py-4">
                <div className="h-4 flex-1 animate-pulse rounded bg-slate-100 dark:bg-slate-800" />
              </div>
            ))}
          </div>
        ) : onus.length === 0 ? (
          <EmptyState icon={RouterIcon} title="Tidak ada ONU" description="ONU yang dilaporkan OLT sebagai belum terdaftar muncul di sini setelah polling." />
        ) : (
          <>
            <div className="table-scroll">
              <table className="w-full text-left text-sm">
                <thead>
                  <tr className="border-b border-slate-200 bg-slate-50 text-xs font-medium uppercase tracking-wide text-slate-500 dark:border-slate-800 dark:bg-slate-800/50 dark:text-slate-400">
                    <th className="px-5 py-3">Serial</th>
                    <th className="px-5 py-3">Port PON</th>
                    <th className="px-5 py-3">Tipe</th>
                    <th className="px-5 py-3">OLT</th>
                    <th className="px-5 py-3">Status</th>
                    <th className="px-5 py-3">Terakhir terlihat</th>
                    <th className="px-5 py-3"></th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
                  {onus.map((n) => (
                    <tr key={n.id}>
                      <td className="px-5 py-3.5 font-mono text-xs text-slate-900 dark:text-slate-100">{n.serial_number}</td>
                      <td className="px-5 py-3.5 font-mono text-xs text-slate-600 dark:text-slate-400">{n.pon_port || '-'}</td>
                      <td className="px-5 py-3.5 text-xs text-slate-600 dark:text-slate-400">{n.onu_type ?? '-'}</td>
                      <td className="px-5 py-3.5 text-slate-700 dark:text-slate-300">{n.olt_name}</td>
                      <td className="px-5 py-3.5">
                        <StatusBadge code={ONU_BADGE[n.status]} label={ONU_LABEL[n.status]} />
                      </td>
                      <td className="px-5 py-3.5 text-slate-500 dark:text-slate-400" title={`Pertama: ${formatDateTime(n.first_seen_at)}`}>
                        {formatRelativeTime(n.last_seen_at)}
                      </td>
                      <td className="px-5 py-3.5 text-right">
                        {canOperate && n.status === 'PENDING' && (
                          <button onClick={() => handleIgnore(n.id, true)} className={iconBtnCls} title="Abaikan" aria-label={`Abaikan ${n.serial_number}`}>
                            <EyeOff className="h-4 w-4" />
                          </button>
                        )}
                        {canOperate && n.status === 'IGNORED' && (
                          <button onClick={() => handleIgnore(n.id, false)} className={iconBtnCls} title="Pulihkan ke antrean" aria-label={`Pulihkan ${n.serial_number}`}>
                            <Eye className="h-4 w-4" />
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
                Menampilkan {onus.length} dari {total} ONU
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

      {form && <OltFormModal olt={form.olt} drivers={drivers} onClose={() => setForm(null)} />}
    </div>
  )
}

function OltFormModal({ olt, drivers, onClose }: { olt?: OLT; drivers: string[]; onClose: () => void }) {
  const { hasRole } = useAuth()
  const toast = useToast()
  const isEdit = !!olt
  const needTenant = !isEdit && hasRole('SUPERADMIN')
  const create = useCreateOLT()
  const update = useUpdateOLT()

  const [tenantId, setTenantId] = useState('')
  const [name, setName] = useState(olt?.name ?? '')
  const [driver, setDriver] = useState(olt?.driver_code ?? drivers[0] ?? '')
  const [host, setHost] = useState(olt?.host ?? '')
  const [snmpPort, setSnmpPort] = useState(String(olt?.snmp_port ?? 161))
  const [sshPort, setSshPort] = useState(String(olt?.ssh_port ?? 22))
  const [community, setCommunity] = useState('')
  const [clearCommunity, setClearCommunity] = useState(false)
  const [sshUser, setSshUser] = useState(olt?.ssh_username ?? '')
  const [sshPass, setSshPass] = useState('')
  const [clearPass, setClearPass] = useState(false)
  const [notes, setNotes] = useState(olt?.notes ?? '')
  const [active, setActive] = useState(olt?.is_active ?? true)
  const [error, setError] = useState<string | null>(null)
  const pending = create.isPending || update.isPending

  async function submit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    const sp = Number(snmpPort)
    const hp = Number(sshPort)
    if (!name.trim() || !host.trim() || !driver) return setError('Nama, driver, dan host wajib diisi.')
    if (![sp, hp].every((p) => Number.isInteger(p) && p >= 1 && p <= 65535)) return setError('Port SNMP dan SSH harus bilangan bulat 1–65535.')
    if (needTenant && !(Number.isInteger(Number(tenantId)) && Number(tenantId) > 0)) return setError('ID tenant wajib diisi (SUPERADMIN tidak terikat tenant).')

    try {
      if (!olt) {
        const input: OLTInput = {
          name: name.trim(), driver_code: driver, host: host.trim(), snmp_port: sp, ssh_port: hp,
          ssh_username: sshUser.trim(), notes: notes.trim(), is_active: active,
        }
        if (needTenant) input.tenant_id = Number(tenantId)
        if (community) input.snmp_community = community
        if (sshPass) input.ssh_password = sshPass
        await create.mutateAsync(input)
        toast.success('OLT ditambahkan.')
      } else {
        // PATCH parsial: hanya kirim yang berubah. Kredensial kosong = tidak berubah, kecuali "hapus" dicentang.
        const input: OLTInput = {}
        if (name.trim() !== olt.name) input.name = name.trim()
        if (driver !== olt.driver_code) input.driver_code = driver
        if (host.trim() !== olt.host) input.host = host.trim()
        if (sp !== olt.snmp_port) input.snmp_port = sp
        if (hp !== olt.ssh_port) input.ssh_port = hp
        if (sshUser.trim() !== (olt.ssh_username ?? '')) input.ssh_username = sshUser.trim()
        if (notes.trim() !== (olt.notes ?? '')) input.notes = notes.trim()
        if (active !== olt.is_active) input.is_active = active
        if (clearCommunity) input.snmp_community = ''
        else if (community) input.snmp_community = community
        if (clearPass) input.ssh_password = ''
        else if (sshPass) input.ssh_password = sshPass
        if (Object.keys(input).length === 0) return onClose()
        await update.mutateAsync({ id: olt.id, input })
        toast.success('OLT diperbarui.')
      }
      onClose()
    } catch (err) {
      setError(errMsg(err, 'Gagal menyimpan OLT'))
    }
  }

  return (
    <Modal title={isEdit ? `Ubah OLT — ${olt.name}` : 'Tambah OLT'} onClose={onClose} dismissable={!pending}>
      <form onSubmit={(e) => void submit(e)} className="space-y-3">
        {needTenant && (
          <label className="block text-xs font-medium text-slate-600 dark:text-slate-400">
            ID tenant
            <input type="number" min={1} max={1_000_000_000} value={tenantId} onChange={(e) => setTenantId(e.target.value)} className={`mt-1 ${inputCls}`} />
          </label>
        )}
        <label className="block text-xs font-medium text-slate-600 dark:text-slate-400">
          Nama
          <input value={name} maxLength={LIMITS.olt.name} onChange={(e) => setName(e.target.value)} className={`mt-1 ${inputCls}`} />
        </label>
        <label className="block text-xs font-medium text-slate-600 dark:text-slate-400">
          Driver
          <select value={driver} onChange={(e) => setDriver(e.target.value)} className={`mt-1 ${inputCls}`}>
            {!drivers.includes(driver) && driver && <option value={driver}>{driver} (tidak tersedia di server)</option>}
            {drivers.map((d) => (
              <option key={d} value={d}>
                {d}
              </option>
            ))}
          </select>
        </label>
        <label className="block text-xs font-medium text-slate-600 dark:text-slate-400">
          Host (IP atau nama host)
          <input value={host} maxLength={LIMITS.olt.host} onChange={(e) => setHost(e.target.value)} placeholder="10.0.0.1" className={`mt-1 font-mono ${inputCls}`} />
        </label>
        <div className="grid grid-cols-2 gap-3">
          <label className="block text-xs font-medium text-slate-600 dark:text-slate-400">
            Port SNMP
            <input type="number" min={1} max={65535} value={snmpPort} onChange={(e) => setSnmpPort(e.target.value)} className={`mt-1 ${inputCls}`} />
          </label>
          <label className="block text-xs font-medium text-slate-600 dark:text-slate-400">
            Port SSH
            <input type="number" min={1} max={65535} value={sshPort} onChange={(e) => setSshPort(e.target.value)} className={`mt-1 ${inputCls}`} />
          </label>
        </div>
        <label className="block text-xs font-medium text-slate-600 dark:text-slate-400">
          SNMP community
          <input
            type="password"
            autoComplete="off"
            value={community}
            maxLength={LIMITS.olt.credential}
            disabled={clearCommunity}
            placeholder={olt?.has_snmp_community ? 'Tersimpan — kosongkan agar tidak berubah' : 'Opsional'}
            onChange={(e) => setCommunity(e.target.value)}
            className={`mt-1 ${inputCls}`}
          />
        </label>
        {olt?.has_snmp_community && (
          <label className="flex items-center gap-2 text-xs text-slate-600 dark:text-slate-400">
            <input type="checkbox" checked={clearCommunity} onChange={(e) => setClearCommunity(e.target.checked)} /> Hapus community tersimpan
          </label>
        )}
        <label className="block text-xs font-medium text-slate-600 dark:text-slate-400">
          Username SSH
          <input value={sshUser} maxLength={LIMITS.olt.credential} autoComplete="off" onChange={(e) => setSshUser(e.target.value)} className={`mt-1 ${inputCls}`} />
        </label>
        <label className="block text-xs font-medium text-slate-600 dark:text-slate-400">
          Password SSH
          <input
            type="password"
            autoComplete="off"
            value={sshPass}
            maxLength={LIMITS.olt.credential}
            disabled={clearPass}
            placeholder={olt?.has_ssh_password ? 'Tersimpan — kosongkan agar tidak berubah' : 'Opsional'}
            onChange={(e) => setSshPass(e.target.value)}
            className={`mt-1 ${inputCls}`}
          />
        </label>
        {olt?.has_ssh_password && (
          <label className="flex items-center gap-2 text-xs text-slate-600 dark:text-slate-400">
            <input type="checkbox" checked={clearPass} onChange={(e) => setClearPass(e.target.checked)} /> Hapus password tersimpan
          </label>
        )}
        <label className="block text-xs font-medium text-slate-600 dark:text-slate-400">
          Catatan
          <input value={notes} maxLength={LIMITS.olt.notes} onChange={(e) => setNotes(e.target.value)} className={`mt-1 ${inputCls}`} />
        </label>
        <label className="flex items-center gap-2 text-sm text-slate-700 dark:text-slate-300">
          <input type="checkbox" checked={active} onChange={(e) => setActive(e.target.checked)} /> Aktif (boleh di-poll)
        </label>
        <p className="text-xs text-slate-500 dark:text-slate-400">Kredensial disimpan terenkripsi dan tidak pernah ditampilkan kembali.</p>
        {error && (
          <p role="alert" className="text-sm text-red-600">
            {error}
          </p>
        )}
        <button type="submit" disabled={pending} className={`${primaryBtnCls} w-full`}>
          {isEdit ? 'Simpan perubahan' : 'Tambah OLT'}
        </button>
      </form>
    </Modal>
  )
}
