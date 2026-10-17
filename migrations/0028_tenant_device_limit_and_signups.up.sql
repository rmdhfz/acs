-- 0028_tenant_device_limit_and_signups
--
--  - tenants.max_devices : batas jumlah perangkat aktif per tenant (dasar paket/plan SaaS). NULL = tak dibatasi
--                          (tenant lama tidak berubah). Ditegakkan saat Inform perangkat baru, pra-registrasi, dan impor.
--  - tenant_signups      : pendaftaran tenant mandiri yang MENUNGGU verifikasi email. Tenant & admin dibuat hanya setelah
--                          tautan verifikasi dipakai, sehingga pendaftaran palsu tidak mengotori tabel tenants/users.
-- Pengecualian audit 7-kolom (CLAUDE.md §2): tenant_signups adalah antrean verifikasi berumur pendek yang dipangkas
-- (bukan data master); jejak audit ada di activity_logs (SIGNUP_REQUESTED / SIGNUP_VERIFIED) dan baris tenant/user hasilnya.
-- Hanya HASH token yang disimpan; password disimpan sebagai hash Argon2id.

ALTER TABLE tenants
    ADD COLUMN max_devices INT UNSIGNED NULL COMMENT 'Batas perangkat aktif tenant (NULL = tak dibatasi)' AFTER max_pending_tasks;

CREATE TABLE tenant_signups (
    id            BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    signup_uuid   CHAR(36)        NOT NULL,
    company_name  VARCHAR(128)    NOT NULL,
    tenant_code   VARCHAR(32)     NOT NULL COMMENT 'Kode tenant yang akan dibuat (unik saat verifikasi)',
    email         VARCHAR(191)    NOT NULL,
    full_name     VARCHAR(128)    NOT NULL,
    password_hash VARCHAR(255)    NOT NULL,
    token_hash    CHAR(64)        NOT NULL COMMENT 'SHA-256 heksadesimal dari token verifikasi (token asli hanya ada di email)',
    ip_address    VARCHAR(45)     NULL,
    expires_at    DATETIME        NOT NULL,
    verified_at   DATETIME        NULL,
    created_at    DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE KEY uq_signup_uuid (signup_uuid),
    UNIQUE KEY uq_signup_token (token_hash),
    KEY idx_signup_email (email, created_at),
    KEY idx_signup_ip (ip_address, created_at),
    KEY idx_signup_created (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='Pendaftaran tenant mandiri menunggu verifikasi email. Pengecualian audit 7-kolom: lihat header migrasi 0028';
