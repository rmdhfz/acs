-- =====================================================================
-- 0010_vpm_software_version_pattern
--
-- Konteks: vendor white-label reference-design (V-SOL/BDCOM/Dasan Zhone
-- dkk) bisa punya perbedaan path/perilaku TR-069 yang nyata antar batch
-- firmware pada "model" yang sama secara nominal (product_class identik).
-- `vendor_parameter_mappings` sebelumnya cuma bisa dipersempit sampai level
-- vendor_id + data_model_version_id + device_model_id -- tidak ada cara
-- membedakan per rentang software_version dalam satu device_model.
--
-- Kolom baru: software_version_pattern (pola SQL LIKE thd devices.
-- software_version, gaya sama seperti zero_touch_rules.serial_pattern --
-- lihat migrations/0009 dan matchSQLLike di usecase/provisioning/service.go).
-- NULL = mapping generik lintas semua versi software (perilaku lama, tidak
-- berubah untuk baris existing).
--
-- Uniqueness: software_version_pattern ditambahkan sbg member BARU di
-- uq_vpm_scope_key, mengikuti pola yang SUDAH ADA di kolom device_model_id
-- (nullable, ikut serta apa adanya di composite unique key). CATATAN yang
-- sama berlaku utk kolom baru ini seperti sudah berlaku utk device_model_id:
-- MariaDB memperlakukan NULL sbg distinct dlm UNIQUE KEY, jadi scope
-- (vendor_id, dmv, device_model_id, logical_key) dgn software_version_pattern
-- NULL TIDAK dijamin unik oleh constraint DB semata (bisa lolos 2 baris
-- NULL) -- ini bukan regresi baru, sudah jadi karakteristik desain tabel ini
-- sejak device_model_id ditambahkan (lihat schema.sql), konsisten
-- dipertahankan di sini alih-alih diam-diam mengubah strategi (mis. kolom
-- generated ternormalisasi) tanpa konfirmasi eksplisit dari pemilik skema.
--
-- Prioritas resolusi (spesifisitas, lihat komentar Resolve() di
-- internal/repository/mysql/catalog_repository.go untuk detail query):
--   1. device_model_id spesifik + software_version_pattern COCOK (LIKE)
--      dgn versi software device saat ini (pattern terpanjang menang bila
--      >1 cocok -- heuristik "lebih spesifik = string lebih panjang").
--   2. device_model_id spesifik, TANPA software_version_pattern.
--   3. device_model_id NULL (generik lintas model) + pattern COCOK.
--   4. device_model_id NULL, TANPA software_version_pattern (paling generik,
--      perilaku Resolve() sebelum migrasi ini).
-- =====================================================================
ALTER TABLE vendor_parameter_mappings
    ADD COLUMN software_version_pattern VARCHAR(255) NULL
        COMMENT 'Pola SQL LIKE thd devices.software_version -- mapping dgn pattern lebih spesifik diutamakan saat Resolve(), NULL = generik lintas versi'
        AFTER device_model_id,
    DROP KEY uq_vpm_scope_key,
    ADD UNIQUE KEY uq_vpm_scope_key
        (vendor_id, data_model_version_id, device_model_id, logical_key, software_version_pattern);
