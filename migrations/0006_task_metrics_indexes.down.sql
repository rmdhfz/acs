-- Catatan: idx_tasks_status adalah index tambahan yang dibuat di 0006_up.sql.
-- Karena fk_tasks_status (index implisit dari foreign key) sudah ada sejak 0001,
-- kita bisa langsung drop idx_tasks_status tanpa perlu drop foreign key.
ALTER TABLE tasks
    DROP KEY idx_tasks_status,
    DROP KEY idx_tasks_completed_at;

ALTER TABLE device_sessions
    DROP KEY idx_device_sessions_status;
