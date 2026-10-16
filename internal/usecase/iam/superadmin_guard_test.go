package iam

import (
	"context"
	"errors"
	"testing"

	"acs/internal/domain"
)

// ADMIN tenant TIDAK boleh membaca/mengubah/mereset/menonaktifkan akun SUPERADMIN,
// bahkan bila akun itu (secara salah) memiliki tenant_id yang sama dengannya.
func TestAdminTenant_TidakBisaMemutasiSuperadmin(t *testing.T) {
	svc, _, _, _ := newCRUDRig()
	svc.users.(*fakeUsers).byID[300] = &domain.User{ID: 300, TenantID: u64(7), Username: "root-ber-tenant", Roles: []string{domain.RoleSuperadmin}}
	ctx := context.Background()

	if _, err := svc.GetUser(ctx, adminT7, 300); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("GetUser: want ErrForbidden, got %v", err)
	}
	inactive := false
	if _, err := svc.UpdateUser(ctx, adminT7, 300, UpdateUserInput{IsActive: &inactive}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("UpdateUser(nonaktifkan): want ErrForbidden, got %v", err)
	}
	if err := svc.ResetUserPassword(ctx, adminT7, 300, "PasswordBaru123!"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("ResetUserPassword: want ErrForbidden, got %v", err)
	}
	if _, err := svc.ReplaceUserRoles(ctx, adminT7, 300, []string{domain.RoleViewer}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("ReplaceUserRoles: want ErrForbidden, got %v", err)
	}
	if err := svc.DeleteUser(ctx, adminT7, 300); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("DeleteUser: want ErrForbidden, got %v", err)
	}
	// sesama superadmin tetap boleh membaca
	if _, err := svc.GetUser(ctx, superadmin, 300); err != nil {
		t.Fatalf("superadmin harus boleh: %v", err)
	}
}
