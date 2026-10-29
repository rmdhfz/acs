import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { LocationPickerModal } from './LocationPickerModal'
import { setAuthToken } from '../lib/api'
import { mockFetch, renderApp } from '../test/utils'

// Leaflet tidak diuji di jsdom: peta diganti stub yang memperlihatkan nilai dari modal dan dapat "diklik" ke koordinat tertentu.
vi.mock('./LocationPickerMap', () => ({
  default: (p: { value: { lat: number; lon: number } | null; onChange: (c: { lat: number; lon: number }) => void }) => (
    <div data-testid="map-stub" data-value={p.value ? `${p.value.lat},${p.value.lon}` : ''}>
      <button onClick={() => p.onChange({ lat: -7.25, lon: 112.75 })}>klik peta</button>
    </div>
  ),
}))

const dev = (over: Partial<{ latitude: number | null; longitude: number | null }> = {}) => ({
  id: 5, serial_number: 'ZTEGC0001', latitude: null, longitude: null, ...over,
})

beforeEach(() => {
  setAuthToken('JWT')
  localStorage.setItem('acs_user', JSON.stringify({ id: 1, uuid: 'u', username: 'noc', roles: ['NOC'], tenant_id: 7 }))
})
afterEach(() => {
  setAuthToken(null)
  localStorage.clear()
})

const saveBtn = () => screen.getByRole('button', { name: 'Simpan lokasi' }) as HTMLButtonElement

describe('LocationPickerModal', () => {
  it('belum ada lokasi: tombol simpan nonaktif sampai ada pilihan', async () => {
    mockFetch(() => undefined)
    renderApp(<LocationPickerModal device={dev()} onClose={() => {}} />)
    expect(await screen.findByTestId('map-stub')).toBeTruthy()
    expect(saveBtn().disabled).toBe(true)
    expect(screen.getByText('Klik peta, seret penanda, atau tempel koordinat.')).toBeTruthy()
  })

  it('tempel koordinat: peta menerima nilainya, ditampilkan 5 desimal, dan PATCH hanya berisi lintang/bujur', async () => {
    const calls = mockFetch((r) => (r.method === 'PATCH' && r.url.endsWith('/devices/5') ? { json: { id: 5 } } : undefined))
    const onClose = vi.fn()
    renderApp(<LocationPickerModal device={dev()} onClose={onClose} />)
    const user = userEvent.setup()
    await screen.findByTestId('map-stub')
    await user.type(screen.getByLabelText('Tempel koordinat'), '-6.2088, 106.8456')
    expect(screen.getByTestId('map-stub').getAttribute('data-value')).toBe('-6.2088,106.8456')
    expect(screen.getByText('-6.20880, 106.84560')).toBeTruthy()
    await user.click(saveBtn())
    await waitFor(() => expect(onClose).toHaveBeenCalledTimes(1))
    expect(calls.filter((c) => c.method === 'PATCH')).toHaveLength(1)
    expect(calls.find((c) => c.method === 'PATCH')?.body).toEqual({ latitude: -6.2088, longitude: 106.8456 })
  })

  it('klik peta menetapkan nilai', async () => {
    const calls = mockFetch((r) => (r.method === 'PATCH' ? { json: { id: 5 } } : undefined))
    renderApp(<LocationPickerModal device={dev()} onClose={() => {}} />)
    const user = userEvent.setup()
    await user.click(await screen.findByRole('button', { name: 'klik peta' }))
    expect(screen.getByText('-7.25000, 112.75000')).toBeTruthy()
    await user.click(saveBtn())
    await waitFor(() => expect(calls.some((c) => c.method === 'PATCH')).toBe(true))
    expect(calls.find((c) => c.method === 'PATCH')?.body).toEqual({ latitude: -7.25, longitude: 112.75 })
  })

  it('koordinat tak terbaca: pesan galat, tidak mengubah pilihan, simpan tetap nonaktif', async () => {
    mockFetch(() => undefined)
    renderApp(<LocationPickerModal device={dev()} onClose={() => {}} />)
    const user = userEvent.setup()
    await screen.findByTestId('map-stub')
    await user.type(screen.getByLabelText('Tempel koordinat'), '95, 10')
    expect((await screen.findByRole('alert')).textContent).toContain('Format koordinat tidak dikenali')
    expect(screen.getByTestId('map-stub').getAttribute('data-value')).toBe('')
    expect(saveBtn().disabled).toBe(true)
    // mengosongkan kolom menghapus pesan
    await user.clear(screen.getByLabelText('Tempel koordinat'))
    expect(screen.queryByRole('alert')).toBeNull()
  })

  it('lokasi yang sudah ada dimuat; tanpa perubahan tidak bisa disimpan lagi', async () => {
    mockFetch(() => undefined)
    renderApp(<LocationPickerModal device={dev({ latitude: -6.2, longitude: 106.8 })} onClose={() => {}} />)
    const user = userEvent.setup()
    expect((await screen.findByTestId('map-stub')).getAttribute('data-value')).toBe('-6.2,106.8')
    expect(saveBtn().disabled).toBe(true)
    await user.click(screen.getByRole('button', { name: 'klik peta' }))
    expect(saveBtn().disabled).toBe(false)
  })

  it('koordinat tersimpan yang di luar rentang diabaikan (bukan dipakai sebagai nilai awal)', async () => {
    mockFetch(() => undefined)
    renderApp(<LocationPickerModal device={dev({ latitude: 123, longitude: 106.8 })} onClose={() => {}} />)
    expect((await screen.findByTestId('map-stub')).getAttribute('data-value')).toBe('')
  })

  it('gagal menyimpan: modal tetap terbuka dan galat ditampilkan', async () => {
    mockFetch((r) => (r.method === 'PATCH' ? { status: 403, json: { message: 'dilarang' } } : undefined))
    const onClose = vi.fn()
    renderApp(<LocationPickerModal device={dev()} onClose={onClose} />)
    const user = userEvent.setup()
    await user.click(await screen.findByRole('button', { name: 'klik peta' }))
    await user.click(saveBtn())
    expect(await screen.findByText('dilarang')).toBeTruthy()
    expect(onClose).not.toHaveBeenCalled()
    expect(saveBtn().disabled).toBe(false) // boleh dicoba lagi
  })
})
