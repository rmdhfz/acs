import { useState, type FormEvent } from 'react'
import { Cable, Cpu, Plus, Route } from 'lucide-react'
import { EmptyState } from '../components/EmptyState'
import { StatusBadge } from '../components/StatusBadge'
import { Modal } from '../components/Modal'
import { PageSpinner } from '../components/Spinner'
import { ApiError } from '../lib/api'
import { LIMITS } from '../lib/limits'
import {
  useAddVendorOUI,
  useCreateDeviceModel,
  useCreateVendor,
  useDeviceModels,
  useParameterMappings,
  useRefs,
  useUpsertParameterMapping,
  useVendorOUIs,
  useVendors,
  type CreateDeviceModelInput,
  type UpsertMappingInput,
} from '../lib/hooks'

const inputCls =
  'w-full rounded-lg border border-slate-300 px-3 py-2 text-sm text-slate-900 outline-none transition-colors focus:border-slate-500 focus:ring-1 focus:ring-slate-500 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-100'
const primaryBtnCls =
  'flex items-center justify-center gap-2 rounded-lg bg-slate-900 px-3 py-2 text-sm font-medium text-white transition-colors hover:bg-slate-800 disabled:cursor-not-allowed disabled:opacity-60 dark:bg-slate-100 dark:text-slate-900 dark:hover:bg-white'

type Tab = 'vendors' | 'models' | 'mappings'

export function CatalogPage() {
  const [tab, setTab] = useState<Tab>('vendors')

  return (
    <div className="mx-auto max-w-7xl px-6 py-8">
      <div className="mb-6">
        <h1 className="text-xl font-semibold text-slate-900 dark:text-slate-100">Catalog Vendor</h1>
        <p className="mt-0.5 text-sm text-slate-500 dark:text-slate-400">
          Menambah vendor/model baru adalah operasi data, bukan kode (TECH.md §5.1) — kelola di sini
        </p>
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
  const [showCreate, setShowCreate] = useState(false)
  const [ouiTargetId, setOuiTargetId] = useState<number | null>(null)

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
                    <VendorOUICell vendorId={v.id} />
                  </td>
                  <td className="px-5 py-3.5">
                    <StatusBadge code={v.is_active ? 'ONLINE' : 'OFFLINE'} label={v.is_active ? 'Aktif' : 'Nonaktif'} />
                  </td>
                  <td className="px-5 py-3.5 text-right">
                    <button onClick={() => setOuiTargetId(v.id)} className="text-xs font-medium text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:hover:text-slate-100">
                      + OUI
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {showCreate && <CreateVendorModal onClose={() => setShowCreate(false)} />}
      {ouiTargetId !== null && <AddOUIModal vendorId={ouiTargetId} onClose={() => setOuiTargetId(null)} />}
    </div>
  )
}

function VendorOUICell({ vendorId }: { vendorId: number }) {
  const { data, isLoading } = useVendorOUIs(vendorId)
  const ouis = data ?? []

  if (isLoading) return <span className="text-xs text-slate-400 dark:text-slate-500">…</span>
  if (ouis.length === 0) return <span className="text-xs text-slate-400 dark:text-slate-500">Belum ada</span>

  return (
    <div className="flex flex-wrap gap-1">
      {ouis.map((o) => (
        <span
          key={o.id}
          title={o.notes ?? undefined}
          className="rounded bg-slate-100 px-1.5 py-0.5 font-mono text-[11px] text-slate-600 dark:bg-slate-800 dark:text-slate-400"
        >
          {o.oui}
        </span>
      ))}
    </div>
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
          <input required maxLength={LIMITS.vendor.oui} value={oui} onChange={(e) => setOui(e.target.value.toUpperCase())} placeholder="mis. 3C6A9D" className={`${inputCls} font-mono`} />
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
  const [showCreate, setShowCreate] = useState(false)
  const { data: models, isLoading } = useDeviceModels(vendorId ? Number(vendorId) : undefined)

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
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {showCreate && <CreateDeviceModelModal vendorId={Number(vendorId)} onClose={() => setShowCreate(false)} />}
    </div>
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
  const [showCreate, setShowCreate] = useState(false)
  const { data: mappings, isLoading } = useParameterMappings(vendorId ? Number(vendorId) : undefined)

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
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {showCreate && <CreateMappingModal vendorId={Number(vendorId)} onClose={() => setShowCreate(false)} />}
    </div>
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
