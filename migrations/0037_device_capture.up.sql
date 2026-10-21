-- 0037_device_capture
--
-- Capture sesi CWMP per perangkat untuk diagnosis kuirk firmware (mis. Fault 9002 akibat atribut xsi:type): operator
-- menyalakan capture untuk SATU perangkat selama maksimal 60 menit; selama itu setiap pesan SOAP (CPE -> ACS dan ACS -> CPE)
-- disimpan dengan nilai rahasia (password/passphrase/PSK/dst.) DISAMARKAN sebelum menyentuh database. Header HTTP
-- (Authorization, Cookie) tidak pernah disimpan.
--
--  - device_capture_switches  : saklar capture per perangkat (kedaluwarsa otomatis; satu baris per perangkat).
--  - device_session_captures  : pesan SOAP hasil capture (diekspos ke API lewat capture_uuid; PK tidak pernah keluar).
--
-- PENGECUALIAN audit 7-kolom (CLAUDE.md §2) -- disengaja, bukan default diam-diam:
--   * device_session_captures : tabel LOG volume tinggi -> audit MINIMAL, hanya created_at. Tanpa soft delete; dipangkas FISIK
--                               (dibatasi 200 pesan terbaru per perangkat + dibuang setelah 7 hari + ikut job retensi) --
--                               cleanup log sesuai kebijakan retensi eksplisit (CLAUDE.md §3).
--   * device_capture_switches : saklar sementara (bukan data master/transaksional); siapa yang menyalakan ada di
--                               enabled_by + activity_logs (CAPTURE_ENABLED/DISABLED) -> audit minimal created_at/updated_at.

CREATE TABLE device_capture_switches (
    id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    device_id   BIGINT UNSIGNED NOT NULL,
    expires_at  DATETIME        NOT NULL COMMENT 'Capture aktif selama expires_at > sekarang; menonaktifkan = expires_at diset ke sekarang',
    enabled_by  BIGINT UNSIGNED NULL COMMENT 'Pengguna yang menyalakan (jejak lengkap di activity_logs)',
    created_at  DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY uq_device_capture_switches_device (device_id),
    KEY idx_device_capture_switches_expires (expires_at),
    CONSTRAINT fk_device_capture_switches_device FOREIGN KEY (device_id) REFERENCES devices (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='Saklar capture sesi CWMP per perangkat. Pengecualian audit 7-kolom (saklar sementara): lihat header migrations/0037';

CREATE TABLE device_session_captures (
    id             BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    capture_uuid   CHAR(36)        NOT NULL COMMENT 'Identifier publik di API; PK tidak pernah diekspos',
    device_id      BIGINT UNSIGNED NOT NULL,
    session_ref    CHAR(8)         NULL COMMENT '8 hex pertama SHA-256 token sesi: mengelompokkan pesan satu sesi tanpa menyimpan token',
    from_cpe       TINYINT(1)      NOT NULL COMMENT '1 = CPE -> ACS, 0 = ACS -> CPE',
    rpc_name       VARCHAR(64)     NOT NULL COMMENT 'Nama RPC/elemen SOAP (Inform, GetParameterValuesResponse, Fault, ...) atau (kosong)',
    http_status    SMALLINT UNSIGNED NULL COMMENT 'Status HTTP balasan ACS (hanya baris ACS -> CPE)',
    body           MEDIUMTEXT      NOT NULL COMMENT 'XML SOAP dengan nilai rahasia disamarkan; dipotong bila melebihi batas',
    body_bytes     INT UNSIGNED    NOT NULL COMMENT 'Ukuran asli sebelum dipotong',
    body_truncated TINYINT(1)      NOT NULL DEFAULT 0,
    created_at     DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    UNIQUE KEY uq_device_session_captures_uuid (capture_uuid),
    KEY idx_device_session_captures_device (device_id, id),
    KEY idx_device_session_captures_created (created_at),
    CONSTRAINT fk_device_session_captures_device FOREIGN KEY (device_id) REFERENCES devices (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='Pesan SOAP hasil capture sesi CWMP (rahasia disamarkan). Pengecualian audit 7-kolom (log volume tinggi): lihat header migrations/0037';
