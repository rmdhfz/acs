import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ConfigHistory } from './ConfigHistory'
import { setAuthToken } from '../lib/api'
import { ConfirmProvider } from '../lib/confirm'
import { mockFetch, renderApp } from '../test/utils'

const snap = (id: number, data: Record<string, string>) => ({ id, device_id: 5, snapshot_data: data, created_at: '2026-10-21T07:36:00Z' })

function setup(snaps: ReturnType<typeof snap>[]) {
  setAuthToken('JWT')
  localStorage.setItem('acs_user', JSON.stringify({ id: 1, uuid: 'u', username: 'u', roles: ['ADMIN'], tenant_id: 7 }))
  mockFetch((r) => (r.url.includes('/config-snapshots') ? { json: { data: snaps, total: snaps.length } } : undefined))
  renderApp(
    <ConfirmProvider>
      <ConfigHistory deviceId={5} />
    </ConfirmProvider>,
  )
}

beforeEach(() => localStorage.clear())
afterEach(() => {
  setAuthToken(null)
  localStorage.clear()
})

describe('ConfigHistory', () => {
  it('satu snapshot: menjelaskan bahwa perbandingan butuh dua snapshot', async () => {
    setup([snap(6, { 'A.B': '1' })])
    expect(await screen.findByText(/baru ada 1 snapshot/i)).toBeTruthy()
  })

  it('petunjuk mengikuti sisi yang sudah dipilih, lalu menampilkan hanya parameter yang berbeda', async () => {
    setup([snap(2, { 'A.SSID': 'baru', 'A.Same': 'x', 'A.New': 'n' }), snap(1, { 'A.SSID': 'lama', 'A.Same': 'x', 'A.Gone': 'g' })])
    const u = userEvent.setup()
    await screen.findByText(/pilih dua snapshot/i)
    await u.click(screen.getAllByRole('button', { name: /bandingkan \(kiri\)/i })[1]) // #1 = lama
    expect(screen.getByText(/pilih snapshot kanan/i)).toBeTruthy()
    await u.click(screen.getAllByRole('button', { name: /bandingkan \(kanan\)/i })[0]) // #2 = baru
    expect(screen.getByText(/3 berbeda/)).toBeTruthy()
    expect(screen.getByText('A.SSID')).toBeTruthy()
    expect(screen.getByText('A.New')).toBeTruthy()
    expect(screen.getByText('A.Gone')).toBeTruthy()
    expect(screen.queryByText('A.Same')).toBeNull()
  })

  it('dua snapshot identik: "Tidak ada perbedaan", bukan tabel kosong', async () => {
    setup([snap(2, { 'A.B': '1' }), snap(1, { 'A.B': '1' })])
    const u = userEvent.setup()
    await screen.findByText(/pilih dua snapshot/i)
    await u.click(screen.getAllByRole('button', { name: /bandingkan \(kiri\)/i })[0])
    await u.click(screen.getAllByRole('button', { name: /bandingkan \(kanan\)/i })[1])
    expect(screen.getByText(/tidak ada perbedaan konfigurasi \(1 parameter sama\)/i)).toBeTruthy()
  })
})
