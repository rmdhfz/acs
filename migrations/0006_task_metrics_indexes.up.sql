-- Index pendukung query yang sekarang dipanggil BERULANG setiap scrape
-- Prometheus (default 15 detik, deploy/prometheus.yml), bukan lagi hanya
-- sesekali saat operator buka dashboard (acs-code-reviewer, review fitur
-- observability ROADMAP.md Fase 2):
--   - device_sessions.status: dipakai CountOpen (acs_cwmp_sessions_open).
--     Tabel ini tumbuh terus seiring Inform CWMP (satu baris per sesi),
--     mirip device_events -- sebelumnya tidak punya index status sama
--     sekali, full table scan tiap scrape akan makin lambat seiring volume.
--   - tasks.completed_at: dipakai AvgCompletionSeconds (filter
--     `completed_at >= ?`).
--   - tasks.task_status_id berdiri sendiri: index composite yang sudah ada
--     (idx_tasks_device_status_priority) diawali device_id, tidak terpakai
--     untuk query lintas-device seperti CountFailedByVendor/CountByStatus
--     platform-wide (tanpa filter device_id).
ALTER TABLE device_sessions
    ADD KEY idx_device_sessions_status (status);

ALTER TABLE tasks
    ADD KEY idx_tasks_completed_at (completed_at),
    ADD KEY idx_tasks_status (task_status_id);
