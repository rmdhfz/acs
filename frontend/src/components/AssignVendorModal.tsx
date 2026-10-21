import { useState, type FormEvent } from 'react'
import { Modal } from './Modal'
import { useAuth } from '../lib/auth'
import { useToast } from '../lib/toast'
import { ApiError } from '../lib/api'
import { useAddVendorOUI, useCreateVendor, useUpdateDevice, useVendors } from '../lib/hooks'
import type { Device } from '../lib/types'

// "Tetapkan vendor" untuk perangkat yang OUI-nya belum ada di katalog ("Belum dikenal"), tanpa migrasi/kode:
//  - Semua ADMIN/NOC: pilih vendor yang SUDAH ada → hanya perangkat ini (PATCH /devices/:id).
//  - SUPERADMIN (katalog bersifat global): juga boleh membuat vendor baru dan mendaftarkan OUI perangkat ke vendor itu,
//    sehingga perangkat lain dengan OUI sama langsung dikenali (menambah dukungan merek = operasi data).
// Mapping parameter tidak dibuat di sini: vendor tanpa mapping memakai fallback standar TR-098/TR-181 (vendor GENERIC).

const inputCls =
  'w-full rounded-lg border border-slate-300 px-3 py-2 text-sm text-slate-900 outline-none transition-colors focus:border-indigo-600 focus:ring-1 focus:ring-indigo-600 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-100'
const primaryBtnCls =
  'flex items-center justify-center gap-2 rounded-full bg-indigo-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-indigo-700 disabled:cursor-not-allowed disabled:opacity-60 dark:bg-indigo-300 dark:text-slate-900 dark:hover:bg-indigo-200'

const NEW = 'new'
const CODE_RE = /^[A-Z0-9_]{2,32}$/

export function AssignVendorModal({ device, onClose }: { device: Device; onClose: () => void }) {
  const { hasRole } = useAuth()
  const isSuper = hasRole('SUPERADMIN')
  const toast = useToast()
  const { data: vendorsResp } = useVendors()
  const vendors = (vendorsResp?.data ?? []).filter((v) => v.code !== 'GENERIC')
  const createVendor = useCreateVendor()
  const addOui = useAddVendorOUI()
  const updateDevice = useUpdateDevice()

  const [vendorId, setVendorId] = useState('')
  const [name, setName] = useState('')
  const [code, setCode] = useState('')
  const [registerOui, setRegisterOui] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const busy = createVendor.isPending || addOui.isPending || updateDevice.isPending
  const oui = (device.oui ?? '').trim().toUpperCase()
  const canRegisterOui = isSuper && /^[0-9A-F]{6}$/.test(oui)

  const onSubmit = async (e: FormEvent) => {
    e.preventDefault()
    setError(null)
    if (!vendorId) return setError('Pilih vendor.')
    try {
      let vid: number
      if (vendorId === NEW) {
        const n = name.trim()
        const c = code.trim().toUpperCase()
        if (!n || n.length > 128) return setError('Nama vendor wajib diisi (maks. 128 karakter).')
        if (!CODE_RE.test(c)) return setError('Kode vendor 2–32 karakter: huruf besar, angka, atau garis bawah.')
        vid = (await createVendor.mutateAsync({ code: c, name: n })).id
      } else {
        vid = Number(vendorId)
      }
      if (canRegisterOui && registerOui) {
        await addOui.mutateAsync({
          vendorId: vid,
          oui,
          notes: `Ditetapkan dari perangkat ${device.serial_number} (ProductClass ${device.product_class ?? '-'})`,
        })
      }
      await updateDevice.mutateAsync({ id: device.id, vendor_id: vid })
      toast.success(
        canRegisterOui && registerOui
          ? `Vendor ditetapkan dan OUI ${oui} didaftarkan: perangkat lain dengan OUI ini otomatis dikenali.`
          : 'Vendor ditetapkan untuk perangkat ini.',
        'Berhasil',
      )
      onClose()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Gagal menetapkan vendor')
    }
  }

  return (
    <Modal title="Tetapkan vendor perangkat" onClose={onClose}>
      <form onSubmit={onSubmit} className="space-y-4">
        <p className="text-xs text-slate-500 dark:text-slate-400">
          OUI <span className="font-mono">{oui || '-'}</span> · ProductClass <span className="font-mono">{device.product_class ?? '-'}</span> belum ada di katalog vendor.
          Perangkat tanpa vendor tetap bisa dikelola lewat mapping standar TR-098/TR-181; menetapkan vendor memungkinkan mapping khusus vendor/model.
        </p>
        <label className="block text-sm">
          <span className="mb-1 block text-slate-600 dark:text-slate-300">Vendor</span>
          <select value={vendorId} onChange={(e) => setVendorId(e.target.value)} className={inputCls}>
            <option value="">Pilih vendor…</option>
            {vendors.map((v) => (
              <option key={v.id} value={v.id}>
                {v.name}
              </option>
            ))}
            {isSuper && <option value={NEW}>+ Buat vendor baru…</option>}
          </select>
        </label>
        {vendorId === NEW && (
          <div className="grid gap-3 sm:grid-cols-2">
            <label className="block text-sm">
              <span className="mb-1 block text-slate-600 dark:text-slate-300">Nama vendor</span>
              <input value={name} onChange={(e) => setName(e.target.value)} maxLength={128} placeholder="mis. Optictimes" className={inputCls} />
            </label>
            <label className="block text-sm">
              <span className="mb-1 block text-slate-600 dark:text-slate-300">Kode</span>
              <input value={code} onChange={(e) => setCode(e.target.value.toUpperCase())} maxLength={32} placeholder="mis. OPTICTIMES" className={inputCls} />
            </label>
          </div>
        )}
        {canRegisterOui ? (
          <label className="flex items-start gap-2 text-sm text-slate-700 dark:text-slate-300">
            <input type="checkbox" checked={registerOui} onChange={(e) => setRegisterOui(e.target.checked)} className="mt-0.5 h-4 w-4 rounded border-slate-300 text-indigo-600" />
            <span>
              Daftarkan juga OUI <span className="font-mono">{oui}</span> ke vendor ini (perangkat lain dengan OUI sama otomatis dikenali).
            </span>
          </label>
        ) : (
          <p className="text-xs text-slate-400 dark:text-slate-500">
            {isSuper ? 'OUI perangkat tidak valid, hanya perangkat ini yang ditetapkan.' : 'Hanya perangkat ini yang ditetapkan. Pendaftaran OUI/vendor baru ke katalog dilakukan SUPERADMIN.'}
          </p>
        )}
        {error && (
          <p role="alert" className="text-sm text-red-600 dark:text-red-400">
            {error}
          </p>
        )}
        <button type="submit" disabled={busy} className={`${primaryBtnCls} w-full`}>
          {busy ? 'Menyimpan…' : 'Tetapkan vendor'}
        </button>
      </form>
    </Modal>
  )
}
