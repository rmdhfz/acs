-- Urutan terbalik dari up (FK): hapus pengiriman, langganan, preferensi, dan notifikasi jenis ini dulu, baru baris ref-nya.
DELETE FROM webhook_deliveries WHERE event_type_id IN (SELECT id FROM ref_webhook_event_types WHERE code IN ('DEVICE_OFFLINE', 'ONU_STATE_CHANGED'));
DELETE FROM webhook_subscriptions WHERE event_type_id IN (SELECT id FROM ref_webhook_event_types WHERE code IN ('DEVICE_OFFLINE', 'ONU_STATE_CHANGED'));
DELETE FROM ref_webhook_event_types WHERE code IN ('DEVICE_OFFLINE', 'ONU_STATE_CHANGED');
DELETE FROM notification_preferences WHERE type_id IN (SELECT id FROM ref_notification_types WHERE code IN ('DEVICE_OFFLINE', 'ONU_STATE_ALERT'));
DELETE FROM notifications WHERE type_id IN (SELECT id FROM ref_notification_types WHERE code IN ('DEVICE_OFFLINE', 'ONU_STATE_ALERT'));
DELETE FROM ref_notification_types WHERE code IN ('DEVICE_OFFLINE', 'ONU_STATE_ALERT');
