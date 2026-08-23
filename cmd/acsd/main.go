// Command acsd adalah entrypoint utama ACS: menjalankan endpoint CWMP
// (untuk CPE) dan REST API internal (untuk BSS/OSS/portal NOC) sebagai dua
// server terpisah dalam satu proses, sesuai TECH.md §2.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"

	"acs/internal/config"
	deliverycwmp "acs/internal/delivery/cwmp"
	deliveryhttp "acs/internal/delivery/http"
	"acs/internal/repository/mysql"
	"acs/internal/usecase/auth"
	"acs/internal/usecase/device"
	"acs/internal/usecase/diagnostics"
	"acs/internal/usecase/firmware"
	"acs/internal/usecase/iam"
	"acs/internal/usecase/provisioning"
	"acs/internal/usecase/session"
	"acs/internal/usecase/task"
	"acs/pkg/cryptoutil"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	db, err := mysql.Connect(cfg.DBDSN)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer db.Close()

	enc, err := cryptoutil.NewEncryptor(cfg.CredentialEncKey)
	if err != nil {
		log.Fatalf("cryptoutil: %v", err)
	}

	refRepo := mysql.NewRefRepository(db)
	tenantRepo := mysql.NewTenantRepository(db)
	userRepo := mysql.NewUserRepository(db)
	apiTokenRepo := mysql.NewAPITokenRepository(db)
	activityLogRepo := mysql.NewActivityLogRepository(db)
	vendorRepo := mysql.NewVendorRepository(db)
	vendorOUIRepo := mysql.NewVendorOUIRepository(db)
	deviceModelRepo := mysql.NewDeviceModelRepository(db)
	paramMappingRepo := mysql.NewVendorParameterMappingRepository(db)
	deviceRepo := mysql.NewDeviceRepository(db)
	deviceParamRepo := mysql.NewDeviceParameterRepository(db)
	deviceSessionRepo := mysql.NewDeviceSessionRepository(db)
	deviceEventRepo := mysql.NewDeviceEventRepository(db)
	opticalMetricRepo := mysql.NewDeviceOpticalMetricRepository(db)
	taskRepo := mysql.NewTaskRepository(db)
	profileRepo := mysql.NewProvisioningProfileRepository(db)
	profileParamRepo := mysql.NewProvisioningProfileParameterRepository(db)
	ztRuleRepo := mysql.NewZeroTouchRuleRepository(db)
	firmwareFileRepo := mysql.NewFirmwareFileRepository(db)
	firmwareJobRepo := mysql.NewFirmwareUpgradeJobRepository(db)
	diagnosticRepo := mysql.NewDeviceDiagnosticRepository(db)

	authSvc := auth.NewService(userRepo, apiTokenRepo, activityLogRepo, cfg.JWTSecret, cfg.JWTExpiry)
	iamSvc := iam.NewService(tenantRepo, userRepo, refRepo, activityLogRepo, enc)
	taskSvc := task.NewService(taskRepo, deviceRepo, deviceModelRepo, paramMappingRepo, refRepo, activityLogRepo)
	provisioningSvc := provisioning.NewService(profileRepo, profileParamRepo, ztRuleRepo, deviceRepo, taskSvc, activityLogRepo)
	deviceSvc := device.NewService(deviceRepo, vendorOUIRepo, deviceModelRepo, refRepo, deviceParamRepo, deviceEventRepo, opticalMetricRepo, enc, activityLogRepo)
	firmwareSvc := firmware.NewService(firmwareFileRepo, firmwareJobRepo, deviceRepo, taskSvc, refRepo, activityLogRepo)
	diagnosticsSvc := diagnostics.NewService(diagnosticRepo, deviceRepo, taskSvc, activityLogRepo)
	sessionSvc := session.NewService(deviceSessionRepo, deviceEventRepo, deviceParamRepo, deviceRepo, tenantRepo, refRepo, deviceSvc, taskSvc, provisioningSvc, firmwareSvc, diagnosticsSvc, enc)

	// ---- REST API internal (BSS/OSS, portal NOC) ----
	restEcho := echo.New()
	restEcho.Use(middleware.Recover())
	restEcho.Use(middleware.RequestLogger())
	restEcho.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: cfg.CORSAllowOrigins,
		AllowMethods: []string{http.MethodGet, http.MethodPost, http.MethodPatch, http.MethodPut, http.MethodDelete, http.MethodOptions},
		AllowHeaders: []string{"Authorization", "Content-Type"},
	}))
	// Rate limit per identifier (default: IP) — jauh lebih longgar dari CWMP
	// (5 req/s) karena dashboard frontend polling beberapa endpoint tiap 5
	// detik per tab; 30 req/s tetap membatasi brute-force /auth/login &
	// scraping API token sambil tidak mengganggu pemakaian normal
	// (ROADMAP.md Fase 2 — gap yang dicatat saat CWMP rate limit ditambahkan).
	restEcho.Use(middleware.RateLimiter(middleware.NewRateLimiterMemoryStore(30)))
	router := &deliveryhttp.Router{
		Auth: authSvc, IAM: iamSvc, Devices: deviceSvc, Tasks: taskSvc, Provisioning: provisioningSvc,
		Firmware: firmwareSvc, Diagnostics: diagnosticsSvc,
		Vendors: vendorRepo, VendorOUIs: vendorOUIRepo, DeviceModels: deviceModelRepo, ParamMappings: paramMappingRepo,
		Refs: refRepo, Activity: activityLogRepo,
	}
	router.Register(restEcho)

	// ---- Endpoint CWMP (CPE) — server terpisah agar rate limiting/exposure
	// publik bisa diatur berbeda dari REST API internal (TECH.md §8). ----
	cwmpEcho := echo.New()
	cwmpEcho.Use(middleware.Recover())
	// Rate limit per identifier (default: IP) — endpoint ini sekarang menjaga
	// shared secret Inform CWMP sungguhan (bukan cuma anti CPE nakal/loop
	// seperti sebelumnya), jadi juga jadi mitigasi brute-force kredensial.
	cwmpEcho.Use(middleware.RateLimiter(middleware.NewRateLimiterMemoryStore(5)))
	deliverycwmp.NewHandler(sessionSvc).Register(cwmpEcho, "/cwmp")

	// echo.Start() menangani graceful shutdown otomatis saat menerima
	// SIGINT/SIGTERM (lihat vendor echo/v5 server.go — signal.NotifyContext
	// internal), jadi cukup ditunggu selesai lewat WaitGroup.
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		log.Printf("REST API listening on %s", cfg.HTTPAddr)
		if err := restEcho.Start(cfg.HTTPAddr); err != nil {
			log.Printf("rest server: %v", err)
		}
	}()
	go func() {
		defer wg.Done()
		log.Printf("CWMP endpoint listening on %s", cfg.CWMPAddr)
		if err := cwmpEcho.Start(cfg.CWMPAddr); err != nil {
			log.Printf("cwmp server: %v", err)
		}
	}()

	stop := make(chan struct{})
	go runSweepers(deviceSvc, taskSvc, stop)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit
	close(stop)

	wg.Wait()
}

// runSweepers menjalankan tugas periodik ringan (FR-22, timeout task) — aman
// dijalankan dari instance manapun karena app server stateless (TECH.md §9).
func runSweepers(deviceSvc *device.Service, taskSvc *task.Service, stop <-chan struct{}) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			ctx := context.Background()
			if n, err := deviceSvc.MarkStaleOffline(ctx, 15*time.Minute); err != nil {
				log.Printf("sweeper: mark stale offline gagal: %v", err)
			} else if n > 0 {
				log.Printf("sweeper: %d device ditandai offline", n)
			}
			if n, err := taskSvc.TimeoutStaleSent(ctx, 5*time.Minute); err != nil {
				log.Printf("sweeper: timeout stale sent gagal: %v", err)
			} else if n > 0 {
				log.Printf("sweeper: %d task ditandai timeout", n)
			}
		}
	}
}
