-- Urutan terbalik dari up.
DELETE FROM notification_preferences WHERE type_id IN (SELECT id FROM ref_notification_types WHERE code = 'BUG_REPORT_UPDATE');
DELETE FROM notifications WHERE type_id IN (SELECT id FROM ref_notification_types WHERE code = 'BUG_REPORT_UPDATE');
DELETE FROM ref_notification_types WHERE code = 'BUG_REPORT_UPDATE';
DROP TABLE IF EXISTS bug_report_screenshots;
ALTER TABLE bug_reports DROP COLUMN expected_behavior;
