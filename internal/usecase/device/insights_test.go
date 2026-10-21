package device

import (
	"context"
	"testing"
	"time"

	"acs/internal/domain"
)

func str(v string) *string   { return &v }

func count(b []domain.BucketCount, key string) int {
	for _, x := range b {
		if x.Key == key {
			return x.Count
		}
	}
	return -1
}

func TestBuildFleetInsights(t *testing.T) {
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	ago := func(d time.Duration) *time.Time { v := now.Add(-d); return &v }
	rows := []domain.FleetRow{
		// online: GPON, RX baik, suhu normal, uptime 2 jam
		{Status: "ONLINE", Gpon: true, RxDBM: f64(-20), TempC: f64(45), UptimeRaw: str("7200")},
		// online: EPON, RX waspada, suhu peringatan, uptime 3 hari
		{Status: "ONLINE", Epon: true, RxDBM: f64(-26), TempC: f64(60), UptimeRaw: str("259200")},
		// online: tanpa mode, RX kritis lemah (mentah per 0,01 dBm), suhu tinggi, uptime 10 hari
		{Status: "ONLINE", RxDBM: f64(-2900), TempC: f64(70), UptimeRaw: str("864000")},
		// online: RX overload, suhu kritis, uptime > 30 hari
		{Status: "ONLINE", Gpon: true, RxDBM: f64(-5), TempC: f64(85), UptimeRaw: str("3000000")},
		// online: semua tak diketahui / tak masuk akal
		{Status: "ONLINE", RxDBM: f64(-5000000), TempC: f64(999), UptimeRaw: str("abc")},
		// offline: 2 jam, 2 hari, 5 hari, 20 hari, 90 hari, tak pernah Inform
		{Status: "OFFLINE", LastInformAt: ago(2 * time.Hour)},
		{Status: "OFFLINE", LastInformAt: ago(48 * time.Hour)},
		{Status: "OFFLINE", LastInformAt: ago(5 * 24 * time.Hour)},
		{Status: "OFFLINE", LastInformAt: ago(20 * 24 * time.Hour)},
		{Status: "OFFLINE", LastInformAt: ago(90 * 24 * time.Hour)},
		{Status: "OFFLINE"},
		// status lain tidak ikut distribusi
		{Status: "FAULTY", Gpon: true, RxDBM: f64(-40)},
		{Status: "UNREGISTERED"},
	}
	in := BuildFleetInsights(rows, now)
	if in.Online != 5 || in.Offline != 6 || in.Other != 2 {
		t.Fatalf("hitungan status salah: %+v", in)
	}
	check := func(name string, b []domain.BucketCount, want map[string]int) {
		t.Helper()
		total := 0
		for _, x := range b {
			total += x.Count
		}
		for k, n := range want {
			if got := count(b, k); got != n {
				t.Errorf("%s[%s] = %d, mau %d", name, k, got, n)
			}
		}
		_ = total
	}
	check("pon", in.PonMode, map[string]int{"gpon": 2, "epon": 1, "unknown": 2})
	check("uptime", in.Uptime, map[string]int{"lt1d": 1, "d1_7": 1, "d7_30": 1, "gt30d": 1, "unknown": 1})
	check("rx", in.OpticalRX, map[string]int{"ok": 1, "warn": 1, "bad": 1, "overload": 1, "unknown": 1})
	check("suhu", in.Temperature, map[string]int{"normal": 1, "warning": 1, "high": 1, "critical": 1, "unknown": 1})
	check("offline", in.OfflineAge, map[string]int{"lt1d": 1, "d1_3": 1, "d3_7": 1, "d7_30": 1, "gt30d": 1, "unknown": 1})

	// keranjang kosong tetap ada (grafik stabil) dan urutannya tetap
	empty := BuildFleetInsights(nil, now)
	if len(empty.OpticalRX) != 5 || len(empty.Temperature) != 5 || len(empty.OfflineAge) != 6 || empty.Online != 0 {
		t.Fatalf("keranjang kosong harus tetap lengkap: %+v", empty)
	}
	if empty.OpticalRX[0].Key != "ok" || empty.OpticalRX[3].Key != "overload" {
		t.Fatalf("urutan keranjang berubah: %+v", empty.OpticalRX)
	}
}

func TestBuildFleetInsights_BatasAmbang(t *testing.T) {
	now := time.Now()
	for rx, want := range map[float64]string{-25: "ok", -25.01: "warn", -27: "warn", -27.01: "bad", -8: "ok", -7.99: "overload"} {
		if got := rxBucket(f64(rx)); got != want {
			t.Errorf("rx %v = %s, mau %s", rx, got, want)
		}
	}
	for c, want := range map[float64]string{59.9: "normal", 60: "warning", 69.9: "warning", 70: "high", 79.9: "high", 80: "critical"} {
		if got := tempBucket(f64(c)); got != want {
			t.Errorf("suhu %v = %s, mau %s", c, got, want)
		}
	}
	_ = now
}

func TestInsights_ScopeTenantDanTanpaRepo(t *testing.T) {
	s := &Service{}
	if _, err := s.Insights(context.Background(), domain.Actor{Roles: []string{domain.RoleAdmin}}); err != domain.ErrNotFound {
		t.Fatalf("tanpa repo harus ErrNotFound: %v", err)
	}
	var got *uint64
	repo := &fleetStub{on: func(t *uint64) { got = t }}
	s.WithFleetInsights(repo)
	tid := uint64(7)
	if _, err := s.Insights(context.Background(), domain.Actor{UserID: 1, TenantID: &tid, Roles: []string{domain.RoleAdmin}}); err != nil {
		t.Fatal(err)
	}
	if got == nil || *got != 7 {
		t.Fatalf("ADMIN harus dibatasi ke tenant sendiri: %v", got)
	}
	if _, err := s.Insights(context.Background(), domain.Actor{UserID: 2, Roles: []string{domain.RoleSuperadmin}}); err != nil || got != nil {
		t.Fatalf("SUPERADMIN lintas tenant (nil): got=%v err=%v", got, err)
	}
	if _, err := s.Insights(context.Background(), domain.Actor{UserID: 3, Roles: []string{domain.RoleAdmin}}); err == nil {
		t.Fatal("non-superadmin tanpa tenant harus ditolak")
	}
}

type fleetStub struct{ on func(*uint64) }

func (f *fleetStub) FleetRows(_ context.Context, t *uint64) ([]domain.FleetRow, error) {
	f.on(t)
	return nil, nil
}
