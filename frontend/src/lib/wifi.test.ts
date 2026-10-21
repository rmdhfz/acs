import { describe, expect, it } from 'vitest'
import { detectModel, parseWifi, wifiReadNames } from './wifi'
import type { DeviceParameter } from './types'

const p = (name: string, value: string, updated = '2026-10-21T03:00:00Z'): DeviceParameter => ({
  id: 0, device_id: 1, parameter_name: name, parameter_value: value, writable: true, created_at: '', updated_at: updated,
})

const W = 'InternetGatewayDevice.LANDevice.1.WLANConfiguration'

describe('parseWifi TR-098', () => {
  // ZTE F660: 8 WLAN (1-4 = 2,4 GHz, 5-8 = 5 GHz)
  const params: DeviceParameter[] = [
    p('InternetGatewayDevice.DeviceInfo.SoftwareVersion', 'V5.2.10P2T10'),
    ...Array.from({ length: 8 }, (_, i) => i + 1).flatMap((n) => [
      p(`${W}.${n}.SSID`, `Rainara-${n}`),
      p(`${W}.${n}.Enable`, n === 3 ? '0' : '1'),
      p(`${W}.${n}.Channel`, n <= 4 ? '6' : '36'),
      p(`${W}.${n}.BeaconType`, n === 2 ? 'None' : '11i'),
      p(`${W}.${n}.IEEE11iAuthenticationMode`, 'PSKAuthentication'),
      p(`${W}.${n}.TotalAssociations`, String(n)),
      p(`${W}.${n}.PreSharedKey.1.KeyPassphrase`, n === 1 ? 'rahasia-123' : ''),
      p(`${W}.${n}.AssociatedDevice.1.AssociatedDeviceMACAddress`, 'AA:BB'),
    ]),
  ]

  it('mengenali 8 SSID berurutan beserta band, status, keamanan, klien', () => {
    const { model, rows } = parseWifi(params)
    expect(model).toBe('TR098')
    expect(rows.map((r) => r.index)).toEqual([1, 2, 3, 4, 5, 6, 7, 8])
    expect(rows[0]).toMatchObject({ ssid: 'Rainara-1', band: '2,4 GHz', enabled: true, security: '11i · PSKAuthentication', clients: '1', passphrase: 'rahasia-123' })
    expect(rows[1].security).toBe('Terbuka')
    expect(rows[2].enabled).toBe(false)
    expect(rows[4]).toMatchObject({ band: '5 GHz', channel: '36' })
    expect(rows[0].passphrasePath).toBe(`${W}.1.PreSharedKey.1.KeyPassphrase`)
  })

  it('path Enable & klien tersambung dari AssociatedDevice (lintas vendor lewat akhiran nama field)', () => {
    const rows = parseWifi([
      p(`${W}.1.SSID`, 'A'),
      p(`${W}.1.Enable`, '1'),
      p(`${W}.1.AssociatedDevice.1.AssociatedDeviceMACAddress`, 'aa:01'),
      p(`${W}.1.AssociatedDevice.1.AssociatedDeviceIPAddress`, '192.168.1.2'),
      p(`${W}.1.AssociatedDevice.1.X_ZTE-COM_DeviceName`, 'HP Budi'),
      p(`${W}.1.AssociatedDevice.1.X_ZTE-COM_RSSI`, '-57'),
      p(`${W}.1.AssociatedDevice.2.AssociatedDeviceMACAddress`, 'aa:02'),
      p(`${W}.2.SSID`, 'B'),
    ]).rows
    expect(rows[0].enablePath).toBe(`${W}.1.Enable`)
    expect(rows[0].clientList).toEqual([
      { mac: 'aa:01', ip: '192.168.1.2', name: 'HP Budi', rssi: '-57' },
      { mac: 'aa:02', ip: null, name: null, rssi: null },
    ])
    expect(rows[1].enablePath).toBe(`${W}.2.Enable`) // dibentuk dari instance bila ONT belum melaporkannya
    expect(rows[1].clientList).toEqual([])
  })

  it('pengaturan lanjutan: hide (terbalik dari SSIDAdvertisementEnabled), maks. klien, isolasi, prioritas — hanya bila dilaporkan ONT', () => {
    const rows = parseWifi([
      p(`${W}.1.SSID`, 'A'),
      p(`${W}.1.SSIDAdvertisementEnabled`, 'false'),
      p(`${W}.1.X_ZTE-COM_MaxUserNum`, '16'),
      p(`${W}.1.X_ZTE-COM_SSIDIsolation`, '1'),
      p(`${W}.1.X_ZTE-COM_Priority`, '3'),
      p(`${W}.1.Stats.UnknownProtoPacketsReceived`, '5'),
      p(`${W}.2.SSID`, 'B'),
      p(`${W}.2.SSIDAdvertisementEnabled`, 'true'),
      p(`${W}.2.X_ZTE-COM_MaxUserNum`, '32'),
    ]).rows
    expect(rows[0]).toMatchObject({
      hidden: true,
      advertisePath: `${W}.1.SSIDAdvertisementEnabled`,
      maxClients: '16',
      maxClientsPath: `${W}.1.X_ZTE-COM_MaxUserNum`,
      isolation: true,
      isolationPath: `${W}.1.X_ZTE-COM_SSIDIsolation`,
      priority: '3',
      priorityPath: `${W}.1.X_ZTE-COM_Priority`,
    })
    // SSID #2: hide dilaporkan (terlihat), isolasi/prioritas TIDAK dilaporkan → null (tidak ditebak)
    expect(rows[1]).toMatchObject({ hidden: false, maxClients: '32', isolation: null, isolationPath: null, priority: null, priorityPath: null })
  })

  it('hide belum terbaca → null, tetapi path standar SSIDAdvertisementEnabled tetap dibentuk dari instance (TR-098 & TR-181)', () => {
    expect(parseWifi([p(`${W}.3.SSID`, 'C')]).rows[0]).toMatchObject({ hidden: null, advertisePath: `${W}.3.SSIDAdvertisementEnabled`, maxClientsPath: null })
    const r181 = parseWifi([
      p('Device.DeviceInfo.X', '1'),
      p('Device.WiFi.SSID.1.SSID', 'K'),
      p('Device.WiFi.AccessPoint.1.SSIDAdvertisementEnabled', 'false'),
      p('Device.WiFi.AccessPoint.1.MaxAssociatedDevices', '20'),
      p('Device.WiFi.AccessPoint.1.IsolationEnable', 'true'),
    ]).rows[0]
    expect(r181).toMatchObject({ hidden: true, maxClients: '20', isolation: true, isolationPath: 'Device.WiFi.AccessPoint.1.IsolationEnable' })
  })

  it('tanpa parameter WLAN: kosong; path tulis default dibentuk dari instance saat SSID ada tanpa passphrase', () => {
    expect(parseWifi([p('InternetGatewayDevice.DeviceInfo.SoftwareVersion', 'x')]).rows).toEqual([])
    const only = parseWifi([p(`${W}.1.SSID`, 'A')]).rows[0]
    expect(only.passphrase).toBeNull()
    expect(only.ssidPath).toBe(`${W}.1.SSID`)
    expect(only.passphrasePath).toBe(`${W}.1.PreSharedKey.1.KeyPassphrase`)
  })

  it('nilai [redacted] dipertahankan apa adanya (non-ADMIN)', () => {
    expect(parseWifi([p(`${W}.1.SSID`, 'A'), p(`${W}.1.PreSharedKey.1.KeyPassphrase`, '[redacted]')]).rows[0].passphrase).toBe('[redacted]')
  })
})

describe('parseWifi TR-181', () => {
  const params = [
    p('Device.DeviceInfo.SoftwareVersion', 'x'),
    p('Device.WiFi.Radio.1.OperatingFrequencyBand', '2.4GHz'),
    p('Device.WiFi.Radio.1.Channel', '1'),
    p('Device.WiFi.Radio.2.OperatingFrequencyBand', '5GHz'),
    p('Device.WiFi.SSID.1.SSID', 'Kantor'),
    p('Device.WiFi.SSID.1.Enable', 'true'),
    p('Device.WiFi.SSID.1.LowerLayers', 'Device.WiFi.Radio.1.'),
    p('Device.WiFi.SSID.2.SSID', 'Kantor-5G'),
    p('Device.WiFi.SSID.2.LowerLayers', 'Device.WiFi.Radio.2.'),
    p('Device.WiFi.AccessPoint.1.Security.ModeEnabled', 'WPA2-Personal'),
    p('Device.WiFi.AccessPoint.1.Security.KeyPassphrase', 'abc12345'),
    p('Device.WiFi.AccessPoint.1.AssociatedDeviceNumberOfEntries', '3'),
  ]
  it('menggabungkan SSID, AccessPoint, dan Radio per instance', () => {
    const { model, rows } = parseWifi(params)
    expect(model).toBe('TR181')
    expect(rows).toHaveLength(2)
    expect(rows[0]).toMatchObject({ ssid: 'Kantor', band: '2,4 GHz', channel: '1', security: 'WPA2-Personal', clients: '3', passphrase: 'abc12345', enabled: true })
    expect(rows[1]).toMatchObject({ band: '5 GHz', passphrase: null })
    expect(rows[1].passphrasePath).toBe('Device.WiFi.AccessPoint.2.Security.KeyPassphrase')
  })
})

describe('wifiReadNames / detectModel', () => {
  it('memilih cabang yang sesuai data model', () => {
    expect(wifiReadNames('TR098')).toEqual(['InternetGatewayDevice.LANDevice.1.WLANConfiguration.'])
    expect(wifiReadNames('TR181')).toContain('Device.WiFi.SSID.')
    expect(detectModel(undefined)).toBe('TR098')
    expect(detectModel([p('Device.X', '1')])).toBe('TR181')
  })
})
