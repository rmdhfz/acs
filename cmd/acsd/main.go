// Command acsd adalah entrypoint utama ACS: menjalankan endpoint CWMP
// (untuk CPE) dan REST API internal (untuk BSS/OSS/portal NOC) sebagai dua
// server terpisah dalam satu proses, sesuai TECH.md §2.
package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"acs/internal/config"
	deliverycwmp "acs/internal/delivery/cwmp"
	deliveryhttp "acs/internal/delivery/http"
	"acs/internal/delivery/ws"
	"acs/internal/domain"
	"acs/internal/metrics"
	"acs/internal/repository/mysql"
	"acs/internal/repository/redisrepo"
	"acs/internal/usecase/auth"
	"acs/internal/usecase/device"
	"acs/internal/usecase/diagnostics"
	"acs/internal/usecase/file"
	"acs/internal/usecase/firmware"
	"acs/internal/usecase/iam"
	"acs/internal/usecase/provisioning"
	"acs/internal/usecase/session"
	"acs/internal/usecase/task"
	"acs/internal/usecase/webhook"
	"acs/pkg/cryptoutil"
	"acs/pkg/objectstorage"
	"acs/pkg/redisutil"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	// Logger terstruktur (log/slog, JSON) — SATU instance dipakai bersama
	// jalur CWMP (delivery/cwmp, usecase/session) sesuai observability
	// TECH.md §10, alih-alih memperkenalkan library logging kedua. Level
	// dikontrol ACS_LOG_LEVEL (default INFO) supaya production bisa menaikkan
	// verbosity (DEBUG = detail per-RPC) tanpa redeploy. slog.SetDefault jaga
	// konsistensi dgn kode lain yang mungkin memakai slog.Default() tanpa
	// instance eksplisit (mis. pustaka pihak ketiga).
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: parseLogLevel(cfg.LogLevel)}))
	slog.SetDefault(logger)

	db, err := mysql.Connect(cfg.DBDSN)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer db.Close()

	enc, err := cryptoutil.NewEncryptor(cfg.CredentialEncKey)
	if err != nil {
		log.Fatalf("cryptoutil: %v", err)
	}

	// ---- Object storage firmware (MinIO/S3-compatible, ROADMAP.md Fase 2) ----
	minioCtx, minioCancel := context.WithTimeout(context.Background(), 30*time.Second)
	objStorage, err := objectstorage.New(minioCtx, objectstorage.Config{
		Endpoint:  cfg.MinIOEndpoint,
		AccessKey: cfg.MinIOAccessKey,
		SecretKey: cfg.MinIOSecretKey,
		Bucket:    cfg.MinIOBucket,
		UseSSL:    cfg.MinIOUseSSL,
	})
	minioCancel()
	if err != nil {
		log.Fatalf("objectstorage: %v", err)
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
	deviceSessionRepoMysql := mysql.NewDeviceSessionRepository(db)
	configSnapshotRepo := mysql.NewDeviceConfigSnapshotRepository(db)
	deviceEventRepo := mysql.NewDeviceEventRepository(db)
	opticalMetricRepo := mysql.NewDeviceOpticalMetricRepository(db)
	taskRepo := mysql.NewTaskRepository(db)
	profileRepo := mysql.NewProvisioningProfileRepository(db)
	profileParamRepo := mysql.NewProvisioningProfileParameterRepository(db)
	ztRuleRepo := mysql.NewZeroTouchRuleRepository(db)
	firmwareFileRepo := mysql.NewFirmwareFileRepository(db)
	firmwareJobRepo := mysql.NewFirmwareUpgradeJobRepository(db)
	firmwareRolloutRepo := mysql.NewFirmwareRolloutBatchRepository(db)
	diagnosticRepo := mysql.NewDeviceDiagnosticRepository(db)
	webhookSubRepo := mysql.NewWebhookSubscriptionRepository(db)
	webhookDeliveryRepo := mysql.NewWebhookDeliveryRepository(db)
	fileRepo := mysql.NewFileRepository(db)

	redisClient, err := redisutil.NewClient(cfg.RedisAddr, logger)
	if err != nil {
		logger.Warn("redis: gagal koneksi, session cache dinonaktifkan", "error", err)
	}
	var deviceSessionRepo domain.DeviceSessionRepository = deviceSessionRepoMysql
	if redisClient != nil {
		deviceSessionRepo = redisrepo.NewDeviceSessionRepository(deviceSessionRepoMysql, redisClient)
	}

	authSvc := auth.NewService(userRepo, apiTokenRepo, tenantRepo, activityLogRepo, cfg.JWTSecret, cfg.JWTExpiry, cfg.OIDCIssuer, cfg.OIDCClientID, cfg.OIDCClientSecret, cfg.OIDCRedirectURL)
	iamSvc := iam.NewService(tenantRepo, userRepo, refRepo, activityLogRepo, enc)
	// webhookSvc dikonstruksi lebih dulu -- session & task memakainya lewat
	// domain.WebhookEnqueuer (interface sempit, tanpa import cycle), worker
	// dispatch-nya dijalankan runSweepers.
	webhookSvc := webhook.NewService(webhookSubRepo, webhookDeliveryRepo, refRepo, activityLogRepo, enc, logger)
	
	wsHub := ws.NewHub()
	go wsHub.Run()

	taskSvc := task.NewService(taskRepo, deviceRepo, deviceModelRepo, paramMappingRepo, refRepo, activityLogRepo, tenantRepo, webhookSvc, wsHub)
	// firmwareSvc dikonstruksi SEBELUM provisioningSvc -- provisioningSvc
	// (ZTP aksi FirmwareFileID, migrations/0009) bergantung pada firmwareSvc
	// lewat domain.FirmwareScheduler (interface sempit, menghindari import
	// cycle usecase/provisioning <-> usecase/firmware).
	firmwareSvc := firmware.NewService(firmwareFileRepo, firmwareJobRepo, firmwareRolloutRepo, deviceRepo, taskSvc, refRepo, activityLogRepo, objStorage)
	provisioningSvc := provisioning.NewService(profileRepo, profileParamRepo, ztRuleRepo, deviceRepo, deviceParamRepo, refRepo, taskSvc, firmwareSvc, activityLogRepo)
	deviceSvc := device.NewService(deviceRepo, vendorOUIRepo, deviceModelRepo, refRepo, deviceParamRepo, deviceEventRepo, opticalMetricRepo, configSnapshotRepo, enc, activityLogRepo, taskSvc, fileRepo, objStorage)
	diagnosticsSvc := diagnostics.NewService(diagnosticRepo, deviceRepo, taskSvc, activityLogRepo)
	sessionSvc := session.NewService(deviceSessionRepo, deviceEventRepo, deviceParamRepo, deviceRepo, tenantRepo, refRepo, deviceSvc, taskSvc, provisioningSvc, firmwareSvc, diagnosticsSvc, enc, logger, webhookSvc, wsHub)
	fileSvc := file.NewService(fileRepo, objStorage, activityLogRepo)

	// ---- Observability: metrics Prometheus (TECH.md §10, ROADMAP.md Fase 2) ----
	// Registry terpisah (bukan prometheus.DefaultRegisterer) supaya /metrics
	// HANYA mengekspos metrik agregat ACS di atas — tidak ikut membocorkan
	// metrik proses Go bawaan client_golang (go_*, process_*) yang biasanya
	// auto-register ke DefaultRegisterer; ini keputusan cakupan minimal
	// sesuai scope sesi ini, bukan larangan permanen menambah metrik proses
	// nanti kalau dibutuhkan.
	metricsRegistry := prometheus.NewRegistry()
	metricsRegistry.MustRegister(metrics.NewCollector(deviceSvc, taskSvc, sessionSvc, refRepo))
	// InformResponseLatency — histogram real-time (BUKAN gauge query-on-scrape
	// spt Collector di atas), lihat komentar lengkap di
	// internal/metrics/inform_latency.go. Diregistrasi terpisah krn bukan
	// bagian dari prometheus.Collector kustom yang sama.
	metricsRegistry.MustRegister(metrics.InformResponseLatency)
	metricsHandler := promhttp.HandlerFor(metricsRegistry, promhttp.HandlerOpts{})

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
		Firmware: firmwareSvc, Diagnostics: diagnosticsSvc, Webhooks: webhookSvc, Sessions: sessionSvc,
		Vendors: vendorRepo, VendorOUIs: vendorOUIRepo, DeviceModels: deviceModelRepo, ParamMappings: paramMappingRepo,
		Refs: refRepo, Activity: activityLogRepo,
		MetricsHandler: metricsHandler,
		Files: fileSvc,
		WSHub: wsHub,
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
	deliverycwmp.NewHandler(sessionSvc, logger).Register(cwmpEcho, "/cwmp")

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
	go runSweepers(deviceSvc, taskSvc, firmwareSvc, webhookSvc, stop)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit
	close(stop)

	wg.Wait()
}

// parseLogLevel menerjemahkan ACS_LOG_LEVEL (string, case-insensitive) ke
// slog.Level. Nilai tidak dikenal fallback ke Info (default aman), bukan
// error fatal — kesalahan ketik di env var utk verbosity log tidak boleh
// mencegah proses start sama sekali.
func parseLogLevel(s string) slog.Level {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "DEBUG":
		return slog.LevelDebug
	case "WARN", "WARNING":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// runSweepers menjalankan tugas periodik ringan (FR-22, timeout task, wave
// rollout firmware, dispatch webhook) — aman dijalankan dari instance manapun
// karena app server stateless (TECH.md §9). Dispatch webhook pakai ticker
// terpisah yang lebih cepat (15 dtk) supaya event fault/value-change sampai
// ke sistem pihak ketiga dengan latensi rendah.
func runSweepers(deviceSvc *device.Service, taskSvc *task.Service, firmwareSvc *firmware.Service, webhookSvc *webhook.Service, stop <-chan struct{}) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	webhookTicker := time.NewTicker(15 * time.Second)
	defer webhookTicker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-webhookTicker.C:
			if n, err := webhookSvc.DispatchDue(context.Background()); err != nil {
				log.Printf("sweeper: dispatch webhook gagal: %v", err)
			} else if n > 0 {
				log.Printf("sweeper: %d webhook delivery diproses", n)
			}
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
			// Cek wave rollout firmware yang sudah tuntas & lanjutkan ke wave
			// berikutnya (atau pause/complete) -- lihat firmware.Service.AdvanceRollout.
			if n, err := firmwareSvc.SweepRolloutBatches(ctx); err != nil {
				log.Printf("sweeper: sweep firmware rollout batch gagal: %v", err)
			} else if n > 0 {
				log.Printf("sweeper: %d firmware rollout batch diproses", n)
			}
		}
	}
}
