package preset

import (
	"context"
	"errors"
	"testing"

	"acs/internal/domain"
)

type fakePresetRepo struct {
	domain.PresetRepository
	byID    map[uint64]*domain.Preset
	updated []uint64
	deleted []uint64
}

func (f *fakePresetRepo) GetByID(_ context.Context, id uint64) (*domain.Preset, error) {
	if p, ok := f.byID[id]; ok {
		cp := *p
		return &cp, nil
	}
	return nil, domain.ErrNotFound
}
func (f *fakePresetRepo) Update(_ context.Context, p *domain.Preset) error {
	f.updated = append(f.updated, p.ID)
	return nil
}
func (f *fakePresetRepo) Delete(_ context.Context, id uint64) error {
	f.deleted = append(f.deleted, id)
	return nil
}

type nopActivity struct{}

func (nopActivity) Record(context.Context, *domain.ActivityLog) error { return nil }
func (nopActivity) ListByEntity(context.Context, string, uint64, domain.Pagination) ([]domain.ActivityLog, int, error) {
	return nil, 0, nil
}
func (nopActivity) ListByTenant(context.Context, *uint64, domain.ActivityLogFilter, domain.Pagination) ([]domain.ActivityLog, int, error) {
	return nil, 0, nil
}

func u64(v uint64) *uint64 { return &v }

func presetRig() (*Service, *fakePresetRepo) {
	repo := &fakePresetRepo{byID: map[uint64]*domain.Preset{
		1: {ID: 1, TenantID: u64(1), Name: "p-t1", Precondition: "{}", Configurations: "[]"},
		2: {ID: 2, TenantID: u64(2), Name: "p-t2", Precondition: "{}", Configurations: "[]"},
		3: {ID: 3, TenantID: nil, Name: "p-global", Precondition: "{}", Configurations: "[]"},
	}}
	return &Service{presets: repo, activity: nopActivity{}}, repo
}

func TestGet_ScopeBaca(t *testing.T) {
	svc, _ := presetRig()
	ctx := context.Background()
	t1 := domain.Actor{UserID: 5, TenantID: u64(1), Roles: []string{domain.RoleAdmin}}

	for _, tc := range []struct {
		name string
		id   uint64
		want error
	}{
		{"milik sendiri", 1, nil},
		{"global boleh dibaca", 3, nil},
		{"tenant lain ditolak", 2, domain.ErrForbidden},
		{"tidak ada", 404, domain.ErrNotFound},
	} {
		_, err := svc.Get(ctx, t1, tc.id)
		if !errors.Is(err, tc.want) && !(tc.want == nil && err == nil) {
			t.Errorf("%s: want %v, got %v", tc.name, tc.want, err)
		}
	}
	// Sebelumnya: actor non-superadmin tanpa tenant -> nil-deref panic.
	orphan := domain.Actor{UserID: 9, Roles: []string{domain.RoleAdmin}}
	if _, err := svc.Get(ctx, orphan, 1); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("actor tanpa tenant: want ErrForbidden, got %v", err)
	}
}

// Celah yang ditutup: preset GLOBAL menegakkan konfigurasi ke device semua
// tenant — ADMIN satu tenant tidak boleh mengubah/menghapusnya.
func TestMutasiPresetGlobal_HanyaSuperadmin(t *testing.T) {
	svc, repo := presetRig()
	ctx := context.Background()
	t1 := domain.Actor{UserID: 5, TenantID: u64(1), Roles: []string{domain.RoleAdmin}}
	super := domain.Actor{UserID: 1, Roles: []string{domain.RoleSuperadmin}}

	if err := svc.Update(ctx, t1, 3, domain.Preset{Name: "hijack", Precondition: "{}", Configurations: "[]"}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("update global oleh ADMIN: want ErrForbidden, got %v", err)
	}
	if err := svc.Delete(ctx, t1, 3); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("delete global oleh ADMIN: want ErrForbidden, got %v", err)
	}
	if len(repo.updated) != 0 || len(repo.deleted) != 0 {
		t.Fatalf("preset global tidak boleh tersentuh: updated=%v deleted=%v", repo.updated, repo.deleted)
	}
	// preset milik tenant sendiri tetap bisa
	if err := svc.Delete(ctx, t1, 1); err != nil {
		t.Fatalf("delete preset sendiri: %v", err)
	}
	// superadmin bisa memutasi preset global
	if err := svc.Delete(ctx, super, 3); err != nil {
		t.Fatalf("superadmin delete global: %v", err)
	}
}
