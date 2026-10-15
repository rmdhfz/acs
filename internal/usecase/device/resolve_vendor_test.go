package device

import (
	"context"
	"errors"
	"testing"

	"acs/internal/domain"
)

// --- fakes minimal: hanya method yang dipakai FindOrCreateFromInform ---

type fakeDevices struct {
	domain.DeviceRepository
	existing *domain.Device
	created  []*domain.Device
	updated  []*domain.Device
}

func (f *fakeDevices) GetByOUISerial(_ context.Context, _, _ string) (*domain.Device, error) {
	if f.existing == nil {
		return nil, domain.ErrNotFound
	}
	return f.existing, nil
}
func (f *fakeDevices) Create(_ context.Context, d *domain.Device) error {
	f.created = append(f.created, d)
	return nil
}
func (f *fakeDevices) Update(_ context.Context, d *domain.Device) error {
	f.updated = append(f.updated, d)
	return nil
}

type fakeOUIs struct {
	domain.VendorOUIRepository
	byOUI map[string]uint64
	err   error // bila diisi: dikembalikan untuk semua lookup
	calls int
}

func (f *fakeOUIs) GetByOUI(_ context.Context, oui string) (*domain.VendorOUI, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	id, ok := f.byOUI[oui]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return &domain.VendorOUI{VendorID: id, OUI: oui}, nil
}

type fakeModels struct {
	domain.DeviceModelRepository
	byKey map[[2]string]uint64 // {vendorID, productClass} -> model id
	calls int
	// lastVendor merekam vendor_id yang dipakai lookup terakhir.
	lastVendor uint64
}

func (f *fakeModels) FindByVendorAndProductClass(_ context.Context, vendorID uint64, pc string) (*domain.DeviceModel, error) {
	f.calls++
	f.lastVendor = vendorID
	id, ok := f.byKey[[2]string{u64s(vendorID), pc}]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return &domain.DeviceModel{ID: id}, nil
}

type fakeRefs struct{ domain.RefRepository }

func (fakeRefs) GetByCode(_ context.Context, _, code string) (domain.RefLookup, error) {
	return domain.RefLookup{ID: 7, Code: code}, nil
}

func u64s(v uint64) string { return string(rune('0' + v)) } // vendor id kecil di test
func ptr[T any](v T) *T    { return &v }

func newTestService(devs *fakeDevices, ouis *fakeOUIs, models *fakeModels) *Service {
	return &Service{devices: devs, vendorOUIs: ouis, deviceModels: models, refs: fakeRefs{}}
}

func informInfo() InformDeviceInfo {
	return InformDeviceInfo{OUI: "00D0D0", SerialNumber: "SN1", ProductClass: "F670L", RemoteIP: "203.0.113.7"}
}

func TestFindOrCreateFromInform_DeviceBaruResolveVendorDanModel(t *testing.T) {
	devs := &fakeDevices{}
	ouis := &fakeOUIs{byOUI: map[string]uint64{"00D0D0": 1}}
	models := &fakeModels{byKey: map[[2]string]uint64{{"1", "F670L"}: 42}}
	svc := newTestService(devs, ouis, models)

	d, isNew, err := svc.FindOrCreateFromInform(context.Background(), informInfo())
	if err != nil || !isNew {
		t.Fatalf("err=%v isNew=%v", err, isNew)
	}
	if d.VendorID == nil || *d.VendorID != 1 || d.DeviceModelID == nil || *d.DeviceModelID != 42 {
		t.Fatalf("vendor/model tidak ter-resolve: vendor=%v model=%v", d.VendorID, d.DeviceModelID)
	}
	if len(devs.created) != 1 {
		t.Fatalf("Create dipanggil %d kali, ingin 1", len(devs.created))
	}
}

func TestFindOrCreateFromInform_OUITakDikenalTetapTercatat(t *testing.T) {
	devs := &fakeDevices{}
	svc := newTestService(devs, &fakeOUIs{}, &fakeModels{})

	d, isNew, err := svc.FindOrCreateFromInform(context.Background(), informInfo())
	if err != nil || !isNew {
		t.Fatalf("err=%v isNew=%v", err, isNew)
	}
	if d.VendorID != nil || d.DeviceModelID != nil {
		t.Fatalf("OUI tak dikenal harus tetap nil, dapat vendor=%v model=%v", d.VendorID, d.DeviceModelID)
	}
}

// Skenario inti uji lapangan: device sudah terlanjur dibuat (OUI belum
// terdaftar), OUI baru ditambahkan, Inform berikutnya harus menyembuhkannya.
func TestFindOrCreateFromInform_DeviceAdaSembuhSetelahOUITerdaftar(t *testing.T) {
	devs := &fakeDevices{existing: &domain.Device{ID: 5, SerialNumber: "SN1"}}
	ouis := &fakeOUIs{byOUI: map[string]uint64{"00D0D0": 1}}
	models := &fakeModels{byKey: map[[2]string]uint64{{"1", "F670L"}: 42}}
	svc := newTestService(devs, ouis, models)

	d, isNew, err := svc.FindOrCreateFromInform(context.Background(), informInfo())
	if err != nil || isNew {
		t.Fatalf("err=%v isNew=%v", err, isNew)
	}
	if d.VendorID == nil || *d.VendorID != 1 || d.DeviceModelID == nil || *d.DeviceModelID != 42 {
		t.Fatalf("tidak sembuh: vendor=%v model=%v", d.VendorID, d.DeviceModelID)
	}
	if len(devs.updated) != 1 || devs.updated[0].VendorID == nil {
		t.Fatalf("hasil resolve harus ikut dipersist lewat Update")
	}
}

func TestFindOrCreateFromInform_VendorManualTidakDitimpa(t *testing.T) {
	// Operator meng-assign vendor 2 secara manual; OUI terdaftar ke vendor 1.
	devs := &fakeDevices{existing: &domain.Device{ID: 5, SerialNumber: "SN1", VendorID: ptr(uint64(2))}}
	ouis := &fakeOUIs{byOUI: map[string]uint64{"00D0D0": 1}}
	models := &fakeModels{byKey: map[[2]string]uint64{{"2", "F670L"}: 99}}
	svc := newTestService(devs, ouis, models)

	d, _, err := svc.FindOrCreateFromInform(context.Background(), informInfo())
	if err != nil {
		t.Fatal(err)
	}
	if *d.VendorID != 2 {
		t.Fatalf("vendor manual ditimpa: %d", *d.VendorID)
	}
	if ouis.calls != 0 {
		t.Fatalf("vendor sudah terisi: GetByOUI tidak boleh dipanggil, dipanggil %d kali", ouis.calls)
	}
	// Model tetap di-resolve, memakai vendor MANUAL (2), bukan vendor dari OUI.
	if models.lastVendor != 2 || d.DeviceModelID == nil || *d.DeviceModelID != 99 {
		t.Fatalf("model harus di-resolve lewat vendor manual: lastVendor=%d model=%v", models.lastVendor, d.DeviceModelID)
	}
}

func TestFindOrCreateFromInform_SudahLengkapTanpaQueryTambahan(t *testing.T) {
	devs := &fakeDevices{existing: &domain.Device{
		ID: 5, SerialNumber: "SN1", VendorID: ptr(uint64(1)), DeviceModelID: ptr(uint64(42)),
	}}
	ouis, models := &fakeOUIs{}, &fakeModels{}
	svc := newTestService(devs, ouis, models)

	if _, _, err := svc.FindOrCreateFromInform(context.Background(), informInfo()); err != nil {
		t.Fatal(err)
	}
	if ouis.calls != 0 || models.calls != 0 {
		t.Fatalf("jalur panas Inform tidak boleh query katalog bila sudah lengkap: ouis=%d models=%d", ouis.calls, models.calls)
	}
}

func TestFindOrCreateFromInform_ErrorLookupTidakMenggagalkanInform(t *testing.T) {
	devs := &fakeDevices{existing: &domain.Device{ID: 5, SerialNumber: "SN1"}}
	ouis := &fakeOUIs{err: errors.New("db: connection reset")}
	svc := newTestService(devs, ouis, &fakeModels{})

	d, _, err := svc.FindOrCreateFromInform(context.Background(), informInfo())
	if err != nil {
		t.Fatalf("error lookup katalog tidak boleh menggagalkan Inform: %v", err)
	}
	if d.VendorID != nil {
		t.Fatalf("vendor harus tetap nil saat lookup error")
	}
	if len(devs.updated) != 1 {
		t.Fatalf("device tetap harus di-update (last_inform_at dst.)")
	}
}
