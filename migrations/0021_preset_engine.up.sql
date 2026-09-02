-- 0021_preset_engine
--
-- Mengaktifkan engine preset (PRESET_ENGINE_DESIGN.md, keputusan 2026-09-02:
-- Opsi C + 5a/5b). Sebelum ini tabel `presets` (migrations/0019) hanya CRUD --
-- tidak ada evaluator. Sekarang usecase/provisioning.EvaluatePresets menegakkan
-- preset ber-`enforce=1` tiap sesi CWMP saat device menyimpang (drift-heal).
--
-- `enforce` default 0 (OFF): preset lama tetap tersimpan tapi engine
-- mengabaikannya sampai operator sengaja mengaktifkan (5b). FR-18 hanya berlaku
-- untuk provisioning_profiles, BUKAN preset (5a) -- ini keputusan produk
-- eksplisit, lihat PRESET_ENGINE_DESIGN.md §5.

ALTER TABLE presets
    ADD COLUMN enforce TINYINT(1) NOT NULL DEFAULT 0
        COMMENT 'Bila 1: EvaluatePresets menegakkan konfigurasi ini tiap sesi CWMP saat device drift. Bila 0: tersimpan tapi diabaikan engine (default aman).'
        AFTER is_active,
    ADD COLUMN channel VARCHAR(64) NULL
        COMMENT 'Grouping opsional gaya GenieACS channel -- informasi UI saja di v1, tidak mengubah evaluasi.'
        AFTER enforce;

CREATE TABLE preset_applications (
    preset_id            BIGINT UNSIGNED NOT NULL,
    device_id            BIGINT UNSIGNED NOT NULL,
    last_applied_at      DATETIME        NULL,
    last_drift_hash      CHAR(64)        NULL
        COMMENT 'sha256 hex dari drift-set terakhir yang di-push -- short-circuit bila CPE melapor nilai lama terus',
    consecutive_failures INT             NOT NULL DEFAULT 0,
    status               VARCHAR(16)     NOT NULL DEFAULT 'PENDING'
        COMMENT 'PENDING (di-push, tunggu konvergensi) | CONVERGED (device sudah sesuai) | FAILED (menyerah setelah N kegagalan)',
    updated_at           DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (preset_id, device_id),
    KEY idx_preset_apps_device (device_id),
    CONSTRAINT fk_preset_apps_preset FOREIGN KEY (preset_id) REFERENCES presets (id) ON DELETE CASCADE,
    CONSTRAINT fk_preset_apps_device FOREIGN KEY (device_id) REFERENCES devices (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='Tracking apply preset per (preset, device): idempotensi drift-check, cooldown, hitung kegagalan (migrations/0021)';
