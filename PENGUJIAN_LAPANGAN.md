# PENGUJIAN_LAPANGAN.md — Rencana Uji Perangkat Fisik (ZTE, FiberHome, Huawei, Nokia, Cdata)

Dokumen kerja untuk sesi uji lapangan dengan CPE sungguhan. Beda dari `VENDOR_ONBOARDING.md` (cara mendaftarkan vendor baru sebagai operasi data) — dokumen ini adalah **checklist eksekusi** untuk hari pengujian itu sendiri: apa yang disiapkan, urutan langkah, apa yang diharapkan berhasil, apa yang **sudah diketahui belum lengkap** (supaya tidak disalahartikan sebagai bug baru saat ditemukan), dan cara melaporkan temuan supaya masuk balik ke `ROADMAP.md`.

**Baca ini sebelum mulai. Status sebelum hari-H: sistem ini BELUM PERNAH menerima Inform dari CPE fisik sama sekali** — validasi sejauh ini hanya lewat **simulator CPE** (`internal/simcpe`; 10 skenario e2e lulus, lihat `PENGUJIAN_E2E.md`). Simulator tidak menjalankan firmware vendor nyata dan tidak punya kuirk per-firmware, jadi bukti "alur ACS benar" ada, tetapi bukti "cocok dengan perangkat nyata" nol. Pengujian ini adalah validasi pertama ke dunia nyata — harapkan hal yang tidak terduga, terutama pada nomor 5 (parameter mapping) dan interoperabilitas per-firmware (`PRD.md` §12 sudah eksplisit menyebut ini sebagai risiko).

---

## 1. Prasyarat Sebelum Mulai

- [ ] ACS sudah jalan dan bisa diakses dari jaringan tempat CPE fisik berada (`docker-compose up`, port CWMP **7547** harus reachable dari CPE — kalau CPE di VLAN/segmen berbeda dari server ACS, pastikan routing/firewall mengizinkan port itu SEBELUM mulai, jangan debug ini di tengah sesi uji).
- [ ] Buat (atau pakai) satu **tenant khusus pilot** di Administration > Tenants — jangan pakai tenant produksi yang sudah dipakai data lain, supaya kalau ada yang berantakan gampang dibersihkan tanpa menyentuh data lain.
- [ ] Set shared secret Inform CWMP tenant pilot itu (tombol "Kredensial CWMP" di tabel Tenants) — catat username/password-nya, ini yang akan diisi ke tiap CPE fisik di langkah 3.
- [ ] Siapkan minimal 1 unit CPE per vendor yang mau diuji (ZTE, FiberHome, Huawei, Nokia, Cdata — boleh mulai dari yang paling gampang diakses dulu, tidak harus kelima-limanya di hari yang sama).
- [ ] Siapkan akses admin GUI/CLI tiap CPE (untuk setting ACS URL) dan catat MAC address / serial number-nya sebelum mulai (untuk dicocokkan nanti di daftar Devices).
- [ ] **Gladi jaringan sudah lulus** — jalankan §3a dari segmen jaringan yang sama dengan CPE (bukan dari mesin ACS) SEBELUM hari-H. Ini menyingkirkan penyebab kegagalan terbesar (routing/firewall/kredensial) tanpa perlu CPE.
- [ ] **Putuskan HTTP vs HTTPS untuk ACS URL.** `acsd` hanya melayani HTTP polos di :7547 (tidak ada konfigurasi TLS di `cmd/acsd`); shared secret Inform lewat Basic Auth tanpa enkripsi. Untuk LAN uji tertutup: OK. Jika firmware CPE mewajibkan `https://`, perlu TLS terminator (reverse proxy) di depan ACS — belum ada di `docker-compose.yml`. Tanyakan ke tim network sebelum hari-H.
- [ ] Rencanakan **urutan nyala CPE bergantian** (lihat batasan rate limiter di §2) — jangan menyalakan semua unit sekaligus.
- [ ] **Backup database dulu** — lihat §7. Kalau pengujian bikin data device/task berantakan, kita bisa restore ke state sebelum pengujian tanpa kehilangan data tenant lain.

## 2. Batasan yang Sudah Diketahui (Baca Supaya Tidak Kaget)

Ini BUKAN daftar lengkap bug potensial — ini yang **sudah kita tahu sebelumnya** akan terjadi/tidak berfungsi, supaya waktu ketemu di lapangan tidak dianggap temuan baru:

- **`vendor_ouis` sudah di-seed** (migrasi `0022`: 3.176 blok MA-L dari registry IEEE — ZTE 502, Huawei 2.088, FiberHome 220, Nokia 358, Cdata 8; sumber, cakupan & pengecualian ada di header file migrasinya). Device dari OUI-OUI itu langsung ter-resolve `vendor_id`-nya. **Tetap mungkin muncul "Vendor belum diketahui"** karena: (a) OUI di `DeviceId` TR-069 adalah OUI produsen dan tidak selalu sama dengan OUI MAC unit; (b) blok MA-M/MA-S (28/36-bit) sengaja tidak di-seed; (c) CPE white-label sering memakai OUI pihak lain. Kalau muncul, tambahkan OUI-nya lewat Catalog Vendor (`VENDOR_ONBOARDING.md` §2). **Device yang sudah terlanjur dibuat tanpa vendor kini sembuh sendiri pada Inform berikutnya** setelah OUI terdaftar — hanya mengisi yang masih kosong, assign manual operator tidak pernah ditimpa; tidak perlu assign manual lagi. `device_model_id` baru terisi bila ada model di katalog dengan `product_class` yang PERSIS sama dengan yang dikirim CPE (daftarkan model setelah Inform pertama menunjukkan ProductClass-nya). **OUI salah ketik** bisa dihapus lewat API (`DELETE /vendors/{id}/ouis/{ouiId}`) dan didaftarkan ulang (baris lama dihidupkan kembali — dibuktikan e2e S11). UI belum punya tombol hapus OUI; pakai API/Postman.
- **Logical key yang SUDAH dipetakan** (berlaku sama persis di kelima vendor — TR-098 & TR-181, lihat migrasi `0004`): `device.manufacturer`, `device.model_name`, `device.serial_number`, `device.hardware_version`, `device.software_version`, `device.uptime`, `device.periodic_inform_interval`, `device.connection_request_url`, `wifi.ssid`, `wifi.wpa_passphrase`, `wan.pppoe.username`, `wan.pppoe.password`. Ini semua asumsi instance pertama (`.1`) untuk WAN/WiFi/PPP — konvensi paling umum di CPE residensial single-WAN/single-radio, TAPI bukan jaminan mutlak spec untuk device tertentu.
- **Logical key yang SENGAJA BELUM dipetakan (jangan dianggap bug kalau tidak ada di datalist provisioning profile):**
  - `wifi.5g.ssid` / split SSID 2.4G vs 5G — index radio WLAN tidak distandarkan, beda per vendor/model/firmware.
  - `wan.ip_address` — ambigu antara `WANIPConnection` vs `WANPPPConnection` (TR-098) atau index `Device.IP.Interface` mana yang WAN (TR-181), tanpa tahu tipe koneksi device nyata.
  - **Optical power (redaman RX/TX)** — SELALU vendor extension nonstandar (pola `X_<OUI>_...`), tidak ada di spec dasar. **Ini bukan blocker** untuk uji dasar — WiFi/PPPoE/info device tetap jalan karena itu yang distandarkan spec.
  - Kalau menemukan path yang benar untuk salah satu di atas dari device fisik yang diuji, catat di §6 untuk ditambahkan ke `vendor_parameter_mappings` (via `device_model_id` spesifik, bukan generik vendor, supaya tidak menimpa asumsi default utk model lain).
- **Cdata paling minim tervalidasi dari 5 vendor ini** — baru ditambahkan sebagai data starting point, belum ada riwayat penggunaan sama sekali sebelum migrasi ini (4 vendor lain setidaknya sudah ada sejak baseline awal).
- **Dark mode dan tampilan mobile**: status verifikasi visual ada di `ROADMAP.md` Fase 1 (catatan keterbatasan jujur di sana). Kalau ada yang aneh secara visual selagi memantau pengujian, kemungkinan besar itu gap yang sudah tercatat, bukan akibat dari pengujian device.
- **STUN/CGNAT: ada fallback TR-111 UDP, tapi BELUM teruji lapangan** — `internal/usecase/device/service.go` mencoba HTTP Connection Request dulu, lalu fallback ke STUN Binding Request UDP (TR-111, `stun_client.go`) bila alamat `UDPConnectionRequestAddress` ada di Inform. Ini belum pernah diuji ke CGNAT nyata. Kalau CPE di belakang NAT berlapis dan Connection Request gagal, itu **belum tentu bug** — catat saja (butuh CPE yang benar-benar mengirim `UDPConnectionRequestAddress` + firewall yang mengizinkan UDP balik).
- **Rate limiter `/cwmp`: 5 req/s per IP sumber (burst 5), in-memory** (`cmd/acsd/main.go`). Satu sesi CWMP = beberapa POST (Inform, balasan RPC, POST kosong penutup). Beberapa CPE di balik **satu IP publik** (NAT lab/kantor) yang boot bersamaan bisa saling memicu **HTTP 429** → Inform ditolak; CPE akan mencoba lagi sendiri. Ini **bukan bug baru** — ini blocker skala B4 di `LAPORAN_KESIAPAN.md` §5.2. Mitigasi hari-H: nyalakan unit bergantian (jeda ±10 detik) dan jangan menyimpulkan "gagal" dari satu 429 — tunggu retry berikutnya.
- **CWMP tanpa TLS di ACS** (lihat §1): kredensial Inform terkirim polos. Jangan uji lewat internet publik tanpa TLS terminator.
- **Kredensial salah → HTTP 401 + `WWW-Authenticate: Basic realm="ACS"`**; POST kosong tanpa kredensial → **204** (normal, itu penutup sesi). Membedakan 401 / 429 / timeout adalah cara tercepat menentukan sumber masalah — lihat tabel di §8.

### Fitur yang ditambahkan SETELAH dokumen ini dibuat (2026-08-23) — supaya tidak kaget saat uji

- **Migrasi sekarang 0001→0022** (0021 = engine preset, 0022 = seed OUI vendor). `docker compose up` menjalankan semua.
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

### 3a. Gladi Jaringan — WAJIB Sebelum Hari-H (tanpa CPE fisik)

Tujuan: membuktikan jalur **segmen-CPE → ACS** (dan sebaliknya) terbuka, kredensial tenant benar, dan ACS merespons, memakai simulator CPE. Jalankan dari **mesin di segmen jaringan yang sama dengan CPE nanti**, bukan dari server ACS. Alur ini sudah dilatih pada stack lokal 2026-10-15 (Inform + self-heal OUI berjalan); **jaringan antar-mesin sungguhan belum**, itu tujuan gladi ini.

**Langkah 1 — jangkauan (10 detik, tanpa kredensial).** Satu POST kosong ke `/cwmp` harus dibalas **204** dengan header `X-Ratelimit-Limit`:

```bash
curl -i -X POST http://<alamat-ACS>:7547/cwmp          # Linux/macOS/Git-Bash
```
```powershell
(Invoke-WebRequest -Method Post -UseBasicParsing http://<alamat-ACS>:7547/cwmp).StatusCode   # harus 204
```

**Langkah 2 — Inform lengkap dengan kredensial tenant pilot** (simulator `cmd/cpesim`; build di mesin ber-Go: `go build -o cpesim ./cmd/cpesim`, atau set `GOOS=windows|linux`):

```bash
./cpesim -acs http://<alamat-ACS>:7547/cwmp -user <cwmp_user_tenant_pilot> -pass <cwmp_pass_tenant_pilot> \
         -vendor zte -serial GLADI-001 -events "0 BOOTSTRAP" -v
```

Hasil yang diharapkan: `[OK] zte GLADI-001 ... inform=200`. Lalu cek **Devices** (scope tenant pilot): `GLADI-001` muncul dengan event `0 BOOTSTRAP`. Hapus device uji itu sesudahnya agar daftar tenant pilot bersih sebelum CPE asli masuk.

**Langkah 3 — Connection Request balik (ACS → segmen CPE)**, jalur yang paling sering diblok firewall:

```bash
./cpesim -acs ... -user ... -pass ... -vendor huawei -serial GLADI-002 -conn-req -cr-host <IP-mesin-ini> -cr-port 7548
```

Biarkan proses cpesim berjalan (ia menunggu; Ctrl+C untuk berhenti). Dari ACS: trigger Connection Request ke `GLADI-002` (Device Detail). Berhasil bila cpesim mencetak baris `[conn-req -> Inform]` dan di ACS muncul sesi baru dengan event `6 CONNECTION REQUEST`. Gagal → port `-cr-port` mesin ini tidak terjangkau dari ACS (firewall/NAT) — persis yang akan menimpa CPE fisik. Catatan: ACS hanya menyimpan `ConnectionRequestURL` bila host-nya sama dengan IP sumber Inform (anti-SSRF), jadi `-cr-host` harus IP asli mesin ini sebagaimana terlihat oleh ACS (bukan IP di balik NAT yang berbeda).

**Langkah 4 — beban kecil realistis:** `-fleet -devices 3 -cycles 2 -concurrency 1` dari satu IP. `-concurrency 4` dari satu IP justru bisa memicu 429 (§2) — itu perilaku rate limiter, bukan kegagalan jaringan.

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

### Template lembar hasil per device (salin ke spreadsheet)

| Kolom | Isi |
|---|---|
| Tanggal / penguji | |
| Vendor · model persis · versi firmware | |
| Serial · MAC · OUI di DeviceId (dari log Inform) · ProductClass | |
| §4-1 Inform pertama muncul di Devices | ✅ / ❌ / skip + catatan |
| §4-2 Vendor teridentifikasi | |
| §4-3 Data model (TR-098 / TR-181) | |
| §4-4 Event tercatat (kode persis) | |
| §4-5 GetParameterValues dasar | |
| §4-6 Set SSID (dicek dari sisi CPE) | |
| §4-7 Retry task saat CPE mati | |
| §4-8 Connection Request | |
| §4-9 Reboot → event `1 BOOT` | |
| §4-10 Redaman optik (bila relevan) | |
| Path TR-069 yang BEDA dari mapping | |
| OUI baru yang perlu didaftarkan | |
| Screenshot / log (nama file) | |

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

---

## 8. Gejala di Lapangan → Kemungkinan Penyebab (urut dari yang paling cepat dicek)

Skill `/diagnose "<gejala>"` memakai dokumen ini sebagai daftar batasan yang sudah diketahui.

| Gejala | Kemungkinan penyebab | Cek / tindakan |
|---|---|---|
| CPE tidak pernah muncul di Devices, tidak ada Inform di log ACS | Jalur jaringan/ACS URL salah; CPE belum Inform | Ulangi §3a langkah 1 dari segmen CPE; cek ACS URL (skema, port 7547/17547, path `/cwmp`); reboot CPE untuk memicu `1 BOOT` |
| CPE log "HTTP 401" | Username/password ACS di CPE ≠ shared secret tenant | Samakan dengan "Kredensial CWMP" tenant pilot; periksa spasi/karakter khusus |
| CPE log "HTTP 429" atau Inform kadang berhasil kadang tidak | Rate limiter 5 req/s per IP (§2) — banyak CPE di balik satu IP | Nyalakan bergantian; tunggu retry; bukan bug baru |
| Device muncul tapi vendor "belum diketahui" | OUI belum di katalog (§2) | Tambah OUI lewat Catalog Vendor → Inform berikutnya menyembuhkan otomatis |
| Device muncul, vendor benar, tapi model kosong | Belum ada `device_models` dengan `product_class` yang persis sama | Lihat ProductClass di Device Detail → daftarkan model → Inform berikutnya terisi |
| Task Set "selesai" tapi nilai di CPE tidak berubah | CPE meng-ACK tapi butuh reboot/apply; atau path mapping salah | Cek dari sisi CPE (§4-6); catat path/firmware di §6 |
| Task Get/Set FAILED dengan Fault 9005 | Path parameter tidak ada di firmware itu | Catat model+firmware; tambah mapping override per `device_model_id` |
| Connection Request gagal | CPE di NAT/CGNAT atau firewall blok port balik | §3a langkah 3; lihat catatan STUN di §2 (belum teruji lapangan) |
| Banyak sesi berstatus `TIMEOUT` di `/cwmp/sessions` | Normal: reaper menutup sesi tanpa POST-kosong setelah 15 menit | Bukan error protokol (§2) |
| Inform berhasil (device muncul) tetapi **task/RPC tidak pernah terkirim** ke CPE, sesi langsung 204 | CPE tidak mengirim balik cookie sesi `acs_session` (CPE tanpa dukungan cookie, atau cookie ber-flag Secure di kanal http — sudah diperbaiki 2026-10-15: Secure hanya bila https) | Lihat log ACS: POST kosong tanpa cookie → "token kosong". Cek di capture apakah CPE mengirim header `Cookie:` pada POST kedua |
| Set parameter ditolak CPE dengan Fault **9006/9007** (tipe/nilai tidak valid) untuk boolean atau angka | ACS saat ini mengirim SEMUA nilai SetParameterValues bertipe `xsd:string` — firmware ketat menolak `xsd:boolean`/`xsd:unsignedInt` yang diberi string | Catat parameter+model+firmware di §6. Perbaikan butuh tipe per-parameter (mapping `parameter_type_id`) — belum dikerjakan |
| Inform ditolak 400 padahal CPE terhubung | XML tidak bisa di-parse (mis. SOAP 1.2 / encoding tak dikenal) | Ambil body Inform dari capture; encoding ISO-8859-1/windows-1252 dan BOM sudah ditoleransi, SOAP 1.2 TIDAK (TR-069 mewajibkan 1.1) |
