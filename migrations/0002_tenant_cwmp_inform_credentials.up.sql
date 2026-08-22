-- Shared secret Inform CWMP per tenant — dibutuhkan agar endpoint CWMP bisa
-- memvalidasi kredensial device yang BENAR-BENAR baru (belum pernah tercatat
-- di tabel devices), sebelum device tsb punya kredensial per-device sendiri.
-- CPE diprovisioning (di luar ACS, oleh teknisi/vendor) untuk mengirim Inform
-- dengan username/password ini; ACS mencocokkannya ke tenant yang bersangkutan
-- lalu meng-assign tenant_id ke device baru saat pertama kali dibuat.
-- Kredensial per-device (devices.inform_username/inform_password_enc) tetap
-- berfungsi sebagai override opsional setelah device dikenal.
ALTER TABLE tenants
    ADD COLUMN cwmp_inform_username     VARCHAR(128)    NULL AFTER is_active,
    ADD COLUMN cwmp_inform_password_enc VARBINARY(255)  NULL COMMENT 'Terenkripsi (AES-GCM) di level aplikasi, bukan plaintext' AFTER cwmp_inform_username,
    ADD UNIQUE KEY uq_tenants_cwmp_inform_username (cwmp_inform_username);
