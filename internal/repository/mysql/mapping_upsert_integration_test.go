package mysql

import (
	"context"
	"os"
	"testing"

	"acs/internal/domain"
)

// Regresi (MariaDB nyata): Upsert mapping pada scope generik (device_model_id &
// software_version_pattern NULL) harus MEMPERBARUI baris yang sama, bukan menambah
// duplikat bersaing (UNIQUE KEY tidak menangkap NULL). Dilewati tanpa ACS_TEST_DSN.
func TestMappingUpsert_ScopeGenerikTidakDuplikat_Integration(t *testing.T) {
	dsn := os.Getenv("ACS_TEST_DSN")
	if dsn == "" {
		t.Skip("ACS_TEST_DSN tidak diisi — test integrasi MariaDB dilewati")
	}
	db, err := Connect(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()

	var vendorID, dmvID uint64
	if err := db.GetContext(ctx, &vendorID, `SELECT id FROM ref_vendors WHERE is_deleted = 0 LIMIT 1`); err != nil {
		t.Fatal(err)
	}
	if err := db.GetContext(ctx, &dmvID, `SELECT id FROM ref_data_model_versions WHERE is_deleted = 0 LIMIT 1`); err != nil {
		t.Fatal(err)
	}
	const key = "zz.test.upsert.generik"
	t.Cleanup(func() { _, _ = db.ExecContext(ctx, `DELETE FROM vendor_parameter_mappings WHERE logical_key = ?`, key) })
	_, _ = db.ExecContext(ctx, `DELETE FROM vendor_parameter_mappings WHERE logical_key = ?`, key)

	repo := NewVendorParameterMappingRepository(db)
	mk := func(path string) *domain.VendorParameterMapping {
		return &domain.VendorParameterMapping{VendorID: vendorID, DataModelVersionID: dmvID, LogicalKey: key, TR069Path: path}
	}
	a, b := mk("A.B.C"), mk("X.Y.Z")
	if err := repo.Upsert(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err := repo.Upsert(ctx, b); err != nil {
		t.Fatal(err)
	}
	if a.ID == 0 || a.ID != b.ID {
		t.Fatalf("upsert kedua harus memperbarui baris yang sama: id %d vs %d", a.ID, b.ID)
	}
	var n int
	if err := db.GetContext(ctx, &n, `SELECT COUNT(*) FROM vendor_parameter_mappings WHERE logical_key = ? AND is_deleted = 0`, key); err != nil || n != 1 {
		t.Fatalf("harus tepat 1 baris, n=%d err=%v", n, err)
	}
	var path string
	_ = db.GetContext(ctx, &path, `SELECT tr069_path FROM vendor_parameter_mappings WHERE id = ?`, a.ID)
	if path != "X.Y.Z" {
		t.Fatalf("path harus yang terbaru, got %q", path)
	}
}
