package tag

import (
	"context"
	"errors"
	"testing"

	"acs/internal/domain"
)

func strp(s string) *string { return &s }

func tagRig() (*Service, *fakeTagRepo) {
	repo := newFakeTagRepo()
	repo.byID[10] = &domain.Tag{ID: 10, TenantID: u64(1), Name: "t1", Color: strp("#ff0000")}
	repo.byID[20] = &domain.Tag{ID: 20, TenantID: u64(2), Name: "t2"}
	repo.byID[30] = &domain.Tag{ID: 30, TenantID: nil, Name: "global"}
	return NewService(repo, nilActivity{}), repo
}

func TestUpdate_ParsialDanScope(t *testing.T) {
	svc, repo := tagRig()
	ctx := context.Background()
	t1Admin := domain.Actor{UserID: 5, TenantID: u64(1), Roles: []string{domain.RoleAdmin}}

	got, err := svc.Update(ctx, t1Admin, 10, UpdateInput{Name: strp("renamed")})
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "renamed" || got.Color == nil || *got.Color != "#ff0000" {
		t.Fatalf("hanya name yang berubah, color tetap: %+v", got)
	}
	// color "" = kosongkan
	got, err = svc.Update(ctx, t1Admin, 10, UpdateInput{Color: strp("")})
	if err != nil || got.Color != nil {
		t.Fatalf("color harus dikosongkan: %+v err=%v", got, err)
	}
	// nama kosong ditolak
	if _, err := svc.Update(ctx, t1Admin, 10, UpdateInput{Name: strp("")}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("want ErrInvalidInput, got %v", err)
	}
	// tag tenant lain ditolak, repo tidak disentuh
	n := len(repo.updated)
	if _, err := svc.Update(ctx, t1Admin, 20, UpdateInput{Name: strp("x")}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("tag tenant lain: want ErrForbidden, got %v", err)
	}
	if len(repo.updated) != n {
		t.Fatal("penolakan tidak boleh menyentuh repo")
	}
	if _, err := svc.Update(ctx, t1Admin, 404, UpdateInput{Name: strp("x")}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

// Celah yang ditutup: tag GLOBAL (tenant_id NULL, dipakai semua tenant) tidak
// boleh diubah/dihapus ADMIN satu tenant — hanya superadmin.
func TestMutasiTagGlobal_HanyaSuperadmin(t *testing.T) {
	svc, repo := tagRig()
	ctx := context.Background()
	t1Admin := domain.Actor{UserID: 5, TenantID: u64(1), Roles: []string{domain.RoleAdmin}}
	super := domain.Actor{UserID: 1, Roles: []string{domain.RoleSuperadmin}}

	if _, err := svc.Update(ctx, t1Admin, 30, UpdateInput{Name: strp("hijack")}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("update tag global oleh ADMIN: want ErrForbidden, got %v", err)
	}
	if err := svc.Delete(ctx, t1Admin, 30); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("delete tag global oleh ADMIN: want ErrForbidden, got %v", err)
	}
	if len(repo.deleted) != 0 {
		t.Fatal("tag global tidak boleh terhapus oleh ADMIN tenant")
	}
	if _, err := svc.Update(ctx, super, 30, UpdateInput{Name: strp("ok")}); err != nil {
		t.Fatalf("superadmin update tag global: %v", err)
	}
	if err := svc.Delete(ctx, super, 30); err != nil {
		t.Fatalf("superadmin delete tag global: %v", err)
	}
}

// Sebelumnya Delete memakai *actor.TenantID tanpa cek nil -> panic untuk akun
// non-superadmin tanpa tenant.
func TestDelete_ActorTanpaTenantTidakPanic(t *testing.T) {
	svc, _ := tagRig()
	orphan := domain.Actor{UserID: 9, Roles: []string{domain.RoleAdmin}} // TenantID nil
	if err := svc.Delete(context.Background(), orphan, 10); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("want ErrForbidden, got %v", err)
	}
}
