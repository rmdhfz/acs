package mysql

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"acs/internal/domain"
)

// Test integrasi MariaDB NYATA untuk filter daftar perangkat berdasarkan level redaman (RX terakhir) dan vendor belum dikenal.
// Dilewati bila ACS_TEST_DSN tidak diisi.
func TestDeviceRepository_ListFilterOpticalAndNoVendor_Integration(t *testing.T) {
	dsn := os.Getenv("ACS_TEST_DSN")
	if dsn == "" {
		t.Skip("ACS_TEST_DSN tidak diisi — test integrasi MariaDB dilewati")
	}
	db, err := Connect(dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()
	repo := NewDeviceRepository(db)
	metrics := NewDeviceOpticalMetricRepository(db)

	var statusID, vendorID uint64
	if err := db.Get(&statusID, `SELECT id FROM ref_device_status WHERE code = 'UNREGISTERED'`); err != nil {
		t.Fatalf("status: %v", err)
	}
	if err := db.Get(&vendorID, `SELECT id FROM ref_vendors WHERE code = 'GENERIC'`); err != nil {
		t.Fatalf("vendor: %v", err)
	}

	// Tiap perangkat diberi tag uji yang sama lewat Search (serial berawalan unik) agar filter tak bercampur data lain.
	prefix := "ITF" + uuid.NewString()[:6]
	oui := "ABCDEF"
	type spec struct {
		name   string
		rx     []float64 // urutan pembacaan; yang terakhir yang dipakai
		vendor bool
	}
	specs := []spec{
		{"ok", []float64{-20}, true},
		{"warn", []float64{-26}, true},
		{"bad-weak", []float64{-30}, true},
		{"bad-overload", []float64{-5}, true},
		{"unknown", nil, false},
		{"recovered", []float64{-30, -19}, true}, // pembacaan terakhir menang → ok
	}
	ids := map[string]uint64{}
	base := time.Now().Add(-time.Hour)
	for _, s := range specs {
		serial := prefix + "-" + s.name
		d := &domain.Device{DeviceUUID: uuid.NewString(), DeviceStatusID: statusID, OUI: &oui, SerialNumber: serial}
		if s.vendor {
			d.VendorID = &vendorID
		}
		if err := repo.Create(ctx, d); err != nil {
			t.Fatal(err)
		}
		ids[s.name] = d.ID
		id := d.ID
		t.Cleanup(func() {
			_, _ = db.Exec(`DELETE FROM device_optical_metrics WHERE device_id = ?`, id)
			_, _ = db.Exec(`DELETE FROM devices WHERE id = ?`, id)
		})
		for i, rx := range s.rx {
			v := rx
			if err := metrics.Create(ctx, &domain.DeviceOpticalMetric{DeviceID: d.ID, RxPowerDBM: &v, RecordedAt: base.Add(time.Duration(i) * time.Minute)}); err != nil {
				t.Fatal(err)
			}
		}
	}

	names := func(f domain.DeviceFilter) map[string]bool {
		f.Search = prefix
		rows, _, err := repo.List(ctx, f, domain.Pagination{Page: 1, PageSize: 50})
		if err != nil {
			t.Fatal(err)
		}
		got := map[string]bool{}
		for _, r := range rows {
			for n, id := range ids {
				if r.ID == id {
					got[n] = true
				}
			}
		}
		return got
	}
	same := func(label string, got map[string]bool, want ...string) {
		t.Helper()
		if len(got) != len(want) {
			t.Fatalf("%s: dapat %v, mau %v", label, got, want)
		}
		for _, w := range want {
			if !got[w] {
				t.Fatalf("%s: dapat %v, mau %v", label, got, want)
			}
		}
	}
	same("ok", names(domain.DeviceFilter{OpticalLevel: domain.OpticalOK}), "ok", "recovered")
	same("warn", names(domain.DeviceFilter{OpticalLevel: domain.OpticalWarn}), "warn")
	same("bad", names(domain.DeviceFilter{OpticalLevel: domain.OpticalBad}), "bad-weak", "bad-overload")
	same("unknown", names(domain.DeviceFilter{OpticalLevel: domain.OpticalUnknown}), "unknown")
	same("no_vendor", names(domain.DeviceFilter{NoVendor: true}), "unknown")
	same("no_vendor+unknown", names(domain.DeviceFilter{NoVendor: true, OpticalLevel: domain.OpticalUnknown}), "unknown")
	same("no_vendor+ok", names(domain.DeviceFilter{NoVendor: true, OpticalLevel: domain.OpticalOK}))
	same("tanpa filter", names(domain.DeviceFilter{}), "ok", "warn", "bad-weak", "bad-overload", "unknown", "recovered")

	if _, _, err := repo.List(ctx, domain.DeviceFilter{OpticalLevel: "ngawur"}, domain.Pagination{Page: 1, PageSize: 5}); err == nil {
		t.Fatal("level tak dikenal harus ditolak")
	}
}
