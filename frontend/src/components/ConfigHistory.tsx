import { useState } from 'react'
import { FileUp, Clock, FileJson, ArrowRightLeft } from 'lucide-react'
import { useConfigSnapshots, useCreateConfigSnapshot } from '../lib/hooks'
import { EmptyState } from './EmptyState'
import { PageSpinner } from './Spinner'
import { formatDateTime } from '../lib/format'
import { useAuth } from '../lib/auth'
import { useToast } from '../lib/toast'
import { useConfirm } from '../lib/confirm'
import { clsx } from 'clsx'

export function ConfigHistory({ deviceId }: { deviceId: number }) {
  const { data, isLoading } = useConfigSnapshots(deviceId)
  const createSnap = useCreateConfigSnapshot()
  const { hasRole } = useAuth()
  const toast = useToast()
  const confirm = useConfirm()

  const [selectedLeft, setSelectedLeft] = useState<number | null>(null)
  const [selectedRight, setSelectedRight] = useState<number | null>(null)

  if (isLoading) return <PageSpinner />
  const snaps = data?.data || []

  const handleCreate = async () => {
    if (await confirm({ title: 'Buat snapshot', message: 'Simpan seluruh nilai parameter device saat ini sebagai satu snapshot untuk perbandingan nanti?', confirmLabel: 'Buat snapshot' })) {
      try {
        await createSnap.mutateAsync(deviceId)
        toast.success('Snapshot konfigurasi tersimpan.', 'Berhasil')
      } catch (e) {
        toast.error(e instanceof Error ? e.message : 'Gagal membuat snapshot')
      }
    }
  }

  const leftSnap = snaps.find((s) => s.id === selectedLeft)
  const rightSnap = snaps.find((s) => s.id === selectedRight)

  // Generate Diff
  const diff: { key: string; leftVal?: string; rightVal?: string; status: 'added' | 'removed' | 'changed' | 'unchanged' }[] = []
  
  if (leftSnap && rightSnap) {
    const keys = new Set([...Object.keys(leftSnap.snapshot_data), ...Object.keys(rightSnap.snapshot_data)])
    Array.from(keys).sort().forEach(k => {
      const lv = leftSnap.snapshot_data[k]
      const rv = rightSnap.snapshot_data[k]
      if (lv === undefined) diff.push({ key: k, rightVal: rv, status: 'added' })
      else if (rv === undefined) diff.push({ key: k, leftVal: lv, status: 'removed' })
      else if (lv !== rv) diff.push({ key: k, leftVal: lv, rightVal: rv, status: 'changed' })
      else diff.push({ key: k, leftVal: lv, rightVal: rv, status: 'unchanged' })
    })
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h3 className="text-lg font-medium text-slate-900 dark:text-white">Configuration History</h3>
        {hasRole('ADMIN', 'NOC') && (
          <button
            onClick={handleCreate}
            disabled={createSnap.isPending}
            className="flex items-center gap-2 rounded-lg bg-blue-600 px-3 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50"
          >
            <FileUp className="h-4 w-4" />
            {createSnap.isPending ? 'Menyimpan...' : 'Buat Snapshot'}
          </button>
        )}
      </div>

      {snaps.length === 0 ? (
        <EmptyState
          icon={FileJson}
          title="Belum ada snapshot"
          description="Klik Buat Snapshot untuk menyimpan state parameter saat ini."
        />
      ) : (
        <div className="grid grid-cols-1 gap-6 lg:grid-cols-3">
          {/* List Snapshots */}
          <div className="rounded-xl border border-slate-200 bg-white shadow-sm dark:border-slate-800 dark:bg-slate-900 overflow-hidden">
            <div className="border-b border-slate-200 bg-slate-50 px-4 py-3 dark:border-slate-800 dark:bg-slate-900/50">
              <h4 className="font-medium text-slate-900 dark:text-white">Riwayat Snapshot</h4>
            </div>
            <div className="divide-y divide-slate-100 dark:divide-slate-800 max-h-[600px] overflow-y-auto">
              {snaps.map((s) => (
                <div key={s.id} className="p-4 transition-colors hover:bg-slate-50 dark:hover:bg-slate-800/50">
                  <div className="flex items-center justify-between mb-2">
                    <span className="flex items-center gap-1.5 text-sm font-medium text-slate-900 dark:text-slate-100">
                      <Clock className="h-4 w-4 text-slate-400" />
                      {formatDateTime(s.created_at)}
                    </span>
                    <span className="text-xs text-slate-500">ID: {s.id}</span>
                  </div>
                  <div className="flex gap-2 mt-3">
                    <button
                      onClick={() => setSelectedLeft(s.id)}
                      className={clsx(
                        "flex-1 rounded-md px-2 py-1 text-xs font-medium transition-colors border",
                        selectedLeft === s.id
                          ? "bg-red-50 text-red-700 border-red-200 dark:bg-red-500/10 dark:text-red-400 dark:border-red-500/20"
                          : "bg-white text-slate-600 border-slate-200 hover:bg-slate-50 dark:bg-slate-800 dark:text-slate-300 dark:border-slate-700 dark:hover:bg-slate-700"
                      )}
                    >
                      Bandingkan (Kiri)
                    </button>
                    <button
                      onClick={() => setSelectedRight(s.id)}
                      className={clsx(
                        "flex-1 rounded-md px-2 py-1 text-xs font-medium transition-colors border",
                        selectedRight === s.id
                          ? "bg-emerald-50 text-emerald-700 border-emerald-200 dark:bg-emerald-500/10 dark:text-emerald-400 dark:border-emerald-500/20"
                          : "bg-white text-slate-600 border-slate-200 hover:bg-slate-50 dark:bg-slate-800 dark:text-slate-300 dark:border-slate-700 dark:hover:bg-slate-700"
                      )}
                    >
                      Bandingkan (Kanan)
                    </button>
                  </div>
                </div>
              ))}
            </div>
          </div>

          {/* Visual Diff */}
          <div className="lg:col-span-2 rounded-xl border border-slate-200 bg-white shadow-sm dark:border-slate-800 dark:bg-slate-900 overflow-hidden flex flex-col">
            <div className="border-b border-slate-200 bg-slate-50 px-4 py-3 dark:border-slate-800 dark:bg-slate-900/50 flex items-center justify-between">
              <h4 className="font-medium text-slate-900 dark:text-white flex items-center gap-2">
                <ArrowRightLeft className="h-4 w-4" />
                Visual Diff
              </h4>
              <div className="text-sm text-slate-500 flex items-center gap-4">
                {leftSnap && <span>Kiri: #{leftSnap.id}</span>}
                {rightSnap && <span>Kanan: #{rightSnap.id}</span>}
              </div>
            </div>
            
            <div className="flex-1 overflow-auto p-0 bg-slate-50 dark:bg-slate-950 font-mono text-sm">
              {!leftSnap || !rightSnap ? (
                <div className="flex h-full items-center justify-center text-slate-400 p-8">
                  Pilih dua snapshot dari daftar di samping untuk melihat perbedaan (diff).
                </div>
              ) : diff.length === 0 ? (
                <div className="flex h-full items-center justify-center text-slate-400 p-8">
                  Tidak ada perbedaan konfigurasi.
                </div>
              ) : (
                <table className="w-full text-left border-collapse">
                  <tbody>
                    {diff.filter(d => d.status !== 'unchanged').map((d, i) => (
                      <tr key={i} className="border-b border-slate-200 dark:border-slate-800">
                        <td className="py-2 px-4 max-w-[200px] truncate" title={d.key}>{d.key}</td>
                        {d.status === 'changed' && (
                          <>
                            <td className="py-2 px-4 bg-red-50 text-red-700 dark:bg-red-500/10 dark:text-red-400 line-through truncate max-w-[150px]">{d.leftVal}</td>
                            <td className="py-2 px-4 bg-emerald-50 text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-400 truncate max-w-[150px]">{d.rightVal}</td>
                          </>
                        )}
                        {d.status === 'added' && (
                          <>
                            <td className="py-2 px-4 text-slate-400 italic">--</td>
                            <td className="py-2 px-4 bg-emerald-50 text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-400 truncate max-w-[150px]">{d.rightVal}</td>
                          </>
                        )}
                        {d.status === 'removed' && (
                          <>
                            <td className="py-2 px-4 bg-red-50 text-red-700 dark:bg-red-500/10 dark:text-red-400 line-through truncate max-w-[150px]">{d.leftVal}</td>
                            <td className="py-2 px-4 text-slate-400 italic">--</td>
                          </>
                        )}
                      </tr>
                    ))}
                  </tbody>
                </table>
              )}
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
