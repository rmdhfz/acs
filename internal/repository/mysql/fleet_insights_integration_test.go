package mysql

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"acs/internal/domain"
)

// Integrasi MariaDB NYATA: FleetRows membawa status, RX/suhu TERAKHIR, uptime, penanda GPON/EPON, dan menghormati filter tenant.
func TestFleetInsightsRepository_Integration(t *testing.T) {
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
	devs := NewDeviceRepository(db)
	params := NewDeviceParameterRepository(db)
	metrics := NewDeviceOpticalMetricRepository(db)

	var online uint64
	if err := db.Get(&online, `SELECT id FROM ref_device_status WHERE code = 'ONLINE'`); err != nil {
		t.Fatal(err)
	}
	sfx := uuid.NewString()[:6]
	res, err := db.Exec(`INSERT INTO tenants (tenant_uuid, code, name) VALUES (?, ?, ?)`, uuid.NewString(), "it-f-"+sfx, "IT Fleet "+sfx)
	if err != nil {
		t.Fatal(err)
	}
	tid64, _ := res.LastInsertId()
	tid := uint64(tid64)
	oui := "ABCDEF"
	d := &domain.Device{DeviceUUID: uuid.NewString(), TenantID: &tid, DeviceStatusID: online, OUI: &oui, SerialNumber: "ITF-" + sfx}
	if err := devs.Create(ctx, d); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM device_optical_metrics WHERE device_id = ?`, d.ID)
		_, _ = db.Exec(`DELETE FROM device_parameters WHERE device_id = ?`, d.ID)
		_, _ = db.Exec(`DELETE FROM devices WHERE id = ?`, d.ID)
		_, _ = db.Exec(`DELETE FROM tenants WHERE id = ?`, tid)
	})
	up, gp := "7200", "up"
	if err := params.UpsertBatch(ctx, []domain.DeviceParameter{
		{DeviceID: d.ID, ParameterName: "InternetGatewayDevice.DeviceInfo.UpTime", ParameterValue: &up},
		{DeviceID: d.ID, ParameterName: "InternetGatewayDevice.WANDevice.1.X_CT-COM_WANGponLinkConfig.LinkState", ParameterValue: &gp},
	}); err != nil {
		t.Fatal(err)
	}
	base := time.Now().Add(-time.Hour)
	for i, v := range [][2]float64{{-30, 80}, {-19, 48}} { // pembacaan terakhir yang dipakai
		rx, tc := v[0], v[1]
		if err := metrics.Create(ctx, &domain.DeviceOpticalMetric{DeviceID: d.ID, RxPowerDBM: &rx, TemperatureCelsius: &tc, RecordedAt: base.Add(time.Duration(i) * time.Minute)}); err != nil {
			t.Fatal(err)
		}
	}

	repo := NewFleetInsightsRepository(db)
	rows, err := repo.FleetRows(ctx, &tid)
	if err != nil || len(rows) != 1 {
		t.Fatalf("filter tenant harus menghasilkan 1 baris: n=%d err=%v", len(rows), err)
	}
	r := rows[0]
	if r.Status != "ONLINE" || r.RxDBM == nil || *r.RxDBM != -19 || r.TempC == nil || *r.TempC != 48 || r.UptimeRaw == nil || *r.UptimeRaw != "7200" || !r.Gpon || r.Epon {
		t.Fatalf("baris armada salah: %+v", r)
	}
	// lintas tenant memuat paling sedikit baris ini
	all, err := repo.FleetRows(ctx, nil)
	if err != nil || len(all) < 1 {
		t.Fatalf("lintas tenant: n=%d err=%v", len(all), err)
	}
}
