import { describe, expect, it } from 'vitest'
import { opticalLevel } from './optical'

describe('opticalLevel', () => {
  it('normal / waspada / kritis menurut ambang GPON', () => {
    expect(opticalLevel(-22)).toBe('ok')
    expect(opticalLevel(-25)).toBe('ok') // tepat di batas masih normal
    expect(opticalLevel(-25.53)).toBe('warn')
    expect(opticalLevel(-26.99)).toBe('warn')
    expect(opticalLevel(-27)).toBe('warn') // tepat di batas kritis belum kritis
    expect(opticalLevel(-27.45)).toBe('bad')
    expect(opticalLevel(-7.9)).toBe('bad') // overload
    expect(opticalLevel(-8)).toBe('ok')
  })

  it('belum terbaca → null (bukan 0 dBm)', () => {
    expect(opticalLevel(null)).toBeNull()
    expect(opticalLevel(undefined)).toBeNull()
    expect(opticalLevel(Number.NaN)).toBeNull()
  })
})
