-- =====================================================================
-- ACS (Auto Configuration Server) — Multi-Vendor TR-069/CWMP
-- Schema: MariaDB 10.6+
-- Charset: utf8mb4 / utf8mb4_unicode_ci, Engine: InnoDB
--
-- Konvensi:
--   - Nilai enumerative disimpan sebagai tabel ref_* (bukan ENUM kolom).
--   - Tabel master/transaksi penting memakai 7 kolom audit standar:
--       created_at, created_by, updated_at, updated_by,
--       deleted_at, deleted_by, is_deleted   (soft delete)
--   - Tabel log volume tinggi (device_events, device_parameters,
--     device_optical_metrics) sengaja memakai audit minimal
--     (created_at/updated_at saja) demi performa tulis — lihat TECH.md §11.
--   - PK internal: BIGINT UNSIGNED AUTO_INCREMENT. Tabel yang berpotensi
--     diekspos ke API eksternal memiliki kolom tambahan `*_uuid CHAR(36)`
--     sebagai identifier publik.
-- =====================================================================

SET NAMES utf8mb4;
SET FOREIGN_KEY_CHECKS = 0;

-- =====================================================================
-- 1. REFERENCE / LOOKUP TABLES (ref_*)
-- =====================================================================

CREATE TABLE ref_vendors (
    id              BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    code            VARCHAR(32)     NOT NULL,
    name            VARCHAR(128)    NOT NULL,
    description     VARCHAR(255)    NULL,
    is_active       TINYINT(1)      NOT NULL DEFAULT 1,
    created_at      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_by      BIGINT UNSIGNED NULL,
    updated_at      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    updated_by      BIGINT UNSIGNED NULL,
    deleted_at      DATETIME        NULL,
    deleted_by      BIGINT UNSIGNED NULL,
    is_deleted      TINYINT(1)      NOT NULL DEFAULT 0,
    UNIQUE KEY uq_ref_vendors_code (code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='Daftar vendor CPE: ZTE, Huawei, FiberHome, Nokia, dll (terbuka untuk ditambah)';

CREATE TABLE ref_device_types (
    id              BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    code            VARCHAR(32)     NOT NULL COMMENT 'ONT, ONU, ROUTER, ACCESS_POINT, MODEM, STB',
    name            VARCHAR(128)    NOT NULL,
    is_active       TINYINT(1)      NOT NULL DEFAULT 1,
    created_at      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_by      BIGINT UNSIGNED NULL,
    updated_at      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    updated_by      BIGINT UNSIGNED NULL,
    deleted_at      DATETIME        NULL,
    deleted_by      BIGINT UNSIGNED NULL,
    is_deleted      TINYINT(1)      NOT NULL DEFAULT 0,
    UNIQUE KEY uq_ref_device_types_code (code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE ref_data_model_versions (
    id              BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    code            VARCHAR(16)     NOT NULL COMMENT 'TR098, TR181',
    name            VARCHAR(64)     NOT NULL,
    root_object     VARCHAR(64)     NOT NULL COMMENT 'InternetGatewayDevice, Device',
    created_at      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_by      BIGINT UNSIGNED NULL,
    updated_at      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    updated_by      BIGINT UNSIGNED NULL,
    deleted_at      DATETIME        NULL,
    deleted_by      BIGINT UNSIGNED NULL,
    is_deleted      TINYINT(1)      NOT NULL DEFAULT 0,
    UNIQUE KEY uq_ref_data_model_versions_code (code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE ref_event_codes (
    id              BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    code            VARCHAR(32)     NOT NULL COMMENT 'cth: "0 BOOTSTRAP", "1 BOOT", "4 VALUE CHANGE", "M Reboot"',
    name            VARCHAR(128)    NOT NULL,
    description     VARCHAR(255)    NULL,
    created_at      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_by      BIGINT UNSIGNED NULL,
    updated_at      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    updated_by      BIGINT UNSIGNED NULL,
    deleted_at      DATETIME        NULL,
    deleted_by      BIGINT UNSIGNED NULL,
    is_deleted      TINYINT(1)      NOT NULL DEFAULT 0,
    UNIQUE KEY uq_ref_event_codes_code (code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='Event code standar CWMP sesuai spec Broadband Forum TR-069';

CREATE TABLE ref_task_types (
    id              BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    code            VARCHAR(48)     NOT NULL COMMENT 'GET_PARAMETER_VALUES, SET_PARAMETER_VALUES, REBOOT, DOWNLOAD, dst',
    rpc_method_name VARCHAR(64)     NOT NULL COMMENT 'Nama RPC CWMP asli: GetParameterValues, SetParameterValues, dst',
    name            VARCHAR(128)    NOT NULL,
    created_at      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_by      BIGINT UNSIGNED NULL,
    updated_at      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    updated_by      BIGINT UNSIGNED NULL,
    deleted_at      DATETIME        NULL,
    deleted_by      BIGINT UNSIGNED NULL,
    is_deleted      TINYINT(1)      NOT NULL DEFAULT 0,
    UNIQUE KEY uq_ref_task_types_code (code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE ref_task_status (
    id              BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    code            VARCHAR(32)     NOT NULL COMMENT 'PENDING, QUEUED, SENT, COMPLETED, FAILED, CANCELLED, TIMEOUT',
    name            VARCHAR(128)    NOT NULL,
    created_at      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_by      BIGINT UNSIGNED NULL,
    updated_at      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    updated_by      BIGINT UNSIGNED NULL,
    deleted_at      DATETIME        NULL,
    deleted_by      BIGINT UNSIGNED NULL,
    is_deleted      TINYINT(1)      NOT NULL DEFAULT 0,
    UNIQUE KEY uq_ref_task_status_code (code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE ref_device_status (
    id              BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    code            VARCHAR(32)     NOT NULL COMMENT 'ONLINE, OFFLINE, PROVISIONING, FAULTY, UNREGISTERED, DECOMMISSIONED',
    name            VARCHAR(128)    NOT NULL,
    created_at      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_by      BIGINT UNSIGNED NULL,
    updated_at      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    updated_by      BIGINT UNSIGNED NULL,
    deleted_at      DATETIME        NULL,
    deleted_by      BIGINT UNSIGNED NULL,
    is_deleted      TINYINT(1)      NOT NULL DEFAULT 0,
    UNIQUE KEY uq_ref_device_status_code (code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE ref_parameter_types (
    id              BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    code            VARCHAR(32)     NOT NULL COMMENT 'string, int, unsignedInt, boolean, dateTime, base64',
    created_at      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_by      BIGINT UNSIGNED NULL,
    updated_at      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    updated_by      BIGINT UNSIGNED NULL,
    deleted_at      DATETIME        NULL,
    deleted_by      BIGINT UNSIGNED NULL,
    is_deleted      TINYINT(1)      NOT NULL DEFAULT 0,
    UNIQUE KEY uq_ref_parameter_types_code (code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

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
  COMMENT='Kapan sebuah zero_touch_rules dievaluasi relatif thd event CWMP Inform (migrations/0009)';

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
  COMMENT='Status lifecycle satu firmware_rollout_batches (migrations/0011)';

CREATE TABLE ref_roles (
    id              BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    code            VARCHAR(32)     NOT NULL COMMENT 'SUPERADMIN, ADMIN, NOC, VIEWER',
    name            VARCHAR(128)    NOT NULL,
    created_at      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_by      BIGINT UNSIGNED NULL,
    updated_at      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    updated_by      BIGINT UNSIGNED NULL,
    deleted_at      DATETIME        NULL,
    deleted_by      BIGINT UNSIGNED NULL,
    is_deleted      TINYINT(1)      NOT NULL DEFAULT 0,
    UNIQUE KEY uq_ref_roles_code (code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- =====================================================================
-- 2. TENANCY & USERS
-- =====================================================================

CREATE TABLE tenants (
    id                        BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    tenant_uuid               CHAR(36)        NOT NULL,
    code                      VARCHAR(32)     NOT NULL,
    name                      VARCHAR(128)    NOT NULL,
    is_active                 TINYINT(1)      NOT NULL DEFAULT 1,
    -- Shared secret Inform CWMP: dibutuhkan agar ACS bisa memvalidasi Inform
    -- dari device yang BENAR-BENAR baru (belum tercatat di `devices`), sebelum
    -- device tsb punya kredensial per-device sendiri. Lihat migrations/0002.
    cwmp_inform_username      VARCHAR(128)    NULL,
    cwmp_inform_password_enc  VARBINARY(255)  NULL COMMENT 'Terenkripsi (AES-GCM) di level aplikasi, bukan plaintext',
    -- White-labeling (Fase 2, migrations/0003) — logo_url eksternal (bukan upload).
    brand_name                VARCHAR(128)    NULL,
    logo_url                  VARCHAR(512)    NULL,
    primary_color             CHAR(7)         NULL,
    -- Kuota task queue per tenant (Fase 2, migrations/0005) — mencegah satu
    -- tenant menghabiskan resource task queue bersama. NULL = tidak dibatasi
    -- (default aman utk tenant existing yang belum diset). Scope kuota ini
    -- HANYA task queue (jumlah task PENDING milik tenant), BUKAN rate limit
    -- koneksi/sesi CWMP itu sendiri (itu di luar cakupan perubahan ini).
    max_pending_tasks         INT UNSIGNED    NULL,
    created_at      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_by      BIGINT UNSIGNED NULL,
    updated_at      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    updated_by      BIGINT UNSIGNED NULL,
    deleted_at      DATETIME        NULL,
    deleted_by      BIGINT UNSIGNED NULL,
    is_deleted      TINYINT(1)      NOT NULL DEFAULT 0,
    UNIQUE KEY uq_tenants_uuid (tenant_uuid),
    UNIQUE KEY uq_tenants_code (code),
    UNIQUE KEY uq_tenants_cwmp_inform_username (cwmp_inform_username)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='Entitas ISP/brand yang menggunakan platform ACS ini (multi-tenant)';

CREATE TABLE users (
    id              BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    user_uuid       CHAR(36)        NOT NULL,
    tenant_id       BIGINT UNSIGNED NULL COMMENT 'NULL untuk superadmin lintas-tenant',
    username        VARCHAR(64)     NOT NULL,
    email           VARCHAR(191)    NOT NULL,
    password_hash   VARCHAR(255)    NOT NULL,
    full_name       VARCHAR(128)    NOT NULL,
    is_active       TINYINT(1)      NOT NULL DEFAULT 1,
    last_login_at   DATETIME        NULL,
    -- Proteksi brute-force login (migrations/0008): failed_login_attempts
    -- di-increment tiap password salah (usecase/auth.Service.Login); saat
    -- mencapai threshold (auth.maxFailedLoginAttempts), locked_until diset
    -- ke masa depan (now + auth.lockoutDuration) dan login ditolak selama
    -- masa itu TANPA membedakan pesan error dari kredensial salah biasa
    -- (menghindari kebocoran info validitas username ke penyerang).
    failed_login_attempts INT UNSIGNED NOT NULL DEFAULT 0,
    locked_until    DATETIME        NULL,
    created_at      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_by      BIGINT UNSIGNED NULL,
    updated_at      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    updated_by      BIGINT UNSIGNED NULL,
    deleted_at      DATETIME        NULL,
    deleted_by      BIGINT UNSIGNED NULL,
    is_deleted      TINYINT(1)      NOT NULL DEFAULT 0,
    UNIQUE KEY uq_users_uuid (user_uuid),
    UNIQUE KEY uq_users_username (username),
    UNIQUE KEY uq_users_email (email),
    KEY idx_users_tenant (tenant_id),
    CONSTRAINT fk_users_tenant FOREIGN KEY (tenant_id) REFERENCES tenants (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE user_roles (
    id              BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    user_id         BIGINT UNSIGNED NOT NULL,
    role_id         BIGINT UNSIGNED NOT NULL,
    created_at      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_by      BIGINT UNSIGNED NULL,
    UNIQUE KEY uq_user_roles (user_id, role_id),
    CONSTRAINT fk_user_roles_user FOREIGN KEY (user_id) REFERENCES users (id),
    CONSTRAINT fk_user_roles_role FOREIGN KEY (role_id) REFERENCES ref_roles (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE api_tokens (
    id              BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    token_uuid      CHAR(36)        NOT NULL,
    user_id         BIGINT UNSIGNED NULL,
    tenant_id       BIGINT UNSIGNED NULL,
    name            VARCHAR(128)    NOT NULL,
    token_hash      VARCHAR(255)    NOT NULL,
    scopes          JSON            NULL,
    expires_at      DATETIME        NULL,
    revoked_at      DATETIME        NULL,
    created_at      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_by      BIGINT UNSIGNED NULL,
    updated_at      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    updated_by      BIGINT UNSIGNED NULL,
    UNIQUE KEY uq_api_tokens_uuid (token_uuid),
    UNIQUE KEY uq_api_tokens_hash (token_hash),
    KEY idx_api_tokens_user (user_id),
    KEY idx_api_tokens_tenant (tenant_id),
    CONSTRAINT fk_api_tokens_user FOREIGN KEY (user_id) REFERENCES users (id),
    CONSTRAINT fk_api_tokens_tenant FOREIGN KEY (tenant_id) REFERENCES tenants (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE activity_logs (
    id              BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    user_id         BIGINT UNSIGNED NULL,
    tenant_id       BIGINT UNSIGNED NULL,
    action          VARCHAR(128)    NOT NULL,
    entity_type     VARCHAR(64)     NOT NULL,
    entity_id       BIGINT UNSIGNED NULL,
    description     TEXT            NULL,
    ip_address      VARCHAR(45)     NULL,
    created_at      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    KEY idx_activity_logs_user (user_id),
    KEY idx_activity_logs_tenant (tenant_id),
    KEY idx_activity_logs_entity (entity_type, entity_id),
    CONSTRAINT fk_activity_logs_user FOREIGN KEY (user_id) REFERENCES users (id),
    CONSTRAINT fk_activity_logs_tenant FOREIGN KEY (tenant_id) REFERENCES tenants (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='Jejak aksi admin/API terhadap data master (siapa, kapan, aksi apa)';

-- =====================================================================
-- 3. VENDOR & DEVICE MODEL CATALOG
-- =====================================================================

CREATE TABLE vendor_ouis (
    id              BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    vendor_id       BIGINT UNSIGNED NOT NULL,
    oui             CHAR(6)         NOT NULL COMMENT 'Organizationally Unique Identifier, 6 hex char',
    notes           VARCHAR(255)    NULL,
    created_at      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_by      BIGINT UNSIGNED NULL,
    updated_at      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    updated_by      BIGINT UNSIGNED NULL,
    deleted_at      DATETIME        NULL,
    deleted_by      BIGINT UNSIGNED NULL,
    is_deleted      TINYINT(1)      NOT NULL DEFAULT 0,
    UNIQUE KEY uq_vendor_ouis_oui (oui),
    KEY idx_vendor_ouis_vendor (vendor_id),
    CONSTRAINT fk_vendor_ouis_vendor FOREIGN KEY (vendor_id) REFERENCES ref_vendors (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='Prefix OUI TR-069 per vendor, dipakai untuk auto-deteksi vendor perangkat baru';

CREATE TABLE device_models (
    id                      BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    vendor_id               BIGINT UNSIGNED NOT NULL,
    device_type_id          BIGINT UNSIGNED NOT NULL,
    data_model_version_id   BIGINT UNSIGNED NOT NULL,
    product_class           VARCHAR(128)    NULL COMMENT 'Nilai ProductClass yang dikirim CPE saat Inform',
    model_name              VARCHAR(128)    NOT NULL,
    description             VARCHAR(255)    NULL,
    is_active               TINYINT(1)      NOT NULL DEFAULT 1,
    created_at              DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_by              BIGINT UNSIGNED NULL,
    updated_at              DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    updated_by              BIGINT UNSIGNED NULL,
    deleted_at              DATETIME        NULL,
    deleted_by              BIGINT UNSIGNED NULL,
    is_deleted               TINYINT(1)      NOT NULL DEFAULT 0,
    KEY idx_device_models_vendor (vendor_id),
    KEY idx_device_models_type (device_type_id),
    KEY idx_device_models_dmv (data_model_version_id),
    UNIQUE KEY uq_device_models_vendor_model (vendor_id, model_name),
    CONSTRAINT fk_device_models_vendor FOREIGN KEY (vendor_id) REFERENCES ref_vendors (id),
    CONSTRAINT fk_device_models_type FOREIGN KEY (device_type_id) REFERENCES ref_device_types (id),
    CONSTRAINT fk_device_models_dmv FOREIGN KEY (data_model_version_id) REFERENCES ref_data_model_versions (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE vendor_parameter_mappings (
    id                      BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    vendor_id               BIGINT UNSIGNED NOT NULL,
    data_model_version_id   BIGINT UNSIGNED NOT NULL,
    device_model_id         BIGINT UNSIGNED NULL COMMENT 'Diisi bila mapping hanya berlaku untuk model spesifik (override)',
    -- software_version_pattern (migrations/0010): pola SQL LIKE thd
    -- devices.software_version, gaya sama seperti zero_touch_rules.serial_pattern.
    -- NULL = generik lintas versi software. Ditambahkan sbg member composite
    -- unique key SETELAH device_model_id, mengikuti pola nullable-in-key yang
    -- sudah ada di kolom tsb (lihat catatan lengkap ttg semantik NULL di
    -- MariaDB UNIQUE KEY pada migrations/0010). Resolve() memprioritaskan
    -- baris dgn pattern yang cocok (LIKE) sebelum fallback ke baris tanpa
    -- pattern -- lihat internal/repository/mysql/catalog_repository.go.
    software_version_pattern VARCHAR(255)   NULL,
    logical_key             VARCHAR(128)    NOT NULL COMMENT 'cth: wifi.5g.ssid, wan.pppoe.username, device.optical.rx_power',
    tr069_path              VARCHAR(512)    NOT NULL,
    parameter_type_id       BIGINT UNSIGNED NULL,
    description             VARCHAR(255)    NULL,
    created_at              DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_by              BIGINT UNSIGNED NULL,
    updated_at              DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    updated_by              BIGINT UNSIGNED NULL,
    deleted_at              DATETIME        NULL,
    deleted_by              BIGINT UNSIGNED NULL,
    is_deleted               TINYINT(1)      NOT NULL DEFAULT 0,
    KEY idx_vpm_vendor_dmv (vendor_id, data_model_version_id),
    KEY idx_vpm_device_model (device_model_id),
    UNIQUE KEY uq_vpm_scope_key (vendor_id, data_model_version_id, device_model_id, logical_key, software_version_pattern),
    CONSTRAINT fk_vpm_vendor FOREIGN KEY (vendor_id) REFERENCES ref_vendors (id),
    CONSTRAINT fk_vpm_dmv FOREIGN KEY (data_model_version_id) REFERENCES ref_data_model_versions (id),
    CONSTRAINT fk_vpm_device_model FOREIGN KEY (device_model_id) REFERENCES device_models (id),
    CONSTRAINT fk_vpm_parameter_type FOREIGN KEY (parameter_type_id) REFERENCES ref_parameter_types (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='Pemetaan logical key -> path TR-069 aktual, per vendor/data-model/opsional per model';

-- =====================================================================
-- 4. DEVICE INVENTORY
-- =====================================================================

CREATE TABLE devices (
    id                              BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    device_uuid                     CHAR(36)        NOT NULL,
    tenant_id                       BIGINT UNSIGNED NULL,
    vendor_id                       BIGINT UNSIGNED NULL,
    device_model_id                 BIGINT UNSIGNED NULL,
    device_status_id                BIGINT UNSIGNED NOT NULL,
    provisioning_profile_id         BIGINT UNSIGNED NULL,
    oui                              CHAR(6)         NULL,
    serial_number                    VARCHAR(128)    NOT NULL,
    product_class                    VARCHAR(128)    NULL,
    mac_address                      VARCHAR(17)     NULL,
    software_version                 VARCHAR(64)     NULL,
    hardware_version                 VARCHAR(64)     NULL,
    ip_address                       VARCHAR(45)     NULL,
    connection_request_url           VARCHAR(255)    NULL,
    connection_request_username      VARCHAR(128)    NULL,
    connection_request_password_enc  VARBINARY(255)  NULL COMMENT 'Terenkripsi (AES-GCM) di level aplikasi, bukan plaintext',
    inform_username                  VARCHAR(128)    NULL COMMENT 'Kredensial yang dipakai CPE saat Inform ke ACS',
    inform_password_enc              VARBINARY(255)  NULL,
    last_inform_at                   DATETIME        NULL,
    last_boot_event_at               DATETIME        NULL,
    notes                            VARCHAR(255)    NULL,
    created_at                       DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_by                       BIGINT UNSIGNED NULL,
    updated_at                       DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    updated_by                       BIGINT UNSIGNED NULL,
    deleted_at                       DATETIME        NULL,
    deleted_by                       BIGINT UNSIGNED NULL,
    is_deleted                        TINYINT(1)      NOT NULL DEFAULT 0,
    UNIQUE KEY uq_devices_uuid (device_uuid),
    UNIQUE KEY uq_devices_oui_serial (oui, serial_number),
    KEY idx_devices_tenant (tenant_id),
    KEY idx_devices_vendor (vendor_id),
    KEY idx_devices_model (device_model_id),
    KEY idx_devices_status (device_status_id),
    KEY idx_devices_mac (mac_address),
    KEY idx_devices_last_inform (last_inform_at),
    CONSTRAINT fk_devices_tenant FOREIGN KEY (tenant_id) REFERENCES tenants (id),
    CONSTRAINT fk_devices_vendor FOREIGN KEY (vendor_id) REFERENCES ref_vendors (id),
    CONSTRAINT fk_devices_model FOREIGN KEY (device_model_id) REFERENCES device_models (id),
    CONSTRAINT fk_devices_status FOREIGN KEY (device_status_id) REFERENCES ref_device_status (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='Inventory seluruh CPE yang dikenal ACS';

-- FK provisioning_profile_id ditambahkan belakangan (setelah tabel provisioning_profiles ada)

CREATE TABLE device_parameters (
    id                  BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    device_id           BIGINT UNSIGNED NOT NULL,
    parameter_name      VARCHAR(512)    NOT NULL,
    parameter_value     TEXT            NULL,
    parameter_type_id   BIGINT UNSIGNED NULL,
    writable            TINYINT(1)      NOT NULL DEFAULT 0,
    created_at          DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at          DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    KEY idx_device_parameters_device (device_id),
    UNIQUE KEY uq_device_parameters_name (device_id, parameter_name(191)),
    CONSTRAINT fk_device_parameters_device FOREIGN KEY (device_id) REFERENCES devices (id),
    CONSTRAINT fk_device_parameters_type FOREIGN KEY (parameter_type_id) REFERENCES ref_parameter_types (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='EAV: nilai parameter TR-069 terkini per device (hasil sync GetParameterValues)';

CREATE TABLE device_sessions (
    id              BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    device_id       BIGINT UNSIGNED NOT NULL,
    session_token   VARCHAR(64)     NOT NULL,
    cwmp_id         VARCHAR(64)     NULL COMMENT 'ID CWMP dari header SOAP, untuk korelasi request/response',
    cwmp_namespace  VARCHAR(64)     NULL COMMENT 'Namespace CWMP yang dideklarasikan CPE pada Inform sesi ini, mis. urn:dslforum-org:cwmp-1-2',
    status          VARCHAR(16)     NOT NULL DEFAULT 'OPEN' COMMENT 'OPEN, CLOSED, ERROR, TIMEOUT (TIMEOUT = sesi basi di-reap sweeper cmd/acsd)',
    remote_ip       VARCHAR(45)     NULL,
    started_at      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    ended_at        DATETIME        NULL,
    created_at      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    KEY idx_device_sessions_device (device_id),
    KEY idx_device_sessions_status (status),
    UNIQUE KEY uq_device_sessions_token (session_token),
    CONSTRAINT fk_device_sessions_device FOREIGN KEY (device_id) REFERENCES devices (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='Sesi CWMP aktif/historis per device — memungkinkan app server stateless/horizontal-scale';

CREATE TABLE device_events (
    id              BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    device_id       BIGINT UNSIGNED NOT NULL,
    session_id      BIGINT UNSIGNED NULL,
    event_code_id   BIGINT UNSIGNED NOT NULL,
    command_key     VARCHAR(255)    NULL,
    raw_payload     LONGTEXT        NULL COMMENT 'Salinan payload Inform mentah untuk keperluan debug, opsional',
    occurred_at     DATETIME        NOT NULL,
    created_at      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    KEY idx_device_events_device (device_id, occurred_at),
    KEY idx_device_events_session (session_id),
    KEY idx_device_events_code (event_code_id),
    CONSTRAINT fk_device_events_device FOREIGN KEY (device_id) REFERENCES devices (id),
    CONSTRAINT fk_device_events_session FOREIGN KEY (session_id) REFERENCES device_sessions (id),
    CONSTRAINT fk_device_events_code FOREIGN KEY (event_code_id) REFERENCES ref_event_codes (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='Histori event Inform per device — kandidat partisi by month saat volume produksi tinggi';

CREATE TABLE device_optical_metrics (
    id                  BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    device_id           BIGINT UNSIGNED NOT NULL,
    rx_power_dbm        DECIMAL(6,2)    NULL,
    tx_power_dbm        DECIMAL(6,2)    NULL,
    voltage             DECIMAL(6,2)    NULL,
    bias_current_ma     DECIMAL(8,3)    NULL,
    temperature_celsius DECIMAL(6,2)    NULL,
    distance_meters     DECIMAL(8,2)    NULL,
    recorded_at         DATETIME        NOT NULL,
    created_at          DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    KEY idx_device_optical_metrics_device (device_id, recorded_at),
    CONSTRAINT fk_device_optical_metrics_device FOREIGN KEY (device_id) REFERENCES devices (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='Metrik optik GPON/EPON per device (RX/TX power, dsb) — kandidat partisi by month';

-- =====================================================================
-- 5. TASK QUEUE (RPC ORCHESTRATION)
-- =====================================================================

CREATE TABLE tasks (
    id              BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    task_uuid       CHAR(36)        NOT NULL,
    device_id       BIGINT UNSIGNED NOT NULL,
    task_type_id    BIGINT UNSIGNED NOT NULL,
    task_status_id  BIGINT UNSIGNED NOT NULL,
    priority        TINYINT UNSIGNED NOT NULL DEFAULT 5 COMMENT '1 = tertinggi, 9 = terendah',
    parameters      JSON            NULL COMMENT 'Payload spesifik per task_type, mis. daftar {path, value} untuk SetParameterValues',
    response        JSON            NULL,
    error_message   TEXT            NULL,
    retry_count     INT UNSIGNED    NOT NULL DEFAULT 0,
    max_retries     INT UNSIGNED    NOT NULL DEFAULT 3,
    scheduled_at    DATETIME        NULL COMMENT 'Diisi bila task baru boleh dieksekusi setelah waktu tertentu (mis. ScheduleInform)',
    expires_at      DATETIME        NULL,
    sent_at         DATETIME        NULL,
    completed_at    DATETIME        NULL,
    created_at      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_by      BIGINT UNSIGNED NULL,
    updated_at      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    updated_by      BIGINT UNSIGNED NULL,
    deleted_at      DATETIME        NULL,
    deleted_by      BIGINT UNSIGNED NULL,
    is_deleted      TINYINT(1)      NOT NULL DEFAULT 0,
    UNIQUE KEY uq_tasks_uuid (task_uuid),
    KEY idx_tasks_device_status_priority (device_id, task_status_id, priority),
    KEY idx_tasks_type (task_type_id),
    KEY idx_tasks_completed_at (completed_at),
    KEY idx_tasks_status (task_status_id),
    CONSTRAINT fk_tasks_device FOREIGN KEY (device_id) REFERENCES devices (id),
    CONSTRAINT fk_tasks_type FOREIGN KEY (task_type_id) REFERENCES ref_task_types (id),
    CONSTRAINT fk_tasks_status FOREIGN KEY (task_status_id) REFERENCES ref_task_status (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='Antrean RPC CWMP yang harus dieksekusi ke device';

-- =====================================================================
-- 6. PROVISIONING PROFILE & ZERO-TOUCH PROVISIONING
-- =====================================================================

CREATE TABLE provisioning_profiles (
    id                  BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    profile_uuid        CHAR(36)        NOT NULL,
    tenant_id           BIGINT UNSIGNED NULL,
    vendor_id           BIGINT UNSIGNED NULL COMMENT 'NULL = berlaku umum lintas vendor',
    device_model_id     BIGINT UNSIGNED NULL COMMENT 'NULL = berlaku umum lintas model dalam vendor tsb',
    name                VARCHAR(128)    NOT NULL,
    description         VARCHAR(255)    NULL,
    is_default          TINYINT(1)      NOT NULL DEFAULT 0,
    is_active           TINYINT(1)      NOT NULL DEFAULT 1,
    created_at          DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_by          BIGINT UNSIGNED NULL,
    updated_at          DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    updated_by          BIGINT UNSIGNED NULL,
    deleted_at          DATETIME        NULL,
    deleted_by          BIGINT UNSIGNED NULL,
    is_deleted          TINYINT(1)      NOT NULL DEFAULT 0,
    UNIQUE KEY uq_provisioning_profiles_uuid (profile_uuid),
    KEY idx_provisioning_profiles_tenant (tenant_id),
    KEY idx_provisioning_profiles_vendor (vendor_id),
    KEY idx_provisioning_profiles_model (device_model_id),
    CONSTRAINT fk_provisioning_profiles_tenant FOREIGN KEY (tenant_id) REFERENCES tenants (id),
    CONSTRAINT fk_provisioning_profiles_vendor FOREIGN KEY (vendor_id) REFERENCES ref_vendors (id),
    CONSTRAINT fk_provisioning_profiles_model FOREIGN KEY (device_model_id) REFERENCES device_models (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE provisioning_profile_parameters (
    id                  BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    profile_id          BIGINT UNSIGNED NOT NULL,
    parameter_name       VARCHAR(512)    NOT NULL COMMENT 'Logical key ATAU raw TR-069 path',
    parameter_value       TEXT            NULL,
    parameter_type_id     BIGINT UNSIGNED NULL,
    apply_order            INT UNSIGNED    NOT NULL DEFAULT 0,
    created_at              DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at              DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    KEY idx_provisioning_profile_parameters_profile (profile_id),
    CONSTRAINT fk_ppp_profile FOREIGN KEY (profile_id) REFERENCES provisioning_profiles (id),
    CONSTRAINT fk_ppp_parameter_type FOREIGN KEY (parameter_type_id) REFERENCES ref_parameter_types (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE zero_touch_rules (
    id                      BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    tenant_id               BIGINT UNSIGNED NULL,
    vendor_id               BIGINT UNSIGNED NULL,
    device_model_id         BIGINT UNSIGNED NULL,
    oui                     CHAR(6)         NULL,
    serial_pattern          VARCHAR(255)    NULL COMMENT 'Pola SQL LIKE, mis. "ZTE%"',
    -- Kolom precondition tambahan (migrations/0009) -- lihat komentar lengkap
    -- di file migrasi tsb utk alasan desain & batasan (CHECK pairing dsb).
    software_version_pattern VARCHAR(255)   NULL COMMENT 'Pola SQL LIKE thd devices.software_version, dievaluasi sama seperti serial_pattern',
    match_parameter_name     VARCHAR(512)   NULL COMMENT 'Nama device_parameters.parameter_name -- precondition opsional, wajib berpasangan dgn match_parameter_value_pattern',
    match_parameter_value_pattern VARCHAR(255) NULL COMMENT 'Pola SQL LIKE thd device_parameters.parameter_value milik match_parameter_name',
    provisioning_profile_id BIGINT UNSIGNED NULL COMMENT 'NULLABLE sejak migrations/0009 -- rule boleh hanya memicu reboot dan/atau firmware push',
    -- Kolom aksi tambahan (migrations/0009).
    post_apply_reboot       TINYINT(1)      NOT NULL DEFAULT 0 COMMENT 'Jika 1, enqueue task REBOOT setelah aksi lain rule ini diterapkan',
    firmware_file_id        BIGINT UNSIGNED NULL COMMENT 'Jika diisi, enqueue task Download firmware ini ke device yang cocok',
    trigger_event_id        BIGINT UNSIGNED NOT NULL COMMENT 'FK ref_ztp_trigger_event -- kapan rule ini dievaluasi (migrations/0009)',
    priority                INT UNSIGNED    NOT NULL DEFAULT 100 COMMENT 'Angka lebih kecil dievaluasi lebih dulu',
    is_active               TINYINT(1)      NOT NULL DEFAULT 1,
    created_at              DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_by              BIGINT UNSIGNED NULL,
    updated_at              DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    updated_by              BIGINT UNSIGNED NULL,
    deleted_at              DATETIME        NULL,
    deleted_by              BIGINT UNSIGNED NULL,
    is_deleted              TINYINT(1)      NOT NULL DEFAULT 0,
    KEY idx_ztr_tenant (tenant_id),
    KEY idx_ztr_vendor (vendor_id),
    KEY idx_ztr_model (device_model_id),
    KEY idx_ztr_priority (priority),
    KEY idx_ztr_firmware (firmware_file_id),
    KEY idx_ztr_trigger_event (trigger_event_id),
    CONSTRAINT fk_ztr_tenant FOREIGN KEY (tenant_id) REFERENCES tenants (id),
    CONSTRAINT fk_ztr_vendor FOREIGN KEY (vendor_id) REFERENCES ref_vendors (id),
    CONSTRAINT fk_ztr_model FOREIGN KEY (device_model_id) REFERENCES device_models (id),
    CONSTRAINT fk_ztr_profile FOREIGN KEY (provisioning_profile_id) REFERENCES provisioning_profiles (id),
    CONSTRAINT fk_ztr_firmware FOREIGN KEY (firmware_file_id) REFERENCES firmware_files (id),
    CONSTRAINT fk_ztr_trigger_event FOREIGN KEY (trigger_event_id) REFERENCES ref_ztp_trigger_event (id),
    CONSTRAINT chk_ztr_match_parameter_pair CHECK ((match_parameter_name IS NULL) = (match_parameter_value_pattern IS NULL))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='Aturan pencocokan device baru ke provisioning profile / reboot / firmware push, kapan dievaluasi ditentukan trigger_event_id';

-- Sekarang FK provisioning_profile_id di tabel devices bisa ditambahkan
ALTER TABLE devices
    ADD CONSTRAINT fk_devices_provisioning_profile
        FOREIGN KEY (provisioning_profile_id) REFERENCES provisioning_profiles (id);

-- =====================================================================
-- 7. FIRMWARE MANAGEMENT
-- =====================================================================

CREATE TABLE firmware_files (
    id                  BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    firmware_uuid       CHAR(36)        NOT NULL,
    vendor_id           BIGINT UNSIGNED NOT NULL,
    device_model_id     BIGINT UNSIGNED NULL,
    version              VARCHAR(64)     NOT NULL,
    file_name             VARCHAR(255)    NOT NULL,
    storage_key            VARCHAR(500)    NOT NULL COMMENT 'Object key di object storage (MinIO/S3-compatible), bukan URL — presigned URL digenerate on-demand',
    file_size_bytes         BIGINT UNSIGNED NULL,
    checksum_sha256          CHAR(64)        NULL,
    release_notes            TEXT            NULL,
    is_active                 TINYINT(1)      NOT NULL DEFAULT 1,
    created_at                DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_by                BIGINT UNSIGNED NULL,
    updated_at                DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    updated_by                BIGINT UNSIGNED NULL,
    deleted_at                DATETIME        NULL,
    deleted_by                BIGINT UNSIGNED NULL,
    is_deleted                 TINYINT(1)      NOT NULL DEFAULT 0,
    UNIQUE KEY uq_firmware_files_uuid (firmware_uuid),
    KEY idx_firmware_files_vendor (vendor_id),
    KEY idx_firmware_files_model (device_model_id),
    CONSTRAINT fk_firmware_files_vendor FOREIGN KEY (vendor_id) REFERENCES ref_vendors (id),
    CONSTRAINT fk_firmware_files_model FOREIGN KEY (device_model_id) REFERENCES device_models (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE firmware_upgrade_jobs (
    id                  BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    job_uuid             CHAR(36)        NOT NULL,
    device_id            BIGINT UNSIGNED NOT NULL,
    firmware_id           BIGINT UNSIGNED NOT NULL,
    -- rollout_batch_id/wave_number (migrations/0011): link balik opsional ke
    -- firmware_rollout_batches (canary/staged rollout) -- NULL utk job
    -- ScheduleUpgrade satuan (perilaku lama, tidak berubah). Device-tracking
    -- TETAP di tabel ini (bukan diduplikasi ke tabel baru), sesuai TECH.md §11
    -- soal tidak menduplikasi entity yang sudah ada.
    rollout_batch_id      BIGINT UNSIGNED NULL COMMENT 'Diisi bila job dibuat sbg bagian dari firmware_rollout_batches',
    wave_number            INT UNSIGNED   NULL COMMENT 'Wave ke berapa (dlm rollout_batch_id) job ini dibuat -- utk hitung success/failure rate per wave',
    task_id                BIGINT UNSIGNED NULL COMMENT 'Task Download terkait di tabel tasks',
    task_status_id           BIGINT UNSIGNED NOT NULL,
    from_version              VARCHAR(64)     NULL,
    to_version                 VARCHAR(64)     NULL,
    scheduled_at                DATETIME        NULL,
    started_at                   DATETIME        NULL,
    completed_at                  DATETIME        NULL,
    error_message                  TEXT            NULL,
    created_at                      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_by                      BIGINT UNSIGNED NULL,
    updated_at                      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    updated_by                      BIGINT UNSIGNED NULL,
    UNIQUE KEY uq_firmware_upgrade_jobs_uuid (job_uuid),
    KEY idx_fuj_device (device_id),
    KEY idx_fuj_firmware (firmware_id),
    KEY idx_fuj_rollout_batch (rollout_batch_id),
    KEY idx_fuj_rollout_batch_wave (rollout_batch_id, wave_number),
    KEY idx_fuj_task (task_id),
    KEY idx_fuj_status (task_status_id),
    CONSTRAINT fk_fuj_device FOREIGN KEY (device_id) REFERENCES devices (id),
    CONSTRAINT fk_fuj_firmware FOREIGN KEY (firmware_id) REFERENCES firmware_files (id),
    CONSTRAINT fk_fuj_task FOREIGN KEY (task_id) REFERENCES tasks (id),
    CONSTRAINT fk_fuj_status FOREIGN KEY (task_status_id) REFERENCES ref_task_status (id),
    CONSTRAINT fk_fuj_rollout_batch FOREIGN KEY (rollout_batch_id) REFERENCES firmware_rollout_batches (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- firmware_rollout_batches (migrations/0011): rencana canary/staged rollout
-- firmware ke populasi device bertahap per wave. Target populasi dipilih via
-- vendor_id/device_model_id -- bentuk filter yang SAMA dgn domain.DeviceFilter
-- yang sudah dipakai listing device lain (internal/domain/device.go), supaya
-- usecase orchestration (belum diimplementasikan di sini) tinggal reuse
-- DeviceRepository.List utk resolve populasi. Individual per-device job TETAP
-- di firmware_upgrade_jobs (di atas, kolom rollout_batch_id/wave_number) --
-- tidak menduplikasi device-tracking di tabel ini.
CREATE TABLE firmware_rollout_batches (
    id                          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    batch_uuid                  CHAR(36)        NOT NULL,
    tenant_id                   BIGINT UNSIGNED NULL COMMENT 'NULL = rollout lintas-tenant/global',
    firmware_file_id            BIGINT UNSIGNED NOT NULL,
    vendor_id                   BIGINT UNSIGNED NULL COMMENT 'Kriteria seleksi populasi target -- NULL = tidak difilter vendor',
    device_model_id             BIGINT UNSIGNED NULL COMMENT 'Kriteria seleksi populasi target -- NULL = tidak difilter model',
    wave_percentage             TINYINT UNSIGNED NOT NULL DEFAULT 10 COMMENT 'Persentase populasi per wave, mis. 10 = 10% per wave',
    max_failure_rate_percent    TINYINT UNSIGNED NOT NULL DEFAULT 10 COMMENT 'Ambang tingkat gagal per wave (%) sebelum rollout di-pause otomatis',
    current_wave                INT UNSIGNED    NOT NULL DEFAULT 0 COMMENT 'Wave terakhir yang sudah/sedang dijalankan (bookkeeping progres, diperbarui usecase orchestration)',
    status_id                   BIGINT UNSIGNED NOT NULL,
    notes                       VARCHAR(255)    NULL,
    scheduled_at                DATETIME        NULL COMMENT 'Diisi bila batch baru boleh dijalankan (wave 1) setelah waktu tertentu',
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

-- =====================================================================
-- 8. DIAGNOSTICS
-- =====================================================================

CREATE TABLE device_diagnostics (
    id              BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    device_id       BIGINT UNSIGNED NOT NULL,
    task_id         BIGINT UNSIGNED NULL,
    diagnostic_type VARCHAR(64)     NOT NULL COMMENT 'PING, TRACEROUTE, WIFI_SCAN, OPTICAL_POWER, SPEED_TEST',
    status          VARCHAR(32)     NOT NULL DEFAULT 'PENDING',
    result          JSON            NULL,
    executed_at     DATETIME        NULL,
    created_at      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    KEY idx_device_diagnostics_device (device_id),
    KEY idx_device_diagnostics_task (task_id),
    CONSTRAINT fk_device_diagnostics_device FOREIGN KEY (device_id) REFERENCES devices (id),
    CONSTRAINT fk_device_diagnostics_task FOREIGN KEY (task_id) REFERENCES tasks (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

SET FOREIGN_KEY_CHECKS = 1;

-- =====================================================================
-- 9. SEED DATA MINIMAL (ref_* & vendor awal)
-- =====================================================================

INSERT INTO ref_device_types (code, name) VALUES
    ('ONT', 'Optical Network Terminal'),
    ('ONU', 'Optical Network Unit'),
    ('ROUTER', 'Router / Gateway'),
    ('ACCESS_POINT', 'Access Point'),
    ('MODEM', 'Modem'),
    ('STB', 'Set-Top Box');

INSERT INTO ref_data_model_versions (code, name, root_object) VALUES
    ('TR098', 'TR-098 (InternetGatewayDevice)', 'InternetGatewayDevice'),
    ('TR181', 'TR-181 (Device:2)', 'Device');

INSERT INTO ref_event_codes (code, name) VALUES
    ('0 BOOTSTRAP', 'Bootstrap'),
    ('1 BOOT', 'Boot'),
    ('2 PERIODIC', 'Periodic Inform'),
    ('3 SCHEDULED', 'Scheduled Inform'),
    ('4 VALUE CHANGE', 'Value Change'),
    ('5 KICKED', 'Kicked'),
    ('6 CONNECTION REQUEST', 'Connection Request'),
    ('7 TRANSFER COMPLETE', 'Transfer Complete'),
    ('8 DIAGNOSTICS COMPLETE', 'Diagnostics Complete'),
    ('9 REQUEST DOWNLOAD', 'Request Download'),
    ('10 AUTONOMOUS TRANSFER COMPLETE', 'Autonomous Transfer Complete'),
    ('M Reboot', 'ACS-initiated Reboot'),
    ('M ScheduleInform', 'ACS-initiated Schedule Inform'),
    ('M Download', 'ACS-initiated Download'),
    ('M Upload', 'ACS-initiated Upload');

INSERT INTO ref_task_types (code, rpc_method_name, name) VALUES
    ('GET_PARAMETER_VALUES', 'GetParameterValues', 'Ambil nilai parameter'),
    ('SET_PARAMETER_VALUES', 'SetParameterValues', 'Ubah nilai parameter'),
    ('GET_PARAMETER_NAMES', 'GetParameterNames', 'Ambil daftar nama parameter'),
    ('ADD_OBJECT', 'AddObject', 'Tambah instance object'),
    ('DELETE_OBJECT', 'DeleteObject', 'Hapus instance object'),
    ('REBOOT', 'Reboot', 'Reboot perangkat'),
    ('FACTORY_RESET', 'FactoryReset', 'Kembalikan ke setelan pabrik'),
    ('DOWNLOAD', 'Download', 'Transfer file ke perangkat (firmware/config)'),
    ('UPLOAD', 'Upload', 'Transfer file dari perangkat (log/config)'),
    ('SCHEDULE_INFORM', 'ScheduleInform', 'Jadwalkan Inform berikutnya'),
    ('SET_PARAMETER_ATTRIBUTES', 'SetParameterAttributes', 'Ubah atribut notifikasi parameter'),
    ('GET_PARAMETER_ATTRIBUTES', 'GetParameterAttributes', 'Ambil atribut notifikasi parameter');

INSERT INTO ref_task_status (code, name) VALUES
    ('PENDING', 'Menunggu eksekusi'),
    ('QUEUED', 'Dalam antrean sesi'),
    ('SENT', 'Terkirim ke perangkat'),
    ('COMPLETED', 'Berhasil'),
    ('FAILED', 'Gagal'),
    ('CANCELLED', 'Dibatalkan'),
    ('TIMEOUT', 'Time-out menunggu respons');

INSERT INTO ref_device_status (code, name) VALUES
    ('ONLINE', 'Online'),
    ('OFFLINE', 'Offline'),
    ('PROVISIONING', 'Sedang diprovisioning'),
    ('FAULTY', 'Bermasalah'),
    ('UNREGISTERED', 'Belum terdaftar/terprovisioning'),
    ('DECOMMISSIONED', 'Sudah tidak digunakan');

INSERT INTO ref_parameter_types (code) VALUES
    ('string'), ('int'), ('unsignedInt'), ('boolean'), ('dateTime'), ('base64');

INSERT INTO ref_roles (code, name) VALUES
    ('SUPERADMIN', 'Superadmin lintas tenant'),
    ('ADMIN', 'Admin tenant'),
    ('NOC', 'NOC / Operator'),
    ('VIEWER', 'Viewer / read-only');

-- migrations/0009: kapan zero_touch_rules dievaluasi relatif thd event Inform.
INSERT INTO ref_ztp_trigger_event (code, name, description) VALUES
    ('BOOTSTRAP_ONLY', 'Hanya saat Bootstrap',
        'Rule dievaluasi hanya saat event CWMP "0 BOOTSTRAP" -- perilaku default/lama, dipakai sbg nilai backfill rule existing'),
    ('BOOTSTRAP_OR_BOOT', 'Bootstrap atau Boot',
        'Rule dievaluasi saat event "0 BOOTSTRAP" ATAU "1 BOOT"'),
    ('EVERY_INFORM', 'Setiap Inform',
        'Rule dievaluasi pada setiap Inform, tanpa memandang event code');

-- migrations/0011: status lifecycle firmware_rollout_batches.
INSERT INTO ref_firmware_rollout_status (code, name) VALUES
    ('PENDING', 'Menunggu dimulai'),
    ('IN_PROGRESS', 'Sedang berjalan'),
    ('PAUSED_FAILURE_THRESHOLD', 'Dijeda -- ambang gagal wave terlampaui'),
    ('COMPLETED', 'Selesai -- seluruh wave sukses diterapkan'),
    ('CANCELLED', 'Dibatalkan operator');

INSERT INTO ref_vendors (code, name) VALUES
    ('ZTE', 'ZTE Corporation'),
    ('HUAWEI', 'Huawei Technologies'),
    ('FIBERHOME', 'FiberHome Technologies'),
    ('NOKIA', 'Nokia (eks Alcatel-Lucent)');

-- migrations/0004_vendor_baseline_catalog: tambah Cdata + mapping parameter
-- standar TR-098/TR-181. Lihat komentar lengkap di file migrasi tsb untuk
-- daftar logical key yang SENGAJA tidak diisi (optical power, split WiFi
-- 2.4G/5G, wan.ip_address) dan alasannya — jangan tambahkan tanpa konfirmasi
-- dari dokumentasi resmi vendor/akses device nyata. `vendor_ouis` juga
-- sengaja masih kosong untuk kelima vendor ini (belum ada OUI yang
-- terverifikasi dari IEEE OUI registry saat migrasi dibuat).
INSERT INTO ref_vendors (code, name, description) VALUES
    ('CDATA', 'C-Data Technology Co., Ltd', 'Produsen ONU/OLT GPON/EPON, umum dipakai ISP kecil-menengah');

INSERT INTO vendor_parameter_mappings
    (vendor_id, data_model_version_id, device_model_id, logical_key, tr069_path, parameter_type_id, description)
SELECT
    v.id, dmv.id, NULL, k.logical_key, k.tr069_path, pt.id, k.description
FROM ref_vendors v
CROSS JOIN (SELECT id FROM ref_data_model_versions WHERE code = 'TR098') dmv
CROSS JOIN (
    SELECT 'device.manufacturer' AS logical_key, 'InternetGatewayDevice.DeviceInfo.Manufacturer' AS tr069_path, 'string' AS ptype, 'Nama manufacturer perangkat (TR-098 DeviceInfo, standar)' AS description
    UNION ALL SELECT 'device.model_name', 'InternetGatewayDevice.DeviceInfo.ModelName', 'string', 'Nama model perangkat (TR-098 DeviceInfo, standar)'
    UNION ALL SELECT 'device.serial_number', 'InternetGatewayDevice.DeviceInfo.SerialNumber', 'string', 'Serial number perangkat (TR-098 DeviceInfo, standar)'
    UNION ALL SELECT 'device.software_version', 'InternetGatewayDevice.DeviceInfo.SoftwareVersion', 'string', 'Versi firmware/software (TR-098 DeviceInfo, standar)'
    UNION ALL SELECT 'device.hardware_version', 'InternetGatewayDevice.DeviceInfo.HardwareVersion', 'string', 'Versi hardware (TR-098 DeviceInfo, standar)'
    UNION ALL SELECT 'device.uptime', 'InternetGatewayDevice.DeviceInfo.UpTime', 'unsignedInt', 'Waktu sejak boot terakhir dalam detik (TR-098 DeviceInfo, standar)'
    UNION ALL SELECT 'device.periodic_inform_interval', 'InternetGatewayDevice.ManagementServer.PeriodicInformInterval', 'unsignedInt', 'Interval Periodic Inform dalam detik (TR-098 ManagementServer, standar)'
    UNION ALL SELECT 'device.connection_request_url', 'InternetGatewayDevice.ManagementServer.ConnectionRequestURL', 'string', 'URL Connection Request milik CPE (TR-098 ManagementServer, standar)'
    UNION ALL SELECT 'wan.pppoe.username', 'InternetGatewayDevice.WANDevice.1.WANConnectionDevice.1.WANPPPConnection.1.Username', 'string', 'Username PPPoE WAN — asumsi instance .1.1.1, lihat migrations/0004'
    UNION ALL SELECT 'wan.pppoe.password', 'InternetGatewayDevice.WANDevice.1.WANConnectionDevice.1.WANPPPConnection.1.Password', 'string', 'Password PPPoE WAN — asumsi instance .1.1.1, lihat migrations/0004'
    UNION ALL SELECT 'wifi.ssid', 'InternetGatewayDevice.LANDevice.1.WLANConfiguration.1.SSID', 'string', 'SSID WiFi radio utama (instance 1) — tidak dibedakan 2.4G/5G, lihat migrations/0004'
    UNION ALL SELECT 'wifi.wpa_passphrase', 'InternetGatewayDevice.LANDevice.1.WLANConfiguration.1.PreSharedKey.1.KeyPassphrase', 'string', 'WPA/WPA2 passphrase WiFi radio utama (instance 1), object PreSharedKey resmi TR-098 Amendment 2'
) k
LEFT JOIN ref_parameter_types pt ON pt.code = k.ptype
WHERE v.code IN ('ZTE', 'HUAWEI', 'FIBERHOME', 'NOKIA', 'CDATA');

INSERT INTO vendor_parameter_mappings
    (vendor_id, data_model_version_id, device_model_id, logical_key, tr069_path, parameter_type_id, description)
SELECT
    v.id, dmv.id, NULL, k.logical_key, k.tr069_path, pt.id, k.description
FROM ref_vendors v
CROSS JOIN (SELECT id FROM ref_data_model_versions WHERE code = 'TR181') dmv
CROSS JOIN (
    SELECT 'device.manufacturer' AS logical_key, 'Device.DeviceInfo.Manufacturer' AS tr069_path, 'string' AS ptype, 'Nama manufacturer perangkat (TR-181 DeviceInfo, standar)' AS description
    UNION ALL SELECT 'device.model_name', 'Device.DeviceInfo.ModelName', 'string', 'Nama model perangkat (TR-181 DeviceInfo, standar)'
    UNION ALL SELECT 'device.serial_number', 'Device.DeviceInfo.SerialNumber', 'string', 'Serial number perangkat (TR-181 DeviceInfo, standar)'
    UNION ALL SELECT 'device.software_version', 'Device.DeviceInfo.SoftwareVersion', 'string', 'Versi firmware/software (TR-181 DeviceInfo, standar)'
    UNION ALL SELECT 'device.hardware_version', 'Device.DeviceInfo.HardwareVersion', 'string', 'Versi hardware (TR-181 DeviceInfo, standar)'
    UNION ALL SELECT 'device.uptime', 'Device.DeviceInfo.UpTime', 'unsignedInt', 'Waktu sejak boot terakhir dalam detik (TR-181 DeviceInfo, standar)'
    UNION ALL SELECT 'device.periodic_inform_interval', 'Device.ManagementServer.PeriodicInformInterval', 'unsignedInt', 'Interval Periodic Inform dalam detik (TR-181 ManagementServer, standar)'
    UNION ALL SELECT 'device.connection_request_url', 'Device.ManagementServer.ConnectionRequestURL', 'string', 'URL Connection Request milik CPE (TR-181 ManagementServer, standar)'
    UNION ALL SELECT 'wan.pppoe.username', 'Device.PPP.Interface.1.Username', 'string', 'Username PPPoE WAN — asumsi instance .1, lihat migrations/0004'
    UNION ALL SELECT 'wan.pppoe.password', 'Device.PPP.Interface.1.Password', 'string', 'Password PPPoE WAN — asumsi instance .1, lihat migrations/0004'
    UNION ALL SELECT 'wifi.ssid', 'Device.WiFi.SSID.1.SSID', 'string', 'SSID WiFi radio utama (instance 1) — tidak dibedakan 2.4G/5G, lihat migrations/0004'
    UNION ALL SELECT 'wifi.wpa_passphrase', 'Device.WiFi.AccessPoint.1.Security.KeyPassphrase', 'string', 'WPA/WPA2 passphrase WiFi radio utama (instance 1), object AccessPoint.Security resmi TR-181'
) k
LEFT JOIN ref_parameter_types pt ON pt.code = k.ptype
WHERE v.code IN ('ZTE', 'HUAWEI', 'FIBERHOME', 'NOKIA', 'CDATA');

-- =====================================================================
-- Webhook keluar (typed event -> BSS/OSS/NMS) — migrations/0013
-- =====================================================================

CREATE TABLE ref_webhook_event_types (
    id              BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    code            VARCHAR(48)     NOT NULL COMMENT 'DEVICE_FAULT, PARAMETER_VALUE_CHANGE, TASK_FAILED',
    name            VARCHAR(128)    NOT NULL,
    description     VARCHAR(255)    NULL,
    created_at      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_by      BIGINT UNSIGNED NULL,
    updated_at      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    updated_by      BIGINT UNSIGNED NULL,
    deleted_at      DATETIME        NULL,
    deleted_by      BIGINT UNSIGNED NULL,
    is_deleted      TINYINT(1)      NOT NULL DEFAULT 0,
    UNIQUE KEY uq_ref_webhook_event_types_code (code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='Jenis event yang bisa dikirim ACS sebagai webhook (migrations/0013)';

INSERT INTO ref_webhook_event_types (code, name, description) VALUES
    ('DEVICE_FAULT', 'Device Fault',
        'CPE mengirim cwmp:Fault dalam sesi CWMP (RPC gagal di sisi CPE)'),
    ('PARAMETER_VALUE_CHANGE', 'Parameter Value Change',
        'CPE mengirim event CWMP "4 VALUE CHANGE"'),
    ('TASK_FAILED', 'Task Failed',
        'Task queue mencapai status FAILED terminal (habis retry)');

CREATE TABLE webhook_subscriptions (
    id                  BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    subscription_uuid   CHAR(36)        NOT NULL,
    tenant_id           BIGINT UNSIGNED NULL COMMENT 'NULL = subscription global lintas tenant (SUPERADMIN saja)',
    event_type_id       BIGINT UNSIGNED NOT NULL,
    name                VARCHAR(128)    NOT NULL,
    target_url          VARCHAR(500)    NOT NULL,
    secret_enc          VARBINARY(255)  NOT NULL COMMENT 'Secret HMAC-SHA256, AES-GCM at-rest (pkg/cryptoutil)',
    is_active           TINYINT(1)      NOT NULL DEFAULT 1,
    description         VARCHAR(255)    NULL,
    created_at          DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_by          BIGINT UNSIGNED NULL,
    updated_at          DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    updated_by          BIGINT UNSIGNED NULL,
    deleted_at          DATETIME        NULL,
    deleted_by          BIGINT UNSIGNED NULL,
    is_deleted          TINYINT(1)      NOT NULL DEFAULT 0,
    UNIQUE KEY uq_webhook_subscriptions_uuid (subscription_uuid),
    KEY idx_webhook_subscriptions_tenant (tenant_id),
    KEY idx_webhook_subscriptions_dispatch (event_type_id, is_active, is_deleted),
    CONSTRAINT fk_webhook_subscriptions_tenant FOREIGN KEY (tenant_id) REFERENCES tenants (id),
    CONSTRAINT fk_webhook_subscriptions_event_type FOREIGN KEY (event_type_id) REFERENCES ref_webhook_event_types (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='Langganan webhook per (tenant, jenis event, URL target) — migrations/0013';

-- LOG volume tinggi: audit minimal (created_at/updated_at saja), tanpa
-- soft-delete — trade-off eksplisit demi throughput tulis (CLAUDE.md §2).
CREATE TABLE webhook_deliveries (
    id                  BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    delivery_uuid       CHAR(36)        NOT NULL,
    subscription_id     BIGINT UNSIGNED NOT NULL,
    event_type_id       BIGINT UNSIGNED NOT NULL,
    payload             JSON            NOT NULL,
    status              VARCHAR(16)     NOT NULL DEFAULT 'PENDING' COMMENT 'PENDING, DELIVERED, FAILED',
    attempt_count       INT UNSIGNED    NOT NULL DEFAULT 0,
    max_attempts        INT UNSIGNED    NOT NULL DEFAULT 6,
    response_status     INT             NULL,
    error_message       VARCHAR(500)    NULL,
    next_attempt_at     DATETIME        NULL,
    delivered_at        DATETIME        NULL,
    created_at          DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at          DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY uq_webhook_deliveries_uuid (delivery_uuid),
    KEY idx_webhook_deliveries_subscription (subscription_id),
    KEY idx_webhook_deliveries_worker (status, next_attempt_at),
    CONSTRAINT fk_webhook_deliveries_subscription FOREIGN KEY (subscription_id) REFERENCES webhook_subscriptions (id),
    CONSTRAINT fk_webhook_deliveries_event_type FOREIGN KEY (event_type_id) REFERENCES ref_webhook_event_types (id),
    CONSTRAINT chk_webhook_deliveries_status CHECK (status IN ('PENDING', 'DELIVERED', 'FAILED'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='Log setiap percobaan pengiriman webhook (audit minimal) — migrations/0013';

-- =====================================================================
-- 12. SNAPSHOT KONFIG, GEOLOKASI, FILE GENERIK, TAG/PRESET, SELF-SERVICE
--     (migrations/0014–0020)
-- =====================================================================

-- migrations/0014: snapshot konfigurasi device (JSON penuh parameter) untuk
-- diff/rollback manual. LOG-ish: audit minimal (created_at saja).
CREATE TABLE device_config_snapshots (
    id              BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    device_id       BIGINT UNSIGNED NOT NULL,
    snapshot_data   JSON            NOT NULL,
    created_at      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    KEY idx_device_config_snapshots_device (device_id),
    CONSTRAINT fk_device_config_snapshots_device FOREIGN KEY (device_id) REFERENCES devices (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='Snapshot konfigurasi device untuk diff/rollback (migrations/0014)';

-- migrations/0015: geolokasi device untuk peta NOC (DeviceMap.tsx).
ALTER TABLE devices
    ADD COLUMN latitude  DECIMAL(10, 8) NULL AFTER notes,
    ADD COLUMN longitude DECIMAL(11, 8) NULL AFTER latitude;

-- migrations/0017: indeks performa tambahan (hot-path list/sweeper).
ALTER TABLE webhook_deliveries
    ADD KEY idx_webhook_deliveries_sub_status (subscription_id, status, created_at);
ALTER TABLE device_sessions
    ADD KEY idx_device_sessions_status_started (status, started_at);

-- migrations/0018: katalog file generik (bukan cuma firmware) — web content,
-- vendor config file, dll. Audit 7-kolom + UUID publik.
CREATE TABLE files (
    id                  BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    file_uuid           CHAR(36)        NOT NULL,
    tenant_id           BIGINT UNSIGNED NULL COMMENT 'NULL = global lintas tenant',
    file_type           VARCHAR(100)    NOT NULL COMMENT '1 Firmware Upgrade Image, 2 Web Content, 3 Vendor Configuration File, dll',
    vendor_id           BIGINT UNSIGNED NULL,
    device_model_id     BIGINT UNSIGNED NULL,
    version             VARCHAR(100)    NULL,
    file_name           VARCHAR(255)    NOT NULL,
    storage_key         VARCHAR(512)    NOT NULL COMMENT 'Object key MinIO/S3',
    file_size_bytes     BIGINT UNSIGNED NOT NULL,
    created_at          DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_by          BIGINT UNSIGNED NULL,
    updated_at          DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    updated_by          BIGINT UNSIGNED NULL,
    deleted_at          DATETIME        NULL,
    deleted_by          BIGINT UNSIGNED NULL,
    is_deleted          TINYINT(1)      NOT NULL DEFAULT 0,
    UNIQUE KEY uq_files_uuid (file_uuid),
    KEY idx_files_tenant_type (tenant_id, file_type),
    KEY idx_files_vendor_model (vendor_id, device_model_id),
    CONSTRAINT fk_files_tenant FOREIGN KEY (tenant_id)       REFERENCES tenants (id)       ON DELETE RESTRICT,
    CONSTRAINT fk_files_vendor FOREIGN KEY (vendor_id)       REFERENCES ref_vendors (id)   ON DELETE RESTRICT,
    CONSTRAINT fk_files_model  FOREIGN KEY (device_model_id) REFERENCES device_models (id) ON DELETE RESTRICT,
    CONSTRAINT fk_files_cb     FOREIGN KEY (created_by)      REFERENCES users (id)         ON DELETE SET NULL,
    CONSTRAINT fk_files_ub     FOREIGN KEY (updated_by)      REFERENCES users (id)         ON DELETE SET NULL,
    CONSTRAINT fk_files_db     FOREIGN KEY (deleted_by)      REFERENCES users (id)         ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='Katalog file generik untuk Download RPC CWMP (migrations/0018)';

-- migrations/0019: tag device + preset (aturan match+config gaya GenieACS).
CREATE TABLE tags (
    id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    tenant_id   BIGINT UNSIGNED NULL,
    name        VARCHAR(255)    NOT NULL,
    color       VARCHAR(7)      NULL,
    created_at  DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY uq_tags_name_tenant (name, tenant_id),
    CONSTRAINT fk_tags_tenant FOREIGN KEY (tenant_id) REFERENCES tenants (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='Tag device untuk segmentasi & precondition preset (migrations/0019)';

CREATE TABLE device_tags (
    device_id   BIGINT UNSIGNED NOT NULL,
    tag_id      BIGINT UNSIGNED NOT NULL,
    created_at  DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (device_id, tag_id),
    CONSTRAINT fk_device_tags_device FOREIGN KEY (device_id) REFERENCES devices (id) ON DELETE CASCADE,
    CONSTRAINT fk_device_tags_tag    FOREIGN KEY (tag_id)    REFERENCES tags (id)    ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='Relasi many-to-many device <-> tag (migrations/0019)';

CREATE TABLE presets (
    id              BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    tenant_id       BIGINT UNSIGNED NULL,
    name            VARCHAR(255)    NOT NULL,
    weight          INT             NOT NULL DEFAULT 0,
    precondition    JSON            NOT NULL COMMENT 'Aturan pencocokan (mis. berdasar model / tag)',
    configurations  JSON            NOT NULL COMMENT 'Aturan set parameter yang diterapkan bila precondition cocok',
    is_active       TINYINT(1)      NOT NULL DEFAULT 1,
    created_at      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    CONSTRAINT fk_presets_tenant FOREIGN KEY (tenant_id) REFERENCES tenants (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='Preset match+config gaya GenieACS (migrations/0019)';

-- migrations/0020: role ENDUSER + mapping akun -> device untuk portal self-service.
INSERT INTO ref_roles (code, name) VALUES ('ENDUSER', 'Pelanggan akhir (portal self-service)');

CREATE TABLE user_devices (
    user_id     BIGINT UNSIGNED NOT NULL,
    device_id   BIGINT UNSIGNED NOT NULL,
    created_at  DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_by  BIGINT UNSIGNED NULL,
    PRIMARY KEY (user_id, device_id),
    KEY idx_user_devices_device (device_id),
    CONSTRAINT fk_user_devices_user   FOREIGN KEY (user_id)   REFERENCES users (id)   ON DELETE CASCADE,
    CONSTRAINT fk_user_devices_device FOREIGN KEY (device_id) REFERENCES devices (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='Mapping akun ENDUSER -> device di portal self-service (migrations/0020)';
