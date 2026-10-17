package mysql

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"

	"acs/internal/domain"
)

// Integrasi MariaDB NYATA untuk MFA (migrations/0027 harus sudah diterapkan). Dilewati tanpa ACS_TEST_DSN.
func TestMFARepository_Integration(t *testing.T) {
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
	repo := NewMFARepository(db)

	suffix := uuid.NewString()[:8]
	u := &domain.User{
		UserUUID: uuid.NewString(), Username: "it-mfa-" + suffix, Email: "it-mfa-" + suffix + "@example.test",
		PasswordHash: "x", FullName: "IT MFA", IsActive: true,
	}
	if err := users.Create(ctx, u); err != nil {
		t.Fatalf("create user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM user_recovery_codes WHERE user_id = ?`, u.ID)
		_, _ = db.Exec(`DELETE FROM users WHERE id = ?`, u.ID)
	})

	// user baru: MFA mati, SELECT * tetap terbaca (kolom baru ter-scan)
	got, err := users.GetByID(ctx, u.ID)
	if err != nil || got.TOTPEnabled || len(got.TOTPSecretEnc) != 0 || got.TOTPLastStep != nil {
		t.Fatalf("state awal: %+v err=%v", got, err)
	}

	// Enable tanpa secret -> konflik
	if err := repo.Enable(ctx, u.ID, 100, []string{h(1)}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("enable tanpa setup harus ErrConflict, dapat %v", err)
	}
	if err := repo.SaveSecret(ctx, u.ID, []byte("ciphertext")); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveSecret(ctx, 99999999, []byte("x")); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("SaveSecret user tak ada: %v", err)
	}
	if err := repo.Enable(ctx, u.ID, 100, []string{h(1), h(2)}); err != nil {
		t.Fatal(err)
	}
	got, _ = users.GetByID(ctx, u.ID)
	if !got.TOTPEnabled || string(got.TOTPSecretEnc) != "ciphertext" || got.TOTPLastStep == nil || *got.TOTPLastStep != 100 {
		t.Fatalf("setelah enable: %+v", got)
	}
	// Enable kedua kali (sudah aktif) -> konflik, kode lama tak tertimpa
	if err := repo.Enable(ctx, u.ID, 101, []string{h(9)}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("enable ganda: %v", err)
	}
	if n, _ := repo.CountRecoveryCodes(ctx, u.ID); n != 2 {
		t.Fatalf("kode pemulihan = %d, mau 2", n)
	}

	// anti-replay: langkah harus naik; yang sama/lebih lama ditolak
	if ok, _ := repo.AdvanceStep(ctx, u.ID, 100); ok {
		t.Fatal("langkah sama harus ditolak")
	}
	if ok, _ := repo.AdvanceStep(ctx, u.ID, 99); ok {
		t.Fatal("langkah lebih lama harus ditolak")
	}
	if ok, _ := repo.AdvanceStep(ctx, u.ID, 101); !ok {
		t.Fatal("langkah lebih baru harus diterima")
	}
	// paralel: 20 goroutine pada langkah yang sama -> TEPAT satu menang
	var wins int32
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if ok, err := repo.AdvanceStep(ctx, u.ID, 200); err == nil && ok {
				atomic.AddInt32(&wins, 1)
			}
		}()
	}
	wg.Wait()
	if wins != 1 {
		t.Fatalf("CAS langkah paralel: %d pemenang, mau 1", wins)
	}

	// kode pemulihan sekali pakai, juga di bawah paralelisme
	wins = 0
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if ok, err := repo.UseRecoveryCode(ctx, u.ID, h(1)); err == nil && ok {
				atomic.AddInt32(&wins, 1)
			}
		}()
	}
	wg.Wait()
	if wins != 1 {
		t.Fatalf("kode pemulihan paralel: %d pemenang, mau 1", wins)
	}
	if ok, _ := repo.UseRecoveryCode(ctx, u.ID, h(77)); ok {
		t.Fatal("kode tak dikenal harus ditolak")
	}
	if n, _ := repo.CountRecoveryCodes(ctx, u.ID); n != 1 {
		t.Fatalf("sisa kode = %d, mau 1", n)
	}

	// Disable membersihkan semuanya
	if err := repo.Disable(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	got, _ = users.GetByID(ctx, u.ID)
	if got.TOTPEnabled || len(got.TOTPSecretEnc) != 0 || got.TOTPLastStep != nil {
		t.Fatalf("setelah disable: %+v", got)
	}
	if n, _ := repo.CountRecoveryCodes(ctx, u.ID); n != 0 {
		t.Fatalf("kode tersisa setelah disable: %d", n)
	}
}

// h — hash 64-hex unik per n; garam acak mencegah bentrok UNIQUE antar-run.
var hashSalt = uuid.NewString()

func h(n int) string {
	sum := sha256.Sum256([]byte(hashSalt + string(rune(n+65))))
	return hex.EncodeToString(sum[:])
}
