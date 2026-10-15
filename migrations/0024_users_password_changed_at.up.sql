-- 0024_users_password_changed_at
--
-- Mencabut sesi lama setelah password berubah (reset lewat email, ganti
-- sendiri, atau reset oleh admin). JWT sesi bersifat stateless; tanpa penanda
-- ini, token yang sudah dicuri tetap sah sampai kedaluwarsa walau pemilik akun
-- sudah mengganti password. AuthMiddleware (ResolveActor) menolak JWT yang
-- `iat`-nya lebih awal dari kolom ini.
--
-- NULL = password belum pernah diganti sejak kolom ada (semua JWT lama tetap
-- berlaku sampai password berikutnya diganti). Bukan perubahan audit 7-kolom:
-- ini atribut keamanan akun, bukan jejak audit.

ALTER TABLE users
    ADD COLUMN password_changed_at DATETIME NULL
        COMMENT 'Waktu terakhir password diganti; JWT dengan iat lebih awal ditolak'
        AFTER password_hash;
