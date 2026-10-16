import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import { QueryCache, QueryClient, QueryClientProvider } from '@tanstack/react-query'
import './index.css'
import App from './App.tsx'
import { AuthProvider } from './lib/auth.tsx'
import { ThemeProvider } from './lib/theme.tsx'
import { WebSocketProvider } from './lib/ws.tsx'
import { ToastProvider } from './lib/toast.tsx'
import { ConfirmProvider } from './lib/confirm.tsx'
import { I18nProvider } from './lib/i18n.tsx'
import { installAutoLabels } from './lib/autolabel.ts'
import { reportQueryError } from './lib/queryErrors.ts'
import { QueryErrorToaster } from './components/QueryErrorToaster.tsx'

const queryClient = new QueryClient({
  // Galat pemuatan data tidak boleh senyap (daftar gagal dimuat ≠ daftar kosong).
  queryCache: new QueryCache({ onError: (err, query) => reportQueryError(err, query.queryKey) }),
  defaultOptions: {
    queries: {
      retry: 1,
      refetchOnWindowFocus: false,
    },
  },
})

installAutoLabels()

const rootEl = document.getElementById('root')!
// Beranda statis (SEO) disembunyikan di route non-beranda sebelum React mengambil alih; tampilkan kembali.
rootEl.style.visibility = ''
createRoot(rootEl).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <I18nProvider>
          <ToastProvider>
            <QueryErrorToaster />
            <ConfirmProvider>
              <BrowserRouter>
                <AuthProvider>
                  <WebSocketProvider>
                    <App />
                  </WebSocketProvider>
                </AuthProvider>
              </BrowserRouter>
            </ConfirmProvider>
          </ToastProvider>
        </I18nProvider>
      </ThemeProvider>
    </QueryClientProvider>
  </StrictMode>,
)
