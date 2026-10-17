-- Rollback 0029: lihat catatan di .up.sql (hapus seed fisik; vendor soft-delete).
DELETE FROM vendor_ouis WHERE notes LIKE 'seed-0029 IEEE MA-L:%';
DELETE m FROM vendor_parameter_mappings m JOIN ref_vendors g ON g.id = m.vendor_id AND g.code = 'GENERIC';
UPDATE ref_vendors SET is_deleted = 1, deleted_at = NOW() WHERE code IN ('GENERIC', 'TPLINK', 'MERCUSYS', 'TENDA', 'RUIJIE', 'VSOL', 'SYROTECH', 'SKYWORTH', 'SAGEMCOM', 'ZYXEL', 'DASAN', 'RAISECOM', 'KAON', 'NETIS', 'TOTOLINK', 'DLINK', 'ASKEY', 'SERCOMM', 'ARCADYAN', 'MITRASTAR', 'UTSTARCOM', 'GENEXIS', 'OPTILINK', 'SUMAVISION', 'UBIQUOSS', 'CALIX', 'ADTRAN', 'GEMTEK');
