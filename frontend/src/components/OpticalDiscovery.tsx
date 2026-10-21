import { useMemo, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { Gauge, RefreshCcw, Save, Search } from 'lucide-react'
import { useAuth } from '../lib/auth'
import { useToast } from '../lib/toast'
import { ApiError } from '../lib/api'
import { useDeviceModel, useDeviceParameters, useParameterMappings, useRefreshDeviceSummary, useRefreshParameterTree, useUpsertParameterMapping } from '../lib/hooks'
import { useWakeDevice } from '../lib/wake'
import {
  applyScale,
  findOpticalCandidates,
  opticalDiscoveryPath,
  OPTICAL_LOGICAL_KEY,
  OPTICAL_ROLE_LABEL,
  OPTICAL_ROLE_UNIT,
  SCALE_OPTIONS,
  type OpticalCandidate,
} from '../lib/opticalDiscovery'
import type { Device } from '../lib/types'

// Asisten "sumber data redaman optik": menelusuri cabang optik ONT, mengenali kandidat RX/TX/suhu/tegangan/bias dari parameter
// yang BENAR-BENAR dilaporkan perangkat, menampilkan pratinjau nilai per skala, lalu (SUPERADMIN) menyimpannya sebagai mapping
// optical.* untuk MODEL perangkat ini (bukan seluruh vendor). Setelah itu polling optik yang sudah ada mengisi tab ini.

const selectCls =
  'rounded-lg border border-slate-300 bg-white px-2 py-1 text-sm text-slate-900 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-100'
const btnCls =
  'inline-flex items-center gap-1.5 whitespace-nowrap rounded-full border border-slate-300 bg-white px-3 py-1.5 text-sm font-medium text-slate-700 transition-colors hover:bg-slate-50 disabled:opacity-50 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-300 dark:hover:bg-slate-800'
const primaryBtnCls =
  'inline-flex items-center gap-1.5 whitespace-nowrap rounded-full bg-indigo-600 px-4 py-1.5 text-sm font-medium text-white transition-colors hover:bg-indigo-700 disabled:cursor-not-allowed disabled:opacity-60 dark:bg-indigo-300 dark:text-slate-900 dark:hover:bg-indigo-200'

interface Choice {
  use: boolean
  scale: number
}

export function OpticalDiscovery({ device }: { device: Device }) {
  const { hasRole } = useAuth()
  const canDiscover = hasRole('ADMIN', 'NOC')
  const isSuper = hasRole('SUPERADMIN')
  const toast = useToast()
  const qc = useQueryClient()
  const { data: params } = useDeviceParameters(device.id)
  const { data: model } = useDeviceModel(device.device_model_id ?? undefined)
  const { data: mappings } = useParameterMappings(device.vendor_id ?? undefined)
  const discover = useRefreshParameterTree()
  const refreshSummary = useRefreshDeviceSummary()
  const upsert = useUpsertParameterMapping()
  const wakeDevice = useWakeDevice(device)
  const [choices, setChoices] = useState<Record<string, Partial<Choice>>>({})
  const [saving, setSaving] = useState(false)

  const candidates = useMemo(() => findOpticalCandidates(params), [params])
  const hasTR181 = (params ?? []).some((p) => p.parameter_name.startsWith('Device.'))

  const choiceOf = (c: OpticalCandidate): Choice => ({
    use: choices[c.path]?.use ?? (c.suggestedScale !== null && !c.warning),
    scale: choices[c.path]?.scale ?? c.suggestedScale ?? 1,
  })
  const set = (path: string, patch: Partial<Choice>) => setChoices((prev) => ({ ...prev, [path]: { ...prev[path], ...patch } }))

  // Mapping optik yang sudah aktif untuk perangkat ini (model-spesifik menang atas generik vendor).
  const active = (mappings ?? []).filter((m) => m.logical_key.startsWith('optical.') && (m.device_model_id === null || m.device_model_id === device.device_model_id))

  const onDiscover = () => {
    discover.mutate(
      { deviceId: device.id, path: opticalDiscoveryPath(hasTR181), depth: 3 },
      {
        onSuccess: () => {
          wakeDevice.wakeQuiet()
          toast.success('Penelusuran cabang optik diantre. Kandidat muncul bertahap setelah ONT Inform; klik "Muat ulang" untuk melihatnya.', 'Berhasil')
        },
        onError: (e) => toast.error(e instanceof ApiError ? e.message : 'Gagal mengantre penelusuran'),
      },
    )
  }

  const onReload = () => {
    void qc.invalidateQueries({ queryKey: ['device', device.id, 'parameters'] })
  }

  const selected = candidates.filter((c) => choiceOf(c).use)
  const canSave = isSuper && device.vendor_id != null && device.device_model_id != null && !!model && selected.length > 0

  const onSave = async () => {
    if (!canSave || !model || device.vendor_id == null || device.device_model_id == null) return
    setSaving(true)
    try {
      // satu mapping per peran: bila beberapa kandidat memilih peran yang sama, hentikan agar tak saling menimpa
      const roles = selected.map((c) => c.role)
      if (new Set(roles).size !== roles.length) {
        toast.error('Pilih satu kandidat per peran (RX, TX, suhu, tegangan, bias).')
        return
      }
      for (const c of selected) {
        await upsert.mutateAsync({
          vendor_id: device.vendor_id,
          data_model_version_id: model.data_model_version_id,
          device_model_id: device.device_model_id,
          logical_key: OPTICAL_LOGICAL_KEY[c.role],
          tr069_path: c.path,
          value_scale: choiceOf(c).scale,
          description: `Ditemukan dari perangkat ${device.serial_number} lewat asisten optik (nilai mentah ${c.raw || '-'})`,
        })
      }
      refreshSummary.mutate([device.id], { onSuccess: () => wakeDevice.wakeQuiet() })
      toast.success(`${selected.length} mapping optik disimpan untuk model ${model.model_name}. Pembacaan optik diantre; hasil tampil setelah ONT Inform.`, 'Berhasil')
    } catch (e) {
      toast.error(e instanceof ApiError ? e.message : 'Gagal menyimpan mapping')
    } finally {
      setSaving(false)
    }
  }

  if (!canDiscover && !isSuper) return null

  return (
    <section aria-label="Sumber data redaman optik" className="rounded-xl border border-slate-200 bg-white shadow-sm dark:border-slate-800 dark:bg-slate-900">
      <div className="flex flex-wrap items-center justify-between gap-2 border-b border-slate-100 px-5 py-3 dark:border-slate-800">
        <div>
          <h3 className="flex items-center gap-2 text-sm font-medium text-slate-900 dark:text-slate-100">
            <Gauge className="h-4 w-4" /> Sumber data redaman optik
          </h3>
          <p className="mt-0.5 text-xs text-slate-500 dark:text-slate-400">
            {active.length > 0
              ? `Mapping aktif: ${active.map((m) => m.logical_key.replace('optical.', '')).join(', ')}.`
              : 'Belum ada mapping optik untuk perangkat ini — path optik TR-098 adalah ekstensi vendor dan harus ditemukan dari ONT.'}
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <button onClick={onDiscover} disabled={discover.isPending} className={btnCls}>
            <Search className="h-4 w-4" /> Telusuri path optik
          </button>
          <button onClick={onReload} className={btnCls}>
            <RefreshCcw className="h-4 w-4" /> Muat ulang
          </button>
        </div>
      </div>

      {candidates.length === 0 ? (
        <p className="px-5 py-5 text-sm text-slate-500 dark:text-slate-400">
          Belum ada kandidat. Klik <b>Telusuri path optik</b>; ONT mengirim struktur cabang <span className="font-mono text-xs">{opticalDiscoveryPath(hasTR181)}</span> pada Inform
          berikutnya (bisa beberapa siklus), lalu klik <b>Muat ulang</b>.
        </p>
      ) : (
        <>
          <div className="table-scroll">
            <table className="w-full text-left text-sm">
              <thead>
                <tr className="border-b border-slate-200 bg-slate-50 text-xs font-medium uppercase tracking-wide text-slate-500 dark:border-slate-800 dark:bg-slate-800/50 dark:text-slate-400">
                  {isSuper && <th className="w-10 px-4 py-3">Pakai</th>}
                  <th className="px-4 py-3">Peran</th>
                  <th className="px-4 py-3">Path TR-069</th>
                  <th className="px-4 py-3">Nilai mentah</th>
                  <th className="px-4 py-3">Skala</th>
                  <th className="px-4 py-3">Hasil</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
                {candidates.map((c) => {
                  const ch = choiceOf(c)
                  const shown = applyScale(c.raw, ch.scale)
                  return (
                    <tr key={c.path} className="align-top text-slate-700 dark:text-slate-300">
                      {isSuper && (
                        <td className="px-4 py-3">
                          <input
                            type="checkbox"
                            checked={ch.use}
                            onChange={(e) => set(c.path, { use: e.target.checked })}
                            aria-label={`Pakai ${c.path}`}
                            className="h-4 w-4 rounded border-slate-300 text-indigo-600"
                          />
                        </td>
                      )}
                      <td className="px-4 py-3 font-medium text-slate-900 dark:text-slate-100">{OPTICAL_ROLE_LABEL[c.role]}</td>
                      <td className="max-w-[28rem] break-all px-4 py-3 font-mono text-xs">{c.path}</td>
                      <td className="px-4 py-3 font-mono text-xs">{c.raw === '' ? <span className="text-slate-400">kosong</span> : c.raw}</td>
                      <td className="px-4 py-3">
                        <select
                          value={ch.scale}
                          onChange={(e) => set(c.path, { scale: Number(e.target.value) })}
                          disabled={!isSuper}
                          aria-label={`Skala ${c.path}`}
                          className={selectCls}
                        >
                          {SCALE_OPTIONS.map((s) => (
                            <option key={s} value={s}>
                              × {s}
                            </option>
                          ))}
                        </select>
                      </td>
                      <td className="px-4 py-3">
                        <span className="font-medium tabular-nums">{shown === null ? '-' : `${shown} ${OPTICAL_ROLE_UNIT[c.role]}`}</span>
                        {c.warning && <p className="mt-1 max-w-xs text-xs text-amber-700 dark:text-amber-400">{c.warning}</p>}
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
          <div className="flex flex-wrap items-center justify-between gap-3 border-t border-slate-100 px-5 py-3 dark:border-slate-800">
            <p className="max-w-2xl text-xs text-slate-500 dark:text-slate-400">
              Cocokkan kolom <b>Hasil</b> dengan angka di GUI ONT (Status &gt; PON) sebelum menyimpan — satuan mentah berbeda per vendor, dan sebagian nilai ambigu.
              Mapping disimpan untuk <b>model</b> {model ? model.model_name : 'perangkat ini'} saja, bukan seluruh vendor.
            </p>
            {isSuper ? (
              <button onClick={() => void onSave()} disabled={!canSave || saving} className={primaryBtnCls} title={!model ? 'Perangkat belum punya model terdaftar' : undefined}>
                <Save className="h-4 w-4" /> {saving ? 'Menyimpan…' : `Simpan mapping (${selected.length})`}
              </button>
            ) : (
              <p className="text-xs text-slate-500 dark:text-slate-400">Penyimpanan mapping (data katalog global) hanya untuk SUPERADMIN — kirim daftar ini ke SUPERADMIN.</p>
            )}
          </div>
        </>
      )}
    </section>
  )
}
