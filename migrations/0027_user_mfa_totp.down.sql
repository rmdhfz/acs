DROP TABLE IF EXISTS user_recovery_codes;
ALTER TABLE users DROP COLUMN totp_last_step, DROP COLUMN totp_enabled, DROP COLUMN totp_secret_enc;
