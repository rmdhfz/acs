-- =====================================================================
-- 0035_vendor_optictimes_zte_models
--
-- Temuan UAT 2026-10-21: ONT Optictimes (serial OPTIF600C9E0, ProductClass RXP4-AC1200, firmware
-- HGUV4.3.7B02) tampil "Belum dikenal" karena OUI 9028F6 belum ada di katalog. Migrasi ini murni
-- OPERASI DATA (CLAUDE.md: menambah dukungan vendor/model = data, bukan kode).
--
-- 1. Vendor OPTICTIMES + OUI 9028F6.
--    Catatan jujur tentang sumbernya: registri IEEE mendaftarkan 9028F6 (MA-L, 2024-05-15) atas
--    "Wuhan Yangtze Optical Technology Co.,Ltd." — BUKAN atas nama "Optictimes". Pemetaan ke vendor
--    Optictimes berdasarkan ONT fisik milik pengguna (serial berawalan OPTI, ProductClass RXP4-AC1200)
--    yang mengirim Inform dengan OUI ini. Perlu dikonfirmasi bila kelak ada OUI Optictimes lain
--    (tambahkan lewat Catalog Vendor atau fitur "Tetapkan vendor" di Console, SUPERADMIN).
--    Vendor ini TIDAK membawa mapping sendiri: parameter standar TR-098 tetap di-resolve lewat
--    fallback vendor GENERIC (usecase/task ResolveMapping), jadi tidak ada regresi pada device ini.
--
-- 2. Model dari ONT nyata yang sudah mengirim Inform pada UAT (semuanya root InternetGatewayDevice /
--    TR-098, teramati dari parameter yang dilaporkan): ZTE F609, ZTE F660, Optictimes RXP4-AC1200.
--    Dengan model terdaftar, data model device diketahui dari model (bukan ditebak dari parameter) dan
--    override mapping per model dimungkinkan. Model ZTE lain (mis. F670L) sengaja TIDAK ditambah sebelum
--    teramati dari perangkat nyata.
--
-- Idempoten: vendor yang sudah ada diaktifkan kembali, OUI/model yang sudah ada tidak ditimpa.
-- Device yang sudah terlanjur tanpa vendor/model sembuh sendiri pada Inform berikutnya (hanya mengisi
-- yang masih kosong; pilihan manual operator tidak pernah ditimpa).
-- =====================================================================

INSERT INTO ref_vendors (code, name, description) VALUES
    ('OPTICTIMES', 'Optictimes', 'seed-0035: ONT/ONU GPON (seri RXP, mis. RXP4-AC1200). OUI 9028F6 terdaftar IEEE atas Wuhan Yangtze Optical Technology Co.,Ltd.')
  ON DUPLICATE KEY UPDATE is_deleted = 0, deleted_at = NULL, deleted_by = NULL;

SET @optictimes := (SELECT id FROM ref_vendors WHERE code = 'OPTICTIMES' AND is_deleted = 0);

INSERT INTO vendor_ouis (vendor_id, oui, notes) VALUES
    (@optictimes, '9028F6', 'seed-0035: IEEE MA-L Wuhan Yangtze Optical Technology Co.,Ltd. (2024-05-15); dipetakan ke Optictimes dari ONT RXP4-AC1200 nyata (UAT 2026-10-21)')
  ON DUPLICATE KEY UPDATE id = id;

INSERT INTO device_models (vendor_id, device_type_id, data_model_version_id, product_class, model_name, description)
SELECT v.id, t.id, d.id, m.product_class, m.model_name, m.descr
FROM (
    SELECT 'OPTICTIMES' AS vendor_code, 'RXP4-AC1200' AS product_class, 'RXP4-AC1200' AS model_name,
           'seed-0035: ONT GPON WiFi AC1200 (Optictimes); TR-098, ekstensi X_CT-COM; teramati dari Inform nyata' AS descr
    UNION ALL
    SELECT 'ZTE', 'F609', 'F609', 'seed-0035: ONT GPON ZTE ZXHN F609 (varian Telkom); TR-098; teramati dari Inform nyata'
    UNION ALL
    SELECT 'ZTE', 'F660', 'F660', 'seed-0035: ONT GPON ZTE ZXHN F660 (varian Telkom); TR-098, 4 SSID; teramati dari Inform nyata'
) m
JOIN ref_vendors v ON v.code = m.vendor_code AND v.is_deleted = 0
JOIN ref_device_types t ON t.code = 'ONT'
JOIN ref_data_model_versions d ON d.code = 'TR098'
ON DUPLICATE KEY UPDATE device_models.id = device_models.id;
