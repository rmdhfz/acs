-- 0031_user_oidc_subject
--
-- Pengikatan akun ke identitas IdP (Google/OIDC) agar login SSO tidak bergantung semata pada EMAIL:
--  - users.oidc_subject : "<issuer>|<sub>" milik IdP, diisi pada login SSO pertama yang berhasil (compare-and-set
--                         hanya bila masih NULL). Login SSO berikutnya wajib membawa nilai yang sama; bila berbeda
--                         (mis. alamat email berpindah tangan di IdP) login ditolak dan dicatat di audit.
--                         Mengubah email pengguna (PATCH /users/:id) mengosongkan ikatan ini.
--  - NULL = belum pernah login lewat SSO (atau ikatan direset).
-- Kolom milik tabel master yang sudah ber-audit 7-kolom; tidak ada tabel baru.

ALTER TABLE users
    ADD COLUMN oidc_subject VARCHAR(320) NULL COMMENT 'Ikatan identitas IdP "<issuer>|<sub>"; NULL = belum terikat (migrations/0031)' AFTER totp_last_step,
    ADD UNIQUE KEY uq_users_oidc_subject (oidc_subject);
