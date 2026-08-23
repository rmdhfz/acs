package metrics

import (
	"context"
	"errors"
	"testing"

	"acs/internal/domain"
)

// fakeRefRepo implementasi minimal domain.RefRepository utk test
// refCodeMap/vendorLabel/labelFromMap — satu-satunya logic non-trivial yang
// ditambahkan Collector di atas usecase yang sudah ada & sudah punya test-nya
// sendiri (device.Service.Stats/task.Service.Stats/dst).
type fakeRefRepo struct {
	rows map[string][]domain.RefLookup
	err  error
}

func (f *fakeRefRepo) GetByCode(context.Context, string, string) (domain.RefLookup, error) {
	return domain.RefLookup{}, errors.New("not implemented")
}
func (f *fakeRefRepo) GetByID(context.Context, string, uint64) (domain.RefLookup, error) {
	return domain.RefLookup{}, errors.New("not implemented")
}
func (f *fakeRefRepo) List(_ context.Context, table string) ([]domain.RefLookup, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.rows[table], nil
}

func TestLabelFromMap(t *testing.T) {
	m := map[uint64]string{1: "ONLINE", 2: "OFFLINE"}

	if got := labelFromMap(m, 1); got != "ONLINE" {
		t.Fatalf("expected ONLINE, got %q", got)
	}
	// id yang tidak ada di map (mis. status baru yang belum di-seed ulang di
	// cache/ref table saat scrape) tidak boleh membuat Collect() panic/gagal
	// — jatuh ke label fallback yang tetap informatif.
	if got := labelFromMap(m, 999); got != "unknown_999" {
		t.Fatalf("expected unknown_999, got %q", got)
	}
	// map nil (mis. refCodeMap gagal load) juga harus tetap fallback, bukan
	// panic nil map read.
	if got := labelFromMap(nil, 1); got != "unknown_1" {
		t.Fatalf("expected unknown_1 utk nil map, got %q", got)
	}
}

func TestVendorLabel(t *testing.T) {
	m := map[uint64]string{5: "ZTE"}
	id5 := uint64(5)
	idUnknown := uint64(42)

	if got := vendorLabel(m, nil); got != "unknown" {
		t.Fatalf("vendor id nil (belum ter-resolve) expected \"unknown\", got %q", got)
	}
	if got := vendorLabel(m, &id5); got != "ZTE" {
		t.Fatalf("expected ZTE, got %q", got)
	}
	if got := vendorLabel(m, &idUnknown); got != "unknown_42" {
		t.Fatalf("expected unknown_42, got %q", got)
	}
}

func TestRefCodeMap(t *testing.T) {
	c := &Collector{
		refs: &fakeRefRepo{rows: map[string][]domain.RefLookup{
			domain.RefTableDeviceStatus: {
				{ID: 1, Code: "ONLINE", Name: "Online"},
				{ID: 2, Code: "OFFLINE", Name: "Offline"},
			},
		}},
	}

	m := c.refCodeMap(context.Background(), domain.RefTableDeviceStatus)
	if len(m) != 2 || m[1] != "ONLINE" || m[2] != "OFFLINE" {
		t.Fatalf("unexpected map: %#v", m)
	}

	// Tabel lain yang tidak ada datanya (bukan error) -> map kosong, bukan nil
	// panic saat dipakai labelFromMap.
	empty := c.refCodeMap(context.Background(), domain.RefTableVendors)
	if len(empty) != 0 {
		t.Fatalf("expected map kosong, got %#v", empty)
	}
}

func TestRefCodeMap_ErrorReturnsNil(t *testing.T) {
	c := &Collector{refs: &fakeRefRepo{err: errors.New("db down")}}

	// Kegagalan query ref_* TIDAK boleh membuat Collect() panic — refCodeMap
	// jatuh ke nil map, dan labelFromMap di atas sudah memverifikasi nil map
	// aman dipakai (fallback "unknown_<id>").
	m := c.refCodeMap(context.Background(), domain.RefTableTaskStatus)
	if m != nil {
		t.Fatalf("expected nil map saat List() error, got %#v", m)
	}
}
