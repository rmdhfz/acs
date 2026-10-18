-- Urutan terbalik dari up (FK): notifikasi billing dulu (hapus baris notifications yang merujuk jenisnya),
-- lalu kolom tenants + FK, lalu log webhook, invoice, ref status, paket.
DELETE FROM notification_preferences WHERE type_id IN (SELECT id FROM ref_notification_types WHERE code LIKE 'BILLING\_%');
DELETE FROM notifications WHERE type_id IN (SELECT id FROM ref_notification_types WHERE code LIKE 'BILLING\_%');
DELETE FROM ref_notification_types WHERE code LIKE 'BILLING\_%';
ALTER TABLE tenants
    DROP FOREIGN KEY fk_tenants_billing_plan,
    DROP COLUMN plan_expires_at,
    DROP COLUMN billing_plan_id;
DROP TABLE IF EXISTS billing_webhook_events;
DROP TABLE IF EXISTS billing_invoices;
DROP TABLE IF EXISTS ref_invoice_status;
DROP TABLE IF EXISTS billing_plans;
