package mysql

import (
	"context"
	"os"
	"testing"

	"acs/internal/domain"
)

// MariaDB nyata: Record mengisi ip_address dari konteks permintaan, dan ADMIN tenant tidak
// melihat baris tenant_id NULL (aksi superadmin/sistem). Dilewati tanpa ACS_TEST_DSN.
func TestActivityLog_IPDanScopeTenant_Integration(t *testing.T) {
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
	repo := NewActivityLogRepository(db)

	var tid uint64
	if err := db.GetContext(ctx, &tid, `SELECT id FROM tenants WHERE is_deleted = 0 LIMIT 1`); err != nil {
		t.Skip("tidak ada tenant")
	}
	marker := "ZZ_TEST_IP_SCOPE"
	t.Cleanup(func() { _, _ = db.ExecContext(ctx, `DELETE FROM activity_logs WHERE action = ?`, marker) })

	rctx := domain.WithRequestMeta(ctx, domain.RequestMeta{IP: "198.51.100.77"})
	if err := repo.Record(rctx, &domain.ActivityLog{TenantID: &tid, Action: marker, EntityType: "test"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.Record(rctx, &domain.ActivityLog{TenantID: nil, Action: marker, EntityType: "test"}); err != nil {
		t.Fatal(err)
	}
	rows, _, err := repo.ListByTenant(ctx, &tid, domain.ActivityLogFilter{Action: marker}, domain.Pagination{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].IPAddress == nil || *rows[0].IPAddress != "198.51.100.77" {
		t.Fatalf("ADMIN tenant harus melihat tepat 1 baris (tanpa NULL) ber-IP: %+v", rows)
	}
	all, _, _ := repo.ListByTenant(ctx, nil, domain.ActivityLogFilter{Action: marker}, domain.Pagination{Page: 1, PageSize: 10})
	if len(all) != 2 {
		t.Fatalf("superadmin (tanpa filter) melihat semua: %d", len(all))
	}
}
