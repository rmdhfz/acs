// Package auth menangani login, penerbitan/verifikasi JWT & API token, dan
// helper RBAC (role + scope tenant) — lihat TECH.md §1/§8.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/oauth2"

	"acs/internal/domain"
	"acs/internal/usecase/notify"
	"acs/pkg/mailer"
	"acs/pkg/passwordhash"
)

var ErrInvalidCredentials = errors.New("auth: email atau password salah")

// ErrTenantInactive -- login/akses ditolak krn tenant pemilik akun sudah
// dinonaktifkan superadmin (lihat usecase/iam.UpdateTenant). BEDA dari
// ErrInvalidCredentials: pesan ini SENGAJA lebih spesifik krn hanya
// dimunculkan SETELAH kredensial terbukti benar (Login) atau lewat token
// bearer yang valid (ResolveActor) -- pada titik itu identitas pemanggil
// sudah terbukti, jadi memberi tahu "tenant nonaktif, hubungi admin" tidak
// membocorkan apa pun ke pihak yang belum terautentikasi (beda kasus dgn
// lockout di bawah yang justru sengaja DISAMARKAN sbg ErrInvalidCredentials).
var ErrTenantInactive = errors.New("auth: tenant tidak aktif, hubungi administrator")

// ErrInvalidToken -- token bearer (JWT/API token) itu sendiri tidak valid
// (signature salah, kedaluwarsa, hash tidak ketemu, dst). Dipakai
// membungkus kegagalan parse/lookup token di ResolveActor supaya
// AuthMiddleware bisa membedakannya dari error SISTEM (mis. DB timeout saat
// checkTenantActive) -- keduanya HARUS direspons beda: token invalid = 401
// wajar & bervolume tinggi (tidak perlu di-log berisik), error sistem =
// 500 + wajib di-log utk on-call, BUKAN ikut disamarkan jadi "invalid
// token" begitu saja (temuan acs-security-reviewer/acs-code-reviewer soal
// checkTenantActive yang sebelumnya menyamaratakan semua error).
var ErrInvalidToken = errors.New("auth: token tidak valid")

// Proteksi brute-force login (CLAUDE.md/audit keamanan Fase 3 -- Login()
// sebelumnya sama sekali tidak melacak percobaan gagal).
const (
	// maxFailedLoginAttempts -- threshold sebelum akun dikunci. 5 dipilih
	// sbg titik tengah wajar: cukup longgar utk typo manusia (2-3x wajar),
	// tapi cukup ketat utk membatasi brute-force online (bukan offline hash
	// cracking -- itu domain kekuatan bcrypt cost, di luar cakupan ini).
	maxFailedLoginAttempts = 5
	// lockoutDuration -- 15 menit: cukup lama utk membuat brute-force online
	// tidak praktis (throughput tebakan jatuh drastis), cukup singkat agar
	// user yang lupa password tidak perlu menunggu lama/minta admin unlock
	// manual (iterasi ini sengaja tidak menyediakan endpoint unlock manual,
	// lihat catatan tugas -- reset password admin ikut membersihkan lockout).
	lockoutDuration = 15 * time.Minute
)

type Claims struct {
	UserID   uint64   `json:"uid"`
	TenantID *uint64  `json:"tid,omitempty"`
	Roles    []string `json:"roles"`
	// Purpose kosong = token sesi. Nilai lain (mis. "mfa" — tahap kedua login) BUKAN sesi dan
	// ditolak oleh ResolveActor/ParseJWT.
	Purpose string `json:"pur,omitempty"`
	// Imp != 0: token impersonate; id SUPERADMIN yang bertindak atas nama UserID (lihat impersonate.go).
	Imp uint64 `json:"imp,omitempty"`
	jwt.RegisteredClaims
}

type Service struct {
	oidcMu      sync.Mutex
	oidcCfg     *oidcSettings
	oidcLastTry time.Time

	users     domain.UserRepository
	tokens    domain.APITokenRepository
	tenants   domain.TenantRepository
	activity  domain.ActivityLogRepository
	jwtSecret []byte
	jwtExpiry time.Duration

	oidcProvider *oidc.Provider
	oauth2Config *oauth2.Config

	// Reset password lewat email & notifikasi keamanan — opsional (lihat
	// WithPasswordReset); tanpa ini fitur tersebut nonaktif dan Service
	// berperilaku persis seperti sebelumnya.
	// MFA TOTP — opsional (WithMFA).
	mfa       domain.MFARepository
	cipher    secretCipher
	mfaIssuer string

	resets   domain.PasswordResetRepository
	notifier mailer.Notifier
	mail     *notify.Builder
	resetTTL time.Duration

	// Notifikasi in-app SECURITY_ALERT (opsional, lihat WithNotifier); nil = tidak ada.
	alerts domain.Notifier
}

// WithNotifier memasang notifikasi persisten untuk kejadian keamanan akun. Kegagalan notifikasi diabaikan.
func (s *Service) WithNotifier(n domain.Notifier) *Service {
	s.alerts = n
	return s
}

// alert mengirim SECURITY_ALERT ke pemilik akun. noEmail=true bila email transaksional kejadian ini sudah ada.
func (s *Service) alert(ctx context.Context, userID uint64, tenantID *uint64, title, body string, noEmail bool) {
	if s.alerts == nil || userID == 0 {
		return
	}
	_ = s.alerts.Notify(ctx, domain.SecurityAlertEvent(userID, tenantID, title, body, noEmail))
}

// WithPasswordReset mengaktifkan reset password lewat email + email notifikasi
// keamanan (akun terkunci, password diganti). notifier tidak pernah memblok
// request dan kegagalannya tidak menggagalkan operasi.
func (s *Service) WithPasswordReset(resets domain.PasswordResetRepository, notifier mailer.Notifier, mail *notify.Builder, ttl time.Duration) *Service {
	s.resets, s.notifier, s.mail, s.resetTTL = resets, notifier, mail, ttl
	return s
}

// notify mengantrekan email bila pengiriman diaktifkan.
func (s *Service) notify(m mailer.Message) {
	if s.notifier != nil {
		s.notifier.Notify(m)
	}
}

func NewService(users domain.UserRepository, tokens domain.APITokenRepository, tenants domain.TenantRepository, activity domain.ActivityLogRepository, jwtSecret []byte, jwtExpiry time.Duration, issuer, clientID, clientSecret, redirectURL string) *Service {
	s := &Service{users: users, tokens: tokens, tenants: tenants, activity: activity, jwtSecret: jwtSecret, jwtExpiry: jwtExpiry}
	if issuer != "" && clientID != "" {
		s.oidcCfg = &oidcSettings{issuer: issuer, clientID: clientID, clientSecret: clientSecret, redirectURL: redirectURL}
		s.initOIDC(context.Background()) // best-effort; bila IdP belum terjangkau dicoba lagi saat dipakai
	}
	return s
}

type oidcSettings struct {
	issuer, clientID, clientSecret, redirectURL string
	allowedDomains                              []string
}

// WithOIDCAllowedDomains membatasi login SSO ke email berdomain tertentu (kosong = tanpa batas). Berlaku untuk
// semua IdP; untuk Google Workspace ini mencegah akun Gmail pribadi dengan email yang sama masuk.
func (s *Service) WithOIDCAllowedDomains(domains []string) *Service {
	if s.oidcCfg != nil {
		s.oidcCfg.allowedDomains = domains
	}
	return s
}

// OIDCProvider menamai IdP untuk tampilan tombol login: "google" bila issuer accounts.google.com, selain itu "oidc".
func (s *Service) OIDCProvider() string {
	if s.oidcCfg == nil {
		return ""
	}
	if u, err := url.Parse(s.oidcCfg.issuer); err == nil && strings.EqualFold(u.Host, "accounts.google.com") {
		return "google"
	}
	return "oidc"
}

// emailDomainAllowed — true bila tak ada pembatasan atau domain email ada di daftar.
func emailDomainAllowed(email string, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}
	at := strings.LastIndex(email, "@")
	if at < 0 {
		return false
	}
	d := strings.ToLower(email[at+1:])
	for _, a := range allowed {
		if d == a {
			return true
		}
	}
	return false
}

// initOIDC melakukan discovery IdP. Sebelumnya kegagalan sesaat saat startup (IdP belum siap)
// mematikan SSO permanen sampai restart; kini dicoba lagi (maks. tiap 30 detik) saat SSO dipakai.
func (s *Service) initOIDC(ctx context.Context) {
	s.oidcMu.Lock()
	defer s.oidcMu.Unlock()
	if s.oidcProvider != nil || s.oidcCfg == nil || time.Since(s.oidcLastTry) < 30*time.Second {
		return
	}
	s.oidcLastTry = time.Now()
	dctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	provider, err := oidc.NewProvider(dctx, s.oidcCfg.issuer)
	if err != nil {
		return
	}
	s.oauth2Config = &oauth2.Config{
		ClientID: s.oidcCfg.clientID, ClientSecret: s.oidcCfg.clientSecret, RedirectURL: s.oidcCfg.redirectURL,
		Endpoint: provider.Endpoint(), Scopes: []string{oidc.ScopeOpenID, "profile", "email"},
	}
	s.oidcProvider = provider
}

// HashPassword menghasilkan hash Argon2id (lihat pkg/passwordhash). Hash bcrypt
// lama tetap diterima Login dan di-upgrade otomatis saat login berhasil.
func HashPassword(password string) (string, error) {
	h, err := passwordhash.Hash(password)
	if errors.Is(err, passwordhash.ErrPasswordTooLong) {
		return "", fmt.Errorf("%w: password maksimal %d karakter", domain.ErrInvalidInput, passwordhash.MaxPasswordLen)
	}
	return h, err
}

// Login memverifikasi kredensial. identifier = EMAIL (berisi "@") atau username
// (kompatibilitas klien/skrip lama). Hash Argon2id; hash bcrypt lama diterima
// lalu di-upgrade ke Argon2id saat login berhasil.
func (s *Service) Login(ctx context.Context, identifier, password string) (*domain.User, string, error) {
	identifier = strings.TrimSpace(identifier)
	var (
		u   *domain.User
		err error
	)
	if strings.Contains(identifier, "@") {
		u, err = s.users.GetByEmail(ctx, strings.ToLower(identifier))
	} else {
		u, err = s.users.GetByUsername(ctx, identifier)
	}
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			// Ratakan waktu respons dgn jalur "akun ada" — cegah enumerasi
			// akun lewat selisih waktu (tanpa ini jalur ini ~instan).
			passwordhash.DummyVerify(password)
			return nil, "", ErrInvalidCredentials
		}
		return nil, "", err
	}
	// Lockout check SEBELUM verifikasi password -- pesan error yang
	// dikembalikan SAMA PERSIS dgn ErrInvalidCredentials biasa, sengaja tidak
	// membedakan "akun terkunci" dari "password salah" supaya penyerang yang
	// menebak akun tidak bisa memakai respons ini utk memastikan akun tsb valid.
	if (u.LockedUntil != nil && u.LockedUntil.After(time.Now())) || !u.IsActive {
		passwordhash.DummyVerify(password)
		reason := "akun_terkunci"
		if !u.IsActive {
			reason = "akun_nonaktif"
		}
		s.auditLoginFailure(ctx, u, reason)
		return nil, "", ErrInvalidCredentials
	}
	ok, needsRehash, verr := passwordhash.Verify(password, u.PasswordHash)
	if errors.Is(verr, passwordhash.ErrBusy) {
		// Antrean Argon2 penuh: BUKAN password salah — jangan hitung sebagai
		// percobaan gagal (bisa mengunci akun sah saat server padat/diserang).
		return nil, "", verr
	}
	if verr != nil || !ok {
		// Password salah -- catat via SATU UPDATE atomik (increment +
		// keputusan lock, lihat RecordFailedLogin) supaya request login
		// paralel utk akun yang sama tidak bisa saling lolos lockout (celah
		// TOCTOU, temuan acs-security-reviewer). Kegagalan mencatat di sini
		// sengaja tidak menggagalkan respons login.
		locked, _ := s.users.RecordFailedLogin(ctx, u.ID, maxFailedLoginAttempts, time.Now().Add(lockoutDuration))
		s.auditLoginFailure(ctx, u, "password_salah")
		if locked {
			_ = s.activity.Record(ctx, &domain.ActivityLog{UserID: &u.ID, TenantID: u.TenantID, Action: "ACCOUNT_LOCKED", EntityType: "user", EntityID: &u.ID})
		}
		if locked {
			s.alert(ctx, u.ID, u.TenantID, "Akun Anda dikunci sementara",
				fmt.Sprintf("Akun dikunci %d menit karena terlalu banyak percobaan masuk dengan password salah.", int(lockoutDuration.Minutes())), s.mail != nil)
		}
		if s.mail != nil && locked {
			// Pemanggilan INI yang mengunci akun (repo membandingkan locked_until
			// dgn nilai yang ia set): beri tahu pemilik akun SEKALI, bukan tiap
			// percobaan gagal / tiap request paralel.
			s.notify(s.mail.AccountLocked(u.Email, u.FullName, int(lockoutDuration.Minutes())))
		}
		return nil, "", ErrInvalidCredentials
	}
	if needsRehash {
		// Upgrade bertahap bcrypt -> Argon2id (atau parameter lebih kuat).
		// Best-effort: kegagalan tidak menggagalkan login.
		// Compare-and-swap: hanya bila hash di DB masih yang barusan diverifikasi
		// — tidak menimpa password yang baru diganti jalur lain (reset/ganti).
		if h, herr := passwordhash.Hash(password); herr == nil {
			_, _ = s.users.RehashPassword(ctx, u.ID, u.PasswordHash, h)
		}
	}

	if err := s.checkTenantActive(ctx, u.TenantID); err != nil {
		return nil, "", err
	}
	// Akun ber-MFA: password benar baru tahap pertama. Lockout TIDAK direset di sini (kalau
	// direset, penyerang yang tahu password bisa menebak kode 6 digit tanpa batas dengan
	// login ulang); reset terjadi setelah tahap kedua lolos (MFAVerifyLogin).
	if u.TOTPEnabled && s.MFAEnabledFeature() {
		mt, err := s.issueMFAToken(u)
		if err != nil {
			return nil, "", err
		}
		return u, mt, ErrMFARequired
	}

	// Password benar -- reset lockout counter, percobaan gagal sebelumnya
	// tidak relevan lagi setelah otentikasi berhasil.
	_ = s.users.ResetLoginLockout(ctx, u.ID)

	token, err := s.issueJWT(u)
	if err != nil {
		return nil, "", err
	}
	_ = s.users.TouchLastLogin(ctx, u.ID, time.Now())
	_ = s.activity.Record(ctx, &domain.ActivityLog{UserID: &u.ID, TenantID: u.TenantID, Action: "LOGIN", EntityType: "user", EntityID: &u.ID})
	return u, token, nil
}

// checkTenantActive menolak dgn ErrTenantInactive bila user py tenant_id dan
// tenant tsb sudah dinonaktifkan (is_active=0) atau sudah tidak ada
// (soft-deleted, -> ErrNotFound dari GetByID). User tanpa tenant_id
// (superadmin lintas-tenant) selalu lolos.
//
// PENTING: hanya domain.ErrNotFound yang diterjemahkan jadi ErrTenantInactive
// -- error LAIN dari GetByID (koneksi DB putus sesaat, timeout, dst.)
// di-propagate apa adanya, BUKAN ikut disamarkan jadi "tenant tidak aktif".
// Dipanggil AuthMiddleware di SETIAP request terautentikasi (lihat
// ResolveActor); menyamaratakan error transient jadi ErrTenantInactive akan
// membuat SEMUA user tenant tsb kelihatan "dinonaktifkan mendadak" saat DB
// sekadar blip sesaat -- menyulitkan on-call membedakan insiden nyata dari
// gangguan infrastruktur (temuan acs-security-reviewer + acs-code-reviewer).
func (s *Service) checkTenantActive(ctx context.Context, tenantID *uint64) error {
	if tenantID == nil {
		return nil
	}
	t, err := s.tenants.GetByID(ctx, *tenantID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return ErrTenantInactive
		}
		return err
	}
	if !t.IsActive {
		return ErrTenantInactive
	}
	return nil
}

// ChangeOwnPassword -- user mengganti password SENDIRI dgn verifikasi
// password lama (BEDA dari iam.Service.ResetUserPassword yang itu admin
// mereset password ORANG LAIN tanpa tahu password lama). Semua role
// (termasuk VIEWER) boleh memanggil ini utk akunnya sendiri -- tidak ada
// RBAC role gate di sini, cukup actor.UserID yg dipakai (bukan target ID
// dari path), lihat delivery/http router.go PATCH /auth/password.
//
// Mengembalikan JWT BARU: pergantian password mencabut semua sesi lama
// (password_changed_at), termasuk sesi pemanggil sendiri — token baru ini
// membuat pemanggil tetap login tanpa harus masuk ulang.
func (s *Service) ChangeOwnPassword(ctx context.Context, actor domain.Actor, currentPassword, newPassword string) (string, error) {
	u, err := s.users.GetByID(ctx, actor.UserID)
	if err != nil {
		return "", err
	}
	// Verifikasi password lama WAJIB sebelum izinkan ganti -- tanpa ini,
	// siapa pun yg mencuri/membajak bearer token (mis. lewat XSS) bisa
	// mengunci pemilik akun asli keluar permanen dgn mengganti password tanpa
	// perlu tahu password lama sama sekali.
	ok, _, verr := passwordhash.Verify(currentPassword, u.PasswordHash)
	if errors.Is(verr, passwordhash.ErrBusy) {
		return "", verr
	}
	if verr != nil || !ok {
		return "", ErrInvalidCredentials
	}
	if len(newPassword) < domain.MinUserPasswordLen {
		return "", fmt.Errorf("%w: password minimal %d karakter", domain.ErrInvalidInput, domain.MinUserPasswordLen)
	}
	hash, err := HashPassword(newPassword)
	if err != nil {
		return "", err
	}
	if err := s.users.UpdatePassword(ctx, u.ID, hash, actor.UserIDPtr()); err != nil {
		return "", err
	}
	// Reset lockout juga di sini utk konsistensi dgn ResetUserPassword admin
	// (ganti password berhasil = kesempatan baru), meski jalur ini hanya bisa
	// dicapai user yg sudah lolos verifikasi password lama (jadi akun ybs
	// pasti sedang tidak dalam status terkunci).
	_ = s.users.ResetLoginLockout(ctx, u.ID)
	// Token reset yang masih beredar tidak boleh bisa dipakai setelah password berubah.
	if s.resets != nil {
		_ = s.resets.InvalidateAllForUser(ctx, u.ID, time.Now())
	}
	// Password lama/baru TIDAK PERNAH dicatat ke activity_logs, hanya aksi +
	// entity (CLAUDE.md §8 -- kredensial tidak boleh muncul di log).
	_ = s.activity.Record(ctx, &domain.ActivityLog{UserID: actor.UserIDPtr(), TenantID: u.TenantID, Action: "CHANGE_OWN_PASSWORD", EntityType: "user", EntityID: &u.ID})
	if s.mail != nil {
		s.notify(s.mail.PasswordChanged(u.Email, u.FullName))
	}
	s.alert(ctx, u.ID, u.TenantID, "Password akun Anda diganti", "Password akun Anda baru saja diganti. Jika bukan Anda, segera hubungi administrator.", s.mail != nil)
	// iat token baru >= password_changed_at (presisi detik) sehingga lolos cek sesi.
	return s.issueJWT(u)
}

func (s *Service) issueJWT(u *domain.User) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID:   u.ID,
		TenantID: u.TenantID,
		Roles:    u.Roles,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   u.UserUUID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.jwtExpiry)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return tok.SignedString(s.jwtSecret)
}

func (s *Service) ParseJWT(tokenStr string) (*domain.Actor, error) {
	claims, err := s.parseClaims(tokenStr)
	if err != nil {
		return nil, err
	}
	if claims.Purpose != "" {
		return nil, ErrInvalidToken
	}
	return &domain.Actor{UserID: claims.UserID, TenantID: claims.TenantID, Roles: claims.Roles, ImpersonatorID: claims.Imp}, nil
}

func (s *Service) parseClaims(tokenStr string) (*Claims, error) {
	claims := &Claims{}
	_, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("auth: signing method tidak didukung")
		}
		return s.jwtSecret, nil
	})
	if err != nil {
		return nil, fmt.Errorf("auth: token tidak valid: %w", err)
	}
	return claims, nil
}

// ResolveActor adalah titik masuk TUNGGAL yang dipakai AuthMiddleware utk
// menerjemahkan header Authorization (Bearer JWT ATAU API token "acs_...")
// menjadi domain.Actor -- termasuk cek tenant aktif (ErrTenantInactive) di
// SETIAP request terautentikasi, bukan cuma saat Login. Ini sengaja tidak
// dibatasi "titik-titik mutasi penting" saja: tanpa pengecekan di sini, JWT
// yang diterbitkan SEBELUM tenant dinonaktifkan tetap berlaku penuh sampai
// expiry (bisa berjam-jam) -- menonaktifkan tenant jadi murni kosmetik bagi
// user yang masih menyimpan token lama. Trade-off: 1 query TenantRepository
// tambahan per request utk actor yang py tenant_id (superadmin dgn
// TenantID=nil selalu lolos tanpa query tambahan).
func (s *Service) ResolveActor(ctx context.Context, tokenStr string) (*domain.Actor, error) {
	var actor *domain.Actor
	var err error
	if strings.HasPrefix(tokenStr, "acs_") {
		actor, err = s.AuthenticateAPIToken(ctx, tokenStr)
	} else {
		var claims *Claims
		if claims, err = s.parseClaims(tokenStr); err == nil && claims.Purpose != "" {
			err = errors.New("token bukan token sesi")
		} else if err == nil {
			actor = &domain.Actor{UserID: claims.UserID, TenantID: claims.TenantID, Roles: claims.Roles, ImpersonatorID: claims.Imp}
			err = s.checkSession(ctx, claims)
			if err == nil && claims.Imp != 0 {
				err = s.checkImpersonator(ctx, claims)
			}
		}
	}
	if err != nil {
		if errors.Is(err, errSessionLookup) {
			return nil, err // error SISTEM (DB) — bukan token invalid; jangan disamarkan
		}
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}
	if err := s.checkTenantActive(ctx, actor.TenantID); err != nil {
		return nil, err
	}
	return actor, nil
}

// errSessionLookup — kegagalan SISTEM saat membaca status sesi (mis. DB putus);
// dibedakan dari token invalid supaya AuthMiddleware merespons 500, bukan 401.
var errSessionLookup = errors.New("auth: gagal memeriksa sesi")

// checkSession menolak JWT milik akun yang sudah dihapus/dinonaktifkan atau
// yang diterbitkan SEBELUM password terakhir diganti (migrations/0024). Satu
// query PK per request (trade-off sama dgn checkTenantActive).
func (s *Service) checkSession(ctx context.Context, claims *Claims) error {
	st, err := s.users.AuthState(ctx, claims.UserID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return errors.New("akun tidak ada")
		}
		return fmt.Errorf("%w: %v", errSessionLookup, err)
	}
	if !st.IsActive {
		return errors.New("akun nonaktif")
	}
	if st.PasswordChangedAt != nil && claims.IssuedAt != nil &&
		claims.IssuedAt.Time.Before(st.PasswordChangedAt.Truncate(time.Second)) {
		return errors.New("sesi dicabut karena password diganti")
	}
	return nil
}

// auditLoginFailure mencatat percobaan login gagal untuk akun YANG ADA (terikat lockout, jadi
// jumlah baris dibatasi). Identifier tak dikenal TIDAK dicatat ke DB (cegah banjir baris oleh
// penyerang); hanya IP+alasan di log aplikasi. Tidak pernah memuat password.
func (s *Service) auditLoginFailure(ctx context.Context, u *domain.User, reason string) {
	_ = s.activity.Record(ctx, &domain.ActivityLog{
		UserID: &u.ID, TenantID: u.TenantID, Action: "LOGIN_FAILED", EntityType: "user", EntityID: &u.ID,
		Details: map[string]any{"reason": reason},
	})
}

// ---- API Token ----

// MaxAPITokenTTL — umur maksimum (dan default) API token. Token tanpa batas
// waktu adalah kredensial permanen yang bocor sekali = akses selamanya; dengan
// batas ini rotasi minimal tahunan dipaksa. Integrasi yang butuh lebih lama
// harus menerbitkan ulang token, bukan memegang token abadi.
const MaxAPITokenTTL = 365 * 24 * time.Hour

// resolveAPITokenExpiry menegakkan kebijakan umur token: tidak diisi = default
// MaxAPITokenTTL; diisi harus di masa depan dan tidak melebihi MaxAPITokenTTL.
func resolveAPITokenExpiry(now time.Time, requested *time.Time) (*time.Time, error) {
	if requested == nil {
		d := now.Add(MaxAPITokenTTL)
		return &d, nil
	}
	if !requested.After(now) {
		return nil, fmt.Errorf("%w: expires_at harus di masa depan", domain.ErrInvalidInput)
	}
	if requested.After(now.Add(MaxAPITokenTTL)) {
		return nil, fmt.Errorf("%w: expires_at maksimal %d hari dari sekarang", domain.ErrInvalidInput, int(MaxAPITokenTTL.Hours()/24))
	}
	return requested, nil
}

// IssueAPIToken — non-superadmin hanya boleh menerbitkan token utk tenant-nya
// sendiri (RBAC scope tenant, CLAUDE.md). Tanpa cek ini, ADMIN tenant A bisa
// minta token dgn tenant_id tenant B lalu dapat akses tulis penuh permanen
// ke data tenant B (celah kritis, ditemukan audit isolasi tenant Fase 2).
func (s *Service) IssueAPIToken(ctx context.Context, actor domain.Actor, name string, tenantID *uint64, expiresAt *time.Time) (string, *domain.APIToken, error) {
	if !actor.IsSuperadmin() {
		if tenantID == nil || actor.TenantID == nil || *tenantID != *actor.TenantID {
			return "", nil, domain.ErrForbidden
		}
	}
	expiresAt, err := resolveAPITokenExpiry(time.Now(), expiresAt)
	if err != nil {
		return "", nil, err
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, err
	}
	plain := "acs_" + base64.RawURLEncoding.EncodeToString(raw)
	hash := sha256.Sum256([]byte(plain))

	rec := &domain.APIToken{
		TokenUUID: uuid.NewString(),
		UserID:    actor.UserIDPtr(),
		TenantID:  tenantID,
		Name:      name,
		TokenHash: hex.EncodeToString(hash[:]),
		ExpiresAt: expiresAt,
		CreatedBy: actor.UserIDPtr(),
	}
	if err := s.tokens.Create(ctx, rec); err != nil {
		return "", nil, err
	}
	_ = s.activity.Record(ctx, &domain.ActivityLog{
		UserID: actor.UserIDPtr(), TenantID: tenantID, Action: "ISSUE_API_TOKEN", EntityType: "api_token", EntityID: &rec.ID,
		Details: map[string]any{"name": name},
	})
	s.alert(ctx, actor.UserID, tenantID, "Token API baru dibuat", fmt.Sprintf("Token API %q dibuat atas akun Anda. Jika bukan Anda, segera cabut token tersebut.", name), false)
	return plain, rec, nil
}

func (s *Service) AuthenticateAPIToken(ctx context.Context, plain string) (*domain.Actor, error) {
	hash := sha256.Sum256([]byte(plain))
	rec, err := s.tokens.GetByHash(ctx, hex.EncodeToString(hash[:]))
	if err != nil {
		return nil, err
	}
	actor := domain.Actor{TenantID: rec.TenantID}
	if rec.UserID != nil {
		// Pemilik token harus masih ada DAN aktif: menonaktifkan/menghapus user
		// harus ikut mematikan API token-nya (sebelumnya token tetap berlaku penuh).
		u, err := s.users.GetByID(ctx, *rec.UserID)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return nil, errors.New("pemilik token tidak ada")
			}
			return nil, fmt.Errorf("%w: %v", errSessionLookup, err)
		}
		if !u.IsActive {
			return nil, errors.New("pemilik token nonaktif")
		}
		actor.UserID = u.ID
		actor.Roles = u.Roles
	}
	return &actor, nil
}

// ListAPITokens — GET /auth/tokens: superadmin lintas seluruh tenant (opsional
// filter tenantID), non-superadmin dipaksa ke tenant sendiri (auth.ScopedTenantFilter,
// pola sama dgn iam.Service.ListUsers) TERLEPAS siapa yang menerbitkan
// token tsb -- ADMIN perlu bisa melihat/mencabut token yang diterbitkan ADMIN
// lain di tenant yang sama (mis. karyawan penerbit token sudah resign),
// bukan cuma token milik dirinya sendiri.
func (s *Service) ListAPITokens(ctx context.Context, actor domain.Actor, tenantID *uint64, p domain.Pagination) ([]domain.APIToken, int, error) {
	if !actor.IsSuperadmin() {
		tid, err := ScopedTenantFilter(actor)
		if err != nil {
			return nil, 0, err
		}
		tenantID = tid
	}
	return s.tokens.List(ctx, tenantID, p)
}

// RevokeAPIToken — DELETE /auth/tokens/:id: superadmin bebas, non-superadmin
// hanya boleh mencabut token milik tenant-nya sendiri (RequireTenantScope,
// pola sama dgn requireUserMutationScope di usecase/iam: resolve dulu baru
// cek scope). Tanpa cek ini, ADMIN tenant A bisa menebak/enumerasi id token
// lalu mencabut token tenant B secara sembarangan (DoS terhadap integrasi
// tenant lain) -- kelas celah yang sama dgn yang sudah diperbaiki di
// IssueAPIToken di atas.
func (s *Service) RevokeAPIToken(ctx context.Context, actor domain.Actor, id uint64) error {
	tok, err := s.tokens.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if err := RequireTenantScope(actor, tok.TenantID); err != nil {
		return err
	}
	if err := s.tokens.Revoke(ctx, id); err != nil {
		return err
	}
	_ = s.activity.Record(ctx, &domain.ActivityLog{
		UserID: actor.UserIDPtr(), TenantID: tok.TenantID, Action: "REVOKE_API_TOKEN", EntityType: "api_token", EntityID: &id,
	})
	return nil
}

// ---- RBAC helpers ----

// RequireRole mengizinkan superadmin selalu lolos, atau actor yang punya
// salah satu role di allowed.
func RequireRole(actor domain.Actor, allowed ...string) error {
	if actor.IsSuperadmin() {
		return nil
	}
	for _, r := range allowed {
		if actor.HasRole(r) {
			return nil
		}
	}
	return domain.ErrForbidden
}

// RequireTenantScope memastikan actor non-superadmin hanya mengakses resource
// milik tenant-nya sendiri (CLAUDE.md: RBAC scope tenant wajib per endpoint mutasi).
func RequireTenantScope(actor domain.Actor, resourceTenantID *uint64) error {
	if actor.IsSuperadmin() {
		return nil
	}
	if resourceTenantID == nil || actor.TenantID == nil || *actor.TenantID != *resourceTenantID {
		return domain.ErrForbidden
	}
	return nil
}

// ScopedTenantFilter mengembalikan filter tenant_id yang aman dipakai di
// query List/Stats: nil (tanpa filter WHERE tenant_id, lintas tenant) HANYA
// valid untuk superadmin. Actor non-superadmin TANPA tenant_id (akun salah
// konfigurasi — mis. dibuat tanpa tenant_id lewat cabang superadmin di
// CreateUser) mengembalikan ErrForbidden, BUKAN nil: nil di titik pemanggilan
// berarti "tanpa filter" di level SQL, yang kalau actor-nya bukan superadmin
// berarti bocor data SELURUH tenant (celah ditemukan acs-security-reviewer
// lewat audit menyeluruh, ROADMAP.md Fase 0).
func ScopedTenantFilter(actor domain.Actor) (*uint64, error) {
	if actor.IsSuperadmin() {
		return nil, nil
	}
	if actor.TenantID == nil {
		return nil, domain.ErrForbidden
	}
	return actor.TenantID, nil
}

// ---- OIDC Integration ----

func (s *Service) HasOIDC() bool {
	s.initOIDC(context.Background())
	s.oidcMu.Lock()
	defer s.oidcMu.Unlock()
	return s.oidcProvider != nil && s.oauth2Config != nil
}

// OIDCConfigured — SSO dikonfigurasi (issuer+client id) walau IdP sedang tak terjangkau;
// dipakai untuk menampilkan tombol SSO.
func (s *Service) OIDCConfigured() bool { return s.oidcCfg != nil }

// Me — profil akun yang sedang login (untuk SPA setelah login SSO yang hanya membawa token).
func (s *Service) Me(ctx context.Context, actor domain.Actor) (*domain.User, error) {
	return s.users.GetByID(ctx, actor.UserID)
}

// ErrOIDCNonceMismatch — nonce di id_token tidak sama dengan yang diikat ke
// browser yang memulai login (cookie). Menandakan id_token hasil replay/
// injeksi, bukan hasil alur yang dimulai browser ini.
var ErrOIDCNonceMismatch = errors.New("auth: nonce OIDC tidak cocok")

// GetOIDCAuthURL membangun URL otorisasi IdP. state (anti-CSRF login) dan
// nonce (mengikat id_token ke sesi browser) HARUS acak per-permintaan dan
// diikat ke browser pemanggil oleh delivery (cookie) — lihat
// internal/delivery/http/oidc_flow.go.
func (s *Service) GetOIDCAuthURL(state, nonce string) (string, error) {
	if !s.HasOIDC() {
		return "", errors.New("auth: OIDC tidak dikonfigurasi")
	}
	opts := []oauth2.AuthCodeOption{oidc.Nonce(nonce)}
	if s.OIDCProvider() == "google" {
		// Selalu tampilkan pemilih akun Google: pengguna dengan beberapa akun tak otomatis masuk dengan yang salah.
		opts = append(opts, oauth2.SetAuthURLParam("prompt", "select_account"))
	}
	return s.oauth2Config.AuthCodeURL(state, opts...), nil
}

// validateOIDCIdentity memeriksa klaim id_token yang sudah lolos verifikasi
// tanda tangan: nonce wajib cocok (konstan-waktu) dan email tidak boleh
// dinyatakan belum terverifikasi. email_verified yang TIDAK ada di klaim
// diterima (Azure AD dan beberapa IdP tidak mengirimnya); yang eksplisit
// false ditolak — tanpa ini seorang pengguna IdP dengan email belum
// terverifikasi milik admin bisa masuk sebagai admin itu (akun dipetakan
// lewat email).
func validateOIDCIdentity(tokenNonce, expectedNonce string, emailVerified *bool, email string) error {
	if expectedNonce == "" || subtle.ConstantTimeCompare([]byte(tokenNonce), []byte(expectedNonce)) != 1 {
		return ErrOIDCNonceMismatch
	}
	if email == "" {
		return errors.New("auth: email tidak ditemukan dalam klaim OIDC")
	}
	if emailVerified != nil && !*emailVerified {
		return errors.New("auth: email OIDC belum terverifikasi di Identity Provider")
	}
	return nil
}

// ErrOIDCSubjectMismatch — akun sudah terikat ke identitas IdP lain.
var ErrOIDCSubjectMismatch = errors.New("auth: identitas IdP tidak cocok dengan akun")

// verifyOIDCBinding mengikat/memverifikasi identitas IdP (issuer|sub): email saja tidak cukup — bila alamat email
// berpindah tangan di IdP, sub-nya berbeda dan login ditolak. Ikatan dibuat pada login pertama (compare-and-set) dan
// dikosongkan saat email pengguna diubah.
func (s *Service) verifyOIDCBinding(ctx context.Context, u *domain.User, subject string) error {
	if u.OIDCSubject != nil && *u.OIDCSubject != "" {
		if subtle.ConstantTimeCompare([]byte(*u.OIDCSubject), []byte(subject)) != 1 {
			_ = s.activity.Record(ctx, &domain.ActivityLog{UserID: &u.ID, TenantID: u.TenantID, Action: "OIDC_SUBJECT_MISMATCH", EntityType: "user", EntityID: &u.ID})
			return ErrOIDCSubjectMismatch
		}
		return nil
	}
	bound, err := s.users.BindOIDCSubject(ctx, u.ID, subject)
	if err != nil {
		return err
	}
	if bound {
		_ = s.activity.Record(ctx, &domain.ActivityLog{UserID: &u.ID, TenantID: u.TenantID, Action: "OIDC_BOUND", EntityType: "user", EntityID: &u.ID})
		return nil
	}
	// kalah balapan dengan login lain: muat ulang dan cocokkan
	cur, err := s.users.GetByID(ctx, u.ID)
	if err != nil || cur.OIDCSubject == nil || subtle.ConstantTimeCompare([]byte(*cur.OIDCSubject), []byte(subject)) != 1 {
		return ErrOIDCSubjectMismatch
	}
	return nil
}

func (s *Service) OIDCLogin(ctx context.Context, code, expectedNonce string) (*domain.User, string, error) {
	if !s.HasOIDC() {
		return nil, "", errors.New("auth: OIDC tidak dikonfigurasi")
	}

	oauth2Token, err := s.oauth2Config.Exchange(ctx, code)
	if err != nil {
		return nil, "", fmt.Errorf("auth: gagal menukar kode OIDC: %w", err)
	}

	rawIDToken, ok := oauth2Token.Extra("id_token").(string)
	if !ok {
		return nil, "", errors.New("auth: tidak ada id_token pada respons OIDC")
	}

	verifier := s.oidcProvider.Verifier(&oidc.Config{ClientID: s.oauth2Config.ClientID})
	idToken, err := verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, "", fmt.Errorf("auth: gagal memverifikasi id_token: %w", err)
	}

	var claims struct {
		Email         string   `json:"email"`
		EmailVerified *bool    `json:"email_verified"`
		Name          string   `json:"name"`
		Groups        []string `json:"groups"` // Bisa didapat dari Azure AD / Okta claims
	}
	if err := idToken.Claims(&claims); err != nil {
		return nil, "", fmt.Errorf("auth: gagal mengekstrak claims: %w", err)
	}

	if err := validateOIDCIdentity(idToken.Nonce, expectedNonce, claims.EmailVerified, claims.Email); err != nil {
		return nil, "", err
	}

	// Cek apakah user ada berdasarkan email
	// Dipetakan lewat EMAIL (unik, ditegakkan sistem), BUKAN username: username bisa dipilih
	// admin tenant mana pun sehingga memetakan lewat username memungkinkan pengambilalihan
	// akun dengan membuat username = email korban.
	email := strings.ToLower(strings.TrimSpace(claims.Email))
	if !emailDomainAllowed(email, s.oidcCfg.allowedDomains) {
		return nil, "", errors.New("auth: domain email tidak diizinkan untuk SSO")
	}
	u, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		// Auto-provisioning sengaja tidak ada: akun harus dibuat admin (multi-tenant).
		return nil, "", errors.New("auth: akun OIDC belum terdaftar, hubungi admin")
	}

	if !u.IsActive {
		return nil, "", ErrInvalidCredentials
	}

	if idToken.Subject == "" {
		return nil, "", errors.New("auth: id_token tanpa sub")
	}
	if err := s.verifyOIDCBinding(ctx, u, s.oidcCfg.issuer+"|"+idToken.Subject); err != nil {
		return nil, "", err
	}

	if err := s.checkTenantActive(ctx, u.TenantID); err != nil {
		return nil, "", err
	}

	token, err := s.issueJWT(u)
	if err != nil {
		return nil, "", err
	}
	_ = s.users.TouchLastLogin(ctx, u.ID, time.Now())
	_ = s.activity.Record(ctx, &domain.ActivityLog{UserID: &u.ID, TenantID: u.TenantID, Action: "OIDC_LOGIN", EntityType: "user", EntityID: &u.ID})
	s.alert(ctx, u.ID, u.TenantID, "Login dengan SSO", "Login dengan SSO (OIDC) berhasil pada akun Anda. Jika bukan Anda, segera ganti password dan hubungi administrator.", false)
	return u, token, nil
}
