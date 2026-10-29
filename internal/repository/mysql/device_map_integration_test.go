package mysql

import (
	"context"
	"os"
	"sort"
	"testing"

	"github.com/google/uuid"

	"acs/internal/domain"
)

// Test integrasi MariaDB NYATA: query peta (ListForMap) — hanya perangkat ber-koordinat yang digambar, hitungan total/located jujur,
// bbox (termasuk yang melintasi bujur 180) dan pemotongan bekerja, dan filter daftar yang sama berlaku. Dilewati bila ACS_TEST_DSN kosong.
func TestDeviceRepository_ListForMap_Integration(t *testing.T) {
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
	reader := repo.(domain.DeviceMapReader)

	var statusID uint64
	if err := db.Get(&statusID, `SELECT id FROM ref_device_status WHERE code = 'ONLINE'`); err != nil {
		t.Fatal(err)
	}
	token := "MAP" + uuid.NewString()[:8] // serial berawalan token ini = milik test; Search mengisolasi dari baris lain
	f64 := func(v float64) *float64 { return &v }
	seed := []struct {
		name     string
		lat, lon *float64
	}{
		{"jakarta", f64(-6.2088), f64(106.8456)},
		{"bandung", f64(-6.9175), f64(107.6191)},
		{"medan", f64(3.5952), f64(98.6722)},
		{"fiji-timur", f64(-17.7), f64(178.0)},  // dekat bujur 180, sisi timur
		{"fiji-barat", f64(-16.5), f64(-179.5)}, // seberang garis tanggal
		{"tanpa-lokasi", nil, nil},
		{"setengah-lokasi", f64(1.0), nil}, // hanya lintang: bukan titik yang bisa digambar
	}
	oui := "ABCDEF"
	for _, s := range seed {
		serial := token + "-" + s.name
		d := &domain.Device{DeviceUUID: uuid.NewString(), DeviceStatusID: statusID, OUI: &oui, SerialNumber: serial, Latitude: s.lat, Longitude: s.lon}
		if err := repo.Create(ctx, d); err != nil {
			t.Fatalf("%s: %v", s.name, err)
		}
		t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM devices WHERE oui = ? AND serial_number = ?`, oui, serial) })
	}
	names := func(res domain.DeviceMapResult) []string {
		var out []string
		for _, p := range res.Points {
			out = append(out, p.SerialNumber[len(token)+1:])
		}
		sort.Strings(out)
		return out
	}
	eq := func(got, want []string) bool {
		sort.Strings(want)
		if len(got) != len(want) {
			return false
		}
		for i := range got {
			if got[i] != want[i] {
				return false
			}
		}
		return true
	}
	flt := domain.DeviceFilter{Search: token}

	// Tanpa bbox: 5 titik tergambar; total 7 cocok filter, 5 di antaranya berkoordinat lengkap.
	res, err := reader.ListForMap(ctx, flt, nil, 100)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"jakarta", "bandung", "medan", "fiji-timur", "fiji-barat"}; !eq(names(res), want) {
		t.Fatalf("titik = %v, mau %v", names(res), want)
	}
	if res.Total != 7 || res.Located != 5 || res.Truncated {
		t.Fatalf("hitungan = total %d located %d truncated %v, mau 7/5/false", res.Total, res.Located, res.Truncated)
	}
	for _, p := range res.Points {
		if p.DeviceUUID == "" || p.DeviceStatusID != statusID || p.Latitude == 0 {
			t.Errorf("titik tak lengkap: %+v", p)
		}
	}

	// bbox Pulau Jawa bagian barat: Jakarta + Bandung saja; hitungan tetap menggambarkan filter, bukan bbox.
	res, err = reader.ListForMap(ctx, flt, &domain.BBox{West: 106, South: -8, East: 108, North: -6}, 100)
	if err != nil {
		t.Fatal(err)
	}
	if !eq(names(res), []string{"jakarta", "bandung"}) || res.Total != 7 || res.Located != 5 {
		t.Fatalf("bbox Jawa: %v total=%d located=%d", names(res), res.Total, res.Located)
	}

	// bbox melintasi garis tanggal (barat 175 > timur -175): kedua Fiji, bukan Indonesia.
	res, err = reader.ListForMap(ctx, flt, &domain.BBox{West: 175, South: -20, East: -175, North: -10}, 100)
	if err != nil {
		t.Fatal(err)
	}
	if !eq(names(res), []string{"fiji-timur", "fiji-barat"}) {
		t.Fatalf("bbox antimeridian: %v", names(res))
	}

	// Pemotongan: limit 2 dari 5 titik -> 2 titik, Truncated, hitungan tetap utuh.
	res, err = reader.ListForMap(ctx, flt, nil, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Points) != 2 || !res.Truncated || res.Located != 5 {
		t.Fatalf("limit 2: %d titik truncated=%v located=%d", len(res.Points), res.Truncated, res.Located)
	}
	// Tepat sebesar limit: tidak dianggap terpotong.
	if res, err = reader.ListForMap(ctx, flt, nil, 5); err != nil || res.Truncated || len(res.Points) != 5 {
		t.Fatalf("limit 5: %d titik truncated=%v err=%v", len(res.Points), res.Truncated, err)
	}

	// Filter daftar yang sama berlaku: serial persis -> satu titik.
	one, err := reader.ListForMap(ctx, domain.DeviceFilter{SerialNumber: token + "-medan"}, nil, 100)
	if err != nil || !eq(names(one), []string{"medan"}) || one.Total != 1 {
		t.Fatalf("serial persis: %v total=%d err=%v", names(one), one.Total, err)
	}
	// Tenant lain tidak melihat apa pun: tenant yang tak punya perangkat uji.
	nobody := uint64(999999999)
	none, err := reader.ListForMap(ctx, domain.DeviceFilter{Search: token, TenantID: &nobody}, nil, 100)
	if err != nil || len(none.Points) != 0 || none.Total != 0 {
		t.Fatalf("tenant lain: %+v err=%v", none, err)
	}
	// Hasil kosong: bukan nil (JSON []).
	if none.Points != nil && len(none.Points) != 0 {
		t.Fatal("harus kosong")
	}
}
