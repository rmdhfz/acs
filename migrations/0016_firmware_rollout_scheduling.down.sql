-- migrations/0016_firmware_rollout_scheduling.down.sql
ALTER TABLE firmware_rollout_batches
DROP COLUMN scheduled_at;
