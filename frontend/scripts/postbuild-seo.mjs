// Pasca-build SEO: (1) render beranda ke HTML statis dan sisipkan ke dist/index.html,
// (2) sisipkan JSON-LD, (3) tulis robots.txt & sitemap.xml dengan URL situs.
// Dijalankan oleh `npm run build` setelah `vite build` dan `vite build --ssr`.
import { readFile, writeFile, rm } from 'node:fs/promises'
import { pathToFileURL } from 'node:url'
import path from 'node:path'
import { loadEnv } from 'vite'

const root = path.resolve(import.meta.dirname, '..')
const dist = path.join(root, 'dist')
const ssrDir = path.join(root, 'dist-ssr')

// .env / .env.production + variabel proses (sama dengan yang dipakai vite build)
const env = { ...loadEnv('production', root, 'VITE_'), ...process.env }
const siteUrl = (env.VITE_SITE_URL || 'http://localhost:5173').replace(/\/+$/, '')
const contactEmail = env.VITE_CONTACT_EMAIL || ''

const mod = await import(pathToFileURL(path.join(ssrDir, 'entry-landing.js')).href)
const html = mod.renderLanding()
const jsonLd = JSON.stringify(mod.buildJsonLd(siteUrl, contactEmail)).replace(/</g, '\\u003c')

const indexPath = path.join(dist, 'index.html')
let index = await readFile(indexPath, 'utf8')
for (const marker of ['<!--ssr-landing-->', '<!--seo-jsonld-->']) {
  if (!index.includes(marker)) throw new Error(`penanda ${marker} tidak ada di dist/index.html`)
}
index = index
  .replace('<!--ssr-landing-->', () => html)
  .replace('<!--seo-jsonld-->', () => `<script type="application/ld+json">${jsonLd}</script>`)
if (/%VITE_[A-Z_]+%/.test(index)) throw new Error('placeholder %VITE_*% belum terganti di dist/index.html')
await writeFile(indexPath, index)

const today = new Date().toISOString().slice(0, 10)
await writeFile(
  path.join(dist, 'sitemap.xml'),
  `<?xml version="1.0" encoding="UTF-8"?>\n<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">\n  <url>\n    <loc>${siteUrl}/</loc>\n    <lastmod>${today}</lastmod>\n    <changefreq>monthly</changefreq>\n    <priority>1.0</priority>\n  </url>\n</urlset>\n`,
)
await writeFile(
  path.join(dist, 'robots.txt'),
  [
    'User-agent: *',
    'Allow: /$',
    'Allow: /og-image.png',
    'Allow: /favicon.svg',
    'Allow: /manifest.webmanifest',
    'Allow: /assets/',
    '# Halaman aplikasi (butuh login) tidak untuk diindeks',
    'Disallow: /login',
    'Disallow: /sso',
    'Disallow: /forgot-password',
    'Disallow: /reset-password',
    'Disallow: /dashboard',
    'Disallow: /devices',
    'Disallow: /tasks',
    'Disallow: /provisioning',
    'Disallow: /presets',
    'Disallow: /firmware',
    'Disallow: /files',
    'Disallow: /tags',
    'Disallow: /webhooks',
    'Disallow: /audit',
    'Disallow: /catalog',
    'Disallow: /administration',
    'Disallow: /profile',
    'Disallow: /self-service',
    'Disallow: /reports',
    'Disallow: /notifications',
    'Disallow: /signup',
    'Disallow: /verify-signup',
    'Disallow: /api/',
    '',
    `Sitemap: ${siteUrl}/sitemap.xml`,
    '',
  ].join('\n'),
)
await rm(ssrDir, { recursive: true, force: true })
console.log(`[seo] beranda ter-prerender (${html.length} byte), JSON-LD, robots.txt, sitemap.xml untuk ${siteUrl}`)
