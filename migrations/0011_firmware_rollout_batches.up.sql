-- =====================================================================
-- 0011_firmware_rollout_batches
--
-- Konteks: audit arsitektur pembanding GenieACS menunjukkan
-- usecase/firmware.Service.ScheduleUpgrade() kita cuma bisa menargetkan
-- SATU device per pemanggilan -- tidak ada konsep canary/staged rollout
-- (upgrade N% populasi dulu, cek tingkat kegagalan lewat sinyal event
-- BOOT/TransferComplete, baru lanjut wave berikutnya).
--
-- Tabel baru:
--   - ref_firmware_rollout_status (ref_*, BUKAN enum kolom): status
--     lifecycle satu batch rollout.
--   - firmware_rollout_batches: baris master satu rencana rollout
--     (firmware target, populasi target via vendor_id/device_model_id --
--     bentuk filter yang SAMA dgn domain.DeviceFilter yang sudah dipakai
--     endpoint listing device lain, lihat internal/domain/device.go --
--     supaya orchestration usecase nanti tinggal reuse DeviceRepository.List
--     utk resolve populasi, bukan query device baru yang terpisah),
--     wave_percentage, max_failure_rate_percent (threshold abort/pause),
--     current_wave (counter bookkeeping progres wave, diperbarui
--     orchestration logic yang MENYUSUL), status_id, audit 7-kolom
--     standar + batch_uuid publik (tabel ini kandidat diekspos lewat REST).
--
-- Individual per-device firmware job TETAP di firmware_upgrade_jobs yang
-- sudah ada (tidak duplikasi device-tracking di tabel baru) -- ditambah
-- 2 kolom nullable: rollout_batch_id (link balik ke batch pemilik, NULL
-- utk job ScheduleUpgrade satuan yang sudah ada sebelumnya, TIDAK berubah
-- perilakunya) dan wave_number (wave ke berapa dalam batch tsb job ini
-- dibuat -- dibutuhkan utk menghitung success/failure RATE PER WAVE,
-- bukan cuma per batch keseluruhan, saat orchestration logic mengevaluasi
-- apakah lanjut ke wave berikutnya).
--
-- Tidak ada logic orkestrasi (wave-advancement, failure-rate gating,
-- penjadwalan job per wave) yang ditambahkan di sini -- murni skema +
-- kemampuan query agregat (lihat FirmwareUpgradeJobRepository di
-- internal/domain/firmware.go), sesuai batas kerja yang diminta.
-- =====================================================================

-- ---------------------------------------------------------------------
-- 1. ref_firmware_rollout_status
-- ---------------------------------------------------------------------
CREATE TABLE ref_firmware_rollout_status (
    id              BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    code            VARCHAR(32)     NOT NULL COMMENT 'PENDING, IN_PROGRESS, PAUSED_FAILURE_THRESHOLD, COMPLETED, CANCELLED',
    name            VARCHAR(128)    NOT NULL,
    created_at      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_by      BIGINT UNSIGNED NULL,
    updated_at      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    updated_by      BIGINT UNSIGNED NULL,
    deleted_at      DATETIME        NULL,
    deleted_by      BIGINT UNSIGNED NULL,
    is_deleted      TINYINT(1)      NOT NULL DEFAULT 0,
    UNIQUE KEY uq_ref_firmware_rollout_status_code (code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='Status lifecycle satu firmware_rollout_batches';

INSERT INTO ref_firmware_rollout_status (code, name) VALUES
    ('PENDING', 'Menunggu dimulai'),
    ('IN_PROGRESS', 'Sedang berjalan'),
    ('PAUSED_FAILURE_THRESHOLD', 'Dijeda -- ambang gagal wave terlampaui'),
    ('COMPLETED', 'Selesai -- seluruh wave sukses diterapkan'),
    ('CANCELLED', 'Dibatalkan operator');

-- ---------------------------------------------------------------------
-- 2. firmware_rollout_batches
-- ---------------------------------------------------------------------
CREATE TABLE firmware_rollout_batches (
    id                          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    batch_uuid                  CHAR(36)        NOT NULL,
    tenant_id                   BIGINT UNSIGNED NULL COMMENT 'NULL = rollout lintas-tenant/global',
    firmware_file_id            BIGINT UNSIGNED NOT NULL,
    vendor_id                   BIGINT UNSIGNED NULL COMMENT 'Kriteria seleksi populasi target -- NULL = tidak difilter vendor',
    device_model_id             BIGINT UNSIGNED NULL COMMENT 'Kriteria seleksi populasi target -- NULL = tidak difilter model',
    wave_percentage             TINYINT UNSIGNED NOT NULL DEFAULT 10 COMMENT 'Persentase populasi per wave, mis. 10 = 10%% per wave',
    max_failure_rate_percent    TINYINT UNSIGNED NOT NULL DEFAULT 10 COMMENT 'Ambang tingkat gagal per wave (%%) sebelum rollout di-pause otomatis',
    current_wave                INT UNSIGNED    NOT NULL DEFAULT 0 COMMENT 'Wave terakhir yang sudah/sedang dijalankan (bookkeeping progres, diperbarui usecase orchestration)',
    status_id                   BIGINT UNSIGNED NOT NULL,
    notes                       VARCHAR(255)    NULL,
    started_at                  DATETIME        NULL,
    completed_at                DATETIME        NULL,
    created_at                  DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_by                  BIGINT UNSIGNED NULL,
    updated_at                  DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    updated_by                  BIGINT UNSIGNED NULL,
    deleted_at                  DATETIME        NULL,
    deleted_by                  BIGINT UNSIGNED NULL,
    is_deleted                  TINYINT(1)      NOT NULL DEFAULT 0,
    UNIQUE KEY uq_firmware_rollout_batches_uuid (batch_uuid),
    KEY idx_frb_tenant (tenant_id),
    KEY idx_frb_firmware (firmware_file_id),
    KEY idx_frb_vendor (vendor_id),
    KEY idx_frb_device_model (device_model_id),
    KEY idx_frb_status (status_id),
    CONSTRAINT fk_frb_tenant FOREIGN KEY (tenant_id) REFERENCES tenants (id),
    CONSTRAINT fk_frb_firmware FOREIGN KEY (firmware_file_id) REFERENCES firmware_files (id),
    CONSTRAINT fk_frb_vendor FOREIGN KEY (vendor_id) REFERENCES ref_vendors (id),
    CONSTRAINT fk_frb_device_model FOREIGN KEY (device_model_id) REFERENCES device_models (id),
    CONSTRAINT fk_frb_status FOREIGN KEY (status_id) REFERENCES ref_firmware_rollout_status (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='Rencana canary/staged rollout firmware ke populasi device bertahap per wave';

-- ---------------------------------------------------------------------
-- 3. Link balik dari firmware_upgrade_jobs (tabel existing) ke batch
-- ---------------------------------------------------------------------
ALTER TABLE firmware_upgrade_jobs
    ADD COLUMN rollout_batch_id BIGINT UNSIGNED NULL
        COMMENT 'NULL = job ScheduleUpgrade satuan (perilaku lama). Diisi bila job ini dibuat sbg bagian dari firmware_rollout_batches'
        AFTER firmware_id,
    ADD COLUMN wave_number INT UNSIGNED NULL
        COMMENT 'Wave ke berapa (dalam rollout_batch_id) job ini dibuat -- dipakai hitung success/failure rate per wave'
        AFTER rollout_batch_id,
    ADD KEY idx_fuj_rollout_batch (rollout_batch_id),
    ADD KEY idx_fuj_rollout_batch_wave (rollout_batch_id, wave_number),
    ADD CONSTRAINT fk_fuj_rollout_batch FOREIGN KEY (rollout_batch_id) REFERENCES firmware_rollout_batches (id);
