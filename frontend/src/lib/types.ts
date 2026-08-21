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
  connection_request_username: string | null
  inform_username: string | null
  last_inform_at: string | null
  last_boot_event_at: string | null
  notes: string | null
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
  parameters: string | null
  response: string | null
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
  logical_key: string
  tr069_path: string
  parameter_type_id: number | null
  description: string | null
}

export interface Tenant {
  id: number
  tenant_uuid: string
  code: string
  name: string
  is_active: boolean
  created_at: string
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
  provisioning_profile_id: number
  priority: number
  is_active: boolean
}

export interface FirmwareFile {
  id: number
  firmware_uuid: string
  vendor_id: number
  device_model_id: number | null
  version: string
  file_name: string
  file_path: string
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

export interface DeviceDiagnostic {
  id: number
  device_id: number
  task_id: number | null
  diagnostic_type: string
  status: string
  result: string | null
  executed_at: string | null
  created_at: string
}
