import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import AutomationPage from './AutomationPage'
import { ConfirmProvider } from '../lib/confirm'
import { setAuthToken } from '../lib/api'
import { mockFetch, renderApp } from '../test/utils'

function login(roles: string[]) {
  setAuthToken('JWT')
  localStorage.setItem('acs_user', JSON.stringify({ id: 1, uuid: 'u', username: 'adm', roles, tenant_id: 7 }))
}

const RULE = {
  id: 'r-1', tenant_id: 7, name: 'Redaman buruk', description: null, trigger_event: 'EVERY_INFORM',
  condition: '{X.Rx} < -27000', actions: [{ type: 'add_tag', tag_id: 5 }, { type: 'set_parameter', name: 'wifi.ssid', value: "'Kantor'" }],
  cooldown_seconds: 3600, max_consecutive_fires: 3, priority: 100, is_active: true, created_at: '', updated_at: '',
}

const ui = (
  <ConfirmProvider>
    <AutomationPage />
  </ConfirmProvider>
)

beforeEach(() => localStorage.clear())
afterEach(() => {
  setAuthToken(null)
  localStorage.clear()
})

function base(extra?: (r: { url: string; method: string; body: unknown }) => { status?: number; json?: unknown } | undefined) {
  return (r: { url: string; method: string; body: unknown }) => {
    const e = extra?.(r)
    if (e) return e
    if (r.url.includes('/tags')) return { json: { data: [{ id: 5, name: 'redaman-buruk' }], total: 1 } }
    if (r.url.includes('/automation-runs')) return { json: { data: [], total: 0 } }
    if (r.url.includes('/automation-rules') && r.method === 'GET') return { json: { data: [RULE], total: 1 } }
    return undefined
  }
}

describe('AutomationPage', () => {
  it('menampilkan aturan: pemicu, kondisi, aksi (tag diterjemahkan ke nama), dan jeda', async () => {
    login(['ADMIN'])
    mockFetch(base())
    renderApp(ui)
    expect(await screen.findByText('Redaman buruk')).toBeTruthy()
    expect(screen.getByText('Setiap Inform')).toBeTruthy()
    expect(screen.getByText('{X.Rx} < -27000')).toBeTruthy()
    expect(await screen.findByText(/Beri tag: redaman-buruk/)).toBeTruthy()
    expect(screen.getByText(/Atur parameter: wifi\.ssid = 'Kantor'/)).toBeTruthy()
    expect(screen.getByText('60 mnt')).toBeTruthy()
  })

  it('NOC hanya membaca: ada Riwayat, tanpa tombol buat/edit/hapus/aktif', async () => {
    login(['NOC'])
    mockFetch(base())
    renderApp(ui)
    await screen.findByText('Redaman buruk')
    expect(screen.getByRole('button', { name: 'Riwayat Redaman buruk' })).toBeTruthy()
    expect(screen.queryByRole('button', { name: /Aturan Baru/ })).toBeNull()
    expect(screen.queryByRole('button', { name: /Edit Redaman buruk|Hapus Redaman buruk/ })).toBeNull()
    expect(screen.queryByRole('checkbox')).toBeNull()
  })

  it('membuat aturan dengan pembangun aksi: POST berisi kondisi, trigger, dan aksi set_parameter', async () => {
    login(['ADMIN'])
    const calls = mockFetch(base((r) => (r.method === 'POST' ? { status: 201, json: RULE } : undefined)))
    renderApp(ui)
    const u = userEvent.setup()
    await u.click(await screen.findByRole('button', { name: /Aturan Baru/ }))
    await u.type(screen.getByLabelText('Nama'), 'SSID seragam')
    await u.selectOptions(screen.getByLabelText('Kapan dievaluasi'), 'BOOTSTRAP_ONLY')
    await u.selectOptions(screen.getByLabelText('Jenis aksi 1'), 'set_parameter')
    await u.type(screen.getByLabelText('Parameter aksi 1'), 'wifi.ssid')
    await u.type(screen.getByLabelText('Nilai aksi 1'), "concat('ACS-', {{@serial_number})")
    await u.click(screen.getByRole('button', { name: 'Simpan' }))
    await waitFor(() => expect(calls.some((c) => c.method === 'POST' && c.url.endsWith('/automation-rules'))).toBe(true))
    const body = calls.find((c) => c.method === 'POST')!.body as Record<string, unknown>
    expect(body.name).toBe('SSID seragam')
    expect(body.trigger_event).toBe('BOOTSTRAP_ONLY')
    expect(body.cooldown_seconds).toBe(3600)
    expect(body.max_consecutive_fires).toBe(3)
    expect(body.actions).toEqual([{ type: 'set_parameter', name: 'wifi.ssid', value: "concat('ACS-', {@serial_number})" }])
    expect('tenant_id' in body).toBe(false)
  })

  it('aksi baca parameter: koma yang diketik tidak hilang dan nama dikirim sebagai daftar', async () => {
    login(['ADMIN'])
    const calls = mockFetch(base((r) => (r.method === 'POST' ? { status: 201, json: RULE } : undefined)))
    renderApp(ui)
    const u = userEvent.setup()
    await u.click(await screen.findByRole('button', { name: /Aturan Baru/ }))
    await u.type(screen.getByLabelText('Nama'), 'Baca')
    await u.selectOptions(screen.getByLabelText('Jenis aksi 1'), 'get_parameter_values')
    const field = screen.getByLabelText('Nama parameter aksi 1') as HTMLInputElement
    await u.type(field, 'A.B, C.D,')
    expect(field.value).toBe('A.B, C.D,') // koma terakhir tetap ada selama mengetik
    await u.click(screen.getByRole('button', { name: 'Simpan' }))
    await waitFor(() => expect(calls.some((c) => c.method === 'POST')).toBe(true))
    expect((calls.find((c) => c.method === 'POST')!.body as { actions: unknown }).actions).toEqual([{ type: 'get_parameter_values', names: ['A.B', 'C.D'] }])
  })

  it('uji pada perangkat: menampilkan kondisi, langkah (saat ini → target), dan kesalahan definisi', async () => {
    login(['ADMIN'])
    let valid = true
    mockFetch(
      base((r) => {
        if (r.url.endsWith('/automation-rules/test')) {
          return valid
            ? {
                json: {
                  ok: true, matched: true, would_fire: true, refs: [{ name: 'X.Rx', value: '-28000' }],
                  steps: [{ type: 'set_parameter', detail: 'set_parameter wifi.ssid=Kantor', path: 'IGD.SSID', value: 'Kantor', current: 'Lama', change: true }],
                },
              }
            : { json: { ok: false, matched: false, would_fire: false, refs: [], steps: [], error: 'condition tidak valid — posisi 3' } }
        }
        return undefined
      }),
    )
    renderApp(ui)
    const u = userEvent.setup()
    await u.click(await screen.findByRole('button', { name: 'Edit Redaman buruk' }))
    await u.type(screen.getByLabelText('ID perangkat untuk uji'), '42')
    await u.click(screen.getByRole('button', { name: /Uji/ }))
    const status = await screen.findByRole('status')
    expect(status.textContent).toContain('terpenuhi')
    expect(status.textContent).toContain('AKAN dijalankan')
    expect(status.textContent).toContain('Lama')
    expect(status.textContent).toContain('Kantor')
    expect(status.textContent).toContain('-28000')

    valid = false
    await u.click(screen.getByRole('button', { name: /Uji/ }))
    await waitFor(() => expect(screen.getByRole('status').textContent).toContain('posisi 3'))
  })

  it('riwayat: menampilkan pemicuan, yang di-suspend, dan tombol buka kembali memanggil reset', async () => {
    login(['ADMIN'])
    const calls = mockFetch(
      base((r) => {
        if (r.url.includes('/automation-runs')) {
          return {
            json: {
              total: 2,
              data: [
                { id: 'a', rule_id: 'r-1', rule_name: 'Redaman buruk', device_id: 1, device_serial: 'ZTEG0001', fired: false, suspended: true, summary: 'reboot', error: 'di-suspend: 3 pemicuan', created_at: new Date().toISOString() },
                { id: 'b', rule_id: 'r-1', rule_name: 'Redaman buruk', device_id: 1, device_serial: 'ZTEG0001', fired: true, suspended: false, summary: 'reboot', error: null, created_at: new Date().toISOString() },
              ],
            },
          }
        }
        if (r.method === 'POST' && r.url.includes('/reset')) return { json: { reset: 2 } }
        return undefined
      }),
    )
    renderApp(ui)
    const u = userEvent.setup()
    await u.click(await screen.findByRole('button', { name: 'Riwayat Redaman buruk' }))
    const dialog = await screen.findByRole('dialog')
    expect(await within(dialog).findByText('di-suspend')).toBeTruthy()
    expect(within(dialog).getByText(/di-suspend: 3 pemicuan/)).toBeTruthy()
    await u.click(within(dialog).getByRole('button', { name: /Buka kembali/ }))
    await u.click(await screen.findByRole('button', { name: 'Buka kembali' }))
    await waitFor(() => expect(calls.some((c) => c.method === 'POST' && c.url.endsWith('/automation-rules/r-1/reset'))).toBe(true))
  })

  it('hapus meminta konfirmasi lalu DELETE', async () => {
    login(['ADMIN'])
    const calls = mockFetch(base((r) => (r.method === 'DELETE' ? { status: 204 } : undefined)))
    renderApp(ui)
    const u = userEvent.setup()
    await u.click(await screen.findByRole('button', { name: 'Hapus Redaman buruk' }))
    await u.click(await screen.findByRole('button', { name: 'Hapus' }))
    await waitFor(() => expect(calls.some((c) => c.method === 'DELETE' && c.url.endsWith('/automation-rules/r-1'))).toBe(true))
  })

  it('daftar kosong menampilkan keadaan kosong', async () => {
    login(['ADMIN'])
    mockFetch((r) => (r.url.includes('/automation-rules') ? { json: { data: [], total: 0 } } : { json: { data: [], total: 0 } }))
    renderApp(ui)
    expect(await screen.findByText('Belum ada aturan otomasi')).toBeTruthy()
  })
})
