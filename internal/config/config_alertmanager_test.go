package config

import "testing"

func TestAlertmanagerToken_BawaanKosong(t *testing.T) {
	baseEnv(t)
	t.Setenv("ACS_ALERTMANAGER_WEBHOOK_TOKEN", "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AlertmanagerWebhookToken != "" {
		t.Fatal("bawaan harus kosong (endpoint tidak dipasang)")
	}
}

func TestAlertmanagerToken_Sah(t *testing.T) {
	baseEnv(t)
	t.Setenv("ACS_ALERTMANAGER_WEBHOOK_TOKEN", "b7f3a91c5d2e48a0963ce1f7")
	cfg, err := Load()
	if err != nil || cfg.AlertmanagerWebhookToken != "b7f3a91c5d2e48a0963ce1f7" {
		t.Fatalf("token sah harus diterima: %v", err)
	}
}

// Token lemah/pendek/berspasi ditolak saat startup, dan pesan galat TIDAK memuat nilainya.
func TestAlertmanagerToken_DitolakTanpaBocor(t *testing.T) {
	for name, v := range map[string]string{
		"terlalu pendek":     "abc123",
		"23 karakter":        "b7f3a91c5d2e48a0963ce1f",
		"placeholder":        "change-me-change-me-change-me-1",
		"memuat spasi":       "b7f3a91c5d2e 48a0963ce1f7aaaa",
		"spasi di tepi":      " b7f3a91c5d2e48a0963ce1f7 ",
		"contoh konfigurasi": "contoh-token-alertmanager-1234567",
	} {
		baseEnv(t)
		t.Setenv("ACS_ALERTMANAGER_WEBHOOK_TOKEN", v)
		_, err := Load()
		if err == nil {
			t.Errorf("%s: harus ditolak", name)
			continue
		}
		if len(v) > 5 && contains(err.Error(), v) {
			t.Errorf("%s: pesan galat membocorkan token: %v", name, err)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
