-- 0026_mapping_value_scale_optical
--
-- (1) vendor_parameter_mappings.value_scale: pengali nilai mentah CPE -> satuan kanonik ACS.
--     Banyak CPE melaporkan redaman optik dalam 0,001 dBm (mis. -2800 = -28,0 dBm) sedangkan
--     yang lain langsung dBm; skala per-mapping (data, bukan percabangan kode vendor) membuat
--     keduanya benar. Default 1 = tanpa konversi.
-- (2) Seed logical key optical.rx_power / optical.tx_power untuk data model TR-181 memakai
--     objek STANDAR Device.Optical.Interface.{i} (OpticalSignalLevel & TransmitOpticalLevel,
--     satuan 0,001 dBm). TR-098 tidak punya objek optik standar — path-nya ekstensi vendor
--     (X_<OUI>_...) dan HARUS ditambahkan per vendor/model lewat Katalog Vendor setelah
--     diverifikasi dari dokumentasi/CPE nyata (jangan ditebak). Hanya INSERT bila belum ada.

ALTER TABLE vendor_parameter_mappings
    ADD COLUMN value_scale DECIMAL(14,6) NOT NULL DEFAULT 1
        COMMENT 'Pengali nilai mentah -> satuan kanonik (mis. 0.001 untuk 0,001 dBm -> dBm)'
        AFTER parameter_type_id;

INSERT INTO vendor_parameter_mappings
    (vendor_id, data_model_version_id, device_model_id, logical_key, tr069_path, parameter_type_id, value_scale, description)
SELECT v.id, dmv.id, NULL, k.logical_key, k.tr069_path, pt.id, 0.001, k.description
FROM ref_vendors v
CROSS JOIN (SELECT id FROM ref_data_model_versions WHERE code = 'TR181') dmv
CROSS JOIN (
    SELECT 'optical.rx_power' AS logical_key, 'Device.Optical.Interface.1.OpticalSignalLevel' AS tr069_path, 'int' AS ptype,
           'Level sinyal optik diterima (TR-181 Device.Optical.Interface, satuan 0,001 dBm). Standar; belum diverifikasi pada CPE nyata' AS description
    UNION ALL SELECT 'optical.tx_power', 'Device.Optical.Interface.1.TransmitOpticalLevel', 'int',
           'Level daya optik terkirim (TR-181 Device.Optical.Interface, satuan 0,001 dBm). Standar; belum diverifikasi pada CPE nyata'
) k
LEFT JOIN ref_parameter_types pt ON pt.code = k.ptype
WHERE v.code IN ('ZTE', 'HUAWEI', 'FIBERHOME', 'NOKIA', 'CDATA') AND v.is_deleted = 0
  AND NOT EXISTS (
      SELECT 1 FROM vendor_parameter_mappings m
       WHERE m.vendor_id = v.id AND m.data_model_version_id = dmv.id AND m.device_model_id IS NULL
         AND m.logical_key = k.logical_key AND m.software_version_pattern IS NULL AND m.is_deleted = 0);
