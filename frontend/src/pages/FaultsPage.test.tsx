import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { FaultsPage } from './FaultsPage'
import { ConfirmProvider } from '../lib/confirm'
import { setAuthToken } from '../lib/api'
import { mockFetch, renderApp } from '../test/utils'

function login(roles: string[]) {
  setAuthToken('JWT')
  localStorage.setItem('acs_user', JSON.stringify({ id: 1, uuid: 'u', username: 'adm', roles, tenant_id: 7 }))
}

const FAULT = {
  id: 'f-1', device_id: 42, device_serial: 'ZTEGCD775EE7', tenant_id: 7, task_id: 9, rpc_method: 'SET_PARAMETER_VALUES',
  fault_code: '9005', fault_string: 'Invalid parameter name', occurrence_count: 3,
  first_seen_at: new Date(Date.now() - 3600_000).toISOString(), last_seen_at: new Date().toISOString(),
}

const ui = (
  <ConfirmProvider>
    <FaultsPage />
  </ConfirmProvider>
)

beforeEach(() => localStorage.clear())
afterEach(() => {
  setAuthToken(null)
  localStorage.clear()
})

describe('FaultsPage', () => {
  it('menampilkan serial (tautan ke perangkat), kode + artinya, RPC, dan jumlah kejadian', async () => {
    login(['VIEWER'])
    mockFetch((r) => {
      if (r.url.includes('/refs/ref_task_types')) return { json: [{ id: 3, code: 'SET_PARAMETER_VALUES', name: 'Set Parameter Values' }] }
      if (r.url.includes('/faults')) return { json: { data: [FAULT], total: 1 } }
      return undefined
    })
    renderApp(ui)
    const link = await screen.findByRole('link', { name: 'ZTEGCD775EE7' })
    expect(link.getAttribute('href')).toBe('/devices/42')
    expect(screen.getByText('9005')).toBeTruthy()
    expect(screen.getByText('Nama parameter tidak valid')).toBeTruthy()
    expect(screen.getByText('Invalid parameter name')).toBeTruthy()
    await waitFor(() => expect(screen.getAllByText('Set Parameter Values').some((el) => el.tagName === 'TD')).toBe(true))
    expect(screen.getByText('3×')).toBeTruthy()
  })

  it('VIEWER tidak melihat tombol tutup; kosong = pesan "Tidak ada fault aktif"', async () => {
    login(['VIEWER'])
    mockFetch((r) => (r.url.includes('/faults') ? { json: { data: [FAULT], total: 1 } } : { json: [] }))
    renderApp(ui)
    await screen.findByText('ZTEGCD775EE7')
    expect(screen.queryByRole('button', { name: /Tutup fault/ })).toBeNull()
  })

  it('ADMIN menutup fault: konfirmasi lalu DELETE /faults/:uuid', async () => {
    login(['ADMIN'])
    const calls = mockFetch((r) => {
      if (r.method === 'DELETE') return { status: 204 }
      if (r.url.includes('/faults')) return { json: { data: [FAULT], total: 1 } }
      return { json: [] }
    })
    renderApp(ui)
    await userEvent.click(await screen.findByRole('button', { name: 'Tutup fault ZTEGCD775EE7' }))
    await userEvent.click(await screen.findByRole('button', { name: 'Tutup fault' }))
    await waitFor(() => expect(calls.some((c) => c.method === 'DELETE' && c.url.endsWith('/faults/f-1'))).toBe(true))
  })

  it('daftar kosong menampilkan keadaan kosong', async () => {
    login(['ADMIN'])
    mockFetch((r) => (r.url.includes('/faults') ? { json: { data: [], total: 0 } } : { json: [] }))
    renderApp(ui)
    expect(await screen.findByText('Tidak ada fault aktif')).toBeTruthy()
  })
})
