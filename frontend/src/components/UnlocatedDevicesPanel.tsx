import { useState } from 'react'
import { MapPin } from 'lucide-react'
import { LocationPickerModal } from './LocationPickerModal'
import { useAuth } from '../lib/auth'
import { useDevices, type DeviceFilters } from '../lib/hooks'
import type { Device } from '../lib/types'

const PAGE_SIZE = 10

// Daftar perangkat (sesuai filter aktif) yang belum punya koordinat lengkap, dengan jalan pintas "Tetapkan lokasi".
// Hanya ADMIN/NOC yang dapat menetapkan (sama dengan RBAC PATCH /devices/{id}); peran lain hanya melihat daftar.
export function UnlocatedDevicesPanel({ filters }: { filters: DeviceFilters }) {
  const { hasRole } = useAuth()
  const canSet = hasRole('ADMIN', 'NOC')
  const [page, setPage] = useState(1)
  const [picking, setPicking] = useState<Device | null>(null)
  const { data, isLoading } = useDevices({ ...filters, has_location: false, page, page_size: PAGE_SIZE, sort: 'serial_number' })

  const rows = data?.data ?? []
  const total = data?.total ?? 0
  const pages = Math.max(1, Math.ceil(total / PAGE_SIZE))
  // Setelah lokasi ditetapkan, halaman terakhir bisa menjadi kosong: mundur ke halaman yang masih ada.
  if (data && page > pages) setPage(pages)
  if (!isLoading && total === 0) return null

  return (
    <section aria-label="Perangkat belum punya lokasi" className="rounded-xl border border-slate-200 bg-white dark:border-slate-800 dark:bg-slate-900">
      <header className="flex items-center gap-2 border-b border-slate-100 px-4 py-3 dark:border-slate-800">
        <MapPin className="h-4 w-4 text-slate-500" />
        <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100">Belum punya lokasi{data ? ` (${total})` : ''}</h3>
        <span className="text-xs text-slate-500 dark:text-slate-400">Tidak tampil di peta sampai koordinatnya diisi.</span>
      </header>
      <ul className="divide-y divide-slate-100 dark:divide-slate-800">
        {rows.map((d) => (
          <li key={d.id} className="flex items-center justify-between gap-3 px-4 py-2 text-sm">
            <div className="min-w-0">
              <span className="font-mono text-xs text-slate-900 dark:text-slate-100">{d.serial_number}</span>
              {d.product_class && <span className="ml-2 text-xs text-slate-500 dark:text-slate-400">{d.product_class}</span>}
            </div>
            {canSet && (
              <button
                type="button"
                onClick={() => setPicking(d)}
                aria-label={`Tetapkan lokasi ${d.serial_number}`}
                className="shrink-0 rounded-lg border border-slate-300 px-3 py-1 text-xs font-medium text-slate-700 hover:bg-slate-50 dark:border-slate-700 dark:text-slate-300 dark:hover:bg-slate-800"
              >
                Tetapkan lokasi
              </button>
            )}
          </li>
        ))}
      </ul>
      {pages > 1 && (
        <footer className="flex items-center justify-between border-t border-slate-100 px-4 py-2 text-xs text-slate-600 dark:border-slate-800 dark:text-slate-300">
          <button type="button" onClick={() => setPage((p) => Math.max(1, p - 1))} disabled={page <= 1} className="rounded px-2 py-1 hover:bg-slate-100 disabled:opacity-40 dark:hover:bg-slate-800">
            Sebelumnya
          </button>
          <span>
            Halaman {page} dari {pages}
          </span>
          <button type="button" onClick={() => setPage((p) => Math.min(pages, p + 1))} disabled={page >= pages} className="rounded px-2 py-1 hover:bg-slate-100 disabled:opacity-40 dark:hover:bg-slate-800">
            Berikutnya
          </button>
        </footer>
      )}
      {picking && <LocationPickerModal device={picking} onClose={() => setPicking(null)} />}
    </section>
  )
}
