# Connection Request untuk CPE di belakang NAT/CGNAT — Usulan Desain

> Status: **USULAN, belum disetujui, belum ada kode baru** (2026-10-29). Keputusan ini tercatat "belum final" di `CLAUDE.md`, jadi implementasi menunggu
> persetujuan pemilik produk. Dokumen ini menyatakan jujur apa yang sudah ada, apa yang **tidak** diketahui, dan urutan kerja yang mengurangi risiko membangun sesuatu yang tak bisa dibuktikan.

## 1. Kondisi sekarang (diverifikasi dari kode & data, 2026-10-29)

- **Yang ada:** `Service.TriggerConnectionRequest` mencoba HTTP Connection Request ke `ManagementServer.ConnectionRequestURL`; bila gagal dan CPE melaporkan
  `ManagementServer.UDPConnectionRequestAddress`, ia mengirim **satu datagram UDP Connection Request** (TR-069 Amendment, Annex G; HMAC-SHA1) lewat `wakeUpViaUDP`
  dengan penjaga SSRF `netguard` (`internal/usecase/device/udp_connreq.go`). Teruji hanya terhadap penerima uji dan simulator (`internal/simcpe/udpconnreq.go`).
- **Yang TIDAK ada:** server STUN di ACS. Kode menyatakan eksplisit "ACS TIDAK berbicara STUN". Klaim di `docs/pengujian/PENGUJIAN_LAPANGAN.md` §2 bahwa
  ada `stun_client.go` yang mengirim STUN Binding Request **keliru** (berkas itu tidak ada) — dikoreksi bersama dokumen ini.
- **Data lapangan:** ketiga ONT fisik UAT (ZTE F660/F609, Optictimes) hanya memiliki `ConnectionRequestURL` di alamat privat `10.0.x.x` pada parameter yang sudah
  tersinkron. Tidak ada `STUNEnable`/`UDPConnectionRequestAddress`/`NATDetected`. **Ini belum membuktikan ONT tak mendukung** — parameter itu tidak ikut Inform dan
  belum pernah dibaca — tetapi **dukungan TR-111 di armada kita tidak diketahui**. CR HTTP ke `10.x` sudah tersimpan otomatis (`ACS_CR_TRUSTED_CIDRS`); uji kirim CR nyata masih menunggu kredensial CR diisi.
- **Seberapa besar masalahnya tidak diketahui.** Belum ada data berapa persen perangkat Group berada di balik CGNAT sehingga CR HTTP tak terjangkau.

## 2. Masalah

ACS membuka koneksi **masuk** ke CPE. Di belakang CGNAT (atau NAT tanpa port-forward) tidak ada jalur masuk. Fallback saat ini: tugas menunggu Periodic Inform berikutnya
(interval bawaan operator, mis. 43200 dtk di ONT Telkom) — aksi interaktif (ubah WiFi, reboot, diagnostik) terasa "macet" berjam-jam.

## 3. Opsi

| # | Opsi | Kelebihan | Kekurangan |
|---|---|---|---|
| A | Periodic Inform pendek (mis. 5–15 menit) lewat aturan otomasi | Nol kode baru; bekerja di NAT apa pun | Beban sesi ×N (kita ±500–900 sesi/dtk di simulator — lihat benchmark, bukan produksi); tetap tertunda menit; menambah trafik ke CPE |
| B | **ACS menjalankan server STUN (TR-111) + mengirim UDP CR dari soket yang sama** | Standar; CPE yang mendukung langsung bisa dipakai; alamat publik **diamati ACS**, bukan klaim CPE (lebih aman) | Perlu CPE yang benar-benar mendukung TR-111; titik jaringan UDP publik; satu simpul (lihat §5); sulit dibuktikan tanpa CGNAT nyata |
| C | STUN publik (tanpa kode ACS) | Tanpa server baru | CGNAT umumnya membatasi filter per alamat/port sumber — datagram dari ACS (bukan dari server STUN publik) **dibuang**; hampir pasti gagal di CGNAT sungguhan |
| D | XMPP Connection Request (Annex K) | Standar | Butuh server XMPP; jarang diimplementasikan firmware; beban operasional besar |
| E | USP (TR-369) lewat WebSocket/MQTT — CPE yang menjaga koneksi | Menyelesaikan NAT secara tuntas, bukan akal-akalan | Butuh CPE berfirmware USP; kerangka USP kita masih stub; lihat `USP_TR369_DESIGN.md` |

## 4. Rekomendasi — bertahap, bukti dulu

**Jangan membangun B sebelum ada bukti dua hal:** (1) CPE nyata di armada mendukung TR-111, (2) ada CPE di belakang CGNAT/NAT nyata untuk mengujinya.
Kode UDP CR yang ada sekarang sudah berstatus "tidak teruji lapangan"; menambah server STUN di atasnya tanpa CPE uji memperbesar kode yang tak bisa dibuktikan.

- **Langkah 0 (tanpa kode, 1 sesi uji lab):** pada ketiga ONT, jalankan GetParameterValues untuk `*.ManagementServer.STUNEnable`, `STUNServerAddress`, `STUNServerPort`,
  `UDPConnectionRequestAddress`, `NATDetected`, `NATRedirected` (tab Parameter Device Detail atau `POST /devices/{id}/tasks/get-parameter-values`). Hasilnya menjawab "apakah B mungkin" untuk model-model itu.
  Sekalian ukur: berapa perangkat produksi yang CR HTTP-nya tak terjangkau (hasil `TRIGGER_CONNECTION_REQUEST` gagal di `activity_logs` vs berhasil).
- **Langkah 1 (murah, reversibel):** bila sebagian besar perangkat tak terjangkau dan STUN tak didukung → opsi A lewat aturan otomasi per tenant (bukan kode).
- **Langkah 2 (hanya bila Langkah 0 positif):** opsi B sesuai §5.
- **Opsi E** tetap jalur jangka panjang; keputusan USP terpisah.

## 5. Desain opsi B (bila Langkah 0 positif)

1. **Server STUN** pada soket UDP khusus (paket baru `internal/delivery/stun`, delivery layer; tanpa logika bisnis). Menjawab Binding Request dengan alamat sumber teramati
   (XOR-MAPPED-ADDRESS, RFC 5389/8489). Atribut khusus TR-111 (penanda perubahan binding, otentikasi USERNAME/MESSAGE-INTEGRITY) **wajib dibaca langsung dari teks TR-111 Annex A sebelum
   implementasi** — tidak ditulis dari ingatan (pelajaran bug `xsi:type`: spesifikasi yang tampak benar belum tentu diterima firmware).
2. **Soket bersama:** UDP CR dikirim lewat `WriteToUDP` pada **soket STUN yang sama** (IP:port sumber identik dengan tujuan yang diketahui NAT). Sekarang `wakeUpViaUDP` membuka soket
   baru dengan port acak — itu gagal di NAT yang membatasi per port. Ini perubahan perilaku yang harus diuji.
3. **Alamat teramati, bukan klaim CPE:** simpan alamat yang dilihat soket (tidak bisa dipalsukan lewat parameter) dan pakai itu sebagai target UDP CR; abaikan `UDPConnectionRequestAddress`
   dari CPE bila beda. Ini menutup satu vektor SSRF/arah-ulang yang sekarang bergantung pada penjaga `netguard` saja. Penyimpanan: tabel baru `device_nat_bindings` (device_id,
   alamat, diamati_pada, binding_berubah) — tabel state, audit minimal sebagai **pengecualian yang dinyatakan di header migrasi** (CLAUDE.md butir 2). Keputusan skema ini menunggu persetujuan.
4. **Satu simpul pemilik soket.** Filter NAT melekat pada IP:port soket ACS; replika lain yang mengirim dari IP berbeda akan dibuang NAT. Jadi: peran `ACS_STUN_ENABLED` aktif di tepat **satu**
   replika; replika lain mengirim permintaan CR lewat Redis pub/sub ke simpul itu (pola yang sama dengan rencana routing USP). Konsekuensi: titik tunggal untuk CR-UDP (bukan untuk Inform/REST).
5. **Keamanan:** hanya menjawab/menyimpan untuk perangkat yang dikenal (verifikasi integritas dengan kredensial CR milik perangkat); batas laju per sumber; abaikan paket tak terautentikasi
   (jawaban sebesar permintaan, jadi amplifikasi kecil, tetapi tetap dibatasi). Port UDP publik harus dibuka sengaja di firewall/compose — default MATI.
6. **Observabilitas:** metrik `acs_stun_binding_total`, `acs_udp_cr_sent_total{hasil}`; log perubahan binding; panel/alert di Grafana yang ada.
7. **Pengujian:** unit (parser/serializer STUN dengan vektor uji RFC), e2e S38 memakai simulator CPE (tambahan klien STUN di `internal/simcpe`) pada Docker loopback.
   **Batas jujur:** simulator tidak meniru perilaku NAT/CGNAT nyata, jadi e2e hanya membuktikan protokol dan alur; **bukti sebenarnya butuh CPE di balik NAT nyata** — dicatat sebagai
   item uji lapangan, tidak ditandai selesai sebelum itu.

## 6. Keputusan yang dibutuhkan dari pemilik produk

1. Setuju urutan: **Langkah 0 (cek dukungan di ONT lab) dulu**, B hanya bila positif?
2. Apakah infrastruktur dapat membuka UDP publik (mis. 3478) dan menerima **satu simpul** pemilik soket?
3. Apakah ada CPE di balik CGNAT/NAT nyata yang dapat dipakai menguji (tanpa itu B tidak bisa ditutup sebagai "selesai")?
4. Setuju tabel `device_nat_bindings` dengan audit minimal sebagai pengecualian terdokumentasi?
