package mysql

import (
	"context"
	"os"
	"testing"

	"acs/internal/domain"
)

// Regresi (MariaDB nyata): setiap tabel ref_* di whitelist harus bisa di-List,
// GetByCode, dan GetByID. Dua bug lama yang lolos dari unit test:
//   - ref_webhook_event_types tidak ada di whitelist (webhook tak bisa dibuat);
//   - ref_parameter_types menghasilkan SQL "code AS name AS name" (500).
//
// Dilewati bila ACS_TEST_DSN tidak diisi.
func TestRefRepository_AllWhitelistedTables_Integration(t *testing.T) {
	dsn := os.Getenv("ACS_TEST_DSN")
	if dsn == "" {
		t.Skip("ACS_TEST_DSN tidak diisi — test integrasi MariaDB dilewati")
	}
	db, err := Connect(dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	repo := NewRefRepository(db)
	ctx := context.Background()

	for table := range refTableWhitelist {
		rows, err := repo.List(ctx, table)
		if err != nil {
			t.Errorf("%s: List: %v", table, err)
			continue
		}
		if len(rows) == 0 {
			t.Errorf("%s: tidak ada baris seed", table)
			continue
		}
		first := rows[0]
		if first.Name == "" {
			t.Errorf("%s: name kosong (mis. alias salah)", table)
		}
		if got, err := repo.GetByCode(ctx, table, first.Code); err != nil || got.ID != first.ID {
			t.Errorf("%s: GetByCode(%q): %v", table, first.Code, err)
		}
		if got, err := repo.GetByID(ctx, table, first.ID); err != nil || got.Code != first.Code {
			t.Errorf("%s: GetByID(%d): %v", table, first.ID, err)
		}
	}
	// Dua tabel yang dulu rusak harus benar-benar ada di whitelist.
	for _, table := range []string{domain.RefTableWebhookEventTypes, domain.RefTableParameterTypes} {
		if !refTableWhitelist[table] {
			t.Errorf("%s harus di whitelist", table)
		}
	}
}
