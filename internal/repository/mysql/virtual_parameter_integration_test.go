package mysql

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"

	"acs/internal/domain"
)

// Test integrasi MariaDB NYATA untuk virtual_parameters (migrations/0039): scope tenant/global, override, soft delete.
// Dilewati bila ACS_TEST_DSN tidak diisi.
func TestVirtualParameterRepository_Integration(t *testing.T) {
	dsn := os.Getenv("ACS_TEST_DSN")
	if dsn == "" {
		t.Skip("ACS_TEST_DSN tidak diisi — test integrasi MariaDB dilewati")
	}
	db, err := Connect(dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()
	repo := NewVirtualParameterRepository(db)

	// dua tenant sungguhan (FK)
	mk := func(code string) uint64 {
		res, err := db.Exec(`INSERT INTO tenants (tenant_uuid, code, name, is_active) VALUES (?, ?, ?, 1)`, uuid.NewString(), code, code)
		if err != nil {
			t.Fatalf("tenant %s: %v", code, err)
		}
		id, _ := res.LastInsertId()
		return uint64(id)
	}
	sfx := uuid.NewString()[:6]
	ta, tb := mk("vpa-"+sfx), mk("vpb-"+sfx)
	name := "rx_" + sfx
	created := []*domain.VirtualParameter{}
	t.Cleanup(func() {
		for _, v := range created {
			_, _ = db.Exec(`DELETE FROM virtual_parameters WHERE id = ?`, v.ID)
		}
		_, _ = db.Exec(`DELETE FROM tenants WHERE id IN (?, ?)`, ta, tb)
	})
	add := func(tenant *uint64, nm, expr string, active bool) *domain.VirtualParameter {
		v := &domain.VirtualParameter{TenantID: tenant, Name: nm, Label: nm, Expression: expr, IsActive: active}
		if err := repo.Create(ctx, v, nil); err != nil {
			t.Fatalf("create %s: %v", nm, err)
		}
		created = append(created, v)
		return v
	}
	glob := add(nil, name, "1", true)
	ownA := add(&ta, name, "2", true)
	add(&ta, "tambahan_"+sfx, "3", true)
	add(&ta, "mati_"+sfx, "4", false)
	add(&tb, "milik_b_"+sfx, "5", true)

	if glob.UUID == "" || glob.CreatedAt.IsZero() {
		t.Fatalf("Create harus mengisi uuid & created_at: %+v", glob)
	}

	// ListForTenant(A): aktif saja; definisi tenant menimpa global bernama sama; milik B tak ikut
	got, err := repo.ListForTenant(ctx, &ta)
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]domain.VirtualParameter{}
	for _, v := range got {
		if v.Name == name || v.Name == "tambahan_"+sfx || v.Name == "mati_"+sfx || v.Name == "milik_b_"+sfx {
			by[v.Name] = v
		}
	}
	if by[name].Expression != "2" || by[name].UUID != ownA.UUID {
		t.Errorf("definisi tenant harus menimpa global: %+v", by[name])
	}
	if _, ok := by["mati_"+sfx]; ok {
		t.Error("definisi nonaktif tidak boleh ikut")
	}
	if _, ok := by["milik_b_"+sfx]; ok {
		t.Error("definisi tenant lain bocor")
	}
	if _, ok := by["tambahan_"+sfx]; !ok {
		t.Error("definisi tenant A hilang")
	}
	// tenant B hanya melihat global + miliknya
	gotB, _ := repo.ListForTenant(ctx, &tb)
	seenGlobal := false
	for _, v := range gotB {
		if v.Name == name {
			seenGlobal = v.Expression == "1"
		}
		if v.Name == "tambahan_"+sfx {
			t.Error("definisi tenant A bocor ke B")
		}
	}
	if !seenGlobal {
		t.Error("tenant B harus melihat definisi global")
	}
	// tanpa tenant = hanya global
	gotG, _ := repo.ListForTenant(ctx, nil)
	for _, v := range gotG {
		if v.TenantID != nil {
			t.Errorf("ListForTenant(nil) hanya global, ada %+v", v)
		}
	}

	// FindByName / CountInScope memakai scope PERSIS (nil = global, bukan "semua")
	if v, err := repo.FindByName(ctx, nil, name); err != nil || v.UUID != glob.UUID {
		t.Errorf("FindByName(global): %+v %v", v, err)
	}
	if v, err := repo.FindByName(ctx, &ta, name); err != nil || v.UUID != ownA.UUID {
		t.Errorf("FindByName(A): %+v %v", v, err)
	}
	if _, err := repo.FindByName(ctx, &tb, name); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("B tidak punya definisi bernama itu sendiri: %v", err)
	}
	if n, _ := repo.CountInScope(ctx, &ta); n != 3 { // name, tambahan, mati (nonaktif tetap dihitung)
		t.Errorf("CountInScope(A) = %d, want 3", n)
	}

	// List dengan tenant: tenant + global; activeOnly menyaring
	rows, total, err := repo.List(ctx, &tb, false, domain.Pagination{Page: 1, PageSize: 500})
	if err != nil || total < 2 {
		t.Fatalf("List(B): %d %v", total, err)
	}
	for _, v := range rows {
		if v.TenantID != nil && *v.TenantID != tb {
			t.Errorf("List(B) bocor: %+v", v)
		}
	}

	// Update + soft delete: baris hilang dari semua pembacaan, nama bebas dipakai lagi
	ownA.Expression, ownA.Label = "22", "Baru"
	if err := repo.Update(ctx, ownA, nil); err != nil {
		t.Fatal(err)
	}
	if v, _ := repo.GetByUUID(ctx, ownA.UUID); v.Expression != "22" || v.Label != "Baru" {
		t.Errorf("update tidak tersimpan: %+v", v)
	}
	if err := repo.SoftDelete(ctx, ownA.ID, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetByUUID(ctx, ownA.UUID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("setelah hapus harus ErrNotFound: %v", err)
	}
	if err := repo.SoftDelete(ctx, ownA.ID, 1); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("hapus dua kali harus ErrNotFound: %v", err)
	}
	if _, err := repo.FindByName(ctx, &ta, name); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("nama harus bebas setelah dihapus (bukan 'kuburan'): %v", err)
	}
	again := add(&ta, name, "7", true) // nama yang sama boleh dibuat lagi
	if again.UUID == ownA.UUID {
		t.Error("uuid baru diharapkan")
	}
}
