package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"acs/internal/domain"
)

type rolesUsers struct {
	*fakeUserRepoAuth
	roles map[uint64][]string
}

func (r rolesUsers) RolesByUserID(_ context.Context, id uint64) ([]string, error) {
	return r.roles[id], nil
}

func impFixture(t *testing.T) (*Service, *fakeUserRepoAuth, domain.Actor) {
	t.Helper()
	tenant := uint64(1)
	super := &domain.User{ID: 1, UserUUID: "u1", Username: "root", IsActive: true, Roles: []string{domain.RoleSuperadmin}}
	admin := &domain.User{ID: 2, UserUUID: "u2", TenantID: &tenant, Username: "adm", IsActive: true}
	super2 := &domain.User{ID: 3, UserUUID: "u3", Username: "root2", IsActive: true}
	off := &domain.User{ID: 4, UserUUID: "u4", TenantID: &tenant, Username: "off", IsActive: false}
	notenant := &domain.User{ID: 5, UserUUID: "u5", Username: "nt", IsActive: true}
	base := &fakeUserRepoAuth{byID: map[uint64]*domain.User{1: super, 2: admin, 3: super2, 4: off, 5: notenant}}
	users := rolesUsers{base, map[uint64][]string{
		1: {domain.RoleSuperadmin}, 2: {domain.RoleAdmin}, 3: {domain.RoleSuperadmin}, 4: {domain.RoleNOC}, 5: {domain.RoleNOC},
	}}
	tenants := &fakeTenantRepoAuthTest{byID: map[uint64]*domain.Tenant{1: {ID: 1, IsActive: true}}}
	svc := NewService(users, &fakeTokenRepoAuth{}, tenants, &fakeActivityRepoAuth{}, []byte("test-secret-at-least-32-bytes!!"), 8*time.Hour, "", "", "", "")
	return svc, base, domain.Actor{UserID: 1, Roles: []string{domain.RoleSuperadmin}}
}

func TestImpersonate_AlurSah(t *testing.T) {
	svc, _, super := impFixture(t)
	ctx := context.Background()
	res, err := svc.Impersonate(ctx, super, 2)
	if err != nil {
		t.Fatal(err)
	}
	if ttl := time.Until(res.ExpiresAt); ttl > impersonateTTL || ttl < impersonateTTL-time.Minute {
		t.Fatalf("TTL harus ~%s (token pendek), dapat %s", impersonateTTL, ttl)
	}
	actor, err := svc.ResolveActor(ctx, res.Token)
	if err != nil {
		t.Fatalf("token impersonate harus sesi sah: %v", err)
	}
	if actor.UserID != 2 || actor.ImpersonatorID != 1 || !actor.HasRole(domain.RoleAdmin) || actor.IsSuperadmin() {
		t.Fatalf("actor harus = target (ADMIN) dengan jejak impersonator: %+v", actor)
	}
	if err := svc.EndImpersonation(ctx, *actor); err != nil {
		t.Fatal(err)
	}
	if err := svc.EndImpersonation(ctx, super); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("EndImpersonation di luar sesi impersonate harus ditolak: %v", err)
	}
}

func TestImpersonate_Penolakan(t *testing.T) {
	svc, _, super := impFixture(t)
	ctx := context.Background()
	tn := uint64(1)
	cases := []struct {
		name  string
		actor domain.Actor
		to    uint64
		want  error
	}{
		{"ADMIN tenant tidak boleh", domain.Actor{UserID: 2, TenantID: &tn, Roles: []string{domain.RoleAdmin}}, 2, domain.ErrForbidden},
		{"sesi impersonate tidak boleh berantai", domain.Actor{UserID: 1, Roles: []string{domain.RoleSuperadmin}, ImpersonatorID: 9}, 2, domain.ErrForbidden},
		{"tidak boleh ke diri sendiri", super, 1, domain.ErrInvalidInput},
		{"tidak boleh ke SUPERADMIN lain", super, 3, ErrImpersonationForbidden},
		{"target nonaktif", super, 4, ErrImpersonationForbidden},
		{"target tanpa tenant", super, 5, ErrImpersonationForbidden},
		{"target tidak ada", super, 99, domain.ErrNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := svc.Impersonate(ctx, tc.actor, tc.to); !errors.Is(err, tc.want) {
				t.Fatalf("want %v, got %v", tc.want, err)
			}
		})
	}
}

// Token impersonate mati bila SUPERADMIN aslinya dinonaktifkan atau sesinya dicabut.
func TestImpersonate_MatiBilaImpersonatorDicabut(t *testing.T) {
	ctx := context.Background()

	svc, base, super := impFixture(t)
	res, _ := svc.Impersonate(ctx, super, 2)
	base.byID[1].IsActive = false
	if _, err := svc.ResolveActor(ctx, res.Token); err == nil {
		t.Fatal("SUPERADMIN dinonaktifkan: token impersonate harus tertolak")
	}

	svc, base, super = impFixture(t)
	res, _ = svc.Impersonate(ctx, super, 2)
	later := time.Now().Add(2 * time.Second)
	base.byID[1].PasswordChangedAt = &later
	if _, err := svc.ResolveActor(ctx, res.Token); err == nil {
		t.Fatal("sesi SUPERADMIN dicabut (ganti password/role): token impersonate harus tertolak")
	}

	// target dinonaktifkan -> juga tertolak (cek sesi target)
	svc, base, super = impFixture(t)
	res, _ = svc.Impersonate(ctx, super, 2)
	base.byID[2].IsActive = false
	if _, err := svc.ResolveActor(ctx, res.Token); err == nil {
		t.Fatal("target dinonaktifkan: token impersonate harus tertolak")
	}
}
