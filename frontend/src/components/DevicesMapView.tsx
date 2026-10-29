import { lazy, Suspense, useEffect, useMemo, useState } from 'react'
import { MapPin } from 'lucide-react'
import { EmptyState } from './EmptyState'
import { PageSpinner } from './Spinner'
import { useDeviceMap, type DeviceFilters } from '../lib/hooks'
import { countStatuses, mapNotice, statusStyle } from '../lib/deviceMap'

// Pustaka peta (Leaflet + markercluster) hanya diunduh saat tampilan Peta dibuka.
const DeviceMap = lazy(() => import('./DeviceMap'))

interface Props {
  // Filter aktif daftar perangkat: peta memakai filter yang SAMA supaya tabel dan peta selalu membicarakan perangkat yang sama.
  filters: DeviceFilters
  onOpenDevice: (deviceId: number) => void
}

export function DevicesMapView({ filters, onOpenDevice }: Props) {
  // Paginasi/urutan tak berlaku di peta.
  const base = useMemo(() => {
    const f: DeviceFilters = { ...filters }
    delete f.sort
    delete f.page
    delete f.page_size
    return f
  }, [filters])
  const filtersKey = JSON.stringify(base)

  // Mode viewport: bila hasil dipotong (> 5000 titik), peta meminta hanya titik di dalam pandangan saat ini. Filter berganti = kembali ke seluruh data.
  const [viewportMode, setViewportMode] = useState(false)
  const [bbox, setBbox] = useState<string | undefined>()
  useEffect(() => {
    setViewportMode(false)
    setBbox(undefined)
  }, [filtersKey])

  const { data, isLoading, isPlaceholderData } = useDeviceMap({ ...base, bbox: viewportMode ? bbox : undefined })
  useEffect(() => {
    if (data?.truncated && !viewportMode) setViewportMode(true)
  }, [data?.truncated, viewportMode])

  const points = data?.data ?? []
  const counts = countStatuses(points)
  const notice = mapNotice(data)

  if (isLoading) return <PageSpinner />
  if (data && data.located === 0) {
    return (
      <EmptyState
        icon={MapPin}
        title={data.total === 0 ? 'Tidak ada perangkat yang cocok dengan filter' : 'Belum ada perangkat yang punya koordinat'}
        description={
          data.total === 0
            ? 'Ubah atau hapus filter untuk melihat perangkat di peta.'
            : `${data.total} perangkat cocok dengan filter tetapi belum punya lokasi. Isi latitude dan longitude lewat Edit perangkat, atau lewat impor massal (kolom latitude dan longitude).`
        }
      />
    )
  }

  return (
    <section aria-label="Peta perangkat" className="space-y-3">
      <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-slate-600 dark:text-slate-300">
        <span className="font-medium text-slate-900 dark:text-slate-100">{points.length} perangkat di peta</span>
        <Legend code="ONLINE" count={counts.online} />
        <Legend code="OFFLINE" count={counts.offline} />
        {counts.other > 0 && <Legend code="OTHER" count={counts.other} />}
        <span className="text-slate-500 dark:text-slate-400">Klaster berwarna merah bila ada perangkat offline di dalamnya.</span>
      </div>
      {notice && (
        <p role="status" className="rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-800 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-300">
          {notice}
        </p>
      )}
      <Suspense fallback={<PageSpinner />}>
        <DeviceMap
          points={points}
          fitKey={isPlaceholderData ? undefined : filtersKey}
          onOpen={onOpenDevice}
          onViewportChange={(b) => {
            if (viewportMode) setBbox(b)
          }}
        />
      </Suspense>
    </section>
  )
}

function Legend({ code, count }: { code: string; count: number }) {
  const style = statusStyle(code === 'OTHER' ? '' : code)
  const label = code === 'OTHER' ? 'Lainnya' : style.label
  return (
    <span className="inline-flex items-center gap-1.5">
      <span className="inline-block h-2.5 w-2.5 rounded-full" style={{ background: style.color }} aria-hidden="true" />
      {label} <span className="font-medium text-slate-900 dark:text-slate-100">{count}</span>
    </span>
  )
}
