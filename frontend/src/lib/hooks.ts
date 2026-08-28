import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from './api'
import type {
  ActivityLog,
  CurrentTenant,
  Device,
  DeviceDiagnostic,
  DeviceEvent,
  DeviceModel,
  DeviceOpticalMetric,
  DeviceParameter,
  DeviceStats,
  GenericFile,
  FirmwareFile,
  Tag,
  Preset,
  FirmwareRolloutBatch,
  FirmwareUpgradeJob,
  ListResponse,
  ProvisioningProfile,
  ProvisioningProfileParameter,
  RefLookup,
  Task,
  TaskStatusCount,
  Tenant,
  User,
  Vendor,
  VendorOUI,
  VendorParameterMapping,
  ZeroTouchRule,
} from './types'

function buildQuery(params: object) {
  const usp = new URLSearchParams()
  for (const [key, value] of Object.entries(params as Record<string, unknown>)) {
    if (value !== undefined && value !== '') usp.set(key, String(value))
  }
  const qs = usp.toString()
  return qs ? `?${qs}` : ''
}

// ---- Reference lookups (ref_*) ----

export function useRefs(table: string) {
  return useQuery({
    queryKey: ['refs', table],
    queryFn: () => api.get<RefLookup[]>(`/refs/${table}`),
    staleTime: 5 * 60_000,
  })
}

export function findRefById(refs: RefLookup[] | undefined, id: number | null | undefined) {
  if (!refs || id == null) return undefined
  return refs.find((r) => r.id === id)
}

export function findRefIdByCode(refs: RefLookup[] | undefined, code: string) {
  return refs?.find((r) => r.code === code)?.id
}

// ---- Vendors ----

export function useVendors() {
  return useQuery({
    queryKey: ['vendors'],
    queryFn: () => api.get<ListResponse<Vendor>>('/vendors?page_size=200'),
    staleTime: 5 * 60_000,
  })
}

// ---- Devices ----

export interface DeviceFilters {
  search?: string
  device_status_id?: number
  vendor_id?: number
  page?: number
  page_size?: number
}

const LIVE_REFRESH_MS = 60000 // Diperpanjang karena sudah menggunakan WebSockets

export function useDevices(filters: DeviceFilters) {
  return useQuery({
    queryKey: ['devices', filters],
    queryFn: () => api.get<ListResponse<Device>>(`/devices${buildQuery(filters)}`),
    refetchInterval: LIVE_REFRESH_MS,
    placeholderData: (prev) => prev,
  })
}

export function useDevice(id: number) {
  return useQuery({
    queryKey: ['device', id],
    queryFn: () => api.get<Device>(`/devices/${id}`),
    refetchInterval: LIVE_REFRESH_MS,
  })
}

export function useUpdateDevice() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, ...payload }: { id: number; notes?: string; connection_request_url?: string; connection_request_username?: string; connection_request_password?: string; latitude?: number; longitude?: number }) =>
      api.patch<Device>(`/devices/${id}`, payload),
    onSuccess: (updatedDevice) => {
      queryClient.setQueryData(['device', updatedDevice.id], updatedDevice)
      queryClient.invalidateQueries({ queryKey: ['devices'] })
    },
  })
}

export function useDeviceEvents(id: number) {
  return useQuery({
    queryKey: ['device', id, 'events'],
    queryFn: () => api.get<ListResponse<DeviceEvent>>(`/devices/${id}/events?page_size=50`),
    refetchInterval: LIVE_REFRESH_MS,
  })
}

export function useDeviceParameters(id: number) {
  return useQuery({
    queryKey: ['device', id, 'parameters'],
    queryFn: () => api.get<DeviceParameter[]>(`/devices/${id}/parameters`),
    refetchInterval: LIVE_REFRESH_MS,
  })
}

export function useDeviceTasks(id: number) {
  return useQuery({
    queryKey: ['device', id, 'tasks'],
    queryFn: () => api.get<ListResponse<Task>>(`/tasks${buildQuery({ device_id: id, page_size: 50 })}`),
    refetchInterval: LIVE_REFRESH_MS,
  })
}

export function useDeviceActivity(id: number) {
  return useQuery({
    queryKey: ['device', id, 'activity'],
    queryFn: () => api.get<ListResponse<ActivityLog>>(`/devices/${id}/activity?page_size=50`),
    refetchInterval: LIVE_REFRESH_MS,
  })
}

export function useDeviceOpticalMetrics(id: number) {
  return useQuery({
    queryKey: ['device', id, 'optical-metrics'],
    queryFn: () => api.get<ListResponse<DeviceOpticalMetric>>(`/devices/${id}/optical-metrics?page_size=200`),
    refetchInterval: LIVE_REFRESH_MS,
  })
}

// ---- Dashboard analitik (ROADMAP.md Fase 1) ----

export function useDeviceStats() {
  return useQuery({
    queryKey: ['devices', 'stats'],
    queryFn: () => api.get<DeviceStats>('/devices/stats'),
    refetchInterval: LIVE_REFRESH_MS,
  })
}

export function useCwmpSessionsCount() {
  return useQuery({
    queryKey: ['cwmp', 'sessions', 'count'],
    queryFn: () => api.get<{ count: number }>('/cwmp/sessions/count'),
    refetchInterval: LIVE_REFRESH_MS,
  })
}

export function useWebhookFailedCount() {
  return useQuery({
    queryKey: ['webhooks', 'deliveries', 'failed-count'],
    queryFn: () => api.get<{ count: number }>('/webhooks/deliveries/failed-count'),
    refetchInterval: LIVE_REFRESH_MS,
  })
}

export function useTaskStats() {
  return useQuery({
    queryKey: ['tasks', 'stats'],
    queryFn: () => api.get<TaskStatusCount[]>('/tasks/stats'),
    refetchInterval: LIVE_REFRESH_MS,
  })
}

// ---- Ringkasan task pending (dipakai stat card dashboard) ----

export function usePendingTaskCount(pendingStatusId: number | undefined) {
  return useQuery({
    queryKey: ['tasks', 'count', 'pending', pendingStatusId],
    queryFn: () =>
      api.get<ListResponse<Task>>(`/tasks${buildQuery({ task_status_id: pendingStatusId, page_size: 1 })}`),
    enabled: pendingStatusId !== undefined,
    refetchInterval: LIVE_REFRESH_MS,
    select: (resp) => resp.total,
  })
}

// ---- Tasks (global) ----

export interface TaskFilters {
  device_id?: number
  task_status_id?: number
  task_type?: string
  page?: number
  page_size?: number
}

export function useTasks(filters: TaskFilters) {
  return useQuery({
    queryKey: ['tasks', filters],
    queryFn: () => api.get<ListResponse<Task>>(`/tasks${buildQuery(filters)}`),
    refetchInterval: LIVE_REFRESH_MS,
    placeholderData: (prev) => prev,
  })
}

export interface CreateTaskInput {
  device_id: number
  task_type: string
  priority?: number
  parameters?: Record<string, unknown>
  max_retries?: number
}

export function useCreateTask() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: CreateTaskInput) => api.post<Task>('/tasks', input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['tasks'] }),
  })
}

export function useCancelTask() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => api.post<void>(`/tasks/${id}/cancel`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['tasks'] }),
  })
}

// ---- Catalog: Vendors, Device Models, Parameter Mappings ----

export function useCreateVendor() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: { code: string; name: string; description?: string }) =>
      api.post<Vendor>('/vendors', input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['vendors'] }),
  })
}

export function useVendorOUIs(vendorId: number | undefined) {
  return useQuery({
    queryKey: ['vendor-ouis', vendorId],
    queryFn: () => api.get<VendorOUI[]>(`/vendors/${vendorId}/ouis`),
    enabled: vendorId !== undefined,
  })
}

export function useAddVendorOUI() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ vendorId, oui, notes }: { vendorId: number; oui: string; notes?: string }) =>
      api.post<VendorOUI>(`/vendors/${vendorId}/ouis`, { oui, notes }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['vendor-ouis'] }),
  })
}

export function useDeviceModels(vendorId: number | undefined) {
  return useQuery({
    queryKey: ['device-models', vendorId],
    queryFn: () => api.get<DeviceModel[]>(`/device-models?vendor_id=${vendorId}`),
    enabled: vendorId !== undefined,
  })
}

export interface CreateDeviceModelInput {
  vendor_id: number
  device_type_id: number
  data_model_version_id: number
  product_class?: string
  model_name: string
  description?: string
}

export function useCreateDeviceModel() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: CreateDeviceModelInput) => api.post<DeviceModel>('/device-models', input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['device-models'] }),
  })
}

export function useParameterMappings(vendorId: number | undefined) {
  return useQuery({
    queryKey: ['parameter-mappings', vendorId],
    queryFn: () => api.get<VendorParameterMapping[]>(`/vendor-parameter-mappings?vendor_id=${vendorId}`),
    enabled: vendorId !== undefined,
  })
}

export interface UpsertMappingInput {
  vendor_id: number
  data_model_version_id: number
  device_model_id?: number
  // Pola SQL LIKE opsional thd devices.software_version (migrations/0010).
  software_version_pattern?: string
  logical_key: string
  tr069_path: string
  parameter_type_id?: number
  description?: string
}

export function useUpsertParameterMapping() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: UpsertMappingInput) => api.post<VendorParameterMapping>('/vendor-parameter-mappings', input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['parameter-mappings'] }),
  })
}

// ---- Provisioning Profiles ----

export function useProvisioningProfiles(tenantId?: number) {
  return useQuery({
    queryKey: ['provisioning-profiles', tenantId],
    queryFn: () =>
      api.get<ListResponse<ProvisioningProfile>>(`/provisioning-profiles${buildQuery({ tenant_id: tenantId, page_size: 200 })}`),
  })
}

export function useProvisioningProfile(id: number | undefined) {
  return useQuery({
    queryKey: ['provisioning-profile', id],
    queryFn: () =>
      api.get<{ profile: ProvisioningProfile; parameters: ProvisioningProfileParameter[] }>(`/provisioning-profiles/${id}`),
    enabled: id !== undefined,
  })
}

export interface ProfileFormInput {
  tenant_id?: number
  vendor_id?: number
  device_model_id?: number
  name: string
  description?: string
  is_default: boolean
  is_active?: boolean
  parameters: ProvisioningProfileParameter[]
}

export function useCreateProfile() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: ProfileFormInput) => api.post<ProvisioningProfile>('/provisioning-profiles', input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['provisioning-profiles'] }),
  })
}

export function useUpdateProfile() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, input }: { id: number; input: ProfileFormInput }) =>
      api.put<void>(`/provisioning-profiles/${id}`, input),
    onSuccess: (_data, { id }) => {
      qc.invalidateQueries({ queryKey: ['provisioning-profiles'] })
      qc.invalidateQueries({ queryKey: ['provisioning-profile', id] })
    },
  })
}

export function useDeleteProfile() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => api.del<void>(`/provisioning-profiles/${id}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['provisioning-profiles'] }),
  })
}

export function useApplyProfile() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ deviceId, profileId }: { deviceId: number; profileId: number }) =>
      api.post<Task>(`/devices/${deviceId}/apply-profile/${profileId}`),
    onSuccess: (_data, { deviceId }) => qc.invalidateQueries({ queryKey: ['device', deviceId, 'tasks'] }),
  })
}

// ---- Zero-Touch Rules ----

export function useZeroTouchRules(tenantId?: number) {
  return useQuery({
    queryKey: ['zero-touch-rules', tenantId],
    queryFn: () => api.get<ZeroTouchRule[]>(`/zero-touch-rules${buildQuery({ tenant_id: tenantId })}`),
  })
}

export interface ZTRuleFormInput {
  tenant_id?: number
  vendor_id?: number
  device_model_id?: number
  oui?: string
  serial_pattern?: string
  software_version_pattern?: string
  // Wajib berpasangan (backend menegakkan CHECK constraint).
  match_parameter_name?: string
  match_parameter_value_pattern?: string
  // Opsional sejak migrations/0009.
  provisioning_profile_id?: number
  post_apply_reboot?: boolean
  firmware_file_id?: number
  // Wajib — FK ref_ztp_trigger_event (id via useRefs('ref_ztp_trigger_event')).
  trigger_event_id: number
  priority: number
  is_active?: boolean
}

export function useCreateZTRule() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: ZTRuleFormInput) => api.post<ZeroTouchRule>('/zero-touch-rules', input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['zero-touch-rules'] }),
  })
}

export function useUpdateZTRule() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, input }: { id: number; input: ZTRuleFormInput }) =>
      api.put<void>(`/zero-touch-rules/${id}`, input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['zero-touch-rules'] }),
  })
}

export function useDeleteZTRule() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => api.del<void>(`/zero-touch-rules/${id}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['zero-touch-rules'] }),
  })
}

// ---- Firmware ----

export function useFirmwareList(vendorId: number | undefined) {
  return useQuery({
    queryKey: ['firmware', vendorId],
    queryFn: () => api.get<ListResponse<FirmwareFile>>(`/firmware?vendor_id=${vendorId}&page_size=200`),
    enabled: vendorId !== undefined,
  })
}

// UploadFirmwareInput: file firmware sungguhan (bukan lagi path string bebas)
// — backend menghitung checksum SHA-256 sendiri dari isi file yang diterima,
// bukan dipercaya dari client (FR-19).
export interface UploadFirmwareInput {
  vendor_id: number
  device_model_id?: number
  version: string
  release_notes?: string
  file: File
}

export function useUploadFirmware() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: UploadFirmwareInput) => {
      const form = new FormData()
      form.set('vendor_id', String(input.vendor_id))
      if (input.device_model_id !== undefined) form.set('device_model_id', String(input.device_model_id))
      form.set('version', input.version)
      if (input.release_notes) form.set('release_notes', input.release_notes)
      form.set('file', input.file)
      return api.postForm<FirmwareFile>('/firmware', form)
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['firmware'] }),
  })
}

export function useFirmwareJobs(deviceId: number) {
  return useQuery({
    queryKey: ['device', deviceId, 'firmware-jobs'],
    queryFn: () => api.get<ListResponse<FirmwareUpgradeJob>>(`/devices/${deviceId}/firmware-jobs?page_size=50`),
    refetchInterval: LIVE_REFRESH_MS,
  })
}

export function useScheduleFirmwareUpgrade() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ deviceId, firmwareId }: { deviceId: number; firmwareId: number }) =>
      api.post<FirmwareUpgradeJob>(`/devices/${deviceId}/firmware-upgrade`, { firmware_id: firmwareId }),
    onSuccess: (_data, { deviceId }) => qc.invalidateQueries({ queryKey: ['device', deviceId, 'firmware-jobs'] }),
  })
}

// ---- Firmware Rollout Batch (canary/staged rollout, migrations/0011) ----

export function useFirmwareRolloutBatches(tenantId?: number) {
  return useQuery({
    queryKey: ['firmware-rollout-batches', tenantId],
    queryFn: () =>
      api.get<ListResponse<FirmwareRolloutBatch>>(
        `/firmware/rollout-batches${buildQuery({ tenant_id: tenantId, page_size: 200 })}`,
      ),
    refetchInterval: LIVE_REFRESH_MS,
  })
}

// ---- Config Snapshots ----

export interface DeviceConfigSnapshot {
  id: number
  device_id: number
  snapshot_data: Record<string, string>
  created_at: string
}

export function useConfigSnapshots(deviceId: number, page: number = 1, pageSize: number = 50) {
  return useQuery({
    queryKey: ['device', deviceId, 'config-snapshots', page, pageSize],
    queryFn: () => api.get<ListResponse<DeviceConfigSnapshot>>(`/devices/${deviceId}/config-snapshots?page=${page}&page_size=${pageSize}`),
  })
}

export function useCreateConfigSnapshot() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (deviceId: number) => api.post<void>(`/devices/${deviceId}/config-snapshots`),
    onSuccess: (_data, deviceId) => qc.invalidateQueries({ queryKey: ['device', deviceId, 'config-snapshots'] }),
  })
}

export interface CreateRolloutBatchInput {
  tenant_id?: number
  firmware_file_id: number
  vendor_id?: number
  device_model_id?: number
  wave_percentage: number
  max_failure_rate_percent: number
  notes?: string
  scheduled_at?: string
}

export function useCreateRolloutBatch() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: CreateRolloutBatchInput) =>
      api.post<FirmwareRolloutBatch>('/firmware/rollout-batches', input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['firmware-rollout-batches'] }),
  })
}

export function useAdvanceRolloutBatch() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => api.post<FirmwareRolloutBatch>(`/firmware/rollout-batches/${id}/advance`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['firmware-rollout-batches'] }),
  })
}

export function useCancelRolloutBatch() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => api.post<void>(`/firmware/rollout-batches/${id}/cancel`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['firmware-rollout-batches'] }),
  })
}

// ---- Diagnostics ----

export function useDeviceDiagnostics(deviceId: number) {
  return useQuery({
    queryKey: ['device', deviceId, 'diagnostics'],
    queryFn: () => api.get<ListResponse<DeviceDiagnostic>>(`/devices/${deviceId}/diagnostics?page_size=50`),
    refetchInterval: LIVE_REFRESH_MS,
  })
}

export function useTriggerDiagnostic() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ deviceId, diagnosticType, parameters }: { deviceId: number; diagnosticType: string; parameters?: Record<string, string> }) =>
      api.post<DeviceDiagnostic>(`/devices/${deviceId}/diagnostics`, { diagnostic_type: diagnosticType, parameters }),
    onSuccess: (_data, { deviceId }) => qc.invalidateQueries({ queryKey: ['device', deviceId, 'diagnostics'] }),
  })
}

// ---- Administration: Tenants & Users ----

export function useTenants() {
  return useQuery({
    queryKey: ['tenants'],
    queryFn: () => api.get<ListResponse<Tenant>>('/tenants?page_size=200'),
  })
}

export interface CreateTenantInput {
  code: string
  name: string
  cwmp_inform_username?: string
  cwmp_inform_password?: string
}

export function useCreateTenant() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: CreateTenantInput) => api.post<Tenant>('/tenants', input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['tenants'] }),
  })
}

export function useSetTenantCWMPCredentials() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ tenantId, username, password }: { tenantId: number; username: string; password: string }) =>
      api.patch<void>(`/tenants/${tenantId}/cwmp-credentials`, { username, password }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['tenants'] }),
  })
}

// useCurrentTenant — dipanggil semua role (bukan cuma superadmin) utk
// white-labeling (ROADMAP.md Fase 2). undefined = actor tanpa tenant
// (superadmin global) -> Layout fallback ke branding default.
export function useCurrentTenant() {
  return useQuery({
    queryKey: ['tenants', 'current'],
    queryFn: () => api.get<CurrentTenant | undefined>('/tenants/current'),
    staleTime: 5 * 60 * 1000,
  })
}

export interface UpdateTenantBrandingInput {
  brand_name?: string | null
  logo_url?: string | null
  primary_color?: string | null
}

export function useUpdateTenantBranding() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ tenantId, input }: { tenantId: number; input: UpdateTenantBrandingInput }) =>
      api.patch<void>(`/tenants/${tenantId}/branding`, input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['tenants'] })
    },
  })
}

// useSetTenantTaskQuota — kebijakan platform-level (superadmin only, beda
// dari branding yang self-service tenant, lihat ROADMAP.md Fase 2). null
// berarti tidak dibatasi.
export function useSetTenantTaskQuota() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ tenantId, maxPendingTasks }: { tenantId: number; maxPendingTasks: number | null }) =>
      api.patch<void>(`/tenants/${tenantId}/task-quota`, { max_pending_tasks: maxPendingTasks }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['tenants'] }),
  })
}

export function useUsers(tenantId?: number) {
  return useQuery({
    queryKey: ['users', tenantId],
    queryFn: () => api.get<ListResponse<User>>(`/users${buildQuery({ tenant_id: tenantId, page_size: 200 })}`),
  })
}

export interface CreateUserInput {
  tenant_id?: number
  username: string
  email: string
  password: string
  full_name: string
  role_codes: string[]
}

export function useCreateUser() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: CreateUserInput) => api.post<User>('/users', input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['users'] }),
  })
}

export interface UpdateUserInput {
  full_name?: string
  email?: string
  is_active?: boolean
}

// useUpdateUser — partial update (PATCH /users/:id), field yang tidak
// disertakan di body tidak diubah backend. RBAC & self-lockout guard
// (mis. ADMIN tidak bisa nonaktifkan akun sendiri) ditegakkan backend —
// tampilkan pesan error 403-nya apa adanya di komponen, jangan diduplikasi.
export function useUpdateUser() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ userId, input }: { userId: number; input: UpdateUserInput }) =>
      api.patch<User>(`/users/${userId}`, input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['users'] }),
  })
}

// useResetUserPassword — admin mereset password user lain (PATCH
// /users/:id/password). Tidak mengubah data yang tampil di tabel users,
// jadi tidak perlu invalidate query.
export function useResetUserPassword() {
  return useMutation({
    mutationFn: ({ userId, newPassword }: { userId: number; newPassword: string }) =>
      api.patch<void>(`/users/${userId}/password`, { new_password: newPassword }),
  })
}

// useReplaceUserRoles — full-replace role (bukan tambah/hapus satu-satu),
// body role_codes menggantikan seluruh set role user tsb. Backend menolak
// assign SUPERADMIN oleh non-superadmin, dan ADMIN mencabut role ADMIN
// dari akun sendiri (self-lockout) — error 403-nya ditampilkan apa adanya.
export function useReplaceUserRoles() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ userId, roleCodes }: { userId: number; roleCodes: string[] }) =>
      api.patch<User>(`/users/${userId}/roles`, { role_codes: roleCodes }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['users'] }),
  })
}

// useDeleteUser — soft-delete (DELETE /users/:id). Backend menolak ADMIN
// menghapus akun sendiri (self-lockout guard).
export function useDeleteUser() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (userId: number) => api.del<void>(`/users/${userId}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['users'] }),
  })
}

// ---- Connection Request (trigger manual CPE Inform) ----

export function useTriggerConnectionRequest() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (deviceId: number) => api.post<{ message: string }>(`/devices/${deviceId}/connection-request`, {}),
    onSuccess: (_data, deviceId) => qc.invalidateQueries({ queryKey: ['device', deviceId] }),
  })
}

export function useRebootDevice() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (deviceId: number) => api.post<{ message: string }>(`/devices/${deviceId}/reboot`, {}),
    onSuccess: (_data, deviceId) => qc.invalidateQueries({ queryKey: ['device', deviceId] }),
  })
}

export function useFactoryResetDevice() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (deviceId: number) => api.post<{ message: string }>(`/devices/${deviceId}/factory-reset`, {}),
    onSuccess: (_data, deviceId) => qc.invalidateQueries({ queryKey: ['device', deviceId] }),
  })
}

export function usePushFileToDevice() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ deviceId, fileId }: { deviceId: number; fileId: number }) =>
      api.post<{ message: string }>(`/devices/${deviceId}/push-file`, { file_id: fileId }),
    onSuccess: (_data, { deviceId }) => qc.invalidateQueries({ queryKey: ['device', deviceId] }),
  })
}

// ---- Webhooks (migrations/0013) ----

export interface WebhookSubscription {
  id: number
  subscription_uuid: string
  tenant_id: number | null
  event_type_id: number
  name: string
  target_url: string
  is_active: boolean
  description: string | null
  created_at: string
  updated_at: string
  // secret: hanya ada satu kali di response create
  secret?: string
}

export interface WebhookDelivery {
  id: number
  delivery_uuid: string
  subscription_id: number
  event_type_id: number
  payload: unknown
  status: string
  attempt_count: number
  max_attempts: number
  response_status: number | null
  error_message: string | null
  next_attempt_at: string | null
  delivered_at: string | null
  created_at: string
  updated_at: string
}

export function useWebhooks(tenantId?: number) {
  return useQuery({
    queryKey: ['webhooks', tenantId],
    queryFn: () =>
      api.get<ListResponse<WebhookSubscription>>(
        `/webhooks${buildQuery({ tenant_id: tenantId, page_size: 100 })}`,
      ),
    refetchInterval: 30_000,
  })
}

export interface CreateWebhookInput {
  tenant_id?: number
  event_type: string
  name: string
  target_url: string
  description?: string
}

export function useCreateWebhook() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: CreateWebhookInput) => api.post<WebhookSubscription>('/webhooks', input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['webhooks'] }),
  })
}

export interface UpdateWebhookInput {
  name: string
  target_url: string
  is_active: boolean
  description?: string
}

export function useUpdateWebhook() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, input }: { id: number; input: UpdateWebhookInput }) =>
      api.patch<void>(`/webhooks/${id}`, { ...input }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['webhooks'] }),
  })
}

export function useDeleteWebhook() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => api.del<void>(`/webhooks/${id}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['webhooks'] }),
  })
}

export function useWebhookDeliveries(subscriptionId: number | undefined) {
  return useQuery({
    queryKey: ['webhook-deliveries', subscriptionId],
    queryFn: () =>
      api.get<ListResponse<WebhookDelivery>>(`/webhooks/${subscriptionId}/deliveries?page_size=50`),
    enabled: subscriptionId !== undefined,
    refetchInterval: 15_000,
  })
}

export function useTestWebhook() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => api.post<WebhookDelivery>(`/webhooks/${id}/test`, {}),
    onSuccess: (_data, id) => qc.invalidateQueries({ queryKey: ['webhook-deliveries', id] }),
  })
}


export function useFiles(params: { page?: number; pageSize?: number } = {}) {
  return useQuery({
    queryKey: ['files', params],
    queryFn: () => api.get<ListResponse<GenericFile>>('/files' + buildQuery(params)),
  })
}

export function useUploadFile() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (formData: FormData) => api.postForm<GenericFile>('/files', formData),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['files'] }),
  })
}

export function useDeleteFile() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => api.del<void>('/files/' + id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['files'] }),
  })
}

// ---- Tags ----
export function useTags(params: { page?: number; pageSize?: number } = {}) {
  return useQuery({
    queryKey: ['tags', params],
    queryFn: () => api.get<ListResponse<Tag>>('/tags' + buildQuery(params)),
  })
}

export function useCreateTag() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: Partial<Tag>) => api.post<Tag>('/tags', data),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['tags'] }),
  })
}

export function useDeleteTag() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => api.del<void>('/tags/' + id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['tags'] }),
  })
}

// ---- Presets ----
export function usePresets(params: { page?: number; pageSize?: number } = {}) {
  return useQuery({
    queryKey: ['presets', params],
    queryFn: () => api.get<ListResponse<Preset>>('/presets' + buildQuery(params)),
  })
}

export function useCreatePreset() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: Partial<Preset>) => api.post<Preset>('/presets', data),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['presets'] }),
  })
}

export function useUpdatePreset() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, data }: { id: number; data: Partial<Preset> }) => api.patch<Preset>('/presets/' + id, data),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['presets'] }),
  })
}

export function useDeletePreset() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => api.del<void>('/presets/' + id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['presets'] }),
  })
}

// ---- Advanced TR-069 RPCs (FR-5) ----

export function useAddObject() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ deviceId, objectName }: { deviceId: number; objectName: string }) =>
      api.post<void>(`/devices/${deviceId}/tasks/add-object`, { object_name: objectName }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['tasks'] }),
  })
}

export function useDeleteObject() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ deviceId, objectName }: { deviceId: number; objectName: string }) =>
      api.post<void>(`/devices/${deviceId}/tasks/delete-object`, { object_name: objectName }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['tasks'] }),
  })
}

export function useGetParameterNames() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ deviceId, path, nextLevel }: { deviceId: number; path: string; nextLevel: boolean }) =>
      api.post<void>(`/devices/${deviceId}/tasks/get-parameter-names`, { path, next_level: nextLevel }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['tasks'] }),
  })
}

export function useGetParameterValues() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ deviceId, names }: { deviceId: number; names: string[] }) =>
      api.post<void>(`/devices/${deviceId}/tasks/get-parameter-values`, { names }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['tasks'] }),
  })
}

export function useSetParameterValues() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ deviceId, values }: { deviceId: number; values: Record<string, string> }) =>
      api.post<void>(`/devices/${deviceId}/tasks/set-parameter-values`, { values }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['tasks'] }),
  })
}
