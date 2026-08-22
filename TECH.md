# TECH.md — Desain Teknis ACS (Auto Configuration Server)

Dokumen ini menjelaskan arsitektur teknis untuk implementasi ACS multi-vendor sesuai `PRD.md`. Skema database detail ada di `schema.sql`.

---

## 1. Tech Stack

| Layer               | Pilihan                                                                        | Catatan                                                                                                                                                              |
| ------------------- | ------------------------------------------------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Bahasa              | Go (1.26.6)                                                                    |                                                                                                                                                                      |
| HTTP Framework      | Echo v5                                                                        | Dipakai untuk REST API internal maupun endpoint CWMP (endpoint CWMP butuh akses body raw XML, di luar binding otomatis Echo)                                         |
| Database            | MariaDB 10.6+                                                                  | InnoDB, utf8mb4                                                                                                                                                      |
| DB Access           | sqlx                                                                           | Tanpa full ORM, query eksplisit, mendukung transactional repository pattern                                                                                          |
| XML/SOAP            | `encoding/xml` standard library + struct mapping manual untuk envelope CWMP    | Tidak menggunakan generator WSDL otomatis — CWMP cukup kecil untuk dipetakan manual dan lebih mudah dikontrol untuk kuirk per-vendor                                 |
| Queue eksekusi task | Database-backed queue (tabel `tasks`)                                          | Tidak butuh broker eksternal (Kafka/RabbitMQ) di Fase 1 — volume & pola akses (per-session, per-device) cocok dengan polling ringan berbasis DB dan index yang tepat |
| Auth CPE↔ACS        | HTTP Basic/Digest Auth per device (kredensial di tabel `devices`)              |                                                                                                                                                                      |
| Auth API internal   | JWT/API token (tabel `api_tokens`) + RBAC                                      |                                                                                                                                                                      |
| Migrasi DB          | `golang-migrate` atau setara                                                   | File migrasi terurut, `schema.sql` adalah representasi akhir untuk referensi/dev seed                                                                                |
| Observability       | structured logging (mis. `zap`/`slog`), metrics endpoint Prometheus-compatible |                                                                                                                                                                      |

## 2. Arsitektur Umum

```
                    ┌───────────────────────────┐
   CPE (ZTE/Huawei/ │                           │
   FiberHome/Nokia) │──── HTTPS (CWMP/SOAP) ───▶│   ACS — CWMP Handler       │
                    │                           │   (delivery/cwmp)          │
                    └───────────────────────────┘             │
                                                                ▼
                                                     usecase: session, task,
                                                     provisioning, ZTP
                                                                │
                                                                ▼
                                                     repository (sqlx) ──▶ MariaDB
                                                                ▲
                                                                │
   ┌───────────────────────────┐             REST API (Echo)  │
   │ BSS/OSS, Portal NOC, dsb  │──── HTTPS (JSON) ────────────▶│
   └───────────────────────────┘
```

Dua "pintu masuk" (delivery) berbeda tapi berbagi usecase & repository yang sama:

1. **CWMP delivery** — endpoint yang bicara dengan CPE (SOAP/XML, stateful per-session).
2. **REST delivery** — endpoint yang bicara dengan sistem/operator internal (JSON, stateless, RBAC).

Struktur direktori mengikuti Clean Architecture yang sudah menjadi standar tim:

```
cmd/
  acsd/                # entrypoint utama (HTTP server, CWMP + REST)
internal/
  domain/              # entities, interfaces (repository & usecase contracts)
  usecase/
    session/           # handling CWMP session lifecycle
    task/               # task queue orchestration
    provisioning/       # profile & zero-touch logic
    device/              # inventory, status
    firmware/
    diagnostics/
  repository/
    mysql/               # implementasi sqlx dari interface repository
  delivery/
    cwmp/                 # SOAP/XML handler, envelope parsing
    http/                  # REST handler (Echo), request/response DTO
  vendor_adapter/          # normalisasi kuirk per vendor (lihat §6)
pkg/
  cwmpxml/                 # struct & (de)serialisasi envelope CWMP
  cryptoutil/              # enkripsi kredensial connection request
migrations/
schema.sql
```

## 3. Alur Sesi CWMP

1. CPE mengirim HTTP POST ke endpoint ACS berisi SOAP envelope `Inform` (memuat `DeviceId`, `Event`, `ParameterList` seperti `InternetGatewayDevice.DeviceInfo.SerialNumber`, dst).
2. ACS memvalidasi Basic Auth (implementasi saat ini: shared secret Inform per tenant di `tenants.cwmp_inform_username`/`cwmp_inform_password_enc`, dengan kredensial per-device di `devices.inform_username`/`inform_password_enc` sebagai override opsional setelah device dikenal — lihat `internal/usecase/session/service.go#authenticateInform`). **Auth hanya diperiksa di request yang membawa `Inform`** (titik pembentukan sesi); request lanjutan dalam sesi yang sama (POST kosong, respons RPC) mengandalkan `session_token` (UUID v4, di cookie `acs_session`) sebagai bearer credential, bukan re-check Basic Auth per request — ini disengaja, bukan celah, karena endpoint CWMP wajib TLS (lihat §8) dan `session_token` acak tidak ditebak. Jika device belum dikenal (serial number baru), catat sebagai device baru berstatus `UNREGISTERED`/`PROVISIONING` dan otomatis dapat `tenant_id` dari tenant pemilik shared secret yang berhasil dipakai.
3. ACS membalas `InformResponse`, lalu **menahan koneksi HTTP (long-ish loop request/response)** sesuai pola CWMP: CPE mengirim POST kosong berikutnya, dan ACS bisa mengirim RPC method sebagai body response bila ada task pending untuk device tersebut.
4. Selama sesi terbuka, ACS mengeksekusi task dari `tasks` (status `PENDING`/`QUEUED`) satu per satu, menunggu response CPE untuk tiap RPC, mencatat `response`/`error_message`, meng-update status task.
5. Sesi ditutup saat tidak ada task tersisa dan CPE mengirim POST kosong tanpa body — ACS membalas HTTP 204/empty untuk menandakan sesi selesai.
6. Setiap event code yang dikirim (mis. `0 BOOTSTRAP`, `1 BOOT`, `4 VALUE CHANGE`) dicatat di `device_events`. Event `0 BOOTSTRAP` memicu evaluasi `zero_touch_rules`.

Catatan penting: CWMP itu **session-bound state machine**, bukan request-response sederhana. Implementasi harus menyimpan state minimal di DB (bukan in-memory saja) agar service bisa horizontal-scale — instance mana pun yang menerima request berikutnya dari CPE yang sama (mis. di belakang load balancer) tetap bisa melanjutkan sesi berdasarkan `session_token` yang disimpan di tabel `device_sessions`.

## 4. Task Queue

- Task dibuat oleh: (a) operator via REST API, (b) sistem eksternal via REST API, (c) usecase provisioning/ZTP secara otomatis.
- Task disimpan di tabel `tasks` dengan `task_status_id`, `priority`, `parameters` (JSON — payload spesifik per `task_type`), `retry_count`, `max_retries`.
- Saat sesi CWMP dengan sebuah device terbuka, usecase `session` mengambil task `PENDING` milik device tsb terurut `priority DESC, created_at ASC`, menandai `SENT`, mengirim RPC, menunggu response pada request berikutnya dalam sesi yang sama.
- Bila device tidak dalam sesi aktif dan task butuh eksekusi segera (`priority` tinggi/`expires_at` dekat), usecase task memicu **Connection Request** ke device (HTTP GET ke `connection_request_url` milik device dengan Basic/Digest Auth) agar CPE membuka sesi baru.
- Retry: task `FAILED`/`TIMEOUT` di-retry hingga `max_retries`, dengan backoff sederhana (mis. jeda ke periodic inform berikutnya) sebelum ditandai gagal permanen.

## 5. Multi-Vendor Abstraction

Perbedaan utama antar-vendor pada level TR-069:

- Root data model berbeda: `InternetGatewayDevice.*` (TR-098) vs `Device.*` (TR-181).
- Path parameter untuk hal yang sama berbeda antar-vendor meski data model version sama (mis. path WiFi SSID Huawei vs ZTE tidak identik persis di banyak firmware).
- Beberapa vendor punya object vendor-extension (`X_<OUI>_...`) untuk fitur non-standar (mis. optical diagnostics tertentu).

**Solusi:** layer `vendor_parameter_mappings` (lihat `schema.sql`) memetakan _logical key_ (mis. `wifi.5g.ssid`, `wan.pppoe.username`, `device.optical.rx_power`) ke _path TR-069 aktual_, per kombinasi `vendor_id` + `data_model_version_id` (opsional per `device_model_id` bila ada pengecualian di level model tertentu).

Alur penerjemahan saat operator mengirim task `SetParameterValues` dengan logical key:

```
logical key + device target
        │
        ▼
resolve vendor_id, device_model_id, data_model_version_id dari device
        │
        ▼
lookup vendor_parameter_mappings (paling spesifik: match device_model dulu, fallback ke vendor+data_model saja)
        │
        ▼
raw TR-069 path → dimasukkan ke RPC SetParameterValues
```

Operator/sistem tetap bisa mengirim raw TR-069 path langsung (bypass mapping) untuk kasus vendor-specific yang belum dipetakan — parameter mapping bersifat _convenience layer_, bukan satu-satunya jalur.

### 5.1 Vendor Extensibility

Menambah vendor baru = operasi data, bukan deploy kode:

1. Insert baris ke `ref_vendors` + `vendor_ouis`.
2. Insert baris ke `device_models` untuk model yang didukung.
3. Insert baris `vendor_parameter_mappings` untuk logical key yang relevan.
4. (Opsional) tambah `provisioning_profiles` default untuk vendor tsb.

Bila suatu vendor punya kuirk protokol yang benar-benar menyimpang dari spec CWMP standar (mis. urutan RPC nonstandar, encoding response yang aneh), baru dibutuhkan kode tambahan di `internal/vendor_adapter/<vendor>/` — didesain sebagai pluggable adapter, bukan percabangan `if vendor == "zte"` yang tersebar di banyak tempat.

## 6. Zero-Touch Provisioning (ZTP)

Saat event `0 BOOTSTRAP` diterima dari device yang belum punya `provisioning_profile_id`:

1. Evaluasi `zero_touch_rules` milik tenant yang relevan, terurut `priority`.
2. Kriteria match (opsional, kombinasi AND jika diisi lebih dari satu): `vendor_id`, `device_model_id`, `oui`, `serial_pattern` (SQL `LIKE`).
3. Rule pertama yang match → assign `provisioning_profile_id` ke device, lalu queue task `SetParameterValues` berisi seluruh `provisioning_profile_parameters` milik profil tsb.
4. Jika tidak ada rule yang match, device tetap masuk inventory dengan status menunggu tindakan manual (tidak silently ignored).

## 7. Firmware Upgrade

1. Admin upload firmware (file disimpan di object storage/filesystem — path & checksum dicatat di `firmware_files`).
2. Admin buat `firmware_upgrade_jobs` untuk satu/banyak device.
3. Usecase firmware men-generate task `Download` (RPC CWMP standar untuk transfer file, `FileType=1 Firmware Upgrade Image`) saat device online/dalam sesi.
4. Hasil `TransferComplete` dari CPE (event code `7`) di-link balik ke `firmware_upgrade_jobs` untuk update status & `software_version` baru di `devices`.

## 8. Keamanan

- **Endpoint CWMP wajib TLS.** Sertifikat dikelola di layer reverse proxy/load balancer atau langsung di Go server.
- **Kredensial connection request** (`connection_request_username`/`password`) disimpan terenkripsi (AES-GCM, key dari secret manager/env, bukan hardcoded) — bukan plaintext di DB.
- **Kredensial Inform Auth** per device/profil juga tidak plaintext.
- **RBAC** di layer REST API: role `SUPERADMIN`, `ADMIN` (scoped per tenant), `NOC`, `VIEWER` — middleware Echo memeriksa scope tenant pada setiap request.
- **Rate limiting** pada endpoint REST publik dan endpoint CWMP (mencegah CPE nakal/loop menginform terlalu sering membebani server, dan sejak Basic Auth CWMP aktif, juga mitigasi brute-force kredensial). Implementasi saat ini: `middleware.RateLimiter` (Echo v5) di `cwmpEcho`, 5 req/s per identifier — **belum ada di REST API internal** (`restEcho`), masih perlu ditambahkan (lihat ROADMAP.md).
- Semua endpoint REST admin memerlukan API token (`api_tokens`) atau JWT sesi user; tidak ada endpoint mutasi tanpa autentikasi.
- Shared secret Inform CWMP (`tenants.cwmp_inform_password`) wajib minimal 16 karakter (`internal/usecase/iam/service.go`) — mengurangi risiko brute-force mengingat endpoint `/cwmp` publik-facing.

## 9. Skalabilitas & Performa

- App server didesain **stateless** — semua state sesi CWMP disimpan di `device_sessions`/`tasks`, sehingga bisa dijalankan multi-instance di belakang load balancer (sticky session per CPE bukan keharusan, hanya optimisasi opsional).
- Tabel volume tinggi (`device_events`, raw parameter sync) menggunakan audit ringan (`created_at` saja, tanpa 7-kolom penuh) untuk mengurangi overhead tulis.
- Pertimbangkan **partisi tabel** (`device_events`, `device_optical_metrics`) berdasarkan rentang waktu (`RANGE` by month) setelah volume produksi terukur — belum diterapkan di `schema.sql` awal, didesain agar mudah ditambahkan kemudian.
- Index pada kolom yang dipakai untuk polling task (`device_id`, `task_status_id`, `priority`) dan pencarian device (`serial_number`, `oui`, `mac_address`) menjadi prioritas.
- Kebijakan retensi: `device_events` dan histori parameter mentah disarankan dirotasi/diarsipkan (mis. > 90 hari dipindah ke cold storage) — di luar cakupan `schema.sql` (kebijakan operasional, bukan skema).

## 10. Observability

- Log terstruktur per sesi CWMP (device_id, session_token, event codes, durasi) dan per task (task_id, device_id, status transition).
- Metrik minimum yang perlu diekspos: jumlah device online/offline, task queue depth per status, rata-rata waktu penyelesaian task, error rate per vendor (mengindikasikan masalah kompatibilitas parameter mapping).
- Alert saat backlog `PENDING` menumpuk melewati threshold, atau error rate per vendor naik tiba-tiba (indikasi firmware baru yang berubah perilaku).

## 11. Pertimbangan Desain Skema (ringkas — detail di `schema.sql`)

- **Primary key:** `BIGINT UNSIGNED AUTO_INCREMENT` untuk seluruh tabel internal (performa & ukuran index lebih baik untuk tabel volume tinggi seperti `device_parameters`/`device_events`/`tasks`). Tabel yang berpotensi diekspos ke API eksternal/publik (`devices`, `tenants`, `tasks`, `users`) memiliki kolom tambahan `*_uuid CHAR(36)` sebagai identifier eksternal, agar PK internal tidak pernah bocor ke luar dan tidak mudah ditebak/di-enumerasi.
- **Referensi/lookup:** semua nilai enumerative (status, tipe task, tipe device, event code, dsb) memakai tabel `ref_*` alih-alih `ENUM` MySQL — konsisten dengan konvensi tim, memudahkan penambahan nilai baru tanpa `ALTER TABLE`, dan bisa dilampiri metadata (deskripsi, urutan tampilan) langsung di baris referensinya.
- **Audit trail:** tabel master/transactional penting memakai 7 kolom standar (`created_at`, `created_by`, `updated_at`, `updated_by`, `deleted_at`, `deleted_by`, `is_deleted`). Tabel log volume tinggi (`device_events`, `device_parameters`, `device_optical_metrics`) sengaja memakai audit minimal (`created_at`/`updated_at` saja) — trade-off eksplisit demi performa tulis, didokumentasikan sebagai komentar di `schema.sql`.
- **Soft delete:** dipakai konsisten via `is_deleted` + `deleted_at`/`deleted_by`, bukan `DELETE` fisik, untuk data master (vendor, device, profile, user, dst).
- **EAV untuk parameter device:** `device_parameters` menggunakan pola Entity-Attribute-Value, sama seperti pola `vendor_vsa_templates` yang sudah dipakai di sistem AAA FreeRADIUS — konsisten karena masalahnya serupa: setiap vendor punya set parameter/atribut yang berbeda dan berubah-ubah, sehingga kolom tetap tidak cocok.

## 12. Yang Sengaja Belum Diputuskan / Perlu Divalidasi

- Library XML/SOAP: implementasi manual vs mengevaluasi library CWMP Go yang sudah ada (perlu riset lisensi & kematangan sebelum dipakai produksi).
- Mekanisme Connection Request untuk CPE di belakang CGNAT (STUN) — Fase 2 per `PRD.md`.
- Strategi partisi tabel log — menunggu data volume produksi nyata sebelum menentukan skema partisi final.
- Object storage untuk file firmware (S3-compatible vs filesystem lokal) — tergantung infrastruktur deployment.
