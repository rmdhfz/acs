import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import VirtualParametersPage from './VirtualParametersPage'
import { ConfirmProvider } from '../lib/confirm'
import { setAuthToken } from '../lib/api'
import { mockFetch, renderApp } from '../test/utils'

function login(roles: string[]) {
  setAuthToken('JWT')
  localStorage.setItem('acs_user', JSON.stringify({ id: 1, uuid: 'u', username: 'adm', roles, tenant_id: 7 }))
}

const vp = (over: Record<string, unknown>) => ({
  id: 'vp-1', tenant_id: 7, name: 'rx_dbm', label: 'Redaman RX', description: null,
  expression: 'round({InternetGatewayDevice.X_ZTE.OpticalRx} / 1000, 1)', unit: 'dBm', is_active: true, created_at: '', updated_at: '', ...over,
})
const OWN = vp({})
const GLOBAL = vp({ id: 'vp-g', tenant_id: null, name: 'uptime_hari', label: 'Uptime (hari)', unit: 'hari', expression: '{A} / 86400' })

const ui = (
  <ConfirmProvider>
    <VirtualParametersPage />
  </ConfirmProvider>
)

beforeEach(() => localStorage.clear())
afterEach(() => {
  setAuthToken(null)
  localStorage.clear()
})

function list(rows: unknown[]) {
  return (r: { url: string; method: string }) => (r.url.includes('/virtual-parameters') && r.method === 'GET' ? { json: { data: rows, total: rows.length } } : undefined)
}

describe('VirtualParametersPage', () => {
  it('ADMIN: mengedit definisi tenant, tetapi tidak definisi GLOBAL (hanya SUPERADMIN)', async () => {
    login(['ADMIN'])
    mockFetch(list([OWN, GLOBAL]))
    renderApp(ui)
    expect(await screen.findByText('Redaman RX')).toBeTruthy()
    expect(screen.getByText('Global')).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Edit rx_dbm' })).toBeTruthy()
    expect(screen.getByRole('checkbox', { name: 'Aktif: rx_dbm' })).toBeTruthy()
    expect(screen.queryByRole('button', { name: 'Edit uptime_hari' })).toBeNull()
    expect(screen.queryByRole('checkbox', { name: 'Aktif: uptime_hari' })).toBeNull()
  })

  it('VIEWER hanya membaca: tanpa tombol tambah/edit/hapus', async () => {
    login(['VIEWER'])
    mockFetch(list([OWN]))
    renderApp(ui)
    await screen.findByText('Redaman RX')
    expect(screen.queryByRole('button', { name: /Virtual Parameter Baru/ })).toBeNull()
    expect(screen.queryByRole('button', { name: /Edit rx_dbm|Hapus rx_dbm/ })).toBeNull()
  })

  it('SUPERADMIN boleh mengubah definisi global', async () => {
    login(['SUPERADMIN'])
    mockFetch(list([GLOBAL]))
    renderApp(ui)
    expect(await screen.findByRole('button', { name: 'Edit uptime_hari' })).toBeTruthy()
  })

  it('membuat definisi: POST tanpa tenant_id (server menentukan scope dari akun)', async () => {
    login(['ADMIN'])
    const calls = mockFetch((r) => {
      if (r.method === 'POST') return { status: 201, json: OWN }
      return list([])(r)
    })
    renderApp(ui)
    const u = userEvent.setup()
    await u.click(await screen.findByRole('button', { name: /Virtual Parameter Baru/ }))
    await u.type(screen.getByLabelText('Nama (kunci)'), 'rx_dbm')
    await u.type(screen.getByLabelText('Label'), 'Redaman RX')
    await u.click(screen.getByRole('button', { name: 'Redaman dBm dari satuan 0,001' })) // contoh mengisi ekspresi
    await u.click(screen.getByRole('button', { name: 'Simpan' }))
    await waitFor(() => expect(calls.some((c) => c.method === 'POST' && c.url.endsWith('/virtual-parameters'))).toBe(true))
    const post = calls.find((c) => c.method === 'POST')!
    const body = post.body as Record<string, unknown>
    expect(body.name).toBe('rx_dbm')
    expect(String(body.expression)).toContain('OpticalRx')
    expect('tenant_id' in body).toBe(false)
  })

  it('uji ekspresi: menampilkan nilai + rujukan, dan kesalahan sintaks sebagai pesan', async () => {
    login(['ADMIN'])
    let ok = true
    mockFetch((r) => {
      if (r.url.endsWith('/virtual-parameters/test')) {
        return ok
          ? { json: { ok: true, value: '-21.5', refs: [{ name: 'InternetGatewayDevice.X_ZTE.OpticalRx', value: '-21500' }, { name: 'Hilang.Param', value: null }] } }
          : { json: { ok: false, value: null, refs: [], error: { pos: 4, message: 'token tak terduga' } } }
      }
      return list([OWN])(r)
    })
    renderApp(ui)
    const u = userEvent.setup()
    await u.click(await screen.findByRole('button', { name: 'Edit rx_dbm' }))
    await u.type(screen.getByLabelText('ID perangkat untuk uji'), '42')
    await u.click(screen.getByRole('button', { name: /Uji/ }))
    const status = await screen.findByRole('status')
    expect(status.textContent).toContain('-21.5')
    expect(status.textContent).toContain('belum dilaporkan')
    expect(status.textContent).toContain('-21500')

    ok = false
    await u.click(screen.getByRole('button', { name: /Uji/ }))
    await waitFor(() => expect(screen.getByRole('status').textContent).toContain('token tak terduga'))
    expect(screen.getByRole('status').textContent).toContain('posisi 4')
  })

  it('uji tanpa ID perangkat meminta ID, tanpa memanggil API', async () => {
    login(['ADMIN'])
    const calls = mockFetch(list([OWN]))
    renderApp(ui)
    const u = userEvent.setup()
    await u.click(await screen.findByRole('button', { name: 'Edit rx_dbm' }))
    await u.click(screen.getByRole('button', { name: /Uji/ }))
    expect((await screen.findByRole('alert')).textContent).toContain('ID perangkat')
    expect(calls.some((c) => c.url.endsWith('/virtual-parameters/test'))).toBe(false)
  })
})
