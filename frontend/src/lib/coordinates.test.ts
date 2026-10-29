import { describe, expect, it } from 'vitest'
import { formatCoordinates, parseCoordinates, validCoordinates } from './coordinates'

describe('parseCoordinates', () => {
  it('format yang lazim disalin dari Google Maps/ponsel', () => {
    expect(parseCoordinates('-6.2088, 106.8456')).toEqual({ lat: -6.2088, lon: 106.8456 })
    expect(parseCoordinates('-6.2088,106.8456')).toEqual({ lat: -6.2088, lon: 106.8456 })
    expect(parseCoordinates('  -6.2088   106.8456  ')).toEqual({ lat: -6.2088, lon: 106.8456 })
    expect(parseCoordinates('3.5952; 98.6722')).toEqual({ lat: 3.5952, lon: 98.6722 })
  })

  it('minus Unicode, tanda derajat, dan bilangan bulat', () => {
    expect(parseCoordinates('−6.2088, 106.8456')).toEqual({ lat: -6.2088, lon: 106.8456 })
    expect(parseCoordinates('-6.2088°, 106.8456°')).toEqual({ lat: -6.2088, lon: 106.8456 })
    expect(parseCoordinates('-6, 107')).toEqual({ lat: -6, lon: 107 })
  })

  it('koma desimal gaya Indonesia hanya bila dipisah titik koma atau spasi', () => {
    expect(parseCoordinates('-6,2088; 106,8456')).toEqual({ lat: -6.2088, lon: 106.8456 })
    expect(parseCoordinates('-6,2088 106,8456')).toEqual({ lat: -6.2088, lon: 106.8456 })
  })

  it('ambigu ditolak, bukan ditebak', () => {
    expect(parseCoordinates('-6,2088,106,8456')).toBeNull()
    expect(parseCoordinates('-6,2088, 106,8456')).toBeNull()
    expect(parseCoordinates('1,2,3')).toBeNull()
  })

  it('di luar rentang atau bukan angka ditolak', () => {
    expect(parseCoordinates('91, 10')).toBeNull()
    expect(parseCoordinates('-91, 10')).toBeNull()
    expect(parseCoordinates('10, 181')).toBeNull()
    expect(parseCoordinates('10, -181')).toBeNull()
    expect(parseCoordinates('abc, def')).toBeNull()
    expect(parseCoordinates('1e3, 2')).toBeNull() // tanpa eksponen
    expect(parseCoordinates('0x10, 2')).toBeNull()
    expect(parseCoordinates('-6.2088')).toBeNull() // satu angka
    expect(parseCoordinates('1 2 3')).toBeNull()
    expect(parseCoordinates('')).toBeNull()
    expect(parseCoordinates('   ')).toBeNull()
    expect(parseCoordinates('NaN, 1')).toBeNull()
    expect(parseCoordinates('Infinity, 1')).toBeNull()
  })

  it('batas persis diterima; nol-nol sah', () => {
    expect(parseCoordinates('90, 180')).toEqual({ lat: 90, lon: 180 })
    expect(parseCoordinates('-90, -180')).toEqual({ lat: -90, lon: -180 })
    expect(parseCoordinates('0, 0')).toEqual({ lat: 0, lon: 0 })
  })
})

describe('validCoordinates & formatCoordinates', () => {
  it('memeriksa rentang dan angka hingga', () => {
    expect(validCoordinates(-6.2, 106.8)).toBe(true)
    expect(validCoordinates(91, 0)).toBe(false)
    expect(validCoordinates(0, 181)).toBe(false)
    expect(validCoordinates(NaN, 0)).toBe(false)
    expect(validCoordinates(0, Infinity)).toBe(false)
  })
  it('lima desimal', () => {
    expect(formatCoordinates(-6.2088, 106.8456)).toBe('-6.20880, 106.84560')
    expect(formatCoordinates(1.123456789, -2.987654321)).toBe('1.12346, -2.98765')
  })
})
