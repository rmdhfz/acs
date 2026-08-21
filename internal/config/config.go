// Package config memuat konfigurasi aplikasi dari environment variable.
package config

import (
	"encoding/base64"
	"fmt"
	"os"
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
		if o = strings.TrimSpace(o); o != "" {
			cfg.CORSAllowOrigins = append(cfg.CORSAllowOrigins, o)
		}
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
