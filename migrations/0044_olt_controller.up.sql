-- 0044_olt_controller
--
-- OLT Controller, irisan 1 (docs/arsitektur/OLT_CONTROLLER_DESIGN.md): inventaris OLT per tenant dan antrean ONU yang terdeteksi
-- belum terdaftar di OLT. Read-only: tidak ada tabel/kolom untuk menulis konfigurasi ke OLT.
--
--  - ref_olt_poll_status       : hasil koneksi/polling terakhir sebuah OLT (UNKNOWN, OK, UNREACHABLE, AUTH_FAILED, ERROR).
--  - ref_onu_discovery_status  : PENDING (terdeteksi), IGNORED (operator abaikan), RESOLVED (tidak lagi belum-terdaftar di OLT).
--  - olts                      : satu baris = satu OLT milik satu tenant. Kredensial (SNMP community, password SSH) disimpan
--                                terenkripsi AES-GCM (ACS_CREDENTIAL_ENC_KEY) dan tidak pernah dikembalikan API.
--  - olt_unregistered_onus     : ONU yang dilaporkan OLT sebagai belum terdaftar. Unik per (olt_id, serial_number): kemunculan ulang
--                                memperbarui baris lama, bukan menambah baris. Jumlah per OLT dibatasi di aplikasi.
--
-- Audit 7-kolom penuh pada olts dan olt_unregistered_onus (data master/transaksional kecil, bukan log volume tinggi). olt_uuid dan
-- onu_uuid adalah identifier publik di API; PK tidak pernah diekspos. driver_code BUKAN ref_*: nilainya nama paket driver di kode
-- (registry), bukan enum yang dikelola operator. Vendor tidak disimpan terpisah: kuirk vendor hidup di driver, bukan di data.

CREATE TABLE ref_olt_poll_status (
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
    UNIQUE KEY uq_ref_olt_poll_status_code (code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

INSERT INTO ref_olt_poll_status (code, name) VALUES
    ('UNKNOWN',      'Belum pernah dicoba'),
    ('OK',           'Terhubung'),
    ('UNREACHABLE',  'Tidak dapat dijangkau'),
    ('AUTH_FAILED',  'Autentikasi ditolak'),
    ('ERROR',        'Galat lain');

CREATE TABLE ref_onu_discovery_status (
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
    UNIQUE KEY uq_ref_onu_discovery_status_code (code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

INSERT INTO ref_onu_discovery_status (code, name) VALUES
    ('PENDING',  'Terdeteksi, belum ditangani'),
    ('IGNORED',  'Diabaikan operator'),
    ('RESOLVED', 'Tidak lagi belum-terdaftar di OLT');

CREATE TABLE olts (
    id                  BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    olt_uuid            CHAR(36)        NOT NULL COMMENT 'Identifier publik di API; PK tidak pernah diekspos',
    tenant_id           BIGINT UNSIGNED NOT NULL,
    name                VARCHAR(128)    NOT NULL,
    driver_code         VARCHAR(32)     NOT NULL COMMENT 'Nama driver terdaftar di registry kode (mis. sim); bukan ref_*',
    host                VARCHAR(255)    NOT NULL COMMENT 'IP/hostname manajemen OLT; divalidasi netguard saat disimpan',
    snmp_port           SMALLINT UNSIGNED NOT NULL DEFAULT 161,
    ssh_port            SMALLINT UNSIGNED NOT NULL DEFAULT 22,
    snmp_community_enc  VARBINARY(512)  NULL COMMENT 'SNMP community terenkripsi AES-GCM; tidak pernah dikembalikan API',
    ssh_username        VARCHAR(128)    NULL,
    ssh_password_enc    VARBINARY(512)  NULL COMMENT 'Password SSH terenkripsi AES-GCM; tidak pernah dikembalikan API',
    notes               VARCHAR(255)    NULL,
    is_active           TINYINT(1)      NOT NULL DEFAULT 1,
    poll_status_id      BIGINT UNSIGNED NOT NULL,
    last_poll_at        DATETIME        NULL,
    last_poll_error     VARCHAR(255)    NULL COMMENT 'Pesan galat yang sudah dibersihkan, bukan teks mentah dari perangkat',
    reported_name       VARCHAR(128)    NULL COMMENT 'Nama sistem yang dilaporkan OLT saat Probe (tak tepercaya, dipangkas)',
    reported_version    VARCHAR(128)    NULL COMMENT 'Versi perangkat lunak yang dilaporkan OLT saat Probe (tak tepercaya, dipangkas)',
    created_at          DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_by          BIGINT UNSIGNED NULL,
    updated_at          DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    updated_by          BIGINT UNSIGNED NULL,
    deleted_at          DATETIME        NULL,
    deleted_by          BIGINT UNSIGNED NULL,
    is_deleted          TINYINT(1)      NOT NULL DEFAULT 0,
    UNIQUE KEY uq_olts_uuid (olt_uuid),
    KEY idx_olts_tenant (tenant_id, is_deleted),
    CONSTRAINT fk_olts_tenant FOREIGN KEY (tenant_id) REFERENCES tenants (id),
    CONSTRAINT fk_olts_poll_status FOREIGN KEY (poll_status_id) REFERENCES ref_olt_poll_status (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='Inventaris OLT per tenant (OLT Controller irisan 1, read-only); kredensial terenkripsi';

CREATE TABLE olt_unregistered_onus (
    id              BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    onu_uuid        CHAR(36)        NOT NULL COMMENT 'Identifier publik di API; PK tidak pernah diekspos',
    olt_id          BIGINT UNSIGNED NOT NULL,
    pon_port        VARCHAR(32)     NOT NULL DEFAULT '' COMMENT 'Penunjuk port PON apa adanya dari OLT (format spesifik vendor)',
    serial_number   VARCHAR(32)     NOT NULL COMMENT 'Serial number ONU apa adanya dari OLT; jangan diubah penulisannya',
    onu_type        VARCHAR(64)     NULL COMMENT 'Tipe ONU yang dilaporkan OLT (tak tepercaya, dipangkas)',
    status_id       BIGINT UNSIGNED NOT NULL,
    first_seen_at   DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    last_seen_at    DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    resolved_at     DATETIME(3)     NULL,
    created_at      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_by      BIGINT UNSIGNED NULL,
    updated_at      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    updated_by      BIGINT UNSIGNED NULL,
    deleted_at      DATETIME        NULL,
    deleted_by      BIGINT UNSIGNED NULL,
    is_deleted      TINYINT(1)      NOT NULL DEFAULT 0,
    UNIQUE KEY uq_olt_onus_uuid (onu_uuid),
    UNIQUE KEY uq_olt_onus_olt_serial (olt_id, serial_number),
    KEY idx_olt_onus_olt_status (olt_id, is_deleted, status_id, last_seen_at),
    CONSTRAINT fk_olt_onus_olt FOREIGN KEY (olt_id) REFERENCES olts (id),
    CONSTRAINT fk_olt_onus_status FOREIGN KEY (status_id) REFERENCES ref_onu_discovery_status (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='ONU yang dilaporkan OLT sebagai belum terdaftar; unik per (olt, serial), kemunculan ulang memperbarui baris';
