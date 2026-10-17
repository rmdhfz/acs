// Konten landing page — SATU sumber untuk tampilan (LandingPage.tsx) dan data
// terstruktur JSON-LD (FAQPage). Hanya klaim yang benar-benar didukung sistem;
// tidak ada testimoni, angka pelanggan, atau peringkat buatan.

export const SITE_NAME = 'ACS Console'
export const TAGLINE = 'ACS multi-vendor TR-069 untuk ISP dan operator fiber optik'
export const META_TITLE = 'ACS Console — ACS TR-069 Multi-Vendor & Multi-Tenant untuk ISP'
export const META_DESCRIPTION =
  'Kelola ribuan ONT/ONU/router dari satu konsol TR-069 (CWMP): provisioning otomatis, firmware canary rollout, preset, notifikasi, multi-tenant, RBAC, login Google, audit trail, dan REST API. Katalog 30+ merek: ZTE, Huawei, FiberHome, Nokia, C-Data, TP-Link, VSOL.'
export const KEYWORDS =
  'ACS, TR-069, CWMP, auto configuration server, manajemen ONT, manajemen ONU, ISP, fiber optik, zero touch provisioning, firmware rollout, multi tenant, ACS multi vendor, aplikasi manajemen router ISP, GenieACS alternatif'

export interface Feature {
  title: string
  body: string
  icon: 'layers' | 'zap' | 'rocket' | 'shield' | 'plug' | 'upload' | 'eye' | 'building' | 'wifi' | 'bell' | 'palette'
}

export const FEATURES: Feature[] = [
  {
    icon: 'layers',
    title: 'Multi-vendor tanpa ubah kode',
    body: '30+ merek ONT/router lewat katalog data. Merek baru cukup ditambah sebagai data.',
  },
  {
    icon: 'zap',
    title: 'Zero-Touch Provisioning',
    body: 'ONT baru dikenali otomatis dan langsung dikonfigurasi saat pertama terhubung.',
  },
  {
    icon: 'wifi',
    title: 'Preset penegak konfigurasi',
    body: 'SSID dan parameter standar operator dijaga, dikembalikan bila diubah pelanggan.',
  },
  {
    icon: 'rocket',
    title: 'Firmware canary rollout',
    body: 'Upgrade bertahap per gelombang; berhenti otomatis bila tingkat gagal melewati batas.',
  },
  {
    icon: 'eye',
    title: 'Diagnostik & redaman optik',
    body: 'Ping, traceroute, dan level optik RX/TX langsung dari ONT dengan hasil terstruktur.',
  },
  {
    icon: 'bell',
    title: 'Notifikasi in-app & email',
    body: 'Lonceng realtime plus email untuk perangkat offline massal, task gagal, dan keamanan akun.',
  },
  {
    icon: 'building',
    title: 'Multi-tenant & RBAC',
    body: 'Satu instalasi untuk banyak perusahaan; data terisolasi, peran dari Superadmin sampai Viewer.',
  },
  {
    icon: 'shield',
    title: 'Login aman, termasuk Google',
    body: 'Masuk dengan Google atau email + verifikasi dua langkah (TOTP); audit trail lengkap.',
  },
  {
    icon: 'wifi',
    title: 'Portal pelanggan',
    body: 'Pelanggan mengganti SSID/password WiFi atau restart ONT sendiri, terbatas pada perangkatnya.',
  },
  {
    icon: 'building',
    title: 'Konsol operator SaaS',
    body: 'Laporan per tenant, masuk-sebagai untuk dukungan yang teraudit, batas perangkat per paket.',
  },
  {
    icon: 'upload',
    title: 'Impor & ekspor dengan pratinjau',
    body: 'CSV massal dicek dulu (baru, diperbarui, error) sebelum dieksekusi.',
  },
  {
    icon: 'plug',
    title: 'REST API & webhook',
    body: 'OpenAPI 3 + Postman; webhook bertanda tangan untuk integrasi BSS/OSS.',
  },
]

export interface Step {
  title: string
  body: string
}

export const STEPS: Step[] = [
  { title: 'ONT menghubungi ACS', body: 'ONT yang sudah diarahkan ke URL ACS mengirim Inform (boot, periodik, atau perubahan nilai). ACS memvalidasi kredensial tenant dan mengenali vendor lewat OUI.' },
  { title: 'ACS mengenali dan mengonfigurasi', body: 'Aturan Zero-Touch dan Preset dievaluasi. Task seperti SetParameterValues, Reboot, atau Download dikirim pada sesi yang sama.' },
  { title: 'NOC memantau dan bertindak', body: 'Tim operasional melihat status, parameter, riwayat event, dan hasil task di Console; memicu Connection Request untuk aksi seketika; semuanya tercatat di audit trail.' },
]

export interface Faq {
  q: string
  a: string
}

export const FAQS: Faq[] = [
  {
    q: 'Apakah ACS mengonfigurasi OLT atau ONT?',
    a: 'ACS mengonfigurasi sisi ONT/CPE lewat protokol TR-069 (CWMP): WiFi, akun PPPoE, LAN, reboot, dan upgrade firmware di dalam perangkat pelanggan. OLT (registrasi ONT di port PON, VLAN, profil bandwidth) tetap dikelola lewat sistem manajemen OLT. Lalu lintas TR-069 hanya melintasi OLT sebagai paket IP biasa.',
  },
  {
    q: 'Merek perangkat apa saja yang didukung?',
    a: 'Katalog memuat 30+ merek (ZTE, Huawei, FiberHome, Nokia, C-Data, TP-Link, Tenda, VSOL, Sagemcom, Zyxel, dan lainnya) dengan ribuan OUI dari registry IEEE. Perangkat yang mengikuti standar TR-098/TR-181 tetap bisa dikelola lewat pemetaan standar meski mereknya belum dikenal. Pemetaan awal ini belum diuji pada semua firmware, jadi uji dengan perangkat Anda sebelum rollout; vendor atau model lain dapat ditambahkan sebagai data tanpa rilis perangkat lunak, kecuali vendor itu memiliki kuirk protokol nonstandar.',
  },
  {
    q: 'Bagaimana ONT mengetahui alamat ACS?',
    a: 'Lewat pengaturan TR-069 di ONT (URL, username, password), OMCI dari OLT, DHCP Option 43, atau firmware yang sudah dikustomisasi operator. Setelah itu ONT yang berinisiatif menghubungi ACS, sehingga ACS tidak perlu mengetahui IP ONT lebih dulu.',
  },
  {
    q: 'Apakah mendukung banyak perusahaan dalam satu instalasi?',
    a: 'Ya. ACS Console bersifat multi-tenant: data, pengguna, kredensial CWMP, dan konfigurasi terisolasi per tenant, dengan kontrol akses berbasis peran.',
  },
  {
    q: 'Bagaimana dengan keamanan data dan akses?',
    a: 'Password disimpan dengan Argon2id, verifikasi dua langkah (TOTP) tersedia, token reset sekali pakai dan kedaluwarsa, sesi dicabut saat password atau peran berubah, kredensial perangkat dienkripsi di database, nilai rahasia disamarkan untuk peran non-admin, dan setiap perubahan tercatat di audit trail. Endpoint CWMP sebaiknya dipasang di belakang TLS.',
  },
  {
    q: 'Apakah bisa diintegrasikan dengan BSS/OSS atau sistem pemantauan?',
    a: 'Bisa. Tersedia REST API terdokumentasi (OpenAPI) dengan token API per tenant, webhook bertanda tangan untuk peristiwa seperti kegagalan task dan perubahan nilai, serta metrik Prometheus.',
  },
  {
    q: 'Sudah dipakai di produksi?',
    a: 'Saat ini ACS Console berada pada tahap pengujian penerimaan pengguna (UAT) bersama tim operasional dan jaringan. Hubungi kami untuk jadwal demo dan rencana pilot.',
  },
]

export const NAV = [
  { href: '#fitur', label: 'Fitur' },
  { href: '#cara-kerja', label: 'Cara kerja' },
  { href: '#ont-vs-olt', label: 'ONT vs OLT' },
  { href: '#faq', label: 'FAQ' },
]

/** Data terstruktur JSON-LD untuk halaman beranda. siteUrl tanpa slash akhir. */
export function buildJsonLd(siteUrl: string, contactEmail: string) {
  const org: Record<string, unknown> = {
    '@type': 'Organization',
    '@id': `${siteUrl}/#organization`,
    name: SITE_NAME,
    url: `${siteUrl}/`,
    logo: `${siteUrl}/favicon.svg`,
  }
  if (contactEmail) org.email = contactEmail
  return {
    '@context': 'https://schema.org',
    '@graph': [
      org,
      {
        '@type': 'WebSite',
        '@id': `${siteUrl}/#website`,
        url: `${siteUrl}/`,
        name: SITE_NAME,
        inLanguage: 'id-ID',
        publisher: { '@id': `${siteUrl}/#organization` },
      },
      {
        '@type': 'SoftwareApplication',
        '@id': `${siteUrl}/#software`,
        name: SITE_NAME,
        applicationCategory: 'BusinessApplication',
        applicationSubCategory: 'Network management / TR-069 Auto Configuration Server',
        operatingSystem: 'Web',
        inLanguage: 'id-ID',
        description: META_DESCRIPTION,
        url: `${siteUrl}/`,
        featureList: FEATURES.map((f) => f.title),
        publisher: { '@id': `${siteUrl}/#organization` },
      },
      {
        '@type': 'HowTo',
        '@id': `${siteUrl}/#howto`,
        name: 'Dari ONT menyala sampai terkonfigurasi lewat ACS Console',
        inLanguage: 'id-ID',
        step: STEPS.map((s, i) => ({ '@type': 'HowToStep', position: i + 1, name: s.title, text: s.body })),
      },
      {
        '@type': 'FAQPage',
        '@id': `${siteUrl}/#faq`,
        mainEntity: FAQS.map((f) => ({
          '@type': 'Question',
          name: f.q,
          acceptedAnswer: { '@type': 'Answer', text: f.a },
        })),
      },
    ],
  }
}
