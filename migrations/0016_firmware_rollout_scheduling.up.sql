-- migrations/0016_firmware_rollout_scheduling.up.sql
ALTER TABLE firmware_rollout_batches
ADD COLUMN scheduled_at DATETIME NULL COMMENT 'Diisi bila batch baru boleh dijalankan (wave 1) setelah waktu tertentu' AFTER notes;
