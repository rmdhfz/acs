// Package http adalah delivery layer REST (Echo) untuk konsumsi internal/BSS
// (TECH.md §2). Handler hanya parsing/validasi request lalu memanggil
// usecase — logic bisnis ada di internal/usecase (CLAUDE.md).
package http

import (
	"net/http"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"

	"acs/internal/delivery/ws"
	"acs/internal/domain"
	"acs/internal/usecase/auth"
	"acs/internal/usecase/automation"
	"acs/internal/usecase/billing"
	"acs/internal/usecase/capture"
	"acs/internal/usecase/catalog"
	"acs/internal/usecase/console"
	"acs/internal/usecase/device"
	"acs/internal/usecase/diagnostics"
	"acs/internal/usecase/fault"
	"acs/internal/usecase/file"
	"acs/internal/usecase/firmware"
	"acs/internal/usecase/iam"
	"acs/internal/usecase/notification"
	"acs/internal/usecase/preset"
	"acs/internal/usecase/provisioning"
	"acs/internal/usecase/report"
	"acs/internal/usecase/selfservice"
	"acs/internal/usecase/session"
	"acs/internal/usecase/signup"
	"acs/internal/usecase/tag"
	"acs/internal/usecase/task"
	"acs/internal/usecase/vparam"
	"acs/internal/usecase/webhook"
)

type Router struct {
	Auth          *auth.Service
	IAM           *iam.Service
	Devices       *device.Service
	Tasks         *task.Service
	Provisioning  *provisioning.Service
	Firmware      *firmware.Service
	Diagnostics   *diagnostics.Service
	Reports       *report.Service
	Signup        *signup.Service // nil/mati = pendaftaran tenant mandiri nonaktif
	Webhooks      *webhook.Service
	Sessions      *session.Service
	Catalog       *catalog.Service
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
	Files          *file.Service
	Tags           *tag.Service
	Presets        *preset.Service
	SelfService    *selfservice.Service
	Notifications  *notification.Service
	// Billing — langganan SaaS lewat Xendit (nil = rute billing tidak dipasang; bila ada tetapi dimatikan lewat
	// konfigurasi, endpoint mutasi menjawab 409 dan webhook 404).
	Billing *billing.Service
	// Fitur Console (migrations/0033): layout menu per pengguna, laporan bug, riwayat impor/ekspor (nil = rute tak dipasang).
	Menu      *console.MenuService
	Bugs      *console.BugService
	Transfers *console.TransferService
	// Capture sesi CWMP per perangkat (migrations/0037); nil = rute tidak dipasang.
	Capture *capture.Service
	// Resource faults (migrations/0038): cwmp:Fault aktif dari CPE; nil = rute tidak dipasang.
	Faults *fault.Service
	// Virtual parameter (migrations/0039): ekspresi turunan per tenant; nil = rute tidak dipasang.
	VParams *vparam.Service
	// Aturan otomasi kondisi-aksi (migrations/0040); nil = rute tidak dipasang.
	Automation *automation.Service
	WSHub      *ws.Hub
	// WSAllowedOrigins = origin Console yang boleh membuka WebSocket (sama
	// dengan ACS_CORS_ALLOW_ORIGINS).
	WSAllowedOrigins []string
	// PublicURL = origin Console (tujuan redirect login SSO), tanpa slash akhir.
	PublicURL string
	// Readiness = dependensi yang diperiksa GET /readyz (lihat health_handler.go).
	Readiness []ReadinessCheck
	// LimiterStore membuat store rate limit untuk satu rute (prefix unik per rute). Nil = in-memory
	// per instance; di deployment multi-instance diisi store Redis agar batas berlaku lintas instance.
	LimiterStore func(prefix string, ratePerSec float64) middleware.RateLimiterStore
}

func (r *Router) limiter(prefix string, ratePerSec float64) echo.MiddlewareFunc {
	if r.LimiterStore != nil {
		return middleware.RateLimiter(r.LimiterStore(prefix, ratePerSec))
	}
	return middleware.RateLimiter(middleware.NewRateLimiterMemoryStore(ratePerSec))
}

func (r *Router) Register(e *echo.Echo) {
	// Probe orkestrator/load balancer: publik, read-only, tanpa data pengguna (di luar /api/v1).
	e.GET("/healthz", r.healthz)
	e.GET("/readyz", r.readyz)

	// QueryGuard: page/page_size/*_id yang rusak ditolak 400 (bukan diam-diam jadi "tanpa filter").
	api := e.Group("/api/v1", QueryGuard)

	// Rate limit KHUSUS lebih ketat dari default REST 30 req/s (cmd/acsd/
	// main.go) -- /auth/login adalah target brute-force paling jelas di
	// seluruh REST API, dan proteksi lockout per-akun (auth.Service.Login)
	// sendiri py celah TOCTOU thd request PARALEL (lihat komentar
	// RecordFailedLogin) -- rate limit per-IP di sini membatasi throughput
	// serangan secara independen sbg lapisan pertahanan kedua, bukan
	// pengganti fix TOCTOU-nya (temuan acs-security-reviewer).
	api.POST("/auth/login", r.login, r.limiter("login", 2))

	// Reset password lewat email — PUBLIK secara sengaja (pemanggil tidak bisa
	// login), kontrol: rate limit per-IP lebih ketat dari login, respons seragam
	// (anti-enumerasi), token sekali-pakai berumur pendek. Lihat auth_handler.go.
	api.POST("/auth/forgot-password", r.forgotPassword, r.limiter("forgot", 0.2))
	api.POST("/auth/reset-password", r.resetPassword, r.limiter("reset", 0.5))

	// Tahap kedua login MFA: publik tetapi dibatasi laju & lockout akun yang sama dengan login.
	api.POST("/auth/mfa/verify", r.mfaVerify, r.limiter("mfa-verify", 2))
	// Pendaftaran tenant mandiri (bawaan mati): publik tetapi dibatasi laju ketat; tenant dibuat hanya setelah email diverifikasi.
	api.POST("/auth/signup", r.signup, r.limiter("signup", 0.1))
	api.POST("/auth/signup/verify", r.verifySignup, r.limiter("signup-verify", 0.5))
	// Callback Xendit (billing): PUBLIK secara sengaja -- Xendit tidak membawa JWT. Kontrol: header X-CALLBACK-TOKEN
	// (waktu-konstan), verifikasi ulang status/nominal ke gateway, body maks 64 KiB, pembatas laju sendiri yang
	// longgar (20/dtk per IP agar callback sah tidak terblokir). Billing mati -> 404. Lihat billing_handler.go.
	if r.Billing != nil {
		api.POST("/billing/xendit/webhook", r.xenditWebhook, r.limiter("billing-webhook", 20))
	}
	api.GET("/auth/config", r.authConfig)
	api.GET("/auth/oidc/login", r.oidcLogin)
	api.GET("/auth/oidc/callback", r.oidcCallback)

	// GET /metrics: TIDAK diautentikasi (konvensi Prometheus exporter),
	// endpoint read-only agregat lintas-tenant untuk operator platform — lihat
	// komentar lengkap di metrics_handler.go & router.go field MetricsHandler
	// soal kenapa ini pengecualian yang aman terhadap aturan "tidak ada
	// endpoint tanpa autentikasi" (CLAUDE.md, yang menyasar endpoint mutasi).
	api.GET("/metrics", r.metrics)

	// Grup "authed" = staff internal (BSS/NOC/portal operator). Middleware
	// staffOnly menolak principal yang HANYA punya role ENDUSER: portal
	// pelanggan dikurung ke /self-service/* (grup terpisah di
	// mountSelfServiceRoutes) supaya token ENDUSER tidak bisa memanggil
	// GET /devices, /tasks, dll. dan membaca data seluruh tenant.
	// Pengecualian: PATCH /auth/password (ganti kata sandi sendiri) — di-mount
	// di grup authedSelf di bawah tanpa gate staffOnly.
	authedSelf := api.Group("", AuthMiddleware(r.Auth))
	authed := api.Group("", AuthMiddleware(r.Auth), staffOnly)
	admin := []string{domain.RoleAdmin, domain.RoleSuperadmin}
	adminOrNOC := []string{domain.RoleAdmin, domain.RoleNOC, domain.RoleSuperadmin}
	superadminOnly := []string{domain.RoleSuperadmin}

	authed.GET("/ws", r.serveWs)

	authed.POST("/auth/tokens", r.issueAPIToken, RequireRoles(admin...))
	// GET/DELETE gated sama seperti penerbitannya (admin...) -- token API
	// hanya bisa dikelola oleh role yang juga bisa menerbitkannya, bukan
	// self-service ke semua role (beda dari PATCH /auth/password di bawah).
	authed.GET("/auth/tokens", r.listAPITokens, RequireRoles(admin...))
	authed.DELETE("/auth/tokens/:id", r.revokeAPIToken, RequireRoles(admin...))
	// Self-service ganti password sendiri (BEDA dari admin-reset
	// PATCH /users/:id/password di bawah) -- actor dari JWT langsung, bukan
	// target :id, jadi semua role yang sudah login boleh, TERMASUK ENDUSER
	// (karena itu di authedSelf, bukan authed yang di-gate staffOnly).
	authedSelf.PATCH("/auth/password", r.changeOwnPassword)
	authedSelf.GET("/auth/me", r.me)
	authedSelf.POST("/auth/impersonation/end", r.endImpersonation)
	// MFA TOTP akun sendiri (hanya sesi login, bukan API token).
	authedSelf.GET("/auth/mfa", r.mfaStatus)
	authedSelf.POST("/auth/mfa/setup", r.mfaSetup)
	authedSelf.POST("/auth/mfa/enable", r.mfaEnable)
	authedSelf.POST("/auth/mfa/disable", r.mfaDisable)

	// Notifikasi persisten (in-app + email). Semua milik pengguna yang login (tanpa :id numerik; id = UUID).
	// Menandai-baca & mengubah preferensi ditolak selama impersonate: SUPERADMIN yang "masuk sebagai" pengguna tidak
	// boleh menghapus jejak peringatan keamanan (mis. notifikasi impersonate itu sendiri). Pengumuman: SUPERADMIN saja.
	authed.GET("/notifications", r.listNotifications)
	authed.GET("/notifications/unread-count", r.notificationUnreadCount)
	authed.GET("/notifications/preferences", r.listNotificationPreferences)
	authed.POST("/notifications/read", r.markNotificationsRead, denyImpersonation)
	authed.PUT("/notifications/preferences", r.updateNotificationPreferences, denyImpersonation)
	authed.POST("/notifications/announce", r.announceNotification, RequireRoles(superadminOnly...), denyImpersonation)

	authed.GET("/refs/:table", r.listRefs)

	authed.GET("/cwmp/sessions/count", r.countOpenSessions, RequireRoles(adminOrNOC...))

	// Audit trail global tenant-scoped (activity_logs). ADMIN/SUPERADMIN —
	// operator NOC/VIEWER melihat aktivitas per-device lewat
	// GET /devices/:id/activity, bukan feed global ini.
	authed.GET("/activity", r.listActivity, RequireRoles(admin...))

	authed.POST("/tenants", r.createTenant, RequireRoles(superadminOnly...))
	authed.GET("/tenants", r.listTenants, RequireRoles(superadminOnly...))
	authed.GET("/tenants/current", r.getCurrentTenant)
	// GET by id memuat cwmp_inform_username, jadi dibatasi ADMIN+ (scope: ADMIN
	// hanya tenant sendiri, dicek di usecase/iam). DELETE = soft delete, superadmin
	// saja, ditolak 409 bila tenant masih punya user/device aktif.
	authed.GET("/tenants/:id", r.getTenant, RequireRoles(admin...))
	authed.DELETE("/tenants/:id", r.deleteTenant, RequireRoles(superadminOnly...))
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
	// Batas perangkat per tenant (paket SaaS): superadmin only.
	authed.PATCH("/tenants/:id/device-limit", r.setTenantDeviceLimit, RequireRoles(superadminOnly...))
	authed.POST("/users", r.createUser, RequireRoles(admin...), denyImpersonation)
	authed.GET("/users", r.listUsers, RequireRoles(admin...))
	// Self-service tenant admin (ROADMAP.md Fase 2): superadmin bisa ke user
	// manapun, ADMIN dibatasi ke user satu tenant + guard self-lockout —
	// keduanya dicek di usecase/iam, role gate di sini cuma menyaring NOC/VIEWER.
	authed.GET("/users/:id", r.getUser, RequireRoles(admin...))
	authed.PATCH("/users/:id", r.updateUser, RequireRoles(admin...), denyImpersonation)
	authed.PATCH("/users/:id/password", r.resetUserPassword, RequireRoles(admin...), denyImpersonation)
	authed.DELETE("/users/:id/mfa", r.adminResetMFA, RequireRoles(admin...), denyImpersonation)
	// SUPERADMIN "masuk sebagai" pengguna tenant (dukungan SaaS); token pendek, terbatas, teraudit.
	authed.POST("/users/:id/impersonate", r.impersonateUser, RequireRoles(superadminOnly...))
	// Laporan SaaS (operator platform).
	authed.GET("/reports/saas", r.getSaaSReport, RequireRoles(superadminOnly...))
	authed.GET("/reports/saas/tenants.csv", r.exportSaaSTenantsCSV, RequireRoles(superadminOnly...))
	// Penautan akun ENDUSER <-> perangkat untuk portal "WiFi Saya".
	authed.GET("/users/:id/devices", r.listUserDevices, RequireRoles(admin...))
	authed.PUT("/users/:id/devices/:deviceId", r.linkUserDevice, RequireRoles(admin...))
	authed.DELETE("/users/:id/devices/:deviceId", r.unlinkUserDevice, RequireRoles(admin...))
	authed.PATCH("/users/:id/roles", r.replaceUserRoles, RequireRoles(admin...), denyImpersonation)
	authed.DELETE("/users/:id", r.deleteUser, RequireRoles(admin...), denyImpersonation)

	if r.Faults != nil {
		// Faults: baca untuk semua peran staf (scope tenant di usecase); menutup fault ADMIN/NOC.
		authed.GET("/faults", r.listFaults)
		authed.GET("/faults/:uuid", r.getFault)
		authed.DELETE("/faults/:uuid", r.dismissFault, RequireRoles(adminOrNOC...))
	}

	if r.Automation != nil {
		// Otomasi: baca (aturan & riwayat) ADMIN/NOC; mutasi, uji, dan reset ADMIN. Rute statis /test didaftarkan sebelum /:uuid.
		authed.GET("/automation-rules", r.listAutomationRules, RequireRoles(adminOrNOC...))
		authed.POST("/automation-rules/test", r.testAutomationRule, RequireRoles(admin...))
		authed.GET("/automation-rules/:uuid", r.getAutomationRule, RequireRoles(adminOrNOC...))
		authed.POST("/automation-rules", r.createAutomationRule, RequireRoles(admin...))
		authed.PATCH("/automation-rules/:uuid", r.updateAutomationRule, RequireRoles(admin...))
		authed.DELETE("/automation-rules/:uuid", r.deleteAutomationRule, RequireRoles(admin...))
		authed.POST("/automation-rules/:uuid/reset", r.resetAutomationRule, RequireRoles(admin...))
		authed.GET("/automation-runs", r.listAutomationRuns, RequireRoles(adminOrNOC...))
	}
	if r.VParams != nil {
		// Virtual parameter: baca untuk semua peran staf (scope tenant di usecase); definisi & uji ekspresi ADMIN. Rute statis /test
		// didaftarkan sebelum /:uuid.
		authed.GET("/virtual-parameters", r.listVirtualParameters)
		authed.POST("/virtual-parameters/test", r.testVirtualParameter, RequireRoles(admin...))
		authed.GET("/virtual-parameters/:uuid", r.getVirtualParameter)
		authed.POST("/virtual-parameters", r.createVirtualParameter, RequireRoles(admin...))
		authed.PATCH("/virtual-parameters/:uuid", r.updateVirtualParameter, RequireRoles(admin...))
		authed.DELETE("/virtual-parameters/:uuid", r.deleteVirtualParameter, RequireRoles(admin...))
		authed.GET("/devices/:id/virtual-parameters", r.deviceVirtualParameters)
	}

	authed.GET("/devices", r.listDevices)
	authed.GET("/devices/stats", r.deviceStats)
	authed.GET("/devices/insights", r.deviceInsights)
	if r.Capture != nil {
		// Capture sesi CWMP: ADMIN/SUPERADMIN saja (pesan SOAP memuat data pelanggan); scope tenant ditegakkan di usecase.
		authed.GET("/devices/:id/capture", r.getCapture, RequireRoles(admin...))
		authed.POST("/devices/:id/capture", r.enableCapture, RequireRoles(admin...))
		authed.DELETE("/devices/:id/capture", r.disableCapture, RequireRoles(admin...))
		authed.GET("/devices/:id/captures", r.listCaptures, RequireRoles(admin...))
		authed.GET("/devices/:id/captures/export", r.exportCaptures, RequireRoles(admin...))
		authed.GET("/devices/:id/captures/:uuid", r.getCaptureMessage, RequireRoles(admin...))
	}
	authed.GET("/devices/:id", r.getDevice)
	// Pra-registrasi perangkat (sebelum CPE pertama kali Inform) & impor massal.
	authed.POST("/devices", r.createDevice, RequireRoles(admin...))
	authed.POST("/devices/import", r.importDevices, RequireRoles(admin...))
	authed.PATCH("/devices/:id", r.updateDevice, RequireRoles(adminOrNOC...))
	// Soft delete device (dekomisioning): ADMIN+ pada tenant sendiri. Task terbuka
	// dibatalkan; CPE yang masih menyala dan Inform lagi di-revive sebagai
	// registrasi baru (usecase/device.Delete, repository Create).
	authed.DELETE("/devices/:id", r.deleteDevice, RequireRoles(admin...))
	authed.GET("/devices/:id/parameters", r.listDeviceParameters)
	authed.GET("/devices/:id/events", r.listDeviceEvents)
	authed.GET("/devices/:id/optical-metrics", r.listOpticalMetrics)
	authed.GET("/devices/:id/activity", r.listDeviceActivity)
	authed.GET("/devices/:id/config-snapshots", r.listConfigSnapshots)
	authed.POST("/devices/:id/config-snapshots", r.createConfigSnapshot, RequireRoles(adminOrNOC...))
	// POST /devices/:id/connection-request: memaksa CPE Inform segera (TR-069\
	// §3.2.2 Connection Request). adminOrNOC — operator NOC sering butuh ini\
	// untuk troubleshoot tanpa harus menunggu periodic inform (15-60 menit).\
	// HTTP 202 Accepted (bukan 200) karena aksi ini asinkron: ACS mengirim\
	// request ke device, tapi Inform dari device bisa datang beberapa detik\
	// kemudian di sesi terpisah (bukan respons sinkron dalam request ini).\
	// 400 dikembalikan bila device tidak memiliki connection_request_url.\
	authed.POST("/devices/:id/connection-request", r.triggerConnectionRequest, RequireRoles(adminOrNOC...))
	authed.POST("/devices/:id/reboot", r.rebootDevice, RequireRoles(adminOrNOC...))
	authed.POST("/devices/:id/factory-reset", r.factoryResetDevice, RequireRoles(admin...))
	authed.POST("/devices/:id/push-file", r.pushFileToDevice, RequireRoles(admin...))

	// Advanced TR-069 RPCs (FR-5)
	authed.POST("/devices/:id/tasks/add-object", r.addObjectDevice, RequireRoles(admin...))
	authed.POST("/devices/:id/tasks/delete-object", r.deleteObjectDevice, RequireRoles(admin...))
	authed.POST("/devices/:id/tasks/get-parameter-names", r.getParameterNamesDevice, RequireRoles(adminOrNOC...))
	authed.POST("/devices/:id/tasks/get-parameter-values", r.getParameterValuesDevice, RequireRoles(adminOrNOC...))
	authed.POST("/devices/:id/tasks/set-parameter-values", r.setParameterValuesDevice, RequireRoles(admin...))
	authed.POST("/devices/refresh-summary", r.refreshDeviceSummary, RequireRoles(adminOrNOC...))
	authed.POST("/devices/:id/refresh", r.refreshDeviceTree, RequireRoles(adminOrNOC...))
	authed.POST("/devices/:id/tasks/set-parameter-attributes", r.setParameterAttributesDevice, RequireRoles(admin...))
	authed.POST("/devices/:id/tasks/get-parameter-attributes", r.getParameterAttributesDevice, RequireRoles(adminOrNOC...))

	authed.GET("/tasks", r.listTasks)
	authed.GET("/tasks/stats", r.taskStats)
	authed.GET("/tasks/:id", r.getTask)
	authed.POST("/tasks", r.createTask, RequireRoles(adminOrNOC...))
	authed.POST("/tasks/:id/cancel", r.cancelTask, RequireRoles(adminOrNOC...))
	authed.POST("/tasks/:id/retry", r.retryTask, RequireRoles(adminOrNOC...))

	authed.GET("/provisioning-profiles", r.listProfiles)
	authed.GET("/provisioning-profiles/:id", r.getProfile)
	authed.POST("/provisioning-profiles", r.createProfile, RequireRoles(admin...))
	authed.PUT("/provisioning-profiles/:id", r.updateProfile, RequireRoles(admin...))
	authed.DELETE("/provisioning-profiles/:id", r.deleteProfile, RequireRoles(admin...))
	authed.POST("/devices/:id/apply-profile/:profileId", r.applyProfile, RequireRoles(admin...))

	authed.GET("/zero-touch-rules", r.listZTRules)
	authed.GET("/zero-touch-rules/:id", r.getZTRule)
	authed.POST("/zero-touch-rules", r.createZTRule, RequireRoles(admin...))
	authed.PUT("/zero-touch-rules/:id", r.updateZTRule, RequireRoles(admin...))
	authed.DELETE("/zero-touch-rules/:id", r.deleteZTRule, RequireRoles(admin...))

	// Katalog vendor — data referensi GLOBAL: baca untuk semua role terautentikasi,
	// mutasi (create/update/delete) superadmin saja. Delete = soft delete dan
	// ditolak 409 selama masih ada data aktif yang merujuk (usecase/catalog).
	authed.GET("/vendors", r.listVendors)
	authed.POST("/vendors", r.createVendor, RequireRoles(superadminOnly...))
	authed.GET("/vendors/:id", r.getVendor)
	authed.PATCH("/vendors/:id", r.updateVendor, RequireRoles(superadminOnly...))
	authed.DELETE("/vendors/:id", r.deleteVendor, RequireRoles(superadminOnly...))
	authed.GET("/vendors/:id/ouis", r.listVendorOUIs)
	authed.POST("/vendors/:id/ouis", r.addVendorOUI, RequireRoles(superadminOnly...))
	authed.DELETE("/vendors/:id/ouis/:ouiId", r.deleteVendorOUI, RequireRoles(superadminOnly...))
	authed.GET("/device-models", r.listDeviceModels)
	authed.POST("/device-models", r.createDeviceModel, RequireRoles(superadminOnly...))
	authed.GET("/device-models/:id", r.getDeviceModel)
	authed.PATCH("/device-models/:id", r.updateDeviceModel, RequireRoles(superadminOnly...))
	authed.DELETE("/device-models/:id", r.deleteDeviceModel, RequireRoles(superadminOnly...))
	authed.GET("/vendor-parameter-mappings", r.listParameterMappings)
	authed.POST("/vendor-parameter-mappings", r.upsertParameterMapping, RequireRoles(superadminOnly...))
	authed.GET("/vendor-parameter-mappings/:id", r.getParameterMapping)
	authed.DELETE("/vendor-parameter-mappings/:id", r.deleteParameterMapping, RequireRoles(superadminOnly...))

	// firmware_files adalah katalog global lintas tenant (tidak py tenant_id,
	// sama seperti vendors/device-models/vendor-parameter-mappings) — upload
	// dibatasi superadmin, konsisten dgn data referensi global lain (temuan
	// audit isolasi tenant Fase 2: sebelumnya admin tenant mana pun bisa
	// upload firmware yang lalu dipakai tenant lain via device-models/vendor
	// yang sama, celah supply-chain/integrity lintas tenant).
	authed.POST("/firmware", r.uploadFirmware, RequireRoles(superadminOnly...))
	authed.GET("/firmware", r.listFirmware)
	// get/update/delete firmware: katalog global, mutasi superadmin saja. Delete =
	// soft delete, ditolak 409 selama rule ZTP/rollout/job upgrade masih memakainya.
	authed.GET("/firmware/:id", r.getFirmware)
	authed.PATCH("/firmware/:id", r.updateFirmware, RequireRoles(superadminOnly...))
	authed.DELETE("/firmware/:id", r.deleteFirmware, RequireRoles(superadminOnly...))
	authed.POST("/devices/:id/firmware-upgrade", r.scheduleFirmwareUpgrade, RequireRoles(admin...))
	authed.GET("/devices/:id/firmware-jobs", r.listFirmwareJobs)

	// Canary/staged rollout firmware ke populasi device (migrations/0011) --
	// gating sama seperti scheduleFirmwareUpgrade (ADMIN/SUPERADMIN, ini versi
	// skala-populasi dari operasi yang sama). GET dibuka ke semua role
	// terautentikasi (pola sama dgn listFirmware/listFirmwareJobs) supaya
	// NOC/VIEWER bisa memantau progres tanpa bisa memicu/membatalkannya.
	authed.POST("/firmware/rollout-batches", r.createRolloutBatch, RequireRoles(admin...))
	authed.GET("/firmware/rollout-batches", r.listRolloutBatches)
	authed.GET("/firmware/rollout-batches/:id", r.getRolloutBatch)
	authed.POST("/firmware/rollout-batches/:id/advance", r.advanceRolloutBatch, RequireRoles(admin...))
	authed.POST("/firmware/rollout-batches/:id/cancel", r.cancelRolloutBatch, RequireRoles(admin...))

	authed.POST("/devices/:id/diagnostics", r.triggerDiagnostic, RequireRoles(adminOrNOC...))
	authed.GET("/devices/:id/diagnostics", r.listDiagnostics)

	// Webhook keluar (typed event → BSS/OSS/NMS, migrations/0013). Mutasi
	// (buat/ubah/hapus/test) gated ADMIN/SUPERADMIN; GET dibuka ke semua role
	// terautentikasi (NOC/VIEWER boleh memantau langganan & log delivery).
	// Semua tenant-scoped di usecase.
	authed.POST("/webhooks", r.createWebhook, RequireRoles(admin...))
	authed.GET("/webhooks/deliveries/failed-count", r.countFailedDeliveries)
	authed.GET("/webhooks", r.listWebhooks)
	authed.GET("/webhooks/:id", r.getWebhook)
	authed.PATCH("/webhooks/:id", r.updateWebhook, RequireRoles(admin...))
	authed.DELETE("/webhooks/:id", r.deleteWebhook, RequireRoles(admin...))
	authed.POST("/webhooks/:id/test", r.testWebhook, RequireRoles(admin...))
	authed.GET("/webhooks/:id/deliveries", r.listWebhookDeliveries)

	authed.POST("/files", r.uploadFile, RequireRoles(admin...))
	authed.GET("/files", r.listFiles)
	authed.DELETE("/files/:id", r.deleteFile, RequireRoles(admin...))

	authed.POST("/tags", r.createTag, RequireRoles(admin...))
	authed.GET("/tags", r.listTags)
	authed.PATCH("/tags/:id", r.updateTag, RequireRoles(admin...))
	authed.DELETE("/tags/:id", r.deleteTag, RequireRoles(admin...))
	// Device <-> tag (segmentasi gaya GenieACS). GET terbuka utk semua role
	// terautentikasi (spt GET /devices); assign/remove adminOrNOC (aksi
	// operasional NOC, bukan mutasi katalog). Filter GET /devices?tag_id=N.
	authed.GET("/devices/:id/tags", r.listDeviceTags)
	authed.POST("/devices/:id/tags", r.assignDeviceTag, RequireRoles(adminOrNOC...))
	authed.DELETE("/devices/:id/tags/:tagId", r.removeDeviceTag, RequireRoles(adminOrNOC...))

	authed.POST("/presets", r.createPreset, RequireRoles(admin...))
	authed.GET("/presets", r.listPresets)
	authed.GET("/presets/:id", r.getPreset)
	authed.PATCH("/presets/:id", r.updatePreset, RequireRoles(admin...))
	authed.DELETE("/presets/:id", r.deletePreset, RequireRoles(admin...))

	// Langganan SaaS (paket, status, checkout, invoice, sync). Sengaja TANPA denyImpersonation (lihat billing_handler.go).
	r.mountBillingRoutes(authed)
	// Layout menu, laporan bug, riwayat impor/ekspor (authedSelf = semua peran masuk; authed = staf).
	r.mountConsoleRoutes(authedSelf, authed)

	r.mountSelfServiceRoutes(api)
}
