-- 0017_performance_indexes
--
-- Indeks performa tambahan untuk query hot-path yang belum tercover migrasi
-- sebelumnya:
--
-- 1. webhook_deliveries (subscription_id + status + created_at) — query
--    ListBySubscription (GET /webhooks/:id/deliveries). Tanpa index ini tiap
--    permintaan halaman delivery per subscription full-scan.
--
-- 2. device_sessions (status + started_at) — query MarkStaleOffline /
--    deteksi device offline (sweeper 1 menit). Melengkapi
--    idx_device_sessions_status (status saja) yang sudah ada.
--
-- CATATAN: query worker webhook "PENDING + next_attempt_at <= NOW()" sudah
-- ter-cover idx_webhook_deliveries_worker (status, next_attempt_at) yang
-- dibuat di 0013 — TIDAK ditambah index duplikat di sini.

ALTER TABLE webhook_deliveries
    ADD KEY idx_webhook_deliveries_sub_status (subscription_id, status, created_at);

ALTER TABLE device_sessions
    ADD KEY idx_device_sessions_status_started (status, started_at);
