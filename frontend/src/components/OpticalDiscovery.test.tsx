import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { OpticalDiscovery } from './OpticalDiscovery'
import { setAuthToken } from '../lib/api'
import { ConfirmProvider } from '../lib/confirm'
import { mockFetch, renderApp } from '../test/utils'
import type { Device } from '../lib/types'

function login(roles: string[]) {
  setAuthToken('JWT')
  localStorage.setItem('acs_user', JSON.stringify({ id: 1, uuid: 'u', username: 'u', roles, tenant_id: roles.includes('SUPERADMIN') ? null : 7 }))
}

const device = {
  id: 5,
  serial_number: 'ZTEGF660',
  vendor_id: 1,
  device_model_id: 11,
  connection_request_url: null,
} as Device

const W = 'InternetGatewayDevice.WANDevice.1.'
const param = (name: string, value: string) => ({ id: 0, device_id: 5, parameter_name: name, parameter_value: value, writable: false, created_at: '', updated_at: '' })

function handler(r: { url: string; method: string }) {
  if (r.url.endsWith('/devices/5/parameters')) {
    return {
      json: [
        param(`${W}X_CT-COM_GponInterfaceConfig.RXPower`, '-18210'),
        param(`${W}X_CT-COM_GponInterfaceConfig.TXPower`, '2740'),
        param('InternetGatewayDevice.LANDevice.1.WLANConfiguration.1.TransmitPower', '100'),
      ],
    }
  }
  if (r.url.endsWith('/device-models/11')) return { json: { id: 11, vendor_id: 1, device_type_id: 1, data_model_version_id: 3, product_class: 'F660', model_name: 'ZXHN F660', description: null, is_active: true } }
  if (r.url.includes('/vendor-parameter-mappings') && r.method === 'GET') return { json: [] }
  return undefined
}

beforeEach(() => localStorage.clear())
afterEach(() => {
  setAuthToken(null)
  localStorage.clear()
})

const renderIt = () =>
  renderApp(
    <ConfirmProvider>
      <OpticalDiscovery device={device} />
    </ConfirmProvider>,
  )

describe('OpticalDiscovery', () => {
  it('SUPERADMIN: kandidat dikenali, skala disarankan, dan mapping disimpan per model dengan value_scale', async () => {
    login(['SUPERADMIN'])
    const calls = mockFetch((r) => {
      if (r.url.endsWith('/vendor-parameter-mappings') && r.method === 'POST') return { status: 201, json: { id: 1 } }
      return handler(r)
    })
    renderIt()
    const u = userEvent.setup()
    await screen.findByText(`${W}X_CT-COM_GponInterfaceConfig.RXPower`)
    // parameter WiFi tidak ikut sebagai kandidat
    expect(screen.queryByText(/WLANConfiguration/)).toBeNull()
    expect(screen.getByText('-18.21 dBm')).toBeTruthy()
    expect(screen.getByText('2.74 dBm')).toBeTruthy()

    // TX mentah 2740 ambigu (milli-dBm atau µW) → diberi peringatan dan TIDAK dipilih otomatis; operator harus mencentangnya sadar.
    expect(screen.getByText(/mungkin µW/i)).toBeTruthy()
    expect(screen.getByRole('button', { name: /simpan mapping \(1\)/i })).toBeTruthy()
    await u.click(screen.getByLabelText(`Pakai ${W}X_CT-COM_GponInterfaceConfig.TXPower`))
    await u.click(screen.getByRole('button', { name: /simpan mapping \(2\)/i }))

    await waitFor(() => expect(calls.filter((c) => c.method === 'POST' && c.url.endsWith('/vendor-parameter-mappings')).length).toBe(2))
    const bodies = calls.filter((c) => c.method === 'POST' && c.url.endsWith('/vendor-parameter-mappings')).map((c) => c.body as Record<string, unknown>)
    expect(bodies[0]).toMatchObject({
      vendor_id: 1,
      device_model_id: 11,
      data_model_version_id: 3,
      logical_key: 'optical.rx_power',
      tr069_path: `${W}X_CT-COM_GponInterfaceConfig.RXPower`,
      value_scale: 0.001,
    })
    expect(bodies[1]).toMatchObject({ logical_key: 'optical.tx_power', value_scale: 0.001 })
    // setelah menyimpan, pembacaan optik diantre ulang
    await waitFor(() => expect(calls.some((c) => c.url.endsWith('/devices/refresh-summary') && c.method === 'POST')).toBe(true))
  })

  it('ADMIN: boleh menelusuri tetapi tidak bisa menyimpan mapping (data katalog global)', async () => {
    login(['ADMIN'])
    mockFetch(handler)
    renderIt()
    await screen.findByText(`${W}X_CT-COM_GponInterfaceConfig.RXPower`)
    expect(screen.queryByRole('button', { name: /simpan mapping/i })).toBeNull()
    expect(screen.getByText(/hanya untuk SUPERADMIN/i)).toBeTruthy()
    expect(screen.getByRole('button', { name: /telusuri path optik/i })).toBeTruthy()
  })

  it('"Telusuri path optik" mengantre refresh cabang WANDevice kedalaman 3', async () => {
    login(['ADMIN'])
    const calls = mockFetch((r) => {
      if (r.url.endsWith('/devices/5/refresh') && r.method === 'POST') return { status: 202, json: {} }
      if (r.url.endsWith('/devices/5/parameters')) return { json: [] }
      return handler(r)
    })
    renderIt()
    const u = userEvent.setup()
    await u.click(await screen.findByRole('button', { name: /telusuri path optik/i }))
    await waitFor(() => expect(calls.some((c) => c.url.endsWith('/devices/5/refresh'))).toBe(true))
    expect(calls.find((c) => c.url.endsWith('/devices/5/refresh'))?.body).toEqual({ path: 'InternetGatewayDevice.WANDevice.', depth: 3 })
  })

  it('peran ganda (dua kandidat RX) ditolak tanpa memanggil API tulis', async () => {
    login(['SUPERADMIN'])
    const calls = mockFetch((r) => {
      if (r.url.endsWith('/devices/5/parameters')) {
        return { json: [param(`${W}X_A_PONConfig.RXPower`, '-20.1'), param(`${W}X_B_PONConfig.RXPower`, '-20.2')] }
      }
      return handler(r)
    })
    renderIt()
    const u = userEvent.setup()
    await screen.findByText(`${W}X_A_PONConfig.RXPower`)
    await u.click(screen.getByRole('button', { name: /simpan mapping \(2\)/i }))
    await screen.findByText(/satu kandidat per peran/i)
    expect(calls.some((c) => c.method === 'POST')).toBe(false)
  })
})
