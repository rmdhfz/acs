# Peta kode ACS

> **Dibuat otomatis** oleh `node scripts/gen-code-map.mjs` — jangan diedit tangan. Tujuan: sesi baru (manusia atau asisten AI) langsung tahu
> "apa ada di mana" tanpa menjelajah ulang repo. Konteks bisnis: `docs/produk/PRD.md`; keputusan arsitektur: `docs/arsitektur/TECH.md`;
> aturan kerja & konvensi: `CLAUDE.md`. Perbarui setelah menambah paket/endpoint/tabel/halaman (`--check` untuk memeriksa).

Ringkasan: 174 operasi REST • 61 tabel • 36 migrasi (terakhir `0036_notification_optical_alert`) • 28 skenario e2e.

## Alur & lapisan (Clean Architecture)
`domain` (entitas + antarmuka) → `usecase/*` (aturan bisnis) → `repository/mysql` (query sqlx) / `delivery/{cwmp,http,ws,usp}` (adapter masuk).
Handler hanya parsing/validasi; keputusan akses & bisnis di usecase; repository hanya query. Entrypoint: `cmd/acsd` (server), `cmd/migrate`,
`cmd/e2e` (uji end-to-end vs simulator CPE `internal/simcpe`), `cmd/cpesim`, `cmd/seed-admin`, `cmd/seal-secrets`.

## Paket Go
| Paket | Berkas | Ringkasan |
|---|---:|---|
| `cmd/acsd` | 1 | Command acsd adalah entrypoint utama ACS: menjalankan endpoint CWMP (untuk CPE) dan REST API internal (untuk BSS/OSS/portal NOC) sebagai dua server terpisah dalam satu proses, sesuai TECH.md §2. |
| `cmd/cpesim` | 1 | Command cpesim adalah CLI simulator CPE TR-069/CWMP multi-vendor (logika inti di internal/simcpe, dipakai bersama cmd/e2e). |
| `cmd/e2e` | 20 | Command e2e menjalankan skenario end-to-end terhadap stack ACS yang benar- benar hidup (docker-compose): REST API + endpoint CWMP + MariaDB + MinIO. |
| `cmd/loadtest` | 1 | Command loadtest mensimulasikan banyak sesi CWMP concurrent (Inform -> InformResponse -> POST kosong penutup) untuk mengukur p95 latency Inform, sesuai NFR di ROADMAP.md Fase 3 ("Load test: ribuan ses |
| `cmd/migrate` | 1 | Command migrate menjalankan migrasi database terurut dari folder migrations/ menggunakan golang-migrate (TECH.md §1). |
| `cmd/seal-secrets` | 1 | Command seal-secrets mengenkripsi (AES-GCM, kunci ACS_CREDENTIAL_ENC_KEY) nilai rahasia yang MASIH plaintext di provisioning_profile_parameters dan presets.configurations — langkah sekali jalan setela |
| `cmd/seed-admin` | 1 | Command seed-admin membuat user superadmin pertama pada instalasi baru. |
| `internal/config` | 1 | memuat konfigurasi aplikasi dari environment variable. |
| `internal/delivery/cwmp` | 3 | delivery layer untuk endpoint CWMP — satu-satunya tempat yang mem-parsing/mem-build XML SOAP (pkg/cwmpxml) dan menerjemahkannya ke/dari tipe polos usecase/session. |
| `internal/delivery/http` | 37 | delivery layer REST (Echo) untuk konsumsi internal/BSS (TECH.md §2). |
| `internal/delivery/usp` | 1 | — |
| `internal/delivery/ws` | 1 | — |
| `internal/domain` | 24 | berisi entities dan interface kontrak (repository & usecase) sesuai Clean Architecture yang dipakai proyek ini — lihat TECH.md §2. |
| `internal/metrics` | 2 | berisi custom Prometheus Collector untuk endpoint GET /api/v1/metrics (ROADMAP.md Fase 2, TECH.md §10 Observability). |
| `internal/payment/xendit` | 1 | klien HTTP kecil untuk Xendit Invoice API v2 (membuat dan membaca invoice). |
| `internal/repository/mysql` | 30 | berisi implementasi sqlx dari interface repository yang didefinisikan di internal/domain. |
| `internal/repository/redisrepo` | 1 | — |
| `internal/simcpe` | 7 | simulator CPE TR-069/CWMP multi-vendor yang dipakai bersama oleh cmd/cpesim (CLI manual) dan cmd/e2e (skenario otomatis). |
| `internal/usecase/auth` | 4 | menangani login, penerbitan/verifikasi JWT & API token, dan helper RBAC (role + scope tenant) — lihat TECH.md §1/§8. |
| `internal/usecase/billing` | 1 | -- pembayaran langganan SaaS lewat gateway (Xendit Invoice API), migrations/0032. |
| `internal/usecase/billing/billingtest` | 1 | berisi fake in-memory untuk menguji usecase/billing dan handler billing tanpa MariaDB/Xendit. |
| `internal/usecase/catalog` | 1 | mengelola data referensi vendor CPE (vendor, OUI, device model, vendor parameter mapping) — operasi data, bukan kode (CLAUDE.md, TECH.md §5). |
| `internal/usecase/console` | 3 | fitur Console bertabel sendiri (migrations/0033): layout menu per pengguna, laporan bug, dan riwayat impor/ekspor. |
| `internal/usecase/device` | 4 | menangani inventory & status CPE (TECH.md §11, FR-22/FR-23/FR-25). |
| `internal/usecase/diagnostics` | 1 | menangani diagnostik TR-069 standar yang dijalankan CPE (Ping & Traceroute): memicu lewat SetParameterValues, menunggu event "8 DIAGNOSTICS COMPLETE", mengambil hasil lewat GetParameterValues, lalu me |
| `internal/usecase/file` | 1 | — |
| `internal/usecase/firmware` | 1 | menangani upload firmware sungguhan ke object storage (MinIO/S3-compatible, ROADMAP.md Fase 2) dan penjadwalan upgrade (TECH.md §7, FR-19/FR-20/FR-21). |
| `internal/usecase/iam` | 1 | menangani tenant dan user management dasar (FR-26/27/28) — dibutuhkan agar sistem punya jalur untuk membuat tenant/user pertama kali, di luar 6 usecase inti yang didaftarkan TECH.md §2. |
| `internal/usecase/notification` | 1 | — notifikasi persisten (in-app + email) untuk staf Console (migrations/0030). |
| `internal/usecase/notify` | 2 | menyusun email transaksional (reset password, peringatan keamanan, selamat datang). |
| `internal/usecase/preset` | 1 | — |
| `internal/usecase/provisioning` | 2 | menangani provisioning profile dan zero-touch provisioning (ZTP) — lihat TECH.md §6, FR-13..FR-18. |
| `internal/usecase/report` | 1 | menyusun laporan SaaS untuk SUPERADMIN (operator platform). |
| `internal/usecase/selfservice` | 2 | usecase portal self-service pelanggan (role ENDUSER, PRD.md §4.2). |
| `internal/usecase/session` | 1 | mengorkestrasi siklus hidup sesi CWMP (TECH.md §3) — Inform -> InformResponse, lalu pertukaran RPC/task selama sesi terbuka. |
| `internal/usecase/signup` | 1 | — pendaftaran tenant mandiri (self-signup) dengan verifikasi email. |
| `internal/usecase/tag` | 1 | — |
| `internal/usecase/task` | 3 | mengorkestrasi antrean RPC CWMP (lihat TECH.md §4) — enqueue, dequeue per device (dipanggil dari usecase/session), retry dengan max_retries, dan resolusi logical key -> raw TR-069 path (TECH.md §5). |
| `internal/usecase/usp_session` | 1 | — |
| `internal/usecase/webhook` | 1 | mengelola langganan webhook keluar (typed event) dan pengirimannya ke sistem pihak ketiga (BSS/OSS/NMS) — `/goal` directive #3, PRD.md §4.1. |
| `internal/vendor_adapter` | 1 | menyediakan titik ekstensi untuk vendor dengan kuirk protokol CWMP nonstandar (urutan RPC aneh, encoding response nonstandar, dll). |
| `pkg/cryptoutil` | 1 | menyediakan enkripsi kredensial (connection request, inform auth) sesuai kebijakan keamanan di TECH.md §8 — tidak pernah menyimpan kredensial CPE dalam bentuk plaintext di database. |
| `pkg/cwmpxml` | 2 | berisi struct dan (de)serialisasi envelope CWMP 1.x (Broadband Forum TR-069) secara manual dengan encoding/xml stdlib — tanpa generator WSDL, sesuai keputusan di TECH.md §1/§12. |
| `pkg/digestauth` | 1 | mengimplementasikan HTTP Digest Authentication (RFC 7616 / RFC 2617) untuk dua sisi: klien (ACS menjawab tantangan CPE pada Connection Request TR-069) dan verifier sederhana (dipakai simulator CPE). |
| `pkg/mailer` | 1 | mengirim email lewat SMTP (STARTTLS atau TLS implisit) dengan antrean asinkron + retry. |
| `pkg/netguard` | 1 | menolak target permintaan keluar (outbound HTTP/UDP) yang menunjuk alamat internal — pertahanan SSRF untuk jalur di mana URL/host berasal dari input yang tidak sepenuhnya tepercaya (Connection Request |
| `pkg/objectstorage` | 1 | wrapper tipis di atas SDK MinIO (S3-compatible) — infra concern, dipakai usecase lewat interface domain.ObjectStorage (bukan dipakai langsung), analog pkg/cryptoutil untuk enkripsi kredensial. |
| `pkg/passwordhash` | 1 | menyimpan & memverifikasi password dengan Argon2id (RFC 9106, pemenang Password Hashing Competition) dalam format PHC string: $argon2id$v=19$m=65536,t=3,p=2$<salt b64>$<hash b64> Parameter ikut tersim |
| `pkg/redisutil` | 2 | — |
| `pkg/totp` | 1 | mengimplementasikan TOTP (RFC 6238, HMAC-SHA1, 30 detik, 6 digit) — kompatibel dengan Google Authenticator, Microsoft Authenticator, Authy, dsb. |
| `pkg/usp` | 1 | — |

## Endpoint REST (dari `openapi.yaml`, dikelompokkan per tag)
**Audit** (1)

| Method | Path | Ringkasan |
|---|---|---|
| GET | `/activity` | Audit trail global (aktivitas admin & perubahan data) |

**Auth** (19)

| Method | Path | Ringkasan |
|---|---|---|
| POST | `/auth/login` | Login (email + password) dan menerbitkan JWT sesi user |
| POST | `/auth/forgot-password` | Minta tautan reset password lewat email |
| POST | `/auth/reset-password` | Tetapkan password baru dengan token dari email |
| POST | `/auth/tokens` | Menerbitkan API token jangka panjang (`acs_...`) |
| GET | `/auth/tokens` | Daftar API token yang pernah diterbitkan (untuk dikelola/dicabut) |
| DELETE | `/auth/tokens/{id}` | Mencabut (revoke) satu API token |
| PATCH | `/auth/password` | Ganti password akun sendiri |
| GET | `/auth/config` | Konfigurasi publik halaman login (SSO / MFA tersedia?) |
| GET | `/auth/me` | Profil akun yang sedang login |
| POST | `/auth/mfa/verify` | Login tahap kedua — verifikasi kode TOTP / kode pemulihan |
| GET | `/auth/mfa` | Status MFA akun sendiri |
| POST | `/auth/mfa/setup` | Mulai enrol MFA — buat rahasia TOTP baru |
| POST | `/auth/mfa/enable` | Aktifkan MFA dengan membuktikan kode TOTP pertama |
| POST | `/auth/mfa/disable` | Nonaktifkan MFA akun sendiri |
| POST | `/auth/impersonation/end` | Akhiri sesi impersonate (catat di audit) |
| POST | `/auth/signup` | Pendaftaran tenant mandiri (self-signup) |
| POST | `/auth/signup/verify` | Verifikasi email pendaftaran — membuat tenant & admin pertama |
| GET | `/auth/oidc/login` | Mulai alur login OIDC (redirect ke Identity Provider) |
| GET | `/auth/oidc/callback` | Callback OIDC — tukar authorization code jadi sesi |

**Billing** (8)

| Method | Path | Ringkasan |
|---|---|---|
| GET | `/billing/plans` | Daftar paket langganan |
| POST | `/billing/plans` | Buat paket langganan (SUPERADMIN) |
| PATCH | `/billing/plans/{code}` | Ubah paket langganan (SUPERADMIN) |
| GET | `/billing/subscription` | Status langganan tenant |
| POST | `/billing/checkout` | Buat invoice pembayaran (checkout) |
| GET | `/billing/invoices` | Daftar invoice langganan |
| POST | `/billing/invoices/{uuid}/sync` | Periksa status invoice ke gateway ("Periksa status") |
| POST | `/billing/xendit/webhook` | Callback invoice Xendit (publik, server-ke-server) |

**Catalog** (17)

| Method | Path | Ringkasan |
|---|---|---|
| GET | `/vendors` | Daftar vendor CPE |
| POST | `/vendors` | Menambah vendor CPE baru |
| GET | `/vendors/{id}` | Detail satu vendor |
| PATCH | `/vendors/{id}` | Ubah vendor (nama, deskripsi, aktif) |
| DELETE | `/vendors/{id}` | Hapus (soft-delete) vendor |
| GET | `/vendors/{id}/ouis` | Daftar OUI (prefix MAC) terdaftar milik vendor |
| POST | `/vendors/{id}/ouis` | Mendaftarkan OUI baru milik vendor |
| DELETE | `/vendors/{id}/ouis/{ouiId}` | Hapus (soft-delete) satu OUI milik vendor |
| GET | `/device-models` | Daftar model device milik satu vendor |
| POST | `/device-models` | Menambah model device baru milik vendor |
| GET | `/device-models/{id}` | Detail satu device model |
| PATCH | `/device-models/{id}` | Ubah device model |
| DELETE | `/device-models/{id}` | Hapus (soft-delete) device model |
| GET | `/vendor-parameter-mappings` | Daftar mapping logical key -> path TR-069 milik satu vendor |
| POST | `/vendor-parameter-mappings` | Upsert mapping logical key -> path TR-069 mentah |
| GET | `/vendor-parameter-mappings/{id}` | Detail satu vendor parameter mapping |
| DELETE | `/vendor-parameter-mappings/{id}` | Hapus (soft-delete) satu parameter mapping |

**Console** (11)

| Method | Path | Ringkasan |
|---|---|---|
| GET | `/auth/menu-layout` | Susunan menu sidebar milik pengguna yang login |
| PUT | `/auth/menu-layout` | Simpan susunan menu sidebar (urutan, sub menu, item disembunyikan) |
| DELETE | `/auth/menu-layout` | Kembalikan menu sidebar ke susunan bawaan |
| POST | `/bug-reports` | Kirim laporan bug dari Console |
| GET | `/bug-reports` | Daftar laporan bug |
| GET | `/bug-reports/{uuid}` | Detail laporan bug |
| PATCH | `/bug-reports/{uuid}` | Ubah status penanganan laporan bug (SUPERADMIN) |
| GET | `/bug-reports/{uuid}/screenshot` | Tangkapan layar laporan bug |
| POST | `/data-transfers` | Catat proses impor/ekspor yang dijalankan di Console |
| GET | `/data-transfers` | Riwayat impor/ekspor |
| GET | `/data-transfers/{uuid}` | Detail satu proses impor/ekspor |

**Devices** (26)

| Method | Path | Ringkasan |
|---|---|---|
| GET | `/devices` | Daftar device (inventory CPE) |
| POST | `/devices` | Pra-registrasi perangkat (sebelum CPE pertama kali Inform) |
| POST | `/devices/refresh-summary` | Antre pembacaan ringkasan daftar (username PPPoE & level optik) untuk banyak device |
| POST | `/devices/import` | Impor massal perangkat (pra-registrasi) dengan dry-run |
| GET | `/devices/stats` | Agregat jumlah device per status dan per vendor |
| DELETE | `/devices/{id}` | Hapus (soft-delete) device |
| GET | `/devices/{id}` | Detail satu device |
| PATCH | `/devices/{id}` | Update catatan/kredensial connection request device |
| GET | `/devices/{id}/parameters` | Nilai parameter TR-069 terkini hasil sync (EAV) |
| GET | `/devices/{id}/events` | Histori event Inform CWMP (BOOT, PERIODIC, VALUE CHANGE, dst.) |
| GET | `/devices/{id}/optical-metrics` | Histori metrik optik GPON/EPON (Rx/Tx power, suhu, dst.) |
| GET | `/devices/{id}/activity` | Timeline audit trail aksi admin/operator terhadap device |
| POST | `/devices/{id}/reboot` | Reboot perangkat |
| POST | `/devices/{id}/factory-reset` | Factory Reset perangkat |
| POST | `/devices/{id}/push-file` | Push file (konfigurasi/log) ke perangkat |
| POST | `/devices/{id}/tasks/add-object` | Eksekusi RPC AddObject (FR-5) |
| POST | `/devices/{id}/tasks/delete-object` | Eksekusi RPC DeleteObject (FR-5) |
| POST | `/devices/{id}/tasks/get-parameter-names` | Eksekusi RPC GetParameterNames (FR-5) |
| POST | `/devices/{id}/tasks/get-parameter-values` | Eksekusi RPC GetParameterValues secara dinamis (FR-5) |
| POST | `/devices/{id}/tasks/set-parameter-attributes` | Eksekusi RPC SetParameterAttributes (aktifkan notifikasi perubahan nilai) |
| POST | `/devices/{id}/tasks/get-parameter-attributes` | Eksekusi RPC GetParameterAttributes (baca konfigurasi notifikasi) |
| POST | `/devices/{id}/refresh` | Telusuri pohon parameter perangkat secara rekursif |
| POST | `/devices/{id}/tasks/set-parameter-values` | Eksekusi RPC SetParameterValues secara dinamis (FR-5) |
| POST | `/devices/{id}/connection-request` | Paksa CPE Inform segera (TR-069 Connection Request) |
| GET | `/devices/{id}/config-snapshots` | Histori snapshot konfigurasi (parameter-tree) device |
| POST | `/devices/{id}/config-snapshots` | Ambil snapshot konfigurasi device sekarang |

**Diagnostics** (2)

| Method | Path | Ringkasan |
|---|---|---|
| POST | `/devices/{id}/diagnostics` | Trigger diagnostic test TR-069 pada device |
| GET | `/devices/{id}/diagnostics` | Histori diagnostic test milik device |

**Files** (3)

| Method | Path | Ringkasan |
|---|---|---|
| POST | `/files` | Upload berkas generik ke object storage |
| GET | `/files` | Daftar berkas generik |
| DELETE | `/files/{id}` | Hapus berkas generik |

**Firmware** (12)

| Method | Path | Ringkasan |
|---|---|---|
| POST | `/firmware` | Upload file firmware ke object storage |
| GET | `/firmware` | Daftar firmware milik satu vendor |
| POST | `/devices/{id}/firmware-upgrade` | Menjadwalkan upgrade firmware untuk sebuah device |
| GET | `/devices/{id}/firmware-jobs` | Histori firmware upgrade job milik device |
| GET | `/firmware/{id}` | Detail satu file firmware |
| PATCH | `/firmware/{id}` | Ubah metadata firmware (version, release_notes, is_active) |
| DELETE | `/firmware/{id}` | Hapus (soft-delete) firmware |
| POST | `/firmware/rollout-batches` | Buat firmware rollout batch (canary/staged rollout) |
| GET | `/firmware/rollout-batches` | Daftar firmware rollout batch |
| GET | `/firmware/rollout-batches/{id}` | Detail satu firmware rollout batch |
| POST | `/firmware/rollout-batches/{id}/advance` | Paksa evaluasi wave rollout sekarang |
| POST | `/firmware/rollout-batches/{id}/cancel` | Batalkan firmware rollout batch |

**Metrics** (1)

| Method | Path | Ringkasan |
|---|---|---|
| GET | `/metrics` | Metrik observability format Prometheus (bukan JSON) |

**Notifications** (6)

| Method | Path | Ringkasan |
|---|---|---|
| GET | `/notifications` | Daftar notifikasi milik pengguna yang login |
| GET | `/notifications/unread-count` | Jumlah notifikasi belum dibaca (untuk badge lonceng) |
| POST | `/notifications/read` | Tandai notifikasi sebagai sudah dibaca |
| GET | `/notifications/preferences` | Pengaturan kanal notifikasi per jenis (efektif) |
| PUT | `/notifications/preferences` | Simpan pengaturan kanal notifikasi |
| POST | `/notifications/announce` | Siarkan pengumuman ke staf (SUPERADMIN) |

**Presets** (5)

| Method | Path | Ringkasan |
|---|---|---|
| POST | `/presets` | Buat preset provisioning |
| GET | `/presets` | Daftar preset |
| GET | `/presets/{id}` | Detail satu preset |
| PATCH | `/presets/{id}` | Ubah preset |
| DELETE | `/presets/{id}` | Hapus preset |

**Provisioning** (6)

| Method | Path | Ringkasan |
|---|---|---|
| GET | `/provisioning-profiles` | Daftar provisioning profile |
| POST | `/provisioning-profiles` | Membuat provisioning profile baru |
| GET | `/provisioning-profiles/{id}` | Detail profile beserta daftar parameternya |
| PUT | `/provisioning-profiles/{id}` | Update profile (full update) beserta parameternya |
| DELETE | `/provisioning-profiles/{id}` | Soft-delete profile |
| POST | `/devices/{id}/apply-profile/{profileId}` | Menerapkan provisioning profile ke device (eksplisit) |

**Refs** (1)

| Method | Path | Ringkasan |
|---|---|---|
| GET | `/refs/{table}` | Daftar baris tabel referensi/enum (`ref_*`) |

**Reports** (2)

| Method | Path | Ringkasan |
|---|---|---|
| GET | `/reports/saas` | Laporan SaaS platform (SUPERADMIN) |
| GET | `/reports/saas/tenants.csv` | Unduh tabel tenant laporan SaaS (CSV) |

**SelfService** (4)

| Method | Path | Ringkasan |
|---|---|---|
| GET | `/self-service/devices` | Device milik akun pelanggan ini |
| GET | `/self-service/devices/{id}` | Detail satu device pelanggan |
| PATCH | `/self-service/devices/{id}/wifi` | Ubah SSID/passphrase WiFi device sendiri |
| POST | `/self-service/devices/{id}/reboot` | Reboot device sendiri |

**Sessions** (2)

| Method | Path | Ringkasan |
|---|---|---|
| GET | `/ws` | Upgrade WebSocket untuk push event realtime ke frontend |
| GET | `/cwmp/sessions/count` | Jumlah sesi CWMP berstatus OPEN saat ini (sesuai cakupan peran) |

**Tags** (7)

| Method | Path | Ringkasan |
|---|---|---|
| POST | `/tags` | Buat tag device |
| GET | `/tags` | Daftar tag |
| PATCH | `/tags/{id}` | Ubah tag (nama, warna) |
| DELETE | `/tags/{id}` | Hapus tag |
| GET | `/devices/{id}/tags` | Tag yang menempel pada satu device |
| POST | `/devices/{id}/tags` | Tempelkan tag ke device |
| DELETE | `/devices/{id}/tags/{tagId}` | Lepas tag dari device |

**Tasks** (6)

| Method | Path | Ringkasan |
|---|---|---|
| GET | `/tasks` | Daftar task (antrean RPC CWMP) |
| POST | `/tasks` | Membuat task RPC CWMP baru untuk sebuah device |
| GET | `/tasks/stats` | Agregat jumlah task per status |
| GET | `/tasks/{id}` | Detail satu task |
| POST | `/tasks/{id}/cancel` | Membatalkan task yang belum selesai |
| POST | `/tasks/{id}/retry` | Coba ulang task yang gagal (task yang sama, bukan task baru) |

**Tenants** (10)

| Method | Path | Ringkasan |
|---|---|---|
| PATCH | `/tenants/{id}/device-limit` | Atur batas perangkat tenant (paket SaaS) |
| POST | `/tenants` | Membuat tenant baru |
| GET | `/tenants` | Daftar seluruh tenant |
| GET | `/tenants/current` | Info branding tenant milik actor sendiri |
| GET | `/tenants/{id}` | Detail satu tenant |
| DELETE | `/tenants/{id}` | Hapus (soft-delete) tenant |
| PATCH | `/tenants/{id}` | Update tenant (kode/nama/aktivasi) |
| PATCH | `/tenants/{id}/cwmp-credentials` | Set/rotate shared secret Inform CWMP tenant |
| PATCH | `/tenants/{id}/branding` | Update branding tenant (nama brand, logo, warna aksen) |
| PATCH | `/tenants/{id}/task-quota` | Set kuota task queue (`max_pending_tasks`) tenant |

**Users** (12)

| Method | Path | Ringkasan |
|---|---|---|
| DELETE | `/users/{id}/mfa` | Admin melepas MFA akun lain |
| GET | `/users/{id}/devices` | Perangkat yang tertaut ke akun pelanggan (ENDUSER) |
| PUT | `/users/{id}/devices/{deviceId}` | Tautkan perangkat ke akun pelanggan (ENDUSER) |
| DELETE | `/users/{id}/devices/{deviceId}` | Lepas tautan perangkat dari akun pelanggan |
| POST | `/users/{id}/impersonate` | SUPERADMIN masuk sebagai pengguna tenant (impersonate) |
| POST | `/users` | Membuat user baru |
| GET | `/users` | Daftar user |
| GET | `/users/{id}` | Detail satu user |
| PATCH | `/users/{id}` | Update data user (nama, email, aktif/nonaktif) |
| DELETE | `/users/{id}` | Soft-delete user |
| PATCH | `/users/{id}/password` | Admin mereset password user lain |
| PATCH | `/users/{id}/roles` | Ganti seluruh role user (full replace) |

**Webhooks** (8)

| Method | Path | Ringkasan |
|---|---|---|
| POST | `/webhooks` | Buat langganan webhook keluar |
| GET | `/webhooks` | Daftar langganan webhook |
| GET | `/webhooks/deliveries/failed-count` | Jumlah delivery webhook berstatus FAILED (untuk badge/alert) |
| GET | `/webhooks/{id}` | Detail satu langganan webhook |
| PATCH | `/webhooks/{id}` | Ubah langganan webhook |
| DELETE | `/webhooks/{id}` | Hapus (soft-delete) langganan webhook |
| POST | `/webhooks/{id}/test` | Kirim payload uji ke URL target langganan |
| GET | `/webhooks/{id}/deliveries` | Log percobaan pengiriman untuk satu langganan |

**ZeroTouch** (5)

| Method | Path | Ringkasan |
|---|---|---|
| GET | `/zero-touch-rules` | Daftar aturan Zero-Touch Provisioning |
| POST | `/zero-touch-rules` | Membuat aturan Zero-Touch Provisioning baru |
| GET | `/zero-touch-rules/{id}` | Detail satu aturan Zero-Touch |
| PUT | `/zero-touch-rules/{id}` | Update aturan Zero-Touch Provisioning (full update) |
| DELETE | `/zero-touch-rules/{id}` | Soft-delete aturan Zero-Touch Provisioning |

## Tabel database (dari `schema.sql`)
| Tabel | Kolom | Keterangan |
|---|---:|---|
| `ref_vendors` | 12 | Daftar vendor CPE: ZTE, Huawei, FiberHome, Nokia, dll (terbuka untuk ditambah) |
| `ref_device_types` | 11 | — |
| `ref_data_model_versions` | 11 | — |
| `ref_event_codes` | 11 | Event code standar CWMP sesuai spec Broadband Forum TR-069 |
| `ref_task_types` | 11 | — |
| `ref_task_status` | 10 | — |
| `ref_device_status` | 10 | — |
| `ref_parameter_types` | 9 | — |
| `ref_ztp_trigger_event` | 11 | Kapan sebuah zero_touch_rules dievaluasi relatif thd event CWMP Inform (migrations/0009) |
| `ref_firmware_rollout_status` | 10 | Status lifecycle satu firmware_rollout_batches (migrations/0011) |
| `ref_roles` | 10 | — |
| `tenants` | 21 | Entitas ISP/brand yang menggunakan platform ACS ini (multi-tenant) |
| `users` | 23 | — |
| `tenant_signups` | 12 | Pendaftaran tenant mandiri menunggu verifikasi email |
| `user_recovery_codes` | 5 | Kode pemulihan MFA sekali pakai (hash saja) |
| `user_roles` | 5 | — |
| `api_tokens` | 13 | — |
| `activity_logs` | 9 | Jejak aksi admin/API terhadap data master (siapa, kapan, aksi apa) |
| `vendor_ouis` | 11 | Prefix OUI TR-069 per vendor, dipakai untuk auto-deteksi vendor perangkat baru |
| `device_models` | 15 | — |
| `vendor_parameter_mappings` | 17 | Pemetaan logical key -> path TR-069 aktual, per vendor/data-model/opsional per model |
| `devices` | 29 | Inventory seluruh CPE yang dikenal ACS |
| `device_parameters` | 8 | EAV: nilai parameter TR-069 terkini per device (hasil sync GetParameterValues) |
| `device_sessions` | 10 | Sesi CWMP aktif/historis per device — memungkinkan app server stateless/horizontal-scale |
| `device_events` | 8 | Histori event Inform per device — kandidat partisi by month saat volume produksi tinggi |
| `device_optical_metrics` | 10 | Metrik optik GPON/EPON per device (RX/TX power, dsb) — kandidat partisi by month |
| `tasks` | 22 | Antrean RPC CWMP yang harus dieksekusi ke device |
| `provisioning_profiles` | 16 | — |
| `provisioning_profile_parameters` | 8 | — |
| `zero_touch_rules` | 22 | Aturan pencocokan device baru ke provisioning profile / reboot / firmware push, kapan dievaluasi ditentukan trigger_event_id |
| `firmware_files` | 18 | — |
| `firmware_upgrade_jobs` | 18 | — |
| `firmware_rollout_batches` | 21 | Rencana canary/staged rollout firmware ke populasi device bertahap per wave |
| `device_diagnostics` | 8 | — |
| `ref_webhook_event_types` | 11 | Jenis event yang bisa dikirim ACS sebagai webhook (migrations/0013) |
| `webhook_subscriptions` | 16 | Langganan webhook per (tenant, jenis event, URL target) — migrations/0013 |
| `webhook_deliveries` | 14 | Log setiap percobaan pengiriman webhook (audit minimal) — migrations/0013 |
| `device_config_snapshots` | 4 | Snapshot konfigurasi device untuk diff/rollback (migrations/0014) |
| `files` | 17 | Katalog file generik untuk Download RPC CWMP (migrations/0018) |
| `tags` | 6 | Tag device untuk segmentasi & precondition preset (migrations/0019) |
| `device_tags` | 3 | Relasi many-to-many device <-> tag (migrations/0019) |
| `presets` | 11 | Preset match+config gaya GenieACS (migrations/0019, engine 0021) |
| `preset_applications` | 7 | Tracking apply preset per (preset, device): idempotensi drift-check, cooldown, kegagalan (migrations/0021) |
| `user_devices` | 4 | Mapping akun ENDUSER -> device di portal self-service (migrations/0020) |
| `password_reset_tokens` | 7 | Token reset password sekali-pakai (hash saja); audit minimal & hapus fisik kedaluwarsa — pengecualian eksplisit (migrations/0023) |
| `ref_notification_severity` | 10 | Tingkat keparahan notifikasi (migrations/0030) |
| `ref_notification_types` | 15 | Katalog jenis notifikasi + bawaan kanal (migrations/0030) |
| `notifications` | 15 | Notifikasi per penerima. Pengecualian audit 7-kolom (log volume tinggi, hanya created_at): lihat header migrations/0030 |
| `notification_preferences` | 7 | Pilihan kanal notifikasi per pengguna. Pengecualian audit 7-kolom (data preferensi milik pengguna): lihat header migrations/0030 |
| `tenants` | 16 | Katalog paket langganan SaaS (migrations/0032) |
| `ref_invoice_status` | 10 | Status invoice langganan (migrations/0032) |
| `billing_invoices` | 20 | Invoice langganan SaaS (Xendit). Nominal dari DB, bukan dari klien (migrations/0032) |
| `billing_webhook_events` | 5 | Log idempotensi callback Xendit. Pengecualian audit 7-kolom (tabel log; hanya received_at): lihat header migrations/0032 |
| `user_menu_layouts` | 10 | Susunan menu sidebar per pengguna (migrations/0033). Reset = soft delete; PUT berikutnya menghidupkan baris yang sama. |
| `ref_bug_report_status` | 10 | Status penanganan laporan bug (migrations/0033) |
| `ref_bug_report_severity` | 10 | Tingkat dampak laporan bug (migrations/0033) |
| `bug_reports` | 21 | Laporan bug dari Console (migrations/0033) |
| `ref_data_transfer_kind` | 10 | Jenis proses transfer data (migrations/0033) |
| `ref_data_transfer_status` | 10 | Hasil proses transfer data (migrations/0033) |
| `data_transfer_logs` | 23 | Riwayat proses impor/ekspor data (migrations/0033) |
| `bug_report_screenshots` | 14 | Tangkapan layar laporan bug, satu per laporan (migrations/0034) |

## Frontend
**Rute Console** (`frontend/src/App.tsx`)

| Path | Halaman |
|---|---|
| `/login` | LoginPage |
| `/forgot-password` | ForgotPasswordPage |
| `/reset-password` | ResetPasswordPage |
| `/sso` | SsoCallbackPage |
| `/signup` | SignupPage |
| `/verify-signup` | VerifySignupPage |
| `/terms` | TermsPage |
| `/privacy` | PrivacyPage |
| `/dashboard` | DashboardPage |
| `/devices` | DevicesPage |
| `/devices/:id` | DeviceDetailPage |
| `/tasks` | TasksPage |
| `/provisioning` | ProvisioningPage |
| `/firmware` | FirmwarePage |
| `/files` | FilesPage |
| `/tags` | TagsPage |
| `/presets` | PresetsPage |
| `/audit` | AuditPage |
| `/profile` | ProfilePage |
| `/notifications` | NotificationsPage |
| `/self-service` | SelfServicePage |
| `/webhooks` | WebhooksPage |
| `/catalog` | CatalogPage |
| `/reports` | ReportsPage |
| `/billing` | BillingPage |
| `/bug-reports` | BugReportsPage |
| `/data-transfers` | DataTransfersPage |
| `/administration` | AdministrationPage |
| `/administration/onboarding` | TenantOnboardingWizard |
| `/` | PublicHome |
| `*` | NotFoundPage |

**Komponen** (`frontend/src/components`, 41): AssignVendorModal, AuthShell, BugReportModal, CatalogDataTools, ChangePasswordModal, CommandPalette, ConfigHistory, CopyButton, CreditFooter, DataTools, DeviceFormModal, DeviceOverview, DeviceWifiTab, EmptyState, ExportMenu, GoogleSignInButton, ImportWizard, Layout, MenuManagerModal, MfaCard, Modal, NavClock, NotificationAttributesModal, NotificationBell, OpticalCell, OpticalDiscovery, Pager, Paginate, ParameterTree, PasswordStrengthBar, ProtectedRoute, QueryErrorBanner, QueryErrorToaster, RecordActions, RowMenu, SidebarNav, Spinner, StatCard, StatusBadge, TaskParts, UserDevicesModal

**Lib/hooks** (`frontend/src/lib`, 34): api, auth, autolabel, clientAdapter, clipboard, confirm, csv, exporter, focusTrap, format, hooks, i18n.extra, i18n, importer, limits, menu, notifications, optical, opticalDiscovery, password, pool, queryErrors, screenshot, session, taskSummary, theme, toast, transferLog, types, usePaged, useStatusParam, wake, wifi, ws

Berkas tes frontend: 31.

## Skenario e2e (`cmd/e2e`)
| Skenario | Fungsi |
|---|---|
| S1-onboarding-multivendor | `scenarioOnboarding` |
| S2-provisioning-push | `scenarioProvisioning` |
| S3-reboot-rpc | `scenarioReboot` |
| S4-get-parameter-values | `scenarioGetParams` |
| S5-fault-9005-no-retry | `scenarioFaultNoRetry` |
| S6-tenant-isolation | `scenarioTenantIsolation` |
| S7-connection-request | `scenarioConnectionRequest` |
| S8-firmware-rollout | `scenarioFirmwareRollout` |
| S9-preset-drift-heal | `scenarioPresetDriftHeal` |
| S10-security-guards | `scenarioSecurityGuards` |
| S11-crud-lengkap | `scenarioCRUDLengkap` |
| S12-auth-email-reset | `scenarioAuthEmailReset` |
| S13-device-provision-import | `scenarioDeviceProvision` |
| S14-diagnostics | `scenarioDiagnostics` |
| S15-optical-poll | `scenarioOpticalPoll` |
| S16-mfa-totp | `scenarioMFA` |
| S17-tree-attributes | `scenarioTreeAndAttributes` |
| S18-portal-pelanggan | `scenarioPortal` |
| S19-saas-operator | `scenarioSaaSOperator` |
| S20-paket-signup | `scenarioPlanAndSignup` |
| S21-vendor-generik | `scenarioGenericVendor` |
| S22-notifikasi | `scenarioNotify` |
| S23-billing-mati | `scenarioBilling` |
| S24-validasi-masukan | `scenarioValidation` |
| S25-udp-cr-rpc-cpe | `scenarioCPEWake` |
| S26-console-idor | `scenarioConsole` |
| S27-ont-nyata-hg6145f | `scenarioRealOnt` |
| S28-ringkasan-daftar-perangkat | `scenarioDeviceSummary` |

## Migrasi (`migrations/`)
`0001_init_schema` · `0002_tenant_cwmp_inform_credentials` · `0003_tenant_branding` · `0004_vendor_baseline_catalog` · `0005_tenant_task_quota` · `0006_task_metrics_indexes` · `0007_firmware_storage_key` · `0008_user_login_lockout` · `0009_ztp_rule_matching_actions` · `0010_vpm_software_version_pattern` · `0011_firmware_rollout_batches` · `0012_device_session_cwmp_namespace` · `0013_webhook_subscriptions` · `0014_device_config_snapshots` · `0015_device_geolocation` · `0016_firmware_rollout_scheduling` · `0017_performance_indexes` · `0018_generic_files` · `0019_tags_and_presets` · `0020_user_devices_mapping` · `0021_preset_engine` · `0022_seed_vendor_ouis` · `0023_password_reset_tokens` · `0024_users_password_changed_at` · `0025_diagnostics_mappings` · `0026_mapping_value_scale_optical` · `0027_user_mfa_totp` · `0028_tenant_device_limit_and_signups` · `0029_generic_vendor_and_more_oui` · `0030_notifications` · `0031_user_oidc_subject` · `0032_billing` · `0033_menu_bugreport_transfers` · `0034_bugreport_expected_screenshot` · `0035_vendor_optictimes_zte_models` · `0036_notification_optical_alert`

## Perintah penting
- Go: `go vet ./...`, `go test ./...`; race + integrasi MariaDB via Docker `golang:1.26.9` (lihat `docs/pengujian/PENGUJIAN_E2E.md`).
- Frontend (`frontend/`): `npx tsc -b`, `npx oxlint`, `npx vitest run`, `npm run build`, `npm run audit:ui`; paritas API: `node scripts/api-parity.mjs`.
- Spec: `npx @redocly/cli@2 lint openapi.yaml`. Peta ini: `node scripts/gen-code-map.mjs`.
