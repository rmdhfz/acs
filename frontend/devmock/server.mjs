// -----------------------------------------------------------------------------
// ACS dev mock API — HANYA untuk melihat UI/UX frontend tanpa backend/DB/Docker.
//
//   node frontend/devmock/server.mjs          (butuh Node 18+, TANPA npm install)
//
// Menyajikan REST /api/v1 palsu di :18080 (sesuai frontend/.env
// VITE_API_BASE_URL). Data statis + sedikit state di memori supaya
// create/hapus terlihat efeknya. BUKAN pengganti backend asli: tidak ada
// TR-069, tidak ada validasi, semua token diterima.
//
// Login: username menentukan peran — `superadmin`, `admin`, `noc`, `viewer`,
// `enduser` (default `superadmin`). Password apa saja.
// -----------------------------------------------------------------------------
import http from 'node:http'

const PORT = 18080
const now = () => new Date().toISOString()
const iso = (offsetMs) => new Date(Date.now() + offsetMs).toISOString()

// ---- ref_* seed (dari schema.sql) ------------------------------------------
const REFS = {
  ref_task_status: [
    { code: 'PENDING', name: 'Menunggu eksekusi' },
    { code: 'QUEUED', name: 'Dalam antrean sesi' },
    { code: 'SENT', name: 'Terkirim ke perangkat' },
    { code: 'COMPLETED', name: 'Berhasil' },
    { code: 'FAILED', name: 'Gagal' },
    { code: 'CANCELLED', name: 'Dibatalkan' },
    { code: 'TIMEOUT', name: 'Time-out menunggu respons' },
  ],
  ref_device_status: [
    { code: 'ONLINE', name: 'Online' },
    { code: 'OFFLINE', name: 'Offline' },
    { code: 'PROVISIONING', name: 'Sedang diprovisioning' },
    { code: 'FAULTY', name: 'Bermasalah' },
    { code: 'UNREGISTERED', name: 'Belum terdaftar/terprovisioning' },
    { code: 'DECOMMISSIONED', name: 'Sudah tidak digunakan' },
  ],
  ref_task_types: [
    { code: 'GET_PARAMETER_VALUES', name: 'Ambil nilai parameter' },
    { code: 'SET_PARAMETER_VALUES', name: 'Ubah nilai parameter' },
    { code: 'GET_PARAMETER_NAMES', name: 'Ambil daftar nama parameter' },
    { code: 'ADD_OBJECT', name: 'Tambah instance object' },
    { code: 'DELETE_OBJECT', name: 'Hapus instance object' },
    { code: 'REBOOT', name: 'Reboot perangkat' },
    { code: 'FACTORY_RESET', name: 'Kembalikan ke setelan pabrik' },
    { code: 'DOWNLOAD', name: 'Transfer file ke perangkat' },
    { code: 'UPLOAD', name: 'Transfer file dari perangkat' },
    { code: 'SCHEDULE_INFORM', name: 'Jadwalkan Inform berikutnya' },
  ],
  ref_parameter_types: ['string', 'int', 'unsignedInt', 'boolean', 'dateTime', 'base64'].map((code) => ({ code, name: code })),
  ref_roles: [
    { code: 'SUPERADMIN', name: 'Superadmin lintas tenant' },
    { code: 'ADMIN', name: 'Admin tenant' },
    { code: 'NOC', name: 'NOC / Operator' },
    { code: 'VIEWER', name: 'Viewer / read-only' },
    { code: 'ENDUSER', name: 'Pelanggan akhir (portal self-service)' },
  ],
  ref_ztp_trigger_event: [
    { code: 'BOOTSTRAP_ONLY', name: 'Hanya saat Bootstrap' },
    { code: 'BOOTSTRAP_OR_BOOT', name: 'Bootstrap atau Boot' },
    { code: 'EVERY_INFORM', name: 'Setiap Inform' },
  ],
  ref_firmware_rollout_status: [
    { code: 'PENDING', name: 'Menunggu dimulai' },
    { code: 'IN_PROGRESS', name: 'Sedang berjalan' },
    { code: 'PAUSED_FAILURE_THRESHOLD', name: 'Dijeda — ambang gagal terlampaui' },
    { code: 'COMPLETED', name: 'Selesai' },
    { code: 'CANCELLED', name: 'Dibatalkan operator' },
  ],
  ref_webhook_event_types: [
    { code: 'DEVICE_FAULT', name: 'Device Fault' },
    { code: 'PARAMETER_VALUE_CHANGE', name: 'Parameter Value Change' },
    { code: 'TASK_FAILED', name: 'Task Failed' },
  ],
  ref_data_model_versions: [
    { code: 'TR-098', name: 'InternetGatewayDevice (TR-098)' },
    { code: 'TR-181', name: 'Device (TR-181)' },
  ],
  ref_device_types: [{ code: 'ONT', name: 'ONT/ONU GPON' }, { code: 'ROUTER', name: 'Router' }],
}
// beri id ke tiap ref row
for (const k of Object.keys(REFS)) REFS[k] = REFS[k].map((r, i) => ({ id: i + 1, ...r }))
const refId = (table, code) => REFS[table]?.find((r) => r.code === code)?.id ?? 1

// ---- data statis ----------------------------------------------------------
const VENDORS = [
  { id: 1, code: 'ZTE', name: 'ZTE Corporation', description: null, is_active: true },
  { id: 2, code: 'HUAWEI', name: 'Huawei Technologies', description: null, is_active: true },
  { id: 3, code: 'FIBERHOME', name: 'FiberHome Technologies', description: null, is_active: true },
  { id: 4, code: 'NOKIA', name: 'Nokia', description: null, is_active: true },
  { id: 5, code: 'CDATA', name: 'C-Data Technology', description: null, is_active: true },
].map((v) => ({ ...v, created_at: iso(-9e9), updated_at: now(), created_by: 1, updated_by: 1, deleted_at: null, deleted_by: null, is_deleted: false }))

const MODELS = [
  { id: 1, vendor_id: 1, device_type_id: 1, data_model_version_id: 1, product_class: 'F660', model_name: 'ZTE F660', description: null, is_active: true },
  { id: 2, vendor_id: 2, device_type_id: 1, data_model_version_id: 2, product_class: 'HG8546M', model_name: 'Huawei HG8546M', description: null, is_active: true },
  { id: 3, vendor_id: 3, device_type_id: 1, data_model_version_id: 1, product_class: 'HG6243C', model_name: 'FiberHome HG6243C', description: null, is_active: true },
]

const STATUS_MIX = ['ONLINE', 'ONLINE', 'ONLINE', 'ONLINE', 'ONLINE', 'OFFLINE', 'OFFLINE', 'PROVISIONING', 'FAULTY', 'UNREGISTERED']
const DEVICES = Array.from({ length: 43 }, (_, i) => {
  const id = i + 1
  const v = VENDORS[i % 5]
  const st = STATUS_MIX[i % STATUS_MIX.length]
  return {
    id,
    device_uuid: `dev-uuid-${String(id).padStart(4, '0')}`,
    tenant_id: 1,
    vendor_id: st === 'UNREGISTERED' ? null : v.id,
    device_model_id: st === 'UNREGISTERED' ? null : MODELS[i % 3].id,
    device_status_id: refId('ref_device_status', st),
    provisioning_profile_id: st === 'ONLINE' ? 1 : null,
    oui: st === 'UNREGISTERED' ? null : ['00259E', '347E5C', '48575D', 'D0577B', 'E067B3'][i % 5],
    serial_number: `${v.code}${String(100000 + id * 37)}`,
    product_class: st === 'UNREGISTERED' ? null : MODELS[i % 3].product_class,
    mac_address: `AA:BB:CC:${String(id).padStart(2, '0')}:${String((id * 7) % 100).padStart(2, '0')}:11`,
    software_version: st === 'UNREGISTERED' ? null : `V9.0.1${i % 5}P${i % 3}`,
    hardware_version: 'V1.0',
    ip_address: `100.64.${id}.${(id * 3) % 254}`,
    connection_request_url: `http://100.64.${id}.${(id * 3) % 254}:7547/cr`,
    connection_request_username: 'acs',
    inform_username: null,
    last_inform_at: iso(-(i % 12) * 3600_000 - 120_000),
    last_boot_event_at: iso(-(i % 30) * 86400_000),
    notes: i % 9 === 0 ? 'Pelanggan VIP — prioritas.' : null,
    created_at: iso(-(i + 5) * 86400_000),
    updated_at: now(),
    created_by: 1, updated_by: 1, deleted_at: null, deleted_by: null, is_deleted: false,
  }
})

const PARAM_TREE = (id) => {
  const dev = DEVICES.find((d) => d.id === id)
  const root = dev?.device_model_id === 2 ? 'Device' : 'InternetGatewayDevice'
  const rows = [
    [`${root}.DeviceInfo.Manufacturer`, VENDORS.find((v) => v.id === dev?.vendor_id)?.name ?? 'Unknown', false],
    [`${root}.DeviceInfo.ModelName`, dev?.product_class ?? '—', false],
    [`${root}.DeviceInfo.SerialNumber`, dev?.serial_number ?? '—', false],
    [`${root}.DeviceInfo.SoftwareVersion`, dev?.software_version ?? '—', false],
    [`${root}.DeviceInfo.UpTime`, String(3600 * (id % 72)), false],
    [`${root}.ManagementServer.PeriodicInformInterval`, '300', true],
    [`${root}.ManagementServer.ConnectionRequestURL`, dev?.connection_request_url ?? '', false],
    [`${root}.LANDevice.1.WLANConfiguration.1.SSID`, `Rumah-${id}`, true],
    [`${root}.LANDevice.1.WLANConfiguration.1.Enable`, 'true', true],
    [`${root}.LANDevice.1.WLANConfiguration.1.KeyPassphrase`, '', true],
    [`${root}.LANDevice.1.WLANConfiguration.5.SSID`, `Rumah-${id}-5G`, true],
    [`${root}.WANDevice.1.WANConnectionDevice.1.WANPPPConnection.1.Username`, `pppoe-${id}@isp.net`, true],
    [`${root}.WANDevice.1.WANConnectionDevice.1.WANPPPConnection.1.ExternalIPAddress`, dev?.ip_address ?? '', false],
  ]
  return rows.map(([name, value, writable], i) => ({
    id: i + 1, device_id: id, parameter_name: name, parameter_value: value,
    parameter_type_id: null, writable, created_at: now(), updated_at: now(),
  }))
}

const TASKS = Array.from({ length: 30 }, (_, i) => {
  const codes = ['COMPLETED', 'COMPLETED', 'COMPLETED', 'PENDING', 'SENT', 'FAILED', 'CANCELLED']
  const code = codes[i % codes.length]
  const type = ['GET_PARAMETER_VALUES', 'SET_PARAMETER_VALUES', 'REBOOT', 'DOWNLOAD'][i % 4]
  return {
    id: i + 1, task_uuid: `task-${i + 1}`, device_id: (i % 43) + 1,
    task_type_id: refId('ref_task_types', type), task_status_id: refId('ref_task_status', code),
    priority: (i % 5) + 3, parameters: type === 'REBOOT' ? {} : { names: ['InternetGatewayDevice.DeviceInfo.'] },
    response: code === 'COMPLETED' ? { ok: true } : null,
    error_message: code === 'FAILED' ? '9005 Invalid Parameter Name' : null,
    retry_count: code === 'FAILED' ? 3 : 0, max_retries: 3,
    scheduled_at: null, expires_at: null,
    sent_at: code === 'SENT' || code === 'COMPLETED' ? iso(-i * 600_000) : null,
    completed_at: code === 'COMPLETED' ? iso(-i * 590_000) : null,
    created_at: iso(-i * 700_000), updated_at: now(), created_by: 1, updated_by: 1,
    deleted_at: null, deleted_by: null, is_deleted: false,
  }
})

// state mutable
let TAGS = [
  { id: 1, tenant_id: 1, name: 'VIP', color: '#dc2626', created_at: iso(-8e8), updated_at: now() },
  { id: 2, tenant_id: 1, name: 'Beta firmware', color: '#2563eb', created_at: iso(-4e8), updated_at: now() },
  { id: 3, tenant_id: null, name: 'Suspended', color: '#64748b', created_at: iso(-2e8), updated_at: now() },
]
let PRESETS = [
  { id: 1, tenant_id: 1, name: 'PPPoE + WiFi standar', weight: 10, precondition: '{"vendor_id":1}', configurations: '[{"op":"set_parameter","key":"device.periodic_inform_interval","value":"300"}]', is_active: true, enforce: true, channel: 'mgmt', created_at: iso(-3e8), updated_at: now() },
  { id: 2, tenant_id: 1, name: 'Nonaktifkan WPS', weight: 5, precondition: '{}', configurations: '[{"op":"set_parameter","key":"InternetGatewayDevice.LANDevice.1.WLANConfiguration.1.WPS.Enable","value":"false"}]', is_active: true, enforce: false, channel: null, created_at: iso(-1e8), updated_at: now() },
]
let FILES = [
  { id: 1, file_uuid: 'f-1', tenant_id: 1, file_type: '1 Firmware Upgrade Image', vendor_id: 1, device_model_id: 1, version: 'V9.0.16', file_name: 'zte-f660-v9.0.16.bin', storage_key: 'firmware/zte-f660-v9.0.16.bin', file_size_bytes: 18234880, created_at: iso(-6e8), updated_at: now(), created_by: 1, updated_by: 1, deleted_at: null, deleted_by: null, is_deleted: false },
  { id: 2, file_uuid: 'f-2', tenant_id: 1, file_type: '3 Vendor Configuration File', vendor_id: 2, device_model_id: null, version: null, file_name: 'huawei-default.cfg', storage_key: 'config/huawei-default.cfg', file_size_bytes: 4096, created_at: iso(-2e8), updated_at: now(), created_by: 1, updated_by: 1, deleted_at: null, deleted_by: null, is_deleted: false },
]
let WEBHOOKS = [
  { id: 1, subscription_uuid: 'wh-1', tenant_id: 1, event_type_id: refId('ref_webhook_event_types', 'TASK_FAILED'), name: 'Alert NOC — task gagal', target_url: 'https://hooks.example.com/acs', is_active: true, description: null, created_at: iso(-5e8), updated_at: now(), created_by: 1, updated_by: 1, deleted_at: null, deleted_by: null, is_deleted: false },
]
let nextId = 1000

const PROFILES = [
  { id: 1, profile_uuid: 'prof-1', tenant_id: 1, vendor_id: 1, device_model_id: null, name: 'ZTE — Aktivasi standar', description: 'PPPoE + WiFi + inform 5 menit', is_default: true, is_active: true, created_at: iso(-7e8), updated_at: now(), created_by: 1, updated_by: 1, deleted_at: null, deleted_by: null, is_deleted: false },
  { id: 2, profile_uuid: 'prof-2', tenant_id: 1, vendor_id: null, device_model_id: null, name: 'Umum — Bridge mode', description: null, is_default: false, is_active: true, created_at: iso(-3e8), updated_at: now(), created_by: 1, updated_by: 1, deleted_at: null, deleted_by: null, is_deleted: false },
]
const ZT_RULES = [
  { id: 1, tenant_id: 1, vendor_id: 1, device_model_id: null, oui: null, serial_pattern: 'ZTE%', software_version_pattern: null, match_parameter_name: null, match_parameter_value_pattern: null, provisioning_profile_id: 1, post_apply_reboot: false, firmware_file_id: null, trigger_event_id: refId('ref_ztp_trigger_event', 'BOOTSTRAP_ONLY'), priority: 10, is_active: true, created_at: iso(-6e8), updated_at: now(), created_by: 1, updated_by: 1, deleted_at: null, deleted_by: null, is_deleted: false },
]
const ROLLOUTS = [
  { id: 1, batch_uuid: 'rb-1', tenant_id: 1, firmware_file_id: 1, vendor_id: 1, device_model_id: 1, wave_percentage: 10, max_failure_rate_percent: 10, current_wave: 2, status_id: refId('ref_firmware_rollout_status', 'IN_PROGRESS'), notes: null, started_at: iso(-2 * 86400_000), completed_at: null, created_at: iso(-3 * 86400_000), updated_at: now(), created_by: 1, updated_by: 1, deleted_at: null, deleted_by: null, is_deleted: false },
  { id: 2, batch_uuid: 'rb-2', tenant_id: 1, firmware_file_id: 1, vendor_id: 2, device_model_id: null, wave_percentage: 25, max_failure_rate_percent: 5, current_wave: 4, status_id: refId('ref_firmware_rollout_status', 'COMPLETED'), notes: null, started_at: iso(-10 * 86400_000), completed_at: iso(-7 * 86400_000), created_at: iso(-11 * 86400_000), updated_at: now(), created_by: 1, updated_by: 1, deleted_at: null, deleted_by: null, is_deleted: false },
]
const USERS = [
  { id: 1, user_uuid: 'u-1', tenant_id: 1, username: 'superadmin', email: 'superadmin@acs.local', full_name: 'Super Admin', is_active: true, last_login_at: now(), roles: ['SUPERADMIN'], created_at: iso(-9e9), updated_at: now(), created_by: null, updated_by: null, deleted_at: null, deleted_by: null, is_deleted: false },
  { id: 2, user_uuid: 'u-2', tenant_id: 1, username: 'noc1', email: 'noc1@acs.local', full_name: 'Operator NOC 1', is_active: true, last_login_at: iso(-3600_000), roles: ['NOC'], created_at: iso(-3e8), updated_at: now(), created_by: 1, updated_by: 1, deleted_at: null, deleted_by: null, is_deleted: false },
]
const TENANTS = [
  { id: 1, tenant_uuid: 't-1', code: 'DEMO', name: 'PT Demo Fiber', is_active: true, cwmp_inform_username: 'demo-inform', brand_name: null, logo_url: null, primary_color: null, max_pending_tasks: null, created_at: iso(-9e9), updated_at: now(), created_by: null, updated_by: null, deleted_at: null, deleted_by: null, is_deleted: false },
]

const roleFor = (username = '') => {
  const u = username.toLowerCase()
  if (u.includes('super')) return ['SUPERADMIN']
  if (u.includes('admin')) return ['ADMIN']
  if (u.includes('noc')) return ['NOC']
  if (u.includes('viewer') || u.includes('view')) return ['VIEWER']
  if (u.includes('end') || u.includes('pelanggan') || u.includes('cust')) return ['ENDUSER']
  return ['SUPERADMIN']
}

const list = (data) => ({ data, total: data.length })
const listMeta = (data) => ({ data, meta: { total: data.length, page: 1, limit: 200 } })

// ---- router --------------------------------------------------------------
function handle(method, path, query, body) {
  // AUTH
  if (method === 'POST' && path === '/auth/login') {
    const username = body?.username || 'superadmin'
    if (!body?.password) return [400, { message: 'password wajib diisi' }]
    return [200, {
      access_token: 'mock.' + Buffer.from(JSON.stringify({ username })).toString('base64') + '.token',
      token_type: 'Bearer',
      user: { id: 1, uuid: 'u-1', username, roles: roleFor(username), tenant_id: username.toLowerCase().includes('super') ? null : 1 },
    }]
  }
  if (method === 'PATCH' && path === '/auth/password') {
    if (!body?.current_password || !body?.new_password) return [400, { message: 'current_password & new_password wajib' }]
    if (String(body.new_password).length < 8) return [400, { message: 'password minimal 8 karakter' }]
    return [204, null]
  }

  // REFS
  let m = path.match(/^\/refs\/(\w+)$/)
  if (method === 'GET' && m) return [REFS[m[1]] ? 200 : 400, REFS[m[1]] ?? { message: `tabel ref tidak dikenal: ${m[1]}` }]

  // DEVICES
  if (method === 'GET' && path === '/devices') {
    let rows = DEVICES
    if (query.search) rows = rows.filter((d) => (d.serial_number + d.mac_address).toLowerCase().includes(query.search.toLowerCase()))
    if (query.vendor_id) rows = rows.filter((d) => String(d.vendor_id) === query.vendor_id)
    if (query.device_status_id) rows = rows.filter((d) => String(d.device_status_id) === query.device_status_id)
    const page = +(query.page || 1), size = +(query.page_size || 50)
    return [200, { data: rows.slice((page - 1) * size, page * size), total: rows.length }]
  }
  if (method === 'GET' && path === '/devices/stats') {
    const by_status = REFS.ref_device_status.map((s) => ({ device_status_id: s.id, count: DEVICES.filter((d) => d.device_status_id === s.id).length })).filter((x) => x.count)
    const by_vendor = VENDORS.map((v) => ({ vendor_id: v.id, count: DEVICES.filter((d) => d.vendor_id === v.id).length })).filter((x) => x.count)
    return [200, { by_status, by_vendor }]
  }
  m = path.match(/^\/devices\/(\d+)$/)
  if (method === 'GET' && m) { const d = DEVICES.find((x) => x.id === +m[1]); return d ? [200, d] : [404, { message: 'device tidak ditemukan' }] }
  m = path.match(/^\/devices\/(\d+)\/parameters$/)
  if (method === 'GET' && m) return [200, PARAM_TREE(+m[1])]
  m = path.match(/^\/devices\/(\d+)\/events$/)
  if (method === 'GET' && m) return [200, list(Array.from({ length: 6 }, (_, i) => ({ id: i + 1, device_id: +m[1], session_id: i + 1, event_code_id: 1, command_key: null, raw_payload: null, occurred_at: iso(-i * 4 * 3600_000), created_at: iso(-i * 4 * 3600_000) })))]
  m = path.match(/^\/devices\/(\d+)\/activity$/)
  if (method === 'GET' && m) return [200, list(Array.from({ length: 4 }, (_, i) => ({ id: i + 1, user_id: 1, tenant_id: 1, action: ['UPDATE_DEVICE', 'APPLY_PROFILE', 'ASSIGN_TAG', 'PRESET_APPLY'][i], entity_type: 'device', entity_id: +m[1], description: null, ip_address: '10.0.0.9', created_at: iso(-i * 6 * 3600_000), username: 'superadmin' })))]
  m = path.match(/^\/devices\/(\d+)\/optical-metrics$/)
  if (method === 'GET' && m) return [200, list(Array.from({ length: 24 }, (_, i) => ({ id: i + 1, device_id: +m[1], rx_power_dbm: -18 - Math.sin(i / 3) * 2 - Math.random(), tx_power_dbm: 2.1 + Math.random() * 0.3, voltage: 3.28, bias_current_ma: 12 + Math.random(), temperature_celsius: 41 + Math.random() * 3, distance_meters: 1230, recorded_at: iso(-i * 3600_000), created_at: iso(-i * 3600_000) })))]
  m = path.match(/^\/devices\/(\d+)\/(firmware-jobs|diagnostics|config-snapshots)$/)
  if (method === 'GET' && m) return [200, list([])]
  m = path.match(/^\/devices\/(\d+)\/tags$/)
  if (method === 'GET' && m) return [200, { data: TAGS.slice(0, +m[1] % 3) }]

  // TASKS
  if (method === 'GET' && path === '/tasks') {
    let rows = TASKS
    if (query.device_id) rows = rows.filter((t) => String(t.device_id) === query.device_id)
    if (query.task_status_id) rows = rows.filter((t) => String(t.task_status_id) === query.task_status_id)
    return [200, { data: rows.slice(0, +(query.page_size || 25)), total: rows.length }]
  }
  if (method === 'GET' && path === '/tasks/stats')
    return [200, REFS.ref_task_status.map((s) => ({ task_status_id: s.id, count: TASKS.filter((t) => t.task_status_id === s.id).length })).filter((x) => x.count)]

  // TENANTS / USERS
  if (method === 'GET' && path === '/tenants/current') return [200, { id: 1, name: 'PT Demo Fiber', brand_name: null, logo_url: null, primary_color: null }]
  if (method === 'GET' && path === '/tenants') return [200, list(TENANTS)]
  if (method === 'GET' && path === '/users') return [200, list(USERS)]

  // CATALOG
  if (method === 'GET' && path === '/vendors') return [200, list(VENDORS)]
  if (method === 'GET' && path === '/device-models') return [200, list(query.vendor_id ? MODELS.filter((x) => String(x.vendor_id) === query.vendor_id) : MODELS)]
  if (method === 'GET' && path === '/vendor-parameter-mappings') return [200, list([])]
  m = path.match(/^\/vendors\/(\d+)\/ouis$/)
  if (method === 'GET' && m) return [200, list([])]

  // PROVISIONING
  if (method === 'GET' && path === '/provisioning-profiles') return [200, list(PROFILES)]
  m = path.match(/^\/provisioning-profiles\/(\d+)$/)
  if (method === 'GET' && m) { const p = PROFILES.find((x) => x.id === +m[1]); return p ? [200, { profile: p, parameters: [{ id: 1, profile_id: p.id, parameter_name: 'wifi.ssid', parameter_value: 'Rumah', parameter_type_id: null, apply_order: 0, created_at: now(), updated_at: now() }] }] : [404, { message: 'tidak ditemukan' }] }
  if (method === 'GET' && path === '/zero-touch-rules') return [200, ZT_RULES]

  // FIRMWARE
  if (method === 'GET' && path === '/firmware') return [200, list(FILES.filter((f) => f.file_type.startsWith('1')))]
  if (method === 'GET' && path === '/firmware/rollout-batches') return [200, list(ROLLOUTS)]
  m = path.match(/^\/firmware\/rollout-batches\/(\d+)$/)
  if (method === 'GET' && m) { const r = ROLLOUTS.find((x) => x.id === +m[1]); return r ? [200, r] : [404, { message: 'tidak ditemukan' }] }

  // FILES / TAGS / PRESETS / WEBHOOKS
  if (method === 'GET' && path === '/files') return [200, listMeta(FILES)]
  if (method === 'GET' && path === '/tags') return [200, listMeta(TAGS)]
  if (method === 'GET' && path === '/presets') return [200, listMeta(PRESETS)]
  if (method === 'GET' && path === '/webhooks') return [200, list(WEBHOOKS)]
  m = path.match(/^\/webhooks\/(\d+)\/deliveries$/)
  if (method === 'GET' && m) return [200, list([{ id: 1, delivery_uuid: 'd-1', subscription_id: +m[1], event_type_id: 3, payload: { task_id: 5 }, status: 'DELIVERED', attempt_count: 1, max_attempts: 5, response_status: 200, error_message: null, next_attempt_at: null, delivered_at: iso(-3600_000), created_at: iso(-3600_000), updated_at: now() }])]
  if (method === 'GET' && path === '/webhooks/deliveries/failed-count') return [200, { count: 2 }]

  // MISC
  if (method === 'GET' && path === '/cwmp/sessions/count') return [200, { count: 7 }]
  if (method === 'GET' && path === '/self-service/devices')
    return [200, list([{ id: 1, serial_number: 'ZTE100037', model: 'ZTE F660', software_version: 'V9.0.11P1', status: 'ONLINE', online: true }])]
  if (method === 'GET' && path === '/metrics')
    return [200, '# HELP acs_devices_by_status jumlah device per status\n# TYPE acs_devices_by_status gauge\nacs_devices_by_status{status="ONLINE"} 25\nacs_cwmp_sessions_open 7\n', 'text/plain']

  // ---- WRITES: simpan sedikit state, sisanya balas sukses -----------------
  if (method === 'POST' && path === '/tags') { const t = { id: ++nextId, tenant_id: 1, name: body?.name || 'tag', color: body?.color ?? null, created_at: now(), updated_at: now() }; TAGS = [t, ...TAGS]; return [201, t] }
  m = path.match(/^\/tags\/(\d+)$/); if (method === 'DELETE' && m) { TAGS = TAGS.filter((t) => t.id !== +m[1]); return [204, null] }
  if (method === 'POST' && path === '/presets') { const p = { id: ++nextId, tenant_id: 1, is_active: true, weight: body?.weight ?? 0, name: body?.name || 'preset', precondition: body?.precondition ?? '{}', configurations: body?.configurations ?? '[]', enforce: !!body?.enforce, channel: body?.channel ?? null, created_at: now(), updated_at: now() }; PRESETS = [p, ...PRESETS]; return [201, p] }
  m = path.match(/^\/presets\/(\d+)$/)
  if (method === 'PATCH' && m) { PRESETS = PRESETS.map((p) => (p.id === +m[1] ? { ...p, ...body, updated_at: now() } : p)); return [200, PRESETS.find((p) => p.id === +m[1])] }
  if (method === 'DELETE' && m) { PRESETS = PRESETS.filter((p) => p.id !== +m[1]); return [204, null] }
  if (method === 'POST' && path === '/webhooks') { const w = { id: ++nextId, subscription_uuid: 'wh-' + nextId, tenant_id: 1, event_type_id: refId('ref_webhook_event_types', body?.event_type || 'TASK_FAILED'), name: body?.name || 'webhook', target_url: body?.target_url || '', is_active: true, description: body?.description ?? null, secret: 'mocksecret_' + Math.random().toString(36).slice(2), created_at: now(), updated_at: now(), created_by: 1, updated_by: 1, deleted_at: null, deleted_by: null, is_deleted: false }; WEBHOOKS = [w, ...WEBHOOKS]; return [201, w] }
  m = path.match(/^\/webhooks\/(\d+)$/)
  if (method === 'PATCH' && m) { WEBHOOKS = WEBHOOKS.map((w) => (w.id === +m[1] ? { ...w, ...body, updated_at: now() } : w)); return [204, null] }
  if (method === 'DELETE' && m) { WEBHOOKS = WEBHOOKS.filter((w) => w.id !== +m[1]); return [204, null] }
  if (method === 'POST' && path.match(/^\/webhooks\/\d+\/test$/)) return [202, { id: ++nextId, delivery_uuid: 'd-' + nextId, subscription_id: 1, event_type_id: 3, payload: { test: true }, status: 'PENDING', attempt_count: 0, max_attempts: 5, response_status: null, error_message: null, next_attempt_at: iso(1000), delivered_at: null, created_at: now(), updated_at: now() }]
  if (method === 'POST' && path === '/files') return [201, { id: ++nextId, file_uuid: 'f-' + nextId, tenant_id: 1, file_type: '1 Firmware Upgrade Image', vendor_id: null, device_model_id: null, version: null, file_name: 'upload.bin', storage_key: 'x', file_size_bytes: 1024, created_at: now(), updated_at: now(), created_by: 1, updated_by: 1, deleted_at: null, deleted_by: null, is_deleted: false }]
  if (method === 'DELETE' && path.match(/^\/files\/\d+$/)) return [204, null]
  if (method === 'POST' && path === '/tasks') return [201, { ...TASKS[0], id: ++nextId, task_uuid: 't-' + nextId, task_status_id: refId('ref_task_status', 'PENDING'), created_at: now() }]
  if (method === 'POST' && path.match(/\/(reboot|factory-reset|connection-request|push-file)$/)) return [202, { message: 'Task diantre. Device akan memproses pada sesi CWMP berikutnya.' }]
  if (method === 'POST' && path.match(/\/tasks\/(add-object|delete-object|get-parameter-names|get-parameter-values|set-parameter-values)$/)) return [202, null]
  if (method === 'POST' && path.match(/\/apply-profile\//)) return [202, { ...TASKS[0], id: ++nextId }]
  if (method === 'POST' && path.match(/\/firmware-upgrade$/)) return [201, { id: ++nextId, job_uuid: 'j-' + nextId, device_id: 1, firmware_id: 1, task_id: null, task_status_id: refId('ref_task_status', 'PENDING'), from_version: null, to_version: null, scheduled_at: null, started_at: null, completed_at: null, error_message: null, created_at: now(), created_by: 1, updated_at: now(), updated_by: 1 }]
  if (method === 'POST' && path.match(/\/rollout-batches\/\d+\/(advance|cancel)$/)) return [200, ROLLOUTS[0]]
  if (method === 'POST' && path.match(/\/diagnostics$/)) return [201, { id: ++nextId, device_id: 1, task_id: null, diagnostic_type: body?.diagnostic_type || 'PING', status: 'PENDING', result: null, executed_at: null, created_at: now() }]

  // fallback: create -> 201 echo, update/delete -> 204, get -> {data:[],total:0}
  if (method === 'GET') return [200, { data: [], total: 0 }]
  if (method === 'DELETE') return [204, null]
  return [200, body ? { ...body, id: ++nextId } : { ok: true }]
}

// ---- server -------------------------------------------------------------
const server = http.createServer((req, res) => {
  const origin = req.headers.origin || '*'
  res.setHeader('Access-Control-Allow-Origin', origin)
  res.setHeader('Access-Control-Allow-Methods', 'GET,POST,PUT,PATCH,DELETE,OPTIONS')
  res.setHeader('Access-Control-Allow-Headers', 'Authorization, Content-Type')
  if (req.method === 'OPTIONS') { res.writeHead(204); return res.end() }

  const url = new URL(req.url, 'http://x')
  if (!url.pathname.startsWith('/api/v1')) { res.writeHead(404); return res.end('not api') }
  const path = url.pathname.slice('/api/v1'.length) || '/'
  const query = Object.fromEntries(url.searchParams)

  let raw = ''
  req.on('data', (c) => (raw += c))
  req.on('end', () => {
    let body = null
    if (raw && (req.headers['content-type'] || '').includes('json')) { try { body = JSON.parse(raw) } catch { /* ignore */ } }
    else if (raw) body = { _raw: raw }
    let status, payload, ctype = 'application/json'
    try {
      ;[status, payload, ctype] = handle(req.method, path, query, body)
      ctype = ctype || 'application/json'
    } catch (e) {
      status = 500; payload = { message: 'mock error: ' + e.message }
    }
    console.log(`${req.method} ${path} -> ${status}`)
    if (payload === null || status === 204) { res.writeHead(status || 204); return res.end() }
    res.writeHead(status, { 'Content-Type': ctype })
    res.end(ctype === 'application/json' ? JSON.stringify(payload) : String(payload))
  })
})

server.listen(PORT, () => {
  console.log(`\n  ACS dev mock API  ->  http://localhost:${PORT}/api/v1`)
  console.log(`  Login: username 'superadmin' | 'admin' | 'noc' | 'viewer' | 'enduser' — password bebas.`)
  console.log(`  Jalankan frontend: cd frontend && npm run dev  (buka http://localhost:5173/login)\n`)
})
