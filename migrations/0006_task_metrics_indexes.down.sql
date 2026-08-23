ALTER TABLE tasks
    DROP KEY idx_tasks_status,
    DROP KEY idx_tasks_completed_at;

ALTER TABLE device_sessions
    DROP KEY idx_device_sessions_status;
