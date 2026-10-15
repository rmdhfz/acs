package iam

import (
	"context"
	"errors"
	"strings"
	"testing"

	"acs/internal/domain"
)

type fakeTenants struct {
	domain.TenantRepository
	byID    map[uint64]*domain.Tenant
	deleted []uint64
}

func (f *fakeTenants) GetByID(_ context.Context, id uint64) (*domain.Tenant, error) {
	if t, ok := f.byID[id]; ok {
		cp := *t
		return &cp, nil
	}
	return nil, domain.ErrNotFound
}
func (f *fakeTenants) SoftDelete(_ context.Context, id, _ uint64) error {
	f.deleted = append(f.deleted, id)
	return nil
}

type fakeUsers struct {
	domain.UserRepository
	byID map[uint64]*domain.User
}

func (f *fakeUsers) GetByID(_ context.Context, id uint64) (*domain.User, error) {
	if u, ok := f.byID[id]; ok {
		cp := *u
		return &cp, nil
	}
	return nil, domain.ErrNotFound
}

type fakeUsage struct {
	domain.UsageRepository
	tenant []domain.UsageCount
}

func (f *fakeUsage) TenantUsage(context.Context, uint64) ([]domain.UsageCount, error) {
	return f.tenant, nil
}

type fakeActivity struct{ actions []string }

func (f *fakeActivity) Record(_ context.Context, a *domain.ActivityLog) error {
	f.actions = append(f.actions, a.Action)
	return nil
}
func (f *fakeActivity) ListByEntity(context.Context, string, uint64, domain.Pagination) ([]domain.ActivityLog, int, error) {
	return nil, 0, nil
}
func (f *fakeActivity) ListByTenant(context.Context, *uint64, domain.ActivityLogFilter, domain.Pagination) ([]domain.ActivityLog, int, error) {
	return nil, 0, nil
}

func u64(v uint64) *uint64 { return &v }

func newCRUDRig() (*Service, *fakeTenants, *fakeUsage, *fakeActivity) {
	tenants := &fakeTenants{byID: map[uint64]*domain.Tenant{
		7: {ID: 7, Code: "ACME", Name: "Acme"},
		8: {ID: 8, Code: "BETA", Name: "Beta"},
	}}
	users := &fakeUsers{byID: map[uint64]*domain.User{
		100: {ID: 100, TenantID: u64(7), Username: "alice"},
		200: {ID: 200, TenantID: u64(8), Username: "bob"},
	}}
	usage, act := &fakeUsage{}, &fakeActivity{}
	return NewService(tenants, users, nil, act, nil, usage), tenants, usage, act
}

var (
	superadmin = domain.Actor{UserID: 1, Roles: []string{domain.RoleSuperadmin}}
	adminT7    = domain.Actor{UserID: 2, TenantID: u64(7), Roles: []string{domain.RoleAdmin}}
)

func TestDeleteTenant_Guard(t *testing.T) {
	svc, tenants, usage, act := newCRUDRig()
	ctx := context.Background()

	// non-superadmin ditolak
	if err := svc.DeleteTenant(ctx, adminT7, 8); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("ADMIN: want ErrForbidden, got %v", err)
	}
	// tidak ada
	if err := svc.DeleteTenant(ctx, superadmin, 999); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	// masih punya user/device aktif -> 409 yang menuntun, tidak menghapus
	usage.tenant = []domain.UsageCount{{Kind: "user", Count: 2}, {Kind: "device", Count: 40}}
	err := svc.DeleteTenant(ctx, superadmin, 8)
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("want ErrConflict, got %v", err)
	}
	for _, want := range []string{"2 user", "40 device", "is_active=false"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("pesan harus memuat %q: %v", want, err)
		}
	}
	if len(tenants.deleted) != 0 {
		t.Fatal("tenant yang masih dipakai tidak boleh terhapus")
	}
	// kosong -> terhapus + audit
	usage.tenant = nil
	if err := svc.DeleteTenant(ctx, superadmin, 8); err != nil {
		t.Fatal(err)
	}
	if len(tenants.deleted) != 1 || tenants.deleted[0] != 8 {
		t.Fatalf("tenant 8 harus terhapus, got %v", tenants.deleted)
	}
	if len(act.actions) != 1 || act.actions[0] != "DELETE_TENANT" {
		t.Fatalf("audit trail: %v", act.actions)
	}
}

func TestDeleteTenant_TenantSendiriDitolak(t *testing.T) {
	svc, tenants, _, _ := newCRUDRig()
	// superadmin yang terikat ke tenant 7 tidak boleh menghapus tenant 7 (self-lockout)
	selfBound := domain.Actor{UserID: 1, TenantID: u64(7), Roles: []string{domain.RoleSuperadmin}}
	if err := svc.DeleteTenant(context.Background(), selfBound, 7); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("want ErrForbidden, got %v", err)
	}
	if len(tenants.deleted) != 0 {
		t.Fatal("tidak boleh terhapus")
	}
}

func TestGetTenant_Scope(t *testing.T) {
	svc, _, _, _ := newCRUDRig()
	ctx := context.Background()

	if _, err := svc.GetTenant(ctx, adminT7, 7); err != nil {
		t.Fatalf("ADMIN tenant sendiri: %v", err)
	}
	if _, err := svc.GetTenant(ctx, adminT7, 8); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("ADMIN tenant lain: want ErrForbidden, got %v", err)
	}
	if _, err := svc.GetTenant(ctx, superadmin, 8); err != nil {
		t.Fatalf("superadmin lintas tenant: %v", err)
	}
	if _, err := svc.GetTenant(ctx, superadmin, 999); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	// ADMIN tanpa tenant (akun salah konfigurasi) ditolak, bukan nil-deref.
	orphan := domain.Actor{UserID: 3, Roles: []string{domain.RoleAdmin}}
	if _, err := svc.GetTenant(ctx, orphan, 7); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("ADMIN tanpa tenant: want ErrForbidden, got %v", err)
	}
}

func TestGetUser_Scope(t *testing.T) {
	svc, _, _, _ := newCRUDRig()
	ctx := context.Background()

	if u, err := svc.GetUser(ctx, adminT7, 100); err != nil || u.Username != "alice" {
		t.Fatalf("ADMIN user tenant sendiri: u=%v err=%v", u, err)
	}
	if _, err := svc.GetUser(ctx, adminT7, 200); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("ADMIN user tenant lain: want ErrForbidden, got %v", err)
	}
	if _, err := svc.GetUser(ctx, superadmin, 200); err != nil {
		t.Fatalf("superadmin: %v", err)
	}
	noc := domain.Actor{UserID: 4, TenantID: u64(7), Roles: []string{domain.RoleNOC}}
	if _, err := svc.GetUser(ctx, noc, 100); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("NOC tidak boleh baca profil user: want ErrForbidden, got %v", err)
	}
}
