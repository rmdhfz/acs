import { useState, type FormEvent } from 'react'
import { SlidersHorizontal, Plus, AlertCircle, AlertTriangle, ShieldCheck } from 'lucide-react'
import { EmptyState } from '../components/EmptyState'
import { PageSpinner } from '../components/Spinner'
import { Modal } from '../components/Modal'
import { useAuth } from '../lib/auth'
import { ApiError } from '../lib/api'
import { usePresets, useCreatePreset, useUpdatePreset, useDeletePreset } from '../lib/hooks'
import { formatDateTime } from '../lib/format'
import type { Preset } from '../lib/types'

const inputCls =
  'w-full rounded-lg border border-slate-300 px-3 py-2 text-sm text-slate-900 outline-none transition-colors focus:border-slate-500 focus:ring-1 focus:ring-slate-500 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-100'
const primaryBtnCls =
  'flex items-center justify-center gap-2 rounded-lg bg-slate-900 px-3 py-2 text-sm font-medium text-white transition-colors hover:bg-slate-800 disabled:cursor-not-allowed disabled:opacity-60 dark:bg-slate-100 dark:text-slate-900 dark:hover:bg-white'

const PRECOND_PLACEHOLDER = `{
  "vendor_id": 3,
  "software_version_pattern": "V9.0.10%"
}`
const CONFIG_PLACEHOLDER = `[
  { "op": "set_parameter", "key": "wifi.ssid", "value": "MyISP" },
  { "op": "set_parameter", "key": "device.periodic_inform_interval", "value": "300" }
]`

export default function PresetsPage() {
  const { hasRole } = useAuth()
  const canManage = hasRole('ADMIN')
  const { data, isLoading } = usePresets({ pageSize: 100 })
  const presets = data?.data ?? []

  const [editing, setEditing] = useState<Preset | null>(null)
  const [creating, setCreating] = useState(false)
  const [toDelete, setToDelete] = useState<Preset | null>(null)
  const deleteMut = useDeletePreset()

  return (
    <div className="mx-auto max-w-5xl px-6 py-8">
      <div className="mb-4 flex items-start justify-between gap-4">
        <div>
          <h1 className="text-xl font-semibold text-slate-900 dark:text-slate-100">Preset</h1>
          <p className="mt-0.5 text-sm text-slate-500 dark:text-slate-400">
            Desired-state config gaya GenieACS. Preset ber-<em>enforcement</em> dijaga otomatis tiap device menyimpang.
          </p>
        </div>
        {canManage && (
          <button onClick={() => setCreating(true)} className={primaryBtnCls}>
            <Plus className="h-4 w-4" /> Preset Baru
          </button>
        )}
      </div>

      <div className="mb-5 flex gap-3 rounded-lg border border-amber-200 bg-amber-50 p-3 text-sm text-amber-800 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-300">
        <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
        <p>
          Preset dengan <strong>enforcement aktif</strong> akan otomatis mendorong ulang konfigurasi ke SEMUA
          device yang cocok setiap kali nilainya menyimpang — berbeda dari Provisioning Profile (FR-18: tidak
          auto re-push). Aktifkan hanya bila memang ingin device dijaga terus-menerus.
        </p>
      </div>

      <div className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm dark:border-slate-800 dark:bg-slate-900">
        {isLoading ? (
          <PageSpinner />
        ) : presets.length === 0 ? (
          <EmptyState icon={SlidersHorizontal} title="Belum ada preset" description="Buat preset untuk menjaga konfigurasi device tetap sesuai standar." />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-left text-sm">
              <thead>
                <tr className="border-b border-slate-200 bg-slate-50 text-xs font-medium uppercase tracking-wide text-slate-500 dark:border-slate-800 dark:bg-slate-800/50 dark:text-slate-400">
                  <th className="px-5 py-3">Nama</th>
                  <th className="px-5 py-3">Weight</th>
                  <th className="px-5 py-3">Enforcement</th>
                  <th className="px-5 py-3">Channel</th>
                  <th className="px-5 py-3">Diubah</th>
                  <th className="px-5 py-3"></th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
                {presets.map((p) => (
                  <tr key={p.id}>
                    <td className="px-5 py-3.5 font-medium text-slate-900 dark:text-slate-100">{p.name}</td>
                    <td className="px-5 py-3.5 tabular-nums text-slate-600 dark:text-slate-300">{p.weight}</td>
                    <td className="px-5 py-3.5">
                      {p.enforce ? (
                        <span className="inline-flex items-center gap-1 rounded-full bg-emerald-100 px-2 py-0.5 text-xs font-medium text-emerald-700 dark:bg-emerald-500/15 dark:text-emerald-300">
                          <ShieldCheck className="h-3 w-3" /> Aktif
                        </span>
                      ) : (
                        <span className="text-xs text-slate-400">Nonaktif</span>
                      )}
                    </td>
                    <td className="px-5 py-3.5 text-slate-500 dark:text-slate-400">{p.channel ?? '—'}</td>
                    <td className="px-5 py-3.5 text-slate-500 dark:text-slate-400">{formatDateTime(p.updated_at)}</td>
                    <td className="px-5 py-3.5 text-right">
                      {canManage && (
                        <div className="flex justify-end gap-3 text-xs font-medium">
                          <button onClick={() => setEditing(p)} className="text-slate-600 hover:text-slate-900 dark:text-slate-400 dark:hover:text-slate-200">
                            Edit
                          </button>
                          <button onClick={() => setToDelete(p)} className="text-red-600 hover:text-red-700 dark:text-red-400 dark:hover:text-red-300">
                            Hapus
                          </button>
                        </div>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {(creating || editing) && (
        <PresetModal preset={editing} onClose={() => { setCreating(false); setEditing(null) }} />
      )}

      {toDelete && (
        <Modal onClose={() => setToDelete(null)} title="Hapus Preset">
          <div className="space-y-4">
            <div className="flex gap-3 rounded-lg border border-red-100 bg-red-50 p-4 text-red-600 dark:border-red-400/20 dark:bg-red-400/10 dark:text-red-400">
              <AlertCircle className="h-5 w-5 shrink-0" />
              <p className="text-sm">Hapus preset <strong>{toDelete.name}</strong>? Device yang sudah terkonfigurasi tidak berubah — hanya enforcement yang berhenti.</p>
            </div>
            <div className="flex justify-end gap-3">
              <button onClick={() => setToDelete(null)} className="rounded-lg px-4 py-2 text-sm font-medium text-slate-700 hover:bg-slate-100 dark:text-slate-300 dark:hover:bg-slate-800">Batal</button>
              <button
                onClick={() => deleteMut.mutate(toDelete.id, { onSuccess: () => setToDelete(null) })}
                disabled={deleteMut.isPending}
                className="rounded-lg bg-red-600 px-4 py-2 text-sm font-medium text-white hover:bg-red-700 disabled:opacity-50"
              >
                {deleteMut.isPending ? 'Menghapus…' : 'Ya, Hapus'}
              </button>
            </div>
          </div>
        </Modal>
      )}
    </div>
  )
}

function PresetModal({ preset, onClose }: { preset: Preset | null; onClose: () => void }) {
  const isEdit = !!preset
  const [name, setName] = useState(preset?.name ?? '')
  const [weight, setWeight] = useState(String(preset?.weight ?? 0))
  const [channel, setChannel] = useState(preset?.channel ?? '')
  const [enforce, setEnforce] = useState(preset?.enforce ?? false)
  const [precondition, setPrecondition] = useState(preset?.precondition || '{}')
  const [configurations, setConfigurations] = useState(preset?.configurations || '[]')
  const [error, setError] = useState<string | null>(null)

  const createMut = useCreatePreset()
  const updateMut = useUpdatePreset()
  const pending = createMut.isPending || updateMut.isPending

  function validJSON(s: string): string | null {
    try {
      JSON.parse(s)
      return null
    } catch (e) {
      return e instanceof Error ? e.message : 'JSON tidak valid'
    }
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    if (!name.trim()) return setError('Nama preset wajib diisi.')
    const pcErr = validJSON(precondition)
    if (pcErr) return setError(`Precondition: ${pcErr}`)
    const cfgErr = validJSON(configurations)
    if (cfgErr) return setError(`Configurations: ${cfgErr}`)

    const payload = {
      name: name.trim(),
      weight: Number(weight) || 0,
      channel: channel.trim() || null,
      enforce,
      precondition,
      configurations,
    }
    try {
      if (isEdit) await updateMut.mutateAsync({ id: preset!.id, data: payload })
      else await createMut.mutateAsync(payload)
      onClose()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Gagal menyimpan preset')
    }
  }

  return (
    <Modal title={isEdit ? `Edit Preset — ${preset!.name}` : 'Preset Baru'} onClose={onClose}>
      <form onSubmit={handleSubmit} className="space-y-4">
        <div className="grid grid-cols-3 gap-3">
          <div className="col-span-2">
            <label className="mb-1 block text-sm font-medium text-slate-600 dark:text-slate-400">Nama</label>
            <input value={name} onChange={(e) => setName(e.target.value)} className={inputCls} required />
          </div>
          <div>
            <label className="mb-1 block text-sm font-medium text-slate-600 dark:text-slate-400">Weight</label>
            <input type="number" value={weight} onChange={(e) => setWeight(e.target.value)} className={inputCls} />
          </div>
        </div>

        <div>
          <label className="mb-1 block text-sm font-medium text-slate-600 dark:text-slate-400">Channel (opsional)</label>
          <input value={channel} onChange={(e) => setChannel(e.target.value)} placeholder="mis. wifi, mgmt" className={inputCls} />
        </div>

        <div>
          <label className="mb-1 block text-sm font-medium text-slate-600 dark:text-slate-400">
            Precondition <span className="font-normal text-slate-400">— JSON, kosakata sama dengan Zero-Touch Rule. <code>{'{}'}</code> = cocok semua device.</span>
          </label>
          <textarea
            value={precondition}
            onChange={(e) => setPrecondition(e.target.value)}
            rows={4}
            spellCheck={false}
            placeholder={PRECOND_PLACEHOLDER}
            className={`${inputCls} font-mono text-xs`}
          />
        </div>

        <div>
          <label className="mb-1 block text-sm font-medium text-slate-600 dark:text-slate-400">
            Configurations <span className="font-normal text-slate-400">— JSON array. v1: hanya <code>op: "set_parameter"</code>.</span>
          </label>
          <textarea
            value={configurations}
            onChange={(e) => setConfigurations(e.target.value)}
            rows={5}
            spellCheck={false}
            placeholder={CONFIG_PLACEHOLDER}
            className={`${inputCls} font-mono text-xs`}
          />
        </div>

        <label className="flex items-start gap-2.5 rounded-lg border border-slate-200 p-3 dark:border-slate-700">
          <input type="checkbox" checked={enforce} onChange={(e) => setEnforce(e.target.checked)} className="mt-0.5" />
          <span className="text-sm text-slate-700 dark:text-slate-300">
            <strong>Aktifkan enforcement</strong>
            <span className="mt-0.5 block text-xs text-slate-500 dark:text-slate-400">
              ACS akan mendorong ulang config ini ke setiap device yang cocok tiap kali menyimpang (drift-heal, tiap sesi CWMP).
              Biarkan nonaktif untuk menyimpan preset tanpa memberlakukannya.
            </span>
          </span>
        </label>

        {error && <p className="text-sm text-red-600 dark:text-red-400">{error}</p>}

        <div className="flex justify-end gap-3 pt-1">
          <button type="button" onClick={onClose} className="rounded-lg px-4 py-2 text-sm font-medium text-slate-600 hover:bg-slate-100 dark:text-slate-400 dark:hover:bg-slate-800">Batal</button>
          <button type="submit" disabled={pending} className={primaryBtnCls}>
            {pending ? 'Menyimpan…' : 'Simpan Preset'}
          </button>
        </div>
      </form>
    </Modal>
  )
}
