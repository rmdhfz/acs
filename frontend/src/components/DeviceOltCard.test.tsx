import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { screen } from '@testing-library/react'
import { DeviceOltCard } from './DeviceOltCard'
import { setAuthToken } from '../lib/api'
import { mockFetch, renderApp } from '../test/utils'
import type { DeviceOLT, RegisteredONU } from '../lib/types'

const minutesAgo = (m: number) => new Date(Date.now() - m * 60_000).toISOString()

const ONU = (over: Partial<RegisteredONU> = {}): RegisteredONU => ({
  id: 'o1', olt_id: 'olt-1', olt_name: 'OLT Pusat', tenant_id: 7, pon_port: 'sim/1/2', onu_index: '7', serial_number: 'SIMREG0002', onu_type: null, state: 'LOS',
  vendor_state: null, rx_power_dbm: -26.04, first_seen_at: minutesAgo(900), last_seen_at: minutesAgo(2), last_state_change_at: minutesAgo(180), device_id: 42,
  device_status: 'OFFLINE', olt_last_poll_at: minutesAgo(2), ...over,
})

function serve(body: DeviceOLT | { status: number }, ontRx?: number | null) {
  mockFetch((r) => {
    if (r.url.includes('/devices/42/optical-metrics')) {
      return ontRx === undefined ? { json: { data: [], total: 0 } } : { json: { data: [{ id: 1, device_id: 42, rx_power_dbm: ontRx, tx_power_dbm: null, voltage: null, bias_current_ma: null, temperature_celsius: null, distance_meters: null, recorded_at: minutesAgo(5) }], total: 1 } }
    }
    if (!r.url.includes('/devices/42/olt')) return undefined
    return 'status' in body ? { status: body.status, json: { message: 'x' } } : { json: body }
  })
}

beforeEach(() => {
  setAuthToken('JWT')
  localStorage.setItem('acs_user', JSON.stringify({ id: 1, uuid: 'u', username: 'u', roles: ['VIEWER'], tenant_id: 7 }))
})
afterEach(() => {
  setAuthToken(null)
  localStorage.clear()
})

describe('DeviceOltCard', () => {
  it('terhubung ke ONU: OLT, port, nomor ONU, keadaan, RX sisi OLT, dan usia data', async () => {
    serve({ linked: true, onu: ONU(), tenant_has_olt: true })
    renderApp(<DeviceOltCard deviceId={42} />)
    expect(await screen.findByText('Posisi di jaringan')).toBeTruthy()
    expect(screen.getByText('LOS (sinyal hilang) di OLT')).toBeTruthy()
    expect(screen.getByText('OLT Pusat')).toBeTruthy()
    expect(screen.getByText('sim/1/2')).toBeTruthy()
    expect(screen.getByText('7')).toBeTruthy()
    expect(screen.getByText('-26.0 dBm')).toBeTruthy()
    expect(screen.getByText(/Data OLT/)).toBeTruthy()
    expect(screen.queryByText(/sudah lebih dari 30 menit/)).toBeNull()
  })

  it('data OLT lama diberi catatan', async () => {
    serve({ linked: true, onu: ONU({ last_seen_at: minutesAgo(120) }), tenant_has_olt: true })
    renderApp(<DeviceOltCard deviceId={42} />)
    expect(await screen.findByText(/sudah lebih dari 30 menit/)).toBeTruthy()
  })

  it('tenant punya OLT tetapi serial tak ditemukan: catatan, bukan kartu data', async () => {
    serve({ linked: false, onu: null, tenant_has_olt: true })
    renderApp(<DeviceOltCard deviceId={42} />)
    expect(await screen.findByText(/tidak ditemukan di data OLT terakhir/)).toBeTruthy()
    expect(screen.queryByText('Port PON')).toBeNull()
  })

  it('tenant tanpa OLT: kartu tersembunyi sepenuhnya', async () => {
    serve({ linked: false, onu: null, tenant_has_olt: false })
    const { container } = renderApp(<DeviceOltCard deviceId={42} />)
    await new Promise((r) => setTimeout(r, 50))
    expect(container.querySelector('section')).toBeNull()
  })

  it('galat API (mis. inventaris tidak aktif, atau perangkat tenant lain): kartu tersembunyi, tidak memunculkan galat di kartu', async () => {
    serve({ status: 400 })
    const { container } = renderApp(<DeviceOltCard deviceId={42} />)
    await new Promise((r) => setTimeout(r, 50))
    expect(container.querySelector('section')).toBeNull()
  })

  it('RX dari dua arah diberi label arah dan tingkat masing-masing, dengan catatan bahwa keduanya bukan besaran yang sama', async () => {
    serve({ linked: true, onu: ONU({ state: 'ONLINE', rx_power_dbm: -28.2 }), tenant_has_olt: true }, -19.5)
    renderApp(<DeviceOltCard deviceId={42} />)
    expect(await screen.findByText('RX sisi OLT (upstream)')).toBeTruthy()
    expect(screen.getByText('RX sisi ONT (downstream)')).toBeTruthy()
    expect(await screen.findByText('-19.5 dBm')).toBeTruthy()
    expect(screen.getByText('-28.2 dBm').closest('dd')!.textContent).toContain('(kritis)')
    expect(screen.getByText('-19.5 dBm').closest('dd')!.textContent).toContain('(normal)')
    expect(screen.getByText(/dibaca 5 menit lalu|dibaca/)).toBeTruthy()
    expect(screen.getByText(/bukan besaran yang sama/)).toBeTruthy()
  })

  it('tanpa pembacaan ONT: tanda strip dan keterangan jujur, tanpa tingkat', async () => {
    serve({ linked: true, onu: ONU({ state: 'ONLINE', rx_power_dbm: -21.0 }), tenant_has_olt: true })
    renderApp(<DeviceOltCard deviceId={42} />)
    expect(await screen.findByText('belum ada pembacaan')).toBeTruthy()
    const ont = screen.getByText('RX sisi ONT (downstream)').parentElement!
    expect(ont.textContent).not.toMatch(/\((normal|lemah|kritis)\)/)
  })

  it('RX sisi OLT tidak diberi tingkat bila ONU tidak online (tak bermakna)', async () => {
    serve({ linked: true, onu: ONU({ state: 'LOS', rx_power_dbm: -31.0 }), tenant_has_olt: true }, -20.0)
    renderApp(<DeviceOltCard deviceId={42} />)
    await screen.findByText('-31.0 dBm')
    expect(screen.getByText('-31.0 dBm').closest('dd')!.textContent).not.toMatch(/kritis|lemah|normal/)
  })

  it('"sejak": baris yang baru ditemukan diberi label jujur; perubahan yang teramati memakai waktunya', async () => {
    const seen = minutesAgo(3)
    serve({ linked: true, onu: ONU({ first_seen_at: seen, last_state_change_at: seen }), tenant_has_olt: true })
    const { unmount } = renderApp(<DeviceOltCard deviceId={42} />)
    expect(await screen.findByText(/waktu mulainya sebelum itu tidak diketahui/)).toBeTruthy()
    expect(screen.queryByText(/Keadaan terpantau sejak/)).toBeNull()
    unmount()
    serve({ linked: true, onu: ONU(), tenant_has_olt: true })
    renderApp(<DeviceOltCard deviceId={42} />)
    expect(await screen.findByText(/Keadaan terpantau sejak/)).toBeTruthy()
  })
})
