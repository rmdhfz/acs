import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { DevicesPage } from './DevicesPage'
import { ConfirmProvider } from '../lib/confirm'
import { setAuthToken } from '../lib/api'
import { mockFetch, renderApp } from '../test/utils'

function login(roles: string[]) {
  setAuthToken('JWT')
  localStorage.setItem('acs_user', JSON.stringify({ id: 1, uuid: 'u', username: 'adm', roles, tenant_id: 7 }))
}

const dev = (over: Record<string, unknown>) => ({
  id: 1, device_uuid: 'u', tenant_id: 7, vendor_id: 1, device_model_id: null, device_status_id: 1, provisioning_profile_id: null,
  oui: null, serial_number: 'X', product_class: 'F660', mac_address: null, software_version: 'V5.2', hardware_version: null,
  ip_address: '10.0.0.1', connection_request_url: null, inform_username: null, last_inform_at: new Date().toISOString(),
  last_boot_event_at: null, latitude: null, longitude: null, notes: null, created_at: '', updated_at: '',
  rx_power_dbm: null, tx_power_dbm: null, optical_at: null, pppoe_username: null, ...over,
})

const LIST = [
  dev({ id: 1, serial_number: 'ELWGC09C2814', rx_power_dbm: -21.14, tx_power_dbm: 2.1, optical_at: new Date().toISOString(), pppoe_username: '001003000332@dukodu.id' }),
  dev({ id: 2, serial_number: 'ZTEGCD775EE7', rx_power_dbm: -25.53, product_class: 'F609' }),
  dev({ id: 3, serial_number: 'ZTXNQAJ2B1DD38E', rx_power_dbm: -27.45, product_class: 'F670L' }),
  dev({ id: 4, serial_number: 'BELUM-ADA-DATA' }),
]

function handler(extra?: (r: { url: string; method: string }) => { status?: number; json?: unknown } | undefined) {
  return (r: { url: string; method: string; body: unknown }) => {
    if (r.url.includes('/refs/ref_device_status')) return { json: [{ id: 1, code: 'ONLINE', name: 'Online' }, { id: 2, code: 'OFFLINE', name: 'Offline' }] }
    if (r.url.includes('/refs/')) return { json: [] }
    if (r.url.includes('/vendors')) return { json: { data: [{ id: 1, name: 'ZTE Corporation', code: 'ZTE' }], total: 1 } }
    if (r.url.includes('/tags')) return { json: { data: [], total: 0 } }
    if (r.url.includes('/tenants/current')) return { json: { id: 7, name: 'Tenant Uji' } }
    if (r.url.includes('/devices/refresh-summary')) return extra?.(r)
    if (r.url.includes('/devices?') || r.url.endsWith('/devices')) return { json: { data: LIST, total: LIST.length } }
    if (r.url.includes('/tasks')) return { json: { data: [], total: 0 } }
    return extra?.(r)
  }
}

const ui = (
  <ConfirmProvider>
    <DevicesPage />
  </ConfirmProvider>
)

beforeEach(() => localStorage.clear())
afterEach(() => {
  setAuthToken(null)
  localStorage.clear()
})

describe('DevicesPage — kolom ringkasan', () => {
  it('menampilkan model, redaman berwarna menurut level, dan username PPPoE; kosong = "-"', async () => {
    login(['ADMIN'])
    mockFetch(handler())
    renderApp(ui)
    expect(await screen.findAllByText('ELWGC09C2814')).toBeTruthy()
    for (const h of ['Model', 'Redaman', 'Username PPPoE']) expect(screen.getByRole('columnheader', { name: new RegExp(h, 'i') })).toBeTruthy()

    // Halaman merender tabel (desktop) dan kartu (mobile): ambil baris tabelnya saja.
    const rowOf = (serial: string) => screen.getAllByText(serial).find((el) => el.tagName === 'TD')?.closest('tr') as HTMLElement
    const ok = within(rowOf('ELWGC09C2814')).getByText('-21.14 dBm')
    const warn = within(rowOf('ZTEGCD775EE7')).getByText('-25.53 dBm')
    const bad = within(rowOf('ZTXNQAJ2B1DD38E')).getByText('-27.45 dBm')
    expect(ok.className).not.toMatch(/amber|red/)
    expect(warn.className).toMatch(/amber/)
    expect(bad.className).toMatch(/red/)
    expect(ok.getAttribute('title')).toMatch(/Normal.*TX 2\.10 dBm/)
    expect(within(rowOf('ELWGC09C2814')).getByText('001003000332@dukodu.id')).toBeTruthy()
    expect(within(rowOf('ELWGC09C2814')).getByText('F660')).toBeTruthy()
    // tanpa data: tidak ada angka dBm dan tidak ada username
    expect(within(rowOf('BELUM-ADA-DATA')).queryByText(/dBm/)).toBeNull()
  })

  it('Segarkan data: tanpa pilihan = seluruh halaman; dengan pilihan = hanya yang dipilih', async () => {
    login(['ADMIN'])
    const calls = mockFetch(handler((r) => (r.url.endsWith('/devices/refresh-summary') ? { status: 202, json: { queued: 4, skipped: [] } } : undefined)))
    renderApp(ui)
    const u = userEvent.setup()
    await screen.findAllByText('ELWGC09C2814')

    await u.click(screen.getByRole('button', { name: /^segarkan data$/i }))
    await waitFor(() => expect(calls.some((c) => c.url.endsWith('/devices/refresh-summary'))).toBe(true))
    expect(calls.find((c) => c.url.endsWith('/devices/refresh-summary'))?.body).toEqual({ device_ids: [1, 2, 3, 4] })

    await u.click(screen.getAllByRole('checkbox', { name: /pilih device ZTEGCD775EE7/i })[0]) // device id 2
    await u.click(await screen.findByRole('button', { name: /segarkan data \(1\)/i }))
    await waitFor(() => expect(calls.filter((c) => c.url.endsWith('/devices/refresh-summary'))).toHaveLength(2))
    expect(calls.filter((c) => c.url.endsWith('/devices/refresh-summary'))[1].body).toEqual({ device_ids: [2] })
  })

  it('filter redaman & vendor "Belum dikenal" diteruskan ke API dan kembali ke halaman 1', async () => {
    login(['ADMIN'])
    const calls = mockFetch(handler())
    renderApp(ui)
    const u = userEvent.setup()
    await screen.findAllByText('ELWGC09C2814')

    await u.selectOptions(screen.getByLabelText('Filter redaman'), 'bad')
    await waitFor(() => expect(calls.some((c) => c.url.includes('/devices?') && c.url.includes('optical_level=bad'))).toBe(true))

    await u.selectOptions(screen.getByLabelText('Filter vendor'), 'none')
    await waitFor(() => expect(calls.some((c) => c.url.includes('no_vendor=true') && c.url.includes('optical_level=bad'))).toBe(true))
    // "none" bukan id vendor: tidak boleh bocor sebagai vendor_id
    expect(calls.some((c) => c.url.includes('vendor_id=none') || c.url.includes('vendor_id=NaN'))).toBe(false)

    await u.selectOptions(screen.getByLabelText('Filter redaman'), '')
    await u.selectOptions(screen.getByLabelText('Filter vendor'), '')
    await waitFor(() => expect(calls[calls.length - 1].url).not.toMatch(/optical_level|no_vendor/))
  })

  it('urutan dan filter parameter: sort dikirim, kondisi per baris menjadi param berulang (maks 5)', async () => {
    login(['ADMIN'])
    const calls = mockFetch(handler())
    renderApp(ui)
    const u = userEvent.setup()
    await screen.findAllByText('ELWGC09C2814')

    await u.selectOptions(screen.getByLabelText('Urutan'), '-last_inform_at')
    await waitFor(() => expect(calls.some((c) => c.url.includes('/devices?') && c.url.includes('sort=-last_inform_at'))).toBe(true))

    await u.click(screen.getByRole('button', { name: /Filter parameter/ }))
    await u.type(screen.getByLabelText(/Kondisi nilai parameter/), 'A.B~prefix~V5{Enter}C.D~exists{Enter}{Enter}E.F~lt~-25')
    await waitFor(() => {
      const last = calls.filter((c) => c.url.includes('/devices?')).at(-1)!
      const q = new URL(last.url, 'http://x').searchParams
      expect(q.getAll('param')).toEqual(['A.B~prefix~V5', 'C.D~exists', 'E.F~lt~-25']) // baris kosong diabaikan
      expect(q.get('sort')).toBe('-last_inform_at')
    })
    expect(screen.getByRole('button', { name: /Filter parameter \(3\)/ })).toBeTruthy()
    // Mengetik ~50 karakter lewat user-event: pada mesin yang sedang sibuk (Docker, build) melewati batas bawaan 5 detik dan tes
    // menjadi flaky (terjadi 2 dari 3 jalan penuh). Batas waktu dinaikkan; isi tes tidak berubah.
  }, 20_000)

  it('VIEWER tidak melihat tombol Segarkan data', async () => {
    login(['VIEWER'])
    mockFetch(handler())
    renderApp(ui)
    expect(await screen.findAllByText('ELWGC09C2814')).toBeTruthy()
    expect(screen.queryByRole('button', { name: /segarkan data/i })).toBeNull()
  })
})
