-- Rollback migrations/0014_performance_indexes.up.sql
-- DROP KEY saja (bukan constraint) — aman karena bukan FK/unique index.

ALTER TABLE webhook_deliveries
    DROP KEY idx_webhook_deliveries_claim,
    DROP KEY idx_webhook_deliveries_sub_status;

ALTER TABLE device_sessions
    DROP KEY idx_device_sessions_status_started;
