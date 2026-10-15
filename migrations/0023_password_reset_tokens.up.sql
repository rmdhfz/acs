-- 0023_password_reset_tokens
--
-- Reset password mandiri lewat email (POST /auth/forgot-password lalu
-- POST /auth/reset-password).
--
-- Keamanan token:
--   * Token asli (32 byte acak, base64url) HANYA ada di email pengguna. Yang
--     disimpan di sini cuma SHA-256-nya (token_hash) — kebocoran DB tidak
--     memberi penyerang tautan reset yang bisa dipakai.
--   * Sekali pakai: `used_at` diisi atomik saat dikonsumsi.
--   * Berumur pendek: `expires_at` (default 30 menit, ACS_PASSWORD_RESET_TTL).
--   * Dibatasi per user lewat idx_prt_user_created (hitung permintaan per jam).
--
-- Pengecualian konvensi audit 7-kolom (CLAUDE.md, skema #2) — SENGAJA:
-- tabel ini token efemeral, bukan data master/transaksional. Audit minimal
-- (created_at + used_at); tidak ada soft delete — baris kedaluwarsa dihapus
-- FISIK oleh usecase/auth (DeleteExpired) sebagai kebijakan retensi eksplisit.
-- Siapa/kapan reset terjadi tetap tercatat di activity_logs.

CREATE TABLE password_reset_tokens (
    id           BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    user_id      BIGINT UNSIGNED NOT NULL,
    token_hash   CHAR(64)        NOT NULL COMMENT 'SHA-256 hex dari token; token asli tidak pernah disimpan',
    expires_at   DATETIME        NOT NULL,
    used_at      DATETIME        NULL     COMMENT 'NULL = belum dipakai',
    requested_ip VARCHAR(45)     NULL,
    created_at   DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE KEY uq_prt_token_hash (token_hash),
    KEY idx_prt_user_created (user_id, created_at),
    KEY idx_prt_expires (expires_at),
    CONSTRAINT fk_prt_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
  COMMENT='Token reset password sekali-pakai (hash saja); audit minimal & hapus fisik kedaluwarsa — pengecualian eksplisit (migrations/0023)';
