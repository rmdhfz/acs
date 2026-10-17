package report

import (
	"context"
	"errors"
	"testing"
	"time"

	"acs/internal/domain"
)

type fakeRepo struct {
	tenants []domain.TenantReport
	daily   map[string]domain.DailyPoint
}

func (f fakeRepo) Platform(context.Context, time.Time) (domain.PlatformStats, error) {
	return domain.PlatformStats{TenantsTotal: len(f.tenants)}, nil
}
func (f fakeRepo) Tenants(context.Context, time.Time) ([]domain.TenantReport, error) {
	return append([]domain.TenantReport(nil), f.tenants...), nil
}
func (f fakeRepo) Vendors(context.Context) ([]domain.VendorStat, error) { return nil, nil }
func (f fakeRepo) Daily(context.Context, time.Time) (map[string]domain.DailyPoint, error) {
	return f.daily, nil
}

func ptrT(t time.Time) *time.Time { return &t }

func TestBuild_HanyaSuperadminAsli(t *testing.T) {
	s := NewService(fakeRepo{})
	ctx := context.Background()
	for _, a := range []domain.Actor{
		{UserID: 2, Roles: []string{domain.RoleAdmin}},
		{UserID: 1, Roles: []string{domain.RoleSuperadmin}, ImpersonatorID: 9}, // sesi impersonate tidak boleh melihat laporan platform
		{},
	} {
		if _, err := s.Build(ctx, a); !errors.Is(err, domain.ErrForbidden) {
			t.Fatalf("%+v harus ditolak: %v", a, err)
		}
	}
	if _, err := s.Build(ctx, domain.Actor{UserID: 1, Roles: []string{domain.RoleSuperadmin}}); err != nil {
		t.Fatal(err)
	}
}

func TestKesehatanTenantDanUrutan(t *testing.T) {
	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	q := uint32(10)
	repo := fakeRepo{tenants: []domain.TenantReport{
		{TenantID: 1, Name: "Sehat", IsActive: true, Devices: 10, DevicesOnline: 9, LastInformAt: ptrT(now.Add(-time.Hour)), Tasks30d: 20, TasksFailed30d: 1},
		{TenantID: 2, Name: "Mati", IsActive: false, Devices: 99},
		{TenantID: 3, Name: "Sepi", IsActive: true, Devices: 10, DevicesOnline: 2, LastInformAt: ptrT(now.Add(-48 * time.Hour))},
		{TenantID: 4, Name: "Gagal", IsActive: true, Devices: 4, DevicesOnline: 4, LastInformAt: ptrT(now), Tasks30d: 50, TasksFailed30d: 30},
		{TenantID: 5, Name: "Antre", IsActive: true, Devices: 1, DevicesOnline: 1, LastInformAt: ptrT(now), PendingTasks: 9, MaxPending: &q},
		{TenantID: 6, Name: "Kosong", IsActive: true},                                                                       // tanpa perangkat: tidak dianggap bermasalah
		{TenantID: 7, Name: "Penuh", IsActive: true, Devices: 9, DevicesOnline: 9, LastInformAt: ptrT(now), MaxDevices: &q}, // 9/10 perangkat
	}, daily: map[string]domain.DailyPoint{"2026-10-17": {NewDevices: 3, Tasks: 7}}}
	s := NewService(repo)
	s.now = func() time.Time { return now }
	rep, err := s.Build(context.Background(), domain.Actor{UserID: 1, Roles: []string{domain.RoleSuperadmin}})
	if err != nil {
		t.Fatal(err)
	}
	health := map[string]string{}
	reasons := map[string]int{}
	for _, tr := range rep.Tenants {
		health[tr.Name] = tr.Health
		reasons[tr.Name] = len(tr.HealthReasons)
	}
	want := map[string]string{"Sehat": "sehat", "Mati": "nonaktif", "Sepi": "perhatian", "Gagal": "perhatian", "Antre": "perhatian", "Kosong": "sehat", "Penuh": "perhatian"}
	for k, v := range want {
		if health[k] != v {
			t.Errorf("%s: kesehatan %q, mau %q", k, health[k], v)
		}
	}
	if reasons["Sepi"] != 2 {
		t.Errorf("Sepi harus punya 2 alasan (tak ada Inform + online rendah): %d", reasons["Sepi"])
	}
	// urutan: perhatian dulu, nonaktif terakhir
	if rep.Tenants[0].Health != "perhatian" || rep.Tenants[len(rep.Tenants)-1].Health != "nonaktif" {
		t.Errorf("urutan salah: %v", health)
	}
	// 30 hari berurutan, hari tanpa data bernilai 0, hari ada data terisi
	if len(rep.Daily) != 30 || rep.Daily[29].Date != "2026-10-17" || rep.Daily[29].NewDevices != 3 || rep.Daily[0].Tasks != 0 {
		t.Errorf("seri harian salah: %d titik, akhir=%+v", len(rep.Daily), rep.Daily[len(rep.Daily)-1])
	}
}

func TestCSVSafe_FormulaInjection(t *testing.T) {
	for _, in := range []string{"=HYPERLINK(\"http://evil\")", "+1+1", "-2+3", "@SUM(A1)", "\tcmd", "\rx"} {
		if got := CSVSafe(in); got[0] != '\'' {
			t.Errorf("CSVSafe(%q) = %q harus diawali '", in, got)
		}
	}
	for _, in := range []string{"", "PT Aman", "a=b", "123"} {
		if CSVSafe(in) != in {
			t.Errorf("CSVSafe(%q) tidak boleh berubah", in)
		}
	}
	rep := &domain.SaaSReport{Tenants: []domain.TenantReport{{Code: "x", Name: "=cmd|' /C calc'!A0", IsActive: true, Health: "sehat", CreatedAt: time.Now()}}}
	rows := TenantsCSV(rep)
	if len(rows) != 2 || rows[1][1][0] != '\'' {
		t.Fatalf("nama tenant berbahaya harus dinetralkan di CSV: %v", rows[1])
	}
}
