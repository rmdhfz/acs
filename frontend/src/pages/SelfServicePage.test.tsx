import { describe, expect, it } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import SelfServicePage from './SelfServicePage'
import { ConfirmProvider } from '../lib/confirm'
import { mockFetch, renderApp } from '../test/utils'

const dev = { id: 3, serial_number: 'ZTE-0001', model: 'F670L', software_version: 'V9.0', status: 'ONLINE', online: true }

function renderPage() {
  return renderApp(
    <ConfirmProvider>
      <SelfServicePage />
    </ConfirmProvider>,
  )
}

describe('SelfServicePage (portal pelanggan)', () => {
  it('tanpa perangkat tertaut: menampilkan keadaan kosong, bukan galat', async () => {
    mockFetch(({ url }) => (url.endsWith('/self-service/devices') ? { json: { data: [], total: 0 } } : undefined))
    renderPage()
    expect(await screen.findByText(/belum ada perangkat/i)).toBeTruthy()
  })

  it('ubah WiFi: validasi sebelum request, lalu PATCH dengan body yang benar', async () => {
    const calls = mockFetch(({ url, method }) => {
      if (url.endsWith('/self-service/devices') && method === 'GET') return { json: { data: [dev], total: 1 } }
      if (url.endsWith('/self-service/devices/3/wifi') && method === 'PATCH') return { status: 202, json: { id: 1 } }
    })
    renderPage()
    const u = userEvent.setup()
    expect(await screen.findByText('ZTE-0001')).toBeTruthy()
    await u.click(screen.getByRole('button', { name: /ubah wifi/i }))

    // kosong semua -> ditolak di klien
    await u.click(screen.getByRole('button', { name: /simpan perubahan/i }))
    expect((await screen.findByRole('alert')).textContent).toMatch(/minimal nama wifi/i)
    expect(calls.some((c) => c.method === 'PATCH')).toBe(false)

    // kata sandi terlalu pendek -> ditolak di klien (label terhubung ke input)
    await u.type(screen.getByLabelText(/kata sandi wifi baru/i), 'pendek')
    await u.click(screen.getByRole('button', { name: /simpan perubahan/i }))
    expect((await screen.findByRole('alert')).textContent).toMatch(/8–63/)
    expect(calls.some((c) => c.method === 'PATCH')).toBe(false)

    // valid
    await u.clear(screen.getByLabelText(/kata sandi wifi baru/i))
    await u.type(screen.getByLabelText(/kata sandi wifi baru/i), 'rahasia-aman-123')
    await u.type(screen.getByLabelText(/nama wifi \(ssid\) baru/i), 'RumahBudi')
    await u.selectOptions(screen.getByLabelText(/terapkan ke/i), '5g')
    await u.click(screen.getByRole('button', { name: /simpan perubahan/i }))
    await waitFor(() => expect(calls.some((c) => c.method === 'PATCH')).toBe(true))
    const patch = calls.find((c) => c.method === 'PATCH')!
    expect(patch.body).toEqual({ ssid: 'RumahBudi', passphrase: 'rahasia-aman-123', band: '5g' })
    expect(await screen.findByText(/perubahan diantre/i)).toBeTruthy()
  })

  it('restart meminta konfirmasi dulu; batal = tidak ada request', async () => {
    const calls = mockFetch(({ url, method }) => {
      if (url.endsWith('/self-service/devices') && method === 'GET') return { json: { data: [dev], total: 1 } }
      if (url.endsWith('/self-service/devices/3/reboot') && method === 'POST') return { status: 204 }
    })
    renderPage()
    const u = userEvent.setup()
    await screen.findByText('ZTE-0001')

    await u.click(screen.getByRole('button', { name: /^restart$/i }))
    await u.click(await screen.findByRole('button', { name: /^batal$/i }))
    expect(calls.some((c) => c.method === 'POST')).toBe(false)

    await u.click(screen.getByRole('button', { name: /^restart$/i }))
    const dialog = await screen.findByRole('dialog')
    await u.click(Array.from(dialog.querySelectorAll('button')).find((b) => b.textContent === 'Restart')!)
    await waitFor(() => expect(calls.some((c) => c.method === 'POST' && c.url.endsWith('/reboot'))).toBe(true))
    expect(await screen.findByText(/perintah restart terkirim/i)).toBeTruthy()
  })

  it('rincian perangkat dimuat dari GET /self-service/devices/:id saat dibuka saja', async () => {
    const calls = mockFetch(({ url, method }) => {
      if (url.endsWith('/self-service/devices') && method === 'GET') return { json: { data: [dev], total: 1 } }
      if (url.endsWith('/self-service/devices/3') && method === 'GET') return { json: { ...dev, software_version: 'V9.1-baru' } }
    })
    renderPage()
    const u = userEvent.setup()
    await screen.findByText('ZTE-0001')
    expect(calls.some((c) => c.url.endsWith('/devices/3'))).toBe(false)
    await u.click(screen.getByRole('button', { name: /lihat rincian perangkat/i }))
    expect(await screen.findByText('V9.1-baru')).toBeTruthy()
  })
})
