package catalog

import (
	"context"
	"errors"
	"strings"
	"testing"

	"acs/internal/domain"
)

// --- fakes (embed interface; hanya method yang dipakai usecase) ---

type fakeVendors struct {
	domain.VendorRepository
	byID    map[uint64]*domain.Vendor
	updated *domain.Vendor
	deleted []uint64
}

func (f *fakeVendors) GetByID(_ context.Context, id uint64) (*domain.Vendor, error) {
	if v, ok := f.byID[id]; ok {
		cp := *v
		return &cp, nil
	}
	return nil, domain.ErrNotFound
}
func (f *fakeVendors) Update(_ context.Context, v *domain.Vendor) error { f.updated = v; return nil }
func (f *fakeVendors) SoftDelete(_ context.Context, id, _ uint64) error {
	f.deleted = append(f.deleted, id)
	return nil
}

type fakeOUIs struct {
	domain.VendorOUIRepository
	byVendor map[uint64][]domain.VendorOUI
	deleted  []uint64
}

func (f *fakeOUIs) ListByVendor(_ context.Context, vendorID uint64) ([]domain.VendorOUI, error) {
	return f.byVendor[vendorID], nil
}
func (f *fakeOUIs) Delete(_ context.Context, id, _ uint64) error {
	f.deleted = append(f.deleted, id)
	return nil
}

type fakeModels struct {
	domain.DeviceModelRepository
	byID    map[uint64]*domain.DeviceModel
	updated *domain.DeviceModel
	deleted []uint64
}

func (f *fakeModels) GetByID(_ context.Context, id uint64) (*domain.DeviceModel, error) {
	if m, ok := f.byID[id]; ok {
		cp := *m
		return &cp, nil
	}
	return nil, domain.ErrNotFound
}
func (f *fakeModels) Update(_ context.Context, m *domain.DeviceModel) error {
	f.updated = m
	return nil
}
func (f *fakeModels) SoftDelete(_ context.Context, id, _ uint64) error {
	f.deleted = append(f.deleted, id)
	return nil
}

type fakeMappings struct {
	domain.VendorParameterMappingRepository
	exists  map[uint64]bool
	deleted []uint64
}

func (f *fakeMappings) GetByID(_ context.Context, id uint64) (*domain.VendorParameterMapping, error) {
	if f.exists[id] {
		return &domain.VendorParameterMapping{ID: id}, nil
	}
	return nil, domain.ErrNotFound
}
func (f *fakeMappings) Delete(_ context.Context, id, _ uint64) error {
	f.deleted = append(f.deleted, id)
	return nil
}

type fakeUsage struct {
	domain.UsageRepository
	vendor, model []domain.UsageCount
}

func (f *fakeUsage) VendorUsage(context.Context, uint64) ([]domain.UsageCount, error) {
	return f.vendor, nil
}
func (f *fakeUsage) DeviceModelUsage(context.Context, uint64) ([]domain.UsageCount, error) {
	return f.model, nil
}

type fakeActivity struct{ actions []string }

func (f *fakeActivity) Record(_ context.Context, a *domain.ActivityLog) error {
	f.actions = append(f.actions, a.Action)
	return nil
}
func (f *fakeActivity) ListByEntity(context.Context, string, uint64, domain.Pagination) ([]domain.ActivityLog, int, error) {
	return nil, 0, nil
}
func (f *fakeActivity) ListByTenant(context.Context, *uint64, domain.ActivityLogFilter, domain.Pagination) ([]domain.ActivityLog, int, error) {
	return nil, 0, nil
}

var (
	superadmin = domain.Actor{UserID: 1, Roles: []string{domain.RoleSuperadmin}}
	tenantAdm  = domain.Actor{UserID: 2, TenantID: u64(7), Roles: []string{domain.RoleAdmin}}
)

func u64(v uint64) *uint64 { return &v }

type rig struct {
	svc      *Service
	vendors  *fakeVendors
	ouis     *fakeOUIs
	models   *fakeModels
	mappings *fakeMappings
	usage    *fakeUsage
	act      *fakeActivity
}

func newRig() *rig {
	r := &rig{
		vendors:  &fakeVendors{byID: map[uint64]*domain.Vendor{1: {ID: 1, Code: "ZTE", Name: "ZTE Corporation", IsActive: true}}},
		ouis:     &fakeOUIs{byVendor: map[uint64][]domain.VendorOUI{1: {{ID: 10, VendorID: 1, OUI: "0015EB"}, {ID: 11, VendorID: 1, OUI: "0019C6"}}, 2: {{ID: 20, VendorID: 2, OUI: "001882"}}}},
		models:   &fakeModels{byID: map[uint64]*domain.DeviceModel{5: {ID: 5, VendorID: 1, ModelName: "F670L", IsActive: true}}},
		mappings: &fakeMappings{exists: map[uint64]bool{9: true}},
		usage:    &fakeUsage{},
		act:      &fakeActivity{},
	}
	r.svc = NewService(r.vendors, r.ouis, r.models, r.mappings, r.usage, r.act)
	return r
}

func TestDeleteVendor_HanyaSuperadmin(t *testing.T) {
	r := newRig()
	if err := r.svc.DeleteVendor(context.Background(), tenantAdm, 1); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("want ErrForbidden, got %v", err)
	}
	if len(r.vendors.deleted) != 0 {
		t.Fatal("non-superadmin tidak boleh menghapus apa pun")
	}
}

func TestDeleteVendor_MasihDipakai409TanpaMenghapusApaPun(t *testing.T) {
	r := newRig()
	r.usage.vendor = []domain.UsageCount{{Kind: "device", Count: 3}, {Kind: "device_model", Count: 1}}

	err := r.svc.DeleteVendor(context.Background(), superadmin, 1)
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("want ErrConflict, got %v", err)
	}
	for _, want := range []string{"3 device", "1 device_model", "is_active=false"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("pesan konflik harus menuntun, tidak memuat %q: %v", want, err)
		}
	}
	// Kegagalan guard tidak boleh meninggalkan efek samping setengah jalan.
	if len(r.vendors.deleted) != 0 || len(r.ouis.deleted) != 0 {
		t.Fatalf("tidak boleh ada penghapusan: vendors=%v ouis=%v", r.vendors.deleted, r.ouis.deleted)
	}
}

func TestDeleteVendor_BebasDipakai_OUIIkutTerhapus(t *testing.T) {
	r := newRig()
	if err := r.svc.DeleteVendor(context.Background(), superadmin, 1); err != nil {
		t.Fatal(err)
	}
	if len(r.vendors.deleted) != 1 || r.vendors.deleted[0] != 1 {
		t.Fatalf("vendor harus di-soft-delete, got %v", r.vendors.deleted)
	}
	if len(r.ouis.deleted) != 2 {
		t.Fatalf("kedua OUI vendor harus ikut dihapus, got %v", r.ouis.deleted)
	}
	if len(r.act.actions) != 1 || r.act.actions[0] != "DELETE_VENDOR" {
		t.Fatalf("audit trail: %v", r.act.actions)
	}
}

func TestDeleteVendor_TidakAda404(t *testing.T) {
	r := newRig()
	if err := r.svc.DeleteVendor(context.Background(), superadmin, 999); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestDeleteVendorOUI_MilikVendorLainDianggap404(t *testing.T) {
	r := newRig()
	// OUI 20 milik vendor 2, bukan vendor 1 — jangan bocorkan & jangan hapus.
	if err := r.svc.DeleteVendorOUI(context.Background(), superadmin, 1, 20); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	if len(r.ouis.deleted) != 0 {
		t.Fatal("OUI vendor lain tidak boleh terhapus")
	}
	if err := r.svc.DeleteVendorOUI(context.Background(), superadmin, 1, 10); err != nil {
		t.Fatal(err)
	}
	if len(r.ouis.deleted) != 1 || r.ouis.deleted[0] != 10 {
		t.Fatalf("OUI 10 harus terhapus, got %v", r.ouis.deleted)
	}
}

func TestUpdateVendor_ParsialDanCodeTidakBerubah(t *testing.T) {
	r := newRig()
	newName := "ZTE Corp (renamed)"
	inactive := false
	v, err := r.svc.UpdateVendor(context.Background(), superadmin, 1, UpdateVendorInput{Name: &newName, IsActive: &inactive})
	if err != nil {
		t.Fatal(err)
	}
	if v.Name != newName || v.IsActive || v.Code != "ZTE" {
		t.Fatalf("hasil update salah: %+v", v)
	}
	if r.vendors.updated == nil || r.vendors.updated.UpdatedBy == nil || *r.vendors.updated.UpdatedBy != 1 {
		t.Fatal("updated_by harus terisi (audit trail 7 kolom)")
	}

	empty := "  "
	if _, err := r.svc.UpdateVendor(context.Background(), superadmin, 1, UpdateVendorInput{Name: &empty}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("name kosong: want ErrInvalidInput, got %v", err)
	}
	if _, err := r.svc.UpdateVendor(context.Background(), tenantAdm, 1, UpdateVendorInput{Name: &newName}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("non-superadmin: want ErrForbidden, got %v", err)
	}
}

func TestDeviceModel_UpdateDanDeleteGuard(t *testing.T) {
	r := newRig()

	pc := ""
	desc := "ONT GPON"
	m, err := r.svc.UpdateDeviceModel(context.Background(), superadmin, 5, UpdateDeviceModelInput{ProductClass: &pc, Description: &desc})
	if err != nil {
		t.Fatal(err)
	}
	if m.ProductClass != nil || m.Description == nil || *m.Description != "ONT GPON" || m.VendorID != 1 {
		t.Fatalf("update model salah (vendor_id harus tetap, product_class '' = kosongkan): %+v", m)
	}

	r.usage.model = []domain.UsageCount{{Kind: "device", Count: 4}}
	if err := r.svc.DeleteDeviceModel(context.Background(), superadmin, 5); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("model dipakai: want ErrConflict, got %v", err)
	}
	if len(r.models.deleted) != 0 {
		t.Fatal("model yang dipakai tidak boleh terhapus")
	}

	r.usage.model = nil
	if err := r.svc.DeleteDeviceModel(context.Background(), superadmin, 5); err != nil {
		t.Fatal(err)
	}
	if len(r.models.deleted) != 1 {
		t.Fatalf("model bebas harus terhapus, got %v", r.models.deleted)
	}
}

func TestDeleteParameterMapping(t *testing.T) {
	r := newRig()
	if err := r.svc.DeleteParameterMapping(context.Background(), superadmin, 404); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	if err := r.svc.DeleteParameterMapping(context.Background(), tenantAdm, 9); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("want ErrForbidden, got %v", err)
	}
	if err := r.svc.DeleteParameterMapping(context.Background(), superadmin, 9); err != nil {
		t.Fatal(err)
	}
	if len(r.mappings.deleted) != 1 || r.mappings.deleted[0] != 9 {
		t.Fatalf("mapping 9 harus terhapus, got %v", r.mappings.deleted)
	}
}
