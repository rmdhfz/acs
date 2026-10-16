package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"acs/internal/domain"
	"acs/pkg/passwordhash"
	"acs/pkg/totp"
)

// ErrMFARequired dikembalikan Login bila password benar tetapi akun ber-MFA: nilai token yang ikut
// dikembalikan adalah token tahap-MFA (BUKAN sesi) yang hanya berlaku untuk MFAVerifyLogin.
var ErrMFARequired = errors.New("auth: verifikasi dua faktor diperlukan")

// ErrInvalidMFACode — kode TOTP/pemulihan salah. Sengaja satu pesan untuk semua penyebab.
var ErrInvalidMFACode = errors.New("auth: kode verifikasi salah")

const (
	mfaPurpose        = "mfa"
	mfaTokenTTL       = 5 * time.Minute
	mfaTOTPWindow     = 1 // toleransi ±1 langkah (30 dtk) untuk selisih jam
	recoveryCodeCount = 10
	// Alfabet tanpa karakter membingungkan (0/O, 1/I/L).
	recoveryAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"
)

// secretCipher — enkripsi rahasia TOTP at-rest (pkg/cryptoutil.Encryptor).
type secretCipher interface {
	Encrypt(plaintext string) ([]byte, error)
	Decrypt(ciphertext []byte) (string, error)
}

// WithMFA mengaktifkan MFA TOTP. Tanpa ini, akun tak bisa enrol dan Login berperilaku seperti sebelumnya.
func (s *Service) WithMFA(repo domain.MFARepository, cipher secretCipher, issuer string) *Service {
	s.mfa, s.cipher, s.mfaIssuer = repo, cipher, issuer
	return s
}

// MFAEnabledFeature — fitur MFA tersedia di instalasi ini.
func (s *Service) MFAEnabledFeature() bool { return s.mfa != nil && s.cipher != nil }

// issueMFAToken — token pendek khusus tahap kedua login. Memuat klaim purpose sehingga
// ResolveActor/ParseJWT MENOLAKNYA sebagai sesi.
func (s *Service) issueMFAToken(u *domain.User) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID:  u.ID,
		Purpose: mfaPurpose,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   u.UserUUID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(mfaTokenTTL)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.jwtSecret)
}

// hashRecoveryCode = HMAC-SHA256 dengan kunci turunan rahasia server (bukan SHA-256 polos): kode
// pemulihan berentropi ~49 bit, jadi hash polos dari DB yang bocor masih bisa di-brute-force offline;
// dengan kunci server, kebocoran DB saja tidak cukup. (Memutar ACS_JWT_SECRET membuat kode lama
// tak berlaku — pengguna cukup mengenrol ulang.)
func (s *Service) hashRecoveryCode(code string) string {
	norm := strings.ToUpper(strings.NewReplacer("-", "", " ", "").Replace(strings.TrimSpace(code)))
	mac := hmac.New(sha256.New, append([]byte("acs-mfa-recovery-v1:"), s.jwtSecret...))
	mac.Write([]byte(norm))
	return hex.EncodeToString(mac.Sum(nil))
}

func (s *Service) generateRecoveryCodes() (plain, hashes []string, err error) {
	for i := 0; i < recoveryCodeCount; i++ {
		b := make([]byte, 10)
		if _, err := rand.Read(b); err != nil {
			return nil, nil, err
		}
		chars := make([]byte, 10)
		for j, v := range b {
			chars[j] = recoveryAlphabet[int(v)%len(recoveryAlphabet)]
		}
		code := string(chars[:5]) + "-" + string(chars[5:])
		plain = append(plain, code)
		hashes = append(hashes, s.hashRecoveryCode(code))
	}
	return plain, hashes, nil
}

// looksLikeTOTP — 6 digit; selain itu diperlakukan sebagai kode pemulihan.
func looksLikeTOTP(code string) bool {
	if len(code) != totp.Digits {
		return false
	}
	for _, r := range code {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// checkSecondFactor memverifikasi kode TOTP (anti-replay lewat CAS langkah) ATAU kode pemulihan
// sekali pakai. Mengembalikan true bila sah.
func (s *Service) checkSecondFactor(ctx context.Context, u *domain.User, code string) (bool, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return false, nil
	}
	if looksLikeTOTP(code) {
		if len(u.TOTPSecretEnc) == 0 {
			return false, nil
		}
		secret, err := s.cipher.Decrypt(u.TOTPSecretEnc)
		if err != nil {
			return false, fmt.Errorf("auth: gagal membuka rahasia MFA: %w", err)
		}
		step, ok := totp.Verify(secret, code, time.Now(), mfaTOTPWindow)
		if !ok {
			return false, nil
		}
		return s.mfa.AdvanceStep(ctx, u.ID, step) // false = kode yang sama dipakai ulang
	}
	return s.mfa.UseRecoveryCode(ctx, u.ID, s.hashRecoveryCode(code))
}

// MFAVerifyLogin menyelesaikan login tahap kedua. Kegagalan dihitung ke lockout yang SAMA dengan
// password salah (jadi brute-force 6 digit terbatas 5 percobaan per 15 menit).
func (s *Service) MFAVerifyLogin(ctx context.Context, mfaToken, code string) (*domain.User, string, error) {
	if !s.MFAEnabledFeature() {
		return nil, "", ErrInvalidToken
	}
	claims, err := s.parseClaims(mfaToken)
	if err != nil || claims.Purpose != mfaPurpose {
		return nil, "", ErrInvalidToken
	}
	if err := s.checkSession(ctx, claims); err != nil {
		if errors.Is(err, errSessionLookup) {
			return nil, "", err
		}
		return nil, "", ErrInvalidToken
	}
	u, err := s.users.GetByID(ctx, claims.UserID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, "", ErrInvalidToken
		}
		return nil, "", err
	}
	if (u.LockedUntil != nil && u.LockedUntil.After(time.Now())) || !u.IsActive || !u.TOTPEnabled {
		s.auditLoginFailure(ctx, u, "mfa_ditolak")
		return nil, "", ErrInvalidCredentials
	}
	ok, err := s.checkSecondFactor(ctx, u, code)
	if err != nil {
		return nil, "", err
	}
	if !ok {
		locked, _ := s.users.RecordFailedLogin(ctx, u.ID, maxFailedLoginAttempts, time.Now().Add(lockoutDuration))
		s.auditLoginFailure(ctx, u, "mfa_salah")
		if locked {
			_ = s.activity.Record(ctx, &domain.ActivityLog{UserID: &u.ID, TenantID: u.TenantID, Action: "ACCOUNT_LOCKED", EntityType: "user", EntityID: &u.ID})
			if s.mail != nil {
				s.notify(s.mail.AccountLocked(u.Email, u.FullName, int(lockoutDuration.Minutes())))
			}
		}
		return nil, "", ErrInvalidMFACode
	}
	if err := s.checkTenantActive(ctx, u.TenantID); err != nil {
		return nil, "", err
	}
	_ = s.users.ResetLoginLockout(ctx, u.ID)
	token, err := s.issueJWT(u)
	if err != nil {
		return nil, "", err
	}
	_ = s.users.TouchLastLogin(ctx, u.ID, time.Now())
	_ = s.activity.Record(ctx, &domain.ActivityLog{UserID: &u.ID, TenantID: u.TenantID, Action: "LOGIN", EntityType: "user", EntityID: &u.ID, Details: map[string]any{"mfa": true}})
	return u, token, nil
}

// MFAStatus — status MFA akun sendiri.
func (s *Service) MFAStatus(ctx context.Context, actor domain.Actor) (enabled bool, recoveryLeft int, err error) {
	if !s.MFAEnabledFeature() {
		return false, 0, nil
	}
	u, err := s.users.GetByID(ctx, actor.UserID)
	if err != nil {
		return false, 0, err
	}
	if !u.TOTPEnabled {
		return false, 0, nil
	}
	n, err := s.mfa.CountRecoveryCodes(ctx, u.ID)
	return true, n, err
}

// MFASetup membuat rahasia baru (belum aktif sampai MFAEnable). Memanggil ulang sebelum aktif
// menggantikan rahasia yang menggantung.
func (s *Service) MFASetup(ctx context.Context, actor domain.Actor) (secret, uri string, err error) {
	if !s.MFAEnabledFeature() {
		return "", "", fmt.Errorf("%w: MFA tidak diaktifkan di instalasi ini", domain.ErrInvalidInput)
	}
	u, err := s.users.GetByID(ctx, actor.UserID)
	if err != nil {
		return "", "", err
	}
	if u.TOTPEnabled {
		return "", "", fmt.Errorf("%w: MFA sudah aktif; nonaktifkan dulu untuk enrol ulang", domain.ErrConflict)
	}
	secret, err = totp.GenerateSecret()
	if err != nil {
		return "", "", err
	}
	enc, err := s.cipher.Encrypt(secret)
	if err != nil {
		return "", "", err
	}
	if err := s.mfa.SaveSecret(ctx, u.ID, enc); err != nil {
		return "", "", err
	}
	return secret, totp.ProvisioningURI(s.mfaIssuer, u.Email, secret), nil
}

// MFAEnable membuktikan pengguna sudah memasukkan rahasia ke aplikasi authenticator, mengaktifkan
// MFA, dan mengembalikan kode pemulihan (hanya ditampilkan SEKALI).
func (s *Service) MFAEnable(ctx context.Context, actor domain.Actor, code string) ([]string, error) {
	if !s.MFAEnabledFeature() {
		return nil, fmt.Errorf("%w: MFA tidak diaktifkan di instalasi ini", domain.ErrInvalidInput)
	}
	u, err := s.users.GetByID(ctx, actor.UserID)
	if err != nil {
		return nil, err
	}
	if u.TOTPEnabled {
		return nil, fmt.Errorf("%w: MFA sudah aktif", domain.ErrConflict)
	}
	if len(u.TOTPSecretEnc) == 0 {
		return nil, fmt.Errorf("%w: jalankan setup MFA lebih dulu", domain.ErrInvalidInput)
	}
	secret, err := s.cipher.Decrypt(u.TOTPSecretEnc)
	if err != nil {
		return nil, err
	}
	step, ok := totp.Verify(secret, strings.TrimSpace(code), time.Now(), mfaTOTPWindow)
	if !ok {
		return nil, ErrInvalidMFACode
	}
	plain, hashes, err := s.generateRecoveryCodes()
	if err != nil {
		return nil, err
	}
	if err := s.mfa.Enable(ctx, u.ID, step, hashes); err != nil {
		return nil, err
	}
	_ = s.activity.Record(ctx, &domain.ActivityLog{UserID: actor.UserIDPtr(), TenantID: u.TenantID, Action: "MFA_ENABLED", EntityType: "user", EntityID: &u.ID})
	return plain, nil
}

// MFADisable mematikan MFA akun sendiri. Wajib password DAN kode kedua (token curian saja tidak
// cukup untuk melucuti MFA). Sesi lain dicabut; respons membawa JWT baru.
func (s *Service) MFADisable(ctx context.Context, actor domain.Actor, password, code string) (string, error) {
	if !s.MFAEnabledFeature() {
		return "", fmt.Errorf("%w: MFA tidak diaktifkan di instalasi ini", domain.ErrInvalidInput)
	}
	u, err := s.users.GetByID(ctx, actor.UserID)
	if err != nil {
		return "", err
	}
	if !u.TOTPEnabled {
		return "", fmt.Errorf("%w: MFA belum aktif", domain.ErrConflict)
	}
	if u.LockedUntil != nil && u.LockedUntil.After(time.Now()) {
		return "", ErrInvalidCredentials
	}
	ok, _, verr := passwordhash.Verify(password, u.PasswordHash)
	if errors.Is(verr, passwordhash.ErrBusy) {
		return "", verr
	}
	if verr != nil || !ok {
		_, _ = s.users.RecordFailedLogin(ctx, u.ID, maxFailedLoginAttempts, time.Now().Add(lockoutDuration))
		s.auditLoginFailure(ctx, u, "password_salah")
		return "", ErrInvalidCredentials
	}
	good, err := s.checkSecondFactor(ctx, u, code)
	if err != nil {
		return "", err
	}
	if !good {
		_, _ = s.users.RecordFailedLogin(ctx, u.ID, maxFailedLoginAttempts, time.Now().Add(lockoutDuration))
		s.auditLoginFailure(ctx, u, "mfa_salah")
		return "", ErrInvalidMFACode
	}
	if err := s.mfa.Disable(ctx, u.ID); err != nil {
		return "", err
	}
	_ = s.users.RevokeSessions(ctx, u.ID)
	_ = s.activity.Record(ctx, &domain.ActivityLog{UserID: actor.UserIDPtr(), TenantID: u.TenantID, Action: "MFA_DISABLED", EntityType: "user", EntityID: &u.ID})
	// Sesi lama (iat < password_changed_at) tercabut; JWT baru ini dikeluarkan setelahnya.
	return s.issueJWT(u)
}

// MFAAdminReset — admin melepas MFA akun yang pemiliknya kehilangan perangkat & kode pemulihan.
// Pemanggil wajib ADMIN tenant yang sama atau SUPERADMIN; sesi akun ditarget dicabut.
func (s *Service) MFAAdminReset(ctx context.Context, actor domain.Actor, targetID uint64) error {
	if !s.MFAEnabledFeature() {
		return fmt.Errorf("%w: MFA tidak diaktifkan di instalasi ini", domain.ErrInvalidInput)
	}
	if err := RequireRole(actor, domain.RoleSuperadmin, domain.RoleAdmin); err != nil {
		return err
	}
	u, err := s.users.GetByID(ctx, targetID)
	if err != nil {
		return err
	}
	if err := RequireTenantScope(actor, u.TenantID); err != nil {
		return err
	}
	if err := s.mfa.Disable(ctx, u.ID); err != nil {
		return err
	}
	_ = s.users.RevokeSessions(ctx, u.ID)
	_ = s.activity.Record(ctx, &domain.ActivityLog{UserID: actor.UserIDPtr(), TenantID: u.TenantID, Action: "MFA_RESET", EntityType: "user", EntityID: &u.ID})
	return nil
}
