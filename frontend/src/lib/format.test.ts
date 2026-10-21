import { describe, expect, it } from 'vitest'
import { formatRelativeTime } from './format'

const ago = (sec: number) => new Date(Date.now() - sec * 1000).toISOString()

// Satuan tidak ambigu: "d" dulu dibaca "hari" padahal berarti detik ("6d lalu" = 6 detik).
describe('formatRelativeTime', () => {
  it('satuan eksplisit: detik/menit/jam/hari', () => {
    expect(formatRelativeTime(ago(2))).toBe('baru saja')
    expect(formatRelativeTime(ago(6))).toBe('6 dtk lalu')
    expect(formatRelativeTime(ago(4 * 60))).toBe('4 mnt lalu')
    expect(formatRelativeTime(ago(2 * 3600))).toBe('2 jam lalu')
    expect(formatRelativeTime(ago(3 * 86400))).toBe('3 hari lalu')
  })
  it('kosong/tak valid → "-"', () => {
    expect(formatRelativeTime(null)).toBe('-')
    expect(formatRelativeTime('bukan tanggal')).toBe('-')
  })
})
