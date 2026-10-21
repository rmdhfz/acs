import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { DeviceWifiTab } from './DeviceWifiTab'
import { ConfirmProvider } from '../lib/confirm'
import { setAuthToken } from '../lib/api'
import { mockFetch, renderApp } from '../test/utils'
import type { Device } from '../lib/types'

function login(roles: string[]) {
  setAuthToken('JWT')
  localStorage.setItem('acs_user', JSON.stringify({ id: 1, uuid: 'u', username: 'adm', roles, tenant_id: 7 }))
}

const device = { id: 5, serial_number: 'ZTEGC85E44E1', connection_request_url: 'http://10.0.123.22:58000/x' } as Device

const W = 'InternetGatewayDevice.LANDevice.1.WLANConfiguration'
const params = (pass: string) =>
  [1, 2, 5].flatMap((n) => [
    { name: `${W}.${n}.SSID`, value: `SSID-${n}` },
    { name: `${W}.${n}.Enable`, value: n === 2 ? '0' : '1' },
    { name: `${W}.${n}.Channel`, value: n === 5 ? '36' : '6' },
    { name: `${W}.${n}.PreSharedKey.1.KeyPassphrase`, value: n === 1 ? pass : '' },
    { name: `${W}.${n}.SSIDAdvertisementEnabled`, value: 'true' },
    { name: `${W}.${n}.X_ZTE-COM_MaxUserNum`, value: '32' },
    ...(n === 5
      ? [
          { name: `${W}.5.X_ZTE-COM_SSIDIsolation`, value: '0' },
          { name: `${W}.5.X_ZTE-COM_Priority`, value: '0' },
        ]
      : []),
    ...(n === 1
      ? [
          { name: `${W}.1.AssociatedDevice.1.AssociatedDeviceMACAddress`, value: 'aa:bb:cc:00:00:01' },
          { name: `${W}.1.AssociatedDevice.1.AssociatedDeviceIPAddress`, value: '192.168.1.2' },
          { name: `${W}.1.AssociatedDevice.1.X_ZTE-COM_DeviceName`, value: 'iPhone Budi' },
          { name: `${W}.1.AssociatedDevice.1.X_ZTE-COM_RSSI`, value: '-57' },
        ]
      : []),
  ]).map((p, i) => ({ id: i, device_id: 5, parameter_name: p.name, parameter_value: p.value, writable: true, created_at: '', updated_at: '2026-10-21T03:00:00Z' }))

type Extra = (r: { url: string; method: string }) => { status?: number; json?: unknown } | undefined
function handler(pass: string, extra?: Extra) {
  return (r: { url: string; method: string; body: unknown }) => {
    if (r.url.includes('/devices/5/parameters')) return { json: params(pass) }
    if (r.url.includes('/tasks?')) return { json: { data: [], total: 0 } }
    if (r.url.endsWith('/refs/ref_task_status')) return { json: [{ id: 1, code: 'PENDING', name: 'Pending' }] }
    if (r.url.endsWith('/refs/ref_task_types')) return { json: [{ id: 1, code: 'SET_PARAMETER_VALUES', name: 'Set' }] }
    return extra?.(r)
  }
}

const ui = (
  <ConfirmProvider>
    <DeviceWifiTab device={device} />
  </ConfirmProvider>
)

beforeEach(() => localStorage.clear())
afterEach(() => {
  setAuthToken(null)
  localStorage.clear()
})

describe('DeviceWifiTab', () => {
  it('menampilkan semua SSID dalam tabel (band, status) dan password tersembunyi sampai diklik', async () => {
    login(['ADMIN'])
    mockFetch(handler('rahasia-123'))
    renderApp(ui)
    expect(await screen.findByText('SSID-1')).toBeTruthy()
    expect(screen.getByText('SSID-2')).toBeTruthy()
    expect(screen.getByText('SSID-5')).toBeTruthy()
    expect(screen.getByText('3 SSID')).toBeTruthy()
    expect(screen.getByText('5 GHz')).toBeTruthy() // dari channel 36
    expect(screen.getByText('Nonaktif')).toBeTruthy() // SSID-2 Enable=0
    expect(screen.queryByText('rahasia-123')).toBeNull()
    await userEvent.setup().click(screen.getByRole('button', { name: /tampilkan password ssid 1/i }))
    expect(screen.getByText('rahasia-123')).toBeTruthy()
  })

  it('NOC: password [redacted] disembunyikan dan tombol Ubah tidak tersedia', async () => {
    login(['NOC'])
    mockFetch(handler('[redacted]'))
    renderApp(ui)
    expect(await screen.findByText('SSID-1')).toBeTruthy()
    expect(screen.getByText('disembunyikan')).toBeTruthy()
    expect(screen.queryByRole('button', { name: /^ubah wifi \d/i })).toBeNull()
  })

  it('baca semua SSID: antre GetParameterValues untuk seluruh cabang WLAN', async () => {
    login(['ADMIN'])
    const calls = mockFetch(handler('x', (r) => (r.url.endsWith('/devices/5/tasks/get-parameter-values') ? { status: 202, json: { message: 'ok' } } : undefined)))
    renderApp(ui)
    await userEvent.setup().click(await screen.findByRole('button', { name: /baca semua ssid/i }))
    await waitFor(() => expect(calls.some((c) => c.url.endsWith('/tasks/get-parameter-values'))).toBe(true))
    expect(calls.find((c) => c.url.endsWith('/tasks/get-parameter-values'))?.body).toEqual({ names: [`${W}.`] })
  })

  it('nonaktifkan SSID aktif: konfirmasi lalu Set Enable=0 ke path SSID tsb + baca ulang; SSID nonaktif ditawari Aktifkan (Enable=1)', async () => {
    login(['ADMIN'])
    const calls = mockFetch(
      handler('x', (r) => (r.url.endsWith('/tasks/set-parameter-values') || r.url.endsWith('/tasks/get-parameter-values') ? { status: 202, json: { message: 'ok' } } : undefined)),
    )
    renderApp(ui)
    const u = userEvent.setup()
    await screen.findByText('SSID-5')

    await u.click(screen.getByRole('button', { name: /^nonaktifkan wifi 5$/i }))
    await u.click(await screen.findByRole('button', { name: /^nonaktifkan$/i }))
    await waitFor(() => expect(calls.some((c) => c.url.endsWith('/tasks/set-parameter-values'))).toBe(true))
    expect(calls.find((c) => c.url.endsWith('/tasks/set-parameter-values'))?.body).toEqual({ values: { [`${W}.5.Enable`]: '0' } })
    await waitFor(() => expect(calls.some((c) => c.url.endsWith('/tasks/get-parameter-values'))).toBe(true))
    expect(calls.find((c) => c.url.endsWith('/tasks/get-parameter-values'))?.body).toEqual({ names: [`${W}.`] })

    await u.click(screen.getByRole('button', { name: /^aktifkan wifi 2$/i }))
    await u.click(await screen.findByRole('button', { name: /^aktifkan$/i }))
    await waitFor(() => expect(calls.filter((c) => c.url.endsWith('/tasks/set-parameter-values'))).toHaveLength(2))
    expect(calls.filter((c) => c.url.endsWith('/tasks/set-parameter-values'))[1].body).toEqual({ values: { [`${W}.2.Enable`]: '1' } })
  })

  it('klien tersambung bisa dibuka per SSID (nama, IP, MAC, sinyal)', async () => {
    login(['ADMIN'])
    mockFetch(handler('x'))
    renderApp(ui)
    const u = userEvent.setup()
    expect(await screen.findByText('SSID-1')).toBeTruthy()
    expect(screen.queryByText('iPhone Budi')).toBeNull()
    await u.click(screen.getByRole('button', { name: /klien ssid 1/i }))
    expect(screen.getByText('iPhone Budi')).toBeTruthy()
    expect(screen.getByText('192.168.1.2')).toBeTruthy()
    expect(screen.getByText('aa:bb:cc:00:00:01')).toBeTruthy()
    expect(screen.getByText('-57 dBm')).toBeTruthy()
  })

  it('pengaturan lanjutan: kirim HANYA yang berubah ke path asli (hide, maks. klien, isolasi, prioritas)', async () => {
    login(['ADMIN'])
    const calls = mockFetch(handler('x', (r) => (r.url.endsWith('/tasks/set-parameter-values') || r.url.endsWith('/tasks/get-parameter-values') ? { status: 202, json: { message: 'ok' } } : undefined)))
    renderApp(ui)
    const u = userEvent.setup()
    await screen.findByText('SSID-5')
    await u.click(screen.getByRole('button', { name: /ubah wifi 5/i }))
    const dialog = await screen.findByRole('dialog')

    // tanpa perubahan → ditolak tanpa memanggil API
    await u.click(within(dialog).getByRole('button', { name: /terapkan ke perangkat/i }))
    expect((await within(dialog).findByRole('alert')).textContent).toMatch(/tidak ada perubahan/i)
    expect(calls.some((c) => c.method === 'POST')).toBe(false)

    // nilai di luar rentang: validasi bawaan browser (min/max) memblokir pengiriman form
    const max = within(dialog).getByLabelText(/maksimum klien/i) as HTMLInputElement
    await u.clear(max)
    await u.type(max, '999')
    expect(max.validity.valid).toBe(false)
    await u.click(within(dialog).getByRole('button', { name: /terapkan ke perangkat/i }))
    expect(calls.some((c) => c.method === 'POST')).toBe(false)

    await u.clear(max)
    await u.type(max, '16')
    await u.click(within(dialog).getByRole('checkbox', { name: /sembunyikan ssid/i }))
    await u.click(within(dialog).getByRole('checkbox', { name: /isolasi ssid/i }))
    await u.clear(within(dialog).getByLabelText(/prioritas/i))
    await u.type(within(dialog).getByLabelText(/prioritas/i), '3')
    await u.click(within(dialog).getByRole('button', { name: /terapkan ke perangkat/i }))
    await u.click(await screen.findByRole('button', { name: /^ubah wifi$/i }))
    await waitFor(() => expect(calls.some((c) => c.url.endsWith('/tasks/set-parameter-values'))).toBe(true))
    expect(calls.find((c) => c.url.endsWith('/tasks/set-parameter-values'))?.body).toEqual({
      values: {
        [`${W}.5.SSIDAdvertisementEnabled`]: '0', // disembunyikan = siaran dimatikan
        [`${W}.5.X_ZTE-COM_SSIDIsolation`]: '1',
        [`${W}.5.X_ZTE-COM_MaxUserNum`]: '16',
        [`${W}.5.X_ZTE-COM_Priority`]: '3',
      },
    })
  })

  it('SSID yang tidak melaporkan isolasi/prioritas: dialog menjelaskan, bukan menebak path', async () => {
    login(['ADMIN'])
    mockFetch(handler('x'))
    renderApp(ui)
    const u = userEvent.setup()
    await screen.findByText('SSID-1')
    await u.click(screen.getByRole('button', { name: /ubah wifi 1/i }))
    const dialog = await screen.findByRole('dialog')
    expect(within(dialog).queryByRole('checkbox', { name: /isolasi ssid/i })).toBeNull()
    expect(within(dialog).getByText(/isolasi ssid: tidak dilaporkan ont ini/i)).toBeTruthy()
    expect(within(dialog).getByText(/prioritas: tidak dilaporkan ont ini/i)).toBeTruthy()
    expect(within(dialog).getByLabelText(/maksimum klien/i)).toBeTruthy() // ini dilaporkan → tersedia
  })

  it('cari pengaturan lanjutan: antre penelusuran cabang LAN/WLAN', async () => {
    login(['ADMIN'])
    const calls = mockFetch(handler('x', (r) => (r.url.endsWith('/devices/5/refresh') ? { status: 202, json: { message: 'ok' } } : undefined)))
    renderApp(ui)
    await userEvent.setup().click(await screen.findByRole('button', { name: /cari pengaturan lanjutan/i }))
    await waitFor(() => expect(calls.some((c) => c.url.endsWith('/devices/5/refresh'))).toBe(true))
    expect(calls.find((c) => c.url.endsWith('/devices/5/refresh'))?.body).toEqual({ path: 'InternetGatewayDevice.LANDevice.1.', depth: 3 })
  })

  it('ubah per baris: validasi, konfirmasi, lalu kirim ke path asli SSID tsb (bukan SSID lain)', async () => {
    login(['ADMIN'])
    const calls = mockFetch(handler('x', (r) => (r.url.endsWith('/devices/5/tasks/set-parameter-values') ? { status: 202, json: { message: 'ok' } } : undefined)))
    renderApp(ui)
    const u = userEvent.setup()
    await screen.findByText('SSID-5')
    await u.click(screen.getByRole('button', { name: /ubah wifi 5/i }))
    const dialog = await screen.findByRole('dialog')

    // validasi: kosong & password pendek ditolak tanpa memanggil API
    await u.click(within(dialog).getByRole('button', { name: /terapkan ke perangkat/i }))
    expect(await within(dialog).findByRole('alert')).toBeTruthy()
    await u.type(within(dialog).getByLabelText(/^password baru \(8/i), 'pendek')
    await u.click(within(dialog).getByRole('button', { name: /terapkan ke perangkat/i }))
    expect((await within(dialog).findByRole('alert')).textContent).toMatch(/8–63 karakter/)
    expect(calls.some((c) => c.method === 'POST')).toBe(false)

    await u.clear(within(dialog).getByLabelText(/^password baru \(8/i))
    await u.type(within(dialog).getByLabelText(/ssid baru/i), 'ACS-UAT-5G')
    await u.type(within(dialog).getByLabelText(/^password baru \(8/i), 'passwordbaru1')
    await u.click(within(dialog).getByRole('button', { name: /terapkan ke perangkat/i }))
    await u.click(await screen.findByRole('button', { name: /^ubah wifi$/i }))
    await waitFor(() => expect(calls.some((c) => c.url.endsWith('/tasks/set-parameter-values'))).toBe(true))
    expect(calls.find((c) => c.url.endsWith('/tasks/set-parameter-values'))?.body).toEqual({
      values: { [`${W}.5.SSID`]: 'ACS-UAT-5G', [`${W}.5.PreSharedKey.1.KeyPassphrase`]: 'passwordbaru1' },
    })
  })
})
