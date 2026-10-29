import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { OnuHistoryModal } from './OnuHistoryModal'
import { OltInventory } from './OltInventory'
import { setAuthToken } from '../lib/api'
import { mockFetch, renderApp } from '../test/utils'
import type { OLT, ONUStateEvent, ONUStateHistory, PONPortSummary, RegisteredONU } from '../lib/types'

const minutesAgo = (m: number) => new Date(Date.now() - m * 60_000).toISOString()
const ev = (from: ONUStateEvent['from_state'], to: ONUStateEvent['to_state'], ago: number, rx: number | null = null): ONUStateEvent => ({ pon_port: 'sim/1/1', from_state: from, to_state: to, rx_power_dbm: rx, occurred_at: minutesAgo(ago) })
const hist = (data: ONUStateEvent[], over: Partial<ONUStateHistory> = {}): ONUStateHistory => ({ data, total: data.length, changes_24h: 1, flapping: false, ...over })

beforeEach(() => {
  setAuthToken('JWT')
  localStorage.setItem('acs_user', JSON.stringify({ id: 1, uuid: 'u', username: 'u', roles: ['NOC'], tenant_id: 7 }))
})
afterEach(() => {
  setAuthToken(null)
  localStorage.clear()
})

describe('OnuHistoryModal', () => {
  it('menampilkan peristiwa terbaru dulu dengan teks yang jelas untuk baru/berubah/hilang, plus catatan batasnya', async () => {
    const calls = mockFetch((r) =>
      /\/olts\/olt-1\/registered-onus\/onu-9\/history/.test(r.url)
        ? { json: hist([ev('LOS', null, 1), ev('ONLINE', 'LOS', 10, -29.4), ev(null, 'ONLINE', 600)], { changes_24h: 1 }) }
        : undefined,
    )
    renderApp(<OnuHistoryModal oltId="olt-1" onu={{ id: 'onu-9', serial_number: 'SIMREG0009' }} onClose={() => {}} />)
    expect(await screen.findByText('Riwayat keadaan — SIMREG0009')).toBeTruthy()
    expect(await screen.findByText(/Tidak lagi dilaporkan OLT \(sebelumnya LOS/)).toBeTruthy()
    expect(screen.getByText('Baru terlihat sebagai')).toBeTruthy()
    expect(screen.getByText('-29.4 dBm', { exact: false })).toBeTruthy()
    expect(screen.getByText(/1 perubahan keadaan dalam 24 jam terakhir · 3 peristiwa tercatat/)).toBeTruthy()
    expect(screen.getByText(/bukan saat terjadi/)).toBeTruthy()
    expect(screen.getByText(/tidak tercatat/)).toBeTruthy()
    expect(screen.queryByText(/flapping/)).toBeNull()
    const items = screen.getAllByRole('listitem')
    expect(items).toHaveLength(3)
    expect(within(items[0]).getByText(/Tidak lagi dilaporkan/)).toBeTruthy() // urutan dari server dipertahankan
    const q = new URL(calls[0].url, 'http://x').searchParams
    expect(q.get('page')).toBe('1')
    expect(q.get('page_size')).toBe('20')
  })

  it('flapping: peringatan merah muncul', async () => {
    mockFetch(() => ({ json: hist([ev('ONLINE', 'LOS', 1), ev('LOS', 'ONLINE', 3)], { flapping: true, changes_24h: 6 }) }))
    renderApp(<OnuHistoryModal oltId="olt-1" onu={{ id: 'onu-9', serial_number: 'S' }} onClose={() => {}} />)
    expect((await screen.findByRole('alert')).textContent).toMatch(/flapping.*4 kali atau lebih dalam 1 jam/)
  })

  it('tanpa peristiwa: pesan kosong; galat server: pesan sopan tanpa detail internal', async () => {
    mockFetch(() => ({ json: hist([], { changes_24h: 0 }) }))
    const { unmount } = renderApp(<OnuHistoryModal oltId="olt-1" onu={{ id: 'o', serial_number: 'S' }} onClose={() => {}} />)
    expect(await screen.findByText('Belum ada peristiwa tercatat untuk ONU ini.')).toBeTruthy()
    unmount()
    mockFetch(() => ({ status: 500, json: { message: 'sql: detail internal' } }))
    renderApp(<OnuHistoryModal oltId="olt-1" onu={{ id: 'o', serial_number: 'S' }} onClose={() => {}} />)
    const a = await screen.findByRole('alert')
    expect(a.textContent).toBe('Riwayat tidak dapat dimuat.')
  })

  it('lebih dari satu halaman: berpindah halaman meminta page berikutnya', async () => {
    const calls = mockFetch(() => ({ json: hist([ev('ONLINE', 'LOS', 1)], { total: 45 }) }))
    renderApp(<OnuHistoryModal oltId="olt-1" onu={{ id: 'o', serial_number: 'S' }} onClose={() => {}} />)
    const user = userEvent.setup()
    expect(await screen.findByText('1 / 3')).toBeTruthy()
    await user.click(screen.getByRole('button', { name: 'Halaman berikutnya' }))
    await waitFor(() => expect(calls.some((c) => c.url.includes('page=2'))).toBe(true))
  })
})

describe('OltInventory -> Riwayat', () => {
  const olt = { id: 'olt-1', name: 'OLT Pusat', tenant_id: 7, driver_code: 'sim', host: '10.0.0.1', snmp_port: 161, ssh_port: 22, is_active: true, poll_status: 'OK', last_poll_at: minutesAgo(1) } as unknown as OLT
  const onu = { id: 'onu-1', olt_id: 'olt-1', olt_name: 'OLT Pusat', tenant_id: 7, pon_port: 'sim/1/1', onu_index: '1', serial_number: 'SIMREG0001', onu_type: 'x', state: 'LOS', vendor_state: null, rx_power_dbm: null, first_seen_at: minutesAgo(600), last_seen_at: minutesAgo(1), last_state_change_at: minutesAgo(10), device_id: null, device_status: null, olt_last_poll_at: minutesAgo(1) } as RegisteredONU
  const port: PONPortSummary = { pon_port: 'sim/1/1', total: 1, online: 0, offline: 0, los: 1, dying_gasp: 0, unknown: 0, linked: 0, min_rx_dbm: null, avg_rx_dbm: null, last_seen_at: minutesAgo(1) }

  it('tombol Riwayat membuka modal ONU itu dan tidak meminta riwayat sebelum dibuka', async () => {
    const calls = mockFetch((r) => {
      if (/\/history/.test(r.url)) return { json: hist([ev('ONLINE', 'LOS', 10)]) }
      if (/\/ports/.test(r.url)) return { json: { data: [port], total: 1 } }
      if (/registered-onus/.test(r.url)) return { json: { data: [onu], total: 1 } }
      return undefined
    })
    renderApp(<OltInventory olts={[olt]} />)
    await screen.findByText('SIMREG0001')
    expect(calls.some((c) => c.url.includes('/history'))).toBe(false)
    await userEvent.setup().click(screen.getByRole('button', { name: 'Riwayat keadaan SIMREG0001' }))
    expect(await screen.findByText('Riwayat keadaan — SIMREG0001')).toBeTruthy()
    expect(calls.some((c) => c.url.includes('/olts/olt-1/registered-onus/onu-1/history'))).toBe(true)
  })
})
