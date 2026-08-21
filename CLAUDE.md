# CLAUDE.md

Panduan kerja untuk Claude (dan Claude Code) saat membantu mengembangkan proyek ACS ini. Baca `PRD.md` untuk konteks bisnis dan `TECH.md` untuk keputusan arsitektur sebelum mengubah kode secara signifikan.

## Ringkasan Proyek

ACS multi-vendor (TR-069/CWMP) untuk manajemen CPE (ONT/ONU/router) lintas brand — ZTE, Huawei, FiberHome, Nokia, dan vendor lain yang ditambahkan kemudian. Backend Go, database MariaDB.

## Stack & Konvensi Wajib

- **Bahasa:** Go 1.26.6
- **HTTP framework:** Echo v5
- **DB access:** sqlx — jangan perkenalkan ORM full (GORM dll) tanpa diskusi eksplisit, ini keluar dari pola yang sudah mapan di semua proyek tim.
- **Database:** MariaDB, InnoDB, `utf8mb4_unicode_ci`.
- **Arsitektur:** Clean Architecture — `domain` → `usecase` → `repository`/`delivery`. Lihat struktur direktori di `TECH.md` §2. Jangan taruh logic bisnis di handler (`delivery`) atau di repository — handler hanya parsing/validasi request & memanggil usecase; repository hanya query, tanpa logic keputusan.

## Konvensi Skema Database (wajib diikuti untuk tabel baru)

1. **Referensi/enum → tabel `ref_*`**, jangan pakai `ENUM` MySQL/MariaDB untuk nilai yang bisa berkembang (status, tipe, kategori).
2. **Audit trail 7-kolom** untuk tabel master/transactional: `created_at`, `created_by`, `updated_at`, `updated_by`, `deleted_at`, `deleted_by`, `is_deleted`. Tabel log volume tinggi boleh audit minimal (`created_at`/`updated_at` saja) — tapi ini pengecualian yang harus disebutkan eksplisit di komentar SQL/PR, bukan default diam-diam.
3. **Soft delete** via `is_deleted` + `deleted_at`/`deleted_by`. Jangan `DELETE` fisik pada data master tanpa alasan kuat (mis. cleanup data log lama sesuai kebijakan retensi eksplisit).
4. **Primary key:** `BIGINT UNSIGNED AUTO_INCREMENT` untuk PK internal. Tabel yang diekspos ke API eksternal tambahkan `*_uuid CHAR(36)` sebagai identifier publik — jangan expose PK auto-increment ke luar.
5. Setiap tabel baru: tambahkan ke `schema.sql`, buat migrasi terurut di `migrations/`, dan **validasi end-to-end** — jalankan migrasi ke instance MariaDB nyata (lokal/dev), bukan hanya baca sintaks. Ini standar kualitas yang konsisten dipakai di semua proyek database tim, bukan opsional.

## Domain-Specific — TR-069/CWMP

- CWMP adalah protokol **stateful berbasis sesi**, bukan request-response biasa. Jangan asumsikan satu Inform = satu HTTP round-trip selesai — sesi tetap terbuka selama ACS masih punya task untuk dikirim ke CPE dalam sesi tsb. Lihat `TECH.md` §3 sebelum mengubah kode di `internal/delivery/cwmp/`.
- Jangan hardcode path parameter TR-069 per vendor di kode Go (mis. `if vendor == "zte" { path = "..." }` tersebar di banyak file). Perbedaan path per vendor **harus** lewat tabel `vendor_parameter_mappings`, bukan percabangan kode. Lihat `TECH.md` §5.
- Root data model berbeda antar generasi/vendor: `InternetGatewayDevice.*` (TR-098) vs `Device.*` (TR-181). Selalu resolve dari `data_model_versions` milik device, jangan diasumsikan tetap.
- Menambah dukungan vendor/model baru = **operasi data** (insert ke `ref_vendors`, `vendor_ouis`, `device_models`, `vendor_parameter_mappings`), bukan perubahan kode, kecuali vendor tsb punya kuirk protokol nonstandar yang memang butuh adapter khusus di `internal/vendor_adapter/`.
- Event code CWMP (`0 BOOTSTRAP`, `1 BOOT`, `2 PERIODIC`, `4 VALUE CHANGE`, `6 CONNECTION REQUEST`, `7 TRANSFER COMPLETE`, dst.) adalah string standar dari spec Broadband Forum — jangan diubah penulisannya saat disimpan/dibandingkan.

## Struktur Direktori

```
cmd/acsd/                      entrypoint
internal/domain/                entities & interface kontrak
internal/usecase/{session,task,provisioning,device,firmware,diagnostics}/
internal/repository/mysql/      implementasi sqlx
internal/delivery/cwmp/         handler SOAP/XML untuk CPE
internal/delivery/http/         handler REST (Echo) untuk internal/BSS
internal/vendor_adapter/        adapter khusus vendor dengan kuirk nonstandar
pkg/cwmpxml/                    struct & (de)serialisasi envelope CWMP
pkg/cryptoutil/                 enkripsi kredensial
migrations/
schema.sql
```

## Alur Kerja yang Diharapkan

- Sebelum menambah tabel/kolom: cek apakah kebutuhan itu sebenarnya bisa dipetakan ke pola EAV (`device_parameters`) atau `ref_*` yang sudah ada, daripada menambah kolom baru per parameter vendor-spesifik.
- Sebelum menambah endpoint REST baru: pastikan sudah lewat middleware auth (API token/JWT) dan RBAC scope tenant — tidak ada endpoint mutasi tanpa autentikasi, bahkan untuk keperluan development sementara.
- Query yang menyentuh kredensial (`connection_request_username/password`) tidak boleh di-log dalam bentuk plaintext di level manapun (aplikasi, migrasi, seed data contoh).
- Perubahan pada `provisioning_profile_parameters` tidak otomatis mendorong ulang konfigurasi ke device yang sudah terprovisioning (lihat FR-18 di `PRD.md`) — ini keputusan produk yang disengaja, jangan "diperbaiki" jadi otomatis tanpa konfirmasi.
- Test yang menyentuh task queue harus mencakup skenario retry dan `max_retries` tercapai, bukan hanya happy path.

## Yang Perlu Dikonfirmasi Sebelum Diimplementasikan (belum final)

- Pilihan library XML/SOAP (manual struct vs library pihak ketiga) — lihat `TECH.md` §12.
- Strategi Connection Request untuk CPE di belakang CGNAT (STUN) — Fase 2, belum ada keputusan desain.
- Strategi partisi tabel log (`device_events`, `device_optical_metrics`) — menunggu data volume produksi.

Bila mengerjakan area yang keputusannya belum final di atas, tanyakan dulu ke pengguna alih-alih mengasumsikan satu pendekatan.
