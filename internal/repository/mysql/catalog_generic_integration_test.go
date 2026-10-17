package mysql

import (
	"context"
	"errors"
	"os"
	"testing"

	"acs/internal/domain"
)

// Integrasi MariaDB NYATA untuk seed migrations/0029: vendor GENERIC membawa mapping standar yang lengkap, vendor
// tambahan ber-OUI tetapi tanpa mapping sendiri, dan daftar vendor memuat oui_count yang cocok dengan isi tabel
// (dipakai UI agar tidak meminta OUI per vendor). Dilewati tanpa ACS_TEST_DSN.
func TestCatalog_GenericVendorSeed_Integration(t *testing.T) {
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

	vendors, _, err := NewVendorRepository(db).List(ctx, domain.Pagination{Page: 1, PageSize: 200})
	if err != nil {
		t.Fatalf("List vendor: %v", err)
	}
	byCode := map[string]domain.Vendor{}
	for _, v := range vendors {
		byCode[v.Code] = v
	}
	generic, tp := byCode[domain.VendorCodeGeneric], byCode["TPLINK"]
	if generic.ID == 0 || tp.ID == 0 {
		t.Fatalf("seed 0029 belum diterapkan (GENERIC=%d TPLINK=%d)", generic.ID, tp.ID)
	}
	// oui_count pada daftar harus sama dengan jumlah baris OUI aktif.
	ouis, err := NewVendorOUIRepository(db).ListByVendor(ctx, tp.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(ouis) == 0 || tp.OUICount != len(ouis) {
		t.Fatalf("oui_count TPLINK = %d, isi tabel = %d", tp.OUICount, len(ouis))
	}

	// GENERIC punya mapping standar di KEDUA data model; vendor tambahan tidak punya mapping sendiri.
	var dmv struct {
		TR098 uint64 `db:"tr098"`
		TR181 uint64 `db:"tr181"`
	}
	if err := db.GetContext(ctx, &dmv,
		`SELECT MAX(CASE WHEN code='TR098' THEN id END) AS tr098, MAX(CASE WHEN code='TR181' THEN id END) AS tr181 FROM ref_data_model_versions`); err != nil {
		t.Fatal(err)
	}
	maps := NewVendorParameterMappingRepository(db)
	for _, c := range []struct {
		dmv  uint64
		key  string
		path string
	}{
		{dmv.TR098, "wifi.ssid", "InternetGatewayDevice.LANDevice.1.WLANConfiguration.1.SSID"},
		{dmv.TR181, "wifi.ssid", "Device.WiFi.SSID.1.SSID"},
		{dmv.TR098, "wan.pppoe.username", "InternetGatewayDevice.WANDevice.1.WANConnectionDevice.1.WANPPPConnection.1.Username"},
		{dmv.TR181, "optical.rx_power", "Device.Optical.Interface.1.OpticalSignalLevel"},
		{dmv.TR098, "diag.ping.host", "InternetGatewayDevice.IPPingDiagnostics.Host"},
	} {
		m, err := maps.Resolve(ctx, generic.ID, c.dmv, nil, c.key, nil)
		if err != nil || m.TR069Path != c.path {
			t.Fatalf("GENERIC %q (dmv %d): %+v %v, harusnya %s", c.key, c.dmv, m, err, c.path)
		}
		if _, err := maps.Resolve(ctx, tp.ID, c.dmv, nil, c.key, nil); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("TPLINK tidak boleh punya mapping sendiri untuk %q (fallback ke GENERIC): %v", c.key, err)
		}
	}
	// skala optik tetap 0,001 pada mapping standar.
	if m, err := maps.Resolve(ctx, generic.ID, dmv.TR181, nil, "optical.tx_power", nil); err != nil || m.ValueScale != 0.001 {
		t.Fatalf("skala optik GENERIC: %+v %v", m, err)
	}
}
