import { Network } from 'lucide-react'
import { StatusBadge } from './StatusBadge'
import { useDeviceOLT } from '../lib/hooks'
import { formatRelativeTime } from '../lib/format'
import { formatRx, isOltDataStale, onuStateBadge, onuStateLabel } from '../lib/oltInventory'

// "Posisi di jaringan": OLT, port PON, dan keadaan ONU menurut OLT untuk perangkat ini (serial sama, tenant sama). Disembunyikan sepenuhnya
// untuk tenant tanpa OLT dan saat inventaris tidak tersedia, supaya tidak menambah kebisingan bagi yang tidak memakai OLT.
export function DeviceOltCard({ deviceId }: { deviceId: number }) {
  const { data, isLoading, isError } = useDeviceOLT(deviceId)
  if (isLoading || isError || !data || !data.tenant_has_olt) return null

  const onu = data.onu
  if (!data.linked || !onu) {
    return (
      <section aria-label="Posisi di jaringan" className="rounded-xl border border-slate-200 bg-white px-5 py-4 text-sm text-slate-600 shadow-sm dark:border-slate-800 dark:bg-slate-900 dark:text-slate-300">
        <div className="flex items-center gap-2 font-medium text-slate-900 dark:text-slate-100">
          <Network className="h-4 w-4 text-slate-500" /> Posisi di jaringan
        </div>
        <p className="mt-1 text-xs text-slate-500 dark:text-slate-400">
          Serial perangkat ini tidak ditemukan di data OLT terakhir. Periksa kesamaan format serial antara OLT dan TR-069, apakah ONU sudah diregistrasi di OLT, dan apakah OLT sudah dipoll.
        </p>
      </section>
    )
  }

  const stale = isOltDataStale(onu.last_seen_at)
  return (
    <section aria-label="Posisi di jaringan" className="rounded-xl border border-slate-200 bg-white shadow-sm dark:border-slate-800 dark:bg-slate-900">
      <div className="flex flex-wrap items-center justify-between gap-2 border-b border-slate-100 px-5 py-3 dark:border-slate-800">
        <div className="flex items-center gap-2">
          <Network className="h-4 w-4 text-slate-500 dark:text-slate-400" />
          <h3 className="text-sm font-medium text-slate-900 dark:text-slate-100">Posisi di jaringan</h3>
          <StatusBadge code={onuStateBadge(onu.state)} label={`${onuStateLabel(onu.state)} di OLT`} />
        </div>
        <span className="text-xs text-slate-500 dark:text-slate-400">Data OLT {formatRelativeTime(onu.last_seen_at)}</span>
      </div>
      <dl className="grid gap-x-6 gap-y-2 px-5 py-4 text-sm sm:grid-cols-4">
        <Item label="OLT" value={onu.olt_name} />
        <Item label="Port PON" value={onu.pon_port || '—'} mono />
        <Item label="ONU" value={onu.onu_index ?? '—'} mono />
        <Item label="RX sisi OLT" value={formatRx(onu.rx_power_dbm)} />
      </dl>
      <p className="px-5 pb-4 text-xs text-slate-500 dark:text-slate-400">
        Keadaan terpantau sejak {formatRelativeTime(onu.last_state_change_at)}.
        {stale && <span className="ml-1 text-amber-700 dark:text-amber-400">Data OLT sudah lebih dari 30 menit; poll ulang OLT untuk memastikan.</span>}
      </p>
    </section>
  )
}

function Item({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div>
      <dt className="text-xs text-slate-500 dark:text-slate-400">{label}</dt>
      <dd className={`mt-0.5 break-all text-slate-900 dark:text-slate-100 ${mono ? 'font-mono text-xs' : ''}`}>{value}</dd>
    </div>
  )
}
