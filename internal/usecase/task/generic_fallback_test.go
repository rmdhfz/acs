package task

import (
	"context"
	"errors"
	"testing"

	"acs/internal/domain"
)

const genericVendorTestID = uint64(99)

// gfMappings: vendor 1 hanya punya wifi.ssid kustom; vendor GENERIC (99) punya mapping standar.
type gfMappings struct {
	domain.VendorParameterMappingRepository
	calls []uint64 // vendor_id yang diminta, berurutan
}

func (g *gfMappings) Resolve(_ context.Context, vendorID, _ uint64, _ *uint64, key string, _ *string) (*domain.VendorParameterMapping, error) {
	g.calls = append(g.calls, vendorID)
	switch {
	case vendorID == 1 && key == "wifi.ssid":
		return &domain.VendorParameterMapping{TR069Path: "Device.X_VENDOR.SSID"}, nil
	case vendorID == genericVendorTestID && (key == "wifi.ssid" || key == "wan.pppoe.username"):
		return &domain.VendorParameterMapping{TR069Path: "Device.Standard." + key}, nil
	}
	return nil, domain.ErrNotFound
}

type gfRefs struct {
	domain.RefRepository
	hasGeneric bool
}

func (r gfRefs) GetByCode(_ context.Context, table, code string) (domain.RefLookup, error) {
	switch {
	case table == domain.RefTableDataModelVersions:
		return map[string]domain.RefLookup{"TR098": {ID: 1, Code: "TR098"}, "TR181": {ID: 2, Code: "TR181"}}[code], nil
	case table == domain.RefTableVendors && code == domain.VendorCodeGeneric && r.hasGeneric:
		return domain.RefLookup{ID: genericVendorTestID, Code: code}, nil
	}
	return domain.RefLookup{}, domain.ErrNotFound
}

func newGenericSvc(vendorID *uint64, hasGeneric bool, gm *gfMappings) *Service {
	return &Service{
		devices:       rvDevices{dev: &domain.Device{ID: 7, VendorID: vendorID}},
		deviceParams:  rvParams{prefixes: map[string]bool{"Device.": true}},
		paramMappings: gm,
		refs:          gfRefs{hasGeneric: hasGeneric},
	}
}

// ONT dengan OUI yang belum dikenal (vendor NULL) tetap bisa dikelola lewat mapping standar.
func TestResolveMapping_VendorTakDikenalPakaiMappingStandar(t *testing.T) {
	gm := &gfMappings{}
	s := newGenericSvc(nil, true, gm)
	got, err := s.ResolveParameterPath(context.Background(), 7, "wan.pppoe.username")
	if err != nil || got != "Device.Standard.wan.pppoe.username" {
		t.Fatalf("got %q err=%v", got, err)
	}
	if len(gm.calls) != 1 || gm.calls[0] != genericVendorTestID {
		t.Fatalf("hanya vendor GENERIC yang boleh diminta: %v", gm.calls)
	}
}

// Mapping vendor sendiri SELALU menang; fallback hanya bila vendor tidak punya key tsb.
func TestResolveMapping_MappingVendorMenangLaluFallback(t *testing.T) {
	vendor := uint64(1)
	gm := &gfMappings{}
	s := newGenericSvc(&vendor, true, gm)

	got, err := s.ResolveParameterPath(context.Background(), 7, "wifi.ssid")
	if err != nil || got != "Device.X_VENDOR.SSID" {
		t.Fatalf("mapping vendor harus menang: %q %v", got, err)
	}
	if len(gm.calls) != 1 {
		t.Fatalf("tidak boleh menyentuh GENERIC bila vendor sudah punya mapping: %v", gm.calls)
	}

	gm.calls = nil
	got, err = s.ResolveParameterPath(context.Background(), 7, "wan.pppoe.username")
	if err != nil || got != "Device.Standard.wan.pppoe.username" {
		t.Fatalf("fallback ke standar: %q %v", got, err)
	}
	if len(gm.calls) != 2 || gm.calls[0] != 1 || gm.calls[1] != genericVendorTestID {
		t.Fatalf("urutan lookup harus vendor lalu GENERIC: %v", gm.calls)
	}

	// key yang tidak ada di keduanya -> galat jelas (bukan menebak path)
	if _, err := s.ResolveParameterPath(context.Background(), 7, "tidak.ada"); err == nil {
		t.Fatal("key tak terpetakan harus galat")
	}
}

// Tanpa vendor GENERIC di katalog (mis. dihapus operator) perilaku lama tetap: vendor NULL -> galat jelas.
func TestResolveMapping_TanpaVendorGenericPerilakuLama(t *testing.T) {
	s := newGenericSvc(nil, false, &gfMappings{})
	_, err := s.ResolveParameterPath(context.Background(), 7, "wifi.ssid")
	if err == nil || errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("harus galat 'vendor belum diketahui', got %v", err)
	}
}

// Perangkat yang vendor-nya memang GENERIC tidak menanyakan GENERIC dua kali.
func TestResolveMapping_VendorGenericTidakDuaKali(t *testing.T) {
	vendor := genericVendorTestID
	gm := &gfMappings{}
	s := newGenericSvc(&vendor, true, gm)
	if _, err := s.ResolveParameterPath(context.Background(), 7, "tidak.ada"); err == nil {
		t.Fatal("harus galat")
	}
	if len(gm.calls) != 1 {
		t.Fatalf("GENERIC hanya boleh sekali: %v", gm.calls)
	}
}
