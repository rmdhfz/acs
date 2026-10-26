export interface ListResponse<T> {
  data: T[] | null
  total: number
}

export interface RefLookup {
  id: number
  code: string
  name: string
}

export interface User {
  id: number
  user_uuid: string
  tenant_id: number | null
  username: string
  email: string
  full_name: string
  is_active: boolean
  last_login_at: string | null
  roles: string[]
  /** MFA TOTP aktif (migrations/0027). */
  mfa_enabled?: boolean
  created_at: string
  updated_at: string
}

// AuthUser adalah bentuk ringkas User yang dikembalikan endpoint /auth/login
// (dto manual di auth_handler.go, field lebih sedikit dari domain.User penuh).
export interface AuthUser {
  id: number
  uuid: string
  username: string
  roles: string[]
  tenant_id: number | null
  /** Terisi bila sesi ini adalah SUPERADMIN (id ini) yang meng-impersonate akun ini. */
  impersonator_id?: number
}

// ---- Laporan SaaS (SUPERADMIN) ----
export interface TenantReport {
  tenant_id: number
  code: string
  name: string
  is_active: boolean
  created_at: string
  devices: number
  devices_online: number
  users: number
  users_active_30d: number
  tasks_30d: number
  tasks_failed_30d: number
  pending_tasks: number
  max_pending_tasks: number | null
  max_devices: number | null
  sessions_30d: number
  webhooks_active: number
  last_inform_at: string | null
  last_login_at: string | null
  health: 'sehat' | 'perhatian' | 'nonaktif'
  health_reasons: string[]
}

export interface SaaSReport {
  generated_at: string
  platform: {
    tenants_total: number
    tenants_active: number
    tenants_new_30d: number
    devices_total: number
    devices_online: number
    devices_new_7d: number
    devices_new_30d: number
    users_total: number
    users_active_30d: number
    tasks_30d: number
    tasks_failed_30d: number
    sessions_24h: number
    failed_logins_24h: number
    webhooks_active: number
    firmware_files: number
  }
  tenants: TenantReport[]
  vendors: { vendor: string; devices: number }[]
  daily: { date: string; new_devices: number; sessions: number; tasks: number; tasks_failed: number }[]
}

export interface Device {
  id: number
  device_uuid: string
  tenant_id: number | null
  vendor_id: number | null
  device_model_id: number | null
  device_status_id: number
  provisioning_profile_id: number | null
  oui: string | null
  serial_number: string
  product_class: string | null
  mac_address: string | null
  software_version: string | null
  hardware_version: string | null
  ip_address: string | null
  connection_request_url: string | null
  // Turunan dari GET /devices/:id: username & password Connection Request sudah diisi (nilainya tak pernah diekspos).
  connection_request_credentials_set?: boolean
  // connection_request_username sengaja TIDAK ada di sini -- backend
  // menyembunyikannya dari response (json:"-", domain/device.go) krn
  // dikelompokkan CLAUDE.md sbg kredensial yg tidak boleh diekspos plaintext,
  // konsisten dgn connection_request_password yg sudah lebih dulu tersembunyi.
  inform_username: string | null
  last_inform_at: string | null
  last_boot_event_at: string | null
  latitude: number | null
  longitude: number | null
  notes: string | null
  // Ringkasan baris daftar — hanya ada di GET /devices (bukan /devices/:id). null = belum pernah terbaca dari perangkat.
  rx_power_dbm?: number | null
  tx_power_dbm?: number | null
  optical_at?: string | null
  pppoe_username?: string | null
  created_at: string
  updated_at: string
}

export interface DeviceEvent {
  id: number
  device_id: number
  session_id: number | null
  event_code_id: number
  command_key: string | null
  occurred_at: string
  created_at: string
}

export interface DeviceParameter {
  id: number
  device_id: number
  parameter_name: string
  parameter_value: string | null
  writable: boolean
  created_at: string
  updated_at: string
}

export interface DeviceOpticalMetric {
  id: number
  device_id: number
  rx_power_dbm: number | null
  tx_power_dbm: number | null
  voltage: number | null
  bias_current_ma: number | null
  temperature_celsius: number | null
  distance_meters: number | null
  recorded_at: string
}

export interface Task {
  id: number
  task_uuid: string
  device_id: number
  task_type_id: number
  task_status_id: number
  priority: number
  parameters: unknown | null
  response: unknown | null
  error_message: string | null
  retry_count: number
  max_retries: number
  scheduled_at: string | null
  sent_at: string | null
  completed_at: string | null
  created_at: string
  updated_at: string
}

export interface Vendor {
  id: number
  code: string
  name: string
  description: string | null
  is_active: boolean
  /** Jumlah OUI terdaftar (hanya pada daftar vendor); kosong = 0. */
  oui_count?: number
}

export interface DeviceStatusCount {
  device_status_id: number
  count: number
}

export interface DeviceVendorCount {
  vendor_id: number | null
  count: number
}

export interface DeviceStats {
  by_status: DeviceStatusCount[] | null
  by_vendor: DeviceVendorCount[] | null
}

export interface TaskStatusCount {
  task_status_id: number
  count: number
}

export interface VendorOUI {
  id: number
  vendor_id: number
  oui: string
  notes: string | null
}

export interface DeviceModel {
  id: number
  vendor_id: number
  device_type_id: number
  data_model_version_id: number
  product_class: string | null
  model_name: string
  description: string | null
  is_active: boolean
}

export interface VendorParameterMapping {
  id: number
  vendor_id: number
  data_model_version_id: number
  device_model_id: number | null
  // Pola SQL LIKE opsional thd devices.software_version (migrations/0010).
  // NULL = mapping generik lintas versi software.
  software_version_pattern: string | null
  logical_key: string
  tr069_path: string
  parameter_type_id: number | null
  value_scale?: number
  description: string | null
}

export interface Tenant {
  id: number
  tenant_uuid: string
  code: string
  name: string
  is_active: boolean
  cwmp_inform_username: string | null
  brand_name: string | null
  logo_url: string | null
  primary_color: string | null
  // max_pending_tasks — kuota task queue tenant ini (ROADMAP.md Fase 2).
  // null = tidak dibatasi.
  max_pending_tasks: number | null
  // max_devices — batas perangkat aktif (paket SaaS, migrations/0028). null = tidak dibatasi.
  max_devices: number | null
  // max_storage_bytes — batas total ukuran berkas aktif tenant dalam byte (migrations/0043). null = tidak dibatasi.
  max_storage_bytes: number | null
  created_at: string
}

// CurrentTenant — DTO sempit dari GET /tenants/current (dipanggil semua role
// terautentikasi, bukan cuma superadmin). Sengaja tidak sama dengan `Tenant`
// (yang punya cwmp_inform_username dkk) — endpoint itu tidak boleh
// mengekspos field sensitif ke role non-superadmin.
export interface CurrentTenant {
  id: number
  name: string
  brand_name: string | null
  logo_url: string | null
  primary_color: string | null
}

export interface ProvisioningProfileParameter {
  id?: number
  profile_id?: number
  parameter_name: string
  parameter_value: string | null
  parameter_type_id?: number | null
  apply_order: number
}

export interface ProvisioningProfile {
  id: number
  profile_uuid: string
  tenant_id: number | null
  vendor_id: number | null
  device_model_id: number | null
  name: string
  description: string | null
  is_default: boolean
  is_active: boolean
  created_at: string
}

export interface ZeroTouchRule {
  id: number
  tenant_id: number | null
  vendor_id: number | null
  device_model_id: number | null
  oui: string | null
  serial_pattern: string | null
  // Precondition tambahan (migrations/0009). match_parameter_name &
  // match_parameter_value_pattern wajib berpasangan (keduanya null atau
  // keduanya terisi).
  software_version_pattern: string | null
  match_parameter_name: string | null
  match_parameter_value_pattern: string | null
  // Nullable sejak migrations/0009 — rule boleh hanya memicu reboot dan/atau
  // firmware push tanpa menerapkan provisioning profile.
  provisioning_profile_id: number | null
  post_apply_reboot: boolean
  firmware_file_id: number | null
  // FK ref_ztp_trigger_event — kapan rule dievaluasi (BOOTSTRAP_ONLY /
  // BOOTSTRAP_OR_BOOT / EVERY_INFORM). Wajib.
  trigger_event_id: number
  priority: number
  is_active: boolean
}

// FirmwareRolloutBatch — rencana canary/staged rollout firmware ke populasi
// device bertahap per wave (migrations/0011). Status via ref_firmware_rollout_status.
export interface FirmwareRolloutBatch {
  id: number
  batch_uuid: string
  tenant_id: number | null
  firmware_file_id: number
  vendor_id: number | null
  device_model_id: number | null
  wave_percentage: number
  max_failure_rate_percent: number
  current_wave: number
  status_id: number
  notes: string | null
  scheduled_at: string | null
  started_at: string | null
  completed_at: string | null
  created_at: string
}

export interface FirmwareFile {
  id: number
  firmware_uuid: string
  vendor_id: number
  device_model_id: number | null
  version: string
  file_name: string
  // storage_key: object key di object storage (MinIO), bukan URL — presigned
  // URL digenerate on-demand oleh backend saat penjadwalan upgrade.
  storage_key: string
  file_size_bytes: number | null
  checksum_sha256: string | null
  release_notes: string | null
  is_active: boolean
  created_at: string
}

export interface FirmwareUpgradeJob {
  id: number
  job_uuid: string
  device_id: number
  firmware_id: number
  task_id: number | null
  task_status_id: number
  from_version: string | null
  to_version: string | null
  scheduled_at: string | null
  started_at: string | null
  completed_at: string | null
  error_message: string | null
  created_at: string
}

export interface ActivityLog {
  id: number
  user_id: number | null
  tenant_id: number | null
  action: string
  entity_type: string
  entity_id: number | null
  description: string | null
  ip_address: string | null
  created_at: string
  username: string | null
}

export interface DeviceDiagnostic {
  id: number
  device_id: number
  task_id: number | null
  diagnostic_type: string
  status: string
  result: unknown | null
  executed_at: string | null
  created_at: string
}

export interface GenericFile {
  id: number
  file_uuid: string
  tenant_id: number | null
  file_type: string
  vendor_id: number | null
  device_model_id: number | null
  version: string | null
  file_name: string
  storage_key: string
  file_size_bytes: number
  created_at: string
}

// Portal self-service pelanggan (role ENDUSER) — DTO sempit dari
// GET /self-service/devices, sengaja tanpa kredensial/tenant/audit.
export interface SelfServiceDevice {
  id: number
  serial_number: string
  model: string | null
  software_version: string | null
  status: string
  online: boolean
}

export interface Tag {
  id: number
  tenant_id: number | null
  name: string
  color: string | null
  created_at: string
  updated_at: string
}

export interface Preset {
  id: number
  tenant_id: number | null
  name: string
  weight: number
  precondition: string
  configurations: string
  is_active: boolean
  // migrations/0021 — engine preset. enforce=true: dijaga tiap sesi CWMP saat drift.
  enforce: boolean
  channel: string | null
  created_at: string
  updated_at: string
}

export interface WebhookSubscription {
  id: number
  tenant_id: number | null
  event_type_id: number
  event_type: string
  target_url: string
  secret_key: string | null
  description: string | null
  is_active: boolean
  created_at: string
  updated_at: string
}

export interface WebhookDelivery {
  id: number
  subscription_id: number
  tenant_id: number | null
  event_type_id: number
  event_type: string
  payload: any
  status: string
  request_headers: string | null
  response_code: number | null
  response_body: string | null
  retry_count: number
  next_retry_at: string | null
  created_at: string
  updated_at: string
}


// ApiToken — token API untuk integrasi BSS/OSS. `token_hash` sengaja TIDAK ada
// di sini: backend menandainya `json:"-"`, dan nilai plaintext token hanya
// dikembalikan SEKALI saat penerbitan (lihat IssuedApiToken).
export interface ApiToken {
  id: number
  token_uuid: string
  user_id: number | null
  tenant_id: number | null
  name: string
  scopes: string | null
  expires_at: string | null
  revoked_at: string | null
  created_at: string
  created_by: number | null
  updated_at: string
  updated_by: number | null
}

// IssuedApiToken — respons POST /auth/tokens. `token` adalah satu-satunya
// kesempatan melihat nilai plaintext; setelah ini hanya hash yang tersimpan.
export interface IssuedApiToken {
  token: string
  id: number
  uuid: string
}

// cwmp:Fault aktif dari CPE (GET /faults). `id` = UUID; PK numerik tidak diekspos.
export interface DeviceFault {
  id: string
  device_id: number
  device_serial: string
  tenant_id: number | null
  task_id: number | null
  rpc_method: string
  fault_code: string
  fault_string: string
  occurrence_count: number
  first_seen_at: string
  last_seen_at: string
}

// Virtual parameter: nilai turunan dari parameter perangkat lewat ekspresi deklaratif (GET /virtual-parameters).
// `id` = UUID; tenant_id null = global (semua tenant).
export interface VirtualParameter {
  id: string
  tenant_id: number | null
  name: string
  label: string
  description: string | null
  expression: string
  unit: string | null
  is_active: boolean
  created_at: string
  updated_at: string
}

// Nilai satu virtual parameter pada satu perangkat; value null = belum dapat dihitung.
export interface VirtualValue {
  name: string
  label: string
  unit: string | null
  value: string | null
}

export interface VirtualParameterTestResult {
  ok: boolean
  value: string | null
  refs: { name: string; value: string | null }[]
  error?: { pos: number; message: string }
}

// Aturan otomasi kondisi-aksi (GET /automation-rules). `id` = UUID aturan.
export type AutomationActionType = 'set_parameter' | 'reboot' | 'get_parameter_values' | 'add_tag' | 'webhook'
export type AutomationTrigger = 'BOOTSTRAP_ONLY' | 'BOOTSTRAP_OR_BOOT' | 'EVERY_INFORM'

export interface AutomationAction {
  type: AutomationActionType
  name?: string
  value?: string
  names?: string[]
  tag_id?: number
}

export interface AutomationRule {
  id: string
  tenant_id: number
  name: string
  description: string | null
  trigger_event: AutomationTrigger
  condition: string
  actions: AutomationAction[]
  cooldown_seconds: number
  max_consecutive_fires: number
  priority: number
  is_active: boolean
  created_at: string
  updated_at: string
}

export interface AutomationRun {
  id: string
  rule_id: string
  rule_name: string
  device_id: number
  device_serial: string
  fired: boolean
  suspended: boolean
  summary: string
  error: string | null
  created_at: string
}

export interface AutomationTestResult {
  ok: boolean
  matched: boolean
  would_fire: boolean
  refs: { name: string; value: string | null }[]
  steps: { type: string; detail: string; path?: string; value?: string | null; current?: string | null; change: boolean; skip?: string }[]
  error?: string
}
