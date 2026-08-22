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
    UNIQUE KEY uq_vpm_scope_key (vendor_id, data_model_version_id, device_model_id, logical_key),
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
    status          VARCHAR(16)     NOT NULL DEFAULT 'OPEN' COMMENT 'OPEN, CLOSED, ERROR',
    remote_ip       VARCHAR(45)     NULL,
    started_at      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    ended_at        DATETIME        NULL,
    created_at      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    KEY idx_device_sessions_device (device_id),
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
    provisioning_profile_id BIGINT UNSIGNED NOT NULL,
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
    CONSTRAINT fk_ztr_tenant FOREIGN KEY (tenant_id) REFERENCES tenants (id),
    CONSTRAINT fk_ztr_vendor FOREIGN KEY (vendor_id) REFERENCES ref_vendors (id),
    CONSTRAINT fk_ztr_model FOREIGN KEY (device_model_id) REFERENCES device_models (id),
    CONSTRAINT fk_ztr_profile FOREIGN KEY (provisioning_profile_id) REFERENCES provisioning_profiles (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='Aturan pencocokan device baru (event BOOTSTRAP) ke provisioning profile';

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
    file_path              VARCHAR(500)    NOT NULL,
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
    KEY idx_fuj_task (task_id),
    KEY idx_fuj_status (task_status_id),
    CONSTRAINT fk_fuj_device FOREIGN KEY (device_id) REFERENCES devices (id),
    CONSTRAINT fk_fuj_firmware FOREIGN KEY (firmware_id) REFERENCES firmware_files (id),
    CONSTRAINT fk_fuj_task FOREIGN KEY (task_id) REFERENCES tasks (id),
    CONSTRAINT fk_fuj_status FOREIGN KEY (task_status_id) REFERENCES ref_task_status (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

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

INSERT INTO ref_vendors (code, name) VALUES
    ('ZTE', 'ZTE Corporation'),
    ('HUAWEI', 'Huawei Technologies'),
    ('FIBERHOME', 'FiberHome Technologies'),
    ('NOKIA', 'Nokia (eks Alcatel-Lucent)');
