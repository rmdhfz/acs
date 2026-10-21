-- Urutan terbalik dari up (FK): hapus preferensi & notifikasi jenis ini dulu, baru jenisnya.
DELETE FROM notification_preferences WHERE type_id IN (SELECT id FROM ref_notification_types WHERE code = 'OPTICAL_ALERT');
DELETE FROM notifications WHERE type_id IN (SELECT id FROM ref_notification_types WHERE code = 'OPTICAL_ALERT');
DELETE FROM ref_notification_types WHERE code = 'OPTICAL_ALERT';
