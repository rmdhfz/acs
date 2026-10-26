package file

import (
	"context"
	"errors"
	"strings"
	"testing"

	"acs/internal/domain"
)

type fakeFiles struct {
	domain.FileRepository
	byID    map[uint64]*domain.File
	deleted []uint64
}

func (f *fakeFiles) GetByID(_ context.Context, id uint64) (*domain.File, error) {
	if x, ok := f.byID[id]; ok {
		return x, nil
	}
	return nil, domain.ErrNotFound
}
func (f *fakeFiles) Delete(_ context.Context, id uint64) error {
	f.deleted = append(f.deleted, id)
	return nil
}
func (f *fakeFiles) List(context.Context, *uint64, domain.Pagination) ([]domain.File, int, error) {
	return nil, 0, nil
}

type fakeAct struct{ domain.ActivityLogRepository }

func (fakeAct) Record(context.Context, *domain.ActivityLog) error { return nil }

func tid(v uint64) *uint64 { return &v }

func TestDelete_Scope(t *testing.T) {
	repo := &fakeFiles{byID: map[uint64]*domain.File{
		1: {ID: 1, TenantID: nil},    // global
		2: {ID: 2, TenantID: tid(7)}, // tenant 7
	}}
	svc := NewService(repo, &fakeStorage{}, fakeAct{})
	ctx := context.Background()
	adminA := domain.Actor{UserID: 2, TenantID: tid(7), Roles: []string{domain.RoleAdmin}}
	adminB := domain.Actor{UserID: 3, TenantID: tid(8), Roles: []string{domain.RoleAdmin}}
	super := domain.Actor{UserID: 1, Roles: []string{domain.RoleSuperadmin}}
	noTenant := domain.Actor{UserID: 4, Roles: []string{domain.RoleAdmin}} // non-superadmin tanpa tenant

	if err := svc.Delete(ctx, adminB, 1); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("ADMIN tidak boleh menghapus file GLOBAL: %v", err)
	}
	if err := svc.Delete(ctx, adminB, 2); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("ADMIN tenant lain: %v", err)
	}
	if err := svc.Delete(ctx, noTenant, 2); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("aktor tanpa tenant tidak boleh (dan tidak boleh panic): %v", err)
	}
	if err := svc.Delete(ctx, adminA, 2); err != nil {
		t.Fatalf("pemilik tenant: %v", err)
	}
	if err := svc.Delete(ctx, super, 1); err != nil {
		t.Fatalf("superadmin hapus global: %v", err)
	}
	if len(repo.deleted) != 2 {
		t.Fatalf("deleted = %v", repo.deleted)
	}
}

func TestList_TanpaTenantDitolak(t *testing.T) {
	svc := NewService(&fakeFiles{}, nil, fakeAct{})
	if _, _, err := svc.List(context.Background(), domain.Actor{UserID: 4, Roles: []string{domain.RoleAdmin}}, domain.Pagination{}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("non-superadmin tanpa tenant: want ErrForbidden, got %v", err)
	}
}

func TestUploadFile_FileTypeDivalidasi(t *testing.T) {
	svc := NewService(&fakeFiles{}, nil, fakeAct{})
	_, err := svc.UploadFile(context.Background(), domain.Actor{UserID: 1, Roles: []string{domain.RoleSuperadmin}}, UploadFileInput{
		FileType: "1_FIRMWARE_UPGRADE_IMAGE", FileName: "x.bin", File: strings.NewReader("x"), FileSize: 1,
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("file_type asal-asalan harus ditolak 400, got %v", err)
	}
}
