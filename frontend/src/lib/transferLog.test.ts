import { describe, expect, it } from 'vitest'
import { moduleKey, recordTransfer, transferStatus } from './transferLog'
import { setAuthToken } from './api'
import { mockFetch } from '../test/utils'

describe('moduleKey', () => {
  it('menghasilkan kunci yang cocok pola server', () => {
    for (const name of ['Perangkat', 'vendor-oui', 'Mapping Parameter', 'Catatan Audit (30 hari)', '###', '9 modul', 'é', '']) {
      expect(moduleKey(name)).toMatch(/^[a-z][a-z0-9_]{1,47}$/)
    }
    expect(moduleKey('vendor-oui')).toBe('vendor_oui')
    expect(moduleKey('Mapping Parameter')).toBe('mapping_parameter')
    expect(moduleKey('x'.repeat(100)).length).toBeLessThanOrEqual(48)
  })
})

describe('transferStatus', () => {
  it('menurunkan status dari hitungan', () => {
    expect(transferStatus(5, 0)).toBe('COMPLETED')
    expect(transferStatus(3, 2)).toBe('PARTIAL')
    expect(transferStatus(0, 4)).toBe('FAILED')
  })
})

describe('recordTransfer', () => {
  it('mengirim hanya ringkasan (tanpa isi baris), hitungan dijepit wajar, kontrol dibuang', async () => {
    setAuthToken('JWT')
    const calls = mockFetch(({ url, method }) => (url.endsWith('/data-transfers') && method === 'POST' ? { status: 201, json: {} } : undefined))
    recordTransfer({ kind: 'IMPORT', module: 'Perangkat', fileName: 'a.csv', total: 10, success: 12, failed: 9, detail: 'baris\n1 gagal\u0000' })
    await new Promise((r) => setTimeout(r, 20))
    expect(calls).toHaveLength(1)
    const b = calls[0].body as Record<string, unknown>
    expect(b).toMatchObject({ kind: 'IMPORT', module: 'perangkat', status: 'COMPLETED', total_rows: 10, success_rows: 10, failed_rows: 0, format: 'csv' })
    expect(b.detail).toBe('baris 1 gagal')
    expect(Object.keys(b).sort()).toEqual(['detail', 'failed_rows', 'file_name', 'format', 'kind', 'module', 'status', 'success_rows', 'total_rows'])
    setAuthToken(null)
  })

  it('kegagalan API tidak melempar (tidak mengganggu alur impor/ekspor)', async () => {
    setAuthToken('JWT')
    mockFetch(() => ({ status: 500, json: { message: 'boom' } }))
    expect(() => recordTransfer({ kind: 'EXPORT', module: 'tasks', total: 1, success: 1, failed: 0 })).not.toThrow()
    await new Promise((r) => setTimeout(r, 20))
    setAuthToken(null)
  })
})
