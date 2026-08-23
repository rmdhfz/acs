-- Object storage firmware (ROADMAP.md Fase 2, keputusan user: S3-compatible
-- / MinIO untuk dev). Sebelumnya `file_path` adalah string bebas yang
-- dikirim client (ACS tidak pernah benar-benar menerima/menyimpan file
-- firmware). Sekarang endpoint upload menerima file sungguhan dan menyimpan
-- object key hasil upload ke MinIO — rename kolom supaya makna barunya
-- jelas dan tidak disalahartikan sebagai path filesystem lokal.
--
-- PENTING kalau migrasi ini dijalankan pada instance yang SUDAH punya baris
-- `firmware_files` dari flow lama (upload metadata-only): rename murni ini
-- TIDAK mentransformasi datanya — nilai lama akan tersimpan sbg `storage_key`
-- padahal bukan object key MinIO valid. `PresignedGetURL` tetap akan
-- "berhasil" (SDK MinIO men-generate signature client-side tanpa mengecek
-- keberadaan objek di server), sehingga job upgrade tetap terjadwal dan baru
-- gagal belakangan saat CPE benar-benar mencoba download (404) -- bukan saat
-- admin menjadwalkan. Sebelum menjalankan migrasi ini di instance dgn data
-- lama: audit baris `firmware_files` existing, tandai `is_active = 0` (atau
-- soft-delete) baris yang file-nya tidak pernah benar-benar diupload ke
-- object storage sungguhan. Tidak relevan utk instance dev ini (schema.sql
-- tidak seed data `firmware_files`), dicatat utk deployment lain manapun yg
-- mewarisi data dari flow lama (temuan acs-code-reviewer).
ALTER TABLE firmware_files
    CHANGE COLUMN file_path storage_key VARCHAR(500) NOT NULL
        COMMENT 'Object key di object storage (MinIO/S3-compatible), bukan URL — presigned URL digenerate on-demand';
