package mysql

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"

	"acs/internal/domain"
)

// Integrasi MariaDB NYATA: CandidatePaths (pencarian instance untuk wildcard aturan otomasi). Memastikan '*' menjadi LIKE '%' dengan bagian literal
// di-escape ('_' pada nama seperti X_ZTE_Foo bukan "satu karakter apa saja"), hasil terisolasi per perangkat, terurut, dan dibatasi limit.
// Dilewati tanpa ACS_TEST_DSN.
func TestDeviceParameterRepository_CandidatePaths_Integration(t *testing.T) {
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
	devs, params := NewDeviceRepository(db), NewDeviceParameterRepository(db)
	lister, ok := params.(domain.DeviceParameterPathLister)
	if !ok {
		t.Fatal("repository parameter harus memenuhi domain.DeviceParameterPathLister")
	}

	var statusID uint64
	if err := db.Get(&statusID, `SELECT id FROM ref_device_status WHERE code = 'UNREGISTERED'`); err != nil {
		t.Fatal(err)
	}
	mk := func() uint64 {
		oui, serial := "ABCDEF", "CAND"+uuid.NewString()[:8]
		d := &domain.Device{DeviceUUID: uuid.NewString(), DeviceStatusID: statusID, OUI: &oui, SerialNumber: serial}
		if err := devs.Create(ctx, d); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_, _ = db.Exec(`DELETE FROM device_parameters WHERE device_id = ?`, d.ID)
			_, _ = db.Exec(`DELETE FROM devices WHERE id = ?`, d.ID)
		})
		return d.ID
	}
	a, b := mk(), mk()
	const w = "InternetGatewayDevice.LANDevice.1.WLANConfiguration."
	seed := func(dev uint64, names ...string) {
		for _, n := range names {
			v := "v"
			if err := params.Upsert(ctx, &domain.DeviceParameter{DeviceID: dev, ParameterName: n, ParameterValue: &v}); err != nil {
				t.Fatal(err)
			}
		}
	}
	seed(a, w+"2.SSID", w+"10.SSID", w+"1.SSID", w+"1.PreSharedKey.1.SSID", w+"1.BSSID", w+"Extra.SSID", "InternetGatewayDevice.LANDevice.2.WLANConfiguration.1.SSID")
	seed(b, w+"7.SSID") // perangkat lain: tidak boleh bocor ke hasil perangkat a
	// nama dengan '_' dan '%' literal: pola tanpa '*' harus cocok persis, bukan lewat LIKE liar
	seed(a, "InternetGatewayDevice.X_ZTE.A_1.Foo", "InternetGatewayDevice.XaZTE.A_1.Foo", "InternetGatewayDevice.X_ZTE.AB1.Foo")

	got, err := lister.CandidatePaths(ctx, a, w+"*.SSID", 100)
	if err != nil {
		t.Fatal(err)
	}
	// superset: LIKE '%' juga melahap "1.PreSharedKey.1.SSID" dan "Extra.SSID"; semuanya dari perangkat a saja, terurut menurut nama
	want := []string{w + "1.PreSharedKey.1.SSID", w + "1.SSID", w + "10.SSID", w + "2.SSID", w + "Extra.SSID"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("kandidat = %v\n mau     %v", got, want)
	}
	// penyaringan ketat usecase menyisakan instance numerik tunggal, urut numerik
	var strict []string
	for _, m := range domain.FilterParamPattern(w+"*.SSID", got) {
		strict = append(strict, m.Path)
	}
	if strings.Join(strict, "|") != strings.Join([]string{w + "1.SSID", w + "2.SSID", w + "10.SSID"}, "|") {
		t.Errorf("setelah FilterParamPattern = %v", strict)
	}

	// '_' pada bagian literal di-escape: X_ZTE tidak boleh cocok dengan XaZTE
	got, _ = lister.CandidatePaths(ctx, a, "InternetGatewayDevice.X_ZTE.A_*.Foo", 100)
	if len(got) != 1 || got[0] != "InternetGatewayDevice.X_ZTE.A_1.Foo" {
		t.Errorf("'_' literal harus di-escape: %v", got)
	}
	// perangkat b hanya melihat miliknya
	if got, _ = lister.CandidatePaths(ctx, b, w+"*.SSID", 100); len(got) != 1 || got[0] != w+"7.SSID" {
		t.Errorf("perangkat b = %v", got)
	}
	// limit dihormati; limit <= 0 = tidak ada hasil (bukan semua baris)
	if got, _ = lister.CandidatePaths(ctx, a, w+"*.SSID", 2); len(got) != 2 {
		t.Errorf("limit 2 -> %d baris", len(got))
	}
	if got, err = lister.CandidatePaths(ctx, a, w+"*.SSID", 0); err != nil || len(got) != 0 {
		t.Errorf("limit 0 -> %v err=%v", got, err)
	}
}
