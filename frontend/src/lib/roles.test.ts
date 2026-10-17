import { describe, expect, it } from 'vitest'
import { roleSatisfies } from './auth'

// Regresi: pencocokan peran dulu persis, sehingga ADMIN/NOC tidak melihat menu setingkat VIEWER (Dashboard/Perangkat/Task).
describe('roleSatisfies (hierarki staf)', () => {
  it('peran lebih tinggi memenuhi syarat peran lebih rendah', () => {
    expect(roleSatisfies(['ADMIN'], ['VIEWER'])).toBe(true)
    expect(roleSatisfies(['NOC'], ['VIEWER'])).toBe(true)
    expect(roleSatisfies(['ADMIN'], ['NOC'])).toBe(true)
    expect(roleSatisfies(['VIEWER'], ['VIEWER'])).toBe(true)
  })
  it('peran lebih rendah tidak memenuhi syarat peran lebih tinggi', () => {
    expect(roleSatisfies(['VIEWER'], ['NOC'])).toBe(false)
    expect(roleSatisfies(['NOC'], ['ADMIN'])).toBe(false)
    expect(roleSatisfies(['VIEWER'], ['ADMIN', 'NOC'])).toBe(false)
    expect(roleSatisfies(['ADMIN'], ['SUPERADMIN'])).toBe(false)
  })
  it('daftar syarat berarti "salah satu" (yang terendah menentukan)', () => {
    expect(roleSatisfies(['NOC'], ['ADMIN', 'NOC'])).toBe(true)
  })
  it('SUPERADMIN selalu lolos; tanpa peran tidak pernah', () => {
    expect(roleSatisfies(['SUPERADMIN'], ['ADMIN'])).toBe(true)
    expect(roleSatisfies([], ['VIEWER'])).toBe(false)
  })
  it('ENDUSER di luar hierarki: hanya cocok persis dan tak mewarisi akses staf', () => {
    expect(roleSatisfies(['ENDUSER'], ['ENDUSER'])).toBe(true)
    expect(roleSatisfies(['ENDUSER'], ['VIEWER'])).toBe(false)
    expect(roleSatisfies(['ADMIN'], ['ENDUSER'])).toBe(false)
  })
})
