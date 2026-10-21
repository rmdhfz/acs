// Command acsd adalah entrypoint utama ACS: menjalankan endpoint CWMP
// (untuk CPE) dan REST API internal (untuk BSS/OSS/portal NOC) sebagai dua
// server terpisah dalam satu proses, sesuai TECH.md §2.
package main

import (
	"context"
	"log"
	"log/slog"
	"net"
	"net/http"
	"net/url"
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
	deliveryusp "acs/internal/delivery/usp"
	"acs/internal/delivery/ws"
	"acs/internal/domain"
	"acs/internal/metrics"
	"acs/internal/payment/xendit"
	"acs/internal/repository/mysql"
	"acs/internal/repository/redisrepo"
	"acs/internal/usecase/auth"
	"acs/internal/usecase/billing"
	"acs/internal/usecase/catalog"
	"acs/internal/usecase/console"
	"acs/internal/usecase/device"
	"acs/internal/usecase/diagnostics"
	"acs/internal/usecase/file"
	"acs/internal/usecase/firmware"
	"acs/internal/usecase/iam"
	"acs/internal/usecase/notification"
	"acs/internal/usecase/notify"
	"acs/internal/usecase/preset"
	"acs/internal/usecase/provisioning"
	"acs/internal/usecase/report"
	"acs/internal/usecase/selfservice"
	"acs/internal/usecase/session"
	"acs/internal/usecase/signup"
	"acs/internal/usecase/tag"
	"acs/internal/usecase/task"
	"acs/internal/usecase/usp_session"
	"acs/internal/usecase/webhook"
	"acs/pkg/cryptoutil"
	"acs/pkg/mailer"
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

	db, err := mysql.ConnectWithPool(cfg.DBDSN, cfg.DBMaxOpenConns, cfg.DBMaxIdleConns, time.Duration(cfg.DBConnMaxLifetimeMinutes)*time.Minute)
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

	// Cache in-memory untuk lookup ref_* imutabel (event code, status, trigger
	// ZTP) — menghilangkan ~6 query DB per Inform di jalur panas CWMP
	// (perf-scale-auditor). GET /refs/:table (List) tetap selalu segar.
	refRepo := mysql.NewCachedRefRepository(mysql.NewRefRepository(db))
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
	profileParamRepo := mysql.NewProvisioningProfileParameterRepository(db, enc)
	ztRuleRepo := mysql.NewZeroTouchRuleRepository(db)
	firmwareFileRepo := mysql.NewFirmwareFileRepository(db)
	firmwareJobRepo := mysql.NewFirmwareUpgradeJobRepository(db)
	firmwareRolloutRepo := mysql.NewFirmwareRolloutBatchRepository(db)
	diagnosticRepo := mysql.NewDeviceDiagnosticRepository(db)
	webhookSubRepo := mysql.NewWebhookSubscriptionRepository(db)
	webhookDeliveryRepo := mysql.NewWebhookDeliveryRepository(db)
	fileRepo := mysql.NewFileRepository(db)
	tagRepo := mysql.NewTagRepository(db)
	presetRepo := mysql.NewPresetRepository(db, enc)
	presetAppRepo := mysql.NewPresetApplicationRepository(db)
	userDeviceRepo := mysql.NewUserDeviceRepository(db)
	usageRepo := mysql.NewUsageRepository(db)

	redisClient, err := redisutil.NewClient(cfg.RedisAddr, cfg.RedisPassword, logger)
	if err != nil {
		logger.Warn("redis: gagal koneksi, session cache dinonaktifkan", "error", err)
	}
	var deviceSessionRepo domain.DeviceSessionRepository = deviceSessionRepoMysql
	if redisClient != nil {
		deviceSessionRepo = redisrepo.NewDeviceSessionRepository(deviceSessionRepoMysql, redisClient)
	}

	// Email (SMTP): reset password + notifikasi keamanan. SMTP belum
	// dikonfigurasi -> notifier Noop & reset lewat email nonaktif (503 di API).
	passwordResetRepo := mysql.NewPasswordResetRepository(db)
	mailNotifier, closeMail := mailer.NewNotifier(mailer.Config{
		Host: cfg.SMTPHost, Port: cfg.SMTPPort, Username: cfg.SMTPUsername, Password: cfg.SMTPPassword,
		From: cfg.SMTPFrom, FromName: cfg.SMTPFromName,
	}, logger)
	defer closeMail()
	mailBuilder := notify.NewBuilder(cfg.SMTPFromName, cfg.PublicURL)
	if cfg.SMTPHost != "" {
		logger.Info("email: SMTP aktif", "host", cfg.SMTPHost, "port", cfg.SMTPPort, "from", mailer.MaskEmail(cfg.SMTPFrom))
	}

	authSvc := auth.NewService(userRepo, apiTokenRepo, tenantRepo, activityLogRepo, cfg.JWTSecret, cfg.JWTExpiry, cfg.OIDCIssuer, cfg.OIDCClientID, cfg.OIDCClientSecret, cfg.OIDCRedirectURL)
	authSvc.WithOIDCAllowedDomains(cfg.OIDCAllowedDomains)
	if cfg.SMTPHost != "" {
		authSvc.WithPasswordReset(passwordResetRepo, mailNotifier, mailBuilder, cfg.PasswordResetTTL)
	}
	// MFA TOTP: rahasia dienkripsi dengan kunci kredensial yang sama (AES-GCM).
	authSvc.WithMFA(mysql.NewMFARepository(db), enc, "ACS")
	tenantQuotaRepo := mysql.NewTenantQuotaRepository(db)
	signupRepo := mysql.NewSignupRepository(db)
	iamSvc := iam.NewService(tenantRepo, userRepo, refRepo, activityLogRepo, enc, usageRepo).WithQuota(tenantQuotaRepo)
	if cfg.SMTPHost != "" {
		iamSvc.WithEmail(mailNotifier, mailBuilder, passwordResetRepo)
	}
	// Pendaftaran tenant mandiri (bawaan mati; butuh SMTP — divalidasi di config).
	signupSvc := signup.NewService(signupRepo, iamSvc, userRepo, activityLogRepo, mailNotifier, mailBuilder, signup.Config{
		Enabled: cfg.SelfSignup, DailyCap: cfg.SignupDailyCap,
		MaxDevices: uint32(cfg.SignupMaxDevices), MaxPendingTasks: uint32(cfg.SignupMaxPendingTasks),
	})
	catalogSvc := catalog.NewService(vendorRepo, vendorOUIRepo, deviceModelRepo, paramMappingRepo, usageRepo, activityLogRepo)
	// webhookSvc dikonstruksi lebih dulu -- session & task memakainya lewat
	// domain.WebhookEnqueuer (interface sempit, tanpa import cycle), worker
	// dispatch-nya dijalankan runSweepers.
	webhookSvc := webhook.NewService(webhookSubRepo, webhookDeliveryRepo, refRepo, activityLogRepo, enc, logger).WithBlockPrivate(cfg.WebhookBlockPrivate)

	wsHub := ws.NewHub()
	go wsHub.Run()

	// Mode HA (ACS_HA=true, butuh Redis): event WebSocket di-fan-out antar-instance, rate limit dibagi
	// lintas instance, sweeper berjalan di satu instance per tick. Tanpa HA: perilaku satu instance.
	haOn := cfg.HA && redisClient != nil
	if cfg.HA && redisClient == nil {
		logger.Warn("ACS_HA=true tetapi Redis tidak tersambung: HA dinonaktifkan (perilaku satu instance)")
	}
	if haOn {
		wsHub.WithBus(context.Background(), redisClient.PubSub("acs:ws:events"))
		logger.Info("HA aktif: fan-out WebSocket, rate limit terdistribusi, kunci leader sweeper")
	}
	limiterStore := func(prefix string, rate float64) middleware.RateLimiterStore {
		mem := middleware.NewRateLimiterMemoryStore(rate)
		if !haOn {
			return mem
		}
		return redisClient.NewRateLimiter(prefix, rate, mem)
	}
	// lockSweeper: satu instance per tick. Redis bermasalah -> tetap jalan (sweeper idempoten).
	lockSweeper := func(name string, ttl time.Duration) (func(), bool) {
		if !haOn {
			return func() {}, true
		}
		release, ok, err := redisClient.TryLock(context.Background(), "sweeper:"+name, ttl)
		if err != nil {
			logger.Warn("HA: kunci leader gagal, sweeper tetap dijalankan", "name", name, "error", err)
			return func() {}, true
		}
		return release, ok
	}

	taskSvc := task.NewService(taskRepo, deviceRepo, deviceParamRepo, deviceModelRepo, paramMappingRepo, refRepo, activityLogRepo, tenantRepo, webhookSvc, wsHub).WithEncryptor(enc)
	// firmwareSvc dikonstruksi SEBELUM provisioningSvc -- provisioningSvc
	// (ZTP aksi FirmwareFileID, migrations/0009) bergantung pada firmwareSvc
	// lewat domain.FirmwareScheduler (interface sempit, menghindari import
	// cycle usecase/provisioning <-> usecase/firmware).
	firmwareSvc := firmware.NewService(firmwareFileRepo, firmwareJobRepo, firmwareRolloutRepo, deviceRepo, taskSvc, refRepo, activityLogRepo, objStorage, usageRepo)
	provisioningSvc := provisioning.NewService(profileRepo, profileParamRepo, ztRuleRepo, deviceRepo, deviceParamRepo, refRepo, taskSvc, firmwareSvc, activityLogRepo, presetRepo, presetAppRepo)
	deviceSvc := device.NewService(deviceRepo, vendorOUIRepo, deviceModelRepo, refRepo, deviceParamRepo, deviceEventRepo, opticalMetricRepo, configSnapshotRepo, enc, activityLogRepo, taskSvc, fileRepo, objStorage, mysql.NewOpenTaskCanceller(db))
	deviceSvc.WithCatalog(vendorRepo, tenantRepo).WithQuota(tenantQuotaRepo).WithCRTrustedCIDRs(cfg.CRTrustedCIDRs)
	diagnosticsSvc := diagnostics.NewService(diagnosticRepo, deviceRepo, taskSvc, activityLogRepo)
	sessionSvc := session.NewService(deviceSessionRepo, deviceEventRepo, deviceParamRepo, deviceRepo, tenantRepo, refRepo, deviceSvc, taskSvc, provisioningSvc, firmwareSvc, diagnosticsSvc, enc, activityLogRepo, logger, webhookSvc, wsHub).WithOpticalPoll(time.Duration(cfg.OpticalPollMinutes) * time.Minute).WithPPPoEPoll(time.Duration(cfg.PPPoEPollMinutes) * time.Minute)
	fileSvc := file.NewService(fileRepo, objStorage, activityLogRepo)
	tagSvc := tag.NewService(tagRepo, activityLogRepo)
	presetSvc := preset.NewService(presetRepo, activityLogRepo)
	selfServiceSvc := selfservice.NewService(userDeviceRepo, deviceRepo, refRepo, taskSvc, activityLogRepo).WithUsers(userRepo)

	// ---- Notifikasi persisten (in-app + email + push WebSocket per pengguna), migrations/0030 ----
	// Satu notifSvc dipasang ke service pemicu lewat domain.Notifier (nil-safe; kegagalan notifikasi tidak
	// pernah menggagalkan alur utama). Email hanya bila SMTP dikonfigurasi.
	notifSvc := notification.NewService(mysql.NewNotificationRepository(db), tenantRepo, activityLogRepo,
		notification.Config{OfflineRatio: cfg.NotifyOfflineRatio, FailedTasks: cfg.NotifyFailedTasks}, logger).WithPusher(wsHub)
	if cfg.SMTPHost != "" {
		notifSvc.WithEmail(mailNotifier, mailBuilder)
	}
	authSvc.WithNotifier(notifSvc)
	iamSvc.WithNotifier(notifSvc)
	signupSvc.WithNotifier(notifSvc)
	deviceSvc.WithNotifier(notifSvc)
	deviceSvc.WithFleetInsights(mysql.NewFleetInsightsRepository(db))
	sessionSvc.WithNotifier(notifSvc)
	firmwareSvc.WithNotifier(notifSvc)

	// ---- Langganan SaaS lewat Xendit (migrations/0032). Bawaan MATI (ACS_BILLING_ENABLED=false): klien gateway tidak
	// dibuat, endpoint mutasi menjawab 409 dan webhook 404. Kunci rahasia/token hanya dipegang klien & usecase —
	// tidak pernah dicatat di log. ----
	var paymentGW domain.PaymentGateway
	billingCfg := billing.Config{
		Enabled: cfg.BillingEnabled, CallbackToken: cfg.XenditCallbackToken,
		Invoice: time.Duration(cfg.BillingInvoiceHours) * time.Hour, PublicURL: cfg.PublicURL,
	}
	if cfg.BillingEnabled {
		xc, err := xendit.New(cfg.XenditSecretKey, cfg.XenditBaseURL)
		if err != nil {
			log.Fatalf("xendit: %v", err)
		}
		paymentGW = xc
		// Base URL non-bawaan (mis. sandbox/mock operator) ikut dipercaya sebagai host payment_url.
		if cfg.XenditBaseURL != xendit.DefaultBaseURL {
			if u, perr := url.Parse(cfg.XenditBaseURL); perr == nil && u.Hostname() != "" {
				billingCfg.AllowedPaymentHosts = []string{u.Hostname()}
			}
		}
		logger.Info("billing: pembayaran Xendit aktif", "base_url", cfg.XenditBaseURL, "invoice_hours", cfg.BillingInvoiceHours)
	}
	// Fitur Console (migrations/0033): layout menu, laporan bug (email ke operator), riwayat impor/ekspor (email ke pelaku).
	smtpOn := cfg.SMTPHost != ""
	menuSvc := console.NewMenuService(mysql.NewMenuLayoutRepository(db))
	bugSvc := console.NewBugService(mysql.NewBugReportRepository(db), activityLogRepo, userRepo, tenantRepo, cfg.BugReportEmail, logger).
		WithNotifier(notifSvc).WithEmail(mailNotifier, mailBuilder, smtpOn)
	transferSvc := console.NewTransferService(mysql.NewDataTransferRepository(db), activityLogRepo, userRepo, logger).
		WithEmail(mailNotifier, mailBuilder, smtpOn)
	billingSvc := billing.NewService(mysql.NewBillingRepository(db), tenantRepo, userRepo, paymentGW, activityLogRepo, billingCfg, logger).WithNotifier(notifSvc)
	if cfg.SMTPHost != "" {
		billingSvc.WithEmail(mailNotifier, mailBuilder)
	}

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
	// Strict-decode: field asing di body JSON ditolak 400 alih-alih diam-diam
	// diabaikan (audit R9, CHECKLIST_SAAS.md §D) — lihat
	// internal/delivery/http/strict_json.go. Endpoint CWMP/USP pakai XML/SOAP,
	// tidak lewat JSONSerializer ini (dipasang di cwmpEcho terpisah di bawah).
	restEcho.JSONSerializer = deliveryhttp.StrictJSONSerializer{}
	restEcho.Use(middleware.Recover())
	setClientIPExtractor(restEcho, cfg.TrustedProxies)
	restEcho.Use(redactingRequestLogger(logger))
	// IP & user agent klien ke konteks permintaan -> activity_logs.ip_address terisi.
	restEcho.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			req := c.Request()
			ctx := domain.WithRequestMeta(req.Context(), domain.RequestMeta{IP: c.RealIP(), UserAgent: req.UserAgent()})
			c.SetRequest(req.WithContext(ctx))
			return next(c)
		}
	})
	// Batas ukuran body: `c.Bind()` di /auth/login (anonim) & endpoint lain
	// membaca body ke memori tanpa batas -> satu POST raksasa = memory
	// exhaustion. 2 MB cukup longgar untuk semua payload JSON REST. Upload
	// firmware (multipart, bisa ratusan MB) DIKECUALIKAN — route itu punya
	// http.MaxBytesReader streaming sendiri (firmware_handler.go) yang dicek
	// dari Content-Length sebelum body dibaca.
	restEcho.Use(middleware.BodyLimitWithConfig(middleware.BodyLimitConfig{
		LimitBytes: 2 << 20, // 2 MiB
		Skipper: func(c *echo.Context) bool {
			// firmware & files: multipart besar, dibatasi streaming di handler masing-masing
			p := c.Request().URL.Path
			return c.Request().Method == http.MethodPost && (p == "/api/v1/firmware" || p == "/api/v1/files")
		},
	}))
	restEcho.Use(deliveryhttp.SecurityHeaders())
	restEcho.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: cfg.CORSAllowOrigins,
		AllowMethods: []string{http.MethodGet, http.MethodPost, http.MethodPatch, http.MethodPut, http.MethodDelete, http.MethodOptions},
		AllowHeaders: []string{"Authorization", "Content-Type"},
	}))
	// Rate limit per identifier (default: IP) — jauh lebih longgar dari CWMP
	// (5 req/s) karena satu halaman Console memuat belasan–puluhan permintaan dan
	// dashboard polling tiap beberapa detik per tab; beberapa pengguna satu kantor
	// berbagi satu IP (NAT). Default 100 req/s (ACS_REST_RATE_LIMIT). Brute-force
	// login/MFA/reset/signup dibatasi pembatas tersendiri per rute (jauh lebih ketat)
	// plus lockout akun, bukan oleh batas global ini.
	restEcho.Use(middleware.RateLimiter(limiterStore("rest", cfg.RESTRatePerIP)))
	router := &deliveryhttp.Router{
		Auth: authSvc, IAM: iamSvc, Devices: deviceSvc, Tasks: taskSvc, Provisioning: provisioningSvc,
		Firmware: firmwareSvc, Diagnostics: diagnosticsSvc, Reports: report.NewService(mysql.NewReportRepository(db)), Signup: signupSvc, Webhooks: webhookSvc, Sessions: sessionSvc,
		Catalog: catalogSvc,
		Vendors: vendorRepo, VendorOUIs: vendorOUIRepo, DeviceModels: deviceModelRepo, ParamMappings: paramMappingRepo,
		Refs: refRepo, Activity: activityLogRepo,
		MetricsHandler: metricsHandler,
		Files:          fileSvc,
		Tags:           tagSvc, Presets: presetSvc,
		SelfService:   selfServiceSvc,
		Notifications: notifSvc,
		Billing:       billingSvc,
		Menu:          menuSvc,
		Bugs:          bugSvc,
		Transfers:     transferSvc,
		WSHub:         wsHub,

		WSAllowedOrigins: cfg.CORSAllowOrigins,
		PublicURL:        cfg.PublicURL,
		LimiterStore:     limiterStore,
		Readiness: []deliveryhttp.ReadinessCheck{
			// DB kritis: tanpa DB ACS tak bisa melayani apa pun. Redis hanya cache sesi (opsional).
			{Name: "database", Critical: true, Fn: func(ctx context.Context) error { return db.PingContext(ctx) }},
			{Name: "redis", Critical: false, Fn: func(ctx context.Context) error {
				if redisClient == nil {
					return nil // tidak dikonfigurasi/gagal saat start: fitur nonaktif, bukan "down"
				}
				return redisClient.DB().Ping(ctx).Err()
			}},
		},
	}
	router.Register(restEcho)

	// ---- Endpoint CWMP (CPE) — server terpisah agar rate limiting/exposure
	// publik bisa diatur berbeda dari REST API internal (TECH.md §8). ----
	cwmpEcho := echo.New()
	cwmpEcho.Use(middleware.Recover())
	// Batas body CWMP: handler.go melakukan io.ReadAll pada endpoint yang
	// PRA-autentikasi (auth Inform dicek belakangan). 4 MiB jauh di atas
	// envelope CWMP terbesar yang wajar (Inform / GetParameterValuesResponse
	// ribuan parameter) tapi mencegah POST raksasa dari CPE nakal/anonim.
	setClientIPExtractor(cwmpEcho, cfg.TrustedProxies)
	cwmpEcho.Use(middleware.BodyLimit(4 << 20))
	// Rate limit per identifier (default: IP) — endpoint ini sekarang menjaga
	// shared secret Inform CWMP sungguhan (bukan cuma anti CPE nakal/loop
	// seperti sebelumnya), jadi juga jadi mitigasi brute-force kredensial.
	cwmpEcho.Use(middleware.RateLimiter(limiterStore("cwmp", cfg.CWMPRatePerIP)))
	deliverycwmp.NewHandler(sessionSvc, logger).WithStorage(objStorage).Register(cwmpEcho, "/cwmp")

	// TR-369 / USP WebSocket Handler (MTP)
	// Endpoint /usp masih kerangka & TANPA autentikasi di port publik CPE: default
	// mati; aktifkan eksplisit (ACS_USP_ENABLED=true) hanya untuk pengembangan.
	if cfg.USPEnabled {
		uspSessionSvc := usp_session.NewService(logger)
		deliveryusp.NewHandler(logger, uspSessionSvc).Mount(cwmpEcho)
		logger.Warn("USP/TR-369 diaktifkan: endpoint /usp TANPA autentikasi — jangan di produksi")
	}

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
	go runSweepers(deviceSvc, taskSvc, firmwareSvc, webhookSvc, sessionSvc, diagnosticsSvc, mysql.NewRetentionRepository(db), cfg.LogRetentionDays, signupSvc.Purge, notifSvc.Evaluate, billingSvc.Evaluate, lockSweeper, stop)

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

// setClientIPExtractor: tanpa proxy tepercaya IP klien = alamat socket (header
// X-Forwarded-For diabaikan). Dengan ACS_TRUSTED_PROXIES, XFF hanya dipercaya bila
// berasal dari proxy yang terdaftar.
func setClientIPExtractor(e *echo.Echo, proxies []*net.IPNet) {
	if len(proxies) == 0 {
		e.IPExtractor = echo.ExtractIPDirect()
		return
	}
	opts := []echo.TrustOption{echo.TrustLoopback(false), echo.TrustLinkLocal(false), echo.TrustPrivateNet(false)}
	for _, n := range proxies {
		opts = append(opts, echo.TrustIPRange(n))
	}
	e.IPExtractor = echo.ExtractIPFromXFFHeader(opts...)
}

// sensitiveQueryParams — nilai query yang tak boleh masuk log akses (token
// sesi, kode/state OIDC, token reset).
var sensitiveQueryParams = []string{"token", "code", "state", "access_token", "id_token", "password"}

// redactURI menyamarkan nilai query sensitif pada URI yang dicatat.
func redactURI(uri string) string {
	i := strings.IndexByte(uri, '?')
	if i < 0 {
		return uri
	}
	q, err := url.ParseQuery(uri[i+1:])
	if err != nil {
		return uri[:i] + "?[query tidak dapat dibaca]"
	}
	for _, k := range sensitiveQueryParams {
		if q.Has(k) {
			q.Set(k, "***")
		}
	}
	return uri[:i] + "?" + q.Encode()
}

// redactingRequestLogger = RequestLogger bawaan Echo, tetapi URI-nya disamarkan
// (JWT/kode OIDC pernah tercatat polos di access log lewat ?token=).
func redactingRequestLogger(logger *slog.Logger) echo.MiddlewareFunc {
	return middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogLatency: true, LogRemoteIP: true, LogHost: true, LogMethod: true, LogURI: true,
		LogUserAgent: true, LogStatus: true, LogContentLength: true, LogResponseSize: true, LogRequestID: true,
		HandleError: true,
		LogValuesFunc: func(c *echo.Context, v middleware.RequestLoggerValues) error {
			attrs := []any{
				"method", v.Method, "uri", redactURI(v.URI), "status", v.Status, "latency", v.Latency.String(),
				"host", v.Host, "remote_ip", v.RemoteIP, "user_agent", v.UserAgent, "request_id", v.RequestID,
			}
			if v.Error != nil {
				logger.Error("REQUEST_ERROR", append(attrs, "error", v.Error.Error())...)
			} else {
				logger.Info("REQUEST", attrs...)
			}
			return nil
		},
	})
}

// runSweepers menjalankan tugas periodik ringan (FR-22, timeout task, wave
// rollout firmware, dispatch webhook) — aman dijalankan dari instance manapun
// karena app server stateless (TECH.md §9). Dispatch webhook pakai ticker
// terpisah yang lebih cepat (15 dtk) supaya event fault/value-change sampai
// ke sistem pihak ketiga dengan latensi rendah.
func runSweepers(deviceSvc *device.Service, taskSvc *task.Service, firmwareSvc *firmware.Service, webhookSvc *webhook.Service, sessionSvc *session.Service, diagSvc *diagnostics.Service, retention domain.RetentionRepository, retentionDays int, signupPurge func(context.Context) (int64, error), notifyEval func(context.Context) (int, error), billingEval func(context.Context) (int, error), lock func(name string, ttl time.Duration) (func(), bool), stop <-chan struct{}) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	// Pemangkasan log: tiap 6 jam (data menahun dihapus bertahap per batch).
	retentionTicker := time.NewTicker(6 * time.Hour)
	defer retentionTicker.Stop()
	webhookTicker := time.NewTicker(15 * time.Second)
	defer webhookTicker.Stop()
	// Evaluator notifikasi (lonjakan perangkat offline, task gagal beruntun): tiap 5 menit, satu instance per tick.
	notifyTicker := time.NewTicker(5 * time.Minute)
	defer notifyTicker.Stop()
	// Evaluator langganan (invoice kedaluwarsa, pengingat masa berlaku): tiap jam. Tidak memutus layanan otomatis.
	billingTicker := time.NewTicker(1 * time.Hour)
	defer billingTicker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-billingTicker.C:
			release, ok := lock("billing", 50*time.Minute)
			if !ok {
				continue // instance lain yang mengevaluasi tick ini
			}
			if n, err := billingEval(context.Background()); err != nil {
				log.Printf("sweeper: evaluasi langganan gagal: %v", err)
			} else if n > 0 {
				log.Printf("sweeper: %d tindakan langganan diproses evaluator", n)
			}
			release()
		case <-notifyTicker.C:
			release, ok := lock("notify", 4*time.Minute)
			if !ok {
				continue // instance lain yang mengevaluasi tick ini
			}
			if n, err := notifyEval(context.Background()); err != nil {
				log.Printf("sweeper: evaluasi notifikasi gagal: %v", err)
			} else if n > 0 {
				log.Printf("sweeper: %d notifikasi dibuat oleh evaluator", n)
			}
			release()
		case <-retentionTicker.C:
			release, ok := lock("retention", 30*time.Minute)
			if !ok {
				continue // instance lain yang memangkas log kali ini
			}
			if retentionDays > 0 {
				cutoff := time.Now().AddDate(0, 0, -retentionDays)
				if res, err := retention.PruneLogs(context.Background(), cutoff); err != nil {
					log.Printf("sweeper: retensi log gagal: %v", err)
				} else {
					log.Printf("sweeper: retensi log (> %d hari) dipangkas: %v", retentionDays, res)
				}
			}
			// Antrean verifikasi pendaftaran lama (kedaluwarsa > 7 hari / terverifikasi > 30 hari) dihapus.
			if n, err := signupPurge(context.Background()); err != nil {
				log.Printf("sweeper: pangkas pendaftaran kedaluwarsa gagal: %v", err)
			} else if n > 0 {
				log.Printf("sweeper: %d pendaftaran tenant lama dipangkas", n)
			}
			release()
		case <-webhookTicker.C:
			if n, err := webhookSvc.DispatchDue(context.Background()); err != nil {
				log.Printf("sweeper: dispatch webhook gagal: %v", err)
			} else if n > 0 {
				log.Printf("sweeper: %d webhook delivery diproses", n)
			}
		case <-ticker.C:
			release, ok := lock("minute", 50*time.Second)
			if !ok {
				continue // instance lain yang menjalankan sweeper tick ini
			}
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
			if n, err := diagSvc.ExpireStale(ctx); err != nil {
				log.Printf("sweeper: kedaluwarsa diagnostik gagal: %v", err)
			} else if n > 0 {
				log.Printf("sweeper: %d diagnostik menggantung ditandai gagal", n)
			}
			// Reap sesi CWMP yang ditinggalkan CPE tanpa POST-kosong penutup.
			// Ambang 15 menit: jauh di atas durasi sesi CWMP normal (detik s/d
			// beberapa menit) sehingga tidak pernah menutup sesi yang hidup,
			// tapi cukup cepat agar metrik acs_cwmp_sessions_open akurat.
			if n, err := sessionSvc.TimeoutStaleSessions(ctx, 15*time.Minute); err != nil {
				log.Printf("sweeper: reap sesi CWMP basi gagal: %v", err)
			} else if n > 0 {
				log.Printf("sweeper: %d sesi CWMP basi di-reap", n)
			}
			// Cek wave rollout firmware yang sudah tuntas & lanjutkan ke wave
			// berikutnya (atau pause/complete) -- lihat firmware.Service.AdvanceRollout.
			if n, err := firmwareSvc.SweepRolloutBatches(ctx); err != nil {
				log.Printf("sweeper: sweep firmware rollout batch gagal: %v", err)
			} else if n > 0 {
				log.Printf("sweeper: %d firmware rollout batch diproses", n)
			}
			release()
		}
	}
}
