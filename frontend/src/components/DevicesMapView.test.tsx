import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { DevicesMapView } from './DevicesMapView'
import { setAuthToken } from '../lib/api'
import { mockFetch, renderApp } from '../test/utils'
import type { DeviceMapPoint, DeviceMapResponse } from '../lib/types'

// Leaflet sendiri tidak diuji di jsdom: DeviceMap diganti stub yang memperlihatkan apa yang DevicesMapView berikan kepadanya.
vi.mock('./DeviceMap', () => ({
  default: (p: { points: unknown[]; fitKey?: string; onOpen: (id: number) => void; onViewportChange?: (b: string | undefined) => void }) => (
    <div data-testid="map-stub" data-count={p.points.length} data-fit={p.fitKey ?? ''}>
      <button onClick={() => p.onViewportChange?.('106,-7,108,-6')}>geser</button>
      <button onClick={() => p.onOpen(5)}>buka</button>
    </div>
  ),
}))

const pt = (id: number, status: string): DeviceMapPoint => ({
  id, device_uuid: `u${id}`, serial_number: `SN${id}`, product_class: 'F660', latitude: -6.2, longitude: 106.8, status, last_inform_at: null,
})

const resp = (over: Partial<DeviceMapResponse> & { data: DeviceMapPoint[] }): DeviceMapResponse => ({
  total: over.data.length, located: over.data.length, truncated: false, ...over,
})

beforeEach(() => {
  setAuthToken('JWT')
  localStorage.setItem('acs_user', JSON.stringify({ id: 1, uuid: 'u', username: 'u', roles: ['NOC'], tenant_id: 7 }))
})
afterEach(() => {
  setAuthToken(null)
  localStorage.clear()
})

describe('DevicesMapView', () => {
  it('memberi titik ke peta dan menghitung legenda per status', async () => {
    mockFetch((r) => (r.url.includes('/devices/map') ? { json: resp({ data: [pt(1, 'ONLINE'), pt(2, 'ONLINE'), pt(3, 'OFFLINE'), pt(4, 'UNREGISTERED')] }) } : undefined))
    renderApp(<DevicesMapView filters={{}} onOpenDevice={() => {}} />)
    const stub = await screen.findByTestId('map-stub')
    expect(stub.getAttribute('data-count')).toBe('4')
    expect(screen.getByText('4 perangkat di peta')).toBeTruthy()
    const legend = screen.getByText('4 perangkat di peta').parentElement!
    expect(legend.textContent).toContain('Online 2')
    expect(legend.textContent).toContain('Offline 1')
    expect(legend.textContent).toContain('Lainnya 1')
    expect(screen.queryByRole('status')).toBeNull() // tak ada catatan bila semua tergambar & tidak terpotong
  })

  it('perangkat tanpa koordinat dinyatakan dengan angka yang benar', async () => {
    mockFetch((r) => (r.url.includes('/devices/map') ? { json: resp({ data: [pt(1, 'ONLINE')], total: 10, located: 1 }) } : undefined))
    renderApp(<DevicesMapView filters={{}} onOpenDevice={() => {}} />)
    expect((await screen.findByRole('status')).textContent).toBe('9 dari 10 perangkat belum punya koordinat dan tidak tampil di peta.')
  })

  it('hasil dipotong: catatan muncul', async () => {
    mockFetch((r) => (r.url.includes('/devices/map') ? { json: resp({ data: [pt(1, 'ONLINE')], total: 9000, located: 9000, truncated: true }) } : undefined))
    renderApp(<DevicesMapView filters={{}} onOpenDevice={() => {}} />)
    expect((await screen.findByRole('status')).textContent).toContain('dipotong pada 5000 titik')
  })

  it('tak ada satu pun yang berkoordinat: tampilan kosong menjelaskan cara mengisinya, tanpa peta', async () => {
    mockFetch((r) => (r.url.includes('/devices/map') ? { json: resp({ data: [], total: 12, located: 0 }) } : undefined))
    renderApp(<DevicesMapView filters={{}} onOpenDevice={() => {}} />)
    expect(await screen.findByText('Belum ada perangkat yang punya koordinat')).toBeTruthy()
    expect(screen.getByText(/12 perangkat cocok dengan filter tetapi belum punya lokasi/)).toBeTruthy()
    expect(screen.queryByTestId('map-stub')).toBeNull()
  })

  it('filter tidak cocok sama sekali: pesannya berbeda dari "belum ada koordinat"', async () => {
    mockFetch((r) => (r.url.includes('/devices/map') ? { json: resp({ data: [], total: 0, located: 0 }) } : undefined))
    renderApp(<DevicesMapView filters={{ search: 'zzz' }} onOpenDevice={() => {}} />)
    expect(await screen.findByText('Tidak ada perangkat yang cocok dengan filter')).toBeTruthy()
  })

  it('memakai filter daftar yang sama, tanpa paginasi/urutan', async () => {
    const calls = mockFetch((r) => (r.url.includes('/devices/map') ? { json: resp({ data: [pt(1, 'ONLINE')] }) } : undefined))
    renderApp(
      <DevicesMapView
        filters={{ search: 'ZTE', device_status_id: 2, vendor_id: 3, tag_id: 4, optical_level: 'bad', param: ['A~eq~1', 'B~exists'], sort: '-id', page: 3, page_size: 25 }}
        onOpenDevice={() => {}}
      />,
    )
    await screen.findByTestId('map-stub')
    const url = new URL(calls.find((c) => c.url.includes('/devices/map'))!.url, 'http://x')
    expect(url.searchParams.get('search')).toBe('ZTE')
    expect(url.searchParams.get('device_status_id')).toBe('2')
    expect(url.searchParams.get('vendor_id')).toBe('3')
    expect(url.searchParams.get('tag_id')).toBe('4')
    expect(url.searchParams.get('optical_level')).toBe('bad')
    expect(url.searchParams.getAll('param')).toEqual(['A~eq~1', 'B~exists'])
    for (const k of ['sort', 'page', 'page_size', 'bbox']) expect(url.searchParams.has(k)).toBe(false)
  })

  it('"buka" pada peta menavigasi ke perangkat itu', async () => {
    mockFetch((r) => (r.url.includes('/devices/map') ? { json: resp({ data: [pt(5, 'ONLINE')] }) } : undefined))
    const onOpen = vi.fn()
    renderApp(<DevicesMapView filters={{}} onOpenDevice={onOpen} />)
    await userEvent.setup().click(await screen.findByRole('button', { name: 'buka' }))
    expect(onOpen).toHaveBeenCalledWith(5)
  })

  it('hasil dipotong -> mode viewport: menggeser peta meminta ulang dengan bbox; tanpa itu bbox tak pernah dikirim', async () => {
    const calls = mockFetch((r) => {
      if (!r.url.includes('/devices/map')) return undefined
      return { json: r.url.includes('bbox=') ? resp({ data: [pt(1, 'ONLINE')], total: 9000, located: 9000 }) : resp({ data: [pt(1, 'ONLINE')], total: 9000, located: 9000, truncated: true }) }
    })
    renderApp(<DevicesMapView filters={{}} onOpenDevice={() => {}} />)
    await screen.findByTestId('map-stub')
    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: 'geser' }))
    await waitFor(() => expect(calls.some((c) => c.url.includes('bbox=106%2C-7%2C108%2C-6'))).toBe(true))
    expect(calls.filter((c) => c.url.includes('bbox=')).length).toBe(1)
  })

  it('tidak terpotong: menggeser peta TIDAK memicu permintaan baru (semua titik sudah ada)', async () => {
    const calls = mockFetch((r) => (r.url.includes('/devices/map') ? { json: resp({ data: [pt(1, 'ONLINE')] }) } : undefined))
    renderApp(<DevicesMapView filters={{}} onOpenDevice={() => {}} />)
    await screen.findByTestId('map-stub')
    await userEvent.setup().click(screen.getByRole('button', { name: 'geser' }))
    await new Promise((r) => setTimeout(r, 50))
    expect(calls.filter((c) => c.url.includes('/devices/map')).length).toBe(1)
  })

  it('fitKey diberikan hanya saat data sudah sesuai filter (bukan data placeholder lama)', async () => {
    mockFetch((r) => (r.url.includes('/devices/map') ? { json: resp({ data: [pt(1, 'ONLINE')] }) } : undefined))
    renderApp(<DevicesMapView filters={{ search: 'a' }} onOpenDevice={() => {}} />)
    const stub = await screen.findByTestId('map-stub')
    expect(stub.getAttribute('data-fit')).toBe(JSON.stringify({ search: 'a' }))
  })
})
