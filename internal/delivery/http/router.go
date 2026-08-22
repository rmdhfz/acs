// Package http adalah delivery layer REST (Echo) untuk konsumsi internal/BSS
// (TECH.md §2). Handler hanya parsing/validasi request lalu memanggil
// usecase — logic bisnis ada di internal/usecase (CLAUDE.md).
package http

import (
	"github.com/labstack/echo/v5"

	"acs/internal/domain"
	"acs/internal/usecase/auth"
	"acs/internal/usecase/device"
	"acs/internal/usecase/diagnostics"
	"acs/internal/usecase/firmware"
	"acs/internal/usecase/iam"
	"acs/internal/usecase/provisioning"
	"acs/internal/usecase/task"
)

type Router struct {
	Auth          *auth.Service
	IAM           *iam.Service
	Devices       *device.Service
	Tasks         *task.Service
	Provisioning  *provisioning.Service
	Firmware      *firmware.Service
	Diagnostics   *diagnostics.Service
	Vendors       domain.VendorRepository
	VendorOUIs    domain.VendorOUIRepository
	DeviceModels  domain.DeviceModelRepository
	ParamMappings domain.VendorParameterMappingRepository
	Refs          domain.RefRepository
}

func (r *Router) Register(e *echo.Echo) {
	api := e.Group("/api/v1")

	api.POST("/auth/login", r.login)

	authed := api.Group("", AuthMiddleware(r.Auth))
	admin := []string{domain.RoleAdmin, domain.RoleSuperadmin}
	adminOrNOC := []string{domain.RoleAdmin, domain.RoleNOC, domain.RoleSuperadmin}
	superadminOnly := []string{domain.RoleSuperadmin}

	authed.POST("/auth/tokens", r.issueAPIToken, RequireRoles(admin...))

	authed.GET("/refs/:table", r.listRefs)

	authed.POST("/tenants", r.createTenant, RequireRoles(superadminOnly...))
	authed.GET("/tenants", r.listTenants, RequireRoles(superadminOnly...))
	authed.PATCH("/tenants/:id/cwmp-credentials", r.setTenantCWMPCredentials, RequireRoles(superadminOnly...))
	authed.POST("/users", r.createUser, RequireRoles(admin...))
	authed.GET("/users", r.listUsers, RequireRoles(admin...))

	authed.GET("/devices", r.listDevices)
	authed.GET("/devices/stats", r.deviceStats)
	authed.GET("/devices/:id", r.getDevice)
	authed.PATCH("/devices/:id", r.updateDevice, RequireRoles(adminOrNOC...))
	authed.GET("/devices/:id/parameters", r.listDeviceParameters)
	authed.GET("/devices/:id/events", r.listDeviceEvents)
	authed.GET("/devices/:id/optical-metrics", r.listOpticalMetrics)

	authed.GET("/tasks", r.listTasks)
	authed.GET("/tasks/stats", r.taskStats)
	authed.GET("/tasks/:id", r.getTask)
	authed.POST("/tasks", r.createTask, RequireRoles(adminOrNOC...))
	authed.POST("/tasks/:id/cancel", r.cancelTask, RequireRoles(adminOrNOC...))

	authed.GET("/provisioning-profiles", r.listProfiles)
	authed.GET("/provisioning-profiles/:id", r.getProfile)
	authed.POST("/provisioning-profiles", r.createProfile, RequireRoles(admin...))
	authed.PUT("/provisioning-profiles/:id", r.updateProfile, RequireRoles(admin...))
	authed.DELETE("/provisioning-profiles/:id", r.deleteProfile, RequireRoles(admin...))
	authed.POST("/devices/:id/apply-profile/:profileId", r.applyProfile, RequireRoles(admin...))

	authed.GET("/zero-touch-rules", r.listZTRules)
	authed.POST("/zero-touch-rules", r.createZTRule, RequireRoles(admin...))
	authed.PUT("/zero-touch-rules/:id", r.updateZTRule, RequireRoles(admin...))
	authed.DELETE("/zero-touch-rules/:id", r.deleteZTRule, RequireRoles(admin...))

	authed.GET("/vendors", r.listVendors)
	authed.POST("/vendors", r.createVendor, RequireRoles(superadminOnly...))
	authed.POST("/vendors/:id/ouis", r.addVendorOUI, RequireRoles(superadminOnly...))
	authed.GET("/device-models", r.listDeviceModels)
	authed.POST("/device-models", r.createDeviceModel, RequireRoles(superadminOnly...))
	authed.POST("/vendor-parameter-mappings", r.upsertParameterMapping, RequireRoles(superadminOnly...))

	authed.POST("/firmware", r.uploadFirmware, RequireRoles(admin...))
	authed.GET("/firmware", r.listFirmware)
	authed.POST("/devices/:id/firmware-upgrade", r.scheduleFirmwareUpgrade, RequireRoles(admin...))
	authed.GET("/devices/:id/firmware-jobs", r.listFirmwareJobs)

	authed.POST("/devices/:id/diagnostics", r.triggerDiagnostic, RequireRoles(adminOrNOC...))
	authed.GET("/devices/:id/diagnostics", r.listDiagnostics)
}
