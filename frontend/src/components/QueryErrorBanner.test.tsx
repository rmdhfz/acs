import { QueryClient, QueryClientProvider, useQuery } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError } from '../lib/api'
import { QueryErrorBanner } from './QueryErrorBanner'

function Failing({ fn }: { fn: () => Promise<string> }) {
  const q = useQuery({ queryKey: ['uji-banner'], queryFn: fn, retry: false })
  return <p>{q.data ?? 'kosong'}</p>
}

function setup(fn: () => Promise<string>) {
  const qc = new QueryClient()
  render(
    <QueryClientProvider client={qc}>
      <QueryErrorBanner />
      <Failing fn={fn} />
    </QueryClientProvider>,
  )
}

afterEach(() => vi.restoreAllMocks())

describe('QueryErrorBanner — status galat inline', () => {
  it('menampilkan peringatan tetap saat data halaman gagal dimuat, dan pulih setelah "Coba lagi"', async () => {
    let calls = 0
    setup(async () => {
      calls++
      if (calls === 1) throw new ApiError(503, 'layanan tak tersedia')
      return 'data-ok'
    })
    const alert = await screen.findByRole('alert')
    expect(alert.textContent).toContain('gagal dimuat')
    expect(alert.textContent).toContain('Server bermasalah (HTTP 503)')
    await userEvent.setup().click(screen.getByRole('button', { name: /coba lagi/i }))
    await waitFor(() => expect(screen.getByText('data-ok')).toBeTruthy())
    await waitFor(() => expect(screen.queryByRole('alert')).toBeNull())
  })

  it('tidak muncul untuk 401 (ditangani AuthProvider) maupun saat tak ada galat', async () => {
    setup(async () => {
      throw new ApiError(401, 'sesi berakhir')
    })
    await waitFor(() => expect(screen.getByText('kosong')).toBeTruthy())
    expect(screen.queryByRole('alert')).toBeNull()
  })
})
