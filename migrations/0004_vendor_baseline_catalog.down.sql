-- Rollback 0004_vendor_baseline_catalog: hapus mapping baseline TR-098/TR-181
-- yang ditambahkan (device_model_id IS NULL = baris generik vendor+dmv dari
-- migrasi ini, bukan override per-model yang mungkin sudah ditambah manual
-- setelahnya lewat Catalog Vendor UI), lalu hapus vendor Cdata.
DELETE vpm FROM vendor_parameter_mappings vpm
    JOIN ref_vendors v ON v.id = vpm.vendor_id
    JOIN ref_data_model_versions dmv ON dmv.id = vpm.data_model_version_id
WHERE v.code IN ('ZTE', 'HUAWEI', 'FIBERHOME', 'NOKIA', 'CDATA')
  AND dmv.code IN ('TR098', 'TR181')
  AND vpm.device_model_id IS NULL
  AND vpm.logical_key IN (
      'device.manufacturer',
      'device.model_name',
      'device.serial_number',
      'device.software_version',
      'device.hardware_version',
      'device.uptime',
      'device.periodic_inform_interval',
      'device.connection_request_url',
      'wan.pppoe.username',
      'wan.pppoe.password',
      'wifi.ssid',
      'wifi.wpa_passphrase'
  );

DELETE FROM ref_vendors WHERE code = 'CDATA';
