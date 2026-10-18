package config

import (
	"strings"
	"testing"
)

// baseEnv mengisi env wajib agar Load() sampai ke bagian billing.
func baseEnv(t *testing.T) {
	t.Helper()
	t.Setenv("ACS_DB_DSN", "u:p@tcp(localhost:3306)/db")
	t.Setenv("ACS_JWT_SECRET", "3f9a1c7e5b2d48a0963ce1f7a4d85b20c6e91f3a7b4d2058")
	t.Setenv("ACS_CREDENTIAL_ENC_KEY", "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")
	t.Setenv("ACS_MINIO_ENDPOINT", "minio:9000")
	t.Setenv("ACS_MINIO_ACCESS_KEY", "a")
	t.Setenv("ACS_MINIO_SECRET_KEY", "b")
	for _, k := range []string{"ACS_BILLING_ENABLED", "ACS_XENDIT_SECRET_KEY", "ACS_XENDIT_CALLBACK_TOKEN", "ACS_XENDIT_BASE_URL", "ACS_BILLING_INVOICE_HOURS"} {
		t.Setenv(k, "")
	}
}

const (
	goodKey   = "xnd_development_Zk8vQ2mX9pL4wN7rT5yB3hJ6aSdF0gQe"
	goodToken = "cb-3f9a1c7e5b2d48a0963ce1f7a4d85b20"
)

func TestBilling_DefaultsOff(t *testing.T) {
	baseEnv(t)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BillingEnabled || cfg.XenditBaseURL != "https://api.xendit.co" || cfg.BillingInvoiceHours != 24 {
		t.Fatalf("bawaan salah: %+v", cfg)
	}
}

func TestBilling_EnabledRequiresSecrets(t *testing.T) {
	baseEnv(t)
	t.Setenv("ACS_BILLING_ENABLED", "true")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "ACS_XENDIT_SECRET_KEY") {
		t.Fatalf("tanpa kunci harus ditolak: %v", err)
	}
	t.Setenv("ACS_XENDIT_SECRET_KEY", goodKey)
	if _, err := Load(); err == nil {
		t.Fatal("tanpa token callback harus ditolak")
	}
	t.Setenv("ACS_XENDIT_CALLBACK_TOKEN", goodToken)
	cfg, err := Load()
	if err != nil || !cfg.BillingEnabled || cfg.XenditSecretKey != goodKey || cfg.XenditCallbackToken != goodToken {
		t.Fatalf("konfigurasi sah: %v", err)
	}
}

func TestBilling_RejectsWeakAndNeverEchoesSecrets(t *testing.T) {
	baseEnv(t)
	t.Setenv("ACS_BILLING_ENABLED", "true")
	cases := map[string][2]string{ // nama -> {kunci, token}
		"token pendek":      {goodKey, "short-token-123"},
		"token placeholder": {goodKey, "change-me-to-a-random-callback-token"},
		"token entropi":     {goodKey, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		"kunci placeholder": {"your-secret-key-placeholder-value", goodToken},
		"kunci berspasi":    {"xnd_development_abc def_0123456789", goodToken},
		"token berspasi":    {goodKey, goodToken + " "},
	}
	for name, kv := range cases {
		t.Setenv("ACS_XENDIT_SECRET_KEY", kv[0])
		t.Setenv("ACS_XENDIT_CALLBACK_TOKEN", kv[1])
		_, err := Load()
		if err == nil {
			t.Errorf("%s: harus ditolak", name)
			continue
		}
		for _, secret := range kv {
			if secret != "" && strings.Contains(err.Error(), strings.TrimSpace(secret)) {
				t.Errorf("%s: pesan galat memuat rahasia: %v", name, err)
			}
		}
	}
}

func TestBilling_BaseURLAndHours(t *testing.T) {
	baseEnv(t)
	t.Setenv("ACS_BILLING_ENABLED", "true")
	t.Setenv("ACS_XENDIT_SECRET_KEY", goodKey)
	t.Setenv("ACS_XENDIT_CALLBACK_TOKEN", goodToken)
	for _, bad := range []string{"http://api.xendit.co", "ftp://x", "https://user:pw@api.xendit.co", "api.xendit.co", "http://evil.example.com"} {
		t.Setenv("ACS_XENDIT_BASE_URL", bad)
		if _, err := Load(); err == nil {
			t.Errorf("base URL %q harus ditolak", bad)
		} else if strings.Contains(err.Error(), goodKey) || strings.Contains(err.Error(), "pw") {
			t.Errorf("galat memuat rahasia: %v", err)
		}
	}
	for _, ok := range []string{"https://api.xendit.co", "https://api.xendit.co/", "http://localhost:9999", "http://127.0.0.1:8081"} {
		t.Setenv("ACS_XENDIT_BASE_URL", ok)
		if _, err := Load(); err != nil {
			t.Errorf("base URL %q harus lolos: %v", ok, err)
		}
	}
	t.Setenv("ACS_XENDIT_BASE_URL", "")
	for _, bad := range []string{"0", "169", "abc", "-1"} {
		t.Setenv("ACS_BILLING_INVOICE_HOURS", bad)
		if _, err := Load(); err == nil {
			t.Errorf("jam %q harus ditolak", bad)
		}
	}
	t.Setenv("ACS_BILLING_INVOICE_HOURS", "168")
	if cfg, err := Load(); err != nil || cfg.BillingInvoiceHours != 168 {
		t.Errorf("168 harus lolos: %v", err)
	}
	t.Setenv("ACS_BILLING_ENABLED", "mungkin")
	if _, err := Load(); err == nil {
		t.Error("ACS_BILLING_ENABLED tidak valid harus ditolak")
	}
}
