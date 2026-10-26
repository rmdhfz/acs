-- 0043_tenant_storage_quota
--
--  - tenants.max_storage_bytes : batas total ukuran berkas generik (tabel files) milik tenant, dalam byte. NULL = tak dibatasi
--                                (tenant lama tidak berubah). Ditegakkan saat POST /files. Katalog firmware global
--                                (firmware_files, hanya SUPERADMIN) tidak dihitung.
-- Alasan: ADMIN tenant boleh mengunggah berkas (hingga 256 MB per berkas) tanpa batas jumlah/total, sehingga satu tenant dapat
-- menghabiskan bucket object storage yang dipakai bersama semua tenant.

ALTER TABLE tenants
    ADD COLUMN max_storage_bytes BIGINT UNSIGNED NULL COMMENT 'Batas total ukuran berkas tenant dalam byte (NULL = tak dibatasi; migrations/0043)' AFTER max_devices;
