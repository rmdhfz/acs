-- Reversal 0009_ztp_rule_matching_actions.
--
-- PERHATIAN: kalau ada baris zero_touch_rules yang sudah dibuat dengan
-- provisioning_profile_id NULL (rule reboot/firmware-only, fitur baru
-- migrasi ini) sebelum di-downgrade, MODIFY COLUMN ... NOT NULL di bawah
-- akan GAGAL. Isi/soft-delete baris tsb dulu sebelum menjalankan down
-- migration ini pada data yang sudah memakai fitur baru.
ALTER TABLE zero_touch_rules
    DROP CONSTRAINT chk_ztr_match_parameter_pair,
    DROP FOREIGN KEY fk_ztr_trigger_event,
    DROP FOREIGN KEY fk_ztr_firmware,
    DROP KEY idx_ztr_trigger_event,
    DROP KEY idx_ztr_firmware,
    DROP COLUMN trigger_event_id,
    DROP COLUMN firmware_file_id,
    DROP COLUMN post_apply_reboot,
    DROP COLUMN match_parameter_value_pattern,
    DROP COLUMN match_parameter_name,
    DROP COLUMN software_version_pattern,
    MODIFY COLUMN provisioning_profile_id BIGINT UNSIGNED NOT NULL;

DROP TABLE ref_ztp_trigger_event;
