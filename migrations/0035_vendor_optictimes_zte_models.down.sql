-- Rollback 0035: lepas referensi device lalu hapus seed fisik; vendor di-soft-delete (pola 0029).
UPDATE devices SET device_model_id = NULL
 WHERE device_model_id IN (SELECT id FROM device_models WHERE description LIKE 'seed-0035:%');
DELETE FROM device_models WHERE description LIKE 'seed-0035:%';
UPDATE devices SET vendor_id = NULL
 WHERE vendor_id = (SELECT id FROM ref_vendors WHERE code = 'OPTICTIMES' AND is_deleted = 0)
   AND oui = '9028F6';
DELETE FROM vendor_ouis WHERE notes LIKE 'seed-0035:%';
UPDATE ref_vendors SET is_deleted = 1, deleted_at = NOW() WHERE code = 'OPTICTIMES';
