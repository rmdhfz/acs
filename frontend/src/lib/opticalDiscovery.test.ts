import { describe, expect, it } from 'vitest'
import { applyScale, findOpticalCandidates, suggestScale } from './opticalDiscovery'
import type { DeviceParameter } from './types'

const p = (name: string, value: string): DeviceParameter => ({
  id: 0, device_id: 1, parameter_name: name, parameter_value: value, writable: false, created_at: '', updated_at: '',
})

const W = 'InternetGatewayDevice.WANDevice.1.'

describe('suggestScale / applyScale', () => {
  it('menyarankan skala pertama yang membuat nilai masuk rentang wajar', () => {
    expect(suggestScale('rx', -18.21)).toBe(1)
    expect(suggestScale('rx', -1821)).toBe(0.01)
    expect(suggestScale('rx', -18210)).toBe(0.001)
    expect(suggestScale('tx', 2.74)).toBe(1)
    expect(suggestScale('temperature', 4440)).toBe(0.01)
    expect(suggestScale('voltage', 3170)).toBe(0.001)
    expect(suggestScale('voltage', 31700)).toBe(0.0001)
    // Ambigu secara inheren (84,3 mA atau 8,43 mA?): saran = skala pertama yang masuk rentang; operator mencocokkan dengan GUI ONT.
    expect(suggestScale('bias', 8430)).toBe(0.01)
    expect(suggestScale('rx', -5_000_000)).toBeNull()
  })
  it('pratinjau konversi', () => {
    expect(applyScale('-18210', 0.001)).toBe(-18.21)
    expect(applyScale('', 1)).toBeNull()
    expect(applyScale('abc', 1)).toBeNull()
  })
})

describe('findOpticalCandidates', () => {
  // Nama contoh bergaya ekstensi vendor (bukan klaim path firmware tertentu): yang diuji adalah PENGENALAN, bukan path.
  const ont = [
    p(`${W}X_CT-COM_GponInterfaceConfig.RXPower`, '-18210'),
    p(`${W}X_CT-COM_GponInterfaceConfig.TXPower`, '2740'),
    p(`${W}X_CT-COM_GponInterfaceConfig.Temperature`, '4440'),
    p(`${W}X_CT-COM_GponInterfaceConfig.Voltage`, '3170'),
    p(`${W}X_CT-COM_GponInterfaceConfig.BiasCurrent`, '8430'),
    p(`${W}X_CT-COM_GponInterfaceConfig.Status`, 'Up'),
    // bukan optik:
    p('InternetGatewayDevice.LANDevice.1.WLANConfiguration.1.TransmitPower', '100'),
    p('InternetGatewayDevice.DeviceInfo.X_ZTE-COM_Temperature', '55'),
    p('InternetGatewayDevice.WANDevice.1.WANConnectionDevice.1.WANPPPConnection.1.Username', 'u'),
  ]

  it('mengenali RX/TX/suhu/tegangan/bias pada objek optik dan mengabaikan yang bukan optik', () => {
    const c = findOpticalCandidates(ont)
    expect(c.map((x) => x.role)).toEqual(['rx', 'tx', 'temperature', 'voltage', 'bias'])
    expect(c[0]).toMatchObject({ path: `${W}X_CT-COM_GponInterfaceConfig.RXPower`, raw: '-18210', value: -18210, suggestedScale: 0.001, warning: null })
    expect(c[1].suggestedScale).toBe(0.001)
    expect(c.some((x) => /WLAN|DeviceInfo|PPP/.test(x.path))).toBe(false)
  })

  it('awalan vendor ZTE & TR-181 standar dikenali', () => {
    const c = findOpticalCandidates([
      p(`${W}X_ZTE-COM_WANPONInterfaceConfig.RxPower`, '-21.5'),
      p(`${W}X_ZTE-COM_WANPONInterfaceConfig.TxPower`, '2.1'),
      p('Device.Optical.Interface.1.OpticalSignalLevel', '-23500'),
      p('Device.Optical.Interface.1.TransmitOpticalLevel', '2100'),
    ])
    expect(c.map((x) => `${x.role}:${x.suggestedScale}`)).toEqual(['rx:0.001', 'rx:1', 'tx:0.001', 'tx:1']) // per peran, lalu urut path
  })

  it('suhu/tegangan tanpa objek optik di sekitarnya tidak ikut (mis. suhu CPU)', () => {
    expect(findOpticalCandidates([p('InternetGatewayDevice.DeviceInfo.Temperature', '55'), p('InternetGatewayDevice.X_FOO.Voltage', '12')])).toEqual([])
  })

  it('"TransmitPower" polos hanya bila jalurnya jelas optik', () => {
    expect(findOpticalCandidates([p('InternetGatewayDevice.X_FOO.TransmitPower', '20')])).toEqual([])
    expect(findOpticalCandidates([p(`${W}X_FOO_PONConfig.TransmitPower`, '2.1')]).map((x) => x.role)).toEqual(['tx'])
  })

  it('peringatan: nilai belum terbaca, satuan µW (positif besar), tak ada skala masuk akal', () => {
    const c = findOpticalCandidates([
      p(`${W}PONInterfaceConfig.RXPower`, ''),
      p(`${W}PONInterfaceConfig.TXPower`, '274'),
    ])
    expect(c[0].warning).toMatch(/belum terbaca/i)
    expect(c[1].warning).toMatch(/µW/)
  })

  it('tanpa parameter: kosong', () => {
    expect(findOpticalCandidates(undefined)).toEqual([])
    expect(findOpticalCandidates([])).toEqual([])
  })
})
