package firmware

import (
	"context"
	"errors"
	"strings"
	"testing"

	"acs/internal/domain"
)

func fwCRUDRig() (*Service, *fakeFirmwareFileRepo, *fakeUsageFW) {
	notes := "catatan lama"
	files := &fakeFirmwareFileRepo{byID: map[uint64]*domain.FirmwareFile{
		1: {ID: 1, VendorID: 3, Version: "V1.0", FileName: "fw.bin", StorageKey: "firmware/3/fw.bin",
			ChecksumSHA256: strPtr("abc"), ReleaseNotes: &notes, IsActive: true},
	}}
	usage := &fakeUsageFW{}
	return NewService(files, nil, nil, nil, nil, nil, &fakeActivityLogFW{}, nil, usage), files, usage
}

func strPtr(s string) *string { return &s }
func boolPtr(b bool) *bool    { return &b }

var (
	fwSuper = domain.Actor{UserID: 1, Roles: []string{domain.RoleSuperadmin}}
	fwAdmin = domain.Actor{UserID: 2, TenantID: func() *uint64 { v := uint64(7); return &v }(), Roles: []string{domain.RoleAdmin}}
)

func TestUpdateFirmware_HanyaMetadataAman(t *testing.T) {
	svc, files, _ := fwCRUDRig()
	ctx := context.Background()

	got, err := svc.UpdateFirmware(ctx, fwSuper, 1, UpdateFirmwareInput{Version: strPtr("V1.1"), IsActive: boolPtr(false), ReleaseNotes: strPtr("")})
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != "V1.1" || got.IsActive || got.ReleaseNotes != nil {
		t.Fatalf("update metadata salah: %+v", got)
	}
	// identitas isi file tidak boleh berubah lewat update
	if got.StorageKey != "firmware/3/fw.bin" || got.FileName != "fw.bin" || got.VendorID != 3 ||
		got.ChecksumSHA256 == nil || *got.ChecksumSHA256 != "abc" {
		t.Fatalf("storage_key/file_name/vendor/checksum tidak boleh berubah: %+v", got)
	}
	if files.updated == nil || files.updated.UpdatedBy == nil || *files.updated.UpdatedBy != 1 {
		t.Fatal("updated_by harus terisi")
	}
	if _, err := svc.UpdateFirmware(ctx, fwSuper, 1, UpdateFirmwareInput{Version: strPtr("  ")}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("version kosong: want ErrInvalidInput, got %v", err)
	}
	if _, err := svc.UpdateFirmware(ctx, fwAdmin, 1, UpdateFirmwareInput{IsActive: boolPtr(false)}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("ADMIN tenant: want ErrForbidden, got %v", err)
	}
	if _, err := svc.UpdateFirmware(ctx, fwSuper, 404, UpdateFirmwareInput{IsActive: boolPtr(false)}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestDeleteFirmware_Guard(t *testing.T) {
	svc, files, usage := fwCRUDRig()
	ctx := context.Background()

	if err := svc.DeleteFirmware(ctx, fwAdmin, 1); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("ADMIN tenant: want ErrForbidden, got %v", err)
	}
	if err := svc.DeleteFirmware(ctx, fwSuper, 404); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}

	usage.firmware = []domain.UsageCount{{Kind: "rollout_batch_aktif", Count: 1}, {Kind: "zero_touch_rule", Count: 2}}
	err := svc.DeleteFirmware(ctx, fwSuper, 1)
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("masih dipakai: want ErrConflict, got %v", err)
	}
	for _, want := range []string{"1 rollout_batch_aktif", "2 zero_touch_rule", "is_active=false"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("pesan harus memuat %q: %v", want, err)
		}
	}
	if len(files.deletedIDs) != 0 {
		t.Fatal("firmware yang dipakai tidak boleh terhapus")
	}

	usage.firmware = nil
	if err := svc.DeleteFirmware(ctx, fwSuper, 1); err != nil {
		t.Fatal(err)
	}
	if len(files.deletedIDs) != 1 || files.deletedIDs[0] != 1 {
		t.Fatalf("firmware bebas harus ter-soft-delete, got %v", files.deletedIDs)
	}
}
