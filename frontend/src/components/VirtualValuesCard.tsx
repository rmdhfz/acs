import { FunctionSquare } from 'lucide-react'
import { useDeviceVirtualParameters } from '../lib/hooks'

// Nilai virtual parameter (nilai turunan) satu perangkat; tidak tampil bila tenant belum punya definisi.
export function VirtualValuesCard({ deviceId }: { deviceId: number }) {
  const { data } = useDeviceVirtualParameters(deviceId)
  if (!data || data.length === 0) return null
  return (
    <section aria-label="Nilai turunan" className="mb-4 rounded-xl border border-slate-200 bg-white p-4 shadow-sm dark:border-slate-800 dark:bg-slate-900">
      <h3 className="mb-3 flex items-center gap-2 text-sm font-semibold text-slate-800 dark:text-slate-200">
        <FunctionSquare className="h-4 w-4 text-indigo-600 dark:text-indigo-300" /> Nilai turunan
      </h3>
      <dl className="grid grid-cols-2 gap-x-6 gap-y-3 sm:grid-cols-3 lg:grid-cols-4">
        {data.map((v) => (
          <div key={v.name}>
            <dt className="text-xs text-slate-500 dark:text-slate-400">{v.label}</dt>
            <dd className="mt-0.5 text-sm font-medium text-slate-900 dark:text-slate-100">
              {v.value == null ? (
                <span title="Parameter sumber belum dilaporkan perangkat" className="text-slate-400">
                  —
                </span>
              ) : (
                <>
                  {v.value}
                  {v.unit ? <span className="ml-1 text-xs font-normal text-slate-500 dark:text-slate-400">{v.unit}</span> : null}
                </>
              )}
            </dd>
          </div>
        ))}
      </dl>
    </section>
  )
}
