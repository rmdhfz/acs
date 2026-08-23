package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"acs/internal/domain"
)

// ---- fakes (pola sama seperti internal/usecase/task/service_test.go) ----

type fakeUserRepoAuth struct {
	byID          map[uint64]*domain.User
	byUsername    map[string]*domain.User
	recordedFails int
	lastLockUntil time.Time
	resetCalled   bool
	updatedHash   string
}

func (f *fakeUserRepoAuth) Create(context.Context, *domain.User) error { return nil }
func (f *fakeUserRepoAuth) GetByID(_ context.Context, id uint64) (*domain.User, error) {
	if u, ok := f.byID[id]; ok {
		return u, nil
	}
	return nil, domain.ErrNotFound
}
func (f *fakeUserRepoAuth) GetByUsername(_ context.Context, username string) (*domain.User, error) {
	if u, ok := f.byUsername[username]; ok {
		return u, nil
	}
	return nil, domain.ErrNotFound
}
func (f *fakeUserRepoAuth) List(context.Context, *uint64, domain.Pagination) ([]domain.User, int, error) {
	return nil, 0, nil
}
func (f *fakeUserRepoAuth) Update(context.Context, *domain.User) error { return nil }
func (f *fakeUserRepoAuth) UpdatePassword(_ context.Context, id uint64, hash string, _ *uint64) error {
	f.updatedHash = hash
	if u, ok := f.byID[id]; ok {
		u.PasswordHash = hash
	}
	return nil
}
func (f *fakeUserRepoAuth) SoftDelete(context.Context, uint64, uint64) error        { return nil }
func (f *fakeUserRepoAuth) TouchLastLogin(context.Context, uint64, time.Time) error { return nil }
func (f *fakeUserRepoAuth) RolesByUserID(context.Context, uint64) ([]string, error) {
	return nil, nil
}
func (f *fakeUserRepoAuth) AssignRole(context.Context, uint64, uint64, *uint64) error { return nil }
func (f *fakeUserRepoAuth) RevokeRole(context.Context, uint64, uint64) error          { return nil }
func (f *fakeUserRepoAuth) RecordFailedLogin(_ context.Context, id uint64, maxAttempts uint32, lockUntil time.Time) error {
	f.recordedFails++
	u, ok := f.byID[id]
	if !ok {
		return domain.ErrNotFound
	}
	u.FailedLoginAttempts++
	if u.FailedLoginAttempts >= maxAttempts {
		u.LockedUntil = &lockUntil
		f.lastLockUntil = lockUntil
	}
	return nil
}
func (f *fakeUserRepoAuth) ResetLoginLockout(_ context.Context, id uint64) error {
	f.resetCalled = true
	if u, ok := f.byID[id]; ok {
		u.FailedLoginAttempts = 0
		u.LockedUntil = nil
	}
	return nil
}

type fakeTokenRepoAuth struct{}

func (f *fakeTokenRepoAuth) Create(context.Context, *domain.APIToken) error { return nil }
func (f *fakeTokenRepoAuth) GetByHash(context.Context, string) (*domain.APIToken, error) {
	return nil, domain.ErrNotFound
}
func (f *fakeTokenRepoAuth) Revoke(context.Context, uint64) error { return nil }
func (f *fakeTokenRepoAuth) ListByUser(context.Context, uint64) ([]domain.APIToken, error) {
	return nil, nil
}

type fakeTenantRepoAuthTest struct {
	byID map[uint64]*domain.Tenant
	err  error // dikembalikan GetByID utk SEMUA id, kalau di-set (simulasi error sistem)
}

func (f *fakeTenantRepoAuthTest) Create(context.Context, *domain.Tenant) error { return nil }
func (f *fakeTenantRepoAuthTest) GetByID(_ context.Context, id uint64) (*domain.Tenant, error) {
	if f.err != nil {
		return nil, f.err
	}
	if t, ok := f.byID[id]; ok {
		return t, nil
	}
	return nil, domain.ErrNotFound
}
func (f *fakeTenantRepoAuthTest) GetByUUID(context.Context, string) (*domain.Tenant, error) {
	return nil, domain.ErrNotFound
}
func (f *fakeTenantRepoAuthTest) GetByCWMPInformUsername(context.Context, string) (*domain.Tenant, error) {
	return nil, domain.ErrNotFound
}
func (f *fakeTenantRepoAuthTest) List(context.Context, domain.Pagination) ([]domain.Tenant, int, error) {
	return nil, 0, nil
}
func (f *fakeTenantRepoAuthTest) Update(context.Context, *domain.Tenant) error { return nil }
func (f *fakeTenantRepoAuthTest) SetCWMPInformCredentials(context.Context, uint64, string, []byte, *uint64) error {
	return nil
}
func (f *fakeTenantRepoAuthTest) UpdateBranding(context.Context, uint64, *string, *string, *string, *uint64) error {
	return nil
}
func (f *fakeTenantRepoAuthTest) SetTaskQuota(context.Context, uint64, *uint32, *uint64) error {
	return nil
}
func (f *fakeTenantRepoAuthTest) SoftDelete(context.Context, uint64, uint64) error { return nil }

type fakeActivityRepoAuth struct{}

func (f *fakeActivityRepoAuth) Record(context.Context, *domain.ActivityLog) error { return nil }
func (f *fakeActivityRepoAuth) ListByEntity(context.Context, string, uint64, domain.Pagination) ([]domain.ActivityLog, int, error) {
	return nil, 0, nil
}

func hashFor(t *testing.T, password string) string {
	t.Helper()
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt.GenerateFromPassword: %v", err)
	}
	return string(h)
}

// ---- tests ----

func TestLogin_LockoutThreshold(t *testing.T) {
	hash := hashFor(t, "correct-password")
	user := &domain.User{ID: 1, Username: "alice", PasswordHash: hash, IsActive: true}
	users := &fakeUserRepoAuth{
		byID:       map[uint64]*domain.User{1: user},
		byUsername: map[string]*domain.User{"alice": user},
	}
	svc := NewService(users, &fakeTokenRepoAuth{}, &fakeTenantRepoAuthTest{}, &fakeActivityRepoAuth{}, []byte("test-secret-at-least-32-bytes!!"), time.Hour)

	// 5x gagal berturut-turut dengan password salah -> akun terkunci.
	for i := 0; i < maxFailedLoginAttempts; i++ {
		_, _, err := svc.Login(context.Background(), "alice", "wrong-password")
		if !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("percobaan gagal ke-%d: error = %v, want ErrInvalidCredentials", i+1, err)
		}
	}
	if user.LockedUntil == nil {
		t.Fatal("akun belum terkunci setelah mencapai threshold")
	}

	// Percobaan ke-6 dengan password BENAR tetap ditolak selama masa lockout.
	_, _, err := svc.Login(context.Background(), "alice", "correct-password")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("login dgn password benar saat locked: error = %v, want ErrInvalidCredentials", err)
	}

	// Lockout habis (simulasikan waktu lalu) -> login normal berhasil lagi.
	past := time.Now().Add(-time.Minute)
	user.LockedUntil = &past
	_, token, err := svc.Login(context.Background(), "alice", "correct-password")
	if err != nil {
		t.Fatalf("login setelah lockout habis: error = %v, want nil", err)
	}
	if token == "" {
		t.Fatal("token kosong setelah login berhasil")
	}
	if !users.resetCalled {
		t.Fatal("ResetLoginLockout tidak dipanggil setelah login berhasil")
	}
}

func TestLogin_UnknownUsernameSameErrorAsWrongPassword(t *testing.T) {
	users := &fakeUserRepoAuth{byID: map[uint64]*domain.User{}, byUsername: map[string]*domain.User{}}
	svc := NewService(users, &fakeTokenRepoAuth{}, &fakeTenantRepoAuthTest{}, &fakeActivityRepoAuth{}, []byte("test-secret-at-least-32-bytes!!"), time.Hour)

	_, _, err := svc.Login(context.Background(), "tidak-ada", "apa-saja")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("username tak dikenal: error = %v, want ErrInvalidCredentials (jangan reveal validitas username)", err)
	}
}

func TestChangeOwnPassword_RequiresCorrectCurrentPassword(t *testing.T) {
	hash := hashFor(t, "old-password")
	user := &domain.User{ID: 1, Username: "alice", PasswordHash: hash, IsActive: true}
	users := &fakeUserRepoAuth{byID: map[uint64]*domain.User{1: user}}
	svc := NewService(users, &fakeTokenRepoAuth{}, &fakeTenantRepoAuthTest{}, &fakeActivityRepoAuth{}, []byte("test-secret-at-least-32-bytes!!"), time.Hour)

	actor := domain.Actor{UserID: 1}

	if err := svc.ChangeOwnPassword(context.Background(), actor, "salah-password-lama", "password-baru-yang-panjang"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("current_password salah: error = %v, want ErrInvalidCredentials", err)
	}
	if users.updatedHash != "" {
		t.Fatal("password tidak seharusnya berubah saat current_password salah")
	}

	if err := svc.ChangeOwnPassword(context.Background(), actor, "old-password", "password-baru-yang-panjang"); err != nil {
		t.Fatalf("current_password benar: error = %v, want nil", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte("password-baru-yang-panjang")); err != nil {
		t.Fatalf("password_hash tidak ter-update ke password baru: %v", err)
	}
}

func TestChangeOwnPassword_RejectsShortNewPassword(t *testing.T) {
	hash := hashFor(t, "old-password")
	user := &domain.User{ID: 1, PasswordHash: hash, IsActive: true}
	users := &fakeUserRepoAuth{byID: map[uint64]*domain.User{1: user}}
	svc := NewService(users, &fakeTokenRepoAuth{}, &fakeTenantRepoAuthTest{}, &fakeActivityRepoAuth{}, []byte("test-secret-at-least-32-bytes!!"), time.Hour)

	err := svc.ChangeOwnPassword(context.Background(), domain.Actor{UserID: 1}, "old-password", "short")
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("password baru terlalu pendek: error = %v, want ErrInvalidInput", err)
	}
}

func TestChangeOwnPassword_OnlyAffectsActorsOwnAccount(t *testing.T) {
	hashA := hashFor(t, "password-a")
	hashB := hashFor(t, "password-b")
	userA := &domain.User{ID: 1, PasswordHash: hashA, IsActive: true}
	userB := &domain.User{ID: 2, PasswordHash: hashB, IsActive: true}
	users := &fakeUserRepoAuth{byID: map[uint64]*domain.User{1: userA, 2: userB}}
	svc := NewService(users, &fakeTokenRepoAuth{}, &fakeTenantRepoAuthTest{}, &fakeActivityRepoAuth{}, []byte("test-secret-at-least-32-bytes!!"), time.Hour)

	// actor.UserID = 1 memanggil ChangeOwnPassword -- tidak ada cara memasukkan
	// ID user lain, jadi hanya user 1 yang mungkin terpengaruh secara desain
	// (lihat signature ChangeOwnPassword: tidak menerima target ID sama sekali).
	if err := svc.ChangeOwnPassword(context.Background(), domain.Actor{UserID: 1}, "password-a", "password-baru-panjang"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if userB.PasswordHash != hashB {
		t.Fatal("password user LAIN (id=2) ikut berubah -- seharusnya tidak mungkin lewat endpoint ini")
	}
}

func TestCheckTenantActive(t *testing.T) {
	activeTenant := &domain.Tenant{ID: 1, IsActive: true}
	inactiveTenant := &domain.Tenant{ID: 2, IsActive: false}
	tid1, tid2, tid3 := uint64(1), uint64(2), uint64(3)

	t.Run("actor tanpa tenant_id (superadmin) selalu lolos", func(t *testing.T) {
		tenants := &fakeTenantRepoAuthTest{byID: map[uint64]*domain.Tenant{1: activeTenant}}
		svc := NewService(&fakeUserRepoAuth{}, &fakeTokenRepoAuth{}, tenants, &fakeActivityRepoAuth{}, []byte("test-secret-at-least-32-bytes!!"), time.Hour)
		if err := svc.checkTenantActive(context.Background(), nil); err != nil {
			t.Fatalf("error = %v, want nil", err)
		}
	})

	t.Run("tenant aktif -> lolos", func(t *testing.T) {
		tenants := &fakeTenantRepoAuthTest{byID: map[uint64]*domain.Tenant{1: activeTenant}}
		svc := NewService(&fakeUserRepoAuth{}, &fakeTokenRepoAuth{}, tenants, &fakeActivityRepoAuth{}, []byte("test-secret-at-least-32-bytes!!"), time.Hour)
		if err := svc.checkTenantActive(context.Background(), &tid1); err != nil {
			t.Fatalf("error = %v, want nil", err)
		}
	})

	t.Run("tenant nonaktif -> ErrTenantInactive", func(t *testing.T) {
		tenants := &fakeTenantRepoAuthTest{byID: map[uint64]*domain.Tenant{2: inactiveTenant}}
		svc := NewService(&fakeUserRepoAuth{}, &fakeTokenRepoAuth{}, tenants, &fakeActivityRepoAuth{}, []byte("test-secret-at-least-32-bytes!!"), time.Hour)
		if err := svc.checkTenantActive(context.Background(), &tid2); !errors.Is(err, ErrTenantInactive) {
			t.Fatalf("error = %v, want ErrTenantInactive", err)
		}
	})

	t.Run("tenant tidak ditemukan/soft-deleted -> ErrTenantInactive (bukan crash)", func(t *testing.T) {
		tenants := &fakeTenantRepoAuthTest{byID: map[uint64]*domain.Tenant{}}
		svc := NewService(&fakeUserRepoAuth{}, &fakeTokenRepoAuth{}, tenants, &fakeActivityRepoAuth{}, []byte("test-secret-at-least-32-bytes!!"), time.Hour)
		if err := svc.checkTenantActive(context.Background(), &tid3); !errors.Is(err, ErrTenantInactive) {
			t.Fatalf("error = %v, want ErrTenantInactive", err)
		}
	})

	t.Run("error SISTEM (bukan ErrNotFound) -> di-propagate apa adanya, BUKAN disamarkan ErrTenantInactive", func(t *testing.T) {
		systemErr := errors.New("db: connection refused")
		tenants := &fakeTenantRepoAuthTest{err: systemErr}
		svc := NewService(&fakeUserRepoAuth{}, &fakeTokenRepoAuth{}, tenants, &fakeActivityRepoAuth{}, []byte("test-secret-at-least-32-bytes!!"), time.Hour)
		err := svc.checkTenantActive(context.Background(), &tid1)
		if errors.Is(err, ErrTenantInactive) {
			t.Fatal("error sistem seharusnya TIDAK disamarkan jadi ErrTenantInactive (menyulitkan diagnosa insiden nyata)")
		}
		if !errors.Is(err, systemErr) {
			t.Fatalf("error = %v, want propagated systemErr", err)
		}
	})
}
