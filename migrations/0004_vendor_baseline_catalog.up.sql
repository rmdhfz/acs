-- =====================================================================
-- 0004_vendor_baseline_catalog
--
-- Konteks: sebelum migrasi ini, `ref_vendors` cuma berisi 4 nama vendor
-- (ZTE/HUAWEI/FIBERHOME/NOKIA dari 0001_init_schema) TANPA satu pun baris
-- di `vendor_ouis`, `device_models`, atau `vendor_parameter_mappings`.
-- Belum pernah ada CPE fisik/emulator yang benar-benar Inform ke sistem
-- ini — migrasi ini murni data starting point, BUKAN validasi lapangan.
--
-- Isi migrasi ini:
--   1. Tambah vendor Cdata (C-Data Technology Co., Ltd) ke ref_vendors.
--   2. Isi vendor_parameter_mappings untuk logical key yang path TR-069-nya
--      benar-benar didefinisikan resmi oleh spec Broadband Forum TR-098
--      (InternetGatewayDevice.*) dan TR-181 (Device:2, Device.*) — SAMA
--      persis di semua vendor by design (itu tujuan standardisasi CWMP).
--
-- Kenapa ada 5x duplikasi baris per logical key (bukan 1 baris "generik"):
-- skema `vendor_parameter_mappings` mendesain vendor_id sebagai NOT NULL
-- dan bagian dari composite unique key (vendor_id, data_model_version_id,
-- device_model_id, logical_key) — lihat schema.sql. Tabel ini memang
-- didesain per-vendor meskipun nilainya kebetulan identik untuk parameter
-- standar; jadi duplikasi 5 baris (satu per vendor) per kombinasi
-- data-model-version x logical-key adalah representasi yang benar dari
-- desain skema saat ini, bukan pelanggaran prinsip "insert data, jangan
-- percabangan kode" — resolve lookup tetap satu jalur data-driven yang
-- sama untuk kelima vendor ini.
--
-- Asumsi instance tunggal (".1") untuk WAN/LAN/WiFi/PPP:
-- Path di bawah memakai instance pertama (mis. WANConnectionDevice.1,
-- WLANConfiguration.1, PPP.Interface.1) — konvensi yang berlaku untuk
-- mayoritas CPE residensial single-WAN/single-radio-utama. Ini BUKAN
-- jaminan mutlak dari spec (TR-069 tidak memaksa index tertentu untuk
-- WAN/LAN interface instance), tapi merupakan default paling umum yang
-- dipakai hampir seluruh implementasi ACS di industri. Kalau ada model
-- tertentu yang butuh instance berbeda, override via baris
-- vendor_parameter_mappings dengan device_model_id spesifik (lookup
-- paling spesifik: device_model dulu, baru fallback ke baris generik ini).
--
-- SENGAJA TIDAK DIISI (jangan tambahkan tanpa konfirmasi dari dokumentasi
-- resmi vendor atau akses device nyata):
--   - wifi.5g.ssid / wifi.24g.ssid (split per band): index WLANConfiguration
--     (TR-098) / SSID instance (TR-181) mana yang mewakili radio 2.4GHz vs
--     5GHz TIDAK distandarkan — beda-beda per vendor bahkan per model/
--     firmware (2, 5, 6, 8, dst). Hanya diisi `wifi.ssid`/`wifi.wpa_passphrase`
--     generik (instance 1) di migrasi ini.
--   - wan.ip_address: untuk TR-098, ExternalIPAddress bisa ada di bawah
--     WANIPConnection ATAU WANPPPConnection tergantung tipe koneksi WAN
--     (DHCP vs PPPoE) — ambigu tanpa tahu konfigurasi device nyata. Untuk
--     TR-181, Device.IP.Interface.{i} adalah list generik tanpa index tetap
--     untuk "yang mana WAN" — index-nya baru diketahui lewat penjelajahan
--     (GetParameterNames) device sungguhan. Tidak diisi sama sekali di sini.
--   - device.optical.rx_power / device.optical.tx_power (redaman optik ONU):
--     di semua 5 vendor ini nyaris selalu lewat vendor extension object
--     nonstandar (pola X_<OUI>_... atau X_<VendorPrefix>_...), BUKAN path
--     standar TR-098/TR-181. Path pastinya beda per vendor DAN kadang per
--     model/firmware dalam vendor yang sama. Tidak ada satu pun yang diisi
--     di sini karena tidak ada kepastian tinggi terhadap path spesifiknya —
--     kalau dipaksa tebak lalu dipakai push SetParameterValues ke device
--     produksi nyata, salah path bisa merusak konfigurasi pelanggan. Perlu
--     dokumentasi resmi per-vendor atau akses device nyata dulu (lihat
--     VENDOR_ONBOARDING.md) sebelum baris ini diisi lewat Catalog Vendor UI.
--   - device_models: tidak ada baris ditambahkan sama sekali di migrasi ini
--     (termasuk untuk Cdata) — tidak ada data model/product_class yang
--     confident dari sample device nyata. Operator mengisi via Catalog
--     Vendor UI begitu ada info model yang dipakai (lihat VENDOR_ONBOARDING.md
--     §3).
--   - vendor_ouis: TIDAK ada satu baris pun ditambahkan untuk kelima vendor
--     ini (termasuk yang sudah ada sebelumnya: ZTE/HUAWEI/FIBERHOME/NOKIA).
--     OUI adalah 6 hex digit persis yang tervalidasi dari IEEE OUI registry
--     publik (https://standards-oui.ieee.org/); satu digit salah akan bikin
--     device baru silently ter-assign ke vendor_id yang SALAH saat Inform
--     pertama (lihat FindOrCreateFromInform), yang lalu bisa membuat mapping
--     parameter vendor yang salah dipakai untuk push konfigurasi — risikonya
--     setara dengan salah path TR-069. Karena tidak ada akses verifikasi
--     langsung ke registry tsb saat migrasi ini dibuat, dan daftar OUI resmi
--     per vendor besar (ZTE/Huawei/FiberHome/Nokia) biasanya mencakup banyak
--     blok (puluhan) yang berubah dari waktu ke waktu, mengisi sebagian dari
--     ingatan/tebakan dinilai lebih berbahaya daripada kosong. TODO operasional:
--     lengkapi tabel ini via Catalog Vendor UI (`+ OUI` per vendor, lihat
--     VENDOR_ONBOARDING.md §2) dengan OUI yang sudah diverifikasi satu per
--     satu dari registry resmi, idealnya dicocokkan juga ke sample device
--     nyata yang benar-benar dipegang.
-- =====================================================================

INSERT INTO ref_vendors (code, name, description) VALUES
    ('CDATA', 'C-Data Technology Co., Ltd', 'Produsen ONU/OLT GPON/EPON, umum dipakai ISP kecil-menengah');

-- ---------------------------------------------------------------------
-- vendor_parameter_mappings — TR-098 (InternetGatewayDevice.*)
-- ---------------------------------------------------------------------
INSERT INTO vendor_parameter_mappings
    (vendor_id, data_model_version_id, device_model_id, logical_key, tr069_path, parameter_type_id, description)
SELECT
    v.id,
    dmv.id,
    NULL,
    k.logical_key,
    k.tr069_path,
    pt.id,
    k.description
FROM ref_vendors v
CROSS JOIN (SELECT id FROM ref_data_model_versions WHERE code = 'TR098') dmv
CROSS JOIN (
    SELECT 'device.manufacturer' AS logical_key, 'InternetGatewayDevice.DeviceInfo.Manufacturer' AS tr069_path, 'string' AS ptype, 'Nama manufacturer perangkat (TR-098 DeviceInfo, standar)' AS description
    UNION ALL SELECT 'device.model_name', 'InternetGatewayDevice.DeviceInfo.ModelName', 'string', 'Nama model perangkat (TR-098 DeviceInfo, standar)'
    UNION ALL SELECT 'device.serial_number', 'InternetGatewayDevice.DeviceInfo.SerialNumber', 'string', 'Serial number perangkat (TR-098 DeviceInfo, standar)'
    UNION ALL SELECT 'device.software_version', 'InternetGatewayDevice.DeviceInfo.SoftwareVersion', 'string', 'Versi firmware/software (TR-098 DeviceInfo, standar)'
    UNION ALL SELECT 'device.hardware_version', 'InternetGatewayDevice.DeviceInfo.HardwareVersion', 'string', 'Versi hardware (TR-098 DeviceInfo, standar)'
    UNION ALL SELECT 'device.uptime', 'InternetGatewayDevice.DeviceInfo.UpTime', 'unsignedInt', 'Waktu sejak boot terakhir dalam detik (TR-098 DeviceInfo, standar)'
    UNION ALL SELECT 'device.periodic_inform_interval', 'InternetGatewayDevice.ManagementServer.PeriodicInformInterval', 'unsignedInt', 'Interval Periodic Inform dalam detik (TR-098 ManagementServer, standar)'
    UNION ALL SELECT 'device.connection_request_url', 'InternetGatewayDevice.ManagementServer.ConnectionRequestURL', 'string', 'URL Connection Request milik CPE (TR-098 ManagementServer, standar)'
    UNION ALL SELECT 'wan.pppoe.username', 'InternetGatewayDevice.WANDevice.1.WANConnectionDevice.1.WANPPPConnection.1.Username', 'string', 'Username PPPoE WAN — asumsi instance WANDevice/WANConnectionDevice/WANPPPConnection pertama (.1.1.1), lihat catatan asumsi instance tunggal di header migrasi'
    UNION ALL SELECT 'wan.pppoe.password', 'InternetGatewayDevice.WANDevice.1.WANConnectionDevice.1.WANPPPConnection.1.Password', 'string', 'Password PPPoE WAN — asumsi instance .1.1.1, lihat catatan asumsi instance tunggal di header migrasi'
    UNION ALL SELECT 'wifi.ssid', 'InternetGatewayDevice.LANDevice.1.WLANConfiguration.1.SSID', 'string', 'SSID WiFi radio utama (instance 1) — TIDAK dibedakan 2.4G/5G di sini, lihat catatan di header migrasi soal ambiguitas index radio'
    UNION ALL SELECT 'wifi.wpa_passphrase', 'InternetGatewayDevice.LANDevice.1.WLANConfiguration.1.PreSharedKey.1.KeyPassphrase', 'string', 'WPA/WPA2 passphrase WiFi radio utama (instance 1), object PreSharedKey resmi TR-098 Amendment 2'
) k
LEFT JOIN ref_parameter_types pt ON pt.code = k.ptype
WHERE v.code IN ('ZTE', 'HUAWEI', 'FIBERHOME', 'NOKIA', 'CDATA');

-- ---------------------------------------------------------------------
-- vendor_parameter_mappings — TR-181 (Device:2, Device.*)
-- ---------------------------------------------------------------------
INSERT INTO vendor_parameter_mappings
    (vendor_id, data_model_version_id, device_model_id, logical_key, tr069_path, parameter_type_id, description)
SELECT
    v.id,
    dmv.id,
    NULL,
    k.logical_key,
    k.tr069_path,
    pt.id,
    k.description
FROM ref_vendors v
CROSS JOIN (SELECT id FROM ref_data_model_versions WHERE code = 'TR181') dmv
CROSS JOIN (
    SELECT 'device.manufacturer' AS logical_key, 'Device.DeviceInfo.Manufacturer' AS tr069_path, 'string' AS ptype, 'Nama manufacturer perangkat (TR-181 DeviceInfo, standar)' AS description
    UNION ALL SELECT 'device.model_name', 'Device.DeviceInfo.ModelName', 'string', 'Nama model perangkat (TR-181 DeviceInfo, standar)'
    UNION ALL SELECT 'device.serial_number', 'Device.DeviceInfo.SerialNumber', 'string', 'Serial number perangkat (TR-181 DeviceInfo, standar)'
    UNION ALL SELECT 'device.software_version', 'Device.DeviceInfo.SoftwareVersion', 'string', 'Versi firmware/software (TR-181 DeviceInfo, standar)'
    UNION ALL SELECT 'device.hardware_version', 'Device.DeviceInfo.HardwareVersion', 'string', 'Versi hardware (TR-181 DeviceInfo, standar)'
    UNION ALL SELECT 'device.uptime', 'Device.DeviceInfo.UpTime', 'unsignedInt', 'Waktu sejak boot terakhir dalam detik (TR-181 DeviceInfo, standar)'
    UNION ALL SELECT 'device.periodic_inform_interval', 'Device.ManagementServer.PeriodicInformInterval', 'unsignedInt', 'Interval Periodic Inform dalam detik (TR-181 ManagementServer, standar)'
    UNION ALL SELECT 'device.connection_request_url', 'Device.ManagementServer.ConnectionRequestURL', 'string', 'URL Connection Request milik CPE (TR-181 ManagementServer, standar)'
    UNION ALL SELECT 'wan.pppoe.username', 'Device.PPP.Interface.1.Username', 'string', 'Username PPPoE WAN — asumsi instance PPP.Interface pertama (.1), lihat catatan asumsi instance tunggal di header migrasi'
    UNION ALL SELECT 'wan.pppoe.password', 'Device.PPP.Interface.1.Password', 'string', 'Password PPPoE WAN — asumsi instance .1, lihat catatan asumsi instance tunggal di header migrasi'
    UNION ALL SELECT 'wifi.ssid', 'Device.WiFi.SSID.1.SSID', 'string', 'SSID WiFi radio utama (instance 1) — TIDAK dibedakan 2.4G/5G di sini, lihat catatan di header migrasi soal ambiguitas index radio'
    UNION ALL SELECT 'wifi.wpa_passphrase', 'Device.WiFi.AccessPoint.1.Security.KeyPassphrase', 'string', 'WPA/WPA2 passphrase WiFi radio utama (instance 1), object AccessPoint.Security resmi TR-181'
) k
LEFT JOIN ref_parameter_types pt ON pt.code = k.ptype
WHERE v.code IN ('ZTE', 'HUAWEI', 'FIBERHOME', 'NOKIA', 'CDATA');
