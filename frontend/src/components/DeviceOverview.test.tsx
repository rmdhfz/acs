import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { DeviceOverview } from './DeviceOverview'
import { I18nProvider } from '../lib/i18n'
import type { Device } from '../lib/types'

const base = {
  id: 5, device_uuid: 'uuid-123', serial_number: 'ELWGC09CED21', oui: '38D82F', product_class: 'F660', mac_address: null,
  software_version: 'V5.2.10P2T10', hardware_version: 'V5.0', ip_address: '172.21.0.1', connection_request_url: null,
  inform_username: 'depok', last_inform_at: new Date(Date.now() - 30_000).toISOString(), last_boot_event_at: null,
  latitude: null, longitude: null, notes: null, created_at: '2026-10-21T03:00:00Z', updated_at: '2026-10-21T03:00:00Z',
} as unknown as Device

function ui(over: Partial<Device> = {}, props: Partial<Parameters<typeof DeviceOverview>[0]> = {}) {
  return render(
    <I18nProvider>
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
    </I18nProvider>,
  )
}

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
})
