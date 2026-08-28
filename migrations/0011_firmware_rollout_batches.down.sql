ALTER TABLE firmware_upgrade_jobs
    DROP FOREIGN KEY fk_fuj_rollout_batch,
    DROP KEY idx_fuj_rollout_batch_wave,
    DROP KEY idx_fuj_rollout_batch,
    DROP COLUMN wave_number,
    DROP COLUMN rollout_batch_id;

DROP TABLE firmware_rollout_batches;
DROP TABLE ref_firmware_rollout_status;
