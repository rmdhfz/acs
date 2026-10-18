import { ArrowRight, Bell, Building2, Eye, Layers, Palette, Plug, Rocket, ShieldCheck, Upload, Wifi, Zap, type LucideIcon } from 'lucide-react'
import { FAQS, FEATURES, NAV, SITE_NAME, STEPS, TAGLINE, type Feature } from '../landing/content'
import { CREDIT_NAME, CREDIT_URL } from '../components/CreditFooter'

// Halaman beranda publik bergaya Google (putih, bersih, tombol pil biru, kartu 16–24px). SENGAJA tidak memakai
// context/hook aplikasi (auth, i18n, router): komponen ini dirender statis saat build (entry-landing.tsx) ke dalam
// index.html supaya crawler & pratinjau sosial membaca konten penuh tanpa JavaScript. Navigasi memakai <a> biasa.

const ICONS: Record<Feature['icon'], LucideIcon> = {
  layers: Layers,
  zap: Zap,
  rocket: Rocket,
  shield: ShieldCheck,
  plug: Plug,
  upload: Upload,
  eye: Eye,
  building: Building2,
  wifi: Wifi,
  bell: Bell,
  palette: Palette,
}

// Empat warna Google bergilir untuk ubin ikon fitur (kontras ikon terhadap latar ≥ 4.5:1).
const TILE = [
  'bg-indigo-50 text-indigo-700',
  'bg-red-50 text-red-700',
  'bg-amber-50 text-amber-700',
  'bg-emerald-50 text-emerald-700',
]

const CONTACT_EMAIL = (import.meta.env.VITE_CONTACT_EMAIL as string | undefined) ?? ''

const btnPrimary =
  'inline-flex h-12 items-center justify-center gap-2 rounded-full bg-indigo-600 px-7 text-[15px] font-medium text-white transition hover:bg-indigo-700 hover:shadow-md focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-600'
const btnOutline =
  'inline-flex h-12 items-center justify-center gap-2 rounded-full border border-slate-300 bg-white px-7 text-[15px] font-medium text-indigo-700 transition hover:bg-indigo-50 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-600'

function Container({ children, className = '' }: { children: React.ReactNode; className?: string }) {
  return <div className={`mx-auto w-full max-w-6xl px-5 sm:px-8 ${className}`}>{children}</div>
}

function SectionHead({ id, eyebrow, title, lead }: { id: string; eyebrow: string; title: string; lead?: string }) {
  return (
    <div className="mx-auto max-w-2xl text-center">
      <p className="text-sm font-medium text-indigo-700">{eyebrow}</p>
      <h2 id={id} className="mt-2 text-3xl font-normal leading-tight tracking-tight text-slate-900 sm:text-[2.5rem]">
        {title}
      </h2>
      {lead && <p className="mt-4 text-lg leading-relaxed text-slate-600">{lead}</p>}
    </div>
  )
}

// Ilustrasi tampilan Console (bukan data nyata) — dibuat dengan elemen HTML agar tajam dan ringan, tanpa gambar bitmap.
function ConsolePreview() {
  const bars = [38, 52, 44, 66, 58, 74, 70, 86, 78, 92]
  return (
    <figure className="relative" aria-label="Ilustrasi tampilan dashboard ACS Console">
      <div className="overflow-hidden rounded-3xl border border-slate-200 bg-white shadow-xl">
        <div className="flex items-center gap-2 border-b border-slate-200 bg-slate-50 px-4 py-3" aria-hidden="true">
          <span className="h-3 w-3 rounded-full bg-red-400" />
          <span className="h-3 w-3 rounded-full bg-amber-400" />
          <span className="h-3 w-3 rounded-full bg-emerald-400" />
          <span className="ml-3 h-6 flex-1 rounded-full bg-slate-200/70" />
        </div>
        <div className="grid grid-cols-[3.2rem_1fr] sm:grid-cols-[9rem_1fr]" aria-hidden="true">
          <div className="space-y-2 border-r border-slate-200 p-3">
            {['Dashboard', 'Perangkat', 'Task', 'Firmware', 'Preset'].map((n, i) => (
              <div key={n} className={`flex h-8 items-center gap-2 rounded-full px-2.5 text-[11px] font-medium ${i === 0 ? 'bg-indigo-50 text-indigo-700' : 'text-slate-500'}`}>
                <span className={`h-3.5 w-3.5 shrink-0 rounded ${i === 0 ? 'bg-indigo-600' : 'bg-slate-300'}`} />
                <span className="hidden truncate sm:block">{n}</span>
              </div>
            ))}
          </div>
          <div className="p-4 sm:p-5">
            <div className="grid grid-cols-3 gap-3">
              {[
                ['Perangkat', 'bg-indigo-600'],
                ['Online', 'bg-emerald-600'],
                ['Task gagal', 'bg-red-600'],
              ].map(([label, color]) => (
                <div key={label} className="rounded-2xl border border-slate-200 p-3">
                  <span className={`block h-1.5 w-8 rounded-full ${color}`} />
                  <p className="mt-2 text-[11px] text-slate-500">{label}</p>
                  <span className="mt-1 block h-4 w-12 rounded bg-slate-200" />
                </div>
              ))}
            </div>
            <div className="mt-3 flex h-28 items-end gap-1.5 rounded-2xl border border-slate-200 p-3 sm:h-36">
              {bars.map((h, i) => (
                <span key={i} className="flex-1 rounded-t-md bg-indigo-200" style={{ height: `${h}%` }}>
                  <span className="block w-full rounded-t-md bg-indigo-600" style={{ height: `${Math.max(30, h - 20)}%` }} />
                </span>
              ))}
            </div>
            <div className="mt-3 space-y-2">
              {[78, 64, 70].map((w, i) => (
                <div key={i} className="flex items-center gap-3 rounded-xl bg-slate-50 px-3 py-2">
                  <span className={`h-2.5 w-2.5 shrink-0 rounded-full ${i === 1 ? 'bg-red-500' : 'bg-emerald-500'}`} />
                  <span className="h-2.5 rounded bg-slate-300" style={{ width: `${w}%` }} />
                </div>
              ))}
            </div>
          </div>
        </div>
      </div>
      <figcaption className="mt-3 text-center text-xs text-slate-500">Ilustrasi tampilan Console</figcaption>
    </figure>
  )
}

function ArchitectureDiagram() {
  return (
    <svg viewBox="0 0 560 330" role="img" aria-labelledby="diag-title diag-desc" className="h-auto w-full">
      <title id="diag-title">Diagram: ACS mengelola ONT lewat TR-069, OLT dikelola terpisah</title>
      <desc id="diag-desc">
        ACS terhubung ke ONT di rumah pelanggan melalui TR-069/CWMP lewat jaringan IP. OLT di sisi operator dikelola lewat sistem
        manajemen OLT, bukan oleh ACS.
      </desc>
      <defs>
        <marker id="ah" viewBox="0 0 10 10" refX="8" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse">
          <path d="M0,0 L10,5 L0,10 z" fill="#1a73e8" />
        </marker>
      </defs>
      <rect x="170" y="14" width="220" height="64" rx="18" fill="#1a73e8" />
      <text x="280" y="42" textAnchor="middle" fill="#fff" fontSize="15" fontWeight="700">ACS Console</text>
      <text x="280" y="62" textAnchor="middle" fill="#e8f0fe" fontSize="11">Server TR-069 / CWMP + Console web</text>

      <rect x="20" y="130" width="520" height="30" rx="15" fill="#f1f3f4" stroke="#dadce0" />
      <text x="280" y="150" textAnchor="middle" fill="#5f6368" fontSize="11">Jaringan IP operator · VLAN manajemen</text>

      <line x1="280" y1="78" x2="280" y2="226" stroke="#1a73e8" strokeWidth="3" markerStart="url(#ah)" markerEnd="url(#ah)" />
      <text x="292" y="196" fill="#1967d2" fontSize="11" fontWeight="700">TR-069 / CWMP</text>

      <rect x="330" y="226" width="210" height="84" rx="18" fill="#e8f0fe" stroke="#1a73e8" strokeWidth="1.5" />
      <text x="435" y="252" textAnchor="middle" fill="#174ea6" fontSize="14" fontWeight="700">ONT / CPE</text>
      <text x="435" y="272" textAnchor="middle" fill="#1967d2" fontSize="11">WiFi · PPPoE · firmware</text>
      <text x="435" y="290" textAnchor="middle" fill="#1967d2" fontSize="11">dikonfigurasi oleh ACS</text>

      <rect x="20" y="226" width="210" height="84" rx="18" fill="#fef7e0" stroke="#f9ab00" strokeDasharray="5 4" />
      <text x="125" y="252" textAnchor="middle" fill="#965100" fontSize="14" fontWeight="700">OLT</text>
      <text x="125" y="272" textAnchor="middle" fill="#7a4100" fontSize="11">registrasi ONT · VLAN · bandwidth</text>
      <text x="125" y="290" textAnchor="middle" fill="#7a4100" fontSize="11">dikelola sistem manajemen OLT</text>

      <line x1="230" y1="268" x2="330" y2="268" stroke="#f9ab00" strokeWidth="4" />
      <text x="280" y="258" textAnchor="middle" fill="#965100" fontSize="10">serat PON</text>
    </svg>
  )
}

export function LandingPage() {
  const demoHref = CONTACT_EMAIL ? `mailto:${CONTACT_EMAIL}?subject=${encodeURIComponent('Permintaan demo ' + SITE_NAME)}` : ''

  return (
    <div className="min-h-screen bg-white text-slate-900" style={{ colorScheme: 'light' }}>
      <a
        href="#konten"
        className="sr-only focus:not-sr-only focus:fixed focus:left-3 focus:top-3 focus:z-50 focus:rounded-full focus:bg-white focus:px-4 focus:py-2 focus:text-sm focus:font-medium focus:text-indigo-700 focus:shadow-lg"
      >
        Lewati ke konten utama
      </a>

      <header className="sticky top-0 z-40 border-b border-slate-200 bg-white/95 backdrop-blur">
        <Container className="flex h-16 items-center justify-between gap-4">
          <a href="/" className="flex items-center gap-3 text-slate-900" aria-label={`${SITE_NAME} — beranda`}>
            <span className="flex h-9 w-9 items-center justify-center rounded-xl bg-indigo-600 text-white" aria-hidden="true">
              <Wifi className="h-5 w-5" />
            </span>
            <span className="text-[17px] font-medium">{SITE_NAME}</span>
          </a>
          <nav aria-label="Navigasi utama" className="hidden items-center gap-1 md:flex">
            {NAV.map((n) => (
              <a key={n.href} href={n.href} className="rounded-full px-4 py-2 text-sm font-medium text-slate-700 transition hover:bg-slate-100">
                {n.label}
              </a>
            ))}
          </nav>
          <div className="flex items-center gap-2">
            <a href="/signup" className="hidden h-10 items-center rounded-full px-5 text-sm font-medium text-indigo-700 transition hover:bg-indigo-50 sm:inline-flex">
              Daftar
            </a>
            <a href="/login" className="inline-flex h-10 items-center rounded-full bg-indigo-600 px-6 text-sm font-medium text-white transition hover:bg-indigo-700 hover:shadow-md">
              Masuk
            </a>
          </div>
        </Container>
      </header>

      <main id="konten">
        {/* HERO */}
        <section aria-labelledby="judul-utama" className="bg-white">
          <Container className="grid items-center gap-12 py-14 sm:py-20 lg:grid-cols-[1.05fr_1fr]">
            <div>
              <p className="inline-flex items-center rounded-full bg-indigo-50 px-4 py-1.5 text-sm font-medium text-indigo-700">
                TR-069 · CWMP · TR-098 &amp; TR-181
              </p>
              <h1 id="judul-utama" className="mt-6 text-4xl font-normal leading-[1.12] tracking-tight text-slate-900 sm:text-6xl">
                Kelola ribuan ONT dari <span className="text-indigo-600">satu konsol</span>
              </h1>
              <p className="mt-6 max-w-xl text-lg leading-relaxed text-slate-600">
                {SITE_NAME} adalah {TAGLINE}. Provisioning otomatis, firmware bertahap, preset konfigurasi, notifikasi, multi-tenant, dan
                audit trail lengkap — tanpa mengubah kode saat menambah merek baru.
              </p>
              <div className="mt-9 flex flex-wrap gap-3">
                <a href="/signup" className={btnPrimary}>
                  Daftar sekarang <ArrowRight className="h-5 w-5" aria-hidden="true" />
                </a>
                <a href="/login" className={btnOutline}>
                  Masuk
                </a>
                {demoHref && (
                  <a href={demoHref} className="inline-flex h-12 items-center rounded-full px-5 text-[15px] font-medium text-slate-700 transition hover:bg-slate-100">
                    Minta demo
                  </a>
                )}
              </div>
              <p className="mt-7 text-sm text-slate-600">
                Katalog 30+ merek: ZTE · Huawei · FiberHome · Nokia · C-Data · TP-Link · VSOL · Tenda · Sagemcom · dan lainnya
              </p>
            </div>
            <ConsolePreview />
          </Container>
        </section>

        {/* FITUR */}
        <section id="fitur" aria-labelledby="judul-fitur" className="scroll-mt-16 bg-slate-50 py-16 sm:py-24">
          <Container>
            <SectionHead
              id="judul-fitur"
              eyebrow="Fitur"
              title="Semua yang dibutuhkan NOC untuk mengoperasikan armada ONT"
              lead="Dibangun untuk operator yang mengelola banyak merek, banyak tenant, dan banyak tim."
            />
            <ul className="mt-14 grid gap-5 sm:grid-cols-2 lg:grid-cols-3">
              {FEATURES.map((f, i) => {
                const Icon = ICONS[f.icon]
                return (
                  <li key={f.title} className="rounded-3xl border border-slate-200 bg-white p-7 transition hover:shadow-md">
                    <span className={`flex h-12 w-12 items-center justify-center rounded-2xl ${TILE[i % TILE.length]}`} aria-hidden="true">
                      <Icon className="h-6 w-6" strokeWidth={1.75} />
                    </span>
                    <h3 className="mt-5 text-lg font-medium text-slate-900">{f.title}</h3>
                    <p className="mt-2 text-[15px] leading-relaxed text-slate-600">{f.body}</p>
                  </li>
                )
              })}
            </ul>
          </Container>
        </section>

        {/* CARA KERJA */}
        <section id="cara-kerja" aria-labelledby="judul-cara" className="scroll-mt-16 bg-white py-16 sm:py-24">
          <Container>
            <SectionHead
              id="judul-cara"
              eyebrow="Cara kerja"
              title="Dari ONT menyala sampai terkonfigurasi"
              lead="TR-069 bersifat sesi: ONT yang memulai kontak, ACS yang menjawab dengan perintah."
            />
            <ol className="mt-14 grid gap-6 md:grid-cols-3">
              {STEPS.map((s, i) => (
                <li key={s.title} className="rounded-3xl bg-slate-50 p-7">
                  <span className="flex h-10 w-10 items-center justify-center rounded-full bg-indigo-600 text-base font-medium text-white" aria-hidden="true">
                    {i + 1}
                  </span>
                  <h3 className="mt-5 text-lg font-medium text-slate-900">{s.title}</h3>
                  <p className="mt-2 text-[15px] leading-relaxed text-slate-600">{s.body}</p>
                </li>
              ))}
            </ol>
          </Container>
        </section>

        {/* ONT vs OLT */}
        <section id="ont-vs-olt" aria-labelledby="judul-ont" className="scroll-mt-16 bg-slate-50 py-16 sm:py-24">
          <Container>
            <SectionHead
              id="judul-ont"
              eyebrow="Batas tanggung jawab"
              title="ACS mengelola ONT, OLT tetap di sistem OLT"
              lead="Pembagian yang jelas mencegah tumpang tindih konfigurasi antara tim jaringan dan tim operasional."
            />
            <div className="mt-14 grid items-center gap-10 lg:grid-cols-2">
              <div className="rounded-3xl border border-slate-200 bg-white p-6 sm:p-8">
                <ArchitectureDiagram />
              </div>
              <div className="space-y-5">
                <div className="rounded-3xl bg-amber-50 p-7">
                  <h3 className="text-lg font-medium text-amber-900">Di sisi OLT (tim jaringan)</h3>
                  <ul className="mt-3 list-disc space-y-1.5 pl-5 text-[15px] text-amber-900">
                    <li>Registrasi ONT di port PON</li>
                    <li>VLAN servis dan jalur manajemen menuju ACS</li>
                    <li>Profil bandwidth dan line/service profile</li>
                  </ul>
                </div>
                <div className="rounded-3xl bg-indigo-50 p-7">
                  <h3 className="text-lg font-medium text-indigo-900">Di sisi ONT lewat ACS (tim operasional)</h3>
                  <ul className="mt-3 list-disc space-y-1.5 pl-5 text-[15px] text-indigo-900">
                    <li>SSID dan password WiFi, akun PPPoE, LAN</li>
                    <li>Reboot, factory reset, dan upgrade firmware</li>
                    <li>Pembacaan status, parameter, dan riwayat event</li>
                  </ul>
                </div>
              </div>
            </div>
          </Container>
        </section>

        {/* FAQ */}
        <section id="faq" aria-labelledby="judul-faq" className="scroll-mt-16 bg-white py-16 sm:py-24">
          <Container className="max-w-3xl">
            <SectionHead id="judul-faq" eyebrow="FAQ" title="Pertanyaan yang sering diajukan" />
            <div className="mt-12 space-y-3">
              {FAQS.map((f) => (
                <details key={f.q} className="group rounded-2xl border border-slate-200 bg-white px-6 py-5 open:bg-slate-50">
                  <summary className="flex cursor-pointer list-none items-center justify-between gap-4 text-left">
                    <h3 className="text-base font-medium text-slate-900">{f.q}</h3>
                    <span className="text-2xl font-light leading-none text-indigo-700 transition group-open:rotate-45" aria-hidden="true">+</span>
                  </summary>
                  <p className="mt-3 text-[15px] leading-relaxed text-slate-600">{f.a}</p>
                </details>
              ))}
            </div>
          </Container>
        </section>

        {/* CTA */}
        <section aria-labelledby="judul-cta" className="bg-indigo-50 py-16 sm:py-20">
          <Container className="text-center">
            <h2 id="judul-cta" className="text-3xl font-normal tracking-tight text-slate-900 sm:text-4xl">
              Siap mengelola armada ONT dengan lebih rapi?
            </h2>
            <p className="mx-auto mt-4 max-w-xl text-lg text-slate-600">
              Masuk ke Console, atau hubungi kami untuk demo dan rencana pilot bersama tim operasional dan jaringan Anda.
            </p>
            <div className="mt-9 flex flex-wrap justify-center gap-3">
              <a href="/signup" className={btnPrimary}>
                Daftar sekarang <ArrowRight className="h-5 w-5" aria-hidden="true" />
              </a>
              <a href="/login" className={btnOutline}>
                Masuk ke Console
              </a>
              {demoHref && (
                <a href={demoHref} className="inline-flex h-12 items-center rounded-full px-5 text-[15px] font-medium text-slate-700 transition hover:bg-white/60">
                  Hubungi kami
                </a>
              )}
            </div>
          </Container>
        </section>
      </main>

      <footer className="border-t border-slate-200 bg-slate-50" aria-label="Footer situs">
        <Container className="grid gap-10 py-12 sm:grid-cols-2 lg:grid-cols-[1.5fr_1fr_1fr_1fr]">
          <div>
            <a href="/" className="flex items-center gap-3 text-slate-900" aria-label={`${SITE_NAME} — beranda`}>
              <span className="flex h-9 w-9 items-center justify-center rounded-xl bg-indigo-600 text-white" aria-hidden="true">
                <Wifi className="h-5 w-5" />
              </span>
              <span className="text-[17px] font-medium">{SITE_NAME}</span>
            </a>
            <p className="mt-4 max-w-xs text-sm leading-relaxed text-slate-600">{TAGLINE}. Kelola armada ONT/ONU dari satu konsol: provisioning, firmware, notifikasi, dan audit.</p>
          </div>
          <nav aria-label="Produk">
            <h2 className="text-sm font-medium text-slate-900">Produk</h2>
            <ul className="mt-4 space-y-2.5 text-sm text-slate-600">
              {NAV.map((n) => (
                <li key={n.href}>
                  <a href={n.href} className="hover:text-indigo-700 hover:underline">{n.label}</a>
                </li>
              ))}
            </ul>
          </nav>
          <nav aria-label="Akun">
            <h2 className="text-sm font-medium text-slate-900">Akun</h2>
            <ul className="mt-4 space-y-2.5 text-sm text-slate-600">
              <li><a href="/login" className="hover:text-indigo-700 hover:underline">Masuk</a></li>
              <li><a href="/signup" className="hover:text-indigo-700 hover:underline">Daftar</a></li>
              <li><a href="/forgot-password" className="hover:text-indigo-700 hover:underline">Lupa password</a></li>
            </ul>
          </nav>
          <nav aria-label="Legal dan kontak">
            <h2 className="text-sm font-medium text-slate-900">Perusahaan</h2>
            <ul className="mt-4 space-y-2.5 text-sm text-slate-600">
              <li><a href="/privacy" className="hover:text-indigo-700 hover:underline">Kebijakan privasi</a></li>
              <li><a href="/terms" className="hover:text-indigo-700 hover:underline">Persyaratan layanan</a></li>
              {CONTACT_EMAIL && (
                <li><a href={`mailto:${CONTACT_EMAIL}`} className="hover:text-indigo-700 hover:underline">Hubungi kami</a></li>
              )}
            </ul>
          </nav>
        </Container>
        <div className="border-t border-slate-200">
          <Container className="flex flex-col items-center justify-between gap-2 py-5 text-xs text-slate-600 sm:flex-row">
            <p>© {new Date().getFullYear()} {SITE_NAME}. TR-069 / CWMP Auto Configuration Server.</p>
            <p>
              Dibuat oleh{' '}
              <a href={CREDIT_URL} target="_blank" rel="noopener noreferrer" className="font-medium text-slate-700 hover:text-indigo-700 hover:underline">
                {CREDIT_NAME}
              </a>
            </p>
          </Container>
        </div>
      </footer>
    </div>
  )
}
