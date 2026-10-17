package catalog

import (
	"context"
	"errors"
	"testing"

	"acs/internal/domain"
)

// Vendor GENERIC (fallback mapping standar, migrations/0029) tidak boleh dihapus atau dinonaktifkan: tanpanya
// perangkat ber-OUI tak dikenal kehilangan resolusi logical key.
func TestGenericVendor_TidakBolehDihapusAtauDinonaktifkan(t *testing.T) {
	r := newRig()
	r.vendors.byID[99] = &domain.Vendor{ID: 99, Code: domain.VendorCodeGeneric, Name: "Standar", IsActive: true}

	if err := r.svc.DeleteVendor(context.Background(), superadmin, 99); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("hapus GENERIC harus ErrConflict, dapat %v", err)
	}
	if len(r.vendors.deleted) != 0 {
		t.Fatal("tidak boleh ada penghapusan")
	}

	off := false
	if _, err := r.svc.UpdateVendor(context.Background(), superadmin, 99, UpdateVendorInput{IsActive: &off}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("nonaktifkan GENERIC harus ErrConflict, dapat %v", err)
	}
	if r.vendors.updated != nil {
		t.Fatal("tidak boleh ada pembaruan")
	}

	// mengganti nama/keterangan tetap boleh
	name := "Standar TR-069 (Generik)"
	if _, err := r.svc.UpdateVendor(context.Background(), superadmin, 99, UpdateVendorInput{Name: &name}); err != nil {
		t.Fatalf("ubah nama GENERIC harus boleh: %v", err)
	}
}
