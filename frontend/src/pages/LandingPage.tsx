import { ArrowRight, Building2, Eye, Layers, Plug, Rocket, ShieldCheck, Upload, Wifi, Zap, type LucideIcon } from 'lucide-react'
import { FAQS, FEATURES, NAV, SITE_NAME, STEPS, TAGLINE, type Feature } from '../landing/content'
import { CREDIT_NAME, CREDIT_URL } from '../components/CreditFooter'

// Halaman beranda publik. SENGAJA tidak memakai context/hook aplikasi (auth, i18n,
// router): komponen ini dirender statis saat build (entry-landing.tsx) ke dalam
// index.html supaya crawler & pratinjau sosial membaca konten penuh tanpa JavaScript.
// Navigasi memakai <a> biasa.

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
}

const CONTACT_EMAIL = (import.meta.env.VITE_CONTACT_EMAIL as string | undefined) ?? ''

const btnPrimary =
  'inline-flex items-center justify-center gap-2 rounded-lg bg-indigo-600 px-5 py-3 text-sm font-semibold text-white shadow-sm transition hover:bg-indigo-500 focus:outline-none focus-visible:ring-2 focus-visible:ring-indigo-300 focus-visible:ring-offset-2 focus-visible:ring-offset-slate-900'
const btnGhost =
  'inline-flex items-center justify-center gap-2 rounded-lg border border-white/25 bg-white/5 px-5 py-3 text-sm font-semibold text-white transition hover:bg-white/10 focus:outline-none focus-visible:ring-2 focus-visible:ring-white/60'

function Container({ children, className = '' }: { children: React.ReactNode; className?: string }) {
  return <div className={`mx-auto w-full max-w-6xl px-5 sm:px-8 ${className}`}>{children}</div>
}

function SectionHead({ id, eyebrow, title, lead }: { id: string; eyebrow: string; title: string; lead?: string }) {
  return (
    <div className="mx-auto max-w-2xl text-center">
      <p className="text-xs font-semibold uppercase tracking-[0.16em] text-indigo-600">{eyebrow}</p>
      <h2 id={id} className="mt-2 text-2xl font-bold tracking-tight text-slate-900 sm:text-3xl">
        {title}
      </h2>
      {lead && <p className="mt-3 text-base leading-relaxed text-slate-600">{lead}</p>}
    </div>
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
          <path d="M0,0 L10,5 L0,10 z" fill="#818cf8" />
        </marker>
      </defs>
      <rect x="170" y="14" width="220" height="64" rx="10" fill="#4f46e5" />
      <text x="280" y="42" textAnchor="middle" fill="#fff" fontSize="15" fontWeight="700">ACS Console</text>
      <text x="280" y="62" textAnchor="middle" fill="#c7d2fe" fontSize="11">Server TR-069 / CWMP + Console web</text>

      <rect x="20" y="130" width="520" height="30" rx="15" fill="#1e293b" stroke="#334155" />
      <text x="280" y="150" textAnchor="middle" fill="#94a3b8" fontSize="11">Jaringan IP operator · VLAN manajemen</text>

      <line x1="280" y1="78" x2="280" y2="226" stroke="#818cf8" strokeWidth="3" markerStart="url(#ah)" markerEnd="url(#ah)" />
      <text x="292" y="196" fill="#c7d2fe" fontSize="11" fontWeight="700">TR-069 / CWMP</text>

      <rect x="330" y="226" width="210" height="84" rx="10" fill="#1e1b4b" stroke="#6366f1" strokeWidth="1.5" />
      <text x="435" y="252" textAnchor="middle" fill="#fff" fontSize="14" fontWeight="700">ONT / CPE</text>
      <text x="435" y="272" textAnchor="middle" fill="#a5b4fc" fontSize="11">WiFi · PPPoE · firmware</text>
      <text x="435" y="290" textAnchor="middle" fill="#a5b4fc" fontSize="11">dikonfigurasi oleh ACS</text>

      <rect x="20" y="226" width="210" height="84" rx="10" fill="#1c1917" stroke="#78716c" strokeDasharray="5 4" />
      <text x="125" y="252" textAnchor="middle" fill="#fbbf24" fontSize="14" fontWeight="700">OLT</text>
      <text x="125" y="272" textAnchor="middle" fill="#d6d3d1" fontSize="11">registrasi ONT · VLAN · bandwidth</text>
      <text x="125" y="290" textAnchor="middle" fill="#d6d3d1" fontSize="11">dikelola sistem manajemen OLT</text>

      <line x1="230" y1="268" x2="330" y2="268" stroke="#f59e0b" strokeWidth="4" />
      <text x="280" y="258" textAnchor="middle" fill="#fbbf24" fontSize="10">serat PON</text>
    </svg>
  )
}

export function LandingPage() {
  const demoHref = CONTACT_EMAIL ? `mailto:${CONTACT_EMAIL}?subject=${encodeURIComponent('Permintaan demo ' + SITE_NAME)}` : ''

  return (
    <div className="min-h-screen bg-white text-slate-900" style={{ colorScheme: 'light' }}>
      <a
        href="#konten"
        className="sr-only focus:not-sr-only focus:fixed focus:left-3 focus:top-3 focus:z-50 focus:rounded-md focus:bg-white focus:px-3 focus:py-2 focus:text-sm focus:font-semibold focus:text-indigo-700"
      >
        Lewati ke konten utama
      </a>

      <header className="sticky top-0 z-40 border-b border-white/10 bg-slate-950/95 backdrop-blur">
        <Container className="flex h-16 items-center justify-between">
          <a href="/" className="flex items-center gap-2 text-white" aria-label={`${SITE_NAME} — beranda`}>
            <span className="flex h-8 w-8 items-center justify-center rounded-lg bg-indigo-600" aria-hidden="true">
              <Wifi className="h-4 w-4" />
            </span>
            <span className="text-base font-bold tracking-tight">{SITE_NAME}</span>
          </a>
          <nav aria-label="Navigasi utama" className="hidden items-center gap-6 md:flex">
            {NAV.map((n) => (
              <a key={n.href} href={n.href} className="text-sm font-medium text-slate-300 transition hover:text-white">
                {n.label}
              </a>
            ))}
          </nav>
          <a href="/login" className="rounded-lg bg-white px-4 py-2 text-sm font-semibold text-slate-900 transition hover:bg-indigo-50">
            Masuk
          </a>
        </Container>
      </header>

      <main id="konten">
        {/* HERO */}
        <section aria-labelledby="judul-utama" className="relative overflow-hidden bg-slate-950 text-white">
          <div
            className="pointer-events-none absolute inset-0 opacity-60"
            style={{ background: 'radial-gradient(60% 60% at 80% 10%, rgba(99,102,241,.35), transparent 70%), radial-gradient(40% 50% at 10% 90%, rgba(14,165,233,.18), transparent 70%)' }}
            aria-hidden="true"
          />
          <Container className="relative grid items-center gap-12 py-16 sm:py-24 lg:grid-cols-2">
            <div>
              <p className="inline-flex items-center rounded-full border border-indigo-400/30 bg-indigo-400/10 px-3 py-1 text-xs font-semibold text-indigo-200">
                TR-069 · CWMP · TR-098 &amp; TR-181
              </p>
              <h1 id="judul-utama" className="mt-5 text-4xl font-extrabold leading-tight tracking-tight sm:text-5xl">
                Kelola ribuan ONT dari <span className="text-indigo-300">satu konsol</span>
              </h1>
              <p className="mt-5 max-w-xl text-lg leading-relaxed text-slate-300">
                {SITE_NAME} adalah {TAGLINE}. Provisioning otomatis, firmware bertahap, preset konfigurasi, multi-tenant, dan audit
                trail lengkap, tanpa mengubah kode saat menambah merek baru.
              </p>
              <div className="mt-8 flex flex-wrap gap-3">
                <a href="/login" className={btnPrimary}>
                  Masuk ke Console <ArrowRight className="h-4 w-4" aria-hidden="true" />
                </a>
                {demoHref && (
                  <a href={demoHref} className={btnGhost}>
                    Minta demo
                  </a>
                )}
                <a href="#cara-kerja" className={btnGhost}>
                  Lihat cara kerja
                </a>
              </div>
              <p className="mt-6 text-sm text-slate-400">Katalog 30+ merek: ZTE · Huawei · FiberHome · Nokia · C-Data · TP-Link · VSOL · Tenda · Sagemcom · dan lainnya</p>
            </div>
            <div className="rounded-2xl border border-white/10 bg-slate-900/70 p-4 shadow-2xl sm:p-6">
              <ArchitectureDiagram />
            </div>
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
            <ul className="mt-12 grid gap-5 sm:grid-cols-2 lg:grid-cols-3">
              {FEATURES.map((f) => {
                const Icon = ICONS[f.icon]
                return (
                  <li key={f.title} className="rounded-xl border border-slate-200 bg-white p-6 shadow-sm">
                    <span className="flex h-10 w-10 items-center justify-center rounded-lg bg-indigo-50 text-indigo-600" aria-hidden="true">
                      <Icon className="h-5 w-5" />
                    </span>
                    <h3 className="mt-4 text-base font-semibold text-slate-900">{f.title}</h3>
                    <p className="mt-2 text-sm leading-relaxed text-slate-600">{f.body}</p>
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
            <ol className="mt-12 grid gap-6 md:grid-cols-3">
              {STEPS.map((s, i) => (
                <li key={s.title} className="relative rounded-xl border border-slate-200 p-6">
                  <span className="flex h-8 w-8 items-center justify-center rounded-full bg-indigo-600 text-sm font-bold text-white" aria-hidden="true">
                    {i + 1}
                  </span>
                  <h3 className="mt-4 text-base font-semibold text-slate-900">{s.title}</h3>
                  <p className="mt-2 text-sm leading-relaxed text-slate-600">{s.body}</p>
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
            <div className="mt-12 grid gap-6 md:grid-cols-2">
              <div className="rounded-xl border border-amber-200 bg-amber-50 p-6">
                <h3 className="text-base font-semibold text-amber-900">Di sisi OLT (tim jaringan)</h3>
                <ul className="mt-3 list-disc space-y-1.5 pl-5 text-sm text-amber-900/90">
                  <li>Registrasi ONT di port PON</li>
                  <li>VLAN servis dan jalur manajemen menuju ACS</li>
                  <li>Profil bandwidth dan line/service profile</li>
                </ul>
              </div>
              <div className="rounded-xl border border-indigo-200 bg-indigo-50 p-6">
                <h3 className="text-base font-semibold text-indigo-900">Di sisi ONT lewat ACS (tim operasional)</h3>
                <ul className="mt-3 list-disc space-y-1.5 pl-5 text-sm text-indigo-900/90">
                  <li>SSID dan password WiFi, akun PPPoE, LAN</li>
                  <li>Reboot, factory reset, dan upgrade firmware</li>
                  <li>Pembacaan status, parameter, dan riwayat event</li>
                </ul>
              </div>
            </div>
          </Container>
        </section>

        {/* FAQ */}
        <section id="faq" aria-labelledby="judul-faq" className="scroll-mt-16 bg-white py-16 sm:py-24">
          <Container className="max-w-3xl">
            <SectionHead id="judul-faq" eyebrow="FAQ" title="Pertanyaan yang sering diajukan" />
            <div className="mt-10 divide-y divide-slate-200 rounded-xl border border-slate-200">
              {FAQS.map((f) => (
                <details key={f.q} className="group p-5 open:bg-slate-50">
                  <summary className="flex cursor-pointer list-none items-center justify-between gap-4 text-left text-base font-semibold text-slate-900">
                    <h3 className="text-base font-semibold">{f.q}</h3>
                    <span className="text-xl leading-none text-indigo-600 transition group-open:rotate-45" aria-hidden="true">+</span>
                  </summary>
                  <p className="mt-3 text-sm leading-relaxed text-slate-600">{f.a}</p>
                </details>
              ))}
            </div>
          </Container>
        </section>

        {/* CTA */}
        <section aria-labelledby="judul-cta" className="bg-slate-950 py-16 text-white sm:py-20">
          <Container className="text-center">
            <h2 id="judul-cta" className="text-2xl font-bold tracking-tight sm:text-3xl">
              Siap mengelola armada ONT dengan lebih rapi?
            </h2>
            <p className="mx-auto mt-3 max-w-xl text-slate-300">
              Masuk ke Console, atau hubungi kami untuk demo dan rencana pilot bersama tim operasional dan jaringan Anda.
            </p>
            <div className="mt-8 flex flex-wrap justify-center gap-3">
              <a href="/login" className={btnPrimary}>
                Masuk ke Console <ArrowRight className="h-4 w-4" aria-hidden="true" />
              </a>
              {demoHref && (
                <a href={demoHref} className={btnGhost}>
                  Hubungi kami
                </a>
              )}
            </div>
          </Container>
        </section>
      </main>

      <footer className="border-t border-slate-200 bg-white py-8">
        <Container className="flex flex-col items-center justify-between gap-3 text-sm text-slate-500 sm:flex-row">
          <p>
            © {new Date().getFullYear()} {SITE_NAME}. TR-069 / CWMP Auto Configuration Server.
          </p>
          <p>
            Dibuat oleh{' '}
            <a href={CREDIT_URL} target="_blank" rel="noopener noreferrer" className="font-medium text-slate-600 underline-offset-2 hover:text-indigo-600 hover:underline">
              {CREDIT_NAME}
            </a>
          </p>
        </Container>
      </footer>
    </div>
  )
}
