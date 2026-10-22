-- Urutan terbalik dari up (FK). Langganan webhook yang memakai AUTOMATION_TRIGGERED harus dihapus lebih dulu agar baris ref bisa dibuang.
DELETE FROM webhook_deliveries WHERE event_type_id IN (SELECT id FROM ref_webhook_event_types WHERE code = 'AUTOMATION_TRIGGERED');
DELETE FROM webhook_subscriptions WHERE event_type_id IN (SELECT id FROM ref_webhook_event_types WHERE code = 'AUTOMATION_TRIGGERED');
DELETE FROM ref_webhook_event_types WHERE code = 'AUTOMATION_TRIGGERED';
DROP TABLE IF EXISTS automation_runs;
DROP TABLE IF EXISTS automation_states;
DROP TABLE IF EXISTS automation_rules;
