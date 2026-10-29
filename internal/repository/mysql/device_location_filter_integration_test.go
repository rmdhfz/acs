package mysql

import (
	"context"
	"os"
	"sort"
	"testing"

	"github.com/google/uuid"

	"acs/internal/domain"
)

// Integrasi MariaDB NYATA: filter HasLocation pada daftar perangkat (dasar daftar "tetapkan lokasi" di peta). Perangkat dengan SATU koordinat
// saja (lintang tanpa bujur) tidak punya lokasi yang bisa digambar, jadi termasuk "belum punya". Dilewati tanpa ACS_TEST_DSN.
func TestDeviceRepository_HasLocationFilter_Integration(t *testing.T) {
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

	var statusID uint64
	if err := db.Get(&statusID, `SELECT id FROM ref_device_status WHERE code = 'UNREGISTERED'`); err != nil {
		t.Fatal(err)
	}
	token := "LOC" + uuid.NewString()[:8]
	f64 := func(v float64) *float64 { return &v }
	oui := "ABCDEF"
	seed := map[string][2]*float64{
		"lengkap":       {f64(-6.2), f64(106.8)},
		"kosong":        {nil, nil},
		"hanya-lintang": {f64(1.0), nil},
		"hanya-bujur":   {nil, f64(2.0)},
		"nol-nol":       {f64(0), f64(0)}, // 0,0 adalah koordinat sah (bukan "kosong")
	}
	for name, c := range seed {
		serial := token + "-" + name
		d := &domain.Device{DeviceUUID: uuid.NewString(), DeviceStatusID: statusID, OUI: &oui, SerialNumber: serial, Latitude: c[0], Longitude: c[1]}
		if err := repo.Create(ctx, d); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM devices WHERE oui = ? AND serial_number = ?`, oui, serial) })
	}
	names := func(hasLoc *bool) []string {
		rows, _, err := repo.List(ctx, domain.DeviceFilter{Search: token, HasLocation: hasLoc}, domain.Pagination{PageSize: 50})
		if err != nil {
			t.Fatal(err)
		}
		out := make([]string, len(rows))
		for i, r := range rows {
			out[i] = r.SerialNumber[len(token)+1:]
		}
		sort.Strings(out)
		return out
	}
	eq := func(a, b []string) bool {
		if len(a) != len(b) {
			return false
		}
		for i := range a {
			if a[i] != b[i] {
				return false
			}
		}
		return true
	}
	yes, no := true, false
	if got := names(nil); len(got) != 5 {
		t.Errorf("tanpa saring = %v", got)
	}
	if got, want := names(&yes), []string{"lengkap", "nol-nol"}; !eq(got, want) {
		t.Errorf("has_location=true = %v, mau %v", got, want)
	}
	if got, want := names(&no), []string{"hanya-bujur", "hanya-lintang", "kosong"}; !eq(got, want) {
		t.Errorf("has_location=false = %v, mau %v", got, want)
	}
}
