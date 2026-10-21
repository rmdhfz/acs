import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AssignVendorModal } from './AssignVendorModal'
import { setAuthToken } from '../lib/api'
import { mockFetch, renderApp } from '../test/utils'
import type { Device } from '../lib/types'

function login(roles: string[]) {
  setAuthToken('JWT')
  localStorage.setItem('acs_user', JSON.stringify({ id: 1, uuid: 'u', username: 'u', roles, tenant_id: roles.includes('SUPERADMIN') ? null : 7 }))
}

const device = { id: 20088, serial_number: 'OPTIF600C9E0', oui: '9028f6', product_class: 'RXP4-AC1200', vendor_id: null } as Device

function handler(extra?: (r: { url: string; method: string }) => { status?: number; json?: unknown } | undefined) {
  return (r: { url: string; method: string; body: unknown }) => {
    if (r.url.includes('/vendors') && r.method === 'GET') {
      return { json: { data: [{ id: 1, code: 'ZTE', name: 'ZTE Corporation' }, { id: 9, code: 'GENERIC', name: 'Standar TR-069 (Generik)' }], total: 2 } }
    }
    return extra?.(r)
  }
}

beforeEach(() => localStorage.clear())
afterEach(() => {
  setAuthToken(null)
  localStorage.clear()
})

describe('AssignVendorModal', () => {
  it('SUPERADMIN: buat vendor baru + daftarkan OUI + tetapkan ke perangkat (urutan benar, OUI huruf besar)', async () => {
    login(['SUPERADMIN'])
    const calls = mockFetch(
      handler((r) => {
        if (r.url.endsWith('/vendors') && r.method === 'POST') return { status: 201, json: { id: 77, code: 'OPTICTIMES', name: 'Optictimes' } }
        if (r.url.endsWith('/vendors/77/ouis') && r.method === 'POST') return { status: 201, json: { id: 1, vendor_id: 77, oui: '9028F6' } }
        if (r.url.endsWith('/devices/20088') && r.method === 'PATCH') return { json: { ...device, vendor_id: 77 } }
        return undefined
      }),
    )
    renderApp(<AssignVendorModal device={device} onClose={() => {}} />)
    const u = userEvent.setup()
    // GENERIC tidak ditawarkan sebagai pilihan vendor
    await screen.findByRole('option', { name: 'ZTE Corporation' })
    expect(screen.queryByRole('option', { name: /standar tr-069/i })).toBeNull()

    await u.selectOptions(screen.getByLabelText(/^vendor$/i), 'new')
    await u.type(screen.getByLabelText(/nama vendor/i), 'Optictimes')
    await u.type(screen.getByLabelText(/^kode$/i), 'optictimes')
    await u.click(screen.getByRole('button', { name: /^tetapkan vendor$/i }))

    await waitFor(() => expect(calls.some((c) => c.method === 'PATCH' && c.url.endsWith('/devices/20088'))).toBe(true))
    const writes = calls.filter((c) => c.method !== 'GET').map((c) => `${c.method} ${c.url.replace(/^.*\/api\/v1/, '')}`)
    expect(writes).toEqual(['POST /vendors', 'POST /vendors/77/ouis', 'PATCH /devices/20088'])
    expect(calls.find((c) => c.method === 'POST' && c.url.endsWith('/vendors'))?.body).toEqual({ code: 'OPTICTIMES', name: 'Optictimes' })
    expect((calls.find((c) => c.url.endsWith('/vendors/77/ouis'))?.body as { oui: string } | undefined)?.oui).toBe('9028F6')
    expect(calls.find((c) => c.method === 'PATCH')?.body).toEqual({ vendor_id: 77 })
  })

  it('SUPERADMIN: kode vendor tak valid ditolak tanpa memanggil API tulis', async () => {
    login(['SUPERADMIN'])
    const calls = mockFetch(handler())
    renderApp(<AssignVendorModal device={device} onClose={() => {}} />)
    const u = userEvent.setup()
    await screen.findByRole('option', { name: 'ZTE Corporation' })
    await u.selectOptions(screen.getByLabelText(/^vendor$/i), 'new')
    await u.type(screen.getByLabelText(/nama vendor/i), 'X')
    await u.type(screen.getByLabelText(/^kode$/i), 'a b')
    await u.click(screen.getByRole('button', { name: /^tetapkan vendor$/i }))
    expect((await screen.findByRole('alert')).textContent).toMatch(/kode vendor/i)
    expect(calls.some((c) => c.method !== 'GET')).toBe(false)
  })

  it('ADMIN: hanya memilih vendor yang sudah ada; tidak ada opsi vendor baru/OUI; hanya perangkat ini yang di-PATCH', async () => {
    login(['ADMIN'])
    const calls = mockFetch(handler((r) => (r.url.endsWith('/devices/20088') && r.method === 'PATCH' ? { json: { ...device, vendor_id: 1 } } : undefined)))
    renderApp(<AssignVendorModal device={device} onClose={() => {}} />)
    const u = userEvent.setup()
    await screen.findByRole('option', { name: 'ZTE Corporation' })
    expect(screen.queryByRole('option', { name: /buat vendor baru/i })).toBeNull()
    expect(screen.queryByRole('checkbox')).toBeNull()
    expect(screen.getByText(/SUPERADMIN/)).toBeTruthy()

    await u.click(screen.getByRole('button', { name: /^tetapkan vendor$/i }))
    expect((await screen.findByRole('alert')).textContent).toMatch(/pilih vendor/i)

    await u.selectOptions(screen.getByLabelText(/^vendor$/i), '1')
    await u.click(screen.getByRole('button', { name: /^tetapkan vendor$/i }))
    await waitFor(() => expect(calls.some((c) => c.method === 'PATCH')).toBe(true))
    expect(calls.filter((c) => c.method !== 'GET').map((c) => c.method)).toEqual(['PATCH'])
    expect(calls.find((c) => c.method === 'PATCH')?.body).toEqual({ vendor_id: 1 })
  })
})
