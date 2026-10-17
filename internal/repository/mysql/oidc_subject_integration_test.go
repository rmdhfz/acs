package mysql

import (
	"context"
	"os"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"

	"acs/internal/domain"
)

// Integrasi MariaDB NYATA untuk ikatan identitas IdP (migrations/0031): compare-and-set hanya satu pemenang pada
// pengikatan paralel, dan mengubah email mengosongkan ikatan. Dilewati tanpa ACS_TEST_DSN.
func TestUserRepository_OIDCSubject_Integration(t *testing.T) {
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
	users := NewUserRepository(db)

	suffix := uuid.NewString()[:8]
	u := &domain.User{
		UserUUID: uuid.NewString(), Username: "it-oidc-" + suffix, Email: "it-oidc-" + suffix + "@example.test",
		PasswordHash: "x", FullName: "IT OIDC", IsActive: true,
	}
	if err := users.Create(ctx, u); err != nil {
		t.Fatalf("create user: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM users WHERE id = ?`, u.ID) })

	got, err := users.GetByID(ctx, u.ID)
	if err != nil || got.OIDCSubject != nil {
		t.Fatalf("state awal harus belum terikat: %+v err=%v", got, err)
	}

	// 20 pengikatan paralel dengan sub berbeda: tepat satu menang
	var wins atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ok, err := users.BindOIDCSubject(ctx, u.ID, "https://accounts.google.com|sub-"+uuid.NewString()[:6])
			if err != nil {
				t.Error(err)
			}
			if ok {
				wins.Add(1)
			}
		}(i)
	}
	wg.Wait()
	if wins.Load() != 1 {
		t.Fatalf("tepat satu pengikatan yang boleh menang, dapat %d", wins.Load())
	}
	bound, _ := users.GetByID(ctx, u.ID)
	if bound.OIDCSubject == nil || *bound.OIDCSubject == "" {
		t.Fatal("ikatan harus tersimpan")
	}

	// update tanpa mengubah email: ikatan tetap
	bound.FullName = "IT OIDC 2"
	if err := users.Update(ctx, bound); err != nil {
		t.Fatal(err)
	}
	if again, _ := users.GetByID(ctx, u.ID); again.OIDCSubject == nil {
		t.Fatal("ikatan tak boleh hilang saat email tidak berubah")
	}

	// mengubah email mengosongkan ikatan -> bisa diikat ulang
	bound.Email = "baru-" + suffix + "@example.test"
	if err := users.Update(ctx, bound); err != nil {
		t.Fatal(err)
	}
	cleared, _ := users.GetByID(ctx, u.ID)
	if cleared.OIDCSubject != nil {
		t.Fatalf("ikatan harus dikosongkan saat email berubah, masih %q", *cleared.OIDCSubject)
	}
	if ok, err := users.BindOIDCSubject(ctx, u.ID, "https://accounts.google.com|baru"); err != nil || !ok {
		t.Fatalf("setelah dikosongkan harus bisa diikat ulang: ok=%v err=%v", ok, err)
	}
}
