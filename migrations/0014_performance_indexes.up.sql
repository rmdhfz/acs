-- Indeks performa tambahan untuk query hot-path yang belum dicover oleh
-- migrasi sebelumnya:
--
-- 1. webhook_deliveries (PENDING + next_attempt_at <= now) — query
--    ClaimDue yang dipanggil worker setiap 15 detik (cmd/acsd runSweepers).
--    Tanpa compound index ini, query akan full-scan tabel yang tumbuh terus
--    seiring volume event CWMP.
--
-- 2. device_sessions (status + started_at) — query MarkStaleOffline
--    yang di-join ke device_sessions untuk mendeteksi device offline
--    (sweeper 1 menit). Index tunggal pada status sudah ada (migrations/0006),
--    ditambah started_at untuk covering scan pada kolom waktu.
--
-- 3. webhook_deliveries (subscription_id + status + created_at) — query
--    ListBySubscription (GET /webhooks/:id/deliveries). Tanpa index ini,
--    setiap permintaan halaman delivery per subscription full-scan.
--
-- CATATAN: ADD KEY (bukan ADD UNIQUE KEY) — ini index performa non-unique,
-- bukan constraint. Tidak ada duplikat-check, aman di-apply ke data existing.

ALTER TABLE webhook_deliveries
    ADD KEY idx_webhook_deliveries_claim (status, next_attempt_at),
    ADD KEY idx_webhook_deliveries_sub_status (subscription_id, status, created_at);

-- Compound index untuk MarkStaleOffline (status + started_at),
-- komplementer dengan idx_device_sessions_status (migrations/0006).
-- Hanya ditambahkan bila belum ada — menggunakan nama unik.
ALTER TABLE device_sessions
    ADD KEY idx_device_sessions_status_started (status, started_at);
