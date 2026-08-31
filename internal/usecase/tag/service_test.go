package tag

import (
	"context"
	"errors"
	"testing"

	"acs/internal/domain"
)

// fakeTagRepo — in-memory minimal domain.TagRepository untuk menguji authz
// tenant di layer usecase (bukan perilaku SQL).
type fakeTagRepo struct {
	byID     map[uint64]*domain.Tag
	assigned map[[2]uint64]bool // {deviceID, tagID}
}

func newFakeTagRepo() *fakeTagRepo {
	return &fakeTagRepo{byID: map[uint64]*domain.Tag{}, assigned: map[[2]uint64]bool{}}
}

func (r *fakeTagRepo) Create(context.Context, *domain.Tag) error { return nil }
func (r *fakeTagRepo) GetByID(_ context.Context, id uint64) (*domain.Tag, error) {
	if t, ok := r.byID[id]; ok {
		return t, nil
	}
	return nil, domain.ErrNotFound
}
func (r *fakeTagRepo) List(context.Context, *uint64, domain.Pagination) ([]domain.Tag, int, error) {
	return nil, 0, nil
}
func (r *fakeTagRepo) Delete(context.Context, uint64) error { return nil }
func (r *fakeTagRepo) AssignToDevice(_ context.Context, deviceID, tagID uint64) error {
	r.assigned[[2]uint64{deviceID, tagID}] = true
	return nil
}
func (r *fakeTagRepo) RemoveFromDevice(_ context.Context, deviceID, tagID uint64) error {
	delete(r.assigned, [2]uint64{deviceID, tagID})
	return nil
}
func (r *fakeTagRepo) ListByDevice(context.Context, uint64) ([]domain.Tag, error) { return nil, nil }

// nilActivity — ActivityLogRepository yang tidak melakukan apa-apa.
type nilActivity struct{}

func (nilActivity) Record(context.Context, *domain.ActivityLog) error { return nil }
func (nilActivity) ListByEntity(context.Context, string, uint64, domain.Pagination) ([]domain.ActivityLog, int, error) {
	return nil, 0, nil
}

func u64(v uint64) *uint64 { return &v }

func TestAssignToDevice_TenantScope(t *testing.T) {
	repo := newFakeTagRepo()
	repo.byID[10] = &domain.Tag{ID: 10, TenantID: u64(1), Name: "tenant1-tag"}
	repo.byID[20] = &domain.Tag{ID: 20, TenantID: u64(2), Name: "tenant2-tag"}
	repo.byID[30] = &domain.Tag{ID: 30, TenantID: nil, Name: "global-tag"}
	svc := NewService(repo, nilActivity{})

	tenant1Admin := domain.Actor{UserID: 5, TenantID: u64(1), Roles: []string{domain.RoleAdmin}}

	// tag milik tenant sendiri -> OK
	if err := svc.AssignToDevice(context.Background(), tenant1Admin, 100, 10); err != nil {
		t.Fatalf("tag tenant sendiri: tak terduga error %v", err)
	}
	// tag global (tenant_id NULL) -> OK utk siapa pun
	if err := svc.AssignToDevice(context.Background(), tenant1Admin, 100, 30); err != nil {
		t.Fatalf("tag global: tak terduga error %v", err)
	}
	// tag milik tenant lain -> ErrForbidden (celah yang sebelumnya "diabaikan sementara")
	if err := svc.AssignToDevice(context.Background(), tenant1Admin, 100, 20); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("tag tenant lain: want ErrForbidden, got %v", err)
	}
	if repo.assigned[[2]uint64{100, 20}] {
		t.Fatal("assign tag tenant lain seharusnya tidak menyentuh repo")
	}

	// superadmin bebas lintas tenant
	super := domain.Actor{UserID: 1, Roles: []string{domain.RoleSuperadmin}}
	if err := svc.AssignToDevice(context.Background(), super, 100, 20); err != nil {
		t.Fatalf("superadmin: tak terduga error %v", err)
	}
}

func TestRemoveFromDevice_TenantScope(t *testing.T) {
	repo := newFakeTagRepo()
	repo.byID[20] = &domain.Tag{ID: 20, TenantID: u64(2), Name: "tenant2-tag"}
	repo.assigned[[2]uint64{100, 20}] = true
	svc := NewService(repo, nilActivity{})

	tenant1Admin := domain.Actor{UserID: 5, TenantID: u64(1), Roles: []string{domain.RoleAdmin}}
	if err := svc.RemoveFromDevice(context.Background(), tenant1Admin, 100, 20); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("want ErrForbidden, got %v", err)
	}
	if !repo.assigned[[2]uint64{100, 20}] {
		t.Fatal("remove tag tenant lain seharusnya tidak menyentuh repo")
	}
}
