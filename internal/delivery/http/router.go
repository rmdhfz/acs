// Package http adalah delivery layer REST (Echo) untuk konsumsi internal/BSS
// (TECH.md §2). Handler hanya parsing/validasi request lalu memanggil
// usecase — logic bisnis ada di internal/usecase (CLAUDE.md).
package http

import (
	"net/http"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"

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
	// Activity — dipakai langsung dari handler catalog (bukan lewat usecase)
	// khusus utk mutasi data vendor/model/mapping global, supaya ada jejak
	// audit siapa mengubah data referensi lintas-tenant ini (temuan
	// acs-security-reviewer, ROADMAP.md Fase 0 audit menyeluruh).
	Activity domain.ActivityLogRepository
	// MetricsHandler membungkus promhttp.HandlerFor(registry, ...) yang
	// dikonstruksi cmd/acsd/main.go (internal/metrics.Collector) — router.go
	// sengaja tidak import paket prometheus langsung, cukup meneruskan
	// http.Handler generik (lihat metrics_handler.go, TECH.md §10).
	MetricsHandler http.Handler
}

func (r *Router) Register(e *echo.Echo) {
	api := e.Group("/api/v1")

	// Rate limit KHUSUS lebih ketat dari default REST 30 req/s (cmd/acsd/
	// main.go) -- /auth/login adalah target brute-force paling jelas di
	// seluruh REST API, dan proteksi lockout per-akun (auth.Service.Login)
	// sendiri py celah TOCTOU thd request PARALEL (lihat komentar
	// RecordFailedLogin) -- rate limit per-IP di sini membatasi throughput
	// serangan secara independen sbg lapisan pertahanan kedua, bukan
	// pengganti fix TOCTOU-nya (temuan acs-security-reviewer).
	api.POST("/auth/login", r.login, middleware.RateLimiter(middleware.NewRateLimiterMemoryStore(2)))
	// GET /metrics: TIDAK diautentikasi (konvensi Prometheus exporter),
	// endpoint read-only agregat lintas-tenant untuk operator platform — lihat
	// komentar lengkap di metrics_handler.go & router.go field MetricsHandler
	// soal kenapa ini pengecualian yang aman terhadap aturan "tidak ada
	// endpoint tanpa autentikasi" (CLAUDE.md, yang menyasar endpoint mutasi).
	api.GET("/metrics", r.metrics)

	authed := api.Group("", AuthMiddleware(r.Auth))
	admin := []string{domain.RoleAdmin, domain.RoleSuperadmin}
	adminOrNOC := []string{domain.RoleAdmin, domain.RoleNOC, domain.RoleSuperadmin}
	superadminOnly := []string{domain.RoleSuperadmin}

	authed.POST("/auth/tokens", r.issueAPIToken, RequireRoles(admin...))
	// GET/DELETE gated sama seperti penerbitannya (admin...) -- token API
	// hanya bisa dikelola oleh role yang juga bisa menerbitkannya, bukan
	// self-service ke semua role (beda dari PATCH /auth/password di bawah).
	authed.GET("/auth/tokens", r.listAPITokens, RequireRoles(admin...))
	authed.DELETE("/auth/tokens/:id", r.revokeAPIToken, RequireRoles(admin...))
	// Self-service ganti password sendiri (BEDA dari admin-reset
	// PATCH /users/:id/password di bawah) -- actor dari JWT langsung, bukan
	// target :id, jadi semua role yang sudah login boleh (tidak ada
	// RequireRoles di sini, sengaja).
	authed.PATCH("/auth/password", r.changeOwnPassword)

	authed.GET("/refs/:table", r.listRefs)

	authed.POST("/tenants", r.createTenant, RequireRoles(superadminOnly...))
	authed.GET("/tenants", r.listTenants, RequireRoles(superadminOnly...))
	authed.GET("/tenants/current", r.getCurrentTenant)
	// Activate/deactivate tenant (ROADMAP.md gap lama) -- superadmin only,
	// sama seperti create tenant. Dampak IsActive=false ke Login/ResolveActor
	// didokumentasikan di usecase/iam.UpdateTenant.
	authed.PATCH("/tenants/:id", r.updateTenant, RequireRoles(superadminOnly...))
	authed.PATCH("/tenants/:id/cwmp-credentials", r.setTenantCWMPCredentials, RequireRoles(superadminOnly...))
	// Branding: superadmin utk tenant manapun, ADMIN utk tenant sendiri saja
	// (dicek di usecase/iam) — role gate di sini cuma menyaring NOC/VIEWER.
	authed.PATCH("/tenants/:id/branding", r.updateTenantBranding, RequireRoles(admin...))
	// Kuota task queue: kebijakan platform-level, superadmin only (bukan
	// self-service tenant seperti branding di atas) — lihat usecase/iam.SetTaskQuota.
	authed.PATCH("/tenants/:id/task-quota", r.setTenantTaskQuota, RequireRoles(superadminOnly...))
	authed.POST("/users", r.createUser, RequireRoles(admin...))
	authed.GET("/users", r.listUsers, RequireRoles(admin...))
	// Self-service tenant admin (ROADMAP.md Fase 2): superadmin bisa ke user
	// manapun, ADMIN dibatasi ke user satu tenant + guard self-lockout —
	// keduanya dicek di usecase/iam, role gate di sini cuma menyaring NOC/VIEWER.
	authed.PATCH("/users/:id", r.updateUser, RequireRoles(admin...))
	authed.PATCH("/users/:id/password", r.resetUserPassword, RequireRoles(admin...))
	authed.PATCH("/users/:id/roles", r.replaceUserRoles, RequireRoles(admin...))
	authed.DELETE("/users/:id", r.deleteUser, RequireRoles(admin...))

	authed.GET("/devices", r.listDevices)
	authed.GET("/devices/stats", r.deviceStats)
	authed.GET("/devices/:id", r.getDevice)
	authed.PATCH("/devices/:id", r.updateDevice, RequireRoles(adminOrNOC...))
	authed.GET("/devices/:id/parameters", r.listDeviceParameters)
	authed.GET("/devices/:id/events", r.listDeviceEvents)
	authed.GET("/devices/:id/optical-metrics", r.listOpticalMetrics)
	authed.GET("/devices/:id/activity", r.listDeviceActivity)

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
	authed.GET("/vendors/:id/ouis", r.listVendorOUIs)
	authed.POST("/vendors/:id/ouis", r.addVendorOUI, RequireRoles(superadminOnly...))
	authed.GET("/device-models", r.listDeviceModels)
	authed.POST("/device-models", r.createDeviceModel, RequireRoles(superadminOnly...))
	authed.POST("/vendor-parameter-mappings", r.upsertParameterMapping, RequireRoles(superadminOnly...))

	// firmware_files adalah katalog global lintas tenant (tidak py tenant_id,
	// sama seperti vendors/device-models/vendor-parameter-mappings) — upload
	// dibatasi superadmin, konsisten dgn data referensi global lain (temuan
	// audit isolasi tenant Fase 2: sebelumnya admin tenant mana pun bisa
	// upload firmware yang lalu dipakai tenant lain via device-models/vendor
	// yang sama, celah supply-chain/integrity lintas tenant).
	authed.POST("/firmware", r.uploadFirmware, RequireRoles(superadminOnly...))
	authed.GET("/firmware", r.listFirmware)
	authed.POST("/devices/:id/firmware-upgrade", r.scheduleFirmwareUpgrade, RequireRoles(admin...))
	authed.GET("/devices/:id/firmware-jobs", r.listFirmwareJobs)

	authed.POST("/devices/:id/diagnostics", r.triggerDiagnostic, RequireRoles(adminOrNOC...))
	authed.GET("/devices/:id/diagnostics", r.listDiagnostics)
}
