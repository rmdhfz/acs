-- Rollback 0017_performance_indexes. DROP KEY saja (bukan constraint/FK).
ALTER TABLE webhook_deliveries
    DROP KEY idx_webhook_deliveries_sub_status;

ALTER TABLE device_sessions
    DROP KEY idx_device_sessions_status_started;
