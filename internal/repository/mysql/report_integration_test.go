package mysql

import (
	"context"
	"os"
	"testing"
	"time"
)

// Integrasi MariaDB NYATA: seluruh query laporan SaaS valid terhadap skema dan hasilnya konsisten satu sama
// lain. Dilewati tanpa ACS_TEST_DSN.
func TestReportRepository_Integration(t *testing.T) {
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
	repo := NewReportRepository(db)
	now := time.Now()

	plat, err := repo.Platform(ctx, now)
	if err != nil {
		t.Fatalf("Platform: %v", err)
	}
	tenants, err := repo.Tenants(ctx, now)
	if err != nil {
		t.Fatalf("Tenants: %v", err)
	}
	if len(tenants) != plat.TenantsTotal {
		t.Fatalf("jumlah baris tenant (%d) harus sama dengan tenants_total (%d)", len(tenants), plat.TenantsTotal)
	}
	var devs, online, users int
	for _, tr := range tenants {
		devs += tr.Devices
		online += tr.DevicesOnline
		users += tr.Users
		if tr.DevicesOnline > tr.Devices || tr.TasksFailed30d > tr.Tasks30d {
			t.Fatalf("tenant %s tidak konsisten: %+v", tr.Code, tr)
		}
	}
	if devs > plat.DevicesTotal || online > plat.DevicesOnline || users > plat.UsersTotal {
		t.Fatalf("jumlah per-tenant melebihi total platform: devs %d/%d online %d/%d users %d/%d",
			devs, plat.DevicesTotal, online, plat.DevicesOnline, users, plat.UsersTotal)
	}
	if plat.DevicesOnline > plat.DevicesTotal || plat.TasksFailed30d > plat.Tasks30d || plat.DevicesNew7d > plat.DevicesNew30d {
		t.Fatalf("platform tidak konsisten: %+v", plat)
	}
	vendors, err := repo.Vendors(ctx)
	if err != nil {
		t.Fatalf("Vendors: %v", err)
	}
	sum := 0
	for _, v := range vendors {
		sum += v.Devices
	}
	if sum > plat.DevicesTotal {
		t.Fatalf("jumlah vendor %d melebihi total perangkat %d", sum, plat.DevicesTotal)
	}
	daily, err := repo.Daily(ctx, now.AddDate(0, 0, -29))
	if err != nil {
		t.Fatalf("Daily: %v", err)
	}
	newDevs := 0
	for _, p := range daily {
		newDevs += p.NewDevices
	}
	if newDevs > plat.DevicesNew30d+5 { // toleransi lintas-hari/zona waktu di tepi jendela
		t.Fatalf("perangkat baru harian (%d) tidak cocok dengan devices_new_30d (%d)", newDevs, plat.DevicesNew30d)
	}
}
