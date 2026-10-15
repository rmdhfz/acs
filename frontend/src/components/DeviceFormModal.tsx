import { useState, type FormEvent } from 'react'
import { AlertCircle, ChevronDown } from 'lucide-react'
import { Modal } from './Modal'
import { Spinner } from './Spinner'
import { ApiError } from '../lib/api'
import { useAuth } from '../lib/auth'
import { useToast } from '../lib/toast'
import { useCreateDevice, useDeviceModels, useTenants, useUpdateDevice, useVendors } from '../lib/hooks'
import type { Device } from '../lib/types'

const inputCls =
  'w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 outline-none transition-colors focus:border-slate-500 focus:ring-1 focus:ring-slate-500 disabled:cursor-not-allowed disabled:bg-slate-50 disabled:text-slate-500 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-100 dark:disabled:bg-slate-800/50'
const labelCls = 'mb-1.5 block text-sm font-medium text-slate-700 dark:text-slate-300'
const hintCls = 'mt-1 text-xs text-slate-400 dark:text-slate-500'

interface Props {
  mode: 'create' | 'edit'
  /** Wajib untuk mode edit. */
  device?: Device
  onClose: () => void
  onSaved?: (d: Device) => void
}

const MAC_RE = /^([0-9A-Fa-f]{2}[:\-.]?){5}[0-9A-Fa-f]{2}$|^[0-9A-Fa-f]{4}\.[0-9A-Fa-f]{4}\.[0-9A-Fa-f]{4}$/

// DeviceFormModal — satu form untuk PRA-REGISTRASI (tambah) dan EDIT perangkat.
// Pra-registrasi membuat baris inventory sebelum CPE pertama kali Inform
// (tenant terkunci; status "Belum terdaftar" sampai CPE terhubung). OUI &
// serial tidak bisa diubah setelah dibuat (kunci identitas TR-069).
export function DeviceFormModal({ mode, device, onClose, onSaved }: Props) {
  const { hasRole } = useAuth()
  const toast = useToast()
  const isEdit = mode === 'edit'
  const isSuper = hasRole('SUPERADMIN')
  const createMut = useCreateDevice()
  const updateMut = useUpdateDevice()
  const { data: vendorsResp } = useVendors()
  const vendors = vendorsResp?.data ?? []
  const { data: tenantsResp } = useTenants(isSuper)
  const tenants = isSuper ? (tenantsResp?.data ?? []) : []

  const [tenantId, setTenantId] = useState('')
  const [oui, setOui] = useState(device?.oui ?? '')
  const [serial, setSerial] = useState(device?.serial_number ?? '')
  const [vendorId, setVendorId] = useState(device?.vendor_id ? String(device.vendor_id) : '')
  const [modelId, setModelId] = useState(device?.device_model_id ? String(device.device_model_id) : '')
  const [productClass, setProductClass] = useState(device?.product_class ?? '')
  const [mac, setMac] = useState(device?.mac_address ?? '')
  const [notes, setNotes] = useState(device?.notes ?? '')
  const [lat, setLat] = useState(device?.latitude?.toString() ?? '')
  const [lon, setLon] = useState(device?.longitude?.toString() ?? '')
  const [crUrl, setCrUrl] = useState(device?.connection_request_url ?? '')
  const [crUser, setCrUser] = useState('')
  const [crPass, setCrPass] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({})

  // API membalas null (bukan []) untuk vendor tanpa model.
  const { data: modelsData } = useDeviceModels(vendorId ? Number(vendorId) : undefined)
  const models = modelsData ?? []
  const busy = createMut.isPending || updateMut.isPending

  function validate(): Record<string, string> {
    const e: Record<string, string> = {}
    if (!isEdit) {
      if (isSuper && !tenantId) e.tenant = 'Pilih tenant pemilik perangkat.'
      if (!/^[0-9A-Za-z]{6}$/.test(oui.trim())) e.oui = 'OUI harus 6 karakter huruf/angka (mis. 00D0D0).'
      if (serial.trim() === '') e.serial = 'Serial number wajib diisi.'
      else if (serial.trim().length > 128) e.serial = 'Maks. 128 karakter.'
    }
    if (mac.trim() !== '' && !MAC_RE.test(mac.trim())) e.mac = 'Format MAC tidak valid (mis. AA:BB:CC:DD:EE:FF).'
    if (notes.length > 255) e.notes = 'Maks. 255 karakter.'
    const hasLat = lat.trim() !== ''
    const hasLon = lon.trim() !== ''
    if (hasLat !== hasLon) e.lat = 'Latitude dan longitude harus diisi berpasangan.'
    else if (hasLat) {
      const la = Number(lat)
      const lo = Number(lon)
      if (!Number.isFinite(la) || la < -90 || la > 90) e.lat = 'Latitude harus angka −90 s/d 90.'
      if (!Number.isFinite(lo) || lo < -180 || lo > 180) e.lon = 'Longitude harus angka −180 s/d 180.'
    }
    if (crUrl.trim() !== '' && !/^https?:\/\/\S+$/i.test(crUrl.trim())) e.crUrl = 'Harus URL http:// atau https://.'
    return e
  }

  async function handleSubmit(ev: FormEvent) {
    ev.preventDefault()
    setError(null)
    const errs = validate()
    setFieldErrors(errs)
    if (Object.keys(errs).length > 0) return
    const hasGeo = lat.trim() !== '' && lon.trim() !== ''
    try {
      if (isEdit && device) {
        const d = await updateMut.mutateAsync({
          id: device.id,
          // string kosong = kosongkan nilai; field yang tak diubah tetap dikirim apa adanya (idempoten)
          notes,
          product_class: productClass,
          mac_address: mac.trim(),
          connection_request_url: crUrl.trim(),
          ...(vendorId ? { vendor_id: Number(vendorId) } : {}),
          ...(modelId ? { device_model_id: Number(modelId) } : {}),
          ...(hasGeo ? { latitude: Number(lat), longitude: Number(lon) } : {}),
          // kredensial hanya dikirim bila diisi (nilai tersimpan tidak pernah ditampilkan)
          ...(crUser.trim() ? { connection_request_username: crUser.trim() } : {}),
          ...(crPass ? { connection_request_password: crPass } : {}),
        })
        toast.success(`Perangkat ${d.serial_number} diperbarui.`)
        onSaved?.(d)
      } else {
        const d = await createMut.mutateAsync({
          ...(isSuper ? { tenant_id: Number(tenantId) } : {}),
          oui: oui.trim().toUpperCase(),
          serial_number: serial.trim(),
          ...(productClass.trim() ? { product_class: productClass.trim() } : {}),
          ...(mac.trim() ? { mac_address: mac.trim() } : {}),
          ...(notes.trim() ? { notes: notes.trim() } : {}),
          ...(vendorId ? { vendor_id: Number(vendorId) } : {}),
          ...(modelId ? { device_model_id: Number(modelId) } : {}),
          ...(hasGeo ? { latitude: Number(lat), longitude: Number(lon) } : {}),
          ...(crUrl.trim() ? { connection_request_url: crUrl.trim() } : {}),
          ...(crUser.trim() ? { connection_request_username: crUser.trim() } : {}),
          ...(crPass ? { connection_request_password: crPass } : {}),
        })
        toast.success(`Perangkat ${d.serial_number} didaftarkan. Status berubah otomatis saat CPE pertama kali Inform.`)
        onSaved?.(d)
      }
      onClose()
    } catch (e) {
      setError(e instanceof ApiError ? e.message.replace(/^input tidak valid: /, '') : 'Gagal menyimpan perangkat')
    }
  }

  const fe = (k: string) => (fieldErrors[k] ? <p className="mt-1 text-xs text-red-600 dark:text-red-400">{fieldErrors[k]}</p> : null)

  return (
    <Modal title={isEdit ? 'Edit Perangkat' : 'Tambah Perangkat'} onClose={onClose} wide dismissable={!busy}>
      <form onSubmit={handleSubmit} noValidate className="space-y-4">
        {!isEdit && (
          <p className="rounded-lg bg-indigo-50 px-3 py-2 text-xs leading-relaxed text-indigo-700 dark:bg-indigo-500/10 dark:text-indigo-300">
            Mendaftarkan perangkat <strong>sebelum</strong> CPE terhubung (pra-registrasi). Begitu CPE dengan OUI &amp; serial ini mengirim Inform
            ke ACS memakai kredensial tenant yang sama, perangkat langsung terhubung dan tenantnya sudah terkunci.
          </p>
        )}

        {isEdit && device && (
          <div className="grid grid-cols-2 gap-3 rounded-lg bg-slate-50 px-3.5 py-2.5 text-xs dark:bg-slate-800/50">
            <div>
              <span className="text-slate-400">OUI</span>
              <p className="font-mono text-slate-700 dark:text-slate-300">{device.oui ?? '—'}</p>
            </div>
            <div>
              <span className="text-slate-400">Serial number</span>
              <p className="break-all font-mono text-slate-700 dark:text-slate-300">{device.serial_number}</p>
            </div>
            <p className="col-span-2 text-slate-400">OUI dan serial adalah identitas TR-069 dan tidak dapat diubah. Salah daftar? Hapus perangkat lalu daftarkan ulang.</p>
          </div>
        )}

        {!isEdit && (
          <>
            {isSuper && (
              <div>
                <label htmlFor="df-tenant" className={labelCls}>
                  Tenant <span className="text-red-500">*</span>
                </label>
                <select id="df-tenant" value={tenantId} onChange={(e) => setTenantId(e.target.value)} className={inputCls} aria-invalid={!!fieldErrors.tenant}>
                  <option value="">Pilih tenant…</option>
                  {tenants.map((t) => (
                    <option key={t.id} value={t.id}>
                      {t.name} ({t.code})
                    </option>
                  ))}
                </select>
                {fe('tenant')}
              </div>
            )}
            <div className="grid gap-4 sm:grid-cols-2">
              <div>
                <label htmlFor="df-oui" className={labelCls}>
                  OUI <span className="text-red-500">*</span>
                </label>
                <input
                  id="df-oui"
                  value={oui}
                  onChange={(e) => setOui(e.target.value.toUpperCase())}
                  maxLength={6}
                  placeholder="00D0D0"
                  autoComplete="off"
                  spellCheck={false}
                  className={`${inputCls} font-mono`}
                  aria-invalid={!!fieldErrors.oui}
                />
                {fieldErrors.oui ? fe('oui') : <p className={hintCls}>6 karakter (umumnya hex); vendor dikenali otomatis dari OUI bila ada di katalog.</p>}
              </div>
              <div>
                <label htmlFor="df-serial" className={labelCls}>
                  Serial number <span className="text-red-500">*</span>
                </label>
                <input
                  id="df-serial"
                  value={serial}
                  onChange={(e) => setSerial(e.target.value)}
                  maxLength={128}
                  placeholder="ZTEGC8A1B2C3"
                  autoComplete="off"
                  spellCheck={false}
                  className={`${inputCls} font-mono`}
                  aria-invalid={!!fieldErrors.serial}
                />
                {fe('serial')}
              </div>
            </div>
          </>
        )}

        <div className="grid gap-4 sm:grid-cols-2">
          <div>
            <label htmlFor="df-vendor" className={labelCls}>
              Vendor
            </label>
            <select
              id="df-vendor"
              value={vendorId}
              onChange={(e) => {
                setVendorId(e.target.value)
                setModelId('')
              }}
              className={inputCls}
            >
              <option value="">{isEdit ? '(tidak diubah)' : 'Otomatis dari OUI'}</option>
              {vendors.map((v) => (
                <option key={v.id} value={v.id}>
                  {v.name}
                </option>
              ))}
            </select>
          </div>
          <div>
            <label htmlFor="df-model" className={labelCls}>
              Model
            </label>
            <select id="df-model" value={modelId} onChange={(e) => setModelId(e.target.value)} disabled={!vendorId} className={inputCls}>
              <option value="">{vendorId ? (isEdit ? '(tidak diubah)' : 'Otomatis dari product class') : 'Pilih vendor dulu'}</option>
              {models.map((m) => (
                <option key={m.id} value={m.id}>
                  {m.model_name}
                </option>
              ))}
            </select>
          </div>
          <div>
            <label htmlFor="df-pc" className={labelCls}>
              Product class
            </label>
            <input id="df-pc" value={productClass} onChange={(e) => setProductClass(e.target.value)} maxLength={128} placeholder="F670L" className={inputCls} />
          </div>
          <div>
            <label htmlFor="df-mac" className={labelCls}>
              MAC address
            </label>
            <input id="df-mac" value={mac} onChange={(e) => setMac(e.target.value)} placeholder="AA:BB:CC:DD:EE:FF" autoComplete="off" className={`${inputCls} font-mono`} aria-invalid={!!fieldErrors.mac} />
            {fe('mac')}
          </div>
        </div>

        <div>
          <label htmlFor="df-notes" className={labelCls}>
            Catatan
          </label>
          <textarea id="df-notes" value={notes} onChange={(e) => setNotes(e.target.value)} rows={2} maxLength={255} placeholder="mis. ONT lantai 2 ruang server, pelanggan korporat" className={inputCls} />
          <div className="flex justify-between">
            {fe('notes') ?? <span />}
            <span className="mt-1 text-xs text-slate-400">{notes.length}/255</span>
          </div>
        </div>

        <div className="grid gap-4 sm:grid-cols-2">
          <div>
            <label htmlFor="df-lat" className={labelCls}>
              Latitude
            </label>
            <input id="df-lat" type="number" step="any" value={lat} onChange={(e) => setLat(e.target.value)} placeholder="-6.200000" className={inputCls} aria-invalid={!!fieldErrors.lat} />
            {fe('lat')}
          </div>
          <div>
            <label htmlFor="df-lon" className={labelCls}>
              Longitude
            </label>
            <input id="df-lon" type="number" step="any" value={lon} onChange={(e) => setLon(e.target.value)} placeholder="106.816666" className={inputCls} aria-invalid={!!fieldErrors.lon} />
            {fe('lon')}
          </div>
        </div>
        {isEdit && <p className={`${hintCls} -mt-2`}>Koordinat dipakai peta (GIS) di daftar perangkat. Koordinat bisa diubah, tidak bisa dikosongkan.</p>}

        <details className="group rounded-lg border border-slate-200 dark:border-slate-700">
          <summary className="flex cursor-pointer list-none items-center justify-between px-3.5 py-2.5 text-sm font-medium text-slate-700 dark:text-slate-300">
            Connection Request (opsional)
            <ChevronDown className="h-4 w-4 text-slate-400 transition-transform group-open:rotate-180" />
          </summary>
          <div className="space-y-4 border-t border-slate-200 px-3.5 py-3.5 dark:border-slate-700">
            <div>
              <label htmlFor="df-crurl" className={labelCls}>
                URL
              </label>
              <input id="df-crurl" value={crUrl} onChange={(e) => setCrUrl(e.target.value)} placeholder="http://203.0.113.7:7547/" className={`${inputCls} font-mono`} aria-invalid={!!fieldErrors.crUrl} />
              {fieldErrors.crUrl ? fe('crUrl') : <p className={hintCls}>Biasanya terisi otomatis dari Inform; isi manual untuk skenario NAT.</p>}
            </div>
            <div className="grid gap-4 sm:grid-cols-2">
              <div>
                <label htmlFor="df-cruser" className={labelCls}>
                  Username
                </label>
                <input id="df-cruser" value={crUser} onChange={(e) => setCrUser(e.target.value)} autoComplete="off" placeholder={isEdit ? '(tersimpan — isi untuk mengganti)' : ''} className={inputCls} />
              </div>
              <div>
                <label htmlFor="df-crpass" className={labelCls}>
                  Password
                </label>
                <input id="df-crpass" type="password" value={crPass} onChange={(e) => setCrPass(e.target.value)} autoComplete="new-password" placeholder={isEdit ? '(tersimpan — isi untuk mengganti)' : ''} className={inputCls} />
              </div>
            </div>
            <p className={`${hintCls} -mt-2`}>Disimpan terenkripsi dan tidak pernah ditampilkan kembali. ACS menjawab tantangan Digest/Basic dari CPE memakai kredensial ini.</p>
          </div>
        </details>

        {error && (
          <div role="alert" className="flex items-start gap-2 rounded-lg bg-red-50 px-3.5 py-2.5 text-sm text-red-700 dark:bg-red-500/10 dark:text-red-400">
            <AlertCircle className="mt-0.5 h-4 w-4 shrink-0" />
            <span>{error}</span>
          </div>
        )}

        <div className="flex justify-end gap-2.5 pt-1">
          <button type="button" onClick={onClose} disabled={busy} className="rounded-lg px-3.5 py-2 text-sm font-medium text-slate-600 transition-colors hover:bg-slate-100 dark:text-slate-400 dark:hover:bg-slate-800">
            Batal
          </button>
          <button type="submit" disabled={busy} className="inline-flex items-center gap-2 rounded-lg bg-indigo-600 px-4 py-2 text-sm font-semibold text-white transition-colors hover:bg-indigo-500 disabled:opacity-60">
            {busy && <Spinner className="h-4 w-4 text-white" />}
            {isEdit ? 'Simpan perubahan' : 'Daftarkan perangkat'}
          </button>
        </div>
      </form>
    </Modal>
  )
}
