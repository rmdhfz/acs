package auth

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"acs/internal/domain"
	"acs/internal/usecase/notify"
	"acs/pkg/mailer"
	"acs/pkg/passwordhash"
)

// ---- fakes ----

type fakeResetRepo struct {
	mu     sync.Mutex
	tokens []*fakeResetToken
}

type fakeResetToken struct {
	userID  uint64
	hash    string
	expires time.Time
	used    bool
	created time.Time
}

func (f *fakeResetRepo) Create(_ context.Context, userID uint64, hash string, exp time.Time, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tokens = append(f.tokens, &fakeResetToken{userID: userID, hash: hash, expires: exp, created: time.Now()})
	return nil
}
func (f *fakeResetRepo) Consume(_ context.Context, hash string, now time.Time) (uint64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, t := range f.tokens {
		if t.hash == hash && !t.used && t.expires.After(now) {
			t.used = true
			return t.userID, nil
		}
	}
	return 0, domain.ErrNotFound
}
func (f *fakeResetRepo) Peek(_ context.Context, hash string, now time.Time) (uint64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, t := range f.tokens {
		if t.hash == hash && !t.used && t.expires.After(now) {
			return t.userID, nil
		}
	}
	return 0, domain.ErrNotFound
}
func (f *fakeResetRepo) CountRecentByUser(_ context.Context, userID uint64, since time.Time) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, t := range f.tokens {
		if t.userID == userID && !t.created.Before(since) {
			n++
		}
	}
	return n, nil
}
func (f *fakeResetRepo) InvalidateAllForUser(_ context.Context, userID uint64, _ time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, t := range f.tokens {
		if t.userID == userID {
			t.used = true
		}
	}
	return nil
}
func (f *fakeResetRepo) DeleteExpired(context.Context, time.Time) (int64, error) { return 0, nil }

type captureNotifier struct {
	mu   sync.Mutex
	msgs []mailer.Message
}

func (c *captureNotifier) Notify(m mailer.Message) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.msgs = append(c.msgs, m)
}
func (c *captureNotifier) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.msgs)
}
func (c *captureNotifier) last() mailer.Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.msgs[len(c.msgs)-1]
}

type resetFixture struct {
	svc      *Service
	users    *fakeUserRepoAuth
	user     *domain.User
	resets   *fakeResetRepo
	notifier *captureNotifier
}

func newResetFixture(t *testing.T) *resetFixture {
	t.Helper()
	user := &domain.User{ID: 1, Username: "hafiz", Email: "Hafiz@Example.test", FullName: "Hafiz", PasswordHash: hashFor(t, "password-lama-ok"), IsActive: true}
	users := &fakeUserRepoAuth{byID: map[uint64]*domain.User{1: user}, byUsername: map[string]*domain.User{"hafiz": user}}
	resets, notifier := &fakeResetRepo{}, &captureNotifier{}
	svc := NewService(users, &fakeTokenRepoAuth{}, &fakeTenantRepoAuthTest{}, &fakeActivityRepoAuth{}, []byte("test-secret-at-least-32-bytes!!"), time.Hour, "", "", "", "").
		WithPasswordReset(resets, notifier, notify.NewBuilder("ACS Console", "https://acs.example.test"), 30*time.Minute)
	return &resetFixture{svc: svc, users: users, user: user, resets: resets, notifier: notifier}
}

var tokenRe = regexp.MustCompile(`#token=([A-Za-z0-9_-]+)`)

func tokenFromMail(t *testing.T, m mailer.Message) string {
	t.Helper()
	g := tokenRe.FindStringSubmatch(m.Text)
	if g == nil {
		t.Fatalf("email tidak memuat tautan reset: %s", m.Text)
	}
	return g[1]
}

// ---- login ----

func TestLogin_ByEmailCaseInsensitiveAndUsername(t *testing.T) {
	fx := newResetFixture(t)
	for _, id := range []string{"hafiz@example.test", "  HAFIZ@EXAMPLE.TEST ", "hafiz"} {
		if _, tok, err := fx.svc.Login(context.Background(), id, "password-lama-ok"); err != nil || tok == "" {
			t.Fatalf("login dgn %q gagal: %v", id, err)
		}
	}
	if _, _, err := fx.svc.Login(context.Background(), "tidak-ada@example.test", "x"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("email tak dikenal: %v", err)
	}
}

func TestLogin_UpgradesLegacyBcryptToArgon2id(t *testing.T) {
	fx := newResetFixture(t)
	if !strings.HasPrefix(fx.user.PasswordHash, "$2") {
		t.Fatal("prasyarat: hash awal harus bcrypt")
	}
	if _, _, err := fx.svc.Login(context.Background(), "hafiz@example.test", "password-lama-ok"); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(fx.user.PasswordHash, "$argon2id$") {
		t.Fatalf("hash harus di-upgrade ke Argon2id, dapat %q", fx.user.PasswordHash[:8])
	}
	// Login berikutnya tetap berhasil & tidak mengganti hash lagi.
	before := fx.user.PasswordHash
	if _, _, err := fx.svc.Login(context.Background(), "hafiz@example.test", "password-lama-ok"); err != nil {
		t.Fatal(err)
	}
	if fx.user.PasswordHash != before {
		t.Fatal("hash Argon2id yang sudah mutakhir tidak boleh di-rehash")
	}
}

func TestLogin_WrongPasswordDoesNotUpgradeAndLockSendsEmailOnce(t *testing.T) {
	fx := newResetFixture(t)
	orig := fx.user.PasswordHash
	for i := 0; i < maxFailedLoginAttempts; i++ {
		if _, _, err := fx.svc.Login(context.Background(), "hafiz@example.test", "salah"); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatal(err)
		}
	}
	if fx.user.PasswordHash != orig {
		t.Fatal("password salah tidak boleh mengubah hash")
	}
	if fx.notifier.count() != 1 || !strings.Contains(fx.notifier.last().Subject, "dikunci") {
		t.Fatalf("harus tepat 1 email 'akun dikunci', dapat %d", fx.notifier.count())
	}
	// Selama terkunci, percobaan tambahan tidak mengirim email lagi.
	_, _, _ = fx.svc.Login(context.Background(), "hafiz@example.test", "salah")
	if fx.notifier.count() != 1 {
		t.Fatalf("tidak boleh spam email saat sudah terkunci, dapat %d", fx.notifier.count())
	}
}

// ---- forgot / reset ----

func TestRequestPasswordReset_UnknownOrInactiveSendsNothing(t *testing.T) {
	fx := newResetFixture(t)
	ctx := context.Background()
	fx.svc.RequestPasswordReset(ctx, "tidak-ada@example.test", "1.2.3.4")
	fx.svc.RequestPasswordReset(ctx, "", "")
	fx.svc.RequestPasswordReset(ctx, "bukan-email", "")
	fx.user.IsActive = false
	fx.svc.RequestPasswordReset(ctx, "hafiz@example.test", "")
	if fx.notifier.count() != 0 || len(fx.resets.tokens) != 0 {
		t.Fatalf("tidak boleh ada email/token: mails=%d tokens=%d", fx.notifier.count(), len(fx.resets.tokens))
	}
}

func TestResetPassword_FullFlow(t *testing.T) {
	fx := newResetFixture(t)
	ctx := context.Background()
	fx.user.FailedLoginAttempts = 4
	locked := time.Now().Add(time.Hour)
	fx.user.LockedUntil = &locked

	fx.svc.RequestPasswordReset(ctx, "HAFIZ@example.test", "1.2.3.4")
	if fx.notifier.count() != 1 {
		t.Fatalf("harus 1 email reset, dapat %d", fx.notifier.count())
	}
	m := fx.notifier.last()
	if m.To[0] != "Hafiz@Example.test" || !strings.Contains(m.HTML, "reset-password#token=") {
		t.Fatalf("email salah: to=%v", m.To)
	}
	token := tokenFromMail(t, m)
	// Token mentah TIDAK disimpan — hanya hash-nya.
	if fx.resets.tokens[0].hash == token || len(fx.resets.tokens[0].hash) != 64 {
		t.Fatal("repo harus menyimpan SHA-256 hex, bukan token mentah")
	}

	// Password terlalu pendek: ditolak TANPA menghanguskan token.
	if err := fx.svc.ResetPassword(ctx, token, "pendek"); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("password pendek: %v", err)
	}
	if err := fx.svc.ResetPassword(ctx, token, "password-baru-banget"); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if ok, _, _ := passwordhash.Verify("password-baru-banget", fx.user.PasswordHash); !ok {
		t.Fatal("password tidak berubah")
	}
	if !strings.HasPrefix(fx.user.PasswordHash, "$argon2id$") {
		t.Fatal("hash baru harus Argon2id")
	}
	if fx.user.LockedUntil != nil || fx.user.FailedLoginAttempts != 0 {
		t.Fatal("lockout harus dibersihkan setelah reset")
	}
	if fx.notifier.count() != 2 || !strings.Contains(fx.notifier.last().Subject, "diganti") {
		t.Fatal("harus ada email konfirmasi 'password diganti'")
	}
	// Sekali pakai.
	if err := fx.svc.ResetPassword(ctx, token, "password-baru-lagi"); !errors.Is(err, ErrInvalidResetToken) {
		t.Fatalf("token kedua kali harus ditolak: %v", err)
	}
	// Bisa login dengan password baru.
	if _, _, err := fx.svc.Login(ctx, "hafiz@example.test", "password-baru-banget"); err != nil {
		t.Fatal(err)
	}
}

func TestResetPassword_InvalidAndExpiredTokens(t *testing.T) {
	fx := newResetFixture(t)
	ctx := context.Background()
	for _, tok := range []string{"", "ngawur", strings.Repeat("a", 200)} {
		if err := fx.svc.ResetPassword(ctx, tok, "password-baru-banget"); !errors.Is(err, ErrInvalidResetToken) {
			t.Errorf("token %q: %v", tok, err)
		}
	}
	fx.svc.RequestPasswordReset(ctx, "hafiz@example.test", "")
	token := tokenFromMail(t, fx.notifier.last())
	fx.resets.tokens[0].expires = time.Now().Add(-time.Minute)
	if err := fx.svc.ResetPassword(ctx, token, "password-baru-banget"); !errors.Is(err, ErrInvalidResetToken) {
		t.Fatalf("token kedaluwarsa: %v", err)
	}
}

func TestRequestPasswordReset_PerAccountRateLimit(t *testing.T) {
	fx := newResetFixture(t)
	for i := 0; i < maxResetRequestsPerHour+3; i++ {
		fx.svc.RequestPasswordReset(context.Background(), "hafiz@example.test", "")
	}
	if fx.notifier.count() != maxResetRequestsPerHour {
		t.Fatalf("email reset = %d, mau tepat %d (batas per jam)", fx.notifier.count(), maxResetRequestsPerHour)
	}
}

func TestChangeOwnPassword_InvalidatesResetTokensAndNotifies(t *testing.T) {
	fx := newResetFixture(t)
	ctx := context.Background()
	fx.svc.RequestPasswordReset(ctx, "hafiz@example.test", "")
	token := tokenFromMail(t, fx.notifier.last())

	if _, err := fx.svc.ChangeOwnPassword(ctx, domain.Actor{UserID: 1}, "password-lama-ok", "password-ganti-sendiri"); err != nil {
		t.Fatal(err)
	}
	if fx.notifier.count() != 2 || !strings.Contains(fx.notifier.last().Subject, "diganti") {
		t.Fatal("harus ada email 'password diganti'")
	}
	if err := fx.svc.ResetPassword(ctx, token, "password-baru-banget"); !errors.Is(err, ErrInvalidResetToken) {
		t.Fatalf("token lama harus mati setelah ganti password: %v", err)
	}
}

func TestPasswordResetDisabledWithoutConfig(t *testing.T) {
	users := &fakeUserRepoAuth{byID: map[uint64]*domain.User{}}
	svc := NewService(users, &fakeTokenRepoAuth{}, &fakeTenantRepoAuthTest{}, &fakeActivityRepoAuth{}, []byte("test-secret-at-least-32-bytes!!"), time.Hour, "", "", "", "")
	if svc.PasswordResetEnabled() {
		t.Fatal("tanpa WithPasswordReset harus nonaktif")
	}
	if err := svc.ResetPassword(context.Background(), "x", "password-baru-banget"); !errors.Is(err, ErrPasswordResetDisabled) {
		t.Fatalf("got %v", err)
	}
	svc.RequestPasswordReset(context.Background(), "a@b.test", "") // tidak boleh panic
}

// ---- sesi: JWT dicabut setelah password berubah / akun nonaktif / dihapus ----

func TestResolveActor_RevokesSessionsAfterPasswordChange(t *testing.T) {
	fx := newResetFixture(t)
	ctx := context.Background()
	_, oldTok, err := fx.svc.Login(ctx, "hafiz@example.test", "password-lama-ok")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fx.svc.ResolveActor(ctx, oldTok); err != nil {
		t.Fatalf("token baru harus sah: %v", err)
	}

	// Password diganti SETELAH token terbit (iat < password_changed_at) -> ditolak.
	later := time.Now().Add(3 * time.Second)
	fx.user.PasswordChangedAt = &later
	if _, err := fx.svc.ResolveActor(ctx, oldTok); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("token lama harus dicabut setelah password diganti: %v", err)
	}

	// Akun nonaktif / terhapus -> ditolak walau token belum kedaluwarsa.
	fx.user.PasswordChangedAt = nil
	fx.user.IsActive = false
	if _, err := fx.svc.ResolveActor(ctx, oldTok); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("akun nonaktif: %v", err)
	}
	fx.user.IsActive = true
	delete(fx.users.byID, 1)
	if _, err := fx.svc.ResolveActor(ctx, oldTok); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("akun terhapus: %v", err)
	}
}

func TestChangeOwnPassword_ReturnsFreshValidToken(t *testing.T) {
	fx := newResetFixture(t)
	ctx := context.Background()
	_, oldTok, _ := fx.svc.Login(ctx, "hafiz@example.test", "password-lama-ok")
	time.Sleep(1100 * time.Millisecond) // pastikan iat token lama < password_changed_at (presisi detik)

	newTok, err := fx.svc.ChangeOwnPassword(ctx, domain.Actor{UserID: 1}, "password-lama-ok", "password-ganti-sendiri")
	if err != nil || newTok == "" {
		t.Fatalf("ChangeOwnPassword: tok=%q err=%v", newTok, err)
	}
	if _, err := fx.svc.ResolveActor(ctx, newTok); err != nil {
		t.Fatalf("token baru dari ChangeOwnPassword harus sah (pemanggil tetap login): %v", err)
	}
	if _, err := fx.svc.ResolveActor(ctx, oldTok); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("token lama harus mati: %v", err)
	}
}

func TestResetPassword_BogusTokenNeverReachesHashing(t *testing.T) {
	fx := newResetFixture(t)
	before := fx.user.PasswordHash
	for i := 0; i < 3; i++ {
		if err := fx.svc.ResetPassword(context.Background(), "token-ngawur-tapi-panjang-xyz", "password-baru-banget"); !errors.Is(err, ErrInvalidResetToken) {
			t.Fatal(err)
		}
	}
	if fx.user.PasswordHash != before {
		t.Fatal("password tidak boleh berubah")
	}
}

func TestResetPassword_InactiveUserKeepsTokenUnspent(t *testing.T) {
	fx := newResetFixture(t)
	ctx := context.Background()
	fx.svc.RequestPasswordReset(ctx, "hafiz@example.test", "")
	token := tokenFromMail(t, fx.notifier.last())
	fx.user.IsActive = false
	if err := fx.svc.ResetPassword(ctx, token, "password-baru-banget"); !errors.Is(err, ErrInvalidResetToken) {
		t.Fatalf("akun nonaktif: %v", err)
	}
	if fx.resets.tokens[0].used {
		t.Fatal("token tidak boleh dihanguskan bila akun nonaktif (dicek sebelum Consume)")
	}
}

func TestRehashPassword_IsCompareAndSwap(t *testing.T) {
	fx := newResetFixture(t)
	old := fx.user.PasswordHash
	if ok, _ := fx.users.RehashPassword(context.Background(), 1, "hash-basi", "hash-baru"); ok {
		t.Fatal("CAS dgn hash basi harus gagal")
	}
	if fx.user.PasswordHash != old {
		t.Fatal("hash tidak boleh berubah")
	}
	if ok, _ := fx.users.RehashPassword(context.Background(), 1, old, "hash-baru"); !ok || fx.user.PasswordHash != "hash-baru" {
		t.Fatal("CAS dgn hash cocok harus berhasil")
	}
}
