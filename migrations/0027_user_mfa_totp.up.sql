-- 0027_user_mfa_totp
--
-- MFA (autentikasi dua faktor) TOTP RFC 6238 untuk login email+password.
--  - users.totp_secret_enc : rahasia TOTP terenkripsi AES-GCM (pkg/cryptoutil); NULL = belum enrol.
--  - users.totp_enabled    : 1 setelah pengguna membuktikan kode pertama; login lalu butuh langkah kedua.
--  - users.totp_last_step  : langkah waktu (30 dtk) terakhir yang dipakai -> kode yang sama tak bisa
--                            dipakai ulang (anti-replay).
--  - user_recovery_codes   : kode pemulihan sekali pakai (hanya hash SHA-256 yang disimpan).
-- Pengecualian audit 7-kolom (CLAUDE.md §2): user_recovery_codes adalah tabel kredensial sekali
-- pakai volume rendah yang dihapus/diganti utuh saat MFA dinonaktifkan atau kode dibuat ulang;
-- jejak audit-nya ada di activity_logs (MFA_ENABLED/MFA_DISABLED/MFA_RESET), bukan di kolom tabel ini.

ALTER TABLE users
    ADD COLUMN totp_secret_enc VARBINARY(512) NULL COMMENT 'Rahasia TOTP terenkripsi AES-GCM; NULL = belum enrol' AFTER password_changed_at,
    ADD COLUMN totp_enabled TINYINT(1) NOT NULL DEFAULT 0 COMMENT '1 = login butuh kode TOTP' AFTER totp_secret_enc,
    ADD COLUMN totp_last_step BIGINT UNSIGNED NULL COMMENT 'Langkah TOTP terakhir yang diterima (anti-replay)' AFTER totp_enabled;

CREATE TABLE user_recovery_codes (
    id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    user_id     BIGINT UNSIGNED NOT NULL,
    code_hash   CHAR(64)        NOT NULL COMMENT 'SHA-256 heksadesimal dari kode pemulihan',
    used_at     DATETIME        NULL,
    created_at  DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    KEY idx_urc_user (user_id),
    UNIQUE KEY uq_urc_hash (code_hash),
    CONSTRAINT fk_urc_user FOREIGN KEY (user_id) REFERENCES users (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='Kode pemulihan MFA sekali pakai (hash saja). Pengecualian audit 7-kolom: lihat header migrasi 0027';
