import { describe, expect, it } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { UserDevicesModal } from './UserDevicesModal'
import { mockFetch, renderApp } from '../test/utils'
import type { User } from '../lib/types'

const cust: User = {
  id: 10, user_uuid: 'u', tenant_id: 7, username: 'budi', email: 'b@x.id', full_name: 'Budi', is_active: true,
  last_login_at: null, roles: ['ENDUSER'], created_at: '', updated_at: '',
}

describe('UserDevicesModal', () => {
  it('kosong -> cari -> tautkan (hanya perangkat se-tenant) -> lepas', async () => {
    let linked = [{ id: 5, serial_number: 'SUDAH-5', model: 'F670L', status: 'ONLINE' }]
    const calls = mockFetch(({ url, method }) => {
      if (url.endsWith('/users/10/devices') && method === 'GET') return { json: { data: linked, total: linked.length } }
      if (url.includes('/devices?') && method === 'GET') {
        return {
          json: {
            total: 3,
            data: [
              { id: 5, serial_number: 'SUDAH-5', tenant_id: 7, product_class: 'F670L' },
              { id: 6, serial_number: 'BARU-6', tenant_id: 7, product_class: 'F670L' },
              { id: 7, serial_number: 'LAIN-7', tenant_id: 8, product_class: 'HG8245' },
            ],
          },
        }
      }
      if (url.endsWith('/users/10/devices/6') && method === 'PUT') {
        linked = [...linked, { id: 6, serial_number: 'BARU-6', model: 'F670L', status: 'OFFLINE' }]
        return { status: 204 }
      }
      if (url.endsWith('/users/10/devices/5') && method === 'DELETE') {
        linked = linked.filter((d) => d.id !== 5)
        return { status: 204 }
      }
    })
    renderApp(<UserDevicesModal user={cust} onClose={() => {}} />)
    const u = userEvent.setup()

    expect(await screen.findByText('SUDAH-5')).toBeTruthy()

    // pencarian < 3 karakter tidak memanggil API
    await u.type(screen.getByLabelText(/cari perangkat/i), 'ba')
    expect(calls.some((c) => c.url.includes('/devices?'))).toBe(false)
    await u.type(screen.getByLabelText(/cari perangkat/i), 'ru')

    // hasil: yang sudah tertaut & beda tenant tidak bisa ditautkan
    await screen.findAllByRole('button', { name: /^tautkan$/i })
    const btn6 = screen.getAllByRole('button', { name: /^tautkan$/i }).filter((b) => !(b as HTMLButtonElement).disabled)[0]
    expect(btn6).toBeTruthy()
    expect((screen.getByRole('button', { name: /tertaut$/i }) as HTMLButtonElement).disabled).toBe(true)
    expect(screen.getByText(/beda tenant/i)).toBeTruthy()
    const buttons = screen.getAllByRole('button', { name: /tautkan|tertaut/i })
    expect(buttons.filter((b) => !(b as HTMLButtonElement).disabled)).toHaveLength(1)

    await u.click(btn6)
    await waitFor(() => expect(calls.some((c) => c.method === 'PUT' && c.url.endsWith('/users/10/devices/6'))).toBe(true))
    expect((await screen.findAllByText('BARU-6')).length).toBeGreaterThan(0)

    // lepas
    await u.click(screen.getByRole('button', { name: /lepas SUDAH-5/i }))
    await waitFor(() => expect(calls.some((c) => c.method === 'DELETE' && c.url.endsWith('/users/10/devices/5'))).toBe(true))
  })

  it('menampilkan keadaan kosong yang jelas', async () => {
    mockFetch(({ url }) => (url.endsWith('/users/10/devices') ? { json: { data: [], total: 0 } } : undefined))
    renderApp(<UserDevicesModal user={cust} onClose={() => {}} />)
    expect(await screen.findByText(/belum ada perangkat tertaut/i)).toBeTruthy()
  })
})
