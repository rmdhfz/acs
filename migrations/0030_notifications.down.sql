-- Urutan terbalik dari up (FK): preferensi & notifikasi dulu, lalu katalog jenis, lalu keparahan.
DROP TABLE IF EXISTS notification_preferences;
DROP TABLE IF EXISTS notifications;
DROP TABLE IF EXISTS ref_notification_types;
DROP TABLE IF EXISTS ref_notification_severity;
