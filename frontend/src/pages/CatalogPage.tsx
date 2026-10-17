import { useI18n } from '../lib/i18n'
import { useState, type FormEvent } from 'react'
import { Cable, Cpu, Plus, Route, X } from 'lucide-react'
import { DeleteButton, DetailButton, DetailModal, EditButton } from '../components/RecordActions'
import { EmptyState } from '../components/EmptyState'
import { CatalogDataTools } from '../components/CatalogDataTools'
import { StatusBadge } from '../components/StatusBadge'
import { Modal } from '../components/Modal'
import { PageSpinner } from '../components/Spinner'
import { ApiError } from '../lib/api'
import { useAuth } from '../lib/auth'
import { useConfirm } from '../lib/confirm'
import { useToast } from '../lib/toast'
import { LIMITS } from '../lib/limits'
import {
  useAddVendorOUI,
  useCreateDeviceModel,
  useCreateVendor,
  useDeleteDeviceModel,
  useDeleteParameterMapping,
  useDeleteVendor,
  useDeleteVendorOUI,
  useDeviceModel,
  useDeviceModels,
  useParameterMapping,
  useParameterMappings,
  useRefs,
  useUpdateDeviceModel,
  useUpdateVendor,
  useUpsertParameterMapping,
  useVendor,
  useVendorOUIs,
  useVendors,
  type CreateDeviceModelInput,
  type UpsertMappingInput,
} from '../lib/hooks'
import type { DeviceModel, Vendor, VendorOUI } from '../lib/types'

const inputCls =
  'w-full rounded-lg border border-slate-300 px-3 py-2 text-sm text-slate-900 outline-none transition-colors focus:border-indigo-600 focus:ring-1 focus:ring-indigo-600 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-100'
const primaryBtnCls =
  'flex items-center justify-center gap-2 rounded-full bg-indigo-600 px-3 py-2 text-sm font-medium text-white transition-colors hover:bg-indigo-700 disabled:cursor-not-allowed disabled:opacity-60 dark:bg-indigo-300 dark:text-slate-900 dark:hover:bg-indigo-200'

type Tab = 'vendors' | 'models' | 'mappings'

const errMsg = (err: unknown, fallback: string) => (err instanceof ApiError ? err.message : fallback)

const secondaryBtnCls =
  'rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm font-medium text-slate-700 transition-colors hover:bg-slate-50 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-300 dark:hover:bg-slate-800'

export function CatalogPage() {
  const { t: tl } = useI18n()
  const [tab, setTab] = useState<Tab>('vendors')

  return (
    <div className="mx-auto max-w-7xl px-6 py-8">
      <div className="mb-6 flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold text-slate-900 dark:text-slate-100">{tl('nav.catalog')}</h1>
          <p className="mt-0.5 text-sm text-slate-500 dark:text-slate-400">
            Menambah vendor/model baru adalah operasi data, bukan kode (TECH.md §5.1) — kelola di sini
          </p>
        </div>
        <CatalogDataTools tab={tab} />
      </div>

      <div className="mb-4 flex gap-1 border-b border-slate-200">
        <TabButton active={tab === 'vendors'} onClick={() => setTab('vendors')} icon={Cable} label="Vendors" />
        <TabButton active={tab === 'models'} onClick={() => setTab('models')} icon={Cpu} label="Device Models" />
        <TabButton active={tab === 'mappings'} onClick={() => setTab('mappings')} icon={Route} label="Parameter Mappings" />
      </div>

      {tab === 'vendors' && <VendorsTab />}
      {tab === 'models' && <DeviceModelsTab />}
      {tab === 'mappings' && <MappingsTab />}
    </div>
  )
}

function TabButton({ active, onClick, icon: Icon, label }: { active: boolean; onClick: () => void; icon: typeof Cable; label: string }) {
  return (
    <button
      onClick={onClick}
      className={`flex items-center gap-1.5 border-b-2 px-3 py-2.5 text-sm font-medium transition-colors ${
        active ? 'border-slate-900 text-slate-900 dark:text-slate-100' : 'border-transparent text-slate-500 dark:text-slate-400 hover:text-slate-800'
      }`}
    >
      <Icon className="h-4 w-4" />
      {label}
    </button>
  )
}

// ---- Vendors ----

function VendorsTab() {
  const { data, isLoading } = useVendors()
  const vendors = data?.data ?? []
  const { hasRole } = useAuth()
  const canManage = hasRole('SUPERADMIN')
  const [showCreate, setShowCreate] = useState(false)
  const [ouiTargetId, setOuiTargetId] = useState<number | null>(null)
  const [editId, setEditId] = useState<number | null>(null)
  const deleteMutation = useDeleteVendor()
  const toast = useToast()
  const confirm = useConfirm()

  async function handleDelete(v: Vendor) {
    const ok = await confirm({
      title: 'Hapus vendor',
      message: (
        <>
          Vendor <strong>{v.name}</strong> ({v.code}) beserta OUI-nya akan dihapus. Ditolak bila masih dipakai device, model, mapping,
          firmware, profil, atau rule — untuk menghentikannya tanpa menghapus, nonaktifkan lewat <em>Edit</em>.
        </>
      ),
      confirmLabel: 'Hapus vendor',
      tone: 'danger',
    })
    if (!ok) return
    try {
      await deleteMutation.mutateAsync(v.id)
      toast.success(`Vendor "${v.name}" dihapus.`)
    } catch (err) {
      toast.error(errMsg(err, 'Gagal menghapus vendor'))
    }
  }

  return (
    <div>
      <div className="mb-4 flex justify-end">
        <button onClick={() => setShowCreate(true)} className={primaryBtnCls}>
          <Plus className="h-4 w-4" /> Vendor Baru
        </button>
      </div>

      <div className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm dark:border-slate-800 dark:bg-slate-900">
        {isLoading ? (
          <PageSpinner />
        ) : vendors.length === 0 ? (
          <EmptyState icon={Cable} title="Belum ada vendor" />
        ) : (
          <table className="w-full text-left text-sm">
            <thead>
              <tr className="border-b border-slate-200 bg-slate-50 text-xs font-medium uppercase tracking-wide text-slate-500 dark:border-slate-800 dark:bg-slate-800/50 dark:text-slate-400">
                <th className="px-5 py-3">Code</th>
                <th className="px-5 py-3">Nama</th>
                <th className="px-5 py-3">OUI Terdaftar</th>
                <th className="px-5 py-3">Status</th>
                <th className="px-5 py-3"></th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
              {vendors.map((v) => (
                <tr key={v.id}>
                  <td className="px-5 py-3.5 font-mono text-xs text-slate-700 dark:text-slate-300">{v.code}</td>
                  <td className="px-5 py-3.5 font-medium text-slate-900 dark:text-slate-100">{v.name}</td>
                  <td className="px-5 py-3.5">
                    <VendorOUICell vendorId={v.id} vendorName={v.name} count={v.oui_count ?? 0} canManage={canManage} />
                  </td>
                  <td className="px-5 py-3.5">
                    <StatusBadge code={v.is_active ? 'ONLINE' : 'OFFLINE'} label={v.is_active ? 'Aktif' : 'Nonaktif'} />
                  </td>
                  <td className="px-5 py-3.5 text-right">
                    <div className="flex items-center justify-end gap-3">
                      <button onClick={() => setOuiTargetId(v.id)} className="whitespace-nowrap text-xs font-medium text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:hover:text-slate-100">
                        + OUI
                      </button>
                      {canManage && <EditButton onClick={() => setEditId(v.id)} title={`Edit vendor ${v.code}`} />}
                      {canManage && <DeleteButton onClick={() => handleDelete(v)} title={`Hapus vendor ${v.code}`} disabled={deleteMutation.isPending} />}
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {showCreate && <CreateVendorModal onClose={() => setShowCreate(false)} />}
      {ouiTargetId !== null && <AddOUIModal vendorId={ouiTargetId} onClose={() => setOuiTargetId(null)} />}
      {editId !== null && <EditVendorModal id={editId} onClose={() => setEditId(null)} />}
    </div>
  )
}

// Seed OUI IEEE (migrasi 0022) membuat satu vendor bisa punya ribuan OUI
// (ZTE ~500, Huawei ~2.000): menampilkan semuanya sebagai chip di sel tabel
// membuat halaman tak terpakai. Sel hanya menampilkan jumlah (dari daftar vendor);
// daftar lengkap, pencarian, dan hapus ada di modal.
const OUI_LIST_LIMIT = 200

function OUIChip({ oui, notes, onDelete, disabled }: { oui: string; notes: string | null; onDelete?: () => void; disabled?: boolean }) {
  return (
    <span
      title={notes ?? undefined}
      className="inline-flex items-center gap-1 rounded bg-slate-100 px-1.5 py-0.5 font-mono text-[11px] text-slate-600 dark:bg-slate-800 dark:text-slate-400"
    >
      {oui}
      {onDelete && (
        <button
          type="button"
          onClick={onDelete}
          disabled={disabled}
          aria-label={`Hapus OUI ${oui}`}
          title={`Hapus OUI ${oui}`}
          className="rounded text-slate-500 transition-colors hover:text-red-600 disabled:opacity-50 dark:hover:text-red-400 dark:text-slate-400"
        >
          <X className="h-3 w-3" />
        </button>
      )}
    </span>
  )
}

// Jumlah OUI datang dari daftar vendor (satu permintaan); daftar lengkapnya baru diambil saat modal dibuka.
// Dulu satu permintaan per vendor per baris — dengan 30+ vendor itu menabrak rate limit API (429).
function VendorOUICell({ vendorId, vendorName, count, canManage }: { vendorId: number; vendorName: string; count: number; canManage: boolean }) {
  const [open, setOpen] = useState(false)

  if (count === 0) return <span className="text-xs text-slate-500 dark:text-slate-400">Belum ada</span>

  return (
    <div className="flex flex-wrap items-center gap-1">
      <span className="rounded-full bg-slate-200 px-2 py-0.5 text-[11px] font-medium text-slate-700 dark:bg-slate-700 dark:text-slate-200">
        {count} OUI
      </span>
      <button
        type="button"
        onClick={() => setOpen(true)}
        className="ml-1 text-xs font-medium text-slate-600 hover:text-slate-900 dark:text-slate-400 dark:hover:text-slate-100"
      >
        {canManage ? 'Kelola' : 'Lihat semua'}
      </button>
      {open && <VendorOUIsModal vendorId={vendorId} vendorName={vendorName} canManage={canManage} onClose={() => setOpen(false)} />}
    </div>
  )
}

function VendorOUIsModal({
  vendorId,
  vendorName,
  canManage,
  onClose,
}: {
  vendorId: number
  vendorName: string
  canManage: boolean
  onClose: () => void
}) {
  const { data, isLoading } = useVendorOUIs(vendorId)
  const ouis: VendorOUI[] = data ?? []
  const [query, setQuery] = useState('')
  const deleteMutation = useDeleteVendorOUI()
  const toast = useToast()
  const confirm = useConfirm()

  const q = query.trim().toLowerCase()
  const filtered = q ? ouis.filter((o) => o.oui.toLowerCase().includes(q) || (o.notes ?? '').toLowerCase().includes(q)) : ouis
  const shown = filtered.slice(0, OUI_LIST_LIMIT)

  async function handleDelete(ouiId: number, oui: string) {
    const ok = await confirm({
      title: 'Hapus OUI',
      message: (
        <>
          OUI <strong className="font-mono">{oui}</strong> dihapus dari vendor ini. Device yang sudah ter-resolve tidak berubah; Inform
          baru dari OUI ini tidak lagi dikenali sampai didaftarkan ulang.
        </>
      ),
      confirmLabel: 'Hapus OUI',
      tone: 'danger',
    })
    if (!ok) return
    try {
      await deleteMutation.mutateAsync({ vendorId, ouiId })
      toast.success(`OUI ${oui} dihapus.`)
    } catch (err) {
      toast.error(errMsg(err, 'Gagal menghapus OUI'))
    }
  }

  return (
    <Modal title={`OUI — ${vendorName}`} onClose={onClose} wide>
      <div className="space-y-3">
        <input maxLength={100}
          autoFocus
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="Cari OUI atau catatan…"
          aria-label="Cari OUI"
          className={inputCls}
        />
        <p className="text-xs text-slate-500 dark:text-slate-400" aria-live="polite">
          {isLoading ? 'Memuat daftar OUI…' : filtered.length === ouis.length ? `${ouis.length} OUI terdaftar` : `${filtered.length} cocok dari ${ouis.length} OUI`}
          {filtered.length > OUI_LIST_LIMIT && ` — menampilkan ${OUI_LIST_LIMIT} pertama, persempit pencarian untuk melihat sisanya`}
        </p>
        {shown.length === 0 ? (
          <p className="py-6 text-center text-sm text-slate-500 dark:text-slate-400">Tidak ada OUI yang cocok.</p>
        ) : (
          <div className="flex flex-wrap gap-1.5">
            {shown.map((o) => (
              <OUIChip
                key={o.id}
                oui={o.oui}
                notes={o.notes}
                onDelete={canManage ? () => handleDelete(o.id, o.oui) : undefined}
                disabled={deleteMutation.isPending}
              />
            ))}
          </div>
        )}
      </div>
    </Modal>
  )
}

function CreateVendorModal({ onClose }: { onClose: () => void }) {
  const [code, setCode] = useState('')
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [error, setError] = useState<string | null>(null)
  const createMutation = useCreateVendor()

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    try {
      await createMutation.mutateAsync({ code, name, description: description || undefined })
      onClose()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Gagal membuat vendor')
    }
  }

  return (
    <Modal title="Vendor Baru" onClose={onClose}>
      <form onSubmit={handleSubmit} className="space-y-3">
        <div>
          <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Code</label>
          <input required maxLength={LIMITS.vendor.code} value={code} onChange={(e) => setCode(e.target.value)} placeholder="mis. ZTE" className={inputCls} />
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Nama</label>
          <input required maxLength={LIMITS.vendor.name} value={name} onChange={(e) => setName(e.target.value)} placeholder="mis. ZTE Corporation" className={inputCls} />
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Deskripsi (opsional)</label>
          <input value={description} onChange={(e) => setDescription(e.target.value)} maxLength={LIMITS.description} className={inputCls} />
        </div>
        {error && <p className="text-sm text-red-600">{error}</p>}
        <button type="submit" disabled={createMutation.isPending} className={`${primaryBtnCls} w-full`}>
          Buat Vendor
        </button>
      </form>
    </Modal>
  )
}

// EditVendorModal memuat vendor segar lewat GET /vendors/:id (bukan dari baris
// tabel yang bisa basi) lalu menampilkan form. Code tidak bisa diubah.
function EditVendorModal({ id, onClose }: { id: number; onClose: () => void }) {
  const { data, isLoading, error } = useVendor(id)
  if (isLoading) {
    return (
      <Modal title="Edit Vendor" onClose={onClose}>
        <PageSpinner />
      </Modal>
    )
  }
  if (error || !data) {
    return (
      <Modal title="Edit Vendor" onClose={onClose}>
        <p role="alert" className="text-sm text-red-600 dark:text-red-400">
          {errMsg(error, 'Vendor tidak ditemukan')}
        </p>
      </Modal>
    )
  }
  return <EditVendorForm vendor={data} onClose={onClose} />
}

function EditVendorForm({ vendor, onClose }: { vendor: Vendor; onClose: () => void }) {
  const [name, setName] = useState(vendor.name)
  const [description, setDescription] = useState(vendor.description ?? '')
  const [isActive, setIsActive] = useState(vendor.is_active)
  const [error, setError] = useState<string | null>(null)
  const mutation = useUpdateVendor()
  const toast = useToast()

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    try {
      await mutation.mutateAsync({ id: vendor.id, input: { name, description, is_active: isActive } })
      toast.success(`Vendor "${name}" diperbarui.`)
      onClose()
    } catch (err) {
      setError(errMsg(err, 'Gagal memperbarui vendor'))
    }
  }

  return (
    <Modal title={`Edit Vendor — ${vendor.code}`} onClose={onClose}>
      <form onSubmit={handleSubmit} className="space-y-3">
        <div>
          <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Code</label>
          <input disabled value={vendor.code} className={`${inputCls} font-mono opacity-60`} />
          <p className="mt-1 text-xs text-slate-500 dark:text-slate-400">Code tidak bisa diubah (dipakai sebagai identitas stabil).</p>
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Nama</label>
          <input required maxLength={LIMITS.vendor.name} value={name} onChange={(e) => setName(e.target.value)} className={inputCls} />
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Deskripsi</label>
          <input value={description} onChange={(e) => setDescription(e.target.value)} maxLength={LIMITS.description} className={inputCls} />
        </div>
        <label className="flex items-start gap-2 text-sm text-slate-700 dark:text-slate-300">
          <input type="checkbox" checked={isActive} onChange={(e) => setIsActive(e.target.checked)} className="mt-0.5" />
          <span>Vendor aktif</span>
        </label>
        {error && <p role="alert" className="text-sm text-red-600 dark:text-red-400">{error}</p>}
        <div className="flex justify-end gap-2 pt-2">
          <button type="button" onClick={onClose} className={secondaryBtnCls}>
            Batal
          </button>
          <button type="submit" disabled={mutation.isPending} className={primaryBtnCls}>
            {mutation.isPending ? 'Menyimpan…' : 'Simpan'}
          </button>
        </div>
      </form>
    </Modal>
  )
}

function AddOUIModal({ vendorId, onClose }: { vendorId: number; onClose: () => void }) {
  const [oui, setOui] = useState('')
  const [notes, setNotes] = useState('')
  const [error, setError] = useState<string | null>(null)
  const addMutation = useAddVendorOUI()

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    try {
      await addMutation.mutateAsync({ vendorId, oui, notes: notes || undefined })
      onClose()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Gagal menambah OUI')
    }
  }

  return (
    <Modal title="Tambah OUI" onClose={onClose}>
      <form onSubmit={handleSubmit} className="space-y-3">
        <div>
          <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">OUI (6 hex digit)</label>
          <input required maxLength={LIMITS.vendor.oui} minLength={6} pattern="[0-9A-Za-z]{6}" title="Tepat 6 karakter huruf/angka (umumnya heksadesimal), mis. 3C6A9D" value={oui} onChange={(e) => setOui(e.target.value.toUpperCase())} placeholder="mis. 3C6A9D" className={`${inputCls} font-mono`} />
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Catatan (opsional)</label>
          <input value={notes} onChange={(e) => setNotes(e.target.value)} maxLength={LIMITS.vendor.notes} className={inputCls} />
        </div>
        {error && <p className="text-sm text-red-600">{error}</p>}
        <button type="submit" disabled={addMutation.isPending} className={`${primaryBtnCls} w-full`}>
          Tambah
        </button>
      </form>
    </Modal>
  )
}

// ---- Device Models ----

function DeviceModelsTab() {
  const { data: vendorsResp } = useVendors()
  const vendors = vendorsResp?.data ?? []
  const [vendorId, setVendorId] = useState('')
  const { hasRole } = useAuth()
  const canManage = hasRole('SUPERADMIN')
  const [showCreate, setShowCreate] = useState(false)
  const [editId, setEditId] = useState<number | null>(null)
  const { data: models, isLoading } = useDeviceModels(vendorId ? Number(vendorId) : undefined)
  const deleteMutation = useDeleteDeviceModel()
  const toast = useToast()
  const confirm = useConfirm()

  async function handleDelete(m: DeviceModel) {
    const ok = await confirm({
      title: 'Hapus device model',
      message: (
        <>
          Model <strong>{m.model_name}</strong> akan dihapus. Ditolak bila masih dipakai device, mapping, firmware, profil, atau rule —
          nonaktifkan lewat <em>Edit</em> bila hanya ingin menghentikannya.
        </>
      ),
      confirmLabel: 'Hapus model',
      tone: 'danger',
    })
    if (!ok) return
    try {
      await deleteMutation.mutateAsync(m.id)
      toast.success(`Model "${m.model_name}" dihapus.`)
    } catch (err) {
      toast.error(errMsg(err, 'Gagal menghapus model'))
    }
  }

  return (
    <div>
      <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
        <select value={vendorId} onChange={(e) => setVendorId(e.target.value)} className={`max-w-xs ${inputCls}`}>
          <option value="">Pilih vendor...</option>
          {vendors.map((v) => (
            <option key={v.id} value={v.id}>
              {v.name}
            </option>
          ))}
        </select>
        {vendorId && (
          <button onClick={() => setShowCreate(true)} className={primaryBtnCls}>
            <Plus className="h-4 w-4" /> Model Baru
          </button>
        )}
      </div>

      <div className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm dark:border-slate-800 dark:bg-slate-900">
        {!vendorId ? (
          <EmptyState icon={Cpu} title="Pilih vendor" description="Device model dikelompokkan per vendor." />
        ) : isLoading ? (
          <PageSpinner />
        ) : !models || models.length === 0 ? (
          <EmptyState icon={Cpu} title="Belum ada device model untuk vendor ini" />
        ) : (
          <table className="w-full text-left text-sm">
            <thead>
              <tr className="border-b border-slate-200 bg-slate-50 text-xs font-medium uppercase tracking-wide text-slate-500 dark:border-slate-800 dark:bg-slate-800/50 dark:text-slate-400">
                <th className="px-5 py-3">Model</th>
                <th className="px-5 py-3">Product Class</th>
                <th className="px-5 py-3">Status</th>
                <th className="px-5 py-3"></th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
              {models.map((m) => (
                <tr key={m.id}>
                  <td className="px-5 py-3.5 font-medium text-slate-900 dark:text-slate-100">{m.model_name}</td>
                  <td className="px-5 py-3.5 font-mono text-xs text-slate-600 dark:text-slate-400">{m.product_class ?? '-'}</td>
                  <td className="px-5 py-3.5">
                    <StatusBadge code={m.is_active ? 'ONLINE' : 'OFFLINE'} label={m.is_active ? 'Aktif' : 'Nonaktif'} />
                  </td>
                  <td className="px-5 py-3.5 text-right">
                    {canManage && (
                      <div className="flex items-center justify-end gap-3">
                        <EditButton onClick={() => setEditId(m.id)} title={`Edit model ${m.model_name}`} />
                        <DeleteButton onClick={() => handleDelete(m)} title={`Hapus model ${m.model_name}`} disabled={deleteMutation.isPending} />
                      </div>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {showCreate && <CreateDeviceModelModal vendorId={Number(vendorId)} onClose={() => setShowCreate(false)} />}
      {editId !== null && <EditDeviceModelModal id={editId} onClose={() => setEditId(null)} />}
    </div>
  )
}

function EditDeviceModelModal({ id, onClose }: { id: number; onClose: () => void }) {
  const { data, isLoading, error } = useDeviceModel(id)
  if (isLoading) {
    return (
      <Modal title="Edit Device Model" onClose={onClose}>
        <PageSpinner />
      </Modal>
    )
  }
  if (error || !data) {
    return (
      <Modal title="Edit Device Model" onClose={onClose}>
        <p role="alert" className="text-sm text-red-600 dark:text-red-400">
          {errMsg(error, 'Model tidak ditemukan')}
        </p>
      </Modal>
    )
  }
  return <EditDeviceModelForm model={data} onClose={onClose} />
}

function EditDeviceModelForm({ model, onClose }: { model: DeviceModel; onClose: () => void }) {
  const { data: deviceTypes } = useRefs('ref_device_types')
  const { data: dmVersions } = useRefs('ref_data_model_versions')
  const [deviceTypeId, setDeviceTypeId] = useState(String(model.device_type_id))
  const [dmVersionId, setDmVersionId] = useState(String(model.data_model_version_id))
  const [productClass, setProductClass] = useState(model.product_class ?? '')
  const [modelName, setModelName] = useState(model.model_name)
  const [description, setDescription] = useState(model.description ?? '')
  const [isActive, setIsActive] = useState(model.is_active)
  const [error, setError] = useState<string | null>(null)
  const mutation = useUpdateDeviceModel()
  const toast = useToast()

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    try {
      await mutation.mutateAsync({
        id: model.id,
        input: {
          model_name: modelName,
          product_class: productClass,
          description,
          is_active: isActive,
          device_type_id: Number(deviceTypeId),
          data_model_version_id: Number(dmVersionId),
        },
      })
      toast.success(`Model "${modelName}" diperbarui.`)
      onClose()
    } catch (err) {
      setError(errMsg(err, 'Gagal memperbarui model'))
    }
  }

  return (
    <Modal title={`Edit Model — ${model.model_name}`} onClose={onClose}>
      <form onSubmit={handleSubmit} className="space-y-3">
        <div>
          <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Nama Model</label>
          <input required maxLength={LIMITS.deviceModel.modelName} value={modelName} onChange={(e) => setModelName(e.target.value)} className={inputCls} />
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Product Class</label>
          <input value={productClass} onChange={(e) => setProductClass(e.target.value)} maxLength={LIMITS.deviceModel.productClass} placeholder="Kosong = tidak dipakai untuk pencocokan" className={`${inputCls} font-mono text-xs`} />
        </div>
        <div className="grid grid-cols-2 gap-3">
          <div>
            <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Device Type</label>
            <select required value={deviceTypeId} onChange={(e) => setDeviceTypeId(e.target.value)} className={inputCls}>
              {deviceTypes?.map((t) => (
                <option key={t.id} value={t.id}>
                  {t.name}
                </option>
              ))}
            </select>
          </div>
          <div>
            <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Data Model Version</label>
            <select required value={dmVersionId} onChange={(e) => setDmVersionId(e.target.value)} className={inputCls}>
              {dmVersions?.map((d) => (
                <option key={d.id} value={d.id}>
                  {d.name}
                </option>
              ))}
            </select>
          </div>
        </div>
        <p className="text-xs text-slate-500 dark:text-slate-400">
          Device Type dan Data Model Version tidak bisa diubah selama model dipakai device (resolusi path parameter akan rusak) — buat model baru.
        </p>
        <div>
          <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Deskripsi</label>
          <input value={description} onChange={(e) => setDescription(e.target.value)} maxLength={LIMITS.description} className={inputCls} />
        </div>
        <label className="flex items-start gap-2 text-sm text-slate-700 dark:text-slate-300">
          <input type="checkbox" checked={isActive} onChange={(e) => setIsActive(e.target.checked)} className="mt-0.5" />
          <span>Model aktif</span>
        </label>
        {error && <p role="alert" className="text-sm text-red-600 dark:text-red-400">{error}</p>}
        <div className="flex justify-end gap-2 pt-2">
          <button type="button" onClick={onClose} className={secondaryBtnCls}>
            Batal
          </button>
          <button type="submit" disabled={mutation.isPending} className={primaryBtnCls}>
            {mutation.isPending ? 'Menyimpan…' : 'Simpan'}
          </button>
        </div>
      </form>
    </Modal>
  )
}

function CreateDeviceModelModal({ vendorId, onClose }: { vendorId: number; onClose: () => void }) {
  const { data: deviceTypes } = useRefs('ref_device_types')
  const { data: dmVersions } = useRefs('ref_data_model_versions')
  const [deviceTypeId, setDeviceTypeId] = useState('')
  const [dmVersionId, setDmVersionId] = useState('')
  const [productClass, setProductClass] = useState('')
  const [modelName, setModelName] = useState('')
  const [description, setDescription] = useState('')
  const [error, setError] = useState<string | null>(null)
  const createMutation = useCreateDeviceModel()

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    const input: CreateDeviceModelInput = {
      vendor_id: vendorId,
      device_type_id: Number(deviceTypeId),
      data_model_version_id: Number(dmVersionId),
      product_class: productClass || undefined,
      model_name: modelName,
      description: description || undefined,
    }
    try {
      await createMutation.mutateAsync(input)
      onClose()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Gagal membuat device model')
    }
  }

  return (
    <Modal title="Device Model Baru" onClose={onClose}>
      <form onSubmit={handleSubmit} className="space-y-3">
        <div>
          <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Nama Model</label>
          <input required maxLength={LIMITS.deviceModel.modelName} value={modelName} onChange={(e) => setModelName(e.target.value)} placeholder="mis. F670L" className={inputCls} />
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Product Class (opsional)</label>
          <input value={productClass} onChange={(e) => setProductClass(e.target.value)} maxLength={LIMITS.deviceModel.productClass} placeholder="ProductClass dari Inform, mis. F670L" className={`${inputCls} font-mono text-xs`} />
        </div>
        <div className="grid grid-cols-2 gap-3">
          <div>
            <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Device Type</label>
            <select required value={deviceTypeId} onChange={(e) => setDeviceTypeId(e.target.value)} className={inputCls}>
              <option value="">Pilih...</option>
              {deviceTypes?.map((t) => (
                <option key={t.id} value={t.id}>
                  {t.name}
                </option>
              ))}
            </select>
          </div>
          <div>
            <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Data Model Version</label>
            <select required value={dmVersionId} onChange={(e) => setDmVersionId(e.target.value)} className={inputCls}>
              <option value="">Pilih...</option>
              {dmVersions?.map((d) => (
                <option key={d.id} value={d.id}>
                  {d.name}
                </option>
              ))}
            </select>
          </div>
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Deskripsi (opsional)</label>
          <input value={description} onChange={(e) => setDescription(e.target.value)} maxLength={LIMITS.description} className={inputCls} />
        </div>
        {error && <p className="text-sm text-red-600">{error}</p>}
        <button type="submit" disabled={createMutation.isPending} className={`${primaryBtnCls} w-full`}>
          Buat Model
        </button>
      </form>
    </Modal>
  )
}

// ---- Parameter Mappings ----

function MappingsTab() {
  const { data: vendorsResp } = useVendors()
  const vendors = vendorsResp?.data ?? []
  const [vendorId, setVendorId] = useState('')
  const { hasRole } = useAuth()
  const canManage = hasRole('SUPERADMIN')
  const [showCreate, setShowCreate] = useState(false)
  const [detailId, setDetailId] = useState<number | null>(null)
  const { data: mappings, isLoading } = useParameterMappings(vendorId ? Number(vendorId) : undefined)
  const deleteMutation = useDeleteParameterMapping()
  const toast = useToast()
  const confirm = useConfirm()

  async function handleDelete(id: number, key: string) {
    const ok = await confirm({
      title: 'Hapus parameter mapping',
      message: (
        <>
          Mapping <strong className="font-mono">{key}</strong> dihapus. Resolusi logical key ini jatuh ke mapping yang lebih generik bila
          ada; bila tidak ada, key ini tidak ter-resolve (path TR-069 mentah tetap bisa dipakai).
        </>
      ),
      confirmLabel: 'Hapus mapping',
      tone: 'danger',
    })
    if (!ok) return
    try {
      await deleteMutation.mutateAsync(id)
      toast.success(`Mapping "${key}" dihapus.`)
    } catch (err) {
      toast.error(errMsg(err, 'Gagal menghapus mapping'))
    }
  }

  return (
    <div>
      <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
        <select value={vendorId} onChange={(e) => setVendorId(e.target.value)} className={`max-w-xs ${inputCls}`}>
          <option value="">Pilih vendor...</option>
          {vendors.map((v) => (
            <option key={v.id} value={v.id}>
              {v.name}
            </option>
          ))}
        </select>
        {vendorId && (
          <button onClick={() => setShowCreate(true)} className={primaryBtnCls}>
            <Plus className="h-4 w-4" /> Mapping Baru
          </button>
        )}
      </div>

      <div className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm dark:border-slate-800 dark:bg-slate-900">
        {!vendorId ? (
          <EmptyState icon={Route} title="Pilih vendor" description="Parameter mapping dikelompokkan per vendor." />
        ) : isLoading ? (
          <PageSpinner />
        ) : !mappings || mappings.length === 0 ? (
          <EmptyState icon={Route} title="Belum ada parameter mapping untuk vendor ini" />
        ) : (
          <table className="w-full text-left text-sm">
            <thead>
              <tr className="border-b border-slate-200 bg-slate-50 text-xs font-medium uppercase tracking-wide text-slate-500 dark:border-slate-800 dark:bg-slate-800/50 dark:text-slate-400">
                <th className="px-5 py-3">Logical Key</th>
                <th className="px-5 py-3">TR-069 Path</th>
                <th className="px-5 py-3">Scope</th>
                <th className="px-5 py-3"></th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
              {mappings.map((m) => (
                <tr key={m.id}>
                  <td className="px-5 py-3.5 font-mono text-xs text-slate-700 dark:text-slate-300">{m.logical_key}</td>
                  <td className="px-5 py-3.5 font-mono text-xs text-slate-500 dark:text-slate-400">{m.tr069_path}</td>
                  <td className="px-5 py-3.5 text-xs text-slate-500 dark:text-slate-400">
                    {[
                      m.device_model_id ? `Model #${m.device_model_id}` : 'Semua model vendor',
                      m.software_version_pattern ? `sw LIKE ${m.software_version_pattern}` : null,
                    ]
                      .filter(Boolean)
                      .join(' · ')}
                  </td>
                  <td className="px-5 py-3.5 text-right">
                    <div className="flex items-center justify-end gap-3">
                      <DetailButton onClick={() => setDetailId(m.id)} title={`Detail mapping ${m.logical_key}`} />
                      {canManage && (
                        <DeleteButton onClick={() => handleDelete(m.id, m.logical_key)} title={`Hapus mapping ${m.logical_key}`} disabled={deleteMutation.isPending} />
                      )}
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {showCreate && <CreateMappingModal vendorId={Number(vendorId)} onClose={() => setShowCreate(false)} />}
      {detailId !== null && <MappingDetailModal id={detailId} onClose={() => setDetailId(null)} />}
    </div>
  )
}

function MappingDetailModal({ id, onClose }: { id: number; onClose: () => void }) {
  const { data, isLoading, error } = useParameterMapping(id)
  const { data: dmVersions } = useRefs('ref_data_model_versions')
  const { data: paramTypes } = useRefs('ref_parameter_types')
  const mono = (v: string) => <span className="font-mono text-xs">{v}</span>
  return (
    <DetailModal
      title="Detail Parameter Mapping"
      onClose={onClose}
      isLoading={isLoading}
      error={error ? errMsg(error, 'Mapping tidak ditemukan') : null}
      wide
      rows={
        data
          ? [
              ['Logical key', mono(data.logical_key)],
              ['TR-069 path', mono(data.tr069_path)],
              ['Data model', dmVersions?.find((d) => d.id === data.data_model_version_id)?.name ?? `#${data.data_model_version_id}`],
              ['Model', data.device_model_id ? `#${data.device_model_id}` : 'Semua model vendor'],
              ['Pola versi sw', data.software_version_pattern ? mono(data.software_version_pattern) : null],
              ['Tipe parameter', paramTypes?.find((p) => p.id === data.parameter_type_id)?.name ?? null],
              ['Deskripsi', data.description],
            ]
          : []
      }
    />
  )
}

function CreateMappingModal({ vendorId, onClose }: { vendorId: number; onClose: () => void }) {
  const { data: dmVersions } = useRefs('ref_data_model_versions')
  const { data: paramTypes } = useRefs('ref_parameter_types')
  const { data: models } = useDeviceModels(vendorId)
  const [dmVersionId, setDmVersionId] = useState('')
  const [deviceModelId, setDeviceModelId] = useState('')
  const [softwareVersionPattern, setSoftwareVersionPattern] = useState('')
  const [logicalKey, setLogicalKey] = useState('')
  const [tr069Path, setTr069Path] = useState('')
  const [parameterTypeId, setParameterTypeId] = useState('')
  const [description, setDescription] = useState('')
  const [error, setError] = useState<string | null>(null)
  const upsertMutation = useUpsertParameterMapping()

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    const input: UpsertMappingInput = {
      vendor_id: vendorId,
      data_model_version_id: Number(dmVersionId),
      device_model_id: deviceModelId ? Number(deviceModelId) : undefined,
      software_version_pattern: softwareVersionPattern || undefined,
      logical_key: logicalKey,
      tr069_path: tr069Path,
      parameter_type_id: parameterTypeId ? Number(parameterTypeId) : undefined,
      description: description || undefined,
    }
    try {
      await upsertMutation.mutateAsync(input)
      onClose()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Gagal menyimpan mapping')
    }
  }

  return (
    <Modal title="Parameter Mapping Baru" onClose={onClose}>
      <form onSubmit={handleSubmit} className="space-y-3">
        <div>
          <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Logical Key</label>
          <input required maxLength={LIMITS.mapping.logicalKey} value={logicalKey} onChange={(e) => setLogicalKey(e.target.value)} placeholder="mis. wifi.5g.ssid" className={`${inputCls} font-mono text-xs`} />
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">TR-069 Path</label>
          <input
            required
            maxLength={LIMITS.mapping.tr069Path}
            pattern="(InternetGatewayDevice|Device)[.][\w.\-]*"
            title="Harus berawalan InternetGatewayDevice. atau Device."
            value={tr069Path}
            onChange={(e) => setTr069Path(e.target.value)}
            placeholder="mis. InternetGatewayDevice.LANDevice.1.WLANConfiguration.5.SSID"
            className={`${inputCls} font-mono text-xs`}
          />
        </div>
        <div className="grid grid-cols-2 gap-3">
          <div>
            <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Data Model Version</label>
            <select required value={dmVersionId} onChange={(e) => setDmVersionId(e.target.value)} className={inputCls}>
              <option value="">Pilih...</option>
              {dmVersions?.map((d) => (
                <option key={d.id} value={d.id}>
                  {d.name}
                </option>
              ))}
            </select>
          </div>
          <div>
            <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Parameter Type (opsional)</label>
            <select value={parameterTypeId} onChange={(e) => setParameterTypeId(e.target.value)} className={inputCls}>
              <option value="">-</option>
              {paramTypes?.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name}
                </option>
              ))}
            </select>
          </div>
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Device Model (opsional — override lebih spesifik)</label>
          <select value={deviceModelId} onChange={(e) => setDeviceModelId(e.target.value)} className={inputCls}>
            <option value="">Semua model vendor ini</option>
            {models?.map((m) => (
              <option key={m.id} value={m.id}>
                {m.model_name}
              </option>
            ))}
          </select>
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Software Version Pattern (opsional — SQL LIKE, mapping lebih spesifik menang)</label>
          <input value={softwareVersionPattern} onChange={(e) => setSoftwareVersionPattern(e.target.value)} maxLength={LIMITS.mapping.softwareVersionPattern} placeholder="mis. V5.% — kosong = berlaku lintas semua versi" className={inputCls} />
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Deskripsi (opsional)</label>
          <input value={description} onChange={(e) => setDescription(e.target.value)} maxLength={LIMITS.description} className={inputCls} />
        </div>
        {error && <p className="text-sm text-red-600">{error}</p>}
        <button type="submit" disabled={upsertMutation.isPending} className={`${primaryBtnCls} w-full`}>
          Simpan Mapping
        </button>
      </form>
    </Modal>
  )
}
