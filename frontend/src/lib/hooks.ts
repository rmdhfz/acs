import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from './api'
import type {
  ActivityLog,
  Device,
  DeviceDiagnostic,
  DeviceEvent,
  DeviceModel,
  DeviceOpticalMetric,
  DeviceParameter,
  DeviceStats,
  FirmwareFile,
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

const LIVE_REFRESH_MS = 5000

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
  provisioning_profile_id: number
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

export interface UploadFirmwareInput {
  vendor_id: number
  device_model_id?: number
  version: string
  file_name: string
  file_path: string
  file_size_bytes?: number
  checksum_sha256?: string
  release_notes?: string
}

export function useUploadFirmware() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: UploadFirmwareInput) => api.post<FirmwareFile>('/firmware', input),
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
