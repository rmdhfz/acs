// Package config memuat konfigurasi aplikasi dari environment variable.
package config

import (
	"encoding/base64"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
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

	// USPEnabled — mengaktifkan endpoint WebSocket /usp (TR-369) yang masih
	// kerangka & tanpa autentikasi. Default MATI.
	USPEnabled bool

	// CWMPRatePerIP — batas request/detik PER IP pada endpoint CWMP (default 5). Naikkan bila banyak
	// CPE berbagi satu IP publik (CGNAT/kantor) atau untuk uji beban; jangan dimatikan di produksi
	// (juga mitigasi brute-force shared secret Inform).
	CWMPRatePerIP float64

	// WebhookBlockPrivate — tolak target webhook di jaringan privat (RFC1918/ULA/CGNAT). Aktifkan pada
	// SaaS publik di mana admin tenant tidak tepercaya; matikan bila receiver webhook ada di LAN yang sama.
	WebhookBlockPrivate bool

	// LogRetentionDays — umur maksimum (hari) tabel log volume tinggi (event, sesi CWMP,
	// metrik optik, riwayat webhook). 0 = tanpa pemangkasan. activity_logs (audit) tidak dipangkas.
	LogRetentionDays int

	// OpticalPollMinutes — interval polling redaman optik per perangkat (menit); 0 = nonaktif.
	// Hanya berlaku untuk vendor/model yang punya mapping optical.* di katalog.
	OpticalPollMinutes int

	OIDCIssuer       string
	OIDCClientID     string
	OIDCClientSecret string
	OIDCRedirectURL  string

	// Email (SMTP) untuk reset password & notifikasi keamanan. SMTPHost kosong
	// = email nonaktif (fitur reset lewat email tidak mengirim apa pun; sisa
	// sistem tetap jalan). Kredensial WAJIB dari env — jangan di-commit.
	SMTPHost     string
	SMTPPort     int
	SMTPUsername string
	SMTPPassword string
	SMTPFrom     string
	SMTPFromName string
	// PublicURL — origin frontend yang dipakai membangun tautan di email
	// (mis. https://acs.example.com). Tanpa slash akhir.
	PublicURL string
	// PasswordResetTTL — masa berlaku tautan reset password.
	PasswordResetTTL time.Duration
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

	cfg.JWTSecret = []byte(os.Getenv("ACS_JWT_SECRET"))
	if len(cfg.JWTSecret) < 32 {
		return nil, fmt.Errorf("config: ACS_JWT_SECRET wajib diisi minimal 32 karakter")
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
	uspEnabled, err := strconv.ParseBool(getEnv("ACS_USP_ENABLED", "false"))
	if err != nil {
		return nil, fmt.Errorf("config: ACS_USP_ENABLED tidak valid: %w", err)
	}
	cfg.USPEnabled = uspEnabled
	cfg.WebhookBlockPrivate, err = strconv.ParseBool(getEnv("ACS_WEBHOOK_BLOCK_PRIVATE", "false"))
	if err != nil {
		return nil, fmt.Errorf("config: ACS_WEBHOOK_BLOCK_PRIVATE tidak valid: %w", err)
	}
	cfg.CWMPRatePerIP, err = strconv.ParseFloat(getEnv("ACS_CWMP_RATE_LIMIT", "5"), 64)
	if err != nil || cfg.CWMPRatePerIP < 1 || cfg.CWMPRatePerIP > 1_000_000 {
		return nil, fmt.Errorf("config: ACS_CWMP_RATE_LIMIT harus angka 1..1000000 (request/detik per IP)")
	}
	cfg.OpticalPollMinutes, err = strconv.Atoi(getEnv("ACS_OPTICAL_POLL_MINUTES", "360"))
	if err != nil || cfg.OpticalPollMinutes < 0 || (cfg.OpticalPollMinutes > 0 && cfg.OpticalPollMinutes < 15) {
		return nil, fmt.Errorf("config: ACS_OPTICAL_POLL_MINUTES harus 0 (nonaktif) atau >= 15")
	}
	cfg.LogRetentionDays, err = strconv.Atoi(getEnv("ACS_LOG_RETENTION_DAYS", "90"))
	if err != nil || cfg.LogRetentionDays < 0 || (cfg.LogRetentionDays > 0 && cfg.LogRetentionDays < 7) {
		return nil, fmt.Errorf("config: ACS_LOG_RETENTION_DAYS harus 0 (nonaktif) atau >= 7")
	}

	cfg.OIDCIssuer = os.Getenv("ACS_OIDC_ISSUER")
	cfg.OIDCClientID = os.Getenv("ACS_OIDC_CLIENT_ID")
	cfg.OIDCClientSecret = os.Getenv("ACS_OIDC_CLIENT_SECRET")
	cfg.OIDCRedirectURL = getEnv("ACS_OIDC_REDIRECT_URL", "http://localhost:8080/api/v1/auth/oidc/callback")

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

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
