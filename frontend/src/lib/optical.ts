// Klasifikasi level terima optik (RX) ONU/ONT GPON untuk kolom "Redaman" di daftar perangkat. Ambang mengikuti praktik
// umum GPON kelas B+ (sensitivitas penerima ±-27/-28 dBm, titik overload ±-8 dBm); sesuaikan bila kebijakan jaringan berbeda.

export type OpticalLevel = 'ok' | 'warn' | 'bad'

/** Batas atas "normal": RX lebih lemah dari ini mulai waspada. */
export const RX_WARN_BELOW_DBM = -25
/** RX lebih lemah dari ini = kritis (dekat/di bawah sensitivitas penerima). */
export const RX_BAD_BELOW_DBM = -27
/** RX lebih kuat dari ini = kritis (receiver overload). */
export const RX_OVERLOAD_ABOVE_DBM = -8

export function opticalLevel(rxDbm: number | null | undefined): OpticalLevel | null {
  if (rxDbm == null || !Number.isFinite(rxDbm)) return null
  if (rxDbm < RX_BAD_BELOW_DBM || rxDbm > RX_OVERLOAD_ABOVE_DBM) return 'bad'
  if (rxDbm < RX_WARN_BELOW_DBM) return 'warn'
  return 'ok'
}

export const OPTICAL_LEVEL_HINT: Record<OpticalLevel, string> = {
  ok: 'Normal',
  warn: 'Waspada: sinyal mulai lemah',
  bad: 'Kritis: di luar rentang aman',
}
