import type { ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { ArrowLeft } from 'lucide-react'

// DRAF TEMPLAT — bukan nasihat hukum. Wajib ditinjau penasihat hukum dan dilengkapi (nama badan hukum, alamat, kontak,
// yurisdiksi, SLA) sebelum dipakai untuk layanan berbayar/publik. Bagian bertanda [ISI: ...] sengaja dibiarkan kosong.
const CONTACT = (import.meta.env.VITE_CONTACT_EMAIL as string | undefined) || '[ISI: email kontak]'

function LegalLayout({ title, updated, children }: { title: string; updated: string; children: ReactNode }) {
  return (
    <div className="min-h-screen bg-white px-4 py-10 dark:bg-slate-950">
      <article className="mx-auto max-w-3xl text-slate-800 dark:text-slate-200">
        <Link to="/signup" className="mb-6 inline-flex items-center gap-1.5 text-sm font-medium text-slate-600 hover:text-slate-900 dark:text-slate-400 dark:hover:text-slate-100">
          <ArrowLeft className="h-4 w-4" /> Kembali
        </Link>
        <div role="note" className="mb-6 rounded-lg border border-amber-400 bg-amber-50 px-4 py-3 text-sm text-amber-950 dark:border-amber-500/50 dark:bg-amber-500/10 dark:text-amber-100">
          <strong>Draf template.</strong> Dokumen ini belum ditinjau penasihat hukum. Bagian bertanda <code>[ISI: …]</code> harus dilengkapi penyelenggara layanan sebelum dipakai secara resmi.
        </div>
        <h1 className="text-3xl font-semibold tracking-tight text-slate-900 dark:text-slate-100">{title}</h1>
        <p className="mt-1 text-sm text-slate-600 dark:text-slate-400">Terakhir diperbarui: {updated}</p>
        <div className="mt-6 space-y-6 text-sm leading-relaxed [&_h2]:mb-2 [&_h2]:text-lg [&_h2]:font-semibold [&_h2]:text-slate-900 dark:[&_h2]:text-slate-100 [&_ul]:list-disc [&_ul]:space-y-1 [&_ul]:pl-5">
          {children}
        </div>
      </article>
    </div>
  )
}

export function TermsPage() {
  return (
    <LegalLayout title="Ketentuan Penggunaan" updated="17 Oktober 2026">
      <section>
        <h2>1. Layanan</h2>
        <p>
          ACS Console adalah layanan manajemen perangkat jaringan (ONT/ONU/router) berbasis TR-069/CWMP yang diselenggarakan oleh [ISI: nama badan hukum]
          (&ldquo;Penyelenggara&rdquo;). Dengan mendaftar, Anda (&ldquo;Pelanggan&rdquo;) menyetujui ketentuan ini atas nama organisasi Anda.
        </p>
      </section>
      <section>
        <h2>2. Akun dan keamanan</h2>
        <ul>
          <li>Anda bertanggung jawab atas kerahasiaan kredensial, termasuk faktor kedua (MFA) dan token API yang Anda terbitkan.</li>
          <li>Anda menjamin berwenang atas perangkat yang Anda daftarkan dan atas data pelanggan akhir Anda.</li>
          <li>Laporkan segera dugaan akses tidak sah ke {CONTACT}.</li>
        </ul>
      </section>
      <section>
        <h2>3. Penggunaan yang dilarang</h2>
        <ul>
          <li>Mengakses data atau perangkat tenant lain, atau mencoba melewati pembatasan akses, kuota, dan batas laju.</li>
          <li>Menggunakan layanan untuk menyerang pihak lain (mis. Connection Request/webhook ke sistem yang bukan milik Anda).</li>
          <li>Mengunggah firmware atau konfigurasi yang Anda tidak berhak distribusikan, atau yang berbahaya bagi perangkat pelanggan.</li>
        </ul>
      </section>
      <section>
        <h2>4. Paket, batas, dan perubahan</h2>
        <p>Paket awal memiliki batas jumlah perangkat dan antrean tugas. Batas dapat diubah Penyelenggara; harga dan penagihan: [ISI: kebijakan harga].</p>
      </section>
      <section>
        <h2>5. Ketersediaan dan tanggung jawab</h2>
        <p>
          Layanan diberikan &ldquo;sebagaimana adanya&rdquo;. Target ketersediaan/SLA: [ISI: SLA]. Perubahan konfigurasi perangkat (termasuk reboot, perubahan WiFi, firmware)
          dijalankan atas perintah Pelanggan; Pelanggan bertanggung jawab atas dampaknya terhadap layanan pelanggan akhirnya. Batas tanggung jawab: [ISI: pasal hukum].
        </p>
      </section>
      <section>
        <h2>6. Penghentian</h2>
        <p>Pelanggan dapat berhenti kapan saja; Penyelenggara dapat menangguhkan akun yang melanggar ketentuan ini. Penghapusan data setelah penghentian: lihat Kebijakan Privasi.</p>
      </section>
      <section>
        <h2>7. Hukum yang berlaku</h2>
        <p>[ISI: yurisdiksi dan penyelesaian sengketa]. Kontak: {CONTACT}.</p>
      </section>
    </LegalLayout>
  )
}

export function PrivacyPage() {
  return (
    <LegalLayout title="Kebijakan Privasi" updated="17 Oktober 2026">
      <section>
        <h2>1. Data yang kami proses</h2>
        <ul>
          <li><strong>Akun:</strong> nama, email, password (disimpan hanya sebagai hash Argon2id), peran, aktivitas login.</li>
          <li><strong>Perangkat:</strong> serial number, OUI/model, versi firmware, alamat IP, parameter konfigurasi dan metrik yang dilaporkan perangkat (mis. redaman optik).</li>
          <li><strong>Operasional:</strong> riwayat sesi/event, task, dan jejak audit (siapa mengubah apa, kapan, dari IP mana).</li>
        </ul>
      </section>
      <section>
        <h2>2. Tujuan</h2>
        <p>Menyediakan layanan manajemen perangkat, keamanan akun, penagihan/kuota, dan pemecahan masalah. Kami tidak menjual data Anda.</p>
      </section>
      <section>
        <h2>3. Perlindungan</h2>
        <ul>
          <li>Kredensial perangkat, rahasia MFA, dan nilai rahasia konfigurasi (password PPPoE/WiFi) dienkripsi saat disimpan (AES-256-GCM).</li>
          <li>Isolasi data antar tenant; akses staf platform tercatat di audit trail, termasuk sesi dukungan (impersonate) yang berbatas waktu.</li>
          <li>Koneksi ke Console dan CPE dianjurkan memakai TLS.</li>
        </ul>
      </section>
      <section>
        <h2>4. Retensi</h2>
        <p>Log operasional dipangkas otomatis setelah [ISI: default 90 hari]; jejak audit disimpan lebih lama. Setelah akun ditutup, data dihapus dalam [ISI: jangka waktu].</p>
      </section>
      <section>
        <h2>5. Pemroses/pihak ketiga</h2>
        <p>Penyedia infrastruktur dan email: [ISI: daftar sub-pemroses dan lokasi data].</p>
      </section>
      <section>
        <h2>6. Hak Anda</h2>
        <p>Anda dapat meminta akses, koreksi, atau penghapusan data pribadi melalui {CONTACT}. Pelanggan yang mengelola data pelanggan akhir bertindak sebagai pengendali data atas data tersebut; Penyelenggara bertindak sebagai pemroses.</p>
      </section>
    </LegalLayout>
  )
}
