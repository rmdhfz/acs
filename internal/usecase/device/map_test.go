package device

import (
	"context"
	"errors"
	"testing"

	"acs/internal/domain"
)

type fakeMapReader struct {
	gotFilter domain.DeviceFilter
	gotBox    *domain.BBox
	gotLimit  int
	res       domain.DeviceMapResult
}

func (f *fakeMapReader) ListForMap(_ context.Context, fl domain.DeviceFilter, box *domain.BBox, limit int) (domain.DeviceMapResult, error) {
	f.gotFilter, f.gotBox, f.gotLimit = fl, box, limit
	return f.res, nil
}

type statusRefs struct{ domain.RefRepository }

func (statusRefs) GetByID(_ context.Context, table string, id uint64) (domain.RefLookup, error) {
	if table != domain.RefTableDeviceStatus {
		return domain.RefLookup{}, errors.New("tabel tak terduga: " + table)
	}
	switch id {
	case 1:
		return domain.RefLookup{ID: 1, Code: "ONLINE"}, nil
	case 2:
		return domain.RefLookup{ID: 2, Code: "OFFLINE"}, nil
	}
	return domain.RefLookup{}, domain.ErrNotFound
}

func TestListForMap_TenantDipaksaUntukNonSuperadmin(t *testing.T) {
	mine, other := uint64(5), uint64(9)
	reader := &fakeMapReader{}
	svc := &Service{mapReader: reader, refs: statusRefs{}}
	admin := domain.Actor{TenantID: &mine, Roles: []string{domain.RoleAdmin}}

	// Klien mencoba menyuntik tenant lain lewat filter: harus ditimpa dengan tenant actor.
	if _, err := svc.ListForMap(context.Background(), admin, domain.DeviceFilter{TenantID: &other}, nil); err != nil {
		t.Fatal(err)
	}
	if reader.gotFilter.TenantID == nil || *reader.gotFilter.TenantID != mine {
		t.Fatalf("tenant filter = %v, mau dipaksa ke %d (tidak boleh bocor ke tenant %d)", reader.gotFilter.TenantID, mine, other)
	}
	if reader.gotLimit != domain.MaxMapPoints {
		t.Errorf("limit = %d, mau %d", reader.gotLimit, domain.MaxMapPoints)
	}

	// Actor non-superadmin tanpa tenant: ditolak, tidak membaca data.
	reader.gotLimit = 0
	if _, err := svc.ListForMap(context.Background(), domain.Actor{Roles: []string{domain.RoleAdmin}}, domain.DeviceFilter{}, nil); err == nil {
		t.Fatal("actor tanpa tenant seharusnya ditolak")
	}
	if reader.gotLimit != 0 {
		t.Error("repositori tidak boleh dipanggil untuk actor tanpa tenant")
	}

	// Superadmin boleh memilih tenant lewat filter.
	super := domain.Actor{Roles: []string{domain.RoleSuperadmin}}
	if _, err := svc.ListForMap(context.Background(), super, domain.DeviceFilter{TenantID: &other}, nil); err != nil {
		t.Fatal(err)
	}
	if reader.gotFilter.TenantID == nil || *reader.gotFilter.TenantID != other {
		t.Errorf("superadmin harus bisa memfilter tenant: %v", reader.gotFilter.TenantID)
	}
}

func TestListForMap_ParameterRahasiaDitolakUntukPeranBiasa(t *testing.T) {
	tid := uint64(1)
	svc := &Service{mapReader: &fakeMapReader{}, refs: statusRefs{}}
	f := domain.DeviceFilter{Params: []domain.ParamCond{{Name: "InternetGatewayDevice.LANDevice.1.WLANConfiguration.1.PreSharedKey.1.KeyPassphrase", Op: domain.ParamOpEq, Value: "x"}}}
	noc := domain.Actor{TenantID: &tid, Roles: []string{domain.RoleNOC}}
	if _, err := svc.ListForMap(context.Background(), noc, f, nil); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("err = %v, mau ErrInvalidInput (filter atas rahasia = oracle penebak)", err)
	}
}

func TestListForMap_StatusDiisiDariRefDanTitikKosongBukanNil(t *testing.T) {
	tid := uint64(1)
	reader := &fakeMapReader{res: domain.DeviceMapResult{
		Points: []domain.DeviceMapPoint{{ID: 1, DeviceStatusID: 1}, {ID: 2, DeviceStatusID: 2}, {ID: 3, DeviceStatusID: 1}, {ID: 4, DeviceStatusID: 99}},
		Total:  10, Located: 4,
	}}
	svc := &Service{mapReader: reader, refs: statusRefs{}}
	res, err := svc.ListForMap(context.Background(), domain.Actor{TenantID: &tid, Roles: []string{domain.RoleViewer}}, domain.DeviceFilter{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"ONLINE", "OFFLINE", "ONLINE", ""} // id tak dikenal -> kosong, bukan galat
	for i, p := range res.Points {
		if p.Status != want[i] {
			t.Errorf("titik %d status = %q, mau %q", p.ID, p.Status, want[i])
		}
	}
	if res.Total != 10 || res.Located != 4 {
		t.Errorf("hitungan dari repositori harus diteruskan: %+v", res)
	}

	empty := &Service{mapReader: &fakeMapReader{}, refs: statusRefs{}}
	r2, err := empty.ListForMap(context.Background(), domain.Actor{TenantID: &tid, Roles: []string{domain.RoleViewer}}, domain.DeviceFilter{}, nil)
	if err != nil || r2.Points == nil {
		t.Fatalf("tanpa titik harus larik kosong (JSON []), bukan null: %+v err=%v", r2, err)
	}
}

func TestListForMap_TanpaPembacaDitolak(t *testing.T) {
	tid := uint64(1)
	svc := &Service{refs: statusRefs{}}
	if _, err := svc.ListForMap(context.Background(), domain.Actor{TenantID: &tid, Roles: []string{domain.RoleViewer}}, domain.DeviceFilter{}, nil); err == nil {
		t.Fatal("tanpa mapReader seharusnya galat")
	}
}
