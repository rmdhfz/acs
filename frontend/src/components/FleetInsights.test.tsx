import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { screen, within } from '@testing-library/react'
import { FleetInsights } from './FleetInsights'
import { setAuthToken } from '../lib/api'
import { mockFetch, renderApp } from '../test/utils'

const b = (pairs: Record<string, number>) => Object.entries(pairs).map(([key, count]) => ({ key, count }))

const DATA = {
  online: 10,
  offline: 4,
  other: 1,
  pon_mode: b({ gpon: 6, epon: 1, unknown: 3 }),
  uptime: b({ lt1d: 2, d1_7: 3, d7_30: 4, gt30d: 1, unknown: 0 }),
  optical_rx: b({ ok: 6, warn: 2, bad: 1, overload: 0, unknown: 1 }),
  temperature: b({ normal: 0, warning: 0, high: 0, critical: 0, unknown: 10 }),
  offline_age: b({ lt1d: 1, d1_3: 1, d3_7: 0, d7_30: 1, gt30d: 1, unknown: 0 }),
}

function setup(json: unknown, status = 200) {
  setAuthToken('JWT')
  localStorage.setItem('acs_user', JSON.stringify({ id: 1, uuid: 'u', username: 'u', roles: ['ADMIN'], tenant_id: 7 }))
  mockFetch((r) => (r.url.endsWith('/devices/insights') ? { status, json } : undefined))
  renderApp(<FleetInsights tooltipStyle={{}} />)
}

beforeEach(() => localStorage.clear())
afterEach(() => {
  setAuthToken(null)
  localStorage.clear()
})

describe('FleetInsights', () => {
  it('menampilkan strip status, distribusi optik, mode PON, dan lama offline dengan persentase', async () => {
    setup(DATA)
    expect(await screen.findByText('Kualitas optik RX')).toBeTruthy()
    // strip: 10 / 15 = 67%
    expect(screen.getAllByText('Daring').find((el) => el.closest('a'))?.closest('a')?.textContent).toMatch(/67%/)
    // optik: dari 10 perangkat daring, Baik 6 dari 9 yang terbaca = 67%, tautan ke filter yang sama
    const optik = screen.getByRole('region', { name: 'Kualitas optik RX' })
    expect(within(optik).getByText('Baik').closest('a')?.getAttribute('href')).toBe('/devices?optical_level=ok')
    expect(within(optik).getByText('Baik 67%')).toBeTruthy()
    expect(within(optik).getByText(/1 perangkat di luar rentang aman/)).toBeTruthy()
    // keranjang kosong (kelebihan beban = 0) tetap tampil sebagai baris tanpa tautan
    expect(within(optik).getByText('Kelebihan beban').closest('a')).toBeNull()
    const pon = screen.getByRole('region', { name: 'Mode PON' })
    expect(within(pon).getByText('GPON')).toBeTruthy()
    expect(within(pon).getByText('7 terdeteksi')).toBeTruthy()
    expect(within(screen.getByRole('region', { name: 'Lama offline' })).getByText('4 luring')).toBeTruthy()
  })

  it('suhu belum terbaca: memberi petunjuk memetakan sumbernya (bukan angka kosong yang membingungkan)', async () => {
    setup(DATA)
    const suhu = await screen.findByRole('region', { name: 'Suhu ONU' })
    expect(within(suhu).getByText('Belum ada data')).toBeTruthy()
    expect(within(suhu).getByText(/petakan sumbernya per model/i)).toBeTruthy()
  })

  it('armada kosong atau API gagal: tidak menampilkan apa pun', async () => {
    setup({ ...DATA, online: 0, offline: 0, other: 0 })
    await new Promise((r) => setTimeout(r, 50))
    expect(screen.queryByText('Kualitas optik RX')).toBeNull()
  })
})
