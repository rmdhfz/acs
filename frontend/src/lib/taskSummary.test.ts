import { describe, expect, it } from 'vitest'
import { friendlyParamName, retryKind, summarizeTask } from './taskSummary'

const W = 'InternetGatewayDevice.LANDevice.1.WLANConfiguration'

describe('summarizeTask', () => {
  it('ubah SSID + password satu WLAN: judul jelas, password ditutup', () => {
    const s = summarizeTask('SET_PARAMETER_VALUES', { values: { [`${W}.2.SSID`]: 'Rumah Budi', [`${W}.2.PreSharedKey.1.KeyPassphrase`]: 'rahasia12345' } })
    expect(s.title).toBe('Ubah WiFi #2')
    expect(s.details).toContain('SSID WiFi #2 → Rumah Budi')
    expect(s.details).toContain('Password WiFi #2 → ••••••')
    expect(JSON.stringify(s)).not.toContain('rahasia12345')
  })

  it('nilai [redacted] dari backend (NOC) tetap tertutup; beberapa SSID; parameter non-WiFi', () => {
    const redacted = summarizeTask('SET_PARAMETER_VALUES', { values: { [`${W}.1.PreSharedKey.1.KeyPassphrase`]: '[redacted]', [`${W}.3.SSID`]: 'X' } })
    expect(redacted.title).toBe('Ubah WiFi (2 SSID)')
    expect(redacted.details).toContain('Password WiFi #1 → ••••••')
    const other = summarizeTask('SET_PARAMETER_VALUES', { values: { 'InternetGatewayDevice.ManagementServer.PeriodicInformInterval': '300' } })
    expect(other).toEqual({ title: 'Ubah 1 parameter', details: ['Interval Periodic Inform → 300'] })
  })

  it('pengaturan lanjutan WiFi dibaca manusia (terlihat/tersembunyi, aktif/nonaktif, maks. klien, prioritas)', () => {
    const s = summarizeTask('SET_PARAMETER_VALUES', {
      values: {
        [`${W}.2.SSIDAdvertisementEnabled`]: '0',
        [`${W}.2.Enable`]: '1',
        [`${W}.2.X_ZTE-COM_MaxUserNum`]: '16',
        [`${W}.2.X_ZTE-COM_SSIDIsolation`]: 'true',
        [`${W}.2.X_ZTE-COM_Priority`]: '3',
      },
    })
    expect(s.title).toBe('Ubah WiFi #2')
    expect(s.details).toEqual(
      expect.arrayContaining([
        'Siaran SSID WiFi #2 → tersembunyi',
        'Status aktif WiFi #2 → aktif',
        'Maks. klien WiFi #2 → 16',
        'Isolasi SSID WiFi #2 → aktif',
        'Prioritas WiFi #2 → 3',
      ]),
    )
  })

  it('ping diagnostik dikenali dari Host', () => {
    const s = summarizeTask('SET_PARAMETER_VALUES', { values: { 'InternetGatewayDevice.IPPingDiagnostics.Host': '8.8.8.8', 'InternetGatewayDevice.IPPingDiagnostics.NumberOfRepetitions': '4' } })
    expect(s.title).toBe('Jalankan Ping ke 8.8.8.8')
  })

  it('baca: cabang WLAN = "semua SSID", cabang lain, daftar nama', () => {
    expect(summarizeTask('GET_PARAMETER_VALUES', { names: [`${W}.`] }).title).toBe('Baca semua SSID WiFi')
    expect(summarizeTask('GET_PARAMETER_VALUES', { names: ['Device.WiFi.SSID.'] }).title).toBe('Baca semua SSID WiFi')
    expect(summarizeTask('GET_PARAMETER_VALUES', { names: ['InternetGatewayDevice.WANDevice.'] }).title).toBe('Baca cabang WANDevice')
    expect(summarizeTask('GET_PARAMETER_VALUES', { names: [`${W}.1.SSID`, 'A.B.C'] }).title).toBe('Baca 2 parameter')
  })

  it('jenis lain, parameter null/rusak tidak crash', () => {
    expect(summarizeTask('REBOOT', null).title).toBe('Restart perangkat')
    expect(summarizeTask('FACTORY_RESET', undefined).title).toBe('Kembalikan ke setelan pabrik')
    expect(summarizeTask('SET_PARAMETER_VALUES', 'bukan json').title).toBe('Ubah parameter')
    expect(summarizeTask('SET_PARAMETER_VALUES', '{"values":{"A.B":"1"}}').title).toBe('Ubah 1 parameter')
    expect(summarizeTask(undefined, {}).title).toBe('Task')
    expect(summarizeTask('JENIS_BARU', {}).title).toBe('JENIS_BARU')
  })
})

describe('friendlyParamName', () => {
  it('TR-181 dan fallback dua segmen terakhir', () => {
    expect(friendlyParamName('Device.WiFi.SSID.5.SSID')).toBe('SSID WiFi #5')
    expect(friendlyParamName('Device.WiFi.AccessPoint.2.Security.KeyPassphrase')).toBe('Password WiFi #2')
    expect(friendlyParamName('A.B.C.D')).toBe('C.D')
  })
})

describe('retryKind', () => {
  it('gagal/batal/timeout = coba lagi; pending yang menunggu retry = kirim sekarang; selebihnya tidak ada', () => {
    expect(retryKind('FAILED', 3, null)).toBe('retry')
    expect(retryKind('CANCELLED', 0, null)).toBe('retry')
    expect(retryKind('TIMEOUT', 1, null)).toBe('retry')
    expect(retryKind('PENDING', 2, null)).toBe('send-now')
    expect(retryKind('PENDING', 0, new Date(Date.now() + 60_000).toISOString())).toBe('send-now')
    expect(retryKind('PENDING', 0, null)).toBeNull()
    expect(retryKind('SENT', 0, null)).toBeNull()
    expect(retryKind('COMPLETED', 0, null)).toBeNull()
    expect(retryKind(undefined, 0, null)).toBeNull()
  })
})
