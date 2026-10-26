package iam

import (
	"context"
	"errors"
	"testing"

	"acs/internal/domain"
)

type fakeStorageQuotaRepo struct {
	calls []*uint64
	err   error
}

func (f *fakeStorageQuotaRepo) SetMaxStorageBytes(_ context.Context, _ uint64, max *uint64, _ *uint64) error {
	f.calls = append(f.calls, max)
	return f.err
}
func (f *fakeStorageQuotaRepo) StorageUsage(context.Context, uint64) (*uint64, uint64, error) {
	return nil, 0, nil
}

// Kuota penyimpanan adalah kebijakan platform: hanya SUPERADMIN; ADMIN tenant (termasuk untuk tenantnya sendiri) ditolak;
// nilai ngawur ditolak; null = tak dibatasi; tanpa repo = galat konfigurasi; aksi tercatat di audit trail.
func TestSetStorageQuota(t *testing.T) {
	ctx := context.Background()
	v := func(n uint64) *uint64 { return &n }

	t.Run("ADMIN tenant ditolak, tidak ada yang tersimpan", func(t *testing.T) {
		svc, _, _, act := newCRUDRig()
		repo := &fakeStorageQuotaRepo{}
		svc.WithStorageQuota(repo)
		if err := svc.SetStorageQuota(ctx, adminT7, 7, v(1<<30)); !errors.Is(err, domain.ErrForbidden) {
			t.Fatalf("want ErrForbidden, got %v", err)
		}
		if len(repo.calls) != 0 || len(act.actions) != 0 {
			t.Fatalf("tidak boleh menyimpan/mencatat: calls=%d actions=%v", len(repo.calls), act.actions)
		}
	})

	t.Run("superadmin menyimpan nilai, mencatat audit", func(t *testing.T) {
		svc, _, _, act := newCRUDRig()
		repo := &fakeStorageQuotaRepo{}
		svc.WithStorageQuota(repo)
		if err := svc.SetStorageQuota(ctx, superadmin, 7, v(5<<30)); err != nil {
			t.Fatal(err)
		}
		if len(repo.calls) != 1 || repo.calls[0] == nil || *repo.calls[0] != 5<<30 {
			t.Fatalf("nilai tidak tersimpan: %v", repo.calls)
		}
		if len(act.actions) != 1 || act.actions[0] != "SET_TENANT_STORAGE_QUOTA" {
			t.Fatalf("audit: %v", act.actions)
		}
	})

	t.Run("null = tak dibatasi", func(t *testing.T) {
		svc, _, _, _ := newCRUDRig()
		repo := &fakeStorageQuotaRepo{}
		svc.WithStorageQuota(repo)
		if err := svc.SetStorageQuota(ctx, superadmin, 7, nil); err != nil {
			t.Fatal(err)
		}
		if len(repo.calls) != 1 || repo.calls[0] != nil {
			t.Fatalf("harus menyimpan nil: %v", repo.calls)
		}
	})

	t.Run("batas atas diterima, di atasnya ditolak", func(t *testing.T) {
		svc, _, _, _ := newCRUDRig()
		repo := &fakeStorageQuotaRepo{}
		svc.WithStorageQuota(repo)
		if err := svc.SetStorageQuota(ctx, superadmin, 7, v(MaxStorageQuotaBytes)); err != nil {
			t.Fatalf("batas atas harus diterima: %v", err)
		}
		if err := svc.SetStorageQuota(ctx, superadmin, 7, v(MaxStorageQuotaBytes+1)); !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("di atas batas harus ErrInvalidInput: %v", err)
		}
		if len(repo.calls) != 1 {
			t.Fatalf("nilai ditolak tidak boleh tersimpan: %d", len(repo.calls))
		}
	})

	t.Run("tanpa repo kuota = galat konfigurasi", func(t *testing.T) {
		svc, _, _, _ := newCRUDRig()
		if err := svc.SetStorageQuota(ctx, superadmin, 7, v(1)); !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("want ErrInvalidInput, got %v", err)
		}
	})

	t.Run("tenant tidak ada diteruskan, tanpa audit", func(t *testing.T) {
		svc, _, _, act := newCRUDRig()
		svc.WithStorageQuota(&fakeStorageQuotaRepo{err: domain.ErrNotFound})
		if err := svc.SetStorageQuota(ctx, superadmin, 999, v(1)); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("want ErrNotFound, got %v", err)
		}
		if len(act.actions) != 0 {
			t.Fatalf("gagal tidak boleh tercatat: %v", act.actions)
		}
	})
}
