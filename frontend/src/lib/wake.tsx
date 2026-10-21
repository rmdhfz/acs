import { useConfirm } from './confirm'
import { ApiError } from './api'
import { useToast } from './toast'
import { useTriggerConnectionRequest } from './hooks'
import type { Device } from './types'

// "Refresh ONT" = Connection Request (TR-069 §3.2.2): ACS menyuruh ONT Inform SEKARANG tanpa reboot, jadi layanan
// pelanggan tidak terganggu. Satu hook dipakai di header detail, tab WiFi, dan Overview agar perilaku & pesannya sama.

type WakeDevice = Pick<Device, 'id' | 'connection_request_url' | 'connection_request_credentials_set'>

export function useWakeDevice(device: WakeDevice | undefined, onNeedCredentials?: () => void) {
  const connReq = useTriggerConnectionRequest()
  const toast = useToast()
  const confirm = useConfirm()

  const hasUrl = !!device?.connection_request_url
  // undefined (API lama / belum dimuat) dianggap "tidak diketahui" → tidak memblokir.
  const credsMissing = device?.connection_request_credentials_set === false

  const hint = !hasUrl
    ? 'URL Connection Request belum diketahui — ACS menyimpannya otomatis saat ONT Inform berikutnya (atau isi manual lewat Edit).'
    : credsMissing
      ? 'Kredensial Connection Request belum diisi (Edit). ONT biasanya menolak tanpa kredensial.'
      : 'Suruh ONT Inform sekarang tanpa reboot — layanan pelanggan tidak terganggu.'

  const wake = async () => {
    if (!device || !hasUrl) return
    if (credsMissing) {
      const sendAnyway = await confirm({
        title: 'Kredensial Connection Request belum diisi',
        message:
          'ONT hampir selalu meminta autentikasi (Digest) untuk Connection Request dan akan menolak (401) bila username/password belum diisi. ' +
          'Isi sama persis dengan halaman TR-069 di ONT, lewat Edit. Tetap kirim sekarang?',
        confirmLabel: 'Kirim tetap',
        cancelLabel: 'Isi kredensial',
      })
      if (!sendAnyway) {
        onNeedCredentials?.()
        return
      }
    }
    try {
      await connReq.mutateAsync(device.id)
      toast.success('Refresh dikirim: ONT akan Inform dalam beberapa detik. Tanpa reboot — layanan pelanggan tidak terganggu.', 'Berhasil')
    } catch (e) {
      toast.error(e instanceof ApiError ? e.message : 'Gagal mengirim Connection Request')
    }
  }

  /** Bangunkan diam-diam setelah mengantre task (agar langsung dijalankan); hanya bila URL & kredensial siap, kegagalan diabaikan. */
  const wakeQuiet = () => {
    if (device && hasUrl && !credsMissing) connReq.mutate(device.id, { onError: () => {} })
  }

  return { wake, wakeQuiet, pending: connReq.isPending, hasUrl, credsMissing, hint }
}
