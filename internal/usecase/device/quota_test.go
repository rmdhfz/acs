package device

import (
	"context"
	"errors"
	"testing"

	"acs/internal/domain"
)

type fakeQuota struct {
	max *uint32
	cur int
}

func (f *fakeQuota) SetMaxDevices(context.Context, uint64, *uint32, *uint64) error { return nil }
func (f *fakeQuota) DeviceCapacity(context.Context, uint64) (*uint32, int, error) {
	return f.max, f.cur, nil
}

func u32(v uint32) *uint32 { return &v }

// Batas perangkat tenant (plan SaaS): perangkat BARU ditolak saat batas tercapai, lewat Inform, pra-registrasi, dan impor;
// perangkat yang sudah ada tetap boleh Inform; tanpa batas / tanpa tenant / tanpa quota repo = tak dibatasi.
func TestKuotaPerangkat_Inform(t *testing.T) {
	ctx := context.Background()
	tenant := ptr(uint64(7))

	// penuh: perangkat baru ditolak, tidak ada baris dibuat
	devs := &fakeDevices{}
	svc := newTestService(devs, &fakeOUIs{}, &fakeModels{})
	svc.WithQuota(&fakeQuota{max: u32(3), cur: 3})
	info := informInfo()
	info.TenantID = tenant
	if _, _, err := svc.FindOrCreateFromInform(ctx, info); !errors.Is(err, domain.ErrQuotaExceeded) {
		t.Fatalf("batas tercapai harus ErrQuotaExceeded: %v", err)
	}
	if len(devs.created) != 0 {
		t.Fatalf("tidak boleh membuat perangkat saat batas tercapai: %d", len(devs.created))
	}

	// masih ada ruang
	svc.WithQuota(&fakeQuota{max: u32(3), cur: 2})
	if _, isNew, err := svc.FindOrCreateFromInform(ctx, info); err != nil || !isNew {
		t.Fatalf("masih ada ruang: err=%v isNew=%v", err, isNew)
	}

	// tak dibatasi (nil), tanpa tenant, atau tanpa repo quota
	for name, s := range map[string]*Service{
		"nil batas": func() *Service {
			x := newTestService(&fakeDevices{}, &fakeOUIs{}, &fakeModels{})
			x.WithQuota(&fakeQuota{cur: 9999})
			return x
		}(),
		"tanpa repo": newTestService(&fakeDevices{}, &fakeOUIs{}, &fakeModels{}),
	} {
		if _, _, err := s.FindOrCreateFromInform(ctx, info); err != nil {
			t.Fatalf("%s harus lolos: %v", name, err)
		}
	}
	noTenant := informInfo()
	full := newTestService(&fakeDevices{}, &fakeOUIs{}, &fakeModels{})
	full.WithQuota(&fakeQuota{max: u32(0), cur: 5})
	if _, _, err := full.FindOrCreateFromInform(ctx, noTenant); err != nil {
		t.Fatalf("perangkat tanpa tenant tak terkena batas tenant: %v", err)
	}
}

func TestKuotaPerangkat_PraRegistrasiDanImpor(t *testing.T) {
	svc, _, _ := pvRig(t)
	svc.WithQuota(&fakeQuota{max: u32(1), cur: 1})
	ctx := context.Background()
	in := CreateDeviceInput{OUI: "00D0D0", SerialNumber: "KUOTA-1"}
	if _, err := svc.Create(ctx, adminT7, in); !errors.Is(err, domain.ErrQuotaExceeded) {
		t.Fatalf("pra-registrasi saat penuh harus ErrQuotaExceeded: %v", err)
	}
	// impor: baris baru gagal per-baris dengan alasan kuota (bukan seluruh impor)
	res, err := svc.Import(ctx, adminT7, ImportInput{Mode: ImportModeUpdate, Rows: []CreateDeviceInput{{OUI: "00D0D0", SerialNumber: "KUOTA-2"}}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Failed != 1 || res.Created != 0 {
		t.Fatalf("impor saat penuh: %+v", res)
	}
	// ada ruang -> berhasil
	svc.WithQuota(&fakeQuota{max: u32(5), cur: 1})
	if _, err := svc.Create(ctx, adminT7, CreateDeviceInput{OUI: "00D0D0", SerialNumber: "KUOTA-3"}); err != nil {
		t.Fatalf("masih ada ruang: %v", err)
	}
}
