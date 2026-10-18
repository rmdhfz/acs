-- 0033_menu_bugreport_transfers
--
-- Tiga fitur Console yang butuh penyimpanan:
--  1. user_menu_layouts   : susunan menu sidebar per pengguna (urutan, sub menu/grup, item disembunyikan) hasil
--                           "Atur menu" (drag & drop). Satu baris per pengguna; pemilik = user_id (tidak ada akses
--                           lintas pengguna, bahkan oleh SUPERADMIN).
--  2. bug_reports         : laporan bug dari tombol di navbar. Diekspos lewat report_uuid. Pelapor melihat laporannya
--                           sendiri; SUPERADMIN melihat/menangani semua. Email ke alamat operator dikirim oleh usecase
--                           (ACS_BUG_REPORT_EMAIL).
--  3. data_transfer_logs  : riwayat setiap proses IMPOR/EKSPOR (modul, format, jumlah baris, status). Dilihat per tenant;
--                           SUPERADMIN melihat semua tenant. Diekspos lewat transfer_uuid.
--
-- Referensi/enum memakai tabel ref_* (CLAUDE.md §1). Semua tabel baru memakai audit 7-kolom penuh; tidak ada
-- pengecualian audit minimal.
--
-- Isi laporan bug & log transfer TIDAK PERNAH memuat password/token/rahasia (ditegakkan di usecase); data baris impor
-- tidak disimpan -- hanya hitungan dan ringkasan galat.

CREATE TABLE user_menu_layouts (
    id           BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    user_id      BIGINT UNSIGNED NOT NULL,
    layout_json  TEXT            NOT NULL COMMENT 'JSON susunan menu (divalidasi usecase: maks 8 KB, <=80 simpul, kedalaman 2)',
    created_at   DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_by   BIGINT UNSIGNED NULL,
    updated_at   DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    updated_by   BIGINT UNSIGNED NULL,
    deleted_at   DATETIME        NULL,
    deleted_by   BIGINT UNSIGNED NULL,
    is_deleted   TINYINT(1)      NOT NULL DEFAULT 0,
    UNIQUE KEY uq_user_menu_layouts_user (user_id),
    CONSTRAINT fk_user_menu_layouts_user FOREIGN KEY (user_id) REFERENCES users (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='Susunan menu sidebar per pengguna (migrations/0033). Reset = soft delete; PUT berikutnya menghidupkan baris yang sama.';

CREATE TABLE ref_bug_report_status (
    id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    code        VARCHAR(32)     NOT NULL COMMENT 'OPEN, IN_PROGRESS, RESOLVED, WONTFIX',
    name        VARCHAR(128)    NOT NULL,
    created_at  DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_by  BIGINT UNSIGNED NULL,
    updated_at  DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    updated_by  BIGINT UNSIGNED NULL,
    deleted_at  DATETIME        NULL,
    deleted_by  BIGINT UNSIGNED NULL,
    is_deleted  TINYINT(1)      NOT NULL DEFAULT 0,
    UNIQUE KEY uq_ref_bug_report_status_code (code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='Status penanganan laporan bug (migrations/0033)';

INSERT INTO ref_bug_report_status (code, name) VALUES
    ('OPEN', 'Baru'),
    ('IN_PROGRESS', 'Ditangani'),
    ('RESOLVED', 'Selesai'),
    ('WONTFIX', 'Tidak diperbaiki');

CREATE TABLE ref_bug_report_severity (
    id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    code        VARCHAR(32)     NOT NULL COMMENT 'MINOR, MAJOR, BLOCKER',
    name        VARCHAR(128)    NOT NULL,
    created_at  DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_by  BIGINT UNSIGNED NULL,
    updated_at  DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    updated_by  BIGINT UNSIGNED NULL,
    deleted_at  DATETIME        NULL,
    deleted_by  BIGINT UNSIGNED NULL,
    is_deleted  TINYINT(1)      NOT NULL DEFAULT 0,
    UNIQUE KEY uq_ref_bug_report_severity_code (code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='Tingkat dampak laporan bug (migrations/0033)';

INSERT INTO ref_bug_report_severity (code, name) VALUES
    ('MINOR', 'Ringan (tampilan/kenyamanan)'),
    ('MAJOR', 'Sedang (fitur tidak berjalan benar)'),
    ('BLOCKER', 'Berat (tidak bisa bekerja)');

CREATE TABLE bug_reports (
    id               BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    report_uuid      CHAR(36)        NOT NULL,
    tenant_id        BIGINT UNSIGNED NULL COMMENT 'Tenant pelapor; NULL = pelapor SUPERADMIN (platform)',
    reporter_id      BIGINT UNSIGNED NOT NULL,
    impersonator_id  BIGINT UNSIGNED NULL COMMENT 'Terisi bila dikirim dalam sesi impersonate (SUPERADMIN yang bertindak)',
    title            VARCHAR(120)    NOT NULL,
    description      TEXT            NOT NULL COMMENT 'Maks 4000 karakter (usecase)',
    page_path        VARCHAR(255)    NOT NULL DEFAULT '' COMMENT 'Path Console saat melapor (tanpa query/fragmen)',
    user_agent       VARCHAR(255)    NOT NULL DEFAULT '',
    app_version      VARCHAR(32)     NOT NULL DEFAULT '',
    severity_id      BIGINT UNSIGNED NOT NULL,
    status_id        BIGINT UNSIGNED NOT NULL,
    admin_note       VARCHAR(500)    NULL COMMENT 'Catatan penanganan oleh SUPERADMIN',
    emailed_at       DATETIME        NULL COMMENT 'Waktu email ke operator diserahkan ke pengirim email; NULL = belum/SMTP mati',
    created_at       DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_by       BIGINT UNSIGNED NULL,
    updated_at       DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    updated_by       BIGINT UNSIGNED NULL,
    deleted_at       DATETIME        NULL,
    deleted_by       BIGINT UNSIGNED NULL,
    is_deleted       TINYINT(1)      NOT NULL DEFAULT 0,
    UNIQUE KEY uq_bug_reports_uuid (report_uuid),
    KEY idx_bug_reports_tenant (tenant_id, created_at),
    KEY idx_bug_reports_reporter (reporter_id, created_at),
    KEY idx_bug_reports_status (status_id, created_at),
    CONSTRAINT fk_bug_reports_tenant   FOREIGN KEY (tenant_id)   REFERENCES tenants (id),
    CONSTRAINT fk_bug_reports_reporter FOREIGN KEY (reporter_id) REFERENCES users (id),
    CONSTRAINT fk_bug_reports_severity FOREIGN KEY (severity_id) REFERENCES ref_bug_report_severity (id),
    CONSTRAINT fk_bug_reports_status   FOREIGN KEY (status_id)   REFERENCES ref_bug_report_status (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='Laporan bug dari Console (migrations/0033)';

CREATE TABLE ref_data_transfer_kind (
    id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    code        VARCHAR(32)     NOT NULL COMMENT 'IMPORT, EXPORT',
    name        VARCHAR(128)    NOT NULL,
    created_at  DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_by  BIGINT UNSIGNED NULL,
    updated_at  DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    updated_by  BIGINT UNSIGNED NULL,
    deleted_at  DATETIME        NULL,
    deleted_by  BIGINT UNSIGNED NULL,
    is_deleted  TINYINT(1)      NOT NULL DEFAULT 0,
    UNIQUE KEY uq_ref_data_transfer_kind_code (code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='Jenis proses transfer data (migrations/0033)';

INSERT INTO ref_data_transfer_kind (code, name) VALUES ('IMPORT', 'Impor'), ('EXPORT', 'Ekspor');

CREATE TABLE ref_data_transfer_status (
    id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    code        VARCHAR(32)     NOT NULL COMMENT 'COMPLETED, PARTIAL, FAILED',
    name        VARCHAR(128)    NOT NULL,
    created_at  DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_by  BIGINT UNSIGNED NULL,
    updated_at  DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    updated_by  BIGINT UNSIGNED NULL,
    deleted_at  DATETIME        NULL,
    deleted_by  BIGINT UNSIGNED NULL,
    is_deleted  TINYINT(1)      NOT NULL DEFAULT 0,
    UNIQUE KEY uq_ref_data_transfer_status_code (code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='Hasil proses transfer data (migrations/0033)';

INSERT INTO ref_data_transfer_status (code, name) VALUES
    ('COMPLETED', 'Selesai'),
    ('PARTIAL', 'Sebagian berhasil'),
    ('FAILED', 'Gagal');

CREATE TABLE data_transfer_logs (
    id               BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    transfer_uuid    CHAR(36)        NOT NULL,
    tenant_id        BIGINT UNSIGNED NULL COMMENT 'Tenant pelaku; NULL = SUPERADMIN tanpa tenant (platform)',
    user_id          BIGINT UNSIGNED NOT NULL COMMENT 'Pelaku proses',
    impersonator_id  BIGINT UNSIGNED NULL COMMENT 'Terisi bila dijalankan dalam sesi impersonate',
    kind_id          BIGINT UNSIGNED NOT NULL,
    status_id        BIGINT UNSIGNED NOT NULL,
    module           VARCHAR(48)     NOT NULL COMMENT 'Modul data: devices, tenants, users, tags, ... (kata kunci tetap, divalidasi usecase)',
    format           VARCHAR(16)     NOT NULL DEFAULT 'csv',
    file_name        VARCHAR(255)    NULL COMMENT 'Nama berkas (basename tersanitasi); isi berkas TIDAK disimpan',
    total_rows       INT UNSIGNED    NOT NULL DEFAULT 0,
    success_rows     INT UNSIGNED    NOT NULL DEFAULT 0,
    failed_rows      INT UNSIGNED    NOT NULL DEFAULT 0,
    detail           VARCHAR(500)    NULL COMMENT 'Ringkasan galat/ catatan (tanpa data pribadi baris)',
    source           VARCHAR(16)     NOT NULL DEFAULT 'CLIENT' COMMENT 'SERVER = dicatat server saat memproses; CLIENT = dilaporkan Console setelah selesai',
    emailed_at       DATETIME        NULL COMMENT 'Waktu email notifikasi ke pelaku diserahkan ke pengirim email',
    created_at       DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_by       BIGINT UNSIGNED NULL,
    updated_at       DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    updated_by       BIGINT UNSIGNED NULL,
    deleted_at       DATETIME        NULL,
    deleted_by       BIGINT UNSIGNED NULL,
    is_deleted       TINYINT(1)      NOT NULL DEFAULT 0,
    UNIQUE KEY uq_data_transfer_logs_uuid (transfer_uuid),
    KEY idx_data_transfer_logs_tenant (tenant_id, created_at),
    KEY idx_data_transfer_logs_user (user_id, created_at),
    CONSTRAINT fk_dtl_tenant FOREIGN KEY (tenant_id) REFERENCES tenants (id),
    CONSTRAINT fk_dtl_user   FOREIGN KEY (user_id)   REFERENCES users (id),
    CONSTRAINT fk_dtl_kind   FOREIGN KEY (kind_id)   REFERENCES ref_data_transfer_kind (id),
    CONSTRAINT fk_dtl_status FOREIGN KEY (status_id) REFERENCES ref_data_transfer_status (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='Riwayat proses impor/ekspor data (migrations/0033)';

-- Jenis notifikasi in-app: laporan bug baru untuk operator platform (email ke operator dikirim langsung oleh usecase).
INSERT INTO ref_notification_types (code, name, description, severity_id, default_in_app, default_email, is_mandatory) VALUES
    ('BUG_REPORT', 'Laporan bug baru', 'Pengguna mengirim laporan bug dari Console.',
        (SELECT id FROM ref_notification_severity WHERE code = 'INFO'), 1, 0, 0);
