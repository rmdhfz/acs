-- =====================================================================
-- 0009_ztp_rule_matching_actions
--
-- Konteks: audit arsitektur pembanding GenieACS (presets) menunjukkan
-- `zero_touch_rules` kita jauh lebih sempit dari yang seharusnya --
-- hanya cocok berdasar vendor_id/device_model_id/oui (exact) dan
-- serial_pattern (SQL LIKE), dan hanya bisa memicu SATU aksi (apply
-- provisioning_profile_id, wajib diisi). Migrasi ini menambah:
--
--   1. Precondition pencocokan baru:
--      - software_version_pattern: pola SQL LIKE thd devices.software_version,
--        dievaluasi sama persis dgn serial_pattern yang sudah ada
--        (lihat matchSQLLike di internal/usecase/provisioning/service.go).
--      - match_parameter_name + match_parameter_value_pattern: precondition
--        BERPASANGAN opsional -- kalau match_parameter_name diisi, rule juga
--        mensyaratkan device punya baris device_parameters dgn nama tsb yang
--        nilainya cocok pola match_parameter_value_pattern (SQL LIKE). Wajib
--        diisi BERSAMAAN (keduanya NULL atau keduanya terisi) -- ditegakkan
--        via CHECK constraint chk_ztr_match_parameter_pair (didukung native
--        sejak MariaDB 10.2.1, instance proyek ini 10.11 -- lihat
--        docker-compose.yml). Evaluasi pencocokan pola & pairing terhadap
--        device_parameters ADALAH logic usecase, BUKAN skema -- sengaja tidak
--        diimplementasikan di migrasi ini.
--   2. Aksi baru selain apply provisioning_profile_id (yang sekarang jadi
--      OPSIONAL -- lihat poin 3):
--      - post_apply_reboot: kalau 1, enqueue task REBOOT ke device setelah
--        aksi lain rule ini diterapkan.
--      - firmware_file_id: kalau diisi, enqueue task Download firmware
--        tsb (FK ke firmware_files, ON DELETE tidak diset eksplisit --
--        default RESTRICT MariaDB, mencegah firmware_files dihapus selagi
--        masih direferensikan rule ZTP aktif).
--   3. provisioning_profile_id sekarang NULLABLE: sebuah rule boleh HANYA
--      memicu reboot dan/atau firmware push tanpa menerapkan provisioning
--      profile sama sekali.
--   4. trigger_event_id (FK ke ref_ztp_trigger_event, tabel ref_* BARU --
--      bukan ENUM kolom, sesuai konvensi CLAUDE.md) menggantikan asumsi
--      hardcoded lama "rule cuma dievaluasi saat event 0 BOOTSTRAP"
--      (lihat EvaluateZeroTouch di usecase/provisioning/service.go, dipanggil
--      usecase/session saat BOOTSTRAP). Baris existing di-backfill ke
--      BOOTSTRAP_ONLY supaya PERILAKU TIDAK BERUBAH DIAM-DIAM utk rule yang
--      sudah ada -- kapan usecase/session mulai benar-benar mengevaluasi
--      trigger_event_id di event lain (mis. 1 BOOT, tiap Inform) adalah
--      perubahan logic yang MENYUSUL, bukan bagian dari migrasi skema ini.
--
-- Tidak ada logic evaluasi/eksekusi (rule matching, enqueue reboot/firmware,
-- wave/rollout orchestration) yang ditambahkan di sini -- murni skema +
-- repository read/write, sesuai batas kerja yang diminta.
-- =====================================================================

-- ---------------------------------------------------------------------
-- 1. ref_ztp_trigger_event (tabel ref_* baru, bukan ENUM)
-- ---------------------------------------------------------------------
CREATE TABLE ref_ztp_trigger_event (
    id              BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    code            VARCHAR(32)     NOT NULL COMMENT 'BOOTSTRAP_ONLY, BOOTSTRAP_OR_BOOT, EVERY_INFORM',
    name            VARCHAR(128)    NOT NULL,
    description     VARCHAR(255)    NULL,
    created_at      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_by      BIGINT UNSIGNED NULL,
    updated_at      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    updated_by      BIGINT UNSIGNED NULL,
    deleted_at      DATETIME        NULL,
    deleted_by      BIGINT UNSIGNED NULL,
    is_deleted      TINYINT(1)      NOT NULL DEFAULT 0,
    UNIQUE KEY uq_ref_ztp_trigger_event_code (code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='Kapan sebuah zero_touch_rules dievaluasi relatif thd event CWMP Inform';

INSERT INTO ref_ztp_trigger_event (code, name, description) VALUES
    ('BOOTSTRAP_ONLY', 'Hanya saat Bootstrap',
        'Rule dievaluasi hanya saat event CWMP "0 BOOTSTRAP" -- perilaku default/lama sebelum migrasi 0009, dipakai sbg nilai backfill rule existing'),
    ('BOOTSTRAP_OR_BOOT', 'Bootstrap atau Boot',
        'Rule dievaluasi saat event "0 BOOTSTRAP" ATAU "1 BOOT"'),
    ('EVERY_INFORM', 'Setiap Inform',
        'Rule dievaluasi pada setiap Inform, tanpa memandang event code');

-- ---------------------------------------------------------------------
-- 2. Kolom precondition & aksi baru di zero_touch_rules (nullable dulu)
-- ---------------------------------------------------------------------
ALTER TABLE zero_touch_rules
    ADD COLUMN software_version_pattern VARCHAR(255) NULL
        COMMENT 'Pola SQL LIKE thd devices.software_version, mis. "V5.%" -- evaluasi sama seperti serial_pattern'
        AFTER serial_pattern,
    ADD COLUMN match_parameter_name VARCHAR(512) NULL
        COMMENT 'Nama device_parameters.parameter_name -- precondition tambahan opsional, wajib diisi berpasangan dgn match_parameter_value_pattern (lihat chk_ztr_match_parameter_pair)'
        AFTER software_version_pattern,
    ADD COLUMN match_parameter_value_pattern VARCHAR(255) NULL
        COMMENT 'Pola SQL LIKE thd device_parameters.parameter_value milik match_parameter_name'
        AFTER match_parameter_name,
    ADD COLUMN post_apply_reboot TINYINT(1) NOT NULL DEFAULT 0
        COMMENT 'Jika 1, enqueue task REBOOT ke device setelah aksi lain rule ini diterapkan'
        AFTER provisioning_profile_id,
    ADD COLUMN firmware_file_id BIGINT UNSIGNED NULL
        COMMENT 'Jika diisi, enqueue task Download firmware ini ke device yang cocok'
        AFTER post_apply_reboot,
    ADD COLUMN trigger_event_id BIGINT UNSIGNED NULL
        AFTER firmware_file_id;

-- ---------------------------------------------------------------------
-- 3. Backfill rule existing ke BOOTSTRAP_ONLY (perilaku lama: evaluasi
--    hanya saat 0 BOOTSTRAP) SEBELUM kolom di-NOT NULL-kan.
-- ---------------------------------------------------------------------
UPDATE zero_touch_rules
    SET trigger_event_id = (SELECT id FROM ref_ztp_trigger_event WHERE code = 'BOOTSTRAP_ONLY')
    WHERE trigger_event_id IS NULL;

-- ---------------------------------------------------------------------
-- 4. provisioning_profile_id jadi nullable, trigger_event_id jadi NOT NULL,
--    FK baru, dan CHECK pairing match_parameter_*.
-- ---------------------------------------------------------------------
ALTER TABLE zero_touch_rules
    MODIFY COLUMN provisioning_profile_id BIGINT UNSIGNED NULL
        COMMENT 'NULLABLE sejak migrasi 0009 -- rule boleh hanya memicu reboot dan/atau firmware push tanpa menerapkan profile parameter',
    MODIFY COLUMN trigger_event_id BIGINT UNSIGNED NOT NULL,
    ADD CONSTRAINT fk_ztr_firmware FOREIGN KEY (firmware_file_id) REFERENCES firmware_files (id),
    ADD CONSTRAINT fk_ztr_trigger_event FOREIGN KEY (trigger_event_id) REFERENCES ref_ztp_trigger_event (id),
    ADD CONSTRAINT chk_ztr_match_parameter_pair
        CHECK ((match_parameter_name IS NULL) = (match_parameter_value_pattern IS NULL)),
    ADD KEY idx_ztr_firmware (firmware_file_id),
    ADD KEY idx_ztr_trigger_event (trigger_event_id);
