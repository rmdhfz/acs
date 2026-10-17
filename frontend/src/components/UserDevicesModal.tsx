import { useState } from 'react'
import { Link2, Search, Unlink } from 'lucide-react'
import { Modal } from './Modal'
import { PageSpinner } from './Spinner'
import { ApiError } from '../lib/api'
import { useToast } from '../lib/toast'
import { useDevices, useLinkUserDevice, useUnlinkUserDevice, useUserDevices } from '../lib/hooks'
import type { User } from '../lib/types'

const inputCls =
  'w-full rounded-lg border border-slate-300 py-2 pl-9 pr-3 text-sm text-slate-900 outline-none transition-colors focus:border-indigo-600 focus:ring-1 focus:ring-indigo-600 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-100'

// Tautan akun pelanggan (ENDUSER) ↔ perangkat: menentukan perangkat apa yang muncul di portal "WiFi Saya".
// Aturan ditegakkan backend (tenant sama, role ENDUSER, maks 20); UI hanya memandu.
export function UserDevicesModal({ user, onClose }: { user: User; onClose: () => void }) {
  const toast = useToast()
  const linked = useUserDevices(user.id)
  const link = useLinkUserDevice()
  const unlink = useUnlinkUserDevice()
  const [search, setSearch] = useState('')
  const term = search.trim()
  const results = useDevices({ search: term, page_size: 8 }, term.length >= 3)
  const linkedItems = linked.data?.data ?? []
  const linkedIds = new Set(linkedItems.map((d) => d.id))

  async function doLink(deviceId: number, serial: string) {
    try {
      await link.mutateAsync({ userId: user.id, deviceId })
      toast.success(`${serial} ditautkan ke ${user.username}.`)
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : 'Gagal menautkan perangkat')
    }
  }

  async function doUnlink(deviceId: number, serial: string) {
    try {
      await unlink.mutateAsync({ userId: user.id, deviceId })
      toast.success(`${serial} dilepas dari ${user.username}.`)
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : 'Gagal melepas tautan')
    }
  }

  return (
    <Modal title={`Perangkat pelanggan — ${user.username}`} onClose={onClose} wide>
      <div className="space-y-5">
        <p className="text-sm text-slate-600 dark:text-slate-400">
          Akun ini hanya melihat dan mengelola perangkat yang ditautkan di sini (menu <em>WiFi Saya</em>). Akun dan perangkat harus satu tenant.
        </p>

        <section aria-label="Perangkat tertaut">
          <h3 className="mb-2 text-xs font-medium uppercase tracking-wide text-slate-500 dark:text-slate-400">Tertaut</h3>
          {linked.isLoading ? (
            <PageSpinner />
          ) : linkedItems.length === 0 ? (
            <p className="rounded-lg border border-dashed border-slate-300 px-3 py-4 text-center text-sm text-slate-500 dark:border-slate-700 dark:text-slate-400">
              Belum ada perangkat tertaut — pelanggan belum melihat apa pun di portal.
            </p>
          ) : (
            <ul className="divide-y divide-slate-100 rounded-lg border border-slate-200 dark:divide-slate-800 dark:border-slate-800">
              {linkedItems.map((d) => (
                <li key={d.id} className="flex items-center justify-between gap-3 px-3 py-2">
                  <div className="min-w-0">
                    <p className="truncate font-mono text-sm text-slate-800 dark:text-slate-200">{d.serial_number}</p>
                    <p className="text-xs text-slate-500 dark:text-slate-400">
                      {d.model ?? 'model tidak diketahui'} · {d.status || '-'}
                    </p>
                  </div>
                  <button
                    type="button"
                    onClick={() => doUnlink(d.id, d.serial_number)}
                    disabled={unlink.isPending}
                    aria-label={`Lepas ${d.serial_number}`}
                    className="flex shrink-0 items-center gap-1 text-xs font-medium text-red-600 hover:text-red-800 disabled:opacity-50 dark:text-red-400 dark:hover:text-red-300"
                  >
                    <Unlink className="h-3.5 w-3.5" /> Lepas
                  </button>
                </li>
              ))}
            </ul>
          )}
        </section>

        <section aria-label="Tautkan perangkat">
          <h3 className="mb-2 text-xs font-medium uppercase tracking-wide text-slate-500 dark:text-slate-400">Tautkan perangkat</h3>
          <div className="relative">
            <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-slate-500" />
            <input
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              aria-label="Cari perangkat berdasarkan serial number atau MAC"
              placeholder="Cari serial number / MAC (min. 3 karakter)"
              maxLength={64}
              className={inputCls}
            />
          </div>
          {term.length >= 3 && (
            <ul className="mt-2 divide-y divide-slate-100 rounded-lg border border-slate-200 dark:divide-slate-800 dark:border-slate-800">
              {(results.data?.data ?? []).length === 0 && !results.isLoading && (
                <li className="px-3 py-3 text-sm text-slate-500 dark:text-slate-400">Tidak ada perangkat yang cocok.</li>
              )}
              {(results.data?.data ?? []).map((d) => {
                const sameTenant = d.tenant_id !== null && d.tenant_id === user.tenant_id
                const already = linkedIds.has(d.id)
                return (
                  <li key={d.id} className="flex items-center justify-between gap-3 px-3 py-2">
                    <div className="min-w-0">
                      <p className="truncate font-mono text-sm text-slate-800 dark:text-slate-200">{d.serial_number}</p>
                      <p className="text-xs text-slate-500 dark:text-slate-400">
                        {d.product_class ?? 'model tidak diketahui'}
                        {!sameTenant && ' · beda tenant dengan akun ini'}
                      </p>
                    </div>
                    <button
                      type="button"
                      onClick={() => doLink(d.id, d.serial_number)}
                      disabled={!sameTenant || already || link.isPending}
                      className="flex shrink-0 items-center gap-1 rounded-full border border-slate-300 px-2.5 py-1 text-xs font-medium text-slate-700 hover:bg-slate-50 disabled:cursor-not-allowed disabled:opacity-50 dark:border-slate-700 dark:text-slate-300 dark:hover:bg-slate-800"
                    >
                      <Link2 className="h-3.5 w-3.5" /> {already ? 'Tertaut' : 'Tautkan'}
                    </button>
                  </li>
                )
              })}
            </ul>
          )}
        </section>
      </div>
    </Modal>
  )
}
