// Package config memuat konfigurasi aplikasi dari environment variable.
package config

import (
	"encoding/base64"
	"fmt"
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
	LogLevel   string
	RedisAddr  string

	OIDCIssuer       string
	OIDCClientID     string
	OIDCClientSecret string
	OIDCRedirectURL  string
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

	cfg.OIDCIssuer = os.Getenv("ACS_OIDC_ISSUER")
	cfg.OIDCClientID = os.Getenv("ACS_OIDC_CLIENT_ID")
	cfg.OIDCClientSecret = os.Getenv("ACS_OIDC_CLIENT_SECRET")
	cfg.OIDCRedirectURL = getEnv("ACS_OIDC_REDIRECT_URL", "http://localhost:8080/api/v1/auth/oidc/callback")

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
