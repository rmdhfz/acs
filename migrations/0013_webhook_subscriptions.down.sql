-- Rollback 0013_webhook_subscriptions. Urutan drop: child dulu (FK), lalu
-- parent, lalu ref_.
DROP TABLE IF EXISTS webhook_deliveries;
DROP TABLE IF EXISTS webhook_subscriptions;
DROP TABLE IF EXISTS ref_webhook_event_types;
