package config

import "testing"

func TestDBPool_Defaults(t *testing.T) {
	baseEnv(t)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DBMaxOpenConns != 50 || cfg.DBMaxIdleConns != 10 || cfg.DBConnMaxLifetimeMinutes != 30 {
		t.Fatalf("bawaan salah (harus 50/10/30, nilai sebelum env ini ada): %+v", cfg)
	}
}

func TestDBPool_Overridable(t *testing.T) {
	baseEnv(t)
	t.Setenv("ACS_DB_MAX_OPEN_CONNS", "200")
	t.Setenv("ACS_DB_MAX_IDLE_CONNS", "50")
	t.Setenv("ACS_DB_CONN_MAX_LIFETIME_MINUTES", "15")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DBMaxOpenConns != 200 || cfg.DBMaxIdleConns != 50 || cfg.DBConnMaxLifetimeMinutes != 15 {
		t.Fatalf("override tidak terbaca: %+v", cfg)
	}
}

func TestDBPool_RejectsInvalid(t *testing.T) {
	baseEnv(t)
	cases := map[string][3]string{ // nama -> {open, idle, lifetime}
		"open 0":               {"0", "", ""},
		"open negatif":         {"-5", "", ""},
		"open bukan angka":     {"abc", "", ""},
		"idle negatif":         {"", "-1", ""},
		"idle melebihi open":   {"10", "20", ""},
		"lifetime 0":           {"", "", "0"},
		"lifetime di atas 24h": {"", "", "1441"},
	}
	for name, v := range cases {
		baseEnv(t)
		if v[0] != "" {
			t.Setenv("ACS_DB_MAX_OPEN_CONNS", v[0])
		}
		if v[1] != "" {
			t.Setenv("ACS_DB_MAX_IDLE_CONNS", v[1])
		}
		if v[2] != "" {
			t.Setenv("ACS_DB_CONN_MAX_LIFETIME_MINUTES", v[2])
		}
		if _, err := Load(); err == nil {
			t.Errorf("%s: harus ditolak", name)
		}
	}
}
