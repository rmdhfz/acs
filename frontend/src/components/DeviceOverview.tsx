import type { ReactNode } from 'react'
import { AlertTriangle, CheckCircle2, ExternalLink, ListChecks, MapPin, NotebookText, Pencil, SlidersHorizontal, Wifi } from 'lucide-react'
import { CopyButton } from './CopyButton'
import { formatDateTime, formatRelativeTime } from '../lib/format'
import type { Device } from '../lib/types'

// Ringkasan perangkat: apa yang paling sering ditanyakan operator (statusnya? firmware? bisa di-Connection Request?
// di mana lokasinya?) di bagian atas, detail teknis (UUID, OUI, dst.) di bawah. Hanya tampilan — semua data dari Device.

export type OverviewTarget = 'wifi' | 'parameters' | 'tasks'

const card = 'rounded-xl border border-slate-200 bg-white shadow-sm dark:border-slate-800 dark:bg-slate-900'

function Tile({ label, value, hint, tone }: { label: string; value: ReactNode; hint?: ReactNode; tone?: 'ok' | 'warn' | 'bad' }) {
  const bar = tone === 'ok' ? 'bg-emerald-500' : tone === 'bad' ? 'bg-red-500' : tone === 'warn' ? 'bg-amber-500' : 'bg-slate-300 dark:bg-slate-600'
  return (
    <div className={`${card} relative overflow-hidden p-4`}>
      <span className={`absolute inset-x-0 top-0 h-1 ${bar}`} />
      <p className="text-xs font-medium uppercase tracking-wide text-slate-500 dark:text-slate-400">{label}</p>
      <p className="mt-1 break-all text-lg font-semibold text-slate-900 dark:text-slate-100">{value}</p>
      {hint && <p className="mt-0.5 text-xs text-slate-500 dark:text-slate-400">{hint}</p>}
    </div>
  )
}

function Row({ label, children, copy, copyLabel }: { label: string; children: ReactNode; copy?: string | null; copyLabel?: string }) {
  return (
    <div className="grid grid-cols-3 gap-4 px-5 py-2.5 text-sm">
      <dt className="text-slate-500 dark:text-slate-400">{label}</dt>
      <dd className="col-span-2 flex items-center gap-1.5 break-all text-slate-800 dark:text-slate-200">
        <span className="min-w-0">{children}</span>
        {copy ? <CopyButton value={copy} label={copyLabel ?? `Salin ${label}`} /> : null}
      </dd>
    </div>
  )
}

function Mono({ v }: { v: string | null | undefined }) {
  return v ? <span className="font-mono text-xs">{v}</span> : <span className="text-slate-400">-</span>
}

function CardHeader({ title, action }: { title: string; action?: ReactNode }) {
  return (
    <div className="flex items-center justify-between border-b border-slate-100 px-5 py-3 dark:border-slate-800">
      <h3 className="text-sm font-medium text-slate-900 dark:text-slate-100">{title}</h3>
      {action}
    </div>
  )
}

export function DeviceOverview({
  device,
  vendorName,
  statusCode,
  statusLabel,
  canEdit,
  onEdit,
  onGoTo,
}: {
  device: Device
  vendorName: string | null
  statusCode: string | undefined
  statusLabel: string
  canEdit: boolean
  onEdit: () => void
  onGoTo: (t: OverviewTarget) => void
}) {
  const online = statusCode === 'ONLINE'
  const hasCr = !!device.connection_request_url
  const hasLoc = device.latitude != null && device.longitude != null
  const mapsUrl = hasLoc ? `https://www.google.com/maps?q=${device.latitude},${device.longitude}` : null
  const editBtn = canEdit ? (
    <button onClick={onEdit} className="inline-flex items-center gap-1 text-sm font-medium text-blue-600 hover:underline dark:text-blue-400">
      <Pencil className="h-3.5 w-3.5" /> Edit
    </button>
  ) : null

  return (
    <div className="space-y-6">
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <Tile
          label="Koneksi"
          value={statusLabel}
          tone={online ? 'ok' : 'bad'}
          hint={device.last_inform_at ? `Inform terakhir ${formatRelativeTime(device.last_inform_at)}` : 'Belum pernah Inform'}
        />
        <Tile label="IP Address" value={device.ip_address ?? '-'} hint={device.mac_address ? `MAC ${device.mac_address}` : 'MAC belum terbaca'} />
        <Tile label="Firmware" value={device.software_version ?? '-'} hint={device.hardware_version ? `Hardware ${device.hardware_version}` : 'Hardware belum terbaca'} />
        <Tile
          label="Terakhir boot"
          value={formatRelativeTime(device.last_boot_event_at)}
          hint={device.last_boot_event_at ? formatDateTime(device.last_boot_event_at) : 'Belum ada event BOOT'}
        />
      </div>

      {!hasCr && (
        <div role="status" className="flex flex-wrap items-start gap-3 rounded-xl border border-amber-200 bg-amber-50 p-4 text-sm text-amber-900 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-200">
          <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
          <div className="min-w-0 flex-1">
            <p className="font-medium">Connection Request belum tersedia</p>
            <p className="mt-0.5 text-xs">
              Tanpa URL ini, ACS tidak bisa memanggil perangkat langsung: perubahan (mis. ubah WiFi) baru dijalankan saat Inform periodik berikutnya, dan tombol “Percepat” nonaktif.
              Biasanya terjadi bila perangkat berada di belakang NAT atau ACS berjalan di Docker (IP sumber tidak sama dengan URL yang dilaporkan perangkat).
            </p>
          </div>
          {canEdit && (
            <button onClick={onEdit} className="rounded-full border border-amber-300 bg-white px-3 py-1 text-xs font-medium text-amber-900 hover:bg-amber-100 dark:border-amber-500/40 dark:bg-transparent dark:text-amber-200">
              Isi URL
            </button>
          )}
        </div>
      )}

      <div className="flex flex-wrap gap-2">
        <button onClick={() => onGoTo('wifi')} className="inline-flex items-center gap-1.5 rounded-full border border-slate-300 bg-white px-3 py-1.5 text-sm font-medium text-slate-700 hover:bg-slate-50 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-300 dark:hover:bg-slate-800">
          <Wifi className="h-4 w-4" /> Lihat / ubah WiFi
        </button>
        <button onClick={() => onGoTo('parameters')} className="inline-flex items-center gap-1.5 rounded-full border border-slate-300 bg-white px-3 py-1.5 text-sm font-medium text-slate-700 hover:bg-slate-50 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-300 dark:hover:bg-slate-800">
          <SlidersHorizontal className="h-4 w-4" /> Parameter TR-069
        </button>
        <button onClick={() => onGoTo('tasks')} className="inline-flex items-center gap-1.5 rounded-full border border-slate-300 bg-white px-3 py-1.5 text-sm font-medium text-slate-700 hover:bg-slate-50 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-300 dark:hover:bg-slate-800">
          <ListChecks className="h-4 w-4" /> Riwayat task
        </button>
      </div>

      <div className="grid gap-6 lg:grid-cols-2">
        <section className={card}>
          <CardHeader title="Identitas perangkat" action={editBtn} />
          <dl className="divide-y divide-slate-100 dark:divide-slate-800">
            <Row label="Serial number" copy={device.serial_number}>
              <Mono v={device.serial_number} />
            </Row>
            <Row label="Vendor">{vendorName ?? <span className="text-amber-700 dark:text-amber-400">Belum diketahui (OUI belum di katalog)</span>}</Row>
            <Row label="Model / Product class">{device.product_class ?? <span className="text-slate-400">-</span>}</Row>
            <Row label="OUI">
              <Mono v={device.oui} />
            </Row>
            <Row label="MAC address" copy={device.mac_address}>
              <Mono v={device.mac_address} />
            </Row>
            <Row label="Device UUID" copy={device.device_uuid}>
              <Mono v={device.device_uuid} />
            </Row>
          </dl>
        </section>

        <section className={card}>
          <CardHeader title="Koneksi & manajemen" />
          <dl className="divide-y divide-slate-100 dark:divide-slate-800">
            <Row label="IP address" copy={device.ip_address}>
              <Mono v={device.ip_address} />
            </Row>
            <Row label="Connection Request" copy={device.connection_request_url} copyLabel="Salin Connection Request URL">
              {hasCr ? (
                <span className="inline-flex flex-wrap items-center gap-1.5">
                  <span className="inline-flex items-center gap-1 rounded-full bg-emerald-50 px-2 py-0.5 text-xs font-medium text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-400">
                    <CheckCircle2 className="h-3 w-3" /> Siap
                  </span>
                  <Mono v={device.connection_request_url} />
                </span>
              ) : (
                <span className="rounded-full bg-amber-50 px-2 py-0.5 text-xs font-medium text-amber-700 dark:bg-amber-500/10 dark:text-amber-400">Belum ada</span>
              )}
            </Row>
            <Row label="Username Inform">
              <Mono v={device.inform_username} />
            </Row>
            <Row label="Pertama terdaftar">
              <span title={formatDateTime(device.created_at)}>{formatRelativeTime(device.created_at)}</span>
            </Row>
            <Row label="Terakhir diperbarui">
              <span title={formatDateTime(device.updated_at)}>{formatRelativeTime(device.updated_at)}</span>
            </Row>
          </dl>
        </section>

        <section className={card}>
          <CardHeader title="Lokasi" action={editBtn} />
          <div className="flex items-start gap-3 px-5 py-4 text-sm">
            <MapPin className="mt-0.5 h-4 w-4 shrink-0 text-slate-400" />
            {hasLoc ? (
              <div className="min-w-0">
                <p className="font-mono text-xs text-slate-800 dark:text-slate-200">
                  {device.latitude}, {device.longitude}
                </p>
                <a href={mapsUrl ?? undefined} target="_blank" rel="noopener noreferrer" className="mt-1 inline-flex items-center gap-1 text-xs font-medium text-blue-600 hover:underline dark:text-blue-400">
                  Buka di Google Maps <ExternalLink className="h-3 w-3" />
                </a>
              </div>
            ) : (
              <p className="text-slate-500 dark:text-slate-400">Lokasi belum diisi.{canEdit ? ' Klik Edit untuk menambahkan koordinat.' : ''}</p>
            )}
          </div>
        </section>

        <section className={card}>
          <CardHeader title="Catatan" action={editBtn} />
          <div className="flex items-start gap-3 px-5 py-4 text-sm">
            <NotebookText className="mt-0.5 h-4 w-4 shrink-0 text-slate-400" />
            {device.notes ? (
              <p className="whitespace-pre-wrap break-words text-slate-800 dark:text-slate-200">{device.notes}</p>
            ) : (
              <p className="text-slate-500 dark:text-slate-400">Belum ada catatan.</p>
            )}
          </div>
        </section>
      </div>
    </div>
  )
}
