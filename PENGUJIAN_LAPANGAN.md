# PENGUJIAN_LAPANGAN.md — Rencana Uji Perangkat Fisik (ZTE, FiberHome, Huawei, Nokia, Cdata)

Dokumen kerja untuk sesi uji lapangan dengan CPE sungguhan. Beda dari `VENDOR_ONBOARDING.md` (cara mendaftarkan vendor baru sebagai operasi data) — dokumen ini adalah **checklist eksekusi** untuk hari pengujian itu sendiri: apa yang disiapkan, urutan langkah, apa yang diharapkan berhasil, apa yang **sudah diketahui belum lengkap** (supaya tidak disalahartikan sebagai bug baru saat ditemukan), dan cara melaporkan temuan supaya masuk balik ke `ROADMAP.md`.

**Baca ini sebelum mulai. Status sebelum hari-H: sistem ini BELUM PERNAH menerima Inform dari CPE fisik sama sekali** — satu-satunya validasi sesi CWMP sejauh ini adalah simulasi manual satu event `BOOTSTRAP` dengan data buatan (lihat `ROADMAP.md` Fase 0). Pengujian ini adalah validasi pertama ke dunia nyata — harapkan hal yang tidak terduga, terutama pada nomor 5 (parameter mapping) dan interoperabilitas per-firmware (`PRD.md` §12 sudah eksplisit menyebut ini sebagai risiko).

---

## 1. Prasyarat Sebelum Mulai

- [ ] ACS sudah jalan dan bisa diakses dari jaringan tempat CPE fisik berada (`docker-compose up`, port CWMP **7547** harus reachable dari CPE — kalau CPE di VLAN/segmen berbeda dari server ACS, pastikan routing/firewall mengizinkan port itu SEBELUM mulai, jangan debug ini di tengah sesi uji).
- [ ] Buat (atau pakai) satu **tenant khusus pilot** di Administration > Tenants — jangan pakai tenant produksi yang sudah dipakai data lain, supaya kalau ada yang berantakan gampang dibersihkan tanpa menyentuh data lain.
- [ ] Set shared secret Inform CWMP tenant pilot itu (tombol "Kredensial CWMP" di tabel Tenants) — catat username/password-nya, ini yang akan diisi ke tiap CPE fisik di langkah 3.
- [ ] Siapkan minimal 1 unit CPE per vendor yang mau diuji (ZTE, FiberHome, Huawei, Nokia, Cdata — boleh mulai dari yang paling gampang diakses dulu, tidak harus kelima-limanya di hari yang sama).
- [ ] Siapkan akses admin GUI/CLI tiap CPE (untuk setting ACS URL) dan catat MAC address / serial number-nya sebelum mulai (untuk dicocokkan nanti di daftar Devices).
- [ ] **Backup database dulu** — lihat §7. Kalau pengujian bikin data device/task berantakan, kita bisa restore ke state sebelum pengujian tanpa kehilangan data tenant lain.

## 2. Batasan yang Sudah Diketahui (Baca Supaya Tidak Kaget)

Ini BUKAN daftar lengkap bug potensial — ini yang **sudah kita tahu sebelumnya** akan terjadi/tidak berfungsi, supaya waktu ketemu di lapangan tidak dianggap temuan baru:

- **`vendor_ouis` KOSONG untuk kelima vendor** (migrasi `0004_vendor_baseline_catalog`) — setiap device baru yang Inform pasti muncul dengan `vendor_id` kosong/"Vendor belum diketahui" di langkah pertama, **ini SELALU terjadi, bukan kegagalan**. Segera tambahkan OUI device yang baru connect lewat Catalog Vendor (`VENDOR_ONBOARDING.md` §2) begitu ketemu MAC address-nya. Device yang sudah kadung dibuat TIDAK otomatis "sembuh" ke vendor yang benar begitu OUI ditambahkan belakangan — assign `vendor_id` manual di device itu kalau perlu.
- **Logical key yang SUDAH dipetakan** (berlaku sama persis di kelima vendor — TR-098 & TR-181, lihat migrasi `0004`): `device.manufacturer`, `device.model_name`, `device.serial_number`, `device.hardware_version`, `device.software_version`, `device.uptime`, `device.periodic_inform_interval`, `device.connection_request_url`, `wifi.ssid`, `wifi.wpa_passphrase`, `wan.pppoe.username`, `wan.pppoe.password`. Ini semua asumsi instance pertama (`.1`) untuk WAN/WiFi/PPP — konvensi paling umum di CPE residensial single-WAN/single-radio, TAPI bukan jaminan mutlak spec untuk device tertentu.
- **Logical key yang SENGAJA BELUM dipetakan (jangan dianggap bug kalau tidak ada di datalist provisioning profile):**
  - `wifi.5g.ssid` / split SSID 2.4G vs 5G — index radio WLAN tidak distandarkan, beda per vendor/model/firmware.
  - `wan.ip_address` — ambigu antara `WANIPConnection` vs `WANPPPConnection` (TR-098) atau index `Device.IP.Interface` mana yang WAN (TR-181), tanpa tahu tipe koneksi device nyata.
  - **Optical power (redaman RX/TX)** — SELALU vendor extension nonstandar (pola `X_<OUI>_...`), tidak ada di spec dasar. **Ini bukan blocker** untuk uji dasar — WiFi/PPPoE/info device tetap jalan karena itu yang distandarkan spec.
  - Kalau menemukan path yang benar untuk salah satu di atas dari device fisik yang diuji, catat di §6 untuk ditambahkan ke `vendor_parameter_mappings` (via `device_model_id` spesifik, bukan generik vendor, supaya tidak menimpa asumsi default utk model lain).
- **Cdata paling minim tervalidasi dari 5 vendor ini** — baru ditambahkan sebagai data starting point, belum ada riwayat penggunaan sama sekali sebelum migrasi ini (4 vendor lain setidaknya sudah ada sejak baseline awal).
- **Dark mode dan tampilan mobile belum pernah dicek visual di browser sungguhan** — kalau lihat ada yang aneh secara visual selagi buka Devices/Dashboard buat monitor pengujian, itu kemungkinan besar memang gap yang sudah tercatat di `ROADMAP.md` Fase 1, bukan akibat dari pengujian device.
- **STUN/CGNAT: ada fallback TR-111 UDP, tapi BELUM teruji lapangan** — `internal/usecase/device/service.go` mencoba HTTP Connection Request dulu, lalu fallback ke STUN Binding Request UDP (TR-111, `stun_client.go`) bila alamat `UDPConnectionRequestAddress` ada di Inform. Ini belum pernah diuji ke CGNAT nyata. Kalau CPE di belakang NAT berlapis dan Connection Request gagal, itu **belum tentu bug** — catat saja (butuh CPE yang benar-benar mengirim `UDPConnectionRequestAddress` + firewall yang mengizinkan UDP balik).

### Fitur yang ditambahkan SETELAH dokumen ini dibuat (2026-08-23) — supaya tidak kaget saat uji

- **Migrasi sekarang 0001→0021** (bukan 0020). `docker compose up` menjalankan semua.
- **Engine preset (migrations/0021)** dievaluasi **tiap Inform**. Preset dengan `enforce=1` yang cocok akan **otomatis mengantre `SetParameterValues`** ke device saat nilainya menyimpang (drift-heal). Di tenant pilot yang bersih **tidak ada preset**, jadi tidak akan terjadi — tapi kalau kamu iseng buat preset enforce lalu lihat task muncul sendiri, **itu memang perilakunya**, bukan bug. Kelola di menu **Presets** (role ADMIN).
- **ZTP trigger per-Inform** — Zero-Touch Rule sekarang bisa di-set trigger `EVERY_INFORM` (bukan cuma `BOOTSTRAP_ONLY`). Rule begitu dievaluasi tiap Periodic Inform — kalau kamu buat ZT rule saat uji, perhatikan trigger-nya.
- **Session reaper** — sesi CWMP `status='OPEN'` yang CPE-nya berhenti tanpa POST-kosong penutup otomatis jadi `status='TIMEOUT'` setelah 15 menit (sweeper `cmd/acsd`). Melihat `TIMEOUT` di `/cwmp/sessions` **normal**, bukan error protokol.
- **Portal self-service (role ENDUSER)** — `/self-service/*`, tidak relevan untuk uji CWMP tapi ada di router.
- **Device ↔ tag** — bisa tag device di Device Detail / filter `GET /devices?tag_id=`.

## 3. Setting ACS URL di CPE (Umum, Semua Vendor)

Field-nya standar TR-069 (`ManagementServer.URL`/`ConnectionRequestUsername` dkk di spec, cuma lokasi menu GUI beda per vendor):

| Field | Isi dengan |
|---|---|
| ACS URL | `http://<alamat-server-ACS>:7547/cwmp` (sesuaikan skema/port dgn `docker-compose.yml` — default `17547` kalau akses dari luar Docker network, `7547` dari dalam) |
| ACS Username | shared secret Inform tenant pilot (langkah 1) |
| ACS Password | shared secret Inform tenant pilot (langkah 1) |
| Periodic Inform | aktifkan, interval bebas (mis. 300 detik) untuk pengujian supaya tidak perlu reboot device tiap mau lihat Inform baru |
| Connection Request Username/Password | isi bebas per device (dicatat), ini dipakai ACS untuk connection request BALIK ke device — beda arah dari ACS URL |

Lokasi menu di tiap vendor beda-beda (biasanya di bawah "WAN"/"Remote Management"/"TR-069"/"CWMP" di GUI admin CPE) — kalau tidak ketemu, cek manual vendor masing-masing, di luar scope dokumen ini.

## 4. Urutan Uji per Device (Ulangi untuk Tiap Unit)

Checklist per device — centang satu-satu, jangan lompat ke langkah berikutnya kalau langkah sebelumnya gagal (supaya jelas di titik mana masalahnya):

1. **[ ] Koneksi pertama (Inform)** — set ACS URL di CPE (§3), tunggu Inform pertama (biasanya langsung/dalam hitungan detik setelah setting disimpan, kalau tidak coba reboot CPE untuk memicu event `1 BOOT`). Cek di **Devices** (scope ke tenant pilot) — device baru harus muncul dengan serial number/MAC yang cocok dengan yang dicatat di §1.
2. **[ ] Vendor teridentifikasi benar** — kolom vendor di Device Detail bukan "Vendor belum diketahui" (lihat §2 kalau gagal — kemungkinan OUI belum terdaftar, bukan bug protokol).
3. **[ ] Data model version benar** — cek Device Detail apakah ke-resolve sebagai TR-098 (`InternetGatewayDevice.*`) atau TR-181 (`Device.*`) sesuai spek device itu.
4. **[ ] Event tercatat** — tab "Histori Event" di Device Detail menunjukkan event CWMP yang baru terjadi (`0 BOOTSTRAP`/`1 BOOT`/`2 PERIODIC`) dengan kode event PERSIS sesuai spec (jangan sampai ada transformasi string yang salah).
5. **[ ] Parameter dasar terbaca** — coba `GetParameterValues` (lewat task/diagnostics UI yang sudah ada) untuk parameter umum: WiFi SSID, WAN IP, firmware version, uptime. Ini pakai path standar TR-098/TR-181 yang SEHARUSNYA sudah benar di semua vendor (lihat §2 soal apa yang TIDAK diharapkan berhasil).
6. **[ ] Parameter dasar bisa ditulis** — buat satu Provisioning Profile kecil (WiFi SSID saja dulu, jangan langsung PPPoE credentials pelanggan asli) untuk vendor ini, apply ke device, verifikasi SSID di CPE beneran berubah (cek dari sisi device, bukan cuma status task "selesai" di ACS — status "selesai" di ACS berarti CPE meng-ACK `SetParameterValues`, TAPI itu tidak 100% jaminan device benar-benar menerapkan reload config-nya dengan benar, terutama untuk parameter yang di sebagian firmware butuh reboot).
7. **[ ] Task queue & retry** — kalau ada kesempatan, matikan CPE di tengah task berjalan lalu nyalakan lagi, verifikasi task retry (bukan langsung dianggap gagal permanen) — ini menguji `internal/usecase/task` di kondisi nyata, bukan cuma unit test simulasi.
8. **[ ] Connection Request (ACS -> CPE)** — coba trigger aksi yang butuh ACS menghubungi CPE duluan (bukan cuma menunggu Periodic Inform), verifikasi berhasil kalau device ada di LAN yang sama/reachable (lihat catatan STUN/CGNAT di §2 kalau device di belakang NAT).
9. **[ ] Reboot command** — kirim task reboot, verifikasi device benar-benar reboot dan Inform lagi dengan event `1 BOOT` setelah nyala.
10. **[ ] (Opsional, hati-hati) Redaman optik** — kalau device adalah ONT/ONU GPON dan kebetulan vendor ini SUDAH punya mapping optical power terkonfirmasi (cek dulu ke migrasi data vendor terbaru), cek tab "Redaman Optik" di Dashboard/Device Detail. Kalau belum ada mapping-nya, skip — ini memang belum diisi (§2), catat model+firmware persisnya kalau mau ditambahkan mapping-nya nanti berdasarkan dokumentasi vendor.

## 5. Yang SEBAIKNYA TIDAK Dicoba Hari Itu (Risiko Terlalu Tinggi untuk Uji Pertama)

- **Firmware upgrade job ke device produksi/pelanggan sungguhan** — kalau mau uji fitur ini, pakai unit CPE yang benar-benar boleh gagal/brick, jangan unit yang sedang dipakai pelanggan aktif. Belum ada uji firmware upgrade end-to-end ke device fisik manapun sejauh ini.
- **Push PPPoE credentials pelanggan asli** — kalau menguji parameter write PPPoE, pakai kredensial dummy/test dulu, bukan kredensial pelanggan aktif, sampai yakin path parameternya benar-benar cocok dengan firmware device tsb (variasi antar-firmware adalah risiko eksplisit di `PRD.md` §12).
- **Bulk actions ke banyak device sekaligus** — untuk sesi uji pertama, lakukan satu-satu dulu supaya kalau ada yang salah, dampaknya kebaca jelas device mana yang bermasalah.

## 6. Yang Perlu Dicatat & Dilaporkan

Untuk tiap device yang diuji, catat (spreadsheet/notes bebas, tidak perlu format khusus):
- Vendor, model persis, versi firmware.
- Hasil tiap nomor checklist §4 (berhasil/gagal/skip, dengan catatan kalau gagal).
- Path TR-069 yang ternyata BEDA dari yang sudah dipetakan (kalau ketemu, ini bahan update `vendor_parameter_mappings` — lihat `VENDOR_ONBOARDING.md`).
- OUI yang belum terdaftar (kalau ketemu vendor "belum diketahui" padahal seharusnya dikenal).
- Screenshot/log kalau ada perilaku yang benar-benar tidak terduga (bukan yang sudah dicatat di §2).

Setelah sesi selesai, hasil ini dipakai untuk update `ROADMAP.md` Fase 3 item "Uji kompatibilitas riil terhadap firmware CPE..." — jangan ditandai selesai kalau baru sebagian vendor/skenario yang tercoba, catat persis apa yang sudah vs belum diuji (pola yang sama seperti entry lain di ROADMAP.md — jujur soal cakupan, bukan klaim blanket "sudah diuji").

## 7. Backup Sebelum Mulai (Safety Net)

Jalankan sebelum sesi pengujian dimulai:

```bash
docker compose exec mariadb sh -c 'mariadb-dump -uacs -p"$MARIADB_PASSWORD" acs' > backup_sebelum_uji_lapangan_$(date +%Y%m%d).sql
```

Kalau pengujian bikin data berantakan dan perlu restore:

```bash
docker compose exec -T mariadb sh -c 'mariadb -uacs -p"$MARIADB_PASSWORD" acs' < backup_sebelum_uji_lapangan_YYYYMMDD.sql
```

Prosedur ini sudah diuji end-to-end (bukan cuma didokumentasikan) — lihat `ROADMAP.md` Fase 3 "Backup/disaster recovery MariaDB".
