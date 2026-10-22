package vparam

import (
	"context"
	"errors"
	"strings"
	"testing"

	"acs/internal/domain"
)

// ---- fakes ----

type fakeRepo struct {
	rows   []*domain.VirtualParameter
	nextID uint64
}

func (r *fakeRepo) Create(_ context.Context, v *domain.VirtualParameter, _ *uint64) error {
	r.nextID++
	v.ID, v.UUID = r.nextID, "uuid-"+strings.Repeat("x", int(r.nextID))
	c := *v
	r.rows = append(r.rows, &c)
	return nil
}
func (r *fakeRepo) GetByUUID(_ context.Context, id string) (*domain.VirtualParameter, error) {
	for _, v := range r.rows {
		if v.UUID == id {
			c := *v
			return &c, nil
		}
	}
	return nil, domain.ErrNotFound
}
func (r *fakeRepo) List(_ context.Context, tenantID *uint64, _ bool, _ domain.Pagination) ([]domain.VirtualParameter, int, error) {
	var out []domain.VirtualParameter
	for _, v := range r.rows {
		if tenantID == nil || v.TenantID == nil || *v.TenantID == *tenantID {
			out = append(out, *v)
		}
	}
	return out, len(out), nil
}
func (r *fakeRepo) ListForTenant(_ context.Context, tenantID *uint64) ([]domain.VirtualParameter, error) {
	seen := map[string]domain.VirtualParameter{}
	for _, v := range r.rows { // global dulu, lalu tenant menimpa
		if v.IsActive && v.TenantID == nil {
			seen[v.Name] = *v
		}
	}
	for _, v := range r.rows {
		if v.IsActive && tenantID != nil && v.TenantID != nil && *v.TenantID == *tenantID {
			seen[v.Name] = *v
		}
	}
	var out []domain.VirtualParameter
	for _, v := range seen {
		out = append(out, v)
	}
	return out, nil
}
func sameScope(a, b *uint64) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}
func (r *fakeRepo) FindByName(_ context.Context, tenantID *uint64, name string) (*domain.VirtualParameter, error) {
	for _, v := range r.rows {
		if v.Name == name && sameScope(v.TenantID, tenantID) {
			c := *v
			return &c, nil
		}
	}
	return nil, domain.ErrNotFound
}
func (r *fakeRepo) CountInScope(_ context.Context, tenantID *uint64) (int, error) {
	n := 0
	for _, v := range r.rows {
		if sameScope(v.TenantID, tenantID) {
			n++
		}
	}
	return n, nil
}
func (r *fakeRepo) Update(_ context.Context, v *domain.VirtualParameter, _ *uint64) error {
	for i, x := range r.rows {
		if x.ID == v.ID {
			c := *v
			r.rows[i] = &c
		}
	}
	return nil
}
func (r *fakeRepo) SoftDelete(_ context.Context, id, _ uint64) error {
	for i, x := range r.rows {
		if x.ID == id {
			r.rows = append(r.rows[:i], r.rows[i+1:]...)
			return nil
		}
	}
	return domain.ErrNotFound
}

type fakeParams struct {
	domain.DeviceParameterRepository
	vals  map[uint64]map[string]string
	calls int
	names []string
}

func (p *fakeParams) ValuesFor(_ context.Context, ids []uint64, names []string) (map[uint64]map[string]string, error) {
	p.calls++
	p.names = names
	out := map[uint64]map[string]string{}
	for _, id := range ids {
		if m, ok := p.vals[id]; ok {
			out[id] = m
		}
	}
	return out, nil
}

type fakeDevices struct{ devs map[uint64]*domain.Device }

func (d *fakeDevices) Get(_ context.Context, actor domain.Actor, id uint64) (*domain.Device, error) {
	dev, ok := d.devs[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	if !actor.IsSuperadmin() && (actor.TenantID == nil || dev.TenantID == nil || *actor.TenantID != *dev.TenantID) {
		return nil, domain.ErrForbidden
	}
	return dev, nil
}

func u64(v uint64) *uint64 { return &v }
func sp(s string) *string  { return &s }

const (
	rxPath = "InternetGatewayDevice.X_ZTE.OpticalRx"
	upPath = "InternetGatewayDevice.DeviceInfo.UpTime"
)

func newSvc() (*Service, *fakeRepo, *fakeParams) {
	repo := &fakeRepo{}
	params := &fakeParams{vals: map[uint64]map[string]string{
		1: {rxPath: "-21500", upPath: "172800"},
		2: {rxPath: "-27400"},
		3: {upPath: "86400"},
	}}
	devs := &fakeDevices{devs: map[uint64]*domain.Device{
		1: {ID: 1, TenantID: u64(7)}, 2: {ID: 2, TenantID: u64(7)}, 3: {ID: 3, TenantID: u64(8)},
	}}
	return NewService(repo, devs, params, nil), repo, params
}

var (
	adminA = domain.Actor{UserID: 1, TenantID: u64(7), Roles: []string{domain.RoleAdmin}}
	adminB = domain.Actor{UserID: 2, TenantID: u64(8), Roles: []string{domain.RoleAdmin}}
	super  = domain.Actor{UserID: 3, Roles: []string{domain.RoleSuperadmin}}
)

func TestCreate_Validation(t *testing.T) {
	s, _, _ := newSvc()
	ctx := context.Background()
	ok := Input{Name: "rx_dbm", Label: "RX (dBm)", Expression: "{" + rxPath + "} / 1000", Unit: sp("dBm")}
	if _, err := s.Create(ctx, adminA, ok); err != nil {
		t.Fatalf("valid: %v", err)
	}
	bad := map[string]Input{
		"nama huruf besar":       {Name: "RxDbm", Label: "x", Expression: "1"},
		"nama diawali angka":     {Name: "1rx", Label: "x", Expression: "1"},
		"label kosong":           {Name: "a", Label: " ", Expression: "1"},
		"ekspresi kosong":        {Name: "a", Label: "x", Expression: ""},
		"ekspresi salah sintaks": {Name: "a", Label: "x", Expression: "1 +"},
		"fungsi tak dikenal":     {Name: "a", Label: "x", Expression: "exec('rm')"},
		"merujuk rahasia":        {Name: "a", Label: "x", Expression: "{InternetGatewayDevice.LANDevice.1.WLANConfiguration.1.KeyPassphrase}"},
		"merujuk password":       {Name: "a", Label: "x", Expression: "first({Device.PPP.Interface.1.Password}, 'x')"},
	}
	for name, in := range bad {
		if _, err := s.Create(ctx, adminA, in); !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("%s harus ErrInvalidInput, got %v", name, err)
		}
	}
	// nama ganda pada scope yang sama = konflik; pada tenant lain boleh
	if _, err := s.Create(ctx, adminA, ok); !errors.Is(err, domain.ErrConflict) {
		t.Errorf("nama ganda harus ErrConflict, got %v", err)
	}
	if _, err := s.Create(ctx, adminB, ok); err != nil {
		t.Errorf("nama sama di tenant lain harus boleh: %v", err)
	}
	// non-superadmin tanpa tenant ditolak; tenant_id dari klien diabaikan untuk non-superadmin
	if _, err := s.Create(ctx, domain.Actor{UserID: 9, Roles: []string{domain.RoleAdmin}}, Input{Name: "z", Label: "z", Expression: "1"}); !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("tanpa tenant harus ErrForbidden, got %v", err)
	}
	v, err := s.Create(ctx, adminA, Input{TenantID: u64(8), Name: "milik_a", Label: "x", Expression: "1"})
	if err != nil || v.TenantID == nil || *v.TenantID != 7 {
		t.Errorf("tenant_id klien harus diabaikan untuk ADMIN: %+v %v", v, err)
	}
	// superadmin membuat global (tenant nil)
	g, err := s.Create(ctx, super, Input{Name: "global_x", Label: "G", Expression: "1"})
	if err != nil || g.TenantID != nil {
		t.Errorf("superadmin tanpa tenant_id = global: %+v %v", g, err)
	}
}

func TestCreate_BatasPerScope(t *testing.T) {
	s, _, _ := newSvc()
	for i := 0; i < domain.VParamMaxPerScope; i++ {
		name := "p" + strings.Repeat("a", i%20) + string(rune('a'+i/20))
		name = name + "_" + string(rune('a'+i%26))
		if _, err := s.Create(context.Background(), adminA, Input{Name: name, Label: "x", Expression: "1"}); err != nil && !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("create %d: %v", i, err)
		}
	}
	// scope sudah penuh (atau hampir): paksa penuh lewat repo
	repo := s.repo.(*fakeRepo)
	for n, _ := repo.CountInScope(context.Background(), u64(7)); n < domain.VParamMaxPerScope; n++ {
		repo.rows = append(repo.rows, &domain.VirtualParameter{ID: uint64(1000 + n), TenantID: u64(7), Name: "isi" + string(rune('a'+n%26)) + string(rune('a'+n/26))})
	}
	if _, err := s.Create(context.Background(), adminA, Input{Name: "kelebihan", Label: "x", Expression: "1"}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("melewati batas harus ErrInvalidInput, got %v", err)
	}
}

func TestMutationScope(t *testing.T) {
	s, _, _ := newSvc()
	ctx := context.Background()
	own, _ := s.Create(ctx, adminA, Input{Name: "milik_a", Label: "A", Expression: "1"})
	glob, _ := s.Create(ctx, super, Input{Name: "milik_global", Label: "G", Expression: "1"})

	// tenant lain: tidak boleh baca/ubah/hapus
	if _, err := s.Get(ctx, adminB, own.UUID); !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("Get lintas tenant: %v", err)
	}
	if _, err := s.Update(ctx, adminB, own.UUID, UpdateInput{Label: sp("hack")}); !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("Update lintas tenant: %v", err)
	}
	if err := s.Delete(ctx, adminB, own.UUID); !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("Delete lintas tenant: %v", err)
	}
	// global: semua tenant boleh BACA, hanya superadmin yang boleh ubah/hapus
	if _, err := s.Get(ctx, adminB, glob.UUID); err != nil {
		t.Errorf("baca global harus boleh: %v", err)
	}
	if _, err := s.Update(ctx, adminA, glob.UUID, UpdateInput{Label: sp("hack")}); !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("ADMIN mengubah global harus ErrForbidden: %v", err)
	}
	if err := s.Delete(ctx, adminA, glob.UUID); !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("ADMIN menghapus global harus ErrForbidden: %v", err)
	}
	// pemilik: ubah (partial) & hapus
	got, err := s.Update(ctx, adminA, own.UUID, UpdateInput{Label: sp("Baru"), Expression: sp("2 * 3"), IsActive: ptr(false)})
	if err != nil || got.Label != "Baru" || got.Expression != "2 * 3" || got.IsActive || got.Name != "milik_a" {
		t.Errorf("update partial salah: %+v %v", got, err)
	}
	if _, err := s.Update(ctx, adminA, own.UUID, UpdateInput{Expression: sp("{" + "Device.WiFi.AccessPoint.1.Security.KeyPassphrase" + "}")}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("update ke rujukan rahasia harus ditolak: %v", err)
	}
	if err := s.Delete(ctx, adminA, own.UUID); err != nil {
		t.Errorf("delete: %v", err)
	}
	// List: tenant melihat milik sendiri + global; tenant_id dari klien diabaikan
	if _, _, err := s.List(ctx, domain.Actor{Roles: []string{domain.RoleAdmin}}, nil, domain.Pagination{}); !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("List tanpa tenant harus ErrForbidden: %v", err)
	}
	rows, _, _ := s.List(ctx, adminB, u64(7), domain.Pagination{})
	for _, r := range rows {
		if r.TenantID != nil && *r.TenantID != 8 {
			t.Errorf("List bocor tenant lain: %+v", r)
		}
	}
}

func ptr[T any](v T) *T { return &v }

func TestForDevice_HitungDanTimpaGlobal(t *testing.T) {
	s, _, _ := newSvc()
	ctx := context.Background()
	_, _ = s.Create(ctx, super, Input{Name: "rx_dbm", Label: "RX global", Expression: "{" + rxPath + "} / 100", Unit: sp("dBm")})
	_, _ = s.Create(ctx, adminA, Input{Name: "rx_dbm", Label: "RX tenant A", Expression: "round({" + rxPath + "} / 1000, 1)", Unit: sp("dBm")})
	_, _ = s.Create(ctx, adminA, Input{Name: "uptime_hari", Label: "Uptime", Expression: "round({" + upPath + "} / 86400, 2)", Unit: sp("hari")})
	_, _ = s.Create(ctx, adminA, Input{Name: "nonaktif", Label: "x", Expression: "1", IsActive: ptr(false)})

	vals, err := s.ForDevice(ctx, adminA, 1)
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]domain.VirtualValue{}
	for _, v := range vals {
		by[v.Name] = v
	}
	if len(by) != 2 {
		t.Fatalf("hanya definisi AKTIF yang berlaku: %+v", vals)
	}
	if by["rx_dbm"].Label != "RX tenant A" || by["rx_dbm"].Value == nil || *by["rx_dbm"].Value != "-21.5" {
		t.Errorf("definisi tenant harus menimpa global: %+v", by["rx_dbm"])
	}
	if by["uptime_hari"].Value == nil || *by["uptime_hari"].Value != "2" || by["uptime_hari"].Unit == nil || *by["uptime_hari"].Unit != "hari" {
		t.Errorf("uptime_hari: %+v", by["uptime_hari"])
	}
	// perangkat 2 tak punya UpTime -> nilai nil (bukan "" / 0)
	vals2, _ := s.ForDevice(ctx, adminA, 2)
	for _, v := range vals2 {
		if v.Name == "uptime_hari" && v.Value != nil {
			t.Errorf("parameter sumber belum ada -> nilai harus nil, got %q", *v.Value)
		}
	}
	// scope: tenant lain tidak bisa membaca perangkat ini
	if _, err := s.ForDevice(ctx, adminB, 1); !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("ForDevice lintas tenant harus ErrForbidden: %v", err)
	}
	// tenant B hanya melihat global (definisi tenant A tidak berlaku)
	valsB, _ := s.ForDevice(ctx, adminB, 3)
	if len(valsB) != 1 || valsB[0].Label != "RX global" {
		t.Errorf("tenant B hanya global: %+v", valsB)
	}
}

func TestProject_SatuQueryBatchDanPerTenant(t *testing.T) {
	s, _, params := newSvc()
	ctx := context.Background()
	_, _ = s.Create(ctx, adminA, Input{Name: "rx_dbm", Label: "RX", Expression: "{" + rxPath + "} / 1000"})
	_, _ = s.Create(ctx, adminA, Input{Name: "uptime_hari", Label: "Up", Expression: "{" + upPath + "} / 86400"})
	_, _ = s.Create(ctx, adminB, Input{Name: "rx_dbm", Label: "RX B", Expression: "{" + upPath + "}"}) // definisi berbeda untuk tenant B

	items := []domain.DeviceListItem{
		{Device: domain.Device{ID: 1, TenantID: u64(7)}},
		{Device: domain.Device{ID: 2, TenantID: u64(7)}},
		{Device: domain.Device{ID: 3, TenantID: u64(8)}},
	}
	s.Project(ctx, items, []string{"rx_dbm", "uptime_hari", "tak_ada"})
	if params.calls != 1 {
		t.Errorf("harus 1 query batch untuk satu halaman, got %d", params.calls)
	}
	if items[0].Virtual["rx_dbm"] != "-21.5" || items[0].Virtual["uptime_hari"] != "2" {
		t.Errorf("device 1: %v", items[0].Virtual)
	}
	if items[1].Virtual["rx_dbm"] != "-27.4" || len(items[1].Virtual) != 1 {
		t.Errorf("device 2 (tanpa UpTime): %v", items[1].Virtual)
	}
	if items[2].Virtual["rx_dbm"] != "86400" || len(items[2].Virtual) != 1 {
		t.Errorf("device 3 (tenant B pakai definisinya sendiri): %v", items[2].Virtual)
	}
	// tanpa nama / tanpa item = tidak ada query
	params.calls = 0
	s.Project(ctx, items, nil)
	s.Project(ctx, nil, []string{"rx_dbm"})
	if params.calls != 0 {
		t.Errorf("tanpa permintaan tidak boleh query, got %d", params.calls)
	}
}

func TestTest_HasilDanKesalahan(t *testing.T) {
	s, _, _ := newSvc()
	ctx := context.Background()
	res, err := s.Test(ctx, adminA, "round({"+rxPath+"} / 1000, 1) + {Hilang.Param}", 1)
	if err != nil || !res.OK || res.Value != nil || len(res.Refs) != 2 {
		t.Fatalf("null menjalar: %+v %v", res, err)
	}
	if res.Refs[0].Value == nil || *res.Refs[0].Value != "-21500" || res.Refs[1].Value != nil {
		t.Errorf("refs: %+v", res.Refs)
	}
	res, _ = s.Test(ctx, adminA, "round({"+rxPath+"} / 1000, 1)", 1)
	if !res.OK || res.Value == nil || *res.Value != "-21.5" {
		t.Errorf("nilai: %+v", res)
	}
	// kesalahan sintaks = hasil (OK=false) dengan posisi, bukan error
	res, err = s.Test(ctx, adminA, "1 + * 2", 1)
	if err != nil || res.OK || res.Error == nil || res.Error.Message == "" {
		t.Errorf("sintaks salah: %+v %v", res, err)
	}
	// rujukan rahasia ditolak sebagai hasil
	res, _ = s.Test(ctx, adminA, "{Device.WiFi.AccessPoint.1.Security.KeyPassphrase}", 1)
	if res.OK || res.Error == nil {
		t.Errorf("rahasia harus ditolak: %+v", res)
	}
	// scope perangkat
	if _, err := s.Test(ctx, adminB, "1", 1); !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("Test pada perangkat tenant lain harus ErrForbidden: %v", err)
	}
	if _, err := s.Test(ctx, adminA, "1", 999); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("perangkat tak ada harus ErrNotFound: %v", err)
	}
}
