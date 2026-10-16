package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"acs/internal/domain"
	"acs/pkg/totp"
)

// fakeCipher — "enkripsi" reversibel sederhana; yang diuji di sini alur MFA, bukan AES.
type fakeCipher struct{}

func (fakeCipher) Encrypt(p string) ([]byte, error) { return []byte("enc:" + p), nil }
func (fakeCipher) Decrypt(c []byte) (string, error) {
	return strings.TrimPrefix(string(c), "enc:"), nil
}

// fakeMFARepo meniru semantik repo MySQL (CAS langkah, kode pemulihan sekali pakai) di atas
// objek User yang sama dengan fakeUserRepoAuth.
type fakeMFARepo struct {
	users map[uint64]*domain.User
	codes map[uint64]map[string]bool // userID -> hash -> terpakai
}

func newFakeMFARepo(users map[uint64]*domain.User) *fakeMFARepo {
	return &fakeMFARepo{users: users, codes: map[uint64]map[string]bool{}}
}

func (f *fakeMFARepo) SaveSecret(_ context.Context, id uint64, enc []byte) error {
	u := f.users[id]
	u.TOTPSecretEnc, u.TOTPEnabled, u.TOTPLastStep = enc, false, nil
	return nil
}
func (f *fakeMFARepo) Enable(_ context.Context, id, step uint64, hashes []string) error {
	u := f.users[id]
	if u.TOTPEnabled || len(u.TOTPSecretEnc) == 0 {
		return domain.ErrConflict
	}
	u.TOTPEnabled, u.TOTPLastStep = true, &step
	f.codes[id] = map[string]bool{}
	for _, h := range hashes {
		f.codes[id][h] = false
	}
	return nil
}
func (f *fakeMFARepo) Disable(_ context.Context, id uint64) error {
	u := f.users[id]
	u.TOTPSecretEnc, u.TOTPEnabled, u.TOTPLastStep = nil, false, nil
	delete(f.codes, id)
	return nil
}
func (f *fakeMFARepo) AdvanceStep(_ context.Context, id, step uint64) (bool, error) {
	u := f.users[id]
	if u.TOTPLastStep != nil && *u.TOTPLastStep >= step {
		return false, nil
	}
	u.TOTPLastStep = &step
	return true, nil
}
func (f *fakeMFARepo) UseRecoveryCode(_ context.Context, id uint64, h string) (bool, error) {
	used, ok := f.codes[id][h]
	if !ok || used {
		return false, nil
	}
	f.codes[id][h] = true
	return true, nil
}
func (f *fakeMFARepo) CountRecoveryCodes(_ context.Context, id uint64) (int, error) {
	n := 0
	for _, used := range f.codes[id] {
		if !used {
			n++
		}
	}
	return n, nil
}

const mfaTestPassword = "Sandi-Rahasia-123"

type mfaFixture struct {
	svc    *Service
	users  *fakeUserRepoAuth
	repo   *fakeMFARepo
	user   *domain.User
	secret string
}

// newMFAFixture membuat satu akun yang SUDAH ber-MFA (enrol dilewati; alur enrol diuji terpisah).
func newMFAFixture(t *testing.T) *mfaFixture {
	t.Helper()
	tenant := uint64(1)
	u := &domain.User{ID: 5, UserUUID: "uuid-5", TenantID: &tenant, Username: "ani", Email: "ani@isp.id",
		PasswordHash: hashFor(t, mfaTestPassword), IsActive: true, Roles: []string{domain.RoleAdmin}}
	users := &fakeUserRepoAuth{byID: map[uint64]*domain.User{5: u}, byUsername: map[string]*domain.User{"ani": u}}
	tenants := &fakeTenantRepoAuthTest{byID: map[uint64]*domain.Tenant{1: {ID: 1, IsActive: true}}}
	svc := NewService(users, &fakeTokenRepoAuth{}, tenants, &fakeActivityRepoAuth{},
		[]byte("test-secret-at-least-32-bytes!!"), time.Hour, "", "", "", "")
	repo := newFakeMFARepo(users.byID)
	svc.WithMFA(repo, fakeCipher{}, "ACS")

	secret, err := totp.GenerateSecret()
	if err != nil {
		t.Fatal(err)
	}
	u.TOTPSecretEnc, u.TOTPEnabled = []byte("enc:"+secret), true
	return &mfaFixture{svc: svc, users: users, repo: repo, user: u, secret: secret}
}

func (f *mfaFixture) codeAt(t *testing.T, offset time.Duration) string {
	t.Helper()
	c, err := totp.Code(f.secret, time.Now().Add(offset))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func (f *mfaFixture) loginForMFAToken(t *testing.T) string {
	t.Helper()
	_, tok, err := f.svc.Login(context.Background(), "ani@isp.id", mfaTestPassword)
	if !errors.Is(err, ErrMFARequired) || tok == "" {
		t.Fatalf("login akun ber-MFA harus ErrMFARequired + token tahap-2, dapat err=%v tok=%q", err, tok)
	}
	return tok
}

func TestMFA_TokenTahap2BukanSesi(t *testing.T) {
	f := newMFAFixture(t)
	mt := f.loginForMFAToken(t)
	if _, err := f.svc.ResolveActor(context.Background(), mt); err == nil {
		t.Fatal("token tahap-MFA TIDAK boleh diterima sebagai sesi (ResolveActor)")
	}
	if _, err := f.svc.ParseJWT(mt); err == nil {
		t.Fatal("token tahap-MFA TIDAK boleh diterima sebagai sesi (ParseJWT)")
	}
	// kebalikannya: JWT sesi biasa tak boleh dipakai di tahap-2
	sess, err := f.svc.issueJWT(f.user)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.svc.MFAVerifyLogin(context.Background(), sess, f.codeAt(t, 0)); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("JWT sesi di tahap-2 harus ditolak, dapat %v", err)
	}
}

func TestMFA_LoginDuaTahapDanAntiReplay(t *testing.T) {
	f := newMFAFixture(t)
	ctx := context.Background()
	mt := f.loginForMFAToken(t)

	u, tok, err := f.svc.MFAVerifyLogin(ctx, mt, f.codeAt(t, 0))
	if err != nil || u.ID != 5 || tok == "" {
		t.Fatalf("kode benar harus lolos: %v", err)
	}
	if actor, err := f.svc.ResolveActor(ctx, tok); err != nil || actor.UserID != 5 {
		t.Fatalf("JWT hasil MFA harus sesi sah: %v %v", actor, err)
	}
	// kode yang SAMA dipakai ulang (replay) harus ditolak walau masih dalam jendela waktu
	if _, _, err := f.svc.MFAVerifyLogin(ctx, mt, f.codeAt(t, 0)); !errors.Is(err, ErrInvalidMFACode) {
		t.Fatalf("replay kode harus ditolak, dapat %v", err)
	}
}

func TestMFA_BruteForceTerkunciDanLockoutTidakDireset(t *testing.T) {
	f := newMFAFixture(t)
	ctx := context.Background()
	mt := f.loginForMFAToken(t)

	for i := 0; i < maxFailedLoginAttempts-1; i++ {
		if _, _, err := f.svc.MFAVerifyLogin(ctx, mt, "000000"); !errors.Is(err, ErrInvalidMFACode) {
			t.Fatalf("kode salah #%d: %v", i, err)
		}
	}
	// Login ulang (password benar) TIDAK boleh mereset hitungan gagal pada akun ber-MFA.
	mt = f.loginForMFAToken(t)
	if f.user.FailedLoginAttempts != maxFailedLoginAttempts-1 {
		t.Fatalf("hitungan gagal tak boleh direset oleh tahap-1: %d", f.user.FailedLoginAttempts)
	}
	// percobaan salah terakhir -> terkunci
	if _, _, err := f.svc.MFAVerifyLogin(ctx, mt, "111111"); !errors.Is(err, ErrInvalidMFACode) {
		t.Fatalf("%v", err)
	}
	if f.user.LockedUntil == nil {
		t.Fatal("akun harus terkunci setelah batas percobaan")
	}
	// ketika terkunci, bahkan kode BENAR ditolak
	if _, _, err := f.svc.MFAVerifyLogin(ctx, mt, f.codeAt(t, 0)); err == nil {
		t.Fatal("akun terkunci tak boleh lolos walau kode benar")
	}
}

func TestMFA_KodePemulihanSekaliPakai(t *testing.T) {
	f := newMFAFixture(t)
	ctx := context.Background()
	plain, hashes, err := f.svc.generateRecoveryCodes()
	if err != nil || len(plain) != recoveryCodeCount {
		t.Fatalf("kode pemulihan: %v %d", err, len(plain))
	}
	f.repo.codes[5] = map[string]bool{}
	for _, h := range hashes {
		f.repo.codes[5][h] = false
	}
	mt := f.loginForMFAToken(t)
	// huruf kecil & tanpa tanda hubung tetap dikenali
	norm := strings.ToLower(strings.ReplaceAll(plain[0], "-", ""))
	if _, _, err := f.svc.MFAVerifyLogin(ctx, mt, norm); err != nil {
		t.Fatalf("kode pemulihan sah harus lolos: %v", err)
	}
	if _, _, err := f.svc.MFAVerifyLogin(ctx, mt, plain[0]); !errors.Is(err, ErrInvalidMFACode) {
		t.Fatalf("kode pemulihan dipakai ulang harus ditolak: %v", err)
	}
	if left, _ := f.repo.CountRecoveryCodes(ctx, 5); left != recoveryCodeCount-1 {
		t.Fatalf("sisa kode = %d", left)
	}
}

func TestMFA_SetupEnableDisable(t *testing.T) {
	f := newMFAFixture(t)
	ctx := context.Background()
	f.repo.Disable(ctx, 5) // mulai dari akun TANPA MFA
	actor := domain.Actor{UserID: 5, TenantID: f.user.TenantID, Roles: f.user.Roles}

	// tanpa MFA: login langsung memberi sesi
	if _, tok, err := f.svc.Login(ctx, "ani@isp.id", mfaTestPassword); err != nil || tok == "" {
		t.Fatalf("akun tanpa MFA login biasa: %v", err)
	}
	// enable sebelum setup ditolak
	if _, err := f.svc.MFAEnable(ctx, actor, "123456"); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("enable tanpa setup: %v", err)
	}
	secret, uri, err := f.svc.MFASetup(ctx, actor)
	if err != nil || secret == "" || !strings.HasPrefix(uri, "otpauth://totp/") || !strings.Contains(uri, secret) {
		t.Fatalf("setup: %q %q %v", secret, uri, err)
	}
	if f.user.TOTPEnabled {
		t.Fatal("setup saja tidak boleh mengaktifkan MFA")
	}
	f.secret = secret
	// kode salah
	if _, err := f.svc.MFAEnable(ctx, actor, "000000"); !errors.Is(err, ErrInvalidMFACode) {
		t.Fatalf("enable dengan kode salah: %v", err)
	}
	codes, err := f.svc.MFAEnable(ctx, actor, f.codeAt(t, 0))
	if err != nil || len(codes) != recoveryCodeCount {
		t.Fatalf("enable: %v %d", err, len(codes))
	}
	if !f.user.TOTPEnabled {
		t.Fatal("MFA harus aktif")
	}
	// setup/enable ulang saat aktif = konflik
	if _, _, err := f.svc.MFASetup(ctx, actor); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("setup saat aktif: %v", err)
	}
	if en, left, _ := f.svc.MFAStatus(ctx, actor); !en || left != recoveryCodeCount {
		t.Fatalf("status: %v %d", en, left)
	}

	// disable: password salah / kode salah ditolak
	if _, err := f.svc.MFADisable(ctx, actor, "salah", f.codeAt(t, 30*time.Second)); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("disable password salah: %v", err)
	}
	if _, err := f.svc.MFADisable(ctx, actor, mfaTestPassword, "000000"); !errors.Is(err, ErrInvalidMFACode) {
		t.Fatalf("disable kode salah: %v", err)
	}
	tok, err := f.svc.MFADisable(ctx, actor, mfaTestPassword, codes[0]) // kode pemulihan juga sah
	if err != nil || tok == "" {
		t.Fatalf("disable: %v", err)
	}
	if f.user.TOTPEnabled || len(f.user.TOTPSecretEnc) != 0 {
		t.Fatal("rahasia & status MFA harus bersih setelah disable")
	}
}

func TestMFA_AdminReset(t *testing.T) {
	f := newMFAFixture(t)
	ctx := context.Background()
	other := uint64(2)
	if err := f.svc.MFAAdminReset(ctx, domain.Actor{UserID: 9, TenantID: &other, Roles: []string{domain.RoleAdmin}}, 5); err == nil {
		t.Fatal("ADMIN tenant lain tidak boleh mereset MFA")
	}
	mine := uint64(1)
	if err := f.svc.MFAAdminReset(ctx, domain.Actor{UserID: 9, TenantID: &mine, Roles: []string{domain.RoleNOC}}, 5); err == nil {
		t.Fatal("NOC tidak boleh mereset MFA")
	}
	if err := f.svc.MFAAdminReset(ctx, domain.Actor{UserID: 9, TenantID: &mine, Roles: []string{domain.RoleAdmin}}, 5); err != nil {
		t.Fatalf("ADMIN tenant sama: %v", err)
	}
	if f.user.TOTPEnabled {
		t.Fatal("MFA harus terlepas")
	}
	if err := f.svc.MFAAdminReset(ctx, domain.Actor{UserID: 1, Roles: []string{domain.RoleSuperadmin}}, 5); err != nil {
		t.Fatalf("superadmin: %v", err)
	}
}

// Tanpa WithMFA perilaku lama tidak berubah (fitur opsional).
func TestMFA_FiturMatiLoginBiasa(t *testing.T) {
	f := newMFAFixture(t)
	f.svc.mfa = nil
	if _, tok, err := f.svc.Login(context.Background(), "ani@isp.id", mfaTestPassword); err != nil || tok == "" {
		t.Fatalf("fitur MFA mati: login biasa harus tetap jalan: %v", err)
	}
	if _, _, err := f.svc.MFASetup(context.Background(), domain.Actor{UserID: 5}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("setup saat fitur mati: %v", err)
	}
}
