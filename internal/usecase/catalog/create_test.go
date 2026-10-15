package catalog

import (
	"context"
	"errors"
	"testing"

	"acs/internal/domain"
)

type recOUIs struct {
	*fakeOUIs
	created []*domain.VendorOUI
}

func (f *recOUIs) Create(_ context.Context, o *domain.VendorOUI) error {
	o.ID = 100 + uint64(len(f.created))
	f.created = append(f.created, o)
	return nil
}

type recModels struct {
	*fakeModels
	created []*domain.DeviceModel
}

func (f *recModels) Create(_ context.Context, m *domain.DeviceModel) error {
	m.ID = 200 + uint64(len(f.created))
	f.created = append(f.created, m)
	return nil
}

type recMappings struct {
	*fakeMappings
	upserted []*domain.VendorParameterMapping
}

func (f *recMappings) Upsert(_ context.Context, m *domain.VendorParameterMapping) error {
	m.ID = 300
	f.upserted = append(f.upserted, m)
	return nil
}

func createRig() (*Service, *recOUIs, *recModels, *recMappings, *fakeModels, *fakeUsage) {
	base := newRig()
	// vendor 1 aktif; vendor 2 ada tetapi sudah dihapus -> tidak ada di fakeVendors (GetByID=404)
	ouis := &recOUIs{fakeOUIs: base.ouis}
	models := &recModels{fakeModels: base.models}
	maps := &recMappings{fakeMappings: base.mappings}
	// model 5 milik vendor 1; model 6 milik vendor 9 (vendor lain)
	base.models.byID[6] = &domain.DeviceModel{ID: 6, VendorID: 9, ModelName: "Lain", DeviceTypeID: 1, DataModelVersionID: 1}
	base.models.byID[5].DeviceTypeID, base.models.byID[5].DataModelVersionID = 1, 1
	svc := NewService(base.vendors, ouis, models, maps, base.usage, base.act)
	return svc, ouis, models, maps, base.models, base.usage
}

func TestAddVendorOUI(t *testing.T) {
	svc, ouis, _, _, _, _ := createRig()
	ctx := context.Background()

	if _, err := svc.AddVendorOUI(ctx, tenantAdm, 1, "ABCDEF", nil); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("non-superadmin: %v", err)
	}
	if _, err := svc.AddVendorOUI(ctx, superadmin, 2, "ABCDEF", nil); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("vendor terhapus/tak ada harus 404 (bukan 500 dari FK): %v", err)
	}
	if _, err := svc.AddVendorOUI(ctx, superadmin, 1, "  ", nil); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("oui kosong: %v", err)
	}
	if len(ouis.created) != 0 {
		t.Fatal("penolakan tidak boleh membuat OUI")
	}
	o, err := svc.AddVendorOUI(ctx, superadmin, 1, "ABCDEF", nil)
	if err != nil || o.VendorID != 1 || o.CreatedBy == nil || *o.CreatedBy != 1 || len(ouis.created) != 1 {
		t.Fatalf("o=%+v err=%v", o, err)
	}
}

func TestCreateDeviceModel(t *testing.T) {
	svc, _, models, _, _, _ := createRig()
	ctx := context.Background()
	mk := func(vendor uint64) *domain.DeviceModel {
		return &domain.DeviceModel{VendorID: vendor, ModelName: "X", DeviceTypeID: 1, DataModelVersionID: 1}
	}
	if err := svc.CreateDeviceModel(ctx, tenantAdm, mk(1)); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("%v", err)
	}
	if err := svc.CreateDeviceModel(ctx, superadmin, mk(2)); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("vendor terhapus: %v", err)
	}
	bad := mk(1)
	bad.DataModelVersionID = 0
	if err := svc.CreateDeviceModel(ctx, superadmin, bad); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("data_model_version_id 0 harus 400: %v", err)
	}
	noName := mk(1)
	noName.ModelName = " "
	if err := svc.CreateDeviceModel(ctx, superadmin, noName); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("model_name kosong: %v", err)
	}
	if len(models.created) != 0 {
		t.Fatal("penolakan tidak boleh membuat model")
	}
	m := mk(1)
	if err := svc.CreateDeviceModel(ctx, superadmin, m); err != nil || !m.IsActive || m.ID == 0 {
		t.Fatalf("m=%+v err=%v", m, err)
	}
}

func TestUpsertParameterMapping(t *testing.T) {
	svc, _, _, maps, _, _ := createRig()
	ctx := context.Background()
	mk := func(vendor uint64, model *uint64) *domain.VendorParameterMapping {
		return &domain.VendorParameterMapping{VendorID: vendor, DeviceModelID: model, LogicalKey: "wifi.ssid", TR069Path: "X.SSID"}
	}
	u := func(v uint64) *uint64 { return &v }

	if err := svc.UpsertParameterMapping(ctx, tenantAdm, mk(1, nil)); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("%v", err)
	}
	if err := svc.UpsertParameterMapping(ctx, superadmin, mk(2, nil)); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("vendor terhapus: %v", err)
	}
	if err := svc.UpsertParameterMapping(ctx, superadmin, mk(1, u(6))); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("model milik vendor lain harus 400: %v", err)
	}
	if err := svc.UpsertParameterMapping(ctx, superadmin, mk(1, u(404))); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("model tak ada: %v", err)
	}
	empty := mk(1, nil)
	empty.TR069Path = ""
	if err := svc.UpsertParameterMapping(ctx, superadmin, empty); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("path kosong: %v", err)
	}
	if len(maps.upserted) != 0 {
		t.Fatal("penolakan tidak boleh menyimpan mapping")
	}
	if err := svc.UpsertParameterMapping(ctx, superadmin, mk(1, u(5))); err != nil || len(maps.upserted) != 1 {
		t.Fatalf("model milik vendor yang sama harus lolos: %v", err)
	}
}

func TestUpdateDeviceModel_GuardDataModelBilaDipakaiDevice(t *testing.T) {
	svc, _, _, _, _, usage := createRig()
	ctx := context.Background()
	newDMV := uint64(2)

	usage.model = []domain.UsageCount{{Kind: "device", Count: 3}}
	if _, err := svc.UpdateDeviceModel(ctx, superadmin, 5, UpdateDeviceModelInput{DataModelVersionID: &newDMV}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("ubah data model saat dipakai device harus 409: %v", err)
	}
	// nilai yang SAMA dengan sekarang bukan perubahan -> lolos
	same := uint64(1)
	if _, err := svc.UpdateDeviceModel(ctx, superadmin, 5, UpdateDeviceModelInput{DataModelVersionID: &same}); err != nil {
		t.Fatalf("nilai sama bukan perubahan: %v", err)
	}
	// field lain selalu boleh walau dipakai device
	desc := "uji"
	if _, err := svc.UpdateDeviceModel(ctx, superadmin, 5, UpdateDeviceModelInput{Description: &desc}); err != nil {
		t.Fatalf("%v", err)
	}
	// dipakai selain device (mis. hanya firmware) tidak menghalangi ubah data model
	usage.model = []domain.UsageCount{{Kind: "firmware", Count: 1}}
	if _, err := svc.UpdateDeviceModel(ctx, superadmin, 5, UpdateDeviceModelInput{DataModelVersionID: &newDMV}); err != nil {
		t.Fatalf("tanpa device boleh: %v", err)
	}
}
