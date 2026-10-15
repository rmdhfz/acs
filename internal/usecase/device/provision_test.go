package device

import (
	"context"
	"errors"
	"strings"
	"testing"

	"acs/internal/domain"
	"acs/pkg/cryptoutil"
)

// ---- fakes (menambah di atas fakeRefs/delActivity/ptr dari test lain di paket ini) ----

type pvDevices struct {
	domain.DeviceRepository
	byKey   map[string]*domain.Device // oui|serial -> device aktif
	deleted map[string]uint64         // oui|serial -> tenant pemilik baris TERHAPUS (revive lintas tenant ditolak)
	nextID  uint64
	creates int
	updates int
}

func (f *pvDevices) GetByOUISerial(_ context.Context, oui, serial string) (*domain.Device, error) {
	if d, ok := f.byKey[oui+"|"+serial]; ok {
		return d, nil
	}
	return nil, domain.ErrNotFound
}
func (f *pvDevices) Create(_ context.Context, d *domain.Device) error {
	key := *d.OUI + "|" + d.SerialNumber
	if owner, ok := f.deleted[key]; ok && (d.TenantID == nil || *d.TenantID != owner) {
		return domain.ErrConflict
	}
	f.nextID++
	d.ID = f.nextID
	cp := *d
	f.byKey[key] = &cp
	f.creates++
	return nil
}
func (f *pvDevices) Update(_ context.Context, d *domain.Device) error {
	f.updates++
	key := *d.OUI + "|" + d.SerialNumber
	cp := *d
	f.byKey[key] = &cp
	return nil
}

type pvVendors struct {
	domain.VendorRepository
	ok map[uint64]bool
}

func (f *pvVendors) GetByID(_ context.Context, id uint64) (*domain.Vendor, error) {
	if f.ok[id] {
		return &domain.Vendor{ID: id}, nil
	}
	return nil, domain.ErrNotFound
}

type pvTenants struct {
	domain.TenantRepository
	ok map[uint64]bool
}

func (f *pvTenants) GetByID(_ context.Context, id uint64) (*domain.Tenant, error) {
	if f.ok[id] {
		return &domain.Tenant{ID: id}, nil
	}
	return nil, domain.ErrNotFound
}

type pvModels struct {
	domain.DeviceModelRepository
	byID map[uint64]uint64 // model -> vendor
}

func (f *pvModels) GetByID(_ context.Context, id uint64) (*domain.DeviceModel, error) {
	if v, ok := f.byID[id]; ok {
		return &domain.DeviceModel{ID: id, VendorID: v}, nil
	}
	return nil, domain.ErrNotFound
}
func (f *pvModels) FindByVendorAndProductClass(context.Context, uint64, string) (*domain.DeviceModel, error) {
	return nil, domain.ErrNotFound
}

type pvOUIs struct{ domain.VendorOUIRepository }

func (pvOUIs) GetByOUI(context.Context, string) (*domain.VendorOUI, error) {
	return nil, domain.ErrNotFound
}

func pvRig(t *testing.T) (*Service, *pvDevices, *delActivity) {
	t.Helper()
	enc, err := cryptoutil.NewEncryptor([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	devs := &pvDevices{byKey: map[string]*domain.Device{}, deleted: map[string]uint64{}}
	act := &delActivity{}
	svc := &Service{
		devices: devs, refs: fakeRefs{}, enc: enc, activity: act,
		vendorOUIs: pvOUIs{}, deviceModels: &pvModels{byID: map[uint64]uint64{5: 1}},
	}
	svc.WithCatalog(&pvVendors{ok: map[uint64]bool{1: true, 2: true}}, &pvTenants{ok: map[uint64]bool{7: true, 8: true}})
	return svc, devs, act
}

var (
	adminT7 = domain.Actor{UserID: 5, TenantID: ptr(uint64(7)), Roles: []string{domain.RoleAdmin}}
	super   = domain.Actor{UserID: 1, Roles: []string{domain.RoleSuperadmin}}
	viewer  = domain.Actor{UserID: 9, TenantID: ptr(uint64(7)), Roles: []string{domain.RoleViewer}}
)

// ---- normalisasi ----

func TestNormalizeCreate(t *testing.T) {
	got, err := normalizeCreate(CreateDeviceInput{
		OUI: " 00d0d0 ", SerialNumber: "  SN-1 ", MACAddress: ptr("aa-bb-cc-dd-ee-ff"), ProductClass: ptr("  "), Notes: ptr(""),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.OUI != "00D0D0" || got.SerialNumber != "SN-1" || got.MACAddress == nil || *got.MACAddress != "AA:BB:CC:DD:EE:FF" {
		t.Fatalf("normalisasi salah: %+v", got)
	}
	if got.ProductClass != nil || got.Notes != nil {
		t.Fatal("string kosong pada field opsional harus jadi nil")
	}

	bad := map[string]CreateDeviceInput{
		"oui pendek":         {OUI: "00D0", SerialNumber: "x"},
		"oui simbol":         {OUI: "00D0D!", SerialNumber: "x"},
		"serial kosong":      {OUI: "00D0D0", SerialNumber: "  "},
		"serial kepanjangan": {OUI: "00D0D0", SerialNumber: strings.Repeat("a", 129)},
		"serial kontrol":     {OUI: "00D0D0", SerialNumber: "a\x00b"},
		"mac salah":          {OUI: "00D0D0", SerialNumber: "x", MACAddress: ptr("12:34")},
		"lat tanpa lon":      {OUI: "00D0D0", SerialNumber: "x", Latitude: ptr(1.0)},
		"lat di luar":        {OUI: "00D0D0", SerialNumber: "x", Latitude: ptr(91.0), Longitude: ptr(0.0)},
		"lon di luar":        {OUI: "00D0D0", SerialNumber: "x", Latitude: ptr(0.0), Longitude: ptr(181.0)},
		"cr url ftp":         {OUI: "00D0D0", SerialNumber: "x", ConnectionRequestURL: ptr("ftp://1.2.3.4/")},
		"notes panjang":      {OUI: "00D0D0", SerialNumber: "x", Notes: ptr(strings.Repeat("n", 256))},
	}
	for name, in := range bad {
		if _, err := normalizeCreate(in); !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("%s: harus ErrInvalidInput, dapat %v", name, err)
		}
	}
}

// ---- Create ----

func TestCreate_RBACDanTenant(t *testing.T) {
	svc, devs, act := pvRig(t)
	ctx := context.Background()
	in := CreateDeviceInput{OUI: "00d0d0", SerialNumber: "SN1", Notes: ptr("rak A"), Latitude: ptr(-6.2), Longitude: ptr(106.8)}

	if _, err := svc.Create(ctx, viewer, in); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("VIEWER tidak boleh membuat: %v", err)
	}
	// ADMIN dikunci ke tenant sendiri.
	other := in
	other.TenantID = ptr(uint64(8))
	if _, err := svc.Create(ctx, adminT7, other); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("ADMIN tidak boleh menarget tenant lain: %v", err)
	}
	d, err := svc.Create(ctx, adminT7, in)
	if err != nil {
		t.Fatal(err)
	}
	if d.TenantID == nil || *d.TenantID != 7 || *d.OUI != "00D0D0" || d.Latitude == nil {
		t.Fatalf("hasil salah: %+v", d)
	}
	if devs.updates != 1 {
		t.Fatalf("Create harus menulis ulang nilai operator (kasus revive): updates=%d", devs.updates)
	}
	if len(act.got) != 1 || act.got[0].Action != "CREATE_DEVICE" {
		t.Fatalf("audit CREATE_DEVICE tidak tercatat: %+v", act.got)
	}
	// Duplikat → 409.
	if _, err := svc.Create(ctx, adminT7, in); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("duplikat harus ErrConflict: %v", err)
	}
	// Superadmin wajib menyebut tenant; tenant tak ada ditolak.
	in2 := CreateDeviceInput{OUI: "00D0D1", SerialNumber: "SN2"}
	if _, err := svc.Create(ctx, super, in2); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("superadmin tanpa tenant: %v", err)
	}
	in2.TenantID = ptr(uint64(99))
	if _, err := svc.Create(ctx, super, in2); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("tenant tak ada: %v", err)
	}
	in2.TenantID = ptr(uint64(8))
	if d, err := svc.Create(ctx, super, in2); err != nil || *d.TenantID != 8 {
		t.Fatalf("superadmin dgn tenant valid: %v", err)
	}
}

func TestCreate_ValidasiVendorModelDanCredEnkripsi(t *testing.T) {
	svc, devs, _ := pvRig(t)
	ctx := context.Background()
	base := CreateDeviceInput{OUI: "00D0D0", SerialNumber: "SN1"}

	bad := base
	bad.VendorID = ptr(uint64(99))
	if _, err := svc.Create(ctx, adminT7, bad); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("vendor tak ada: %v", err)
	}
	bad = base
	bad.VendorID, bad.DeviceModelID = ptr(uint64(2)), ptr(uint64(5)) // model 5 milik vendor 1
	if _, err := svc.Create(ctx, adminT7, bad); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("model bukan milik vendor: %v", err)
	}
	ok := base
	ok.DeviceModelID = ptr(uint64(5)) // vendor diturunkan dari model
	ok.ConnectionRequestUsername, ok.ConnectionRequestPassword = ptr("acs"), ptr("rahasia-cr")
	d, err := svc.Create(ctx, adminT7, ok)
	if err != nil {
		t.Fatal(err)
	}
	if d.VendorID == nil || *d.VendorID != 1 {
		t.Fatalf("vendor harus diturunkan dari model: %v", d.VendorID)
	}
	stored := devs.byKey["00D0D0|SN1"]
	if len(stored.ConnectionRequestPasswordEnc) == 0 || strings.Contains(string(stored.ConnectionRequestPasswordEnc), "rahasia-cr") {
		t.Fatal("password connection request harus tersimpan terenkripsi, bukan plaintext")
	}
}

func TestCreate_BarisTerhapusTenantLainDitolak(t *testing.T) {
	svc, devs, _ := pvRig(t)
	devs.deleted["00D0D0|SN1"] = 8 // pernah dimiliki tenant 8 lalu dihapus
	_, err := svc.Create(context.Background(), adminT7, CreateDeviceInput{OUI: "00D0D0", SerialNumber: "SN1"})
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("revive lintas tenant harus ErrConflict: %v", err)
	}
	if strings.Contains(err.Error(), "8") && strings.Contains(err.Error(), "tenant 8") {
		t.Fatal("jangan membocorkan tenant pemilik lama")
	}
}

// ---- Import ----

func importRows() []CreateDeviceInput {
	return []CreateDeviceInput{
		{OUI: "00D0D0", SerialNumber: "A1", Notes: ptr("baru")},            // 0 create
		{OUI: "00D0D0", SerialNumber: "EXIST", Notes: ptr("catatan baru")}, // 1 update (notes beda)
		{OUI: "00D0D0", SerialNumber: "SAME", Notes: ptr("sama")},          // 2 unchanged
		{OUI: "BAD", SerialNumber: "X"},                                    // 3 error oui
		{OUI: "00D0D0", SerialNumber: "A1"},                                // 4 error duplikat dalam file
		{OUI: "00D0D0", SerialNumber: "OTHERTENANT"},                       // 5 error tenant lain
		{OUI: "00D0D0", SerialNumber: "A2", VendorID: ptr(uint64(99))},     // 6 error vendor
	}
}

func seedExisting(devs *pvDevices) {
	devs.byKey["00D0D0|EXIST"] = &domain.Device{ID: 100, TenantID: ptr(uint64(7)), OUI: ptr("00D0D0"), SerialNumber: "EXIST", Notes: ptr("lama")}
	devs.byKey["00D0D0|SAME"] = &domain.Device{ID: 101, TenantID: ptr(uint64(7)), OUI: ptr("00D0D0"), SerialNumber: "SAME", Notes: ptr("sama")}
	devs.byKey["00D0D0|OTHERTENANT"] = &domain.Device{ID: 102, TenantID: ptr(uint64(8)), OUI: ptr("00D0D0"), SerialNumber: "OTHERTENANT"}
}

func TestImport_DryRunKlasifikasiTanpaMenulis(t *testing.T) {
	svc, devs, act := pvRig(t)
	seedExisting(devs)

	res, err := svc.Import(context.Background(), adminT7, ImportInput{Mode: ImportModeUpdate, DryRun: true, Rows: importRows()})
	if err != nil {
		t.Fatal(err)
	}
	if res.Created != 1 || res.Updated != 1 || res.Unchanged != 1 || res.Failed != 4 || res.Skipped != 0 {
		t.Fatalf("ringkasan salah: %+v", res)
	}
	want := []string{ActionCreate, ActionUpdate, ActionUnchanged, ActionError, ActionError, ActionError, ActionError}
	for i, w := range want {
		if res.Results[i].Action != w {
			t.Errorf("baris %d: action=%s, mau %s (%s)", i, res.Results[i].Action, w, res.Results[i].Error)
		}
	}
	if len(res.Results[1].Changes) != 1 || res.Results[1].Changes[0] != "notes" {
		t.Errorf("changes baris update: %v", res.Results[1].Changes)
	}
	if !strings.Contains(res.Results[4].Error, "duplikat") || strings.Contains(res.Results[5].Error, "tenant 8") {
		t.Errorf("pesan error: dup=%q other=%q", res.Results[4].Error, res.Results[5].Error)
	}
	if devs.creates != 0 || devs.updates != 0 || len(act.got) != 0 {
		t.Fatalf("dry-run TIDAK boleh menulis: creates=%d updates=%d audit=%d", devs.creates, devs.updates, len(act.got))
	}
	if *devs.byKey["00D0D0|EXIST"].Notes != "lama" {
		t.Fatal("dry-run tidak boleh mengubah data")
	}
}

func TestImport_EksekusiSamaDenganDryRun(t *testing.T) {
	svc, devs, act := pvRig(t)
	seedExisting(devs)
	ctx := context.Background()

	dry, _ := svc.Import(ctx, adminT7, ImportInput{Mode: ImportModeUpdate, DryRun: true, Rows: importRows()})
	real, err := svc.Import(ctx, adminT7, ImportInput{Mode: ImportModeUpdate, Rows: importRows()})
	if err != nil {
		t.Fatal(err)
	}
	if dry.Created != real.Created || dry.Updated != real.Updated || dry.Unchanged != real.Unchanged || dry.Failed != real.Failed {
		t.Fatalf("pratinjau harus = eksekusi: dry=%+v real=%+v", dry, real)
	}
	if _, ok := devs.byKey["00D0D0|A1"]; !ok {
		t.Fatal("baris create harus tersimpan")
	}
	if *devs.byKey["00D0D0|EXIST"].Notes != "catatan baru" {
		t.Fatal("baris update harus tersimpan")
	}
	if len(act.got) != 1 || act.got[0].Action != "IMPORT_DEVICES" {
		t.Fatalf("harus ada SATU audit IMPORT_DEVICES: %+v", act.got)
	}
	// Impor ulang: idempoten — tidak ada lagi create/update.
	again, _ := svc.Import(ctx, adminT7, ImportInput{Mode: ImportModeUpdate, Rows: importRows()[:3]})
	if again.Created != 0 || again.Updated != 0 || again.Unchanged != 3 {
		t.Fatalf("impor ulang harus idempoten: %+v", again)
	}
}

func TestImport_ModeSkipMelewatiYangAda(t *testing.T) {
	svc, devs, _ := pvRig(t)
	seedExisting(devs)
	res, err := svc.Import(context.Background(), adminT7, ImportInput{Mode: ImportModeSkip, Rows: importRows()[:3]})
	if err != nil {
		t.Fatal(err)
	}
	if res.Created != 1 || res.Skipped != 2 || res.Updated != 0 {
		t.Fatalf("mode skip: %+v", res)
	}
	if *devs.byKey["00D0D0|EXIST"].Notes != "lama" {
		t.Fatal("mode skip tidak boleh mengubah baris yang ada")
	}
}

func TestImport_GuardPermintaan(t *testing.T) {
	svc, _, _ := pvRig(t)
	ctx := context.Background()
	if _, err := svc.Import(ctx, viewer, ImportInput{Mode: ImportModeSkip, Rows: importRows()}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("VIEWER: %v", err)
	}
	if _, err := svc.Import(ctx, adminT7, ImportInput{Mode: "ngawur", Rows: importRows()}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("mode salah: %v", err)
	}
	if _, err := svc.Import(ctx, adminT7, ImportInput{Mode: ImportModeSkip}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("rows kosong: %v", err)
	}
	many := make([]CreateDeviceInput, MaxImportRows+1)
	if _, err := svc.Import(ctx, adminT7, ImportInput{Mode: ImportModeSkip, Rows: many}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("terlalu banyak baris: %v", err)
	}
}

func TestImport_SuperadminPerBarisTenantDanOrphan(t *testing.T) {
	svc, devs, _ := pvRig(t)
	devs.byKey["00D0D0|ORPHAN"] = &domain.Device{ID: 200, OUI: ptr("00D0D0"), SerialNumber: "ORPHAN"} // tanpa tenant
	rows := []CreateDeviceInput{
		{OUI: "00D0D0", SerialNumber: "S1", TenantID: ptr(uint64(7))},
		{OUI: "00D0D0", SerialNumber: "S2"}, // superadmin tanpa tenant → error
		{OUI: "00D0D0", SerialNumber: "ORPHAN", TenantID: ptr(uint64(8)), Notes: ptr("klaim")},
	}
	res, err := svc.Import(context.Background(), super, ImportInput{Mode: ImportModeUpdate, Rows: rows})
	if err != nil {
		t.Fatal(err)
	}
	if res.Created != 1 || res.Failed != 1 || res.Updated != 1 {
		t.Fatalf("ringkasan: %+v", res)
	}
	if got := devs.byKey["00D0D0|ORPHAN"]; got.TenantID == nil || *got.TenantID != 8 {
		t.Fatalf("superadmin boleh mengklaim device tanpa tenant: %+v", got.TenantID)
	}
	// ADMIN tidak boleh menyentuh device tanpa tenant.
	devs.byKey["00D0D0|ORPHAN2"] = &domain.Device{ID: 201, OUI: ptr("00D0D0"), SerialNumber: "ORPHAN2"}
	r2, _ := svc.Import(context.Background(), adminT7, ImportInput{Mode: ImportModeUpdate, Rows: []CreateDeviceInput{{OUI: "00D0D0", SerialNumber: "ORPHAN2", Notes: ptr("x")}}})
	if r2.Failed != 1 || r2.Updated != 0 {
		t.Fatalf("ADMIN vs orphan: %+v", r2)
	}
}

// ---- Update (PATCH) yang diperluas ----

func TestUpdate_FieldBaruDanValidasi(t *testing.T) {
	svc, devs, _ := pvRig(t)
	devs.byKey["00D0D0|U1"] = &domain.Device{ID: 300, TenantID: ptr(uint64(7)), OUI: ptr("00D0D0"), SerialNumber: "U1"}
	devsByID := &pvByID{pvDevices: devs, id: 300, key: "00D0D0|U1"}
	svc.devices = devsByID
	ctx := context.Background()

	if _, err := svc.Update(ctx, adminT7, 300, UpdateDeviceInput{MACAddress: ptr("bad")}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("mac salah: %v", err)
	}
	if _, err := svc.Update(ctx, adminT7, 300, UpdateDeviceInput{Latitude: ptr(1.0)}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("lat tanpa lon: %v", err)
	}
	if _, err := svc.Update(ctx, adminT7, 300, UpdateDeviceInput{VendorID: ptr(uint64(99))}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("vendor tak ada: %v", err)
	}
	d, err := svc.Update(ctx, adminT7, 300, UpdateDeviceInput{
		MACAddress: ptr("aabbccddeeff"), ProductClass: ptr("F670L"), VendorID: ptr(uint64(1)), DeviceModelID: ptr(uint64(5)),
		Latitude: ptr(-6.2), Longitude: ptr(106.8),
	})
	if err != nil {
		t.Fatal(err)
	}
	if *d.MACAddress != "AA:BB:CC:DD:EE:FF" || *d.ProductClass != "F670L" || *d.VendorID != 1 || *d.DeviceModelID != 5 || *d.Latitude != -6.2 {
		t.Fatalf("hasil update: %+v", d)
	}
}

// pvByID menambah GetByID di atas pvDevices (Update memakai Service.Get → GetByID).
type pvByID struct {
	*pvDevices
	id  uint64
	key string
}

func (f *pvByID) GetByID(_ context.Context, id uint64) (*domain.Device, error) {
	if id == f.id {
		return f.byKey[f.key], nil
	}
	return nil, domain.ErrNotFound
}
