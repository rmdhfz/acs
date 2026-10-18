-- Urutan terbalik dari up (FK): notifikasi BUG_REPORT dulu, lalu log transfer + ref, laporan bug + ref, layout menu.
DELETE FROM notification_preferences WHERE type_id IN (SELECT id FROM ref_notification_types WHERE code = 'BUG_REPORT');
DELETE FROM notifications WHERE type_id IN (SELECT id FROM ref_notification_types WHERE code = 'BUG_REPORT');
DELETE FROM ref_notification_types WHERE code = 'BUG_REPORT';
DROP TABLE IF EXISTS data_transfer_logs;
DROP TABLE IF EXISTS ref_data_transfer_status;
DROP TABLE IF EXISTS ref_data_transfer_kind;
DROP TABLE IF EXISTS bug_reports;
DROP TABLE IF EXISTS ref_bug_report_severity;
DROP TABLE IF EXISTS ref_bug_report_status;
DROP TABLE IF EXISTS user_menu_layouts;
