-- 0025_diagnostics_mappings
--
-- Pemetaan logical key untuk diagnostik Ping & Traceroute (standar TR-098 §IPPingDiagnostics /
-- TraceRouteDiagnostics dan TR-181 Device.IP.Diagnostics.IPPing / TraceRoute). Path-nya
-- standar lintas vendor, tetapi — sesuai aturan proyek — tetap lewat vendor_parameter_mappings
-- (bukan hardcode di kode) sehingga override per vendor/model dimungkinkan. Berlaku untuk
-- vendor baseline (ZTE, HUAWEI, FIBERHOME, NOKIA, CDATA); vendor lain ditambahkan lewat
-- Katalog Vendor (impor CSV). Hanya INSERT jika belum ada (idempoten; unique key tidak
-- menangkap NULL pada device_model_id, jadi dicek NOT EXISTS).

INSERT INTO vendor_parameter_mappings
    (vendor_id, data_model_version_id, device_model_id, logical_key, tr069_path, parameter_type_id, description)
SELECT v.id, dmv.id, NULL, k.logical_key, k.tr069_path, pt.id, k.description
FROM ref_vendors v
CROSS JOIN (SELECT id FROM ref_data_model_versions WHERE code = 'TR098') dmv
CROSS JOIN (
    SELECT 'diag.ping.state' AS logical_key, 'InternetGatewayDevice.IPPingDiagnostics.DiagnosticsState' AS tr069_path, 'string' AS ptype, 'Diagnostik Ping (TR098, parameter standar DiagnosticsState)' AS description
    UNION ALL SELECT 'diag.ping.host' AS logical_key, 'InternetGatewayDevice.IPPingDiagnostics.Host' AS tr069_path, 'string' AS ptype, 'Diagnostik Ping (TR098, parameter standar Host)' AS description
    UNION ALL SELECT 'diag.ping.repetitions' AS logical_key, 'InternetGatewayDevice.IPPingDiagnostics.NumberOfRepetitions' AS tr069_path, 'unsignedInt' AS ptype, 'Diagnostik Ping (TR098, parameter standar NumberOfRepetitions)' AS description
    UNION ALL SELECT 'diag.ping.timeout' AS logical_key, 'InternetGatewayDevice.IPPingDiagnostics.Timeout' AS tr069_path, 'unsignedInt' AS ptype, 'Diagnostik Ping (TR098, parameter standar Timeout)' AS description
    UNION ALL SELECT 'diag.ping.block_size' AS logical_key, 'InternetGatewayDevice.IPPingDiagnostics.DataBlockSize' AS tr069_path, 'unsignedInt' AS ptype, 'Diagnostik Ping (TR098, parameter standar DataBlockSize)' AS description
    UNION ALL SELECT 'diag.ping.success_count' AS logical_key, 'InternetGatewayDevice.IPPingDiagnostics.SuccessCount' AS tr069_path, 'unsignedInt' AS ptype, 'Diagnostik Ping (TR098, parameter standar SuccessCount)' AS description
    UNION ALL SELECT 'diag.ping.failure_count' AS logical_key, 'InternetGatewayDevice.IPPingDiagnostics.FailureCount' AS tr069_path, 'unsignedInt' AS ptype, 'Diagnostik Ping (TR098, parameter standar FailureCount)' AS description
    UNION ALL SELECT 'diag.ping.avg_rtt' AS logical_key, 'InternetGatewayDevice.IPPingDiagnostics.AverageResponseTime' AS tr069_path, 'unsignedInt' AS ptype, 'Diagnostik Ping (TR098, parameter standar AverageResponseTime)' AS description
    UNION ALL SELECT 'diag.ping.min_rtt' AS logical_key, 'InternetGatewayDevice.IPPingDiagnostics.MinimumResponseTime' AS tr069_path, 'unsignedInt' AS ptype, 'Diagnostik Ping (TR098, parameter standar MinimumResponseTime)' AS description
    UNION ALL SELECT 'diag.ping.max_rtt' AS logical_key, 'InternetGatewayDevice.IPPingDiagnostics.MaximumResponseTime' AS tr069_path, 'unsignedInt' AS ptype, 'Diagnostik Ping (TR098, parameter standar MaximumResponseTime)' AS description
    UNION ALL SELECT 'diag.traceroute.state' AS logical_key, 'InternetGatewayDevice.TraceRouteDiagnostics.DiagnosticsState' AS tr069_path, 'string' AS ptype, 'Diagnostik Traceroute (TR098, parameter standar DiagnosticsState)' AS description
    UNION ALL SELECT 'diag.traceroute.host' AS logical_key, 'InternetGatewayDevice.TraceRouteDiagnostics.Host' AS tr069_path, 'string' AS ptype, 'Diagnostik Traceroute (TR098, parameter standar Host)' AS description
    UNION ALL SELECT 'diag.traceroute.tries' AS logical_key, 'InternetGatewayDevice.TraceRouteDiagnostics.NumberOfTries' AS tr069_path, 'unsignedInt' AS ptype, 'Diagnostik Traceroute (TR098, parameter standar NumberOfTries)' AS description
    UNION ALL SELECT 'diag.traceroute.timeout' AS logical_key, 'InternetGatewayDevice.TraceRouteDiagnostics.Timeout' AS tr069_path, 'unsignedInt' AS ptype, 'Diagnostik Traceroute (TR098, parameter standar Timeout)' AS description
    UNION ALL SELECT 'diag.traceroute.block_size' AS logical_key, 'InternetGatewayDevice.TraceRouteDiagnostics.DataBlockSize' AS tr069_path, 'unsignedInt' AS ptype, 'Diagnostik Traceroute (TR098, parameter standar DataBlockSize)' AS description
    UNION ALL SELECT 'diag.traceroute.max_hops' AS logical_key, 'InternetGatewayDevice.TraceRouteDiagnostics.MaxHopCount' AS tr069_path, 'unsignedInt' AS ptype, 'Diagnostik Traceroute (TR098, parameter standar MaxHopCount)' AS description
    UNION ALL SELECT 'diag.traceroute.response_time' AS logical_key, 'InternetGatewayDevice.TraceRouteDiagnostics.ResponseTime' AS tr069_path, 'unsignedInt' AS ptype, 'Diagnostik Traceroute (TR098, parameter standar ResponseTime)' AS description
    UNION ALL SELECT 'diag.traceroute.hops' AS logical_key, 'InternetGatewayDevice.TraceRouteDiagnostics.RouteHops.' AS tr069_path, 'string' AS ptype, 'Diagnostik Traceroute (TR098, parameter standar RouteHops.)' AS description
) k
LEFT JOIN ref_parameter_types pt ON pt.code = k.ptype
WHERE v.code IN ('ZTE', 'HUAWEI', 'FIBERHOME', 'NOKIA', 'CDATA') AND v.is_deleted = 0
  AND NOT EXISTS (
      SELECT 1 FROM vendor_parameter_mappings m
       WHERE m.vendor_id = v.id AND m.data_model_version_id = dmv.id AND m.device_model_id IS NULL
         AND m.logical_key = k.logical_key AND m.software_version_pattern IS NULL AND m.is_deleted = 0);

INSERT INTO vendor_parameter_mappings
    (vendor_id, data_model_version_id, device_model_id, logical_key, tr069_path, parameter_type_id, description)
SELECT v.id, dmv.id, NULL, k.logical_key, k.tr069_path, pt.id, k.description
FROM ref_vendors v
CROSS JOIN (SELECT id FROM ref_data_model_versions WHERE code = 'TR181') dmv
CROSS JOIN (
    SELECT 'diag.ping.state' AS logical_key, 'Device.IP.Diagnostics.IPPing.DiagnosticsState' AS tr069_path, 'string' AS ptype, 'Diagnostik Ping (TR181, parameter standar DiagnosticsState)' AS description
    UNION ALL SELECT 'diag.ping.host' AS logical_key, 'Device.IP.Diagnostics.IPPing.Host' AS tr069_path, 'string' AS ptype, 'Diagnostik Ping (TR181, parameter standar Host)' AS description
    UNION ALL SELECT 'diag.ping.repetitions' AS logical_key, 'Device.IP.Diagnostics.IPPing.NumberOfRepetitions' AS tr069_path, 'unsignedInt' AS ptype, 'Diagnostik Ping (TR181, parameter standar NumberOfRepetitions)' AS description
    UNION ALL SELECT 'diag.ping.timeout' AS logical_key, 'Device.IP.Diagnostics.IPPing.Timeout' AS tr069_path, 'unsignedInt' AS ptype, 'Diagnostik Ping (TR181, parameter standar Timeout)' AS description
    UNION ALL SELECT 'diag.ping.block_size' AS logical_key, 'Device.IP.Diagnostics.IPPing.DataBlockSize' AS tr069_path, 'unsignedInt' AS ptype, 'Diagnostik Ping (TR181, parameter standar DataBlockSize)' AS description
    UNION ALL SELECT 'diag.ping.success_count' AS logical_key, 'Device.IP.Diagnostics.IPPing.SuccessCount' AS tr069_path, 'unsignedInt' AS ptype, 'Diagnostik Ping (TR181, parameter standar SuccessCount)' AS description
    UNION ALL SELECT 'diag.ping.failure_count' AS logical_key, 'Device.IP.Diagnostics.IPPing.FailureCount' AS tr069_path, 'unsignedInt' AS ptype, 'Diagnostik Ping (TR181, parameter standar FailureCount)' AS description
    UNION ALL SELECT 'diag.ping.avg_rtt' AS logical_key, 'Device.IP.Diagnostics.IPPing.AverageResponseTime' AS tr069_path, 'unsignedInt' AS ptype, 'Diagnostik Ping (TR181, parameter standar AverageResponseTime)' AS description
    UNION ALL SELECT 'diag.ping.min_rtt' AS logical_key, 'Device.IP.Diagnostics.IPPing.MinimumResponseTime' AS tr069_path, 'unsignedInt' AS ptype, 'Diagnostik Ping (TR181, parameter standar MinimumResponseTime)' AS description
    UNION ALL SELECT 'diag.ping.max_rtt' AS logical_key, 'Device.IP.Diagnostics.IPPing.MaximumResponseTime' AS tr069_path, 'unsignedInt' AS ptype, 'Diagnostik Ping (TR181, parameter standar MaximumResponseTime)' AS description
    UNION ALL SELECT 'diag.traceroute.state' AS logical_key, 'Device.IP.Diagnostics.TraceRoute.DiagnosticsState' AS tr069_path, 'string' AS ptype, 'Diagnostik Traceroute (TR181, parameter standar DiagnosticsState)' AS description
    UNION ALL SELECT 'diag.traceroute.host' AS logical_key, 'Device.IP.Diagnostics.TraceRoute.Host' AS tr069_path, 'string' AS ptype, 'Diagnostik Traceroute (TR181, parameter standar Host)' AS description
    UNION ALL SELECT 'diag.traceroute.tries' AS logical_key, 'Device.IP.Diagnostics.TraceRoute.NumberOfTries' AS tr069_path, 'unsignedInt' AS ptype, 'Diagnostik Traceroute (TR181, parameter standar NumberOfTries)' AS description
    UNION ALL SELECT 'diag.traceroute.timeout' AS logical_key, 'Device.IP.Diagnostics.TraceRoute.Timeout' AS tr069_path, 'unsignedInt' AS ptype, 'Diagnostik Traceroute (TR181, parameter standar Timeout)' AS description
    UNION ALL SELECT 'diag.traceroute.block_size' AS logical_key, 'Device.IP.Diagnostics.TraceRoute.DataBlockSize' AS tr069_path, 'unsignedInt' AS ptype, 'Diagnostik Traceroute (TR181, parameter standar DataBlockSize)' AS description
    UNION ALL SELECT 'diag.traceroute.max_hops' AS logical_key, 'Device.IP.Diagnostics.TraceRoute.MaxHopCount' AS tr069_path, 'unsignedInt' AS ptype, 'Diagnostik Traceroute (TR181, parameter standar MaxHopCount)' AS description
    UNION ALL SELECT 'diag.traceroute.response_time' AS logical_key, 'Device.IP.Diagnostics.TraceRoute.ResponseTime' AS tr069_path, 'unsignedInt' AS ptype, 'Diagnostik Traceroute (TR181, parameter standar ResponseTime)' AS description
    UNION ALL SELECT 'diag.traceroute.hops' AS logical_key, 'Device.IP.Diagnostics.TraceRoute.RouteHops.' AS tr069_path, 'string' AS ptype, 'Diagnostik Traceroute (TR181, parameter standar RouteHops.)' AS description
) k
LEFT JOIN ref_parameter_types pt ON pt.code = k.ptype
WHERE v.code IN ('ZTE', 'HUAWEI', 'FIBERHOME', 'NOKIA', 'CDATA') AND v.is_deleted = 0
  AND NOT EXISTS (
      SELECT 1 FROM vendor_parameter_mappings m
       WHERE m.vendor_id = v.id AND m.data_model_version_id = dmv.id AND m.device_model_id IS NULL
         AND m.logical_key = k.logical_key AND m.software_version_pattern IS NULL AND m.is_deleted = 0);
