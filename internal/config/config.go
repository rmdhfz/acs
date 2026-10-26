// Package config memuat konfigurasi aplikasi dari environment variable.
package config

import (
	"encoding/base64"
	"fmt"
	"net"
	"net/mail"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"acs/pkg/netguard"
)

type Config struct {
	HTTPAddr           string
	CWMPAddr           string
	DBDSN              string
	JWTSecret          []byte
	JWTExpiry          time.Duration
	CredentialEncKey   []byte
	SessionIdleTimeout time.Duration
	// CORSAllowOrigins adalah origin frontend yang diizinkan memanggil REST
	// API dari browser (mis. http://localhost:5173 saat dev). Default dev
	// mengizinkan localhost Vite; production wajib override eksplisit.
	CORSAllowOrigins []string
	// MinIO/S3-compatible object storage untuk firmware (ROADMAP.md Fase 2).
	// Endpoint & kredensial WAJIB dari env, tidak ada default hardcoded —
	// hanya nama bucket yang boleh punya default konstanta.
	MinIOEndpoint  string
	MinIOAccessKey string
	MinIOSecretKey string
	MinIOBucket    string
	MinIOUseSSL    bool
	// LogLevel — level minimum log/slog terstruktur (DEBUG/INFO/WARN/ERROR,
	// default INFO). Dipakai cmd/acsd utk konfigurasi slog.Logger tunggal yang
	// dipakai bersama delivery/cwmp & usecase/session (observability TECH.md
	// §10) — supaya production bisa menaikkan verbosity (DEBUG, per-RPC) saat
	// investigasi tanpa redeploy kode.
	LogLevel  string
	RedisAddr string
	// RedisPassword — kosong = Redis tanpa auth (hanya untuk jaringan tertutup).
	RedisPassword string

	// TrustedProxies — CIDR/IP reverse proxy/LB yang boleh menentukan IP klien lewat
	// X-Forwarded-For. Kosong (default) = IP klien = alamat socket (header
	// X-Forwarded-For diabaikan, tidak bisa dipalsukan). WAJIB diisi bila ACS di
	// belakang proxy: tanpa itu semua klien terlihat satu IP (rate limit bersama,
	// ip_address device salah, Connection Request URL tak tertangkap).
	TrustedProxies []*net.IPNet

	// CRTrustedCIDRs — rentang IP yang boleh menjadi host ConnectionRequestURL walau BUKAN alamat sumber Inform.
	// Kosong (default) = ketat: host harus sama dengan IP sumber Inform (anti-SSRF). Diisi bila IP sumber tidak mewakili
	// CPE (ACS di balik NAT/port-forward, mis. Docker Desktop: semua CPE terlihat 172.x) sementara URL yang dilaporkan CPE
	// berada di jaringan manajemen yang dikenal (mis. 10.0.0.0/8). Tetap hanya literal IP dan lolos netguard (loopback,
	// link-local/metadata, dst. ditolak) — tidak pernah nama DNS.
	CRTrustedCIDRs []*net.IPNet

	// USPEnabled — mengaktifkan endpoint WebSocket /usp (TR-369) yang masih
	// kerangka & tanpa autentikasi. Default MATI.
	USPEnabled bool

	// CWMPRatePerIP — batas request/detik PER IP pada endpoint CWMP (default 5). Naikkan bila banyak
	// CPE berbagi satu IP publik (CGNAT/kantor) atau untuk uji beban; jangan dimatikan di produksi
	// (juga mitigasi brute-force shared secret Inform).
	CWMPRatePerIP float64

	// RESTRatePerIP — batas request/detik PER IP pada REST API (default 100). Satu halaman Console memuat
	// 15–35 permintaan sekaligus dan beberapa pengguna satu kantor berbagi satu IP publik (NAT); 30/dtk lama
	// memicu 429 ("tidak dapat menjangkau server") pada pemakaian normal. Endpoint sensitif (login, MFA,
	// lupa/reset password, signup) punya pembatas laju sendiri yang jauh lebih ketat dan tidak terpengaruh.
	RESTRatePerIP float64

	// WebhookBlockPrivate — tolak target webhook di jaringan privat (RFC1918/ULA/CGNAT). Aktifkan pada
	// SaaS publik di mana admin tenant tidak tepercaya; matikan bila receiver webhook ada di LAN yang sama.
	WebhookBlockPrivate bool

	// TransferAllowedHosts — host yang boleh menjadi URL task Download/Upload TR-069 yang dibuat lewat API (nama host,
	// *.subdomain, IP, atau CIDR). nil (default) = tidak dibatasi. Mendorong firmware/konfigurasi dari host sembarang
	// adalah risiko bila token ADMIN tenant bocor; unduhan dari katalog firmware/berkas ACS (URL presigned) tidak terkena.
	TransferAllowedHosts *netguard.HostAllowlist

	// HA — mode multi-instance (beberapa acsd di balik load balancer, berbagi MariaDB + Redis): rate limit
	// terdistribusi, fan-out WebSocket antar-instance, dan kunci leader untuk sweeper. Butuh Redis.
	// Mati (bawaan) = perilaku satu instance seperti sebelumnya.
	HA bool

	// Pendaftaran tenant mandiri (bawaan MATI). Butuh SMTP (email verifikasi). Tenant baru mulai dengan batas paket awal.
	SelfSignup            bool
	SignupDailyCap        int  // plafon pendaftaran per 24 jam di seluruh platform
	SignupMaxDevices      uint // batas perangkat awal tenant baru (0 = tak dibatasi)
	SignupMaxPendingTasks uint // batas antrean task awal (0 = tak dibatasi)

	// LogRetentionDays — umur maksimum (hari) tabel log volume tinggi (event, sesi CWMP,
	// metrik optik, riwayat webhook). 0 = tanpa pemangkasan. activity_logs (audit) tidak dipangkas.
	LogRetentionDays int

	// NotifyOfflineRatio — ambang rasio perangkat OFFLINE (0..1) untuk notifikasi DEVICE_OFFLINE_SPIKE
	// (berlaku bila tenant punya >= 10 perangkat). Bawaan 0,30. ACS_NOTIFY_OFFLINE_RATIO.
	NotifyOfflineRatio float64
	// NotifyFailedTasks — jumlah task FAILED dalam 30 menit terakhir yang memicu notifikasi TASK_FAILURES.
	// Bawaan 5. ACS_NOTIFY_FAILED_TASKS.
	NotifyFailedTasks int

	// OpticalPollMinutes — interval polling redaman optik per perangkat (menit); 0 = nonaktif.
	// Hanya berlaku untuk vendor/model yang punya mapping optical.* di katalog.
	OpticalPollMinutes int

	// PPPoEPollMinutes — interval penyegaran username PPPoE per perangkat (menit); 0 = nonaktif. ACS_PPPOE_POLL_MINUTES.
	PPPoEPollMinutes int

	OIDCIssuer       string
	OIDCClientID     string
	OIDCClientSecret string
	OIDCRedirectURL  string
	// OIDCAllowedDomains — bila diisi, hanya email berdomain ini yang boleh masuk lewat SSO (mis. Google Workspace).
	OIDCAllowedDomains []string

	// Email (SMTP) untuk reset password & notifikasi keamanan. SMTPHost kosong
	// = email nonaktif (fitur reset lewat email tidak mengirim apa pun; sisa
	// sistem tetap jalan). Kredensial WAJIB dari env — jangan di-commit.
	SMTPHost     string
	SMTPPort     int
	SMTPUsername string
	SMTPPassword string
	SMTPFrom     string
	SMTPFromName string
	// BugReportEmail — penerima email laporan bug dari Console (bawaan operator platform). Kosong (env diisi kosong)
	// = tidak ada email; laporan tetap tersimpan di database.
	BugReportEmail string
	// PublicURL — origin frontend yang dipakai membangun tautan di email
	// (mis. https://acs.example.com). Tanpa slash akhir.
	PublicURL string
	// PasswordResetTTL — masa berlaku tautan reset password.
	PasswordResetTTL time.Duration

	// Pembayaran langganan SaaS lewat Xendit (Invoice API). Bawaan MATI. Bila BillingEnabled, kunci rahasia dan
	// token callback WAJIB (config menolak start tanpa itu). Keduanya rahasia: tidak boleh dicetak di log/respons.
	BillingEnabled bool
	// XenditSecretKey — kunci rahasia API Xendit (HTTP Basic: username = kunci, password kosong).
	XenditSecretKey string
	// XenditCallbackToken — token verifikasi callback (header X-CALLBACK-TOKEN) yang diatur di dashboard Xendit.
	XenditCallbackToken string
	// XenditBaseURL — dasar URL API (bawaan https://api.xendit.co); berguna untuk tes. Harus https kecuali loopback.
	XenditBaseURL string
	// BillingInvoiceHours — masa berlaku invoice sebelum kedaluwarsa (jam, 1..168).
	BillingInvoiceHours int
	// DBMaxOpenConns/DBMaxIdleConns/DBConnMaxLifetimeMinutes — ukuran connection
	// pool mysql.Connect(). Bawaan (50/10/30) dipertahankan sama seperti sebelum
	// dikonfigurasi; load test 2026-10-18 menemukan throughput plateau ~450-480
	// sesi/dtk terlepas dari concurrency krn 50 koneksi hardcoded jadi bottleneck
	// (CPU acsd/MariaDB masih longgar saat idle pasca-tes) — lihat
	// docs/laporan/PROGRESS.md LANJUTAN 14. Operator produksi/HA yang perlu
	// menaikkannya kini bisa lewat env tanpa rebuild.
	DBMaxOpenConns           int
	DBMaxIdleConns           int
	DBConnMaxLifetimeMinutes int
}

func Load() (*Config, error) {
	cfg := &Config{
		HTTPAddr: getEnv("ACS_HTTP_ADDR", ":8080"),
		CWMPAddr: getEnv("ACS_CWMP_ADDR", ":7547"),
		DBDSN:    os.Getenv("ACS_DB_DSN"),
	}
	if cfg.DBDSN == "" {
		return nil, fmt.Errorf("config: ACS_DB_DSN wajib diisi")
	}
	var err error
	if cfg.DBMaxOpenConns, err = strconv.Atoi(getEnv("ACS_DB_MAX_OPEN_CONNS", "50")); err != nil || cfg.DBMaxOpenConns < 1 || cfg.DBMaxOpenConns > 10000 {
		return nil, fmt.Errorf("config: ACS_DB_MAX_OPEN_CONNS harus bilangan 1..10000")
	}
	if cfg.DBMaxIdleConns, err = strconv.Atoi(getEnv("ACS_DB_MAX_IDLE_CONNS", "10")); err != nil || cfg.DBMaxIdleConns < 0 || cfg.DBMaxIdleConns > cfg.DBMaxOpenConns {
		return nil, fmt.Errorf("config: ACS_DB_MAX_IDLE_CONNS harus bilangan 0..ACS_DB_MAX_OPEN_CONNS")
	}
	if cfg.DBConnMaxLifetimeMinutes, err = strconv.Atoi(getEnv("ACS_DB_CONN_MAX_LIFETIME_MINUTES", "30")); err != nil || cfg.DBConnMaxLifetimeMinutes < 1 || cfg.DBConnMaxLifetimeMinutes > 1440 {
		return nil, fmt.Errorf("config: ACS_DB_CONN_MAX_LIFETIME_MINUTES harus bilangan 1..1440")
	}

	cfg.JWTSecret = []byte(os.Getenv("ACS_JWT_SECRET"))
	if len(cfg.JWTSecret) < 32 {
		return nil, fmt.Errorf("config: ACS_JWT_SECRET wajib diisi minimal 32 karakter")
	}
	if weakSecret(cfg.JWTSecret) {
		return nil, fmt.Errorf("config: ACS_JWT_SECRET lemah/placeholder (mis. salinan .env.example) — buat acak: openssl rand -hex 32")
	}

	expiry, err := time.ParseDuration(getEnv("ACS_JWT_EXPIRY", "8h"))
	if err != nil {
		return nil, fmt.Errorf("config: ACS_JWT_EXPIRY tidak valid: %w", err)
	}
	cfg.JWTExpiry = expiry

	keyB64 := os.Getenv("ACS_CREDENTIAL_ENC_KEY")
	if keyB64 == "" {
		return nil, fmt.Errorf("config: ACS_CREDENTIAL_ENC_KEY wajib diisi (base64, 32 byte)")
	}
	key, err := base64.StdEncoding.DecodeString(keyB64)
	if err != nil {
		return nil, fmt.Errorf("config: ACS_CREDENTIAL_ENC_KEY bukan base64 valid: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("config: ACS_CREDENTIAL_ENC_KEY harus 32 byte setelah decode base64 (dapat %d byte)", len(key))
	}
	cfg.CredentialEncKey = key

	idleTimeout, err := time.ParseDuration(getEnv("ACS_SESSION_IDLE_TIMEOUT", "30s"))
	if err != nil {
		return nil, fmt.Errorf("config: ACS_SESSION_IDLE_TIMEOUT tidak valid: %w", err)
	}
	cfg.SessionIdleTimeout = idleTimeout

	origins := getEnv("ACS_CORS_ALLOW_ORIGINS", "http://localhost:5173,http://127.0.0.1:5173")
	for _, o := range strings.Split(origins, ",") {
		o = strings.TrimSpace(o)
		if o != "" {
			if !strings.HasPrefix(o, "http://") && !strings.HasPrefix(o, "https://") {
				return nil, fmt.Errorf("config: origin CORS tidak valid (harus http/https): %s", o)
			}
			cfg.CORSAllowOrigins = append(cfg.CORSAllowOrigins, o)
		}
	}
	// Echo middleware.CORSConfig menganggap AllowOrigins kosong sebagai
	// "izinkan semua origin" (*), yang merupakan risiko keamanan besar bila
	// ada salah konfigurasi. Pastikan setidaknya localhost diizinkan bila
	// ternyata origin ter-parse kosong.
	if len(cfg.CORSAllowOrigins) == 0 {
		cfg.CORSAllowOrigins = []string{"http://localhost:5173"}
	}

	cfg.MinIOEndpoint = os.Getenv("ACS_MINIO_ENDPOINT")
	if cfg.MinIOEndpoint == "" {
		return nil, fmt.Errorf("config: ACS_MINIO_ENDPOINT wajib diisi")
	}
	cfg.MinIOAccessKey = os.Getenv("ACS_MINIO_ACCESS_KEY")
	if cfg.MinIOAccessKey == "" {
		return nil, fmt.Errorf("config: ACS_MINIO_ACCESS_KEY wajib diisi")
	}
	cfg.MinIOSecretKey = os.Getenv("ACS_MINIO_SECRET_KEY")
	if cfg.MinIOSecretKey == "" {
		return nil, fmt.Errorf("config: ACS_MINIO_SECRET_KEY wajib diisi")
	}
	cfg.MinIOBucket = getEnv("ACS_MINIO_BUCKET", "acs-firmware")
	cfg.LogLevel = getEnv("ACS_LOG_LEVEL", "INFO")

	useSSL, err := strconv.ParseBool(getEnv("ACS_MINIO_USE_SSL", "false"))
	if err != nil {
		return nil, fmt.Errorf("config: ACS_MINIO_USE_SSL tidak valid: %w", err)
	}
	cfg.MinIOUseSSL = useSSL

	cfg.RedisAddr = getEnv("ACS_REDIS_ADDR", "localhost:6379")
	cfg.RedisPassword = os.Getenv("ACS_REDIS_PASSWORD")

	for _, p := range strings.Split(os.Getenv("ACS_TRUSTED_PROXIES"), ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if !strings.Contains(p, "/") {
			if ip := net.ParseIP(p); ip != nil {
				if ip.To4() != nil {
					p += "/32"
				} else {
					p += "/128"
				}
			}
		}
		_, n, perr := net.ParseCIDR(p)
		if perr != nil {
			return nil, fmt.Errorf("config: ACS_TRUSTED_PROXIES tidak valid (%q): harus IP atau CIDR", p)
		}
		cfg.TrustedProxies = append(cfg.TrustedProxies, n)
	}
	for _, p := range strings.Split(os.Getenv("ACS_CR_TRUSTED_CIDRS"), ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if !strings.Contains(p, "/") {
			if ip := net.ParseIP(p); ip != nil {
				if ip.To4() != nil {
					p += "/32"
				} else {
					p += "/128"
				}
			}
		}
		_, n, perr := net.ParseCIDR(p)
		if perr != nil {
			return nil, fmt.Errorf("config: ACS_CR_TRUSTED_CIDRS tidak valid (%q): harus IP atau CIDR", p)
		}
		cfg.CRTrustedCIDRs = append(cfg.CRTrustedCIDRs, n)
	}
	uspEnabled, err := strconv.ParseBool(getEnv("ACS_USP_ENABLED", "false"))
	if err != nil {
		return nil, fmt.Errorf("config: ACS_USP_ENABLED tidak valid: %w", err)
	}
	cfg.USPEnabled = uspEnabled
	cfg.SelfSignup, err = strconv.ParseBool(getEnv("ACS_SELF_SIGNUP", "false"))
	if err != nil {
		return nil, fmt.Errorf("config: ACS_SELF_SIGNUP tidak valid: %w", err)
	}
	if cfg.SignupDailyCap, err = strconv.Atoi(getEnv("ACS_SIGNUP_DAILY_CAP", "50")); err != nil || cfg.SignupDailyCap < 1 || cfg.SignupDailyCap > 100000 {
		return nil, fmt.Errorf("config: ACS_SIGNUP_DAILY_CAP harus 1..100000")
	}
	if cfg.SignupMaxDevices, err = parseUintEnv("ACS_SIGNUP_MAX_DEVICES", "25"); err != nil {
		return nil, err
	}
	if cfg.SignupMaxPendingTasks, err = parseUintEnv("ACS_SIGNUP_MAX_PENDING_TASKS", "50"); err != nil {
		return nil, err
	}
	cfg.HA, err = strconv.ParseBool(getEnv("ACS_HA", "false"))
	if err != nil {
		return nil, fmt.Errorf("config: ACS_HA tidak valid: %w", err)
	}
	cfg.WebhookBlockPrivate, err = strconv.ParseBool(getEnv("ACS_WEBHOOK_BLOCK_PRIVATE", "false"))
	if err != nil {
		return nil, fmt.Errorf("config: ACS_WEBHOOK_BLOCK_PRIVATE tidak valid: %w", err)
	}
	if cfg.TransferAllowedHosts, err = netguard.ParseHostAllowlist(os.Getenv("ACS_TRANSFER_ALLOWED_HOSTS")); err != nil {
		return nil, fmt.Errorf("config: ACS_TRANSFER_ALLOWED_HOSTS tidak valid: %w", err)
	}
	cfg.CWMPRatePerIP, err = strconv.ParseFloat(getEnv("ACS_CWMP_RATE_LIMIT", "5"), 64)
	if err != nil || cfg.CWMPRatePerIP < 1 || cfg.CWMPRatePerIP > 1_000_000 {
		return nil, fmt.Errorf("config: ACS_CWMP_RATE_LIMIT harus angka 1..1000000 (request/detik per IP)")
	}
	cfg.RESTRatePerIP, err = strconv.ParseFloat(getEnv("ACS_REST_RATE_LIMIT", "100"), 64)
	if err != nil || cfg.RESTRatePerIP < 5 || cfg.RESTRatePerIP > 1_000_000 {
		return nil, fmt.Errorf("config: ACS_REST_RATE_LIMIT harus angka 5..1000000 (request/detik per IP)")
	}
	cfg.NotifyOfflineRatio, err = strconv.ParseFloat(getEnv("ACS_NOTIFY_OFFLINE_RATIO", "0.30"), 64)
	if err != nil || cfg.NotifyOfflineRatio <= 0 || cfg.NotifyOfflineRatio > 1 {
		return nil, fmt.Errorf("config: ACS_NOTIFY_OFFLINE_RATIO harus angka di atas 0 sampai 1 (mis. 0.30)")
	}
	cfg.NotifyFailedTasks, err = strconv.Atoi(getEnv("ACS_NOTIFY_FAILED_TASKS", "5"))
	if err != nil || cfg.NotifyFailedTasks < 1 || cfg.NotifyFailedTasks > 100000 {
		return nil, fmt.Errorf("config: ACS_NOTIFY_FAILED_TASKS harus bilangan 1..100000")
	}
	cfg.OpticalPollMinutes, err = strconv.Atoi(getEnv("ACS_OPTICAL_POLL_MINUTES", "360"))
	if err != nil || cfg.OpticalPollMinutes < 0 || (cfg.OpticalPollMinutes > 0 && cfg.OpticalPollMinutes < 15) {
		return nil, fmt.Errorf("config: ACS_OPTICAL_POLL_MINUTES harus 0 (nonaktif) atau >= 15")
	}
	cfg.PPPoEPollMinutes, err = strconv.Atoi(getEnv("ACS_PPPOE_POLL_MINUTES", "1440"))
	if err != nil || cfg.PPPoEPollMinutes < 0 || (cfg.PPPoEPollMinutes > 0 && cfg.PPPoEPollMinutes < 60) {
		return nil, fmt.Errorf("config: ACS_PPPOE_POLL_MINUTES harus 0 (nonaktif) atau >= 60")
	}
	cfg.LogRetentionDays, err = strconv.Atoi(getEnv("ACS_LOG_RETENTION_DAYS", "90"))
	if err != nil || cfg.LogRetentionDays < 0 || (cfg.LogRetentionDays > 0 && cfg.LogRetentionDays < 7) {
		return nil, fmt.Errorf("config: ACS_LOG_RETENTION_DAYS harus 0 (nonaktif) atau >= 7")
	}

	cfg.OIDCIssuer = os.Getenv("ACS_OIDC_ISSUER")
	cfg.OIDCClientID = os.Getenv("ACS_OIDC_CLIENT_ID")
	cfg.OIDCClientSecret = os.Getenv("ACS_OIDC_CLIENT_SECRET")
	cfg.OIDCRedirectURL = getEnv("ACS_OIDC_REDIRECT_URL", "http://localhost:8080/api/v1/auth/oidc/callback")
	for _, d := range strings.Split(os.Getenv("ACS_OIDC_ALLOWED_DOMAINS"), ",") {
		if d = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(d), "@"))); d != "" {
			cfg.OIDCAllowedDomains = append(cfg.OIDCAllowedDomains, d)
		}
	}

	cfg.SMTPHost = os.Getenv("ACS_SMTP_HOST")
	smtpPort, err := strconv.Atoi(getEnv("ACS_SMTP_PORT", "587"))
	if err != nil || smtpPort < 1 || smtpPort > 65535 {
		return nil, fmt.Errorf("config: ACS_SMTP_PORT tidak valid")
	}
	cfg.SMTPPort = smtpPort
	cfg.SMTPUsername = os.Getenv("ACS_SMTP_USERNAME")
	cfg.SMTPPassword = os.Getenv("ACS_SMTP_PASSWORD")
	cfg.SMTPFrom = getEnv("ACS_SMTP_FROM", cfg.SMTPUsername)
	cfg.SMTPFromName = getEnv("ACS_SMTP_FROM_NAME", "ACS Console")
	cfg.BugReportEmail = DefaultBugReportEmail
	if v, ok := os.LookupEnv("ACS_BUG_REPORT_EMAIL"); ok {
		cfg.BugReportEmail = strings.TrimSpace(v)
	}
	if cfg.BugReportEmail != "" {
		if a, err := mail.ParseAddress(cfg.BugReportEmail); err != nil || a.Address != cfg.BugReportEmail {
			return nil, fmt.Errorf("config: ACS_BUG_REPORT_EMAIL bukan alamat email polos yang valid")
		}
	}
	if cfg.SelfSignup && cfg.SMTPHost == "" {
		return nil, fmt.Errorf("config: ACS_SELF_SIGNUP=true butuh email (ACS_SMTP_HOST) untuk verifikasi pendaftaran")
	}
	if cfg.SMTPHost != "" && cfg.SMTPFrom == "" {
		return nil, fmt.Errorf("config: ACS_SMTP_FROM (atau ACS_SMTP_USERNAME) wajib diisi bila ACS_SMTP_HOST diisi")
	}

	cfg.PublicURL = strings.TrimRight(getEnv("ACS_PUBLIC_URL", "http://localhost:5173"), "/")
	if !strings.HasPrefix(cfg.PublicURL, "http://") && !strings.HasPrefix(cfg.PublicURL, "https://") {
		return nil, fmt.Errorf("config: ACS_PUBLIC_URL harus diawali http:// atau https://")
	}
	resetTTL, err := time.ParseDuration(getEnv("ACS_PASSWORD_RESET_TTL", "30m"))
	if err != nil || resetTTL < time.Minute || resetTTL > 24*time.Hour {
		return nil, fmt.Errorf("config: ACS_PASSWORD_RESET_TTL tidak valid (1m–24h)")
	}
	cfg.PasswordResetTTL = resetTTL

	if err := loadBilling(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// minXenditTokenLen — panjang minimum token callback (cegah token tebakan).
const minXenditTokenLen = 24

// loadBilling memuat konfigurasi pembayaran. Pesan galat TIDAK PERNAH memuat nilai kunci/token.
func loadBilling(cfg *Config) error {
	var err error
	if cfg.BillingEnabled, err = strconv.ParseBool(getEnv("ACS_BILLING_ENABLED", "false")); err != nil {
		return fmt.Errorf("config: ACS_BILLING_ENABLED tidak valid: harus true/false")
	}
	cfg.XenditSecretKey = os.Getenv("ACS_XENDIT_SECRET_KEY")
	cfg.XenditCallbackToken = os.Getenv("ACS_XENDIT_CALLBACK_TOKEN")
	cfg.XenditBaseURL = strings.TrimRight(getEnv("ACS_XENDIT_BASE_URL", "https://api.xendit.co"), "/")
	if cfg.BillingInvoiceHours, err = strconv.Atoi(getEnv("ACS_BILLING_INVOICE_HOURS", "24")); err != nil || cfg.BillingInvoiceHours < 1 || cfg.BillingInvoiceHours > 168 {
		return fmt.Errorf("config: ACS_BILLING_INVOICE_HOURS harus bilangan 1..168")
	}
	if !cfg.BillingEnabled {
		return nil
	}
	if cfg.XenditSecretKey == "" || cfg.XenditCallbackToken == "" {
		return fmt.Errorf("config: ACS_BILLING_ENABLED=true wajib ACS_XENDIT_SECRET_KEY dan ACS_XENDIT_CALLBACK_TOKEN")
	}
	for name, v := range map[string]string{"ACS_XENDIT_SECRET_KEY": cfg.XenditSecretKey, "ACS_XENDIT_CALLBACK_TOKEN": cfg.XenditCallbackToken} {
		if strings.TrimSpace(v) != v || strings.ContainsAny(v, " \t\r\n") {
			return fmt.Errorf("config: %s tidak boleh memuat spasi", name)
		}
		if weakSecret([]byte(v)) {
			return fmt.Errorf("config: %s lemah/placeholder (mis. salinan contoh konfigurasi)", name)
		}
	}
	if len(cfg.XenditCallbackToken) < minXenditTokenLen {
		return fmt.Errorf("config: ACS_XENDIT_CALLBACK_TOKEN minimal %d karakter", minXenditTokenLen)
	}
	u, perr := url.Parse(cfg.XenditBaseURL)
	if perr != nil || u.Host == "" || u.User != nil || (u.Scheme != "https" && !(u.Scheme == "http" && isLoopbackHost(u.Hostname()))) {
		return fmt.Errorf("config: ACS_XENDIT_BASE_URL tidak valid (harus https://, atau http:// hanya untuk localhost)")
	}
	return nil
}

func isLoopbackHost(h string) bool {
	if strings.EqualFold(h, "localhost") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

// weakSecret menolak secret placeholder dari contoh konfigurasi dan secret berentropi rendah. JWT yang
// ditandatangani secret yang dapat ditebak bisa dipalsukan (mis. peran SUPERADMIN).
func weakSecret(b []byte) bool {
	low := strings.ToLower(string(b))
	for _, bad := range []string{"change-me", "changeme", "example", "your-secret", "secret-key", "password", "contoh"} {
		if strings.Contains(low, bad) {
			return true
		}
	}
	distinct := map[byte]struct{}{}
	for _, c := range b {
		distinct[c] = struct{}{}
	}
	return len(distinct) < 10
}

func parseUintEnv(name, def string) (uint, error) {
	v, err := strconv.ParseUint(getEnv(name, def), 10, 32)
	if err != nil || v > 10_000_000 {
		return 0, fmt.Errorf("config: %s harus bilangan 0..10000000", name)
	}
	return uint(v), nil
}

// DefaultBugReportEmail = penerima laporan bug bila ACS_BUG_REPORT_EMAIL tidak diatur.
const DefaultBugReportEmail = "hfzrmd@gmail.com"

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
