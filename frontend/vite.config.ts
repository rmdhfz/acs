import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import pkg from './package.json' with { type: 'json' }

// URL publik situs untuk canonical/OG/sitemap (build-time). Atur lewat VITE_SITE_URL.
process.env.VITE_SITE_URL = (process.env.VITE_SITE_URL || 'http://localhost:5173').replace(/\/+$/, '')

// https://vite.dev/config/
export default defineConfig({
  // Versi aplikasi (dari package.json) ditampilkan di footer sidebar & dilampirkan pada laporan bug.
  define: { __APP_VERSION__: JSON.stringify(pkg.version) },
  plugins: [react(), tailwindcss()],
  server: {
    port: 5173,
  },
  // Tes komponen (Vitest + Testing Library). csv.test.mjs tetap dijalankan oleh node --test (lihat "test").
  test: {
    environment: 'jsdom',
    include: ['src/**/*.test.{ts,tsx}'],
    restoreMocks: true,
    setupFiles: ['src/test/setup.ts'],
  },
})
