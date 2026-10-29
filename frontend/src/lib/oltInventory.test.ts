import { describe, expect, it } from 'vitest'
import { flaggedPorts, formatRx, isOltDataStale, onuStateBadge, onuStateLabel, portHint, portTone, stateSince, sumPorts } from './oltInventory'

describe('onuStateLabel / onuStateBadge', () => {
  it('memetakan keadaan yang dikenal', () => {
    expect(onuStateLabel('LOS')).toBe('LOS (sinyal hilang)')
    expect(onuStateLabel('DYING_GASP')).toBe('Dying gasp (daya hilang)')
    expect(onuStateBadge('ONLINE')).toBe('ONLINE')
    expect(onuStateBadge('LOS')).toBe('FAULTY')
    expect(onuStateBadge('DYING_GASP')).toBe('TIMEOUT')
  })
  it('kode asing diperlakukan tidak diketahui, bukan disamarkan sebagai online', () => {
    expect(onuStateLabel('MELAYANG')).toBe('Tidak diketahui')
    expect(onuStateBadge('MELAYANG')).toBe('DECOMMISSIONED')
    expect(onuStateBadge('')).toBe('DECOMMISSIONED')
  })
})

describe('isOltDataStale', () => {
  const now = Date.parse('2026-10-29T12:00:00Z')
  const ago = (min: number) => new Date(now - min * 60_000).toISOString()
  it('batas 30 menit, sama dengan backend', () => {
    expect(isOltDataStale(ago(29), now)).toBe(false)
    expect(isOltDataStale(ago(30), now)).toBe(false)
    expect(isOltDataStale(ago(31), now)).toBe(true)
  })
  it('belum pernah dipoll atau tak valid = basi', () => {
    expect(isOltDataStale(null, now)).toBe(true)
    expect(isOltDataStale(undefined, now)).toBe(true)
    expect(isOltDataStale('bukan tanggal', now)).toBe(true)
  })
})

describe('portTone', () => {
  const p = (over: Partial<Parameters<typeof portTone>[0]>) => ({ total: 4, los: 0, dying_gasp: 0, offline: 0, unknown: 0, ...over })
  it('LOS atau dying gasp = merah, mengalahkan offline', () => {
    expect(portTone(p({ los: 1, offline: 2 }))).toBe('bad')
    expect(portTone(p({ dying_gasp: 1 }))).toBe('bad')
  })
  it('offline atau tak diketahui = kuning; semua online = hijau; kosong = idle', () => {
    expect(portTone(p({ offline: 1 }))).toBe('warn')
    expect(portTone(p({ unknown: 1 }))).toBe('warn')
    expect(portTone(p({}))).toBe('ok')
    expect(portTone(p({ total: 0 }))).toBe('idle')
  })
})

describe('formatRx & sumPorts', () => {
  it('RX satu desimal dan tanda strip bila tak ada', () => {
    expect(formatRx(-21.54)).toBe('-21.5 dBm')
    expect(formatRx(null)).toBe('—')
    expect(formatRx(undefined)).toBe('—')
    expect(formatRx(NaN)).toBe('—')
  })
  it('menjumlah seluruh port; masalah = LOS + dying gasp', () => {
    expect(
      sumPorts([
        { pon_port: 'a', total: 3, online: 1, offline: 0, los: 1, dying_gasp: 1, unknown: 0, linked: 0, min_rx_dbm: null, avg_rx_dbm: null, last_seen_at: null },
        { pon_port: 'b', total: 2, online: 1, offline: 1, los: 0, dying_gasp: 0, unknown: 0, linked: 0, min_rx_dbm: null, avg_rx_dbm: null, last_seen_at: null },
      ]),
    ).toEqual({ total: 5, online: 2, problem: 2, offline: 1 })
    expect(sumPorts([])).toEqual({ total: 0, online: 0, problem: 0, offline: 0 })
  })
})

describe('stateSince', () => {
  const T = '2026-10-29T07:00:00Z'
  it('baris yang baru ditemukan (sejak == penemuan) bukan "sejak" yang teramati', () => {
    expect(stateSince({ first_seen_at: T, last_state_change_at: T })).toEqual({ observed: false, at: T })
    // selisih sub-detik karena pembulatan penyimpanan tetap dianggap sama
    expect(stateSince({ first_seen_at: T, last_state_change_at: '2026-10-29T07:00:00.500Z' }).observed).toBe(false)
  })
  it('perubahan yang pernah teramati memakai waktu perubahan', () => {
    expect(stateSince({ first_seen_at: T, last_state_change_at: '2026-10-29T07:05:00Z' })).toEqual({ observed: true, at: '2026-10-29T07:05:00Z' })
  })
  it('tanggal rusak tidak dianggap teramati', () => {
    expect(stateSince({ first_seen_at: 'x', last_state_change_at: T }).observed).toBe(false)
  })
})

describe('portHint / flaggedPorts', () => {
  const port = (total: number, los: number, dying_gasp: number, pon_port = 'p') => ({ pon_port, total, los, dying_gasp, online: 0, offline: 0, unknown: 0, linked: 0, min_rx_dbm: null, avg_rx_dbm: null, last_seen_at: null })
  it('LOS massal pada satu port = dugaan feeder; dying gasp massal = dugaan listrik', () => {
    expect(portHint(port(8, 7, 0))?.kind).toBe('FEEDER')
    expect(portHint(port(8, 7, 0))?.text).toContain('7 dari 8 ONU LOS')
    expect(portHint(port(6, 0, 4))?.kind).toBe('POWER')
  })
  it('ambang konservatif: kurang dari 3 ONU atau kurang dari setengah port tidak ditandai', () => {
    expect(portHint(port(2, 2, 0))).toBeNull() // dua ONU bisa kebetulan
    expect(portHint(port(10, 4, 0))).toBeNull() // 40% < 50%
    expect(portHint(port(10, 5, 0))?.kind).toBe('FEEDER') // tepat 50%
    expect(portHint(port(3, 3, 0))?.kind).toBe('FEEDER') // tepat 3 ONU
    expect(portHint(port(0, 0, 0))).toBeNull()
    expect(portHint(port(10, 1, 1))).toBeNull()
  })
  it('LOS dan dying gasp sama-sama memenuhi ambang: yang lebih banyak menang; seri = feeder', () => {
    expect(portHint(port(10, 3, 6))?.kind).toBe('POWER')
    expect(portHint(port(10, 5, 5))?.kind).toBe('FEEDER')
  })
  it('flaggedPorts: hanya yang ditandai, terparah dulu, stabil menurut nama', () => {
    const f = flaggedPorts([port(10, 5, 0, 'b'), port(10, 0, 0, 'ok'), port(10, 9, 0, 'a'), port(10, 5, 0, 'c')])
    expect(f.map((x) => x.port)).toEqual(['a', 'b', 'c'])
  })
})
