# Laporan UAT Sementara — 21 Oktober 2026

> **Status: SEMENTARA (draft kerja).** Disusun saat jeda makan siang (±12.30 WIB) sebagai pegangan sesi lanjutan pukul 13.00.
> Setelah pengujian selesai, dokumen ini dirapikan lalu diterbitkan sebagai PDF (`scripts/build-docs-pdf.mjs`).
> Acuan test case: `docs/pengujian/CHECKLIST_UAT_ZTE_F609.xlsx` (55 test case). Ringkasan hasil sistem otomatis: `PENGUJIAN_E2E.md`.

**Legenda:** ✅ lulus (bukti ada) · ⚠️ lulus dengan catatan / sebagian · ❌ gagal / belum berfungsi · ⏳ belum diuji · ➖ tidak relevan

---

## 1. Ringkasan eksekutif

| Hal | Status |
|---|---|
| ONT fisik pertama kali terhubung ke ACS ini | ✅ **3 ONT** mengirim Inform dan tercatat di tenant `DEPOK UAT` |
| Baca konfigurasi dari ONT (SSID, klien WiFi, PPPoE username) | ✅ |
| Ubah konfigurasi di ONT (SSID & password) | ✅ — **setelah** perbaikan bug protokol (lihat T-07) |
| Reboot dari ACS | ✅ (ZTE F660) |
| Refresh ONT / Connection Request (ACS → ONT, tanpa reboot) | ✅ **terbukti pada Optictimes** (event `6 CONNECTION REQUEST` ×2, ONT online & task selesai) — tanpa kredensial CR di ACS; F660/F609 belum diuji kirim |
| Redaman optik ZTE | ❌ kosong — path TR-098 ZTE belum dipetakan (T-10) |
| Aktif/nonaktif SSID, Hide SSID, Max Clients | ⏳ fitur sudah ada & lulus tes otomatis; **belum dicoba di ONT fisik** |
| Isolation SSID, Priority | ⏳ path tidak dilaporkan F660; belum bisa (T-11) |
| Setelan TR-069 ONT dikembalikan ke ACS Telkom | ❌ **belum** — ketiga ONT masih menunjuk ke ACS ini (T-14) |

**Satu kalimat jujur:** jalur utama CWMP (Inform → baca → tulis) terbukti pada ONT nyata untuk dua model ZTE dan satu Optictimes, tetapi yang terbukti baru **SSID/password pada ZTE F660**; kompatibilitas lain masih perlu diuji satu per satu.

---

## 2. Perangkat & lingkungan uji

### 2.1 Perangkat fisik (semua root `InternetGatewayDevice` / TR-098, ProvisioningCode `TLCO.GRP2`)

| Merek / model | Serial | Firmware / HW | IP WAN TR069 | Catatan |
|---|---|---|---|---|
| ZTE F660 | ELWGC09CED21 | V5.2.10P2T10 / V5.0 | 10.0.123.11 | 4 SSID (WLAN 1–4), 2,4 GHz; perangkat uji utama |
| ZTE F609 | ZTEGC85E44E1 | V7.0.10P1N14 / V5.3 | 10.0.123.22 | Inform & vendor OK; belum ada uji tulis |
| Optictimes RXP4-AC1200 | OPTIF600C9E0 | HGUV4.3.7B02 / V1.0 | 10.0.122.211 | Vendor awalnya "Belum dikenal" → diperbaiki (T-09); ekstensi `X_CT-COM` |

### 2.2 Topologi & pengaturan

- ACS berjalan di **laptop (Docker Desktop)**; ONT diarahkan ke `http://172.30.1.220:17547/cwmp` dengan kredensial CWMP tenant `DEPOK UAT`, Periodic Inform 300 detik.
- Pagi hari laptop di jaringan `192.168.40.196`, siang `172.30.1.220` (IP berubah — T-04).
- Agar ONT bisa menjangkau laptop: **static route** ditambahkan di router + **Windows Firewall dimatikan seluruhnya** (`netsh advfirewall set allprofiles state off`) — harus dikembalikan (T-03).
- Versi: schema DB 35 (migrasi 0035 ditambahkan siang ini), kode cabang `dev` (belum di-push).

### 2.3 Hasil uji otomatis (di luar perangkat fisik, simulator CPE)

| Uji | Hasil |
|---|---|
| `go build`, `go vet` | ✅ |
| `go test -race ./...` (36 paket + integrasi MariaDB/Redis nyata) | ✅ |
| e2e `cmd/e2e` (28 skenario, simulator CPE) | ✅ 27 lulus · 0 gagal · 1 dilewati (S12: SMTP tidak diset) — dijalankan ulang setelah migrasi 0035, hasil sama |
| Frontend: `tsc`, vitest | ✅ 124 tes lulus |
| OpenAPI lint / paritas API frontend↔backend | ✅ valid · 169/169 |

> **Batas klaim:** simulator bukan firmware nyata. Lulus di sini = alur ACS benar; **bukan** bukti kompatibilitas ONT.

---

## 3. Hasil per test case (checklist 55 TC)

### A. Persiapan

| ID | Hasil | Bukti / catatan |
|---|---|---|
| PRE-01 Stack Docker sehat | ✅ | Docker Desktop awalnya mati → dinyalakan; mariadb/redis/minio/acsd sehat |
| PRE-02 Health endpoint | ✅ | `/readyz` 200 (db ok, redis ok); POST kosong `/cwmp` → 204 |
| PRE-03 Versi kode & skema | ✅ | image `acsd` ternyata basi → di-rebuild dari HEAD; skema 34 → 35 |
| PRE-04 Console & login | ✅ | `:5173` |
| PRE-05 Tenant pilot baru | ✅ | `DEPOK UAT` |
| PRE-06 Kredensial CWMP | ✅ | username `depok` (password dirotasi setelah uji) |
| PRE-07 Backup DB | ⚠️ | backup penuh dibuat **sebelum pembersihan data** (`bin/backup_sebelum_bersih_20261021.sql`, 14,5 MB), bukan backup pra-UAT bernama standar |
| PRE-08 IP & firewall | ⚠️ | IP berubah; firewall dimatikan total (belum diganti rule khusus port 17547) |
| PRE-09 Gladi jangkauan | ✅ | 204 via IP LAN laptop; ONT → laptop bisa di-ping setelah firewall dimatikan |
| PRE-10 Gladi Inform simulator | ✅ | 200 dgn kredensial benar, 401 salah; 409 pada serial milik tenant lain (perilaku benar) |
| PRE-11 Gladi Connection Request | ⏳ | |
| PRE-12 OUI di katalog | ⚠️ | OUI ZTE ada; OUI Optictimes **belum ada** → ditambah (T-09) |
| PRE-13 Rencana rollback | ⚠️ | setelan awal TR-069 Telkom difoto; konfirmasi password asli ACS Telkom belum |
| PRE-14 Ping ONT → ACS | ✅ | berhasil setelah firewall dimatikan (penyebab sebelumnya: firewall) |

### B. Koneksi (Inform)

| ID | Hasil | Bukti / catatan |
|---|---|---|
| INF-01 Setelan TR-069 awal | ✅ | foto: `acs.telkom.net:9090`, interval 43200 |
| INF-02 Arahkan ONT ke ACS | ✅ | 3 ONT |
| INF-03 Inform pertama muncul | ✅ | setelah routing + firewall + reboot |
| INF-04 Vendor teridentifikasi | ⚠️ | ZTE otomatis ✅; Optictimes ❌ "Belum dikenal" → diperbaiki (T-09) |
| INF-05 Data model | ✅ | TR-098 |
| INF-06 Identitas terbaca | ✅ | serial, SW, HW; model kosong pada awal → kini terisi (migrasi 0035) |
| INF-07 Event kode persis | ✅ | `0 BOOTSTRAP`, `1 BOOT`, `2 PERIODIC`, `4 VALUE CHANGE`, `M Reboot` |
| INF-08 IP sumber & CR URL tersimpan | ⚠️ | IP sumber tetap terbaca `172.21.0.1` (NAT Docker), tetapi setelah `ACS_CR_TRUSTED_CIDRS=10.0.0.0/8` CR URL F609 **tersimpan otomatis** (`http://10.0.123.22:58000/…`); F660 & Optictimes menyusul pada Inform berikutnya (T-06) |
| INF-09 Kredensial salah → 401 | ✅ | lewat simulator |
| INF-10 Periodic Inform stabil | ⚠️ | F660 terlihat Inform tiap 5 menit; 3 siklus belum didokumentasikan sistematis |

### C. Baca-saja

| ID | Hasil | Bukti / catatan |
|---|---|---|
| RD-01 Refresh pohon | ⚠️ | GetParameterNames 1× selesai; pohon penuh belum ditelusuri |
| RD-02 Info perangkat | ✅ | SW/HW/ProvisioningCode dari Inform |
| RD-03 SSID & PPPoE | ✅ | 4 SSID F660 (+ klien tersambung: nama, IP, MAC, RSSI); username PPPoE `0010030xxxxx@dukodu.id` di daftar |
| RD-04 Path palsu → Fault 9005 | ⏳ | lulus di simulator (e2e S5), belum di ONT |
| RD-05 Redaman optik | ❌ | tidak terbaca: path optik TR-098 ZTE belum dipetakan (T-10) |
| RD-06 Snapshot konfigurasi | ⏳ | |
| RD-07 Diagnostik Ping | ❌ | Fault 9002 (penyebab sama dengan T-07); **belum diulang** setelah perbaikan |

### D. Connection Request

| ID | Hasil | Catatan |
|---|---|---|
| CR-01 Kredensial CR diisi | ⚠️ | belum diisi untuk satu pun ONT; UI kini memandu (banner + konfirmasi) |
| CR-02 Connection Request ACS → ONT | ✅ | **Optictimes OPTIF600C9E0** 14.14 & 14.15 WIB: event `6 CONNECTION REQUEST`, status online, task baca selesai — ONT ini tidak mewajibkan autentikasi CR. F660/F609 (ZTE, biasanya Digest) belum diuji |
| CR-03 Password CR salah ditolak | ⏳ | |
| CR-04 Task terkirim di Inform berikutnya | ✅ | semua task Set/Get dieksekusi pada Periodic Inform |
| CR-05 Isi CR URL manual | ➖ | tidak lagi diperlukan: URL tersimpan otomatis (T-06) dan diisi dari parameter saat detail dibuka |
| CR-06 UDP/STUN | ⏳ | |

### E. Tulis

| ID | Hasil | Catatan |
|---|---|---|
| WR-01 Ubah SSID | ✅ | SSID #2 F660 → "Hafiz Ramadhan" dikonfirmasi pengguna (task COMPLETED) |
| WR-02 Kembalikan SSID asli | ⏳ | belum dikonfirmasi (SSID asli #2: `SSID2`, nonaktif) |
| WR-03 Ubah interval Inform | ⏳ | |
| WR-04 Reboot via ACS | ✅ | F660: task COMPLETED, Inform `1 BOOT` setelahnya |
| WR-05 Fault 9006/9007 (tipe) | ➖ | tidak ditemui; yang ditemui 9002 → T-07 |
| (tambahan) Aktif/nonaktif SSID, Hide, Max Clients | ⏳ | tes otomatis lulus; belum di ONT |

### F. Keamanan & isolasi

| ID | Hasil | Catatan |
|---|---|---|
| SEC-01 Isolasi tenant | ⚠️ | lulus di e2e (S6); belum diuji dengan akun tenant lain pada ONT |
| SEC-02 Kredensial tak plaintext | ✅ | API & UI menutup password; nilai rahasia task baru **dienkripsi** di `tasks.parameters` (T-12 diperbaiki; task lama sebelum perubahan tetap plaintext). Log acsd belum diperiksa khusus |
| SEC-03 REST tanpa token | ✅ | 401 (`/devices`, `/devices/refresh-summary`, `/tasks/:id/retry`) |
| SEC-04 Rate limit | ✅ | tidak ada 429 untuk 3 ONT |
| SEC-05 ENDUSER | ⏳ | lulus di e2e (S10) |

### G. Stabilitas & observasi

| ID | Hasil | Catatan |
|---|---|---|
| OBS-01 Log acsd bersih | ⚠️ | ada ERROR fault 9002 (sudah dijelaskan); WARN "unauthorized" tiap Inform sudah dihilangkan (T-13) |
| OBS-02 Dashboard akurat | ✅ | status online/offline sesuai |
| OBS-03 Audit trail | ⏳ | |
| OBS-04 Console tanpa error | ⏳ | |
| OBS-05 Sesi tidak menumpuk | ⏳ | |

### H. Pemulihan

| ID | Hasil | Catatan |
|---|---|---|
| RST-01 Kembalikan setelan TR-069 ONT | ❌ | **belum** (T-14) |
| RST-02 Bersihkan data uji | ✅ | 151 tenant sintetis + 19.910 device uji dihapus, integritas relasi diverifikasi (0 yatim); backup tersedia |
| RST-03 Kumpulkan bukti & laporan | ⚠️ | dokumen ini; log/screenshot belum dikumpulkan ke satu folder |

**Rekap:** dari 55 TC — ✅ ±22 · ⚠️ ±14 · ❌ 5 · ⏳ ±14 (angka perkiraan; dihitung ulang di laporan akhir).

---

## 4. Temuan & tindak lanjut

| # | Temuan | Dampak | Status |
|---|---|---|---|
| T-01 | Docker Desktop mati saat sesi dimulai | stack tidak jalan | ✅ diatasi |
| T-02 | Image `acsd` basi (dibangun sebelum commit terakhir) | yang diuji bukan kode terbaru | ✅ rebuild |
| T-03 | **Windows Firewall dimatikan total** untuk uji | laptop terbuka | ⚠️ **Nyalakan lagi** + rule `TCP 17547` (perintah ada di sesi) |
| T-04 | IP laptop berubah antar-jaringan | ACS URL di ONT jadi salah | ⚠️ gunakan IP tetap/reservasi |
| T-05 | ONT tak punya rute ke laptop | Inform tak masuk | ✅ static route di router |
| T-06 | IP sumber terbaca `172.21.0.1` (NAT Docker) → CR URL tak tersimpan | "Percepat" nonaktif; CR tak jalan otomatis | ✅ **diperbaiki**: opt-in `ACS_CR_TRUSTED_CIDRS` (default ketat; hanya literal IP di rentang tepercaya & lolos netguard). Terbukti di F609. Tinggal isi kredensial CR ONT di Device Detail untuk uji kirim CR |
| T-07 | **SetParameterValues → Fault 9002** pada ZTE F660 | tidak bisa ubah apa pun | ✅ **diperbaiki**: atribut tipe XML `_XMLSchema-instance:type` → `xsi:type` literal (commit `fix(cwmp)`); lulus e2e & ONT nyata |
| T-08 | ParameterKey 36 karakter (batas TR-069: 32) | potensi tolak di firmware ketat | ✅ diperbaiki (32 hex) |
| T-09 | Optictimes tampil "Belum dikenal" (OUI `9028F6` tak ada di katalog) | vendor/model kosong | ✅ migrasi 0035 (vendor + OUI + model RXP4-AC1200/F609/F660) + fitur **Tetapkan vendor** di Console. **Catatan:** IEEE mendaftarkan 9028F6 atas *Wuhan Yangtze Optical Technology Co.,Ltd.*, bukan "Optictimes" — pemetaan berdasarkan ONT fisik; mohon konfirmasi |
| T-10 | Path redaman optik ZTE TR-098 belum dipetakan | kolom/tab Redaman kosong | ⏳ butuh Refresh Children di `WANDevice.1` dari ONT, lalu mapping sebagai data |
| T-11 | Isolation & Priority tidak dilaporkan F660 di cabang WLAN | tidak bisa diatur | ⏳ tombol "Cari pengaturan lanjutan" menelusuri cabang lain; path **tidak ditebak** |
| T-12 | `tasks.parameters` menyimpan password WiFi plaintext (hanya diredaksi di API untuk non-ADMIN) | rahasia tersimpan di DB | ✅ **diperbaiki** untuk task baru: nilai sensitif dienkripsi (`enc:v1:`), dibuka saat dikirim; API tak pernah mengembalikannya (semua role). **Sisa:** task lama (mis. 887) masih plaintext — belum ada backfill |
| T-13 | Log WARN `Inform ditolak (unauthorized)` tiap Inform (ONT mencoba tanpa kredensial dulu) | kebisingan log, menyamarkan penolakan sungguhan | ✅ **diperbaiki**: percobaan tanpa kredensial = DEBUG; kredensial salah tetap WARN |
| T-14 | Ketiga ONT masih diarahkan ke ACS ini; ACS Telkom (`acs.telkom.net:9090`) tidak mengelolanya | layanan yang dikelola Telkom bisa terganggu | ❌ **kembalikan sebelum selesai** (RST-01); pastikan password ACS Telkom diketahui |
| T-15 | Akun `e2e-super` memakai password uji yang tertulis di dokumen | risiko bila ACS diekspos | ⏳ ganti/nonaktifkan |
| T-16 | Fault `9002` pada task Ping (`882`) terjadi sebelum perbaikan T-07 | belum tahu apakah Ping sekarang berhasil | ⏳ klik **Coba lagi** pada task itu |
| T-17 | **Laptop masuk Modern Standby 12.22–13.00** (idle timeout) → ACS tak terjangkau, ketiga ONT tampak OFFLINE saat sesi lanjutan dimulai; setelah bangun, ONT baru Inform lagi menurut jeda retry-nya (F609 13.30; F660 & Optictimes belum) | uji tersendat; bukan bug ACS | ⚠️ matikan sleep selama UAT: `powercfg /change standby-timeout-ac 0` (dan `monitor-timeout`); reboot ONT untuk mempercepat Inform |
| T-18 | Tampilan WiFi Optictimes: band "-", channel 0, "6d lalu" ambigu; ONT tak punya `SSIDAdvertisementEnabled` (hide lewat `X_CT-COM_SSIDHide`) | informasi menyesatkan; tombol Hide bisa menulis path yang tak ada | ✅ **diperbaiki** (data nyata): band dari ChannelsInUse/Standard, channel "11 (auto)", hide mengikuti parameter yang DILAPORKAN ONT (standar terbalik / vendor langsung), waktu "dtk/mnt/jam/hari". Isolasi standar (`IsolationEnable`) & `MaxUserNum` Optictimes terdeteksi otomatis |

---

## 5. Fitur yang dibangun/diperbaiki hari ini (di luar checklist)

- **Perbaikan protokol:** `xsi:type` literal, `ParameterKey` ≤ 32, rincian fault per parameter di pesan error task, error lama dihapus saat task akhirnya berhasil.
- **Tab WiFi** di detail perangkat: semua SSID, band/status/keamanan/channel, klien tersambung, password (tersembunyi), aktif/nonaktif, ubah SSID/password, Hide SSID, Max Clients; Isolation/Priority bila dilaporkan ONT; verifikasi baca-ulang otomatis.
- **Task:** ringkasan "apa yang dikerjakan" + tombol **Coba lagi** (`POST /tasks/{id}/retry`, task yang sama).
- **Daftar perangkat:** kolom Model, Redaman (berwarna), Username PPPoE; pencarian PPPoE/model; **Segarkan data** (`POST /devices/refresh-summary`).
- **Overview perangkat** baru; **Tetapkan vendor** untuk merek yang belum dikenal.
- **Data:** vendor OPTICTIMES + OUI + model (migrasi 0035); 4 migrasi lain tidak berubah.
- **Operasional:** pembersihan data uji, checklist Excel (55 TC), 10 commit terarah.
- **Tambahan sore hari (belum diuji di ONT fisik, lulus di simulator + MariaDB nyata):**
  - **Asisten sumber redaman optik** (tab Redaman Optik): menelusuri cabang optik ONT, mengenali kandidat RX/TX/suhu/tegangan/bias dari parameter yang benar-benar dilaporkan, pratinjau skala, SUPERADMIN menyimpan mapping per model (API mapping kini menerima `value_scale`). Path optik ZTE/Optictimes TIDAK ditebak.
  - **Filter daftar perangkat:** `optical_level` (ok/warn/bad/unknown; ambang -25/-27/-8 dBm) dan `no_vendor` ("Belum dikenal").
  - **Penyegaran PPPoE berkala** (`ACS_PPPOE_POLL_MINUTES`, bawaan 1440; 0 = mati) pada Inform; polling optik berkala sudah ada (`ACS_OPTICAL_POLL_MINUTES`).
  - **Peringatan redaman:** notifikasi in-app `OPTICAL_ALERT` (WARNING di bawah -25 dBm, CRITICAL di bawah -27 / di atas -8 dBm; ditekan 24 jam per perangkat+level; migrasi 0036) + webhook DEVICE_FAULT untuk kritis.
  - **Sengaja ditunda:** aksi massal tulis dengan pratinjau (berisiko sebelum uji tulis fisik aman) dan ekspor/impor hasil UAT.

---

## 6. Rencana sesi 13.00 (urut prioritas)

1. **Nyalakan lagi firewall** dengan rule `TCP 17547` dan pastikan ONT tetap Inform (T-03).
2. **Coba lagi task Ping `882`** di F660 (T-16) — memverifikasi perbaikan T-07 pada jalur diagnostik.
3. **Uji tulis di F660:** nonaktif/aktifkan SSID #3, Hide SSID #2, Max Clients; lalu **kembalikan** (WR-02, pengaturan lanjutan).
4. **Connection Request manual:** isi CR URL + kredensial CR di Device Detail, klik **Percepat** (CR-02/CR-05).
5. **Optictimes:** baca & ubah WiFi (vendor kini dikenal; fallback mapping GENERIC) — **belum pernah diuji tulis**.
6. **Redaman optik ZTE:** Refresh Children `InternetGatewayDevice.WANDevice.1` → catat path & nilai → mapping data (T-10). Cocokkan dengan GUI ONT.
7. **Isolation/Priority:** tombol "Cari pengaturan lanjutan"; bila tak ada, telusuri akar `X_ZTE-COM_*` (T-11).
8. **F609:** uji tulis SSID (belum ada).
9. ~~Putuskan T-12/T-13~~ — sudah diperbaiki (lihat temuan). Tinggal verifikasi lapangan: CR URL F660/Optictimes tersimpan setelah Inform berikutnya.
10. **Pemulihan:** kembalikan setelan TR-069 ketiga ONT ke Telkom (T-14), catat hasil, lalu susun laporan akhir (PDF).

---

## 7. Yang sengaja TIDAK diuji

Factory reset, firmware upgrade, push kredensial PPPoE pelanggan asli, aksi massal ke banyak perangkat, kompatibilitas selain tiga ONT di atas (28 vendor katalog lain masih data awal tanpa uji perangkat).

---

*Dokumen kerja — Claude Code (sesi 21 Okt 2026). Semua klaim "lulus" di atas punya bukti di sesi (task COMPLETED di DB, tangkapan layar, log, atau hasil tes); yang tidak punya bukti ditandai ⏳.*
