import { useEffect, useState, type FormEvent } from 'react'
import { FileSliders, Plus, Radar, Trash2 } from 'lucide-react'
import { EmptyState } from '../components/EmptyState'
import { StatusBadge } from '../components/StatusBadge'
import { Modal } from '../components/Modal'
import { PageSpinner } from '../components/Spinner'
import { useAuth } from '../lib/auth'
import { ApiError } from '../lib/api'
import {
  useCreateProfile,
  useCreateZTRule,
  useDeleteProfile,
  useDeleteZTRule,
  useDeviceModels,
  useParameterMappings,
  useProvisioningProfile,
  useProvisioningProfiles,
  useUpdateProfile,
  useUpdateZTRule,
  useVendors,
  useZeroTouchRules,
  type ProfileFormInput,
  type ZTRuleFormInput,
} from '../lib/hooks'
import type { ProvisioningProfileParameter } from '../lib/types'

const inputCls =
  'w-full rounded-lg border border-slate-300 px-3 py-2 text-sm text-slate-900 outline-none transition-colors focus:border-slate-500 focus:ring-1 focus:ring-slate-500'
const primaryBtnCls =
  'flex items-center justify-center gap-2 rounded-lg bg-slate-900 px-3 py-2 text-sm font-medium text-white transition-colors hover:bg-slate-800 disabled:cursor-not-allowed disabled:opacity-60'

type Tab = 'profiles' | 'rules'

export function ProvisioningPage() {
  const [tab, setTab] = useState<Tab>('profiles')
  const { hasRole } = useAuth()
  const canManage = hasRole('ADMIN')

  return (
    <div className="mx-auto max-w-7xl px-6 py-8">
      <div className="mb-6">
        <h1 className="text-xl font-semibold text-slate-900">Provisioning</h1>
        <p className="mt-0.5 text-sm text-slate-500">Profil parameter default & aturan zero-touch provisioning</p>
      </div>

      <div className="mb-4 flex gap-1 border-b border-slate-200">
        <TabButton active={tab === 'profiles'} onClick={() => setTab('profiles')} icon={FileSliders} label="Provisioning Profiles" />
        <TabButton active={tab === 'rules'} onClick={() => setTab('rules')} icon={Radar} label="Zero-Touch Rules" />
      </div>

      {tab === 'profiles' ? <ProfilesTab canManage={canManage} /> : <ZTRulesTab canManage={canManage} />}
    </div>
  )
}

function TabButton({ active, onClick, icon: Icon, label }: { active: boolean; onClick: () => void; icon: typeof FileSliders; label: string }) {
  return (
    <button
      onClick={onClick}
      className={`flex items-center gap-1.5 border-b-2 px-3 py-2.5 text-sm font-medium transition-colors ${
        active ? 'border-slate-900 text-slate-900' : 'border-transparent text-slate-500 hover:text-slate-800'
      }`}
    >
      <Icon className="h-4 w-4" />
      {label}
    </button>
  )
}

// ---- Profiles ----

function ProfilesTab({ canManage }: { canManage: boolean }) {
  const { data, isLoading } = useProvisioningProfiles()
  const { data: vendorsResp } = useVendors()
  const vendors = vendorsResp?.data ?? []
  const [modalId, setModalId] = useState<number | 'new' | null>(null)
  const deleteMutation = useDeleteProfile()

  const profiles = data?.data ?? []

  return (
    <div>
      {canManage && (
        <div className="mb-4 flex justify-end">
          <button onClick={() => setModalId('new')} className={primaryBtnCls}>
            <Plus className="h-4 w-4" /> Profil Baru
          </button>
        </div>
      )}

      <div className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm">
        {isLoading ? (
          <PageSpinner />
        ) : profiles.length === 0 ? (
          <EmptyState icon={FileSliders} title="Belum ada provisioning profile" description="Buat profil untuk mengisi parameter default saat device di-provisioning." />
        ) : (
          <table className="w-full text-left text-sm">
            <thead>
              <tr className="border-b border-slate-200 bg-slate-50 text-xs font-medium uppercase tracking-wide text-slate-500">
                <th className="px-5 py-3">Nama</th>
                <th className="px-5 py-3">Vendor</th>
                <th className="px-5 py-3">Status</th>
                <th className="px-5 py-3"></th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100">
              {profiles.map((p) => (
                <tr key={p.id} onClick={() => canManage && setModalId(p.id)} className={canManage ? 'cursor-pointer hover:bg-slate-50' : ''}>
                  <td className="px-5 py-3.5">
                    <p className="font-medium text-slate-900">{p.name}</p>
                    {p.description && <p className="text-xs text-slate-400">{p.description}</p>}
                  </td>
                  <td className="px-5 py-3.5 text-slate-600">
                    {p.vendor_id ? vendors.find((v) => v.id === p.vendor_id)?.name ?? `#${p.vendor_id}` : 'Semua vendor'}
                  </td>
                  <td className="px-5 py-3.5">
                    <div className="flex gap-1.5">
                      {p.is_default && <StatusBadge code="COMPLETED" label="Default" />}
                      <StatusBadge code={p.is_active ? 'ONLINE' : 'OFFLINE'} label={p.is_active ? 'Aktif' : 'Nonaktif'} />
                    </div>
                  </td>
                  <td className="px-5 py-3.5 text-right">
                    {canManage && (
                      <button
                        onClick={(e) => {
                          e.stopPropagation()
                          if (confirm(`Hapus profil "${p.name}"?`)) deleteMutation.mutate(p.id)
                        }}
                        className="rounded-md p-1.5 text-slate-400 transition-colors hover:bg-red-50 hover:text-red-600"
                      >
                        <Trash2 className="h-4 w-4" />
                      </button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {modalId !== null && <ProfileModal id={modalId === 'new' ? undefined : modalId} onClose={() => setModalId(null)} />}
    </div>
  )
}

function ProfileModal({ id, onClose }: { id: number | undefined; onClose: () => void }) {
  const isEdit = id !== undefined
  const { data: existing, isLoading } = useProvisioningProfile(id)
  const { data: vendorsResp } = useVendors()
  const vendors = vendorsResp?.data ?? []

  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [vendorId, setVendorId] = useState('')
  const [deviceModelId, setDeviceModelId] = useState('')
  const [isDefault, setIsDefault] = useState(false)
  const [isActive, setIsActive] = useState(true)
  const [params, setParams] = useState<ProvisioningProfileParameter[]>([])
  const [error, setError] = useState<string | null>(null)

  const { data: models } = useDeviceModels(vendorId ? Number(vendorId) : undefined)
  const { data: mappings } = useParameterMappings(vendorId ? Number(vendorId) : undefined)

  useEffect(() => {
    if (existing) {
      setName(existing.profile.name)
      setDescription(existing.profile.description ?? '')
      setVendorId(existing.profile.vendor_id?.toString() ?? '')
      setDeviceModelId(existing.profile.device_model_id?.toString() ?? '')
      setIsDefault(existing.profile.is_default)
      setIsActive(existing.profile.is_active)
      setParams(existing.parameters)
    }
  }, [existing])

  const createMutation = useCreateProfile()
  const updateMutation = useUpdateProfile()

  function addParam() {
    setParams((p) => [...p, { parameter_name: '', parameter_value: '', apply_order: p.length }])
  }
  function updateParam(i: number, patch: Partial<ProvisioningProfileParameter>) {
    setParams((p) => p.map((row, idx) => (idx === i ? { ...row, ...patch } : row)))
  }
  function removeParam(i: number) {
    setParams((p) => p.filter((_, idx) => idx !== i))
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    const input: ProfileFormInput = {
      name,
      description: description || undefined,
      vendor_id: vendorId ? Number(vendorId) : undefined,
      device_model_id: deviceModelId ? Number(deviceModelId) : undefined,
      is_default: isDefault,
      is_active: isActive,
      parameters: params.filter((p) => p.parameter_name.trim() !== ''),
    }
    try {
      if (isEdit && id !== undefined) {
        await updateMutation.mutateAsync({ id, input })
      } else {
        await createMutation.mutateAsync(input)
      }
      onClose()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Gagal menyimpan profil')
    }
  }

  const isPending = createMutation.isPending || updateMutation.isPending

  return (
    <Modal title={isEdit ? 'Edit Provisioning Profile' : 'Provisioning Profile Baru'} onClose={onClose} wide>
      {isEdit && isLoading ? (
        <PageSpinner />
      ) : (
        <form onSubmit={handleSubmit} className="space-y-3">
          <div>
            <label className="mb-1 block text-xs font-medium text-slate-600">Nama</label>
            <input required value={name} onChange={(e) => setName(e.target.value)} className={inputCls} />
          </div>
          <div>
            <label className="mb-1 block text-xs font-medium text-slate-600">Deskripsi</label>
            <input value={description} onChange={(e) => setDescription(e.target.value)} className={inputCls} />
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="mb-1 block text-xs font-medium text-slate-600">Vendor (opsional — kosong = semua vendor)</label>
              <select
                value={vendorId}
                onChange={(e) => {
                  setVendorId(e.target.value)
                  setDeviceModelId('')
                }}
                className={inputCls}
              >
                <option value="">Semua vendor</option>
                {vendors.map((v) => (
                  <option key={v.id} value={v.id}>
                    {v.name}
                  </option>
                ))}
              </select>
            </div>
            <div>
              <label className="mb-1 block text-xs font-medium text-slate-600">Device Model (opsional)</label>
              <select value={deviceModelId} onChange={(e) => setDeviceModelId(e.target.value)} disabled={!vendorId} className={inputCls}>
                <option value="">Semua model</option>
                {models?.map((m) => (
                  <option key={m.id} value={m.id}>
                    {m.model_name}
                  </option>
                ))}
              </select>
            </div>
          </div>
          <div className="flex gap-5">
            <label className="flex items-center gap-2 text-sm text-slate-700">
              <input type="checkbox" checked={isDefault} onChange={(e) => setIsDefault(e.target.checked)} />
              Profil default
            </label>
            {isEdit && (
              <label className="flex items-center gap-2 text-sm text-slate-700">
                <input type="checkbox" checked={isActive} onChange={(e) => setIsActive(e.target.checked)} />
                Aktif
              </label>
            )}
          </div>

          <div>
            <div className="mb-1.5 flex items-center justify-between">
              <label className="text-xs font-medium text-slate-600">Parameter</label>
              <button type="button" onClick={addParam} className="text-xs font-medium text-slate-600 hover:text-slate-900">
                + tambah parameter
              </button>
            </div>
            <p className="mb-1.5 text-xs text-slate-400">
              {vendorId
                ? mappings && mappings.length > 0
                  ? `Ketik untuk cari logical key vendor ini (${mappings.length} tersedia) — raw TR-069 path tetap bisa diketik manual.`
                  : 'Belum ada parameter mapping utk vendor ini — ketik raw TR-069 path langsung, atau tambahkan mapping-nya dulu di Catalog Vendor.'
                : 'Pilih vendor di atas utk autocomplete logical key, atau ketik raw TR-069 path langsung.'}
            </p>
            <datalist id="param-logical-keys">
              {mappings?.map((m) => (
                <option key={m.id} value={m.logical_key} />
              ))}
            </datalist>
            <div className="space-y-2">
              {params.map((p, i) => {
                const matchedMapping = mappings?.find((m) => m.logical_key === p.parameter_name)
                return (
                  <div key={i}>
                    <div className="flex gap-2">
                      <input
                        placeholder="parameter_name / logical key"
                        list="param-logical-keys"
                        value={p.parameter_name}
                        onChange={(e) => updateParam(i, { parameter_name: e.target.value })}
                        className={`${inputCls} flex-1 font-mono text-xs`}
                      />
                      <input
                        placeholder="value"
                        value={p.parameter_value ?? ''}
                        onChange={(e) => updateParam(i, { parameter_value: e.target.value })}
                        className={`${inputCls} flex-1 font-mono text-xs`}
                      />
                      <button type="button" onClick={() => removeParam(i)} className="shrink-0 rounded-md p-2 text-slate-400 hover:bg-red-50 hover:text-red-600">
                        <Trash2 className="h-4 w-4" />
                      </button>
                    </div>
                    {matchedMapping && (
                      <p className="mt-0.5 truncate pl-0.5 font-mono text-[11px] text-slate-400">-&gt; {matchedMapping.tr069_path}</p>
                    )}
                  </div>
                )
              })}
              {params.length === 0 && <p className="text-xs text-slate-400">Belum ada parameter.</p>}
            </div>
          </div>

          {error && <p className="text-sm text-red-600">{error}</p>}
          <button type="submit" disabled={isPending} className={`${primaryBtnCls} w-full`}>
            {isEdit ? 'Simpan Perubahan' : 'Buat Profil'}
          </button>
        </form>
      )}
    </Modal>
  )
}

// ---- Zero-Touch Rules ----

function ZTRulesTab({ canManage }: { canManage: boolean }) {
  const { data: rules, isLoading } = useZeroTouchRules()
  const { data: vendorsResp } = useVendors()
  const { data: profilesResp } = useProvisioningProfiles()
  const vendors = vendorsResp?.data ?? []
  const profiles = profilesResp?.data ?? []
  const [modalId, setModalId] = useState<number | 'new' | null>(null)
  const deleteMutation = useDeleteZTRule()

  return (
    <div>
      {canManage && (
        <div className="mb-4 flex justify-end">
          <button onClick={() => setModalId('new')} className={primaryBtnCls}>
            <Plus className="h-4 w-4" /> Rule Baru
          </button>
        </div>
      )}

      <div className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm">
        {isLoading ? (
          <PageSpinner />
        ) : !rules || rules.length === 0 ? (
          <EmptyState icon={Radar} title="Belum ada zero-touch rule" description="Device baru yang BOOTSTRAP tanpa rule yang cocok akan menunggu tindakan manual (FR-15)." />
        ) : (
          <table className="w-full text-left text-sm">
            <thead>
              <tr className="border-b border-slate-200 bg-slate-50 text-xs font-medium uppercase tracking-wide text-slate-500">
                <th className="px-5 py-3">Prioritas</th>
                <th className="px-5 py-3">Kriteria</th>
                <th className="px-5 py-3">Profil</th>
                <th className="px-5 py-3">Status</th>
                <th className="px-5 py-3"></th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100">
              {[...rules]
                .sort((a, b) => a.priority - b.priority)
                .map((r) => (
                  <tr key={r.id} onClick={() => canManage && setModalId(r.id)} className={canManage ? 'cursor-pointer hover:bg-slate-50' : ''}>
                    <td className="px-5 py-3.5 tabular-nums text-slate-600">{r.priority}</td>
                    <td className="px-5 py-3.5 text-xs text-slate-600">
                      {[
                        r.vendor_id ? vendors.find((v) => v.id === r.vendor_id)?.name ?? `vendor#${r.vendor_id}` : null,
                        r.oui ? `OUI ${r.oui}` : null,
                        r.serial_pattern ? `serial LIKE ${r.serial_pattern}` : null,
                      ]
                        .filter(Boolean)
                        .join(' · ') || 'Semua device'}
                    </td>
                    <td className="px-5 py-3.5 text-slate-700">
                      {profiles.find((p) => p.id === r.provisioning_profile_id)?.name ?? `#${r.provisioning_profile_id}`}
                    </td>
                    <td className="px-5 py-3.5">
                      <StatusBadge code={r.is_active ? 'ONLINE' : 'OFFLINE'} label={r.is_active ? 'Aktif' : 'Nonaktif'} />
                    </td>
                    <td className="px-5 py-3.5 text-right">
                      {canManage && (
                        <button
                          onClick={(e) => {
                            e.stopPropagation()
                            if (confirm('Hapus rule ini?')) deleteMutation.mutate(r.id)
                          }}
                          className="rounded-md p-1.5 text-slate-400 transition-colors hover:bg-red-50 hover:text-red-600"
                        >
                          <Trash2 className="h-4 w-4" />
                        </button>
                      )}
                    </td>
                  </tr>
                ))}
            </tbody>
          </table>
        )}
      </div>

      {modalId !== null && <ZTRuleModal id={modalId === 'new' ? undefined : modalId} onClose={() => setModalId(null)} />}
    </div>
  )
}

function ZTRuleModal({ id, onClose }: { id: number | undefined; onClose: () => void }) {
  const isEdit = id !== undefined
  const { data: rules } = useZeroTouchRules()
  const existing = isEdit ? rules?.find((r) => r.id === id) : undefined
  const { data: vendorsResp } = useVendors()
  const { data: profilesResp } = useProvisioningProfiles()
  const vendors = vendorsResp?.data ?? []
  const profiles = profilesResp?.data ?? []

  const [vendorId, setVendorId] = useState('')
  const [deviceModelId, setDeviceModelId] = useState('')
  const [oui, setOui] = useState('')
  const [serialPattern, setSerialPattern] = useState('')
  const [profileId, setProfileId] = useState('')
  const [priority, setPriority] = useState('10')
  const [isActive, setIsActive] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const { data: models } = useDeviceModels(vendorId ? Number(vendorId) : undefined)

  useEffect(() => {
    if (existing) {
      setVendorId(existing.vendor_id?.toString() ?? '')
      setDeviceModelId(existing.device_model_id?.toString() ?? '')
      setOui(existing.oui ?? '')
      setSerialPattern(existing.serial_pattern ?? '')
      setProfileId(existing.provisioning_profile_id.toString())
      setPriority(existing.priority.toString())
      setIsActive(existing.is_active)
    }
  }, [existing])

  const createMutation = useCreateZTRule()
  const updateMutation = useUpdateZTRule()

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    const input: ZTRuleFormInput = {
      vendor_id: vendorId ? Number(vendorId) : undefined,
      device_model_id: deviceModelId ? Number(deviceModelId) : undefined,
      oui: oui || undefined,
      serial_pattern: serialPattern || undefined,
      provisioning_profile_id: Number(profileId),
      priority: Number(priority),
      is_active: isActive,
    }
    try {
      if (isEdit && id !== undefined) {
        await updateMutation.mutateAsync({ id, input })
      } else {
        await createMutation.mutateAsync(input)
      }
      onClose()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Gagal menyimpan rule')
    }
  }

  const isPending = createMutation.isPending || updateMutation.isPending

  return (
    <Modal title={isEdit ? 'Edit Zero-Touch Rule' : 'Zero-Touch Rule Baru'} onClose={onClose}>
      <form onSubmit={handleSubmit} className="space-y-3">
        <p className="text-xs text-slate-500">Kriteria dikombinasikan dengan AND — kosongkan yang tidak dipakai. Rule prioritas terkecil dievaluasi lebih dulu.</p>
        <div>
          <label className="mb-1 block text-xs font-medium text-slate-600">Vendor</label>
          <select
            value={vendorId}
            onChange={(e) => {
              setVendorId(e.target.value)
              setDeviceModelId('')
            }}
            className={inputCls}
          >
            <option value="">Semua vendor</option>
            {vendors.map((v) => (
              <option key={v.id} value={v.id}>
                {v.name}
              </option>
            ))}
          </select>
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-slate-600">Device Model</label>
          <select value={deviceModelId} onChange={(e) => setDeviceModelId(e.target.value)} disabled={!vendorId} className={inputCls}>
            <option value="">Semua model</option>
            {models?.map((m) => (
              <option key={m.id} value={m.id}>
                {m.model_name}
              </option>
            ))}
          </select>
        </div>
        <div className="grid grid-cols-2 gap-3">
          <div>
            <label className="mb-1 block text-xs font-medium text-slate-600">OUI</label>
            <input value={oui} onChange={(e) => setOui(e.target.value)} placeholder="mis. 3C6A9D" className={inputCls} />
          </div>
          <div>
            <label className="mb-1 block text-xs font-medium text-slate-600">Serial Pattern (SQL LIKE)</label>
            <input value={serialPattern} onChange={(e) => setSerialPattern(e.target.value)} placeholder="mis. ZTE%" className={inputCls} />
          </div>
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-slate-600">Provisioning Profile</label>
          <select required value={profileId} onChange={(e) => setProfileId(e.target.value)} className={inputCls}>
            <option value="">Pilih profil...</option>
            {profiles.map((p) => (
              <option key={p.id} value={p.id}>
                {p.name}
              </option>
            ))}
          </select>
        </div>
        <div className="grid grid-cols-2 gap-3">
          <div>
            <label className="mb-1 block text-xs font-medium text-slate-600">Priority</label>
            <input type="number" min={1} value={priority} onChange={(e) => setPriority(e.target.value)} className={inputCls} />
          </div>
          <label className="mt-6 flex items-center gap-2 text-sm text-slate-700">
            <input type="checkbox" checked={isActive} onChange={(e) => setIsActive(e.target.checked)} />
            Aktif
          </label>
        </div>
        {error && <p className="text-sm text-red-600">{error}</p>}
        <button type="submit" disabled={isPending} className={`${primaryBtnCls} w-full`}>
          {isEdit ? 'Simpan Perubahan' : 'Buat Rule'}
        </button>
      </form>
    </Modal>
  )
}
