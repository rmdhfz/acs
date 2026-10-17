// Konten landing page — SATU sumber untuk tampilan (LandingPage.tsx) dan data
// terstruktur JSON-LD (FAQPage). Hanya klaim yang benar-benar didukung sistem;
// tidak ada testimoni, angka pelanggan, atau peringkat buatan.

export const SITE_NAME = 'ACS Console'
export const TAGLINE = 'ACS multi-vendor TR-069 untuk ISP dan operator fiber optik'
export const META_TITLE = 'ACS Console — ACS TR-069 Multi-Vendor & Multi-Tenant untuk ISP'
export const META_DESCRIPTION =
  'Kelola ribuan ONT/ONU/router dari satu konsol berbasis TR-069 (CWMP): provisioning otomatis, firmware canary rollout, preset, multi-tenant, RBAC, audit trail, dan REST API. Mendukung ZTE, Huawei, FiberHome, Nokia, Cdata.'
export const KEYWORDS =
  'ACS, TR-069, CWMP, auto configuration server, manajemen ONT, manajemen ONU, ISP, fiber optik, zero touch provisioning, firmware rollout, multi tenant, GenieACS alternatif'

export interface Feature {
  title: string
  body: string
  icon: 'layers' | 'zap' | 'rocket' | 'shield' | 'plug' | 'upload' | 'eye' | 'building' | 'wifi'
}

export const FEATURES: Feature[] = [
  {
    icon: 'layers',
    title: 'Multi-vendor tanpa ubah kode',
    body: 'Perbedaan path parameter tiap merek ditangani lewat katalog data (vendor, OUI, model, pemetaan parameter), bukan percabangan kode. Katalog memuat 30+ merek (ZTE, Huawei, FiberHome, Nokia, C-Data, TP-Link, Tenda, VSOL, Sagemcom, Zyxel, dan lainnya) beserta ribuan OUI, ditambah pemetaan standar TR-098/TR-181 sebagai cadangan untuk merek yang belum dikenal. Menambah merek baru adalah operasi data. Root TR-098 (InternetGatewayDevice.*) maupun TR-181 (Device.*) ditentukan per perangkat.',
  },
  {
    icon: 'zap',
    title: 'Zero-Touch Provisioning',
    body: 'Aturan berprioritas mengenali ONT baru lewat OUI, serial, model, atau versi firmware lalu menerapkan profil, reboot, atau firmware secara otomatis saat Inform pertama masuk.',
  },
  {
    icon: 'wifi',
    title: 'Preset penegak konfigurasi',
    body: 'Jaga SSID dan parameter standar operator. Bila pelanggan mengubahnya, ACS mengembalikan nilainya pada sesi berikutnya, dengan cooldown dan batas percobaan agar tidak berulang tanpa henti.',
  },
  {
    icon: 'rocket',
    title: 'Firmware canary rollout',
    body: 'Naikkan firmware bertahap per gelombang dengan ambang kegagalan. Rollout berhenti otomatis bila tingkat gagal melewati batas, dan pembatalan ikut menghentikan unduhan yang belum terkirim.',
  },
  {
    icon: 'building',
    title: 'Multi-tenant & RBAC',
    body: 'Satu instalasi untuk banyak perusahaan atau anak usaha. Data terisolasi per tenant, dengan peran Superadmin, Admin, NOC, Viewer, dan portal pelanggan (End User).',
  },
  {
    icon: 'eye',
    title: 'Audit trail & observability',
    body: 'Setiap perubahan tercatat (siapa, kapan, apa). Metrik Prometheus, aturan alert, dan timeline per perangkat membantu NOC menelusuri masalah dengan cepat.',
  },
  {
    icon: 'upload',
    title: 'Impor & ekspor dengan pratinjau',
    body: 'Impor CSV massal dengan pratinjau yang sudah dicek terhadap data eksisting (baru, diperbarui, tidak berubah, error) sebelum dieksekusi. Ekspor CSV/JSON tanpa membocorkan rahasia.',
  },
  {
    icon: 'plug',
    title: 'REST API & webhook',
    body: 'Kontrak OpenAPI 3 lengkap beserta koleksi Postman. Webhook bertanda tangan HMAC dengan retry untuk menghubungkan BSS/OSS dan sistem pemantauan Anda.',
  },
  {
    icon: 'shield',
    title: 'Keamanan sebagai standar',
    body: 'Login email dengan hash Argon2id, verifikasi dua langkah (TOTP), reset password lewat email sekali pakai, pembatasan percobaan, kredensial terenkripsi di database, pencabutan sesi, dan perlindungan SSRF pada Connection Request.',
  },
  {
    icon: 'zap',
    title: 'Diagnostik & redaman optik',
    body: 'Jalankan ping dan traceroute dari ONT, serta baca level optik (RX/TX) berbasis pemetaan data. Hasil tampil terstruktur di Console dan ikut menjadi riwayat perangkat.',
  },
  {
    icon: 'building',
    title: 'Konsol operator SaaS',
    body: 'Laporan platform per tenant (KPI, tren 30 hari, ekspor CSV), masuk-sebagai untuk dukungan yang teraudit, serta batas perangkat dan kuota task per tenant.',
  },
  {
    icon: 'wifi',
    title: 'Portal pelanggan (WiFi Saya)',
    body: 'Pelanggan mengganti SSID dan password WiFi atau me-restart ONT miliknya sendiri. Akses dibatasi pada perangkat yang ditautkan admin, dengan validasi input dan jeda antar-perubahan.',
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
