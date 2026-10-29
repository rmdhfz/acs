import { describe, expect, it } from 'vitest'
import { formatDurationSeconds, formatRelativeTime } from './format'

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

describe('formatDurationSeconds', () => {
  it('dua satuan terbesar, seragam dengan teks diagnosis backend', () => {
    expect(formatDurationSeconds(0)).toBe('0 detik')
    expect(formatDurationSeconds(45)).toBe('45 detik')
    expect(formatDurationSeconds(59)).toBe('59 detik')
    expect(formatDurationSeconds(60)).toBe('1 menit')
    expect(formatDurationSeconds(3599)).toBe('59 menit')
    expect(formatDurationSeconds(3600)).toBe('1 jam')
    expect(formatDurationSeconds(200 * 60)).toBe('3 jam 20 menit')
    expect(formatDurationSeconds(47 * 3600)).toBe('47 jam')
    expect(formatDurationSeconds(48 * 3600)).toBe('2 hari')
    expect(formatDurationSeconds(52 * 3600)).toBe('2 hari 4 jam')
    expect(formatDurationSeconds(30 * 3600)).toBe('30 jam')
  })
  it('kosong, negatif, dan tak hingga tidak merusak tampilan', () => {
    expect(formatDurationSeconds(null)).toBe('-')
    expect(formatDurationSeconds(undefined)).toBe('-')
    expect(formatDurationSeconds(NaN)).toBe('-')
    expect(formatDurationSeconds(Infinity)).toBe('-')
    expect(formatDurationSeconds(-30)).toBe('0 detik')
  })
})
