import { afterEach, describe, expect, it, vi } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { useCurrentTenant } from './hooks'

// Regresi: GET /tenants/current menjawab 204 untuk SUPERADMIN (tanpa tenant). Dulu queryFn mengembalikan
// undefined -> TanStack Query v5 menganggapnya galat -> toast "Gagal memuat data" palsu di setiap halaman.
function wrapper(client: QueryClient) {
  return ({ children }: { children: ReactNode }) => <QueryClientProvider client={client}>{children}</QueryClientProvider>
}

afterEach(() => vi.unstubAllGlobals())

describe('useCurrentTenant', () => {
  it('204 (superadmin tanpa tenant) = sukses dengan data null, bukan galat', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response(null, { status: 204 })))
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const { result } = renderHook(() => useCurrentTenant(), { wrapper: wrapper(client) })
    await waitFor(() => expect(result.current.isFetching).toBe(false))
    expect(result.current.isError).toBe(false)
    expect(result.current.isSuccess).toBe(true)
    expect(result.current.data).toBeNull()
  })

  it('200 mengembalikan tenant apa adanya', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ id: 3, code: 'T3', name: 'Tenant 3' }), { status: 200 })))
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const { result } = renderHook(() => useCurrentTenant(), { wrapper: wrapper(client) })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(result.current.data).toMatchObject({ id: 3, code: 'T3' })
  })
})
