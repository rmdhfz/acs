import { lazy, Suspense, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { MapPin } from 'lucide-react'
import { Modal } from './Modal'
import { PageSpinner } from './Spinner'
import { ApiError } from '../lib/api'
import { formatCoordinates, parseCoordinates, validCoordinates, type Coordinates } from '../lib/coordinates'
import { useUpdateDevice } from '../lib/hooks'
import { useToast } from '../lib/toast'
import type { Device } from '../lib/types'

// Pustaka peta hanya diunduh saat pemilih dibuka.
const LocationPickerMap = lazy(() => import('./LocationPickerMap'))

type PickerDevice = Pick<Device, 'id' | 'serial_number' | 'latitude' | 'longitude'>

const inputCls =
  'w-full rounded-lg border border-slate-300 px-3 py-2 text-sm text-slate-900 outline-none focus:border-indigo-600 focus:ring-1 focus:ring-indigo-600 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-100'

// Tetapkan lokasi perangkat dari peta: klik atau seret penanda, atau tempel koordinat ("-6.2088, 106.8456") dari Google Maps/ponsel.
// Tidak memakai layanan pencarian alamat pihak ketiga (privasi dan kebijakan pemakaian); koordinat disimpan lewat PATCH /devices/{id}.
export function LocationPickerModal({ device, onClose }: { device: PickerDevice; onClose: () => void }) {
  const update = useUpdateDevice()
  const toast = useToast()
  const qc = useQueryClient()
  const initial: Coordinates | null =
    device.latitude != null && device.longitude != null && validCoordinates(device.latitude, device.longitude) ? { lat: device.latitude, lon: device.longitude } : null
  const [value, setValue] = useState<Coordinates | null>(initial)
  const [text, setText] = useState('')
  const [pasteError, setPasteError] = useState<string | null>(null)

  const onPaste = (t: string) => {
    setText(t)
    if (!t.trim()) {
      setPasteError(null)
      return
    }
    const c = parseCoordinates(t)
    if (c) {
      setValue(c)
      setPasteError(null)
    } else {
      setPasteError('Format koordinat tidak dikenali. Contoh: -6.2088, 106.8456 (lintang -90..90, bujur -180..180).')
    }
  }

  const unchanged = !!initial && !!value && initial.lat === value.lat && initial.lon === value.lon
  const save = async () => {
    if (!value) return
    try {
      await update.mutateAsync({ id: device.id, latitude: value.lat, longitude: value.lon })
      void qc.invalidateQueries({ queryKey: ['device-map'] })
      toast.success(`Lokasi ${device.serial_number} disimpan.`, 'Berhasil')
      onClose()
    } catch (e) {
      toast.error(e instanceof ApiError || e instanceof Error ? e.message : 'Gagal menyimpan lokasi')
    }
  }

  return (
    <Modal title={`Tetapkan lokasi — ${device.serial_number}`} onClose={onClose} wide dismissable={!update.isPending}>
      <div className="space-y-3 p-5" data-modal-body>
        <div>
          <label htmlFor="paste-coords" className="block text-xs font-medium text-slate-700 dark:text-slate-300">
            Tempel koordinat
          </label>
          <input
            id="paste-coords"
            type="text"
            value={text}
            maxLength={80}
            onChange={(e) => onPaste(e.target.value)}
            placeholder="-6.2088, 106.8456"
            aria-invalid={!!pasteError}
            aria-describedby={pasteError ? 'paste-coords-err' : undefined}
            className={`mt-1 ${inputCls}`}
          />
          {pasteError && (
            <p id="paste-coords-err" role="alert" className="mt-1 text-xs text-red-600 dark:text-red-400">
              {pasteError}
            </p>
          )}
        </div>

        <Suspense fallback={<PageSpinner />}>
          <LocationPickerMap value={value} onChange={setValue} />
        </Suspense>

        <p className="flex items-center gap-2 text-sm text-slate-700 dark:text-slate-300" aria-live="polite">
          <MapPin className="h-4 w-4 text-slate-500" />
          {value ? <span>Lokasi dipilih: <span className="font-mono">{formatCoordinates(value.lat, value.lon)}</span></span> : <span className="text-slate-500 dark:text-slate-400">Klik peta, seret penanda, atau tempel koordinat.</span>}
        </p>

        <div className="flex justify-end gap-2 border-t border-slate-100 pt-3 dark:border-slate-800">
          <button type="button" onClick={onClose} disabled={update.isPending} className="rounded-lg border border-slate-300 px-4 py-2 text-sm font-medium text-slate-700 hover:bg-slate-50 disabled:opacity-50 dark:border-slate-700 dark:text-slate-300 dark:hover:bg-slate-800">
            Batal
          </button>
          <button type="button" onClick={() => void save()} disabled={!value || unchanged || update.isPending} className="rounded-lg bg-indigo-600 px-4 py-2 text-sm font-medium text-white hover:bg-indigo-700 disabled:cursor-not-allowed disabled:opacity-50">
            {update.isPending ? 'Menyimpan...' : 'Simpan lokasi'}
          </button>
        </div>
      </div>
    </Modal>
  )
}
