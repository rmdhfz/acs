import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { DeviceOverview } from './DeviceOverview'
import { ConfirmProvider } from '../lib/confirm'
import { setAuthToken } from '../lib/api'
import { mockFetch, renderApp } from '../test/utils'

// Peta pemilih lokasi (Leaflet) tidak diuji di jsdom.
vi.mock('./LocationPickerMap', () => ({ default: () => <div data-testid="map-stub" /> }))
import type { Device } from '../lib/types'

const base = {
  id: 5, device_uuid: 'uuid-123', serial_number: 'ELWGC09CED21', oui: '38D82F', product_class: 'F660', mac_address: null,
  software_version: 'V5.2.10P2T10', hardware_version: 'V5.0', ip_address: '172.21.0.1', connection_request_url: null,
  inform_username: 'depok', last_inform_at: new Date(Date.now() - 30_000).toISOString(), last_boot_event_at: null,
  latitude: null, longitude: null, notes: null, created_at: '2026-10-21T03:00:00Z', updated_at: '2026-10-21T03:00:00Z',
} as unknown as Device

function ui(over: Partial<Device> = {}, props: Partial<Parameters<typeof DeviceOverview>[0]> = {}) {
  return renderApp(
    <ConfirmProvider>
      <DeviceOverview
        device={{ ...base, ...over }}
        vendorName="ZTE Corporation"
        statusCode="ONLINE"
        statusLabel="Online"
        canEdit
        onEdit={() => {}}
        onGoTo={() => {}}
        {...props}
      />
    </ConfirmProvider>,
  )
}

beforeEach(() => {
  setAuthToken('JWT')
  localStorage.setItem('acs_user', JSON.stringify({ id: 1, uuid: 'u', username: 'adm', roles: ['ADMIN'], tenant_id: 7 }))
})
afterEach(() => {
  setAuthToken(null)
  localStorage.clear()
})

describe('DeviceOverview', () => {
  it('ringkasan: status, IP, firmware di atas; identitas & vendor di bawah', () => {
    ui()
    expect(screen.getByText('Online')).toBeTruthy()
    expect(screen.getByText(/Inform terakhir/)).toBeTruthy()
    expect(screen.getAllByText('172.21.0.1').length).toBeGreaterThan(0)
    expect(screen.getByText('V5.2.10P2T10')).toBeTruthy()
    expect(screen.getByText('Hardware V5.0')).toBeTruthy()
    expect(screen.getByText('ZTE Corporation')).toBeTruthy()
    expect(screen.getByText('F660')).toBeTruthy()
  })

  it('tanpa Connection Request URL: tampil peringatan dengan tombol Isi URL; dengan URL: status Siap', async () => {
    const onEdit = vi.fn()
    const { unmount } = ui({}, { onEdit })
    expect(screen.getByRole('status').textContent).toMatch(/Connection Request belum tersedia/)
    await userEvent.setup().click(screen.getByRole('button', { name: /isi url/i }))
    expect(onEdit).toHaveBeenCalled()
    unmount()

    ui({ connection_request_url: 'http://10.0.123.11:58000' })
    expect(screen.queryByRole('status')).toBeNull()
    expect(screen.getByText('Siap')).toBeTruthy()
    expect(screen.getByText('http://10.0.123.11:58000')).toBeTruthy()
  })

  it('offline & belum pernah Inform; lokasi kosong vs terisi (tautan peta); tombol Edit hanya bila boleh', () => {
    const { unmount } = ui({ last_inform_at: null }, { statusCode: 'OFFLINE', statusLabel: 'Offline', canEdit: false })
    expect(screen.getByText('Belum pernah Inform')).toBeTruthy()
    expect(screen.getByText(/Lokasi belum diisi/)).toBeTruthy()
    expect(screen.queryByRole('button', { name: /^edit$/i })).toBeNull()
    unmount()

    ui({ latitude: -6.4, longitude: 106.8, notes: 'Rumah pak Budi' })
    const link = screen.getByRole('link', { name: /google maps/i }) as HTMLAnchorElement
    expect(link.href).toContain('q=-6.4,106.8')
    expect(link.rel).toContain('noopener')
    expect(screen.getByText('Rumah pak Budi')).toBeTruthy()
  })

  it('pintasan membuka tab WiFi, Parameter, dan Task', async () => {
    const onGoTo = vi.fn()
    ui({}, { onGoTo })
    const u = userEvent.setup()
    await u.click(screen.getByRole('button', { name: /lihat \/ ubah wifi/i }))
    await u.click(screen.getByRole('button', { name: /parameter tr-069/i }))
    await u.click(screen.getByRole('button', { name: /riwayat task/i }))
    expect(onGoTo.mock.calls.map((c) => c[0])).toEqual(['wifi', 'parameters', 'tasks'])
  })

  describe('Refresh ONT (Connection Request, tanpa reboot)', () => {
    const WITH_URL = { connection_request_url: 'http://10.0.123.11:58000', connection_request_credentials_set: true }

    it('URL & kredensial siap: kirim POST /devices/:id/connection-request', async () => {
      const calls = mockFetch(({ url, method }) => (url.endsWith('/devices/5/connection-request') && method === 'POST' ? { status: 202, json: { message: 'ok' } } : undefined))
      ui(WITH_URL)
      await userEvent.setup().click(screen.getByRole('button', { name: /refresh ont/i }))
      await waitFor(() => expect(calls.some((c) => c.method === 'POST' && c.url.endsWith('/devices/5/connection-request'))).toBe(true))
    })

    it('tanpa URL: tombol nonaktif dengan petunjuk yang jelas', () => {
      ui({ connection_request_url: null })
      const btn = screen.getByRole('button', { name: /refresh ont/i }) as HTMLButtonElement
      expect(btn.disabled).toBe(true)
      expect(btn.title).toMatch(/belum diketahui/i)
    })

    it('kredensial belum diisi: banner + konfirmasi; "Isi kredensial" membuka Edit tanpa mengirim, "Kirim tetap" mengirim', async () => {
      const onEdit = vi.fn()
      const calls = mockFetch(({ url, method }) => (url.endsWith('/devices/5/connection-request') && method === 'POST' ? { status: 202, json: { message: 'ok' } } : undefined))
      const { unmount } = ui({ connection_request_url: 'http://10.0.123.11:58000', connection_request_credentials_set: false }, { onEdit })
      expect(screen.getByText(/kredensial connection request belum diisi/i, { selector: 'p' })).toBeTruthy()
      const u = userEvent.setup()

      await u.click(screen.getByRole('button', { name: /refresh ont/i }))
      // Ada dua tombol "Isi kredensial": di banner dan di dialog konfirmasi (dirender belakangan) → pakai yang di dialog.
      await waitFor(() => expect(screen.getAllByRole('button', { name: /isi kredensial/i }).length).toBe(2))
      const fillBtns = screen.getAllByRole('button', { name: /isi kredensial/i })
      await u.click(fillBtns[fillBtns.length - 1])
      expect(onEdit).toHaveBeenCalled()
      expect(calls.some((c) => c.method === 'POST')).toBe(false)
      unmount()

      ui({ connection_request_url: 'http://10.0.123.11:58000', connection_request_credentials_set: false })
      await u.click(screen.getByRole('button', { name: /refresh ont/i }))
      await u.click(await screen.findByRole('button', { name: /kirim tetap/i }))
      await waitFor(() => expect(calls.some((c) => c.method === 'POST')).toBe(true))
    })

    it('VIEWER tidak melihat tombol Refresh ONT', () => {
      ui(WITH_URL, { canEdit: false })
      expect(screen.queryByRole('button', { name: /refresh ont/i })).toBeNull()
    })
  })

  it('kartu Lokasi: "Pilih di peta" membuka pemilih bagi yang boleh mengubah, dan tidak ada bagi yang tidak', async () => {
    mockFetch(() => undefined)
    const { unmount } = ui({}, { canEdit: true })
    expect(screen.getByText(/Klik Pilih di peta untuk menetapkannya/)).toBeTruthy()
    await userEvent.setup().click(screen.getByRole('button', { name: /pilih di peta/i }))
    expect(await screen.findByText('Tetapkan lokasi — ELWGC09CED21')).toBeTruthy()
    unmount()

    ui({}, { canEdit: false })
    expect(screen.queryByRole('button', { name: /pilih di peta/i })).toBeNull()
    expect(screen.queryByText(/Klik Pilih di peta/)).toBeNull()
  })
})
