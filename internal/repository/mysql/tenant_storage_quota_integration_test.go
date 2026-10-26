package mysql

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"

	"acs/internal/domain"
)

// Integrasi MariaDB NYATA: kuota penyimpanan tenant (migrations/0043). Total hanya menghitung berkas AKTIF milik tenant
// itu; berkas terhapus dan milik tenant lain tidak dihitung; nil = tak dibatasi; tenant tak ada = ErrNotFound; kolom baru ikut
// terbaca lewat SELECT * ke domain.Tenant. Dilewati tanpa ACS_TEST_DSN.
func TestTenantStorageQuota_Integration(t *testing.T) {
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

	tenants := NewTenantRepository(db)
	quota := NewTenantStorageQuotaRepository(db)
	sfx := uuid.NewString()[:8]
	mk := func(code string) *domain.Tenant {
		tn := &domain.Tenant{TenantUUID: uuid.NewString(), Code: code + sfx, Name: "it-quota-" + code, IsActive: true}
		if err := tenants.Create(ctx, tn); err != nil {
			t.Fatalf("create tenant: %v", err)
		}
		t.Cleanup(func() {
			_, _ = db.Exec(`DELETE FROM files WHERE tenant_id = ?`, tn.ID)
			_, _ = db.Exec(`DELETE FROM tenants WHERE id = ?`, tn.ID)
		})
		return tn
	}
	a, b := mk("A"), mk("B")
	addFile := func(tenantID uint64, size uint64, deleted bool) {
		del := 0
		if deleted {
			del = 1
		}
		if _, err := db.Exec(`INSERT INTO files (file_uuid, tenant_id, file_type, file_name, storage_key, file_size_bytes, is_deleted) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			uuid.NewString(), tenantID, "3 Vendor Configuration File", "f.xml", "files/config/"+uuid.NewString(), size, del); err != nil {
			t.Fatalf("insert file: %v", err)
		}
	}
	addFile(a.ID, 200, false)
	addFile(a.ID, 300, false)
	addFile(a.ID, 999, true)   // terhapus: tidak dihitung
	addFile(b.ID, 5000, false) // tenant lain: tidak dihitung

	// tanpa batas: (nil, 0) walau berkas ada
	if max, used, err := quota.StorageUsage(ctx, a.ID); err != nil || max != nil || used != 0 {
		t.Fatalf("tanpa batas harus (nil,0): max=%v used=%d err=%v", max, used, err)
	}

	// dengan batas: total hanya berkas aktif tenant itu
	limit := uint64(1000)
	if err := quota.SetMaxStorageBytes(ctx, a.ID, &limit, nil); err != nil {
		t.Fatal(err)
	}
	max, used, err := quota.StorageUsage(ctx, a.ID)
	if err != nil || max == nil || *max != 1000 || used != 500 {
		t.Fatalf("StorageUsage: max=%v used=%d err=%v (harapan 1000/500)", max, used, err)
	}
	if bMax, _, err := quota.StorageUsage(ctx, b.ID); err != nil || bMax != nil {
		t.Fatalf("tenant lain tidak boleh ikut berbatas: %v %v", bMax, err)
	}

	// kolom baru terbaca lewat SELECT * ke domain.Tenant
	got, err := tenants.GetByID(ctx, a.ID)
	if err != nil || got.MaxStorageBytes == nil || *got.MaxStorageBytes != 1000 {
		t.Fatalf("GetByID harus memuat max_storage_bytes: %+v err=%v", got, err)
	}

	// batas besar (> int32/uint32) tersimpan utuh
	big := uint64(50) << 40
	if err := quota.SetMaxStorageBytes(ctx, a.ID, &big, nil); err != nil {
		t.Fatal(err)
	}
	if max, _, _ := quota.StorageUsage(ctx, a.ID); max == nil || *max != big {
		t.Fatalf("batas besar rusak: %v", max)
	}

	// kembali tak dibatasi
	if err := quota.SetMaxStorageBytes(ctx, a.ID, nil, nil); err != nil {
		t.Fatal(err)
	}
	if max, used, _ := quota.StorageUsage(ctx, a.ID); max != nil || used != 0 {
		t.Fatalf("null harus kembali tak dibatasi: %v %d", max, used)
	}

	// tenant tidak ada
	if err := quota.SetMaxStorageBytes(ctx, 987654321, &limit, nil); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("tenant tak ada harus ErrNotFound: %v", err)
	}
}
