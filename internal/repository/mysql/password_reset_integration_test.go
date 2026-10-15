package mysql

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"acs/internal/domain"
)

// Test integrasi ke MariaDB NYATA (migrations/0023 harus sudah diterapkan).
// Dilewati bila ACS_TEST_DSN tidak diisi, mis.:
//
//	ACS_TEST_DSN='acs:<pass>@tcp(127.0.0.1:13306)/acs?parseTime=true&loc=Local&charset=utf8mb4' go test ./internal/repository/mysql -run Integration
func TestPasswordResetRepository_Integration(t *testing.T) {
	dsn := os.Getenv("ACS_TEST_DSN")
	if dsn == "" {
		t.Skip("ACS_TEST_DSN tidak diisi — test integrasi MariaDB dilewati")
	}
	db, err := Connect(dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	// t.Cleanup (bukan defer): dijalankan LIFO, jadi cleanup user di bawah masih
	// punya koneksi saat berjalan.
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()

	users := NewUserRepository(db)
	repo := NewPasswordResetRepository(db)

	suffix := uuid.NewString()[:8]
	u := &domain.User{
		UserUUID: uuid.NewString(), Username: "it-prt-" + suffix, Email: "It-Prt-" + suffix + "@example.test",
		PasswordHash: "x", FullName: "IT", IsActive: true,
	}
	if err := users.Create(ctx, u); err != nil {
		t.Fatalf("create user: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM users WHERE id = ?`, u.ID) }) // cascade ke token

	// GetByEmail: tak-peka-huruf-besar, hanya yang belum dihapus.
	got, err := users.GetByEmail(ctx, "it-prt-"+suffix+"@EXAMPLE.test")
	if err != nil || got.ID != u.ID {
		t.Fatalf("GetByEmail case-insensitive: %v (id=%v)", err, got)
	}
	if _, err := users.GetByEmail(ctx, "tidak-ada-"+suffix+"@example.test"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("email tak ada harus ErrNotFound, dapat %v", err)
	}

	now := time.Now()
	if err := repo.Create(ctx, u.ID, fmt.Sprintf("%064x", 1), now.Add(30*time.Minute), "10.0.0.1"); err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(ctx, u.ID, fmt.Sprintf("%064x", 2), now.Add(-time.Minute), ""); err != nil {
		t.Fatal(err)
	}
	if n, _ := repo.CountRecentByUser(ctx, u.ID, now.Add(-time.Hour)); n != 2 {
		t.Fatalf("CountRecentByUser = %d, mau 2", n)
	}

	// Kedaluwarsa → ditolak.
	if _, err := repo.Consume(ctx, fmt.Sprintf("%064x", 2), now); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("token kedaluwarsa: %v", err)
	}

	// Konsumsi PARALEL: tepat satu pemenang.
	var wins int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if id, err := repo.Consume(ctx, fmt.Sprintf("%064x", 1), time.Now()); err == nil && id == u.ID {
				atomic.AddInt32(&wins, 1)
			}
		}()
	}
	wg.Wait()
	if wins != 1 {
		t.Fatalf("Consume paralel: %d pemenang, mau tepat 1 (sekali pakai)", wins)
	}

	// InvalidateAllForUser mematikan token yang belum dipakai.
	if err := repo.Create(ctx, u.ID, fmt.Sprintf("%064x", 3), now.Add(30*time.Minute), ""); err != nil {
		t.Fatal(err)
	}
	if err := repo.InvalidateAllForUser(ctx, u.ID, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Consume(ctx, fmt.Sprintf("%064x", 3), time.Now()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("token yang di-invalidate harus ditolak: %v", err)
	}

	// Peek: baca-saja (tidak menghanguskan) — token valid tetap bisa di-Consume.
	if err := repo.Create(ctx, u.ID, fmt.Sprintf("%064x", 4), now.Add(30*time.Minute), ""); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if id, err := repo.Peek(ctx, fmt.Sprintf("%064x", 4), time.Now()); err != nil || id != u.ID {
			t.Fatalf("Peek #%d: id=%d err=%v", i, id, err)
		}
	}
	if _, err := repo.Peek(ctx, fmt.Sprintf("%064x", 99), time.Now()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Peek token tak ada: %v", err)
	}
	if _, err := repo.Consume(ctx, fmt.Sprintf("%064x", 4), time.Now()); err != nil {
		t.Fatalf("Consume setelah Peek: %v", err)
	}
	if _, err := repo.Peek(ctx, fmt.Sprintf("%064x", 4), time.Now()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Peek setelah Consume harus ErrNotFound: %v", err)
	}

	// ---- sesi: AuthState, password_changed_at, CAS rehash, lock sekali ----
	st, err := users.AuthState(ctx, u.ID)
	if err != nil || !st.IsActive || st.PasswordChangedAt != nil {
		t.Fatalf("AuthState awal: %+v err=%v", st, err)
	}
	if err := users.UpdatePassword(ctx, u.ID, "hash-1", nil); err != nil {
		t.Fatal(err)
	}
	st, _ = users.AuthState(ctx, u.ID)
	if st.PasswordChangedAt == nil || time.Since(*st.PasswordChangedAt) > time.Minute {
		t.Fatalf("UpdatePassword harus mengisi password_changed_at: %+v", st)
	}
	if ok, err := users.RehashPassword(ctx, u.ID, "hash-basi", "hash-2"); err != nil || ok {
		t.Fatalf("CAS dgn hash basi: ok=%v err=%v", ok, err)
	}
	if ok, err := users.RehashPassword(ctx, u.ID, "hash-1", "hash-2"); err != nil || !ok {
		t.Fatalf("CAS dgn hash cocok: ok=%v err=%v", ok, err)
	}
	chk, _ := users.GetByID(ctx, u.ID)
	if chk.PasswordHash != "hash-2" || chk.PasswordChangedAt == nil {
		t.Fatalf("setelah rehash: hash=%q changedAt=%v (rehash tidak boleh mengubah password_changed_at)", chk.PasswordHash, chk.PasswordChangedAt)
	}
	if _, err := users.AuthState(ctx, 999999999); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("AuthState user tak ada: %v", err)
	}
	// RecordFailedLogin: locked=true tepat saat melewati ambang.
	lockUntil := time.Now().Add(15 * time.Minute)
	for i := 1; i <= 3; i++ {
		locked, err := users.RecordFailedLogin(ctx, u.ID, 3, lockUntil)
		if err != nil {
			t.Fatal(err)
		}
		if want := i == 3; locked != want {
			t.Fatalf("percobaan gagal ke-%d: locked=%v, mau %v", i, locked, want)
		}
	}

	// Retensi: hapus fisik yang kedaluwarsa.
	if n, err := repo.DeleteExpired(ctx, time.Now()); err != nil || n < 1 {
		t.Fatalf("DeleteExpired: n=%d err=%v", n, err)
	}
}
