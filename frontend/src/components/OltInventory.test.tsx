import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { OltInventory } from './OltInventory'
import { setAuthToken } from '../lib/api'
import { mockFetch, renderApp } from '../test/utils'
import type { OLT, PONPortSummary, RegisteredONU } from '../lib/types'

const minutesAgo = (m: number) => new Date(Date.now() - m * 60_000).toISOString()

const olt = (id: string, name: string, lastPollMin: number | null): OLT =>
  ({ id, name, tenant_id: 7, driver_code: 'sim', host: '10.0.0.1', snmp_port: 161, ssh_port: 22, is_active: true, poll_status: 'OK', last_poll_at: lastPollMin == null ? null : minutesAgo(lastPollMin) }) as unknown as OLT

const onu = (n: number, over: Partial<RegisteredONU> = {}): RegisteredONU => ({
  id: `onu-${n}`, olt_id: 'olt-1', olt_name: 'OLT Pusat', tenant_id: 7, pon_port: n <= 2 ? 'sim/1/1' : 'sim/1/2', onu_index: String(n), serial_number: `SIMREG000${n}`,
  onu_type: 'SIM-ONU', state: 'ONLINE', vendor_state: 'v', rx_power_dbm: -21.54, first_seen_at: minutesAgo(600), last_seen_at: minutesAgo(1), last_state_change_at: minutesAgo(120),
  device_id: null, device_status: null, olt_last_poll_at: minutesAgo(1), ...over,
})

const PORTS: PONPortSummary[] = [
  { pon_port: 'sim/1/1', total: 3, online: 1, offline: 0, los: 1, dying_gasp: 1, unknown: 0, linked: 1, min_rx_dbm: -24.8, avg_rx_dbm: -22.1, last_seen_at: minutesAgo(1) },
  { pon_port: 'sim/1/2', total: 2, online: 1, offline: 1, los: 0, dying_gasp: 0, unknown: 0, linked: 0, min_rx_dbm: null, avg_rx_dbm: null, last_seen_at: minutesAgo(1) },
]

const ONUS: RegisteredONU[] = [
  onu(1, { state: 'ONLINE', device_id: 42, device_status: 'ONLINE' }),
  onu(2, { state: 'LOS', rx_power_dbm: null }),
  onu(3, { state: 'DYING_GASP', rx_power_dbm: null }),
  onu(4, { state: 'UNKNOWN', vendor_state: 'aneh-1' }),
]

function handler(opts: { onus?: RegisteredONU[]; total?: number; ports?: PONPortSummary[]; status?: number } = {}) {
  return (r: { url: string }) => {
    if (opts.status && /registered-onus|\/ports/.test(r.url)) return { status: opts.status, json: { message: 'tidak aktif' } }
    if (/\/olts\/[^/]+\/ports/.test(r.url)) return { json: { data: opts.ports ?? PORTS, total: (opts.ports ?? PORTS).length } }
    if (/registered-onus/.test(r.url)) {
      const data = opts.onus ?? ONUS
      return { json: { data, total: opts.total ?? data.length } }
    }
    return undefined
  }
}

beforeEach(() => {
  setAuthToken('JWT')
  localStorage.setItem('acs_user', JSON.stringify({ id: 1, uuid: 'u', username: 'u', roles: ['NOC'], tenant_id: 7 }))
})
afterEach(() => {
  setAuthToken(null)
  localStorage.clear()
})

describe('OltInventory', () => {
  it('menampilkan ringkasan, keadaan netral dengan label jelas, RX, dan tautan ke perangkat ACS', async () => {
    mockFetch(handler())
    renderApp(<OltInventory olts={[olt('olt-1', 'OLT Pusat', 1)]} />)
    expect(await screen.findByText('SIMREG0001')).toBeTruthy()
    expect(screen.getByText(/5 ONU · 2 online/)).toBeTruthy()
    expect(screen.getByText(/2 LOS\/dying gasp/)).toBeTruthy()
    const rows = screen.getAllByRole('row')
    const losRow = rows.find((r) => within(r).queryByText('SIMREG0002'))!
    expect(within(losRow).getByText('LOS (sinyal hilang)')).toBeTruthy()
    expect(within(losRow).getByText('Belum tertaut')).toBeTruthy()
    const gaspRow = rows.find((r) => within(r).queryByText('SIMREG0003'))!
    expect(within(gaspRow).getByText('Dying gasp (daya hilang)')).toBeTruthy()
    const unknownRow = rows.find((r) => within(r).queryByText('SIMREG0004'))!
    expect(within(unknownRow).getByText('Tidak diketahui')).toBeTruthy()
    expect(within(unknownRow).getByText('(aneh-1)')).toBeTruthy() // keadaan asli ditampilkan hanya untuk yang tak terpetakan
    const okRow = rows.find((r) => within(r).queryByText('SIMREG0001'))!
    expect(within(okRow).getByText('-21.5 dBm')).toBeTruthy()
    const link = within(okRow).getByRole('link', { name: /Buka perangkat \(ONLINE\)/ })
    expect(link.getAttribute('href')).toBe('/devices/42')
  })

  it('usia data: sebut waktu polling, beri catatan bila > 30 menit, dan diam bila segar', async () => {
    mockFetch(handler())
    const { unmount } = renderApp(<OltInventory olts={[olt('olt-1', 'OLT Pusat', 1)]} />)
    await screen.findByText('SIMREG0001')
    expect(screen.queryByText(/sudah lebih dari 30 menit/)).toBeNull()
    unmount()

    mockFetch(handler())
    renderApp(<OltInventory olts={[olt('olt-1', 'OLT Pusat', 90)]} />)
    expect(await screen.findByText(/sudah lebih dari 30 menit/)).toBeTruthy()
    expect(screen.getByText(/ACS_OLT_POLL_MINUTES/)).toBeTruthy()
  })

  it('OLT yang belum pernah dipoll: tampilan kosong menyuruh Poll, tanpa catatan basi', async () => {
    mockFetch(handler({ onus: [], ports: [] }))
    renderApp(<OltInventory olts={[olt('olt-1', 'OLT Pusat', null)]} />)
    expect(await screen.findByText('Belum ada ONU terdaftar')).toBeTruthy()
    expect(screen.getByText(/belum pernah dipoll/)).toBeTruthy()
    expect(screen.getByText(/Tekan Poll pada OLT di atas/)).toBeTruthy()
    expect(screen.queryByText(/sudah lebih dari 30 menit/)).toBeNull()
  })

  it('OLT sudah dipoll tetapi kosong: pesan berbeda (driver mungkin tak mendukung)', async () => {
    mockFetch(handler({ onus: [], ports: [] }))
    renderApp(<OltInventory olts={[olt('olt-1', 'OLT Pusat', 2)]} />)
    expect(await screen.findByText(/sudah dipoll tetapi tidak melaporkan ONU terdaftar/)).toBeTruthy()
  })

  it('filter keadaan, port, dan cari dikirim ke server; kembali ke halaman 1', async () => {
    const calls = mockFetch(handler())
    renderApp(<OltInventory olts={[olt('olt-1', 'OLT Pusat', 1)]} />)
    await screen.findByText('SIMREG0001')
    const user = userEvent.setup()
    await user.selectOptions(screen.getByLabelText('Filter keadaan'), 'LOS')
    await user.selectOptions(screen.getByLabelText('Filter port PON'), 'sim/1/1')
    await user.type(screen.getByLabelText('Cari ONU terdaftar'), 'REG2')
    await waitFor(() => {
      const last = new URL(calls.filter((c) => c.url.includes('registered-onus')).at(-1)!.url, 'http://x').searchParams
      expect(last.get('state')).toBe('LOS')
      expect(last.get('pon_port')).toBe('sim/1/1')
      expect(last.get('search')).toBe('REG2')
      expect(last.get('page')).toBe('1')
    })
  })

  it('tab Port PON: baris bermasalah ditandai dan "Lihat ONU" memfilter port itu', async () => {
    const calls = mockFetch(handler())
    renderApp(<OltInventory olts={[olt('olt-1', 'OLT Pusat', 1)]} />)
    const user = userEvent.setup()
    await user.click(await screen.findByRole('button', { name: 'Port PON' }))
    const row = (await screen.findByText('sim/1/1')).closest('tr')!
    expect(within(row).getAllByText('1').length).toBeGreaterThanOrEqual(2) // LOS 1 dan dying gasp 1
    expect(row.className).toMatch(/bg-red/)
    expect(within(row).getByText('-24.8 dBm / -22.1 dBm')).toBeTruthy()
    const ok = screen.getByText('sim/1/2').closest('tr')!
    expect(ok.className).toMatch(/bg-amber/) // ada yang offline
    expect(within(ok).getByText('— / —')).toBeTruthy()

    await user.click(screen.getByRole('button', { name: 'Lihat ONU di port sim/1/1' }))
    expect(screen.getByRole('button', { name: 'ONU terdaftar' }).getAttribute('aria-pressed')).toBe('true')
    await waitFor(() => {
      const last = new URL(calls.filter((c) => c.url.includes('registered-onus')).at(-1)!.url, 'http://x').searchParams
      expect(last.get('pon_port')).toBe('sim/1/1')
    })
  })

  it('beberapa OLT: pemilih OLT memuat OLT lain dan menyetel ulang filter', async () => {
    const calls = mockFetch(handler())
    renderApp(<OltInventory olts={[olt('olt-1', 'OLT Pusat', 1), olt('olt-2', 'OLT Cabang', 1)]} />)
    await screen.findByText('SIMREG0001')
    const user = userEvent.setup()
    await user.selectOptions(screen.getByLabelText('Filter keadaan'), 'LOS')
    await user.selectOptions(screen.getByLabelText('Pilih OLT'), 'olt-2')
    await waitFor(() => {
      const u = calls.filter((c) => c.url.includes('/olts/olt-2/registered-onus')).at(-1)
      expect(u).toBeTruthy()
      expect(new URL(u!.url, 'http://x').searchParams.get('state')).toBeNull() // filter tidak terbawa ke OLT lain
    })
    expect((screen.getByLabelText('Filter keadaan') as HTMLSelectElement).value).toBe('')
  })

  it('paginasi: total di atas satu halaman menampilkan 1 / 3 dan tombol berikutnya meminta page=2', async () => {
    const calls = mockFetch(handler({ total: 120 }))
    renderApp(<OltInventory olts={[olt('olt-1', 'OLT Pusat', 1)]} />)
    await screen.findByText('SIMREG0001')
    expect(screen.getByText('1 / 3')).toBeTruthy()
    await userEvent.setup().click(screen.getByRole('button', { name: 'Halaman berikutnya' }))
    await waitFor(() => expect(calls.some((c) => c.url.includes('registered-onus') && c.url.includes('page=2'))).toBe(true))
  })

  it('API menolak (inventaris tidak aktif): pesan jelas, bukan tabel kosong yang menyesatkan', async () => {
    mockFetch(handler({ status: 400 }))
    renderApp(<OltInventory olts={[olt('olt-1', 'OLT Pusat', 1)]} />)
    expect(await screen.findByText('Inventaris ONU terdaftar tidak tersedia untuk OLT ini.')).toBeTruthy()
    expect(screen.queryByText('Belum ada ONU terdaftar')).toBeNull()
  })

  it('tanpa OLT: tidak merender apa pun', () => {
    mockFetch(handler())
    const { container } = renderApp(<OltInventory olts={[]} />)
    expect(container.querySelector('section')).toBeNull()
  })

  const lastOnuQuery = (calls: { url: string }[]) => new URL(calls.filter((c) => c.url.includes('registered-onus')).at(-1)!.url, 'http://x').searchParams

  it('urutan masalah-dulu diminta bawaan; "Bermasalah saja" menambah problem=true dan bisa dimatikan lagi', async () => {
    const calls = mockFetch(handler())
    renderApp(<OltInventory olts={[olt('olt-1', 'OLT Pusat', 1)]} />)
    await screen.findByText('SIMREG0001')
    expect(lastOnuQuery(calls).get('sort')).toBe('severity')
    expect(lastOnuQuery(calls).has('problem')).toBe(false)
    const user = userEvent.setup()
    const chip = screen.getByRole('button', { name: 'Bermasalah saja' })
    expect(chip.getAttribute('aria-pressed')).toBe('false')
    await user.click(chip)
    expect(chip.getAttribute('aria-pressed')).toBe('true')
    await waitFor(() => expect(lastOnuQuery(calls).get('problem')).toBe('true'))
    await user.click(chip)
    await waitFor(() => expect(lastOnuQuery(calls).has('problem')).toBe(false))
  })

  it('tautan dalam /olts?olt=...&problem=1: memilih OLT itu dan menyalakan filter sejak render pertama', async () => {
    const calls = mockFetch(handler())
    renderApp(<OltInventory olts={[olt('olt-1', 'OLT Satu', 1), olt('olt-2', 'OLT Dua', 1)]} />, '/olts?olt=olt-2&problem=1')
    await screen.findByText('SIMREG0001')
    const onuCalls = calls.filter((c) => c.url.includes('registered-onus'))
    expect(onuCalls.every((c) => c.url.includes('/olts/olt-2/'))).toBe(true) // tak pernah memuat OLT pertama dulu
    expect(lastOnuQuery(calls).get('problem')).toBe('true')
    expect(screen.getByRole('button', { name: 'Bermasalah saja' }).getAttribute('aria-pressed')).toBe('true')
    expect((screen.getByLabelText('Pilih OLT') as HTMLSelectElement).value).toBe('olt-2')
  })

  it('tautan dalam dengan OLT yang tidak dikenal jatuh ke OLT pertama (tanpa galat)', async () => {
    mockFetch(handler())
    renderApp(<OltInventory olts={[olt('olt-1', 'OLT Satu', 1), olt('olt-2', 'OLT Dua', 1)]} />, '/olts?olt=hilang')
    await screen.findByText('SIMREG0001')
    expect((screen.getByLabelText('Pilih OLT') as HTMLSelectElement).value).toBe('olt-1')
  })

  it('ganti OLT manual mereset filter (termasuk "bermasalah saja")', async () => {
    const calls = mockFetch(handler())
    renderApp(<OltInventory olts={[olt('olt-1', 'OLT Satu', 1), olt('olt-2', 'OLT Dua', 1)]} />, '/olts?problem=1')
    await screen.findByText('SIMREG0001')
    expect(lastOnuQuery(calls).get('problem')).toBe('true')
    await userEvent.setup().selectOptions(screen.getByLabelText('Pilih OLT'), 'olt-2')
    await waitFor(() => {
      const q = lastOnuQuery(calls)
      expect(calls.filter((c) => c.url.includes('registered-onus')).at(-1)!.url).toContain('/olts/olt-2/')
      expect(q.has('problem')).toBe(false)
    })
  })

  it('"Keadaan sejak": baris baru ditemukan diberi label jujur, perubahan yang teramati memakai waktunya', async () => {
    const seen = minutesAgo(2)
    mockFetch(
      handler({
        onus: [
          onu(2, { state: 'LOS', first_seen_at: seen, last_state_change_at: seen }),
          onu(1, { state: 'ONLINE', first_seen_at: minutesAgo(600), last_state_change_at: minutesAgo(120) }),
        ],
      }),
    )
    renderApp(<OltInventory olts={[olt('olt-1', 'OLT Pusat', 1)]} />)
    await screen.findByText('SIMREG0002')
    const rows = screen.getAllByRole('row')
    const losRow = rows.find((r) => within(r).queryByText('SIMREG0002'))!
    expect(within(losRow).getByText(/terlihat sejak/)).toBeTruthy()
    const okRow = rows.find((r) => within(r).queryByText('SIMREG0001'))!
    expect(within(okRow).queryByText(/terlihat sejak/)).toBeNull()
  })

  it('ringkasan menyebut jumlah offline; banner dugaan muncul untuk port dengan LOS massal dan menyaring port itu', async () => {
    const hot: PONPortSummary = { pon_port: 'sim/2/1', total: 8, online: 1, offline: 0, los: 7, dying_gasp: 0, unknown: 0, linked: 0, min_rx_dbm: null, avg_rx_dbm: null, last_seen_at: minutesAgo(1) }
    const calls = mockFetch(handler({ ports: [...PORTS, hot] }))
    renderApp(<OltInventory olts={[olt('olt-1', 'OLT Pusat', 1)]} />)
    await screen.findByText('SIMREG0001')
    expect(screen.getByText(/1 offline/)).toBeTruthy()
    const banner = await screen.findByRole('alert', { name: 'Port PON dengan dugaan gangguan' })
    expect(within(banner).getByText(/7 dari 8 ONU LOS/)).toBeTruthy()
    expect(within(banner).getByText(/bukan kepastian/)).toBeTruthy()
    expect(within(banner).queryByText('sim/1/1')).toBeNull() // port kecil biasa tidak ditandai
    await userEvent.setup().click(within(banner).getByRole('button', { name: 'Lihat ONU bermasalah di port sim/2/1' }))
    await waitFor(() => {
      const q = lastOnuQuery(calls)
      expect(q.get('pon_port')).toBe('sim/2/1')
      expect(q.get('problem')).toBe('true')
    })
  })

  it('tanpa port yang memenuhi ambang: tidak ada banner', async () => {
    mockFetch(handler())
    renderApp(<OltInventory olts={[olt('olt-1', 'OLT Pusat', 1)]} />)
    await screen.findByText('SIMREG0001')
    expect(screen.queryByRole('alert', { name: 'Port PON dengan dugaan gangguan' })).toBeNull()
  })
})

