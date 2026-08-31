-- Rollback 0006_task_metrics_indexes.
--
-- idx_tasks_status (task_status_id) yang dibuat 0006_up.sql menjadi SATU-SATUNYA
-- index yang melayani fk_tasks_status setelah MariaDB membuang index implisit FK
-- yang dianggap redundan. DROP KEY langsung ditolak (errno 1553). Jadi: lepas FK
-- dulu, drop index, pasang ulang FK (yang otomatis membuat kembali index-nya).
ALTER TABLE tasks DROP FOREIGN KEY fk_tasks_status;

ALTER TABLE tasks
    DROP KEY idx_tasks_status,
    DROP KEY idx_tasks_completed_at;

ALTER TABLE tasks
    ADD CONSTRAINT fk_tasks_status FOREIGN KEY (task_status_id) REFERENCES ref_task_status (id);

ALTER TABLE device_sessions
    DROP KEY idx_device_sessions_status;
