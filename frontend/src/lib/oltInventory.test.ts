import { describe, expect, it } from 'vitest'
import { formatRx, isOltDataStale, onuStateBadge, onuStateLabel, portTone, sumPorts } from './oltInventory'

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
    ).toEqual({ total: 5, online: 2, problem: 2 })
    expect(sumPorts([])).toEqual({ total: 0, online: 0, problem: 0 })
  })
})
