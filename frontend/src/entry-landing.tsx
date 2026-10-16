// Entry SSR (hanya dipakai saat build — lihat scripts/postbuild-seo.mjs): merender
// beranda ke string HTML statis dan membangun JSON-LD. Tidak ikut bundel klien.
import { renderToString } from 'react-dom/server'
import { LandingPage } from './pages/LandingPage'
import { buildJsonLd } from './landing/content'

export function renderLanding(): string {
  return renderToString(<LandingPage />)
}

export { buildJsonLd }
