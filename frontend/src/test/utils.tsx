import type { ReactElement, ReactNode } from 'react'
import { render } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { AuthProvider } from '../lib/auth'
import { I18nProvider } from '../lib/i18n'
import { ThemeProvider } from '../lib/theme'
import { ToastProvider } from '../lib/toast'

// Membungkus komponen dengan seluruh provider aplikasi (kecuali WebSocket/Confirm).
export function renderApp(ui: ReactElement, route = '/') {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const wrap = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={qc}>
      <ThemeProvider>
        <I18nProvider>
          <ToastProvider>
            <MemoryRouter initialEntries={[route]}>
              <AuthProvider>{children}</AuthProvider>
            </MemoryRouter>
          </ToastProvider>
        </I18nProvider>
      </ThemeProvider>
    </QueryClientProvider>
  )
  return render(ui, { wrapper: wrap })
}

type Handler = (req: { url: string; method: string; body: unknown }) => { status?: number; json?: unknown } | undefined

// Mengganti fetch global: handler dicocokkan berurutan; request tak tertangani -> 404 (membuat tes gagal jelas).
export function mockFetch(handler: Handler) {
  const calls: { url: string; method: string; body: unknown }[] = []
  const impl = async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input)
    const method = (init?.method ?? 'GET').toUpperCase()
    // FormData (multipart) diubah ke objek sederhana; berkas dicatat sebagai { file: nama, size, type }.
    const body =
      init?.body instanceof FormData
        ? Object.fromEntries([...init.body.entries()].map(([k, v]) => [k, typeof v === 'string' ? v : { file: v.name, size: v.size, type: v.type }]))
        : init?.body
          ? JSON.parse(String(init.body))
          : undefined
    calls.push({ url, method, body })
    const r = handler({ url, method, body }) ?? { status: 404, json: { message: `tidak ada mock untuk ${method} ${url}` } }
    return new Response(r.json === undefined ? null : JSON.stringify(r.json), {
      status: r.status ?? 200,
      headers: { 'Content-Type': 'application/json' },
    })
  }
  globalThis.fetch = impl as typeof fetch
  return calls
}
