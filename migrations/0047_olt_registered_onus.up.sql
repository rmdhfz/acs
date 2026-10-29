-- 0047_olt_registered_onus
--
-- OLT Controller, irisan B (docs/arsitektur/PETA_STATUS_DIAGNOSIS.md): snapshot ONU yang SUDAH terdaftar di OLT — di port PON mana,
-- ID ONU, dan keadaannya menurut OLT (ONLINE/OFFLINE/LOS/DYING_GASP). Dipakai untuk menautkan ONU di OLT dengan perangkat di ACS
-- (serial number + tenant yang sama) dan sebagai bukti diagnosis "kenapa offline?". Read-only terhadap OLT: tidak ada kolom/tabel
-- untuk menulis konfigurasi ke OLT.
--
--  - ref_olt_onu_state     : keadaan ONU yang DINORMALISASI driver dari nilai spesifik vendor (nilai aslinya disimpan apa adanya di
--                            olt_registered_onus.vendor_state). Status yang tak terpetakan = UNKNOWN, bukan ditebak.
--  - olt_registered_onus   : satu baris per (olt, serial). Unik per (olt_id, serial_number): kemunculan ulang memperbarui baris lama.
--                            ONU yang tidak lagi dilaporkan OLT dihapus-lunak (is_deleted); muncul lagi = dihidupkan kembali.
--                            Jumlah per OLT dibatasi di aplikasi (domain.OLTMaxRegisteredONUs).
--
-- Audit 7-kolom penuh (data master/transaksional, bukan log volume tinggi), sama dengan olt_unregistered_onus. onu_uuid = identifier
-- publik; PK tidak pernah diekspos. rx_power_dbm di sini adalah daya terima SISI OLT (upstream), BERBEDA dari device_optical_metrics
-- (dilaporkan ONT lewat TR-069) — dua sumber, dua tabel, tidak dicampur.

CREATE TABLE ref_olt_onu_state (
    id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    code        VARCHAR(32)     NOT NULL,
    name        VARCHAR(128)    NOT NULL,
    created_at  DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_by  BIGINT UNSIGNED NULL,
    updated_at  DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    updated_by  BIGINT UNSIGNED NULL,
    deleted_at  DATETIME        NULL,
    deleted_by  BIGINT UNSIGNED NULL,
    is_deleted  TINYINT(1)      NOT NULL DEFAULT 0,
    UNIQUE KEY uq_ref_olt_onu_state_code (code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

INSERT INTO ref_olt_onu_state (code, name) VALUES
    ('UNKNOWN',    'Tidak diketahui / tak terpetakan'),
    ('ONLINE',     'Online di OLT'),
    ('OFFLINE',    'Offline di OLT'),
    ('LOS',        'LOS — kehilangan sinyal optik'),
    ('DYING_GASP', 'Dying gasp — ONU kehilangan daya');

CREATE TABLE olt_registered_onus (
    id                    BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    onu_uuid              CHAR(36)        NOT NULL COMMENT 'Identifier publik di API; PK tidak pernah diekspos',
    olt_id                BIGINT UNSIGNED NOT NULL,
    pon_port              VARCHAR(32)     NOT NULL DEFAULT '' COMMENT 'Penunjuk port PON apa adanya dari OLT (format spesifik vendor)',
    onu_index             VARCHAR(16)     NULL COMMENT 'ID/nomor ONU pada port itu menurut OLT (apa adanya)',
    serial_number         VARCHAR(32)     NOT NULL COMMENT 'Serial number ONU apa adanya dari OLT; dicocokkan tanpa peka huruf besar/kecil dengan devices.serial_number',
    onu_type              VARCHAR(64)     NULL COMMENT 'Tipe ONU yang dilaporkan OLT (tak tepercaya, dipangkas)',
    state_id              BIGINT UNSIGNED NOT NULL,
    vendor_state          VARCHAR(64)     NULL COMMENT 'Nilai keadaan ASLI dari OLT sebelum dinormalisasi (tak tepercaya, dipangkas)',
    rx_power_dbm          DECIMAL(6,2)    NULL COMMENT 'Daya terima sisi OLT untuk ONU ini (dBm); NULL = OLT tidak melaporkan',
    first_seen_at         DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    last_seen_at          DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    last_state_change_at  DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    created_at            DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_by            BIGINT UNSIGNED NULL,
    updated_at            DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    updated_by            BIGINT UNSIGNED NULL,
    deleted_at            DATETIME        NULL,
    deleted_by            BIGINT UNSIGNED NULL,
    is_deleted            TINYINT(1)      NOT NULL DEFAULT 0,
    UNIQUE KEY uq_olt_reg_onus_uuid (onu_uuid),
    UNIQUE KEY uq_olt_reg_onus_olt_serial (olt_id, serial_number),
    KEY idx_olt_reg_onus_olt_port (olt_id, is_deleted, pon_port),
    KEY idx_olt_reg_onus_serial (serial_number, is_deleted),
    CONSTRAINT fk_olt_reg_onus_olt FOREIGN KEY (olt_id) REFERENCES olts (id),
    CONSTRAINT fk_olt_reg_onus_state FOREIGN KEY (state_id) REFERENCES ref_olt_onu_state (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='ONU terdaftar di OLT (snapshot polling); unik per (olt, serial), tautan ke devices lewat serial + tenant';
