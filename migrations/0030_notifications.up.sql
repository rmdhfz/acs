-- 0030_notifications
--
-- Sistem notifikasi persisten (in-app + email) menggantikan inferensi sisi-klien di Console.
--
--  - ref_notification_severity : tingkat keparahan (INFO / WARNING / CRITICAL) -- pola ref_* (CLAUDE.md §1).
--  - ref_notification_types    : katalog jenis notifikasi + bawaan kanal (in-app/email) + penanda wajib (is_mandatory:
--                                tak boleh dimatikan pengguna). Menambah jenis baru = operasi data (INSERT), bukan skema.
--  - notifications             : satu baris per (penerima, kejadian). Diekspos ke API lewat notification_uuid;
--                                PK auto-increment tidak pernah keluar.
--  - notification_preferences  : pilihan kanal per (pengguna, jenis); baris tidak ada = pakai bawaan jenis.
--
-- PENGECUALIAN audit 7-kolom (CLAUDE.md §2) -- disengaja, bukan default diam-diam:
--   * notifications            : tabel LOG volume tinggi (satu baris per penerima per kejadian) -> audit MINIMAL, hanya
--                                created_at. Tidak ada soft delete; baris lama dihapus FISIK oleh job retensi
--                                (ACS_LOG_RETENTION_DAYS, retention_repository.go) -- cleanup log sesuai kebijakan retensi
--                                eksplisit yang diizinkan CLAUDE.md §3. Siapa pemicu kejadian ada di activity_logs.
--   * notification_preferences : data preferensi milik pengguna sendiri (bukan master/transaksional bisnis) -> audit
--                                minimal created_at/updated_at; pemilik baris = user_id itu sendiri.
-- Tabel ref_* tetap memakai audit 7-kolom penuh.
--
-- Isi notifikasi TIDAK PERNAH memuat password/token/rahasia (ditegakkan di usecase/notification).

CREATE TABLE ref_notification_severity (
    id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    code        VARCHAR(32)     NOT NULL COMMENT 'INFO, WARNING, CRITICAL',
    name        VARCHAR(128)    NOT NULL,
    created_at  DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_by  BIGINT UNSIGNED NULL,
    updated_at  DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    updated_by  BIGINT UNSIGNED NULL,
    deleted_at  DATETIME        NULL,
    deleted_by  BIGINT UNSIGNED NULL,
    is_deleted  TINYINT(1)      NOT NULL DEFAULT 0,
    UNIQUE KEY uq_ref_notification_severity_code (code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='Tingkat keparahan notifikasi (migrations/0030)';

INSERT INTO ref_notification_severity (code, name) VALUES
    ('INFO', 'Informasi'),
    ('WARNING', 'Peringatan'),
    ('CRITICAL', 'Kritis');

CREATE TABLE ref_notification_types (
    id               BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    code             VARCHAR(48)     NOT NULL COMMENT 'SECURITY_ALERT, DEVICE_OFFLINE_SPIKE, TASK_FAILURES, ROLLOUT_PAUSED, DEVICE_LIMIT_REACHED, NEW_TENANT_SIGNUP, ANNOUNCEMENT',
    name             VARCHAR(128)    NOT NULL,
    description      VARCHAR(255)    NOT NULL DEFAULT '',
    severity_id      BIGINT UNSIGNED NOT NULL COMMENT 'Keparahan bawaan (dapat ditimpa per kejadian)',
    default_in_app   TINYINT(1)      NOT NULL DEFAULT 1,
    default_email    TINYINT(1)      NOT NULL DEFAULT 0,
    is_mandatory     TINYINT(1)      NOT NULL DEFAULT 0 COMMENT '1 = pengguna tidak boleh mematikan kanal jenis ini',
    created_at       DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_by       BIGINT UNSIGNED NULL,
    updated_at       DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    updated_by       BIGINT UNSIGNED NULL,
    deleted_at       DATETIME        NULL,
    deleted_by       BIGINT UNSIGNED NULL,
    is_deleted       TINYINT(1)      NOT NULL DEFAULT 0,
    UNIQUE KEY uq_ref_notification_types_code (code),
    CONSTRAINT fk_ref_notification_types_severity FOREIGN KEY (severity_id) REFERENCES ref_notification_severity (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='Katalog jenis notifikasi + bawaan kanal (migrations/0030)';

INSERT INTO ref_notification_types (code, name, description, severity_id, default_in_app, default_email, is_mandatory) VALUES
    ('SECURITY_ALERT', 'Peringatan keamanan', 'Kejadian keamanan pada akun Anda: akun terkunci, perubahan password, perubahan MFA, login SSO, token API, atau akses dukungan.',
        (SELECT id FROM ref_notification_severity WHERE code = 'INFO'), 1, 1, 1),
    ('DEVICE_OFFLINE_SPIKE', 'Lonjakan perangkat offline', 'Sebagian besar perangkat tenant tiba-tiba offline.',
        (SELECT id FROM ref_notification_severity WHERE code = 'WARNING'), 1, 1, 0),
    ('TASK_FAILURES', 'Kegagalan task beruntun', 'Banyak task gagal dalam waktu singkat.',
        (SELECT id FROM ref_notification_severity WHERE code = 'WARNING'), 1, 0, 0),
    ('ROLLOUT_PAUSED', 'Rollout firmware dijeda', 'Rollout firmware dijeda otomatis karena ambang kegagalan terlampaui.',
        (SELECT id FROM ref_notification_severity WHERE code = 'CRITICAL'), 1, 1, 0),
    ('DEVICE_LIMIT_REACHED', 'Batas perangkat tercapai', 'Perangkat baru ditolak karena batas perangkat tenant sudah penuh.',
        (SELECT id FROM ref_notification_severity WHERE code = 'WARNING'), 1, 1, 0),
    ('NEW_TENANT_SIGNUP', 'Tenant baru mendaftar', 'Tenant baru selesai mendaftar mandiri dan terverifikasi.',
        (SELECT id FROM ref_notification_severity WHERE code = 'INFO'), 1, 1, 0),
    ('ANNOUNCEMENT', 'Pengumuman', 'Pengumuman dari operator platform.',
        (SELECT id FROM ref_notification_severity WHERE code = 'INFO'), 1, 1, 0);

CREATE TABLE notifications (
    id                BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    notification_uuid CHAR(36)        NOT NULL COMMENT 'Identifier publik di API; PK tidak pernah diekspos',
    tenant_id         BIGINT UNSIGNED NULL COMMENT 'Tenant konteks kejadian; NULL = tingkat platform',
    user_id           BIGINT UNSIGNED NOT NULL COMMENT 'Penerima',
    type_id           BIGINT UNSIGNED NOT NULL,
    severity_id       BIGINT UNSIGNED NOT NULL,
    title             VARCHAR(160)    NOT NULL,
    body              VARCHAR(1000)   NOT NULL,
    link              VARCHAR(255)    NULL COMMENT 'Path internal Console (diawali /), mis. /devices/12',
    entity_type       VARCHAR(32)     NULL,
    entity_id         BIGINT UNSIGNED NULL,
    dedupe_key        VARCHAR(128)    NULL COMMENT 'Kunci penekan duplikat per (type, tenant) dalam jendela waktu',
    read_at           DATETIME        NULL,
    emailed_at        DATETIME        NULL COMMENT 'Waktu email diantrekan (dasar throttle 1 email per user+jenis per jam)',
    created_at        DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE KEY uq_notifications_uuid (notification_uuid),
    KEY idx_notifications_user_read (user_id, read_at, id),
    KEY idx_notifications_dedupe (type_id, tenant_id, dedupe_key, created_at),
    KEY idx_notifications_created (created_at),
    CONSTRAINT fk_notifications_tenant FOREIGN KEY (tenant_id) REFERENCES tenants (id),
    CONSTRAINT fk_notifications_user FOREIGN KEY (user_id) REFERENCES users (id),
    CONSTRAINT fk_notifications_type FOREIGN KEY (type_id) REFERENCES ref_notification_types (id),
    CONSTRAINT fk_notifications_severity FOREIGN KEY (severity_id) REFERENCES ref_notification_severity (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='Notifikasi per penerima. Pengecualian audit 7-kolom (log volume tinggi, hanya created_at): lihat header migrations/0030';

CREATE TABLE notification_preferences (
    id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    user_id     BIGINT UNSIGNED NOT NULL,
    type_id     BIGINT UNSIGNED NOT NULL,
    in_app      TINYINT(1)      NOT NULL DEFAULT 1,
    email       TINYINT(1)      NOT NULL DEFAULT 0,
    created_at  DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY uq_notification_preferences (user_id, type_id),
    KEY idx_notification_preferences_type (type_id),
    CONSTRAINT fk_notification_preferences_user FOREIGN KEY (user_id) REFERENCES users (id),
    CONSTRAINT fk_notification_preferences_type FOREIGN KEY (type_id) REFERENCES ref_notification_types (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='Pilihan kanal notifikasi per pengguna. Pengecualian audit 7-kolom (data preferensi milik pengguna): lihat header migrations/0030';
