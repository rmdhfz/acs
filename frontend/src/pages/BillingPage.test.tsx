import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { BillingPage } from './BillingPage'
import { setAuthToken } from '../lib/api'
import { mockFetch, renderApp } from '../test/utils'

const PLANS = [
  { code: 'STARTER', name: 'Starter', description: 'Untuk ISP kecil', max_devices: 100, price_idr: 199000, period_days: 30, is_active: true, sort_order: 1 },
  { code: 'PRO', name: 'Pro', description: '', max_devices: null, price_idr: 999000, period_days: 30, is_active: true, sort_order: 2 },
]

function login(roles: string[], extra: Record<string, unknown> = {}) {
  setAuthToken('JWT') // token dibaca dari variabel modul, bukan langsung dari localStorage
  localStorage.setItem('acs_user', JSON.stringify({ id: 1, uuid: 'u', username: 'adm', roles, tenant_id: roles.includes('SUPERADMIN') ? null : 7, ...extra }))
}

const sub = (over: Record<string, unknown> = {}) => ({
  enabled: true, status: 'ACTIVE', plan: PLANS[0], expires_at: '2026-12-01T00:00:00Z', days_left: 44, max_devices: 100, pending_invoice: null, ...over,
})

const inv = (over: Record<string, unknown> = {}) => ({
  id: 'inv-1', number: 'INV-2026-0001', tenant_id: 7, plan_code: 'PRO', plan_name: 'Pro', amount_idr: 999000, status: 'PENDING',
  payment_url: 'https://checkout.xendit.co/web/abc', period_days: 30, created_at: '2026-10-18T00:00:00Z', expires_at: '2026-10-19T00:00:00Z', paid_at: null, ...over,
})

const assign = vi.fn()
const realLocation = window.location

beforeEach(() => {
  localStorage.clear()
  assign.mockReset()
  Object.defineProperty(window, 'location', { configurable: true, value: { ...realLocation, assign } })
})
afterEach(() => {
  setAuthToken(null)
  Object.defineProperty(window, 'location', { configurable: true, value: realLocation })
  localStorage.clear()
})

describe('BillingPage', () => {
  it('billing dimatikan: info jelas dan tombol beli nonaktif', async () => {
    login(['ADMIN'])
    mockFetch(({ url }) => {
      if (url.includes('/billing/subscription')) return { json: sub({ enabled: false, status: 'NONE', plan: null, expires_at: null, days_left: null, max_devices: null }) }
      if (url.includes('/billing/plans')) return { json: { data: PLANS } }
      if (url.includes('/billing/invoices')) return { json: { data: [], meta: { total: 0, page: 1, limit: 10 } } }
    })
    renderApp(<BillingPage />)
    expect(await screen.findByText(/pembayaran online belum diaktifkan/i)).toBeTruthy()
    const buy = await screen.findAllByRole('button', { name: /pilih paket/i })
    expect(buy.every((b) => (b as HTMLButtonElement).disabled)).toBe(true)
  })

  it('checkout hanya mengirim plan_code (tanpa nominal) dan mengarahkan ke halaman bayar https', async () => {
    login(['ADMIN'])
    const calls = mockFetch(({ url, method }) => {
      if (url.includes('/billing/subscription')) return { json: sub() }
      if (url.includes('/billing/plans')) return { json: { data: PLANS } }
      if (url.includes('/billing/invoices')) return { json: { data: [], meta: { total: 0, page: 1, limit: 10 } } }
      if (url.endsWith('/billing/checkout') && method === 'POST') return { status: 201, json: { invoice: inv(), payment_url: 'https://checkout.xendit.co/web/abc' } }
    })
    renderApp(<BillingPage />)
    const u = userEvent.setup()
    const pro = (await screen.findAllByRole('button', { name: /pilih paket/i }))[0]
    await waitFor(() => expect((pro as HTMLButtonElement).disabled).toBe(false))
    await u.click(pro)
    await waitFor(() => expect(assign).toHaveBeenCalledWith('https://checkout.xendit.co/web/abc'))
    const post = calls.find((c) => c.method === 'POST' && c.url.endsWith('/billing/checkout'))!
    expect(Object.keys(post.body as object)).toEqual(['plan_code'])
  })

  it('tautan bayar yang bukan https TIDAK dibuka', async () => {
    login(['ADMIN'])
    mockFetch(({ url, method }) => {
      if (url.includes('/billing/subscription')) return { json: sub() }
      if (url.includes('/billing/plans')) return { json: { data: PLANS } }
      if (url.includes('/billing/invoices')) return { json: { data: [], meta: { total: 0, page: 1, limit: 10 } } }
      if (url.endsWith('/billing/checkout') && method === 'POST') return { status: 201, json: { invoice: inv({ payment_url: 'javascript:alert(1)' }), payment_url: 'javascript:alert(1)' } }
    })
    renderApp(<BillingPage />)
    const u = userEvent.setup()
    const btn = (await screen.findAllByRole('button', { name: /pilih paket/i }))[0]
    await waitFor(() => expect((btn as HTMLButtonElement).disabled).toBe(false))
    await u.click(btn)
    await waitFor(() => expect(screen.getByText(/tautan pembayaran tidak tersedia/i)).toBeTruthy())
    expect(assign).not.toHaveBeenCalled()
  })

  it('invoice tertunda: tombol bayar (https) dan "Periksa status" memanggil sync', async () => {
    login(['ADMIN'])
    const pending = inv()
    const calls = mockFetch(({ url, method }) => {
      if (url.includes('/billing/subscription')) return { json: sub({ pending_invoice: pending }) }
      if (url.includes('/billing/plans')) return { json: { data: PLANS } }
      if (url.includes('/billing/invoices')) return { json: { data: [pending], meta: { total: 1, page: 1, limit: 10 } } }
      if (url.endsWith('/billing/invoices/inv-1/sync') && method === 'POST') return { json: inv({ status: 'PAID', paid_at: '2026-10-18T01:00:00Z', payment_url: null }) }
    })
    renderApp(<BillingPage />)
    const u = userEvent.setup()
    expect(await screen.findByText(/INV-2026-0001 \(Pro/)).toBeTruthy()
    const link = screen.getAllByRole('link', { name: /bayar/i })[0] as HTMLAnchorElement
    expect(link.href).toBe('https://checkout.xendit.co/web/abc')
    expect(link.rel).toContain('noopener')
    await u.click(screen.getAllByRole('button', { name: /periksa status/i })[0])
    await waitFor(() => expect(calls.some((c) => c.method === 'POST' && c.url.endsWith('/invoices/inv-1/sync'))).toBe(true))
  })

  it('SUPERADMIN harus memilih tenant dulu; form paket baru memvalidasi kode & harga', async () => {
    login(['SUPERADMIN'])
    const calls = mockFetch(({ url, method }) => {
      if (url.includes('/tenants')) return { json: { data: [{ id: 7, code: 'NUSA', name: 'Nusa Net' }], total: 1 } }
      if (url.includes('/billing/plans') && method === 'GET') return { json: { data: PLANS } }
    })
    renderApp(<BillingPage />)
    const u = userEvent.setup()
    expect((await screen.findAllByText(/pilih tenant/i)).length).toBeGreaterThan(0)
    expect(calls.some((c) => c.url.includes('/billing/subscription'))).toBe(false)

    await u.click(await screen.findByRole('button', { name: /paket baru/i }))
    await u.type(screen.getByLabelText('Kode'), '1bad')
    await u.type(screen.getByLabelText('Nama'), 'Gold')
    await u.click(screen.getByRole('button', { name: /simpan/i }))
    expect((await screen.findByRole('alert')).textContent).toMatch(/kode paket/i)
    expect(calls.some((c) => c.method === 'POST' && c.url.endsWith('/billing/plans'))).toBe(false)
  })
})
