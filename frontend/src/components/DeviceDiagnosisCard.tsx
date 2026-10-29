import { useState } from 'react'
import { AlertOctagon, AlertTriangle, Gauge, Info, RefreshCw, Stethoscope, Wifi } from 'lucide-react'
import { ApiError } from '../lib/api'
import { formatDurationSeconds } from '../lib/format'
import { useDeviceDiagnosis, useGetParameterValues } from '../lib/hooks'
import { useToast } from '../lib/toast'
import { useWakeDevice } from '../lib/wake'
import type { Device, DiagnosisFinding, DiagnosisSeverity, DiagnosisVerdict } from '../lib/types'
import type { OverviewTarget } from './DeviceOverview'
import { Spinner } from './Spinner'

// "Kenapa offline?" — hipotesis berbukti dari sisi ACS (GET /devices/:id/diagnosis), bukan vonis. Terbuka otomatis untuk perangkat yang
// tidak online; untuk yang online disediakan tombol "Periksa koneksi" supaya operator bisa memeriksa sebelum ada keluhan.

const VERDICT: Record<DiagnosisVerdict, { label: string; cls: string }> = {
  OFFLINE: { label: 'Offline', cls: 'bg-red-50 text-red-700 dark:bg-red-500/10 dark:text-red-400' },
  REACHABLE: { label: 'Terjangkau', cls: 'bg-emerald-50 text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-400' },
  NEVER_CONNECTED: { label: 'Belum pernah terhubung', cls: 'bg-amber-50 text-amber-700 dark:bg-amber-500/10 dark:text-amber-400' },
}

const SEVERITY: Record<DiagnosisSeverity, { label: string; Icon: typeof Info; cls: string }> = {
  critical: { label: 'Kritis', Icon: AlertOctagon, cls: 'text-red-600 dark:text-red-400' },
  warning: { label: 'Waspada', Icon: AlertTriangle, cls: 'text-amber-600 dark:text-amber-400' },
  info: { label: 'Info', Icon: Info, cls: 'text-slate-500 dark:text-slate-400' },
}

const btn =
  'inline-flex items-center gap-1.5 rounded-full border border-slate-300 bg-white px-3 py-1.5 text-sm font-medium text-slate-700 hover:bg-slate-50 disabled:opacity-50 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-300 dark:hover:bg-slate-800'

export function DeviceDiagnosisCard({
  device,
  statusCode,
  canAct,
  onEdit,
  onGoTo,
}: {
  device: Device
  statusCode: string | undefined
  // ADMIN/NOC: boleh menjalankan aksi yang disarankan (Connection Request, baca parameter). Membaca diagnosis boleh semua staf.
  canAct: boolean
  onEdit: () => void
  onGoTo: (t: OverviewTarget) => void
}) {
  const proactive = !!statusCode && statusCode !== 'ONLINE'
  const [manualOpen, setManualOpen] = useState(false)
  const open = proactive || manualOpen
  const { data, isLoading, isFetching, refetch } = useDeviceDiagnosis(device.id, open)
  const wake = useWakeDevice(device, onEdit)
  const getValues = useGetParameterValues()
  const toast = useToast()

  const readInterval = async () => {
    if (!data?.inform_interval_parameter) return
    try {
      await getValues.mutateAsync({ deviceId: device.id, names: [data.inform_interval_parameter] })
      toast.success(
        wake.hasUrl
          ? 'Pembacaan interval diantre. Tekan "Hubungi sekarang" agar perangkat segera menjalankannya.'
          : 'Pembacaan interval diantre dan akan dijalankan saat perangkat Inform berikutnya.',
        'Berhasil',
      )
    } catch (e) {
      toast.error(e instanceof ApiError ? e.message : 'Gagal mengantre pembacaan interval')
    }
  }

  if (!open) {
    return (
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-slate-200 bg-white px-5 py-3 shadow-sm dark:border-slate-800 dark:bg-slate-900">
        <p className="text-sm text-slate-600 dark:text-slate-300">Perangkat online. Periksa kesehatan koneksi bila ada keluhan pelanggan.</p>
        <button type="button" onClick={() => setManualOpen(true)} className={btn}>
          <Stethoscope className="h-4 w-4" /> Periksa koneksi
        </button>
      </div>
    )
  }

  const verdict = data ? VERDICT[data.verdict] : null
  return (
    <section aria-label="Diagnosis koneksi" className="rounded-xl border border-slate-200 bg-white shadow-sm dark:border-slate-800 dark:bg-slate-900">
      <div className="flex flex-wrap items-center justify-between gap-2 border-b border-slate-100 px-5 py-3 dark:border-slate-800">
        <div className="flex items-center gap-2">
          <Stethoscope className="h-4 w-4 text-slate-500 dark:text-slate-400" />
          <h3 className="text-sm font-medium text-slate-900 dark:text-slate-100">Diagnosis koneksi</h3>
          {verdict && <span className={`rounded-full px-2.5 py-0.5 text-xs font-medium ${verdict.cls}`}>{verdict.label}</span>}
        </div>
        <button type="button" onClick={() => void refetch()} disabled={isFetching} className="inline-flex items-center gap-1 text-xs font-medium text-slate-500 hover:text-slate-800 disabled:opacity-50 dark:text-slate-400 dark:hover:text-slate-200">
          <RefreshCw className={`h-3.5 w-3.5 ${isFetching ? 'animate-spin' : ''}`} /> Periksa ulang
        </button>
      </div>

      {isLoading || !data ? (
        <div className="flex items-center gap-2 px-5 py-6 text-sm text-slate-500 dark:text-slate-400">
          <Spinner /> Memeriksa bukti dari sisi ACS...
        </div>
      ) : (
        <div className="space-y-4 px-5 py-4">
          <p className="text-sm font-medium text-slate-900 dark:text-slate-100">{data.summary}</p>
          <p className="text-xs text-slate-500 dark:text-slate-400">
            {data.silent_seconds == null ? 'Belum pernah Inform' : `Diam ${formatDurationSeconds(data.silent_seconds)}`} · ambang offline {formatDurationSeconds(data.offline_threshold_seconds)} · interval Inform{' '}
            {data.inform_interval_seconds ? formatDurationSeconds(data.inform_interval_seconds) : 'belum diketahui'}
          </p>

          <ul className="space-y-3" aria-label="Temuan diagnosis">
            {data.findings.map((f) => (
              <Finding key={f.code} f={f} />
            ))}
          </ul>

          <div className="flex flex-wrap gap-2">
            {canAct && data.actions.includes('CONNECTION_REQUEST') && (
              <button type="button" onClick={() => void wake.wake()} disabled={wake.pending} title={wake.hint} className={btn}>
                <Wifi className="h-4 w-4" /> Hubungi sekarang
              </button>
            )}
            {canAct && data.actions.includes('READ_INFORM_INTERVAL') && data.inform_interval_parameter && (
              <button type="button" onClick={() => void readInterval()} disabled={getValues.isPending} className={btn}>
                <RefreshCw className="h-4 w-4" /> Baca interval Inform
              </button>
            )}
            {data.actions.includes('OPEN_OPTICAL') && (
              <button type="button" onClick={() => onGoTo('optical')} className={btn}>
                <Gauge className="h-4 w-4" /> Lihat redaman optik
              </button>
            )}
          </div>

          <p className="text-xs italic text-slate-500 dark:text-slate-400">{data.limits}</p>
        </div>
      )}
    </section>
  )
}

function Finding({ f }: { f: DiagnosisFinding }) {
  const sev = SEVERITY[f.severity] ?? SEVERITY.info
  return (
    <li className="flex gap-3">
      <sev.Icon className={`mt-0.5 h-4 w-4 shrink-0 ${sev.cls}`} aria-hidden="true" />
      <div className="min-w-0 text-sm">
        <p className="font-medium text-slate-900 dark:text-slate-100">
          <span className={`mr-2 text-xs font-semibold uppercase ${sev.cls}`}>{sev.label}</span>
          {f.title}
        </p>
        <p className="mt-0.5 text-slate-600 dark:text-slate-300">{f.evidence}</p>
        <p className="mt-0.5 text-xs text-slate-500 dark:text-slate-400">Saran: {f.hint}</p>
      </div>
    </li>
  )
}
