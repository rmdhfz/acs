import { describe, expect, it } from 'vitest'
import { bboxParam, clusterColor, countStatuses, mapNotice, statusStyle } from './deviceMap'

describe('statusStyle', () => {
  it('memberi warna & label untuk status yang dikenal', () => {
    expect(statusStyle('ONLINE')).toEqual({ color: '#10b981', label: 'Online' })
    expect(statusStyle('OFFLINE').color).toBe('#ef4444')
    expect(statusStyle('UNREGISTERED').label).toBe('Belum terdaftar')
  })
  it('status tak dikenal abu-abu dengan kodenya, bukan disamarkan sebagai online', () => {
    expect(statusStyle('DECOMMISSIONED')).toEqual({ color: '#64748b', label: 'DECOMMISSIONED' })
    expect(statusStyle('')).toEqual({ color: '#64748b', label: 'Tidak diketahui' })
  })
})

describe('countStatuses & clusterColor', () => {
  const pts = (...s: string[]) => s.map((status) => ({ status }))
  it('menghitung per kelompok', () => {
    expect(countStatuses(pts('ONLINE', 'ONLINE', 'OFFLINE', 'UNREGISTERED', ''))).toEqual({ online: 2, offline: 1, other: 2 })
    expect(countStatuses([])).toEqual({ online: 0, offline: 0, other: 0 })
  })
  it('klaster mengikuti yang terparah', () => {
    expect(clusterColor(countStatuses(pts('ONLINE', 'ONLINE')))).toBe('#10b981')
    expect(clusterColor(countStatuses(pts('ONLINE', 'UNREGISTERED')))).toBe('#f59e0b')
    // satu offline di antara ratusan online tetap menandai klaster merah
    expect(clusterColor({ online: 400, offline: 1, other: 3 })).toBe('#ef4444')
  })
})

describe('mapNotice', () => {
  it('diam bila semua tergambar dan tidak dipotong', () => {
    expect(mapNotice({ total: 5, located: 5, truncated: false })).toBeNull()
    expect(mapNotice(undefined)).toBeNull()
  })
  it('menyebut perangkat tanpa koordinat dengan angka yang benar', () => {
    expect(mapNotice({ total: 10, located: 4, truncated: false })).toBe('6 dari 10 perangkat belum punya koordinat dan tidak tampil di peta.')
  })
  it('menyebut pemotongan, dan keduanya digabung', () => {
    expect(mapNotice({ total: 9000, located: 9000, truncated: true })).toContain('dipotong pada 5000 titik')
    const both = mapNotice({ total: 9000, located: 8000, truncated: true })!
    expect(both).toContain('1000 dari 9000')
    expect(both).toContain('dipotong')
  })
})

describe('bboxParam', () => {
  it('format barat,selatan,timur,utara dibulatkan 5 desimal', () => {
    expect(bboxParam({ west: 106.123456789, south: -6.9, east: 107.1, north: -6.1 })).toBe('106.12346,-6.9,107.1,-6.1')
  })
  it('seluruh dunia (>= 360 derajat) = tanpa bbox', () => {
    expect(bboxParam({ west: -180, south: -90, east: 180, north: 90 })).toBeUndefined()
    expect(bboxParam({ west: -400, south: -60, east: 400, north: 60 })).toBeUndefined()
  })
  it('bujur di luar rentang dibungkus; barat > timur = melintasi bujur 180', () => {
    // digeser satu salinan dunia ke timur: 190..200 sebenarnya -170..-160
    expect(bboxParam({ west: 190, south: 0, east: 200, north: 10 })).toBe('-170,0,-160,10')
    // melintasi garis tanggal: 170..190 -> barat 170, timur -170
    expect(bboxParam({ west: 170, south: -20, east: 190, north: -10 })).toBe('170,-20,-170,-10')
  })
  it('lintang dijepit', () => {
    expect(bboxParam({ west: 0, south: -100, east: 10, north: 95 })).toBe('0,-90,10,90')
  })
  it('batas tak hingga/NaN ditolak (tanpa bbox)', () => {
    expect(bboxParam({ west: NaN, south: 0, east: 1, north: 1 })).toBeUndefined()
    expect(bboxParam({ west: 0, south: 0, east: Infinity, north: 1 })).toBeUndefined()
  })
})
