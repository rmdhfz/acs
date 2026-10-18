-- 0034_bugreport_expected_screenshot
--
-- Laporan bug (migrations/0033) diperkaya:
--  - bug_reports.expected_behavior : "apa yang diharapkan" sebagai kolom TERPISAH dari description ("apa yang terjadi").
--    Baris lama (sebelum migrasi) mendapat '' (tidak ada data harapan).
--  - bug_report_screenshots        : satu tangkapan layar (PNG/JPEG, maks 2 MiB, divalidasi usecase: tanda tangan berkas +
--                                    decode header + dimensi) per laporan, disimpan di database agar ikut cadangan DB.
--                                    Hanya pelapor & SUPERADMIN yang dapat membacanya (lewat endpoint terautentikasi).
--  - jenis notifikasi BUG_REPORT_UPDATE : pelapor diberi tahu saat status laporannya berubah (mis. sudah ditangani).
--
-- Audit 7-kolom penuh (CLAUDE.md §2); tidak ada pengecualian.

ALTER TABLE bug_reports
    ADD COLUMN expected_behavior TEXT NOT NULL COMMENT 'Apa yang diharapkan pelapor (maks 2000 karakter, divalidasi usecase)' AFTER description;

CREATE TABLE bug_report_screenshots (
    id             BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    bug_report_id  BIGINT UNSIGNED NOT NULL,
    content_type   VARCHAR(16)     NOT NULL COMMENT 'image/png atau image/jpeg (ditentukan server dari isi berkas, bukan dari klien)',
    width          SMALLINT UNSIGNED NOT NULL,
    height         SMALLINT UNSIGNED NOT NULL,
    size_bytes     INT UNSIGNED    NOT NULL,
    sha256         CHAR(64)        NOT NULL,
    data           MEDIUMBLOB      NOT NULL,
    created_at     DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_by     BIGINT UNSIGNED NULL,
    updated_at     DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    updated_by     BIGINT UNSIGNED NULL,
    deleted_at     DATETIME        NULL,
    deleted_by     BIGINT UNSIGNED NULL,
    is_deleted     TINYINT(1)      NOT NULL DEFAULT 0,
    UNIQUE KEY uq_bug_report_screenshots_report (bug_report_id),
    CONSTRAINT fk_bug_report_screenshots_report FOREIGN KEY (bug_report_id) REFERENCES bug_reports (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='Tangkapan layar laporan bug, satu per laporan (migrations/0034)';

INSERT INTO ref_notification_types (code, name, description, severity_id, default_in_app, default_email, is_mandatory) VALUES
    ('BUG_REPORT_UPDATE', 'Status laporan bug berubah', 'Laporan bug yang Anda kirim sedang ditangani atau sudah selesai.',
        (SELECT id FROM ref_notification_severity WHERE code = 'INFO'), 1, 0, 0);
