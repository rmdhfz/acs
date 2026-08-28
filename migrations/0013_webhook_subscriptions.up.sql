-- =====================================================================
-- 0013_webhook_subscriptions
--
-- Konteks: `/goal` directive #3 ("Implement robust webhook mechanisms for
-- device fault alerts and parameter value changes") + `PRD.md` §4.1
-- ("REST API internal dikonsumsi sistem lain ... BSS/OSS, portal NOC").
-- Sebelum ini ACS TIDAK punya mekanisme push keluar sama sekali — sistem
-- pihak ketiga harus polling. Ini menambahkan event webhook bertipe
-- (typed), dengan HMAC signing + delivery log + retry berjenjang.
--
-- GenieACS tidak punya webhook event bertipe sebagai fitur inti (hanya
-- generic provision/extension script) — ini diferensiator integrasi.
--
-- Tabel:
--   1. ref_webhook_event_types (ref_*, BUKAN enum kolom): jenis event yang
--      bisa di-subscribe. Diseed HANYA untuk event yang benar-benar sudah
--      di-wire di usecase (bukan diseed di depan lalu menyesatkan):
--        - DEVICE_FAULT           : cwmp:Fault diterima dari CPE dalam sesi
--        - PARAMETER_VALUE_CHANGE : event CWMP "4 VALUE CHANGE" dari CPE
--        - TASK_FAILED            : task queue mencapai status FAILED terminal
--   2. webhook_subscriptions: master table (audit 7-kolom + subscription_uuid
--      publik). SATU baris = SATU (tenant, event type, target URL). Operator
--      yang mau >1 event bikin >1 subscription — konsisten dgn cara ref_*
--      dipakai di tabel lain & menjaga query "subscription mana yang cocok
--      event X" tetap flat (index (event_type_id, is_deleted)). secret_enc
--      = secret HMAC-SHA256 (dikirim balik ke client SEKALI saat create,
--      disimpan terenkripsi AES-GCM spt tenants.cwmp_inform_password_enc —
--      lihat pkg/cryptoutil). tenant_id NULL = subscription global lintas
--      tenant, hanya SUPERADMIN yang boleh membuatnya (ditegakkan usecase).
--   3. webhook_deliveries: LOG volume tinggi — audit MINIMAL (created_at/
--      updated_at saja, TANPA 7 kolom penuh & TANPA soft-delete), sama
--      seperti device_events/device_sessions (trade-off eksplisit demi
--      throughput tulis, CLAUDE.md §2 & TECH.md §9). status VARCHAR + CHECK
--      bukan tabel ref_ terpisah — mengikuti preseden device_sessions.status
--      (log-ish, nilai stabil & sedikit). Retry: worker periodik cmd/acsd
--      memproses baris PENDING yg next_attempt_at <= NOW(), backoff
--      eksponensial, sampai attempt_count = max_attempts lalu FAILED
--      permanen.
-- =====================================================================

-- ---------------------------------------------------------------------
-- 1. ref_webhook_event_types
-- ---------------------------------------------------------------------
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
        'CPE mengirim event CWMP "4 VALUE CHANGE" — satu/lebih parameter berubah nilainya di sisi CPE'),
    ('TASK_FAILED', 'Task Failed',
        'Task queue mencapai status FAILED terminal (habis retry atau gagal permanen)');

-- ---------------------------------------------------------------------
-- 2. webhook_subscriptions
-- ---------------------------------------------------------------------
CREATE TABLE webhook_subscriptions (
    id                  BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    subscription_uuid   CHAR(36)        NOT NULL,
    tenant_id           BIGINT UNSIGNED NULL COMMENT 'NULL = subscription global lintas tenant (SUPERADMIN saja)',
    event_type_id       BIGINT UNSIGNED NOT NULL,
    name                VARCHAR(128)    NOT NULL,
    target_url          VARCHAR(500)    NOT NULL COMMENT 'URL HTTPS endpoint penerima; ACS POST JSON ke sini',
    secret_enc          VARBINARY(255)  NOT NULL COMMENT 'Secret HMAC-SHA256 (AES-GCM at-rest, pkg/cryptoutil) — dikirim plaintext ke client HANYA saat create',
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

-- ---------------------------------------------------------------------
-- 3. webhook_deliveries  (LOG volume tinggi — audit minimal, no soft-delete)
-- ---------------------------------------------------------------------
CREATE TABLE webhook_deliveries (
    id                  BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    delivery_uuid       CHAR(36)        NOT NULL,
    subscription_id     BIGINT UNSIGNED NOT NULL,
    event_type_id       BIGINT UNSIGNED NOT NULL COMMENT 'Denormalisasi dari subscription — mempermudah query log per jenis event',
    payload             JSON            NOT NULL COMMENT 'Body JSON yang di-POST ke target_url',
    status              VARCHAR(16)     NOT NULL DEFAULT 'PENDING'
                        COMMENT 'PENDING (menunggu/antre retry), DELIVERED (2xx), FAILED (habis attempt). Pola VARCHAR+CHECK spt device_sessions.status, bukan tabel ref_ terpisah',
    attempt_count       INT UNSIGNED    NOT NULL DEFAULT 0,
    max_attempts        INT UNSIGNED    NOT NULL DEFAULT 6,
    response_status     INT             NULL COMMENT 'HTTP status code terakhir dari target (NULL bila belum pernah / error koneksi)',
    error_message       VARCHAR(500)    NULL,
    next_attempt_at     DATETIME        NULL COMMENT 'Kapan worker boleh mencoba lagi (NULL saat DELIVERED/FAILED)',
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
  COMMENT='Log setiap percobaan pengiriman webhook (audit minimal, high-volume) — migrations/0013';
