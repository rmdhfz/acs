-- 0032_billing
--
-- Pembayaran langganan SaaS lewat Xendit (Invoice API). Bawaan fitur MATI (ACS_BILLING_ENABLED=false); tabel ini
-- tetap dibuat agar daftar paket bisa dikelola sebelum pembayaran diaktifkan.
--
--  - billing_plans          : katalog paket langganan (kode, batas perangkat, harga rupiah bulat, lama periode).
--  - ref_invoice_status     : status invoice (PENDING / PAID / EXPIRED / FAILED) -- pola ref_* (CLAUDE.md §1).
--  - billing_invoices       : satu baris per invoice. NOMINAL (amount_idr) disalin dari paket SAAT invoice dibuat dan
--                             menjadi satu-satunya sumber kebenaran nominal; tidak pernah berasal dari klien/payload webhook.
--  - billing_webhook_events : log idempotensi callback Xendit (kunci = SHA-256 "<xendit_invoice_id>|<status>").
--  - tenants.billing_plan_id / plan_expires_at : paket aktif tenant dan kapan masa berlakunya habis.
--  - ref_notification_types : jenis notifikasi BILLING_PAID / BILLING_EXPIRING / BILLING_EXPIRED / BILLING_FAILED.
--
-- PENGECUALIAN audit 7-kolom (CLAUDE.md §2) -- disengaja, bukan default diam-diam:
--   * billing_webhook_events : tabel LOG idempotensi (satu baris per callback yang diproses) -> audit MINIMAL, hanya
--                              received_at (setara created_at). Tidak ada soft delete. Siapa/apa yang berubah akibat
--                              callback tercatat di activity_logs (BILLING_PAID, dst.) dan billing_invoices.
-- billing_plans, billing_invoices, ref_invoice_status memakai audit 7-kolom penuh.
--
-- Uang selalu bilangan bulat rupiah (BIGINT UNSIGNED) -- tanpa FLOAT/DECIMAL.

CREATE TABLE billing_plans (
    id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    plan_code   VARCHAR(32)     NOT NULL COMMENT 'Kode paket, mis. STARTER (^[A-Z][A-Z0-9_]{1,31}$)',
    name        VARCHAR(64)     NOT NULL,
    description VARCHAR(255)    NOT NULL DEFAULT '',
    max_devices INT UNSIGNED    NULL COMMENT 'Batas perangkat aktif; NULL = tak dibatasi. Disalin ke tenants.max_devices saat pembayaran lunas',
    price_idr   BIGINT UNSIGNED NOT NULL COMMENT 'Harga per periode dalam rupiah (bilangan bulat)',
    period_days SMALLINT UNSIGNED NOT NULL DEFAULT 30 COMMENT 'Lama satu periode langganan (hari)',
    is_active   TINYINT(1)      NOT NULL DEFAULT 1 COMMENT '0 = tidak bisa dibeli baru (tenant lama tetap berjalan sampai masa berlaku habis)',
    sort_order  INT             NOT NULL DEFAULT 0,
    created_at  DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_by  BIGINT UNSIGNED NULL,
    updated_at  DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    updated_by  BIGINT UNSIGNED NULL,
    deleted_at  DATETIME        NULL,
    deleted_by  BIGINT UNSIGNED NULL,
    is_deleted  TINYINT(1)      NOT NULL DEFAULT 0,
    UNIQUE KEY uq_billing_plans_code (plan_code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='Katalog paket langganan SaaS (migrations/0032)';

-- !!! HARGA & BATAS DI BAWAH HANYA CONTOH !!!
-- Angka ini placeholder untuk pengembangan/UAT dan WAJIB disesuaikan pemilik produk sebelum billing diaktifkan di
-- produksi (ubah lewat PATCH /api/v1/billing/plans/:code oleh SUPERADMIN, atau UPDATE data langsung).
INSERT INTO billing_plans (plan_code, name, description, max_devices, price_idr, period_days, is_active, sort_order) VALUES
    ('STARTER',    'Starter',    'CONTOH: untuk ISP kecil, hingga 50 perangkat.',       50,   199000, 30, 1, 10),
    ('BUSINESS',   'Business',   'CONTOH: untuk ISP menengah, hingga 500 perangkat.',   500,  899000, 30, 1, 20),
    ('ENTERPRISE', 'Enterprise', 'CONTOH: tanpa batas perangkat, dukungan prioritas.',  NULL, 2499000, 30, 1, 30);

CREATE TABLE ref_invoice_status (
    id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    code        VARCHAR(32)     NOT NULL COMMENT 'PENDING, PAID, EXPIRED, FAILED',
    name        VARCHAR(128)    NOT NULL,
    created_at  DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_by  BIGINT UNSIGNED NULL,
    updated_at  DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    updated_by  BIGINT UNSIGNED NULL,
    deleted_at  DATETIME        NULL,
    deleted_by  BIGINT UNSIGNED NULL,
    is_deleted  TINYINT(1)      NOT NULL DEFAULT 0,
    UNIQUE KEY uq_ref_invoice_status_code (code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='Status invoice langganan (migrations/0032)';

INSERT INTO ref_invoice_status (code, name) VALUES
    ('PENDING', 'Menunggu pembayaran'),
    ('PAID',    'Lunas'),
    ('EXPIRED', 'Kedaluwarsa'),
    ('FAILED',  'Gagal dibuat');

CREATE TABLE billing_invoices (
    id                BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    invoice_uuid      CHAR(36)        NOT NULL COMMENT 'Identifier publik di API; PK tidak pernah diekspos',
    invoice_number    VARCHAR(64)     NOT NULL COMMENT '= external_id di Xendit: ACS-<tenant_id>-<unixnano>-<acak>',
    tenant_id         BIGINT UNSIGNED NOT NULL,
    plan_id           BIGINT UNSIGNED NOT NULL,
    amount_idr        BIGINT UNSIGNED NOT NULL COMMENT 'Nominal disalin dari paket saat dibuat -- sumber kebenaran nominal',
    status_id         BIGINT UNSIGNED NOT NULL,
    xendit_invoice_id VARCHAR(64)     NULL COMMENT 'id invoice di Xendit; NULL sampai gateway menjawab',
    payment_url       VARCHAR(512)    NULL COMMENT 'URL halaman bayar Xendit (https, host xendit.co); hanya diberikan ke klien saat PENDING',
    period_days       SMALLINT UNSIGNED NOT NULL COMMENT 'Salinan plan.period_days saat dibuat',
    expires_at        DATETIME        NOT NULL,
    paid_at           DATETIME        NULL,
    paid_amount_idr   BIGINT UNSIGNED NULL,
    created_at        DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_by        BIGINT UNSIGNED NULL COMMENT 'users.id pembuat; NULL = sistem',
    updated_at        DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    updated_by        BIGINT UNSIGNED NULL,
    deleted_at        DATETIME        NULL,
    deleted_by        BIGINT UNSIGNED NULL,
    is_deleted        TINYINT(1)      NOT NULL DEFAULT 0,
    UNIQUE KEY uq_billing_invoices_uuid (invoice_uuid),
    UNIQUE KEY uq_billing_invoices_number (invoice_number),
    UNIQUE KEY uq_billing_invoices_xendit (xendit_invoice_id),
    KEY idx_billing_invoices_tenant_status (tenant_id, status_id, id),
    KEY idx_billing_invoices_status_expiry (status_id, expires_at),
    CONSTRAINT fk_billing_invoices_tenant FOREIGN KEY (tenant_id) REFERENCES tenants (id),
    CONSTRAINT fk_billing_invoices_plan FOREIGN KEY (plan_id) REFERENCES billing_plans (id),
    CONSTRAINT fk_billing_invoices_status FOREIGN KEY (status_id) REFERENCES ref_invoice_status (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='Invoice langganan SaaS (Xendit). Nominal dari DB, bukan dari klien (migrations/0032)';

CREATE TABLE billing_webhook_events (
    id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    event_key   CHAR(64)        NOT NULL COMMENT 'SHA-256 heksadesimal dari "<xendit_invoice_id>|<status>"',
    invoice_id  BIGINT UNSIGNED NULL,
    outcome     VARCHAR(32)     NOT NULL COMMENT 'PAID, EXPIRED, dst. -- hasil pemrosesan callback',
    received_at DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE KEY uq_billing_webhook_events_key (event_key),
    KEY idx_billing_webhook_events_invoice (invoice_id),
    CONSTRAINT fk_billing_webhook_events_invoice FOREIGN KEY (invoice_id) REFERENCES billing_invoices (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='Log idempotensi callback Xendit. Pengecualian audit 7-kolom (tabel log; hanya received_at): lihat header migrations/0032';

ALTER TABLE tenants
    ADD COLUMN billing_plan_id BIGINT UNSIGNED NULL COMMENT 'Paket langganan aktif terakhir (migrations/0032)' AFTER max_devices,
    ADD COLUMN plan_expires_at DATETIME NULL COMMENT 'Masa berlaku langganan habis pada; NULL = belum pernah berlangganan' AFTER billing_plan_id,
    ADD CONSTRAINT fk_tenants_billing_plan FOREIGN KEY (billing_plan_id) REFERENCES billing_plans (id);

INSERT INTO ref_notification_types (code, name, description, severity_id, default_in_app, default_email, is_mandatory) VALUES
    ('BILLING_PAID', 'Pembayaran langganan berhasil', 'Pembayaran invoice langganan diterima dan masa berlaku diperpanjang.',
        (SELECT id FROM ref_notification_severity WHERE code = 'INFO'), 1, 1, 0),
    ('BILLING_EXPIRING', 'Langganan segera berakhir', 'Masa berlaku langganan tenant kurang dari 7 hari lagi.',
        (SELECT id FROM ref_notification_severity WHERE code = 'WARNING'), 1, 1, 0),
    ('BILLING_EXPIRED', 'Langganan telah berakhir', 'Masa berlaku langganan tenant telah lewat.',
        (SELECT id FROM ref_notification_severity WHERE code = 'WARNING'), 1, 1, 0),
    ('BILLING_FAILED', 'Invoice langganan gagal/kedaluwarsa', 'Invoice langganan kedaluwarsa tanpa pembayaran.',
        (SELECT id FROM ref_notification_severity WHERE code = 'WARNING'), 1, 1, 0);
