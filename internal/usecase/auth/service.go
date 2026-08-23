// Package auth menangani login, penerbitan/verifikasi JWT & API token, dan
// helper RBAC (role + scope tenant) — lihat TECH.md §1/§8.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"acs/internal/domain"
)

var ErrInvalidCredentials = errors.New("auth: username atau password salah")

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
	jwt.RegisteredClaims
}

type Service struct {
	users     domain.UserRepository
	tokens    domain.APITokenRepository
	tenants   domain.TenantRepository
	activity  domain.ActivityLogRepository
	jwtSecret []byte
	jwtExpiry time.Duration
}

func NewService(users domain.UserRepository, tokens domain.APITokenRepository, tenants domain.TenantRepository, activity domain.ActivityLogRepository, jwtSecret []byte, jwtExpiry time.Duration) *Service {
	return &Service{users: users, tokens: tokens, tenants: tenants, activity: activity, jwtSecret: jwtSecret, jwtExpiry: jwtExpiry}
}

func HashPassword(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(b), err
}

func (s *Service) Login(ctx context.Context, username, password string) (*domain.User, string, error) {
	u, err := s.users.GetByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, "", ErrInvalidCredentials
		}
		return nil, "", err
	}
	// Lockout check SEBELUM verifikasi password (dan sebelum bcrypt dipanggil
	// sama sekali) -- pesan error yang dikembalikan SAMA PERSIS dgn
	// ErrInvalidCredentials biasa, sengaja tidak membedakan "akun terkunci"
	// dari "password salah" supaya penyerang yang menebak username tidak
	// bisa memakai respons ini utk memastikan username tsb valid.
	if u.LockedUntil != nil && u.LockedUntil.After(time.Now()) {
		return nil, "", ErrInvalidCredentials
	}
	if !u.IsActive {
		return nil, "", ErrInvalidCredentials
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		// Password salah -- catat via SATU UPDATE atomik (increment +
		// keputusan lock, lihat RecordFailedLogin) supaya request login
		// paralel utk username yang sama tidak bisa saling lolos lockout
		// (celah TOCTOU yang sempat ada di versi dua-langkah sebelumnya,
		// ditemukan acs-security-reviewer). Kegagalan mencatat di sini
		// sengaja tidak menggagalkan respons login (tetap
		// ErrInvalidCredentials) -- gangguan pada proteksi brute-force tidak
		// boleh membuka celah lain (mis. leak error DB ke klien) atau
		// menghalangi user lain login.
		_ = s.users.RecordFailedLogin(ctx, u.ID, maxFailedLoginAttempts, time.Now().Add(lockoutDuration))
		return nil, "", ErrInvalidCredentials
	}

	// Password benar -- reset lockout counter, percobaan gagal sebelumnya
	// tidak relevan lagi setelah otentikasi berhasil.
	_ = s.users.ResetLoginLockout(ctx, u.ID)

	if err := s.checkTenantActive(ctx, u.TenantID); err != nil {
		return nil, "", err
	}

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
func (s *Service) ChangeOwnPassword(ctx context.Context, actor domain.Actor, currentPassword, newPassword string) error {
	u, err := s.users.GetByID(ctx, actor.UserID)
	if err != nil {
		return err
	}
	// Verifikasi password lama WAJIB sebelum izinkan ganti -- tanpa ini,
	// siapa pun yg mencuri/membajak bearer token (mis. lewat XSS) bisa
	// mengunci pemilik akun asli keluar permanen dgn mengganti password tanpa
	// perlu tahu password lama sama sekali.
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(currentPassword)); err != nil {
		return ErrInvalidCredentials
	}
	if len(newPassword) < domain.MinUserPasswordLen {
		return fmt.Errorf("%w: password minimal %d karakter", domain.ErrInvalidInput, domain.MinUserPasswordLen)
	}
	hash, err := HashPassword(newPassword)
	if err != nil {
		return err
	}
	if err := s.users.UpdatePassword(ctx, u.ID, hash, actor.UserIDPtr()); err != nil {
		return err
	}
	// Reset lockout juga di sini utk konsistensi dgn ResetUserPassword admin
	// (ganti password berhasil = kesempatan baru), meski jalur ini hanya bisa
	// dicapai user yg sudah lolos verifikasi password lama (jadi akun ybs
	// pasti sedang tidak dalam status terkunci).
	_ = s.users.ResetLoginLockout(ctx, u.ID)
	// Password lama/baru TIDAK PERNAH dicatat ke activity_logs, hanya aksi +
	// entity (CLAUDE.md §8 -- kredensial tidak boleh muncul di log).
	_ = s.activity.Record(ctx, &domain.ActivityLog{UserID: actor.UserIDPtr(), TenantID: u.TenantID, Action: "CHANGE_OWN_PASSWORD", EntityType: "user", EntityID: &u.ID})
	return nil
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
	return &domain.Actor{UserID: claims.UserID, TenantID: claims.TenantID, Roles: claims.Roles}, nil
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
		actor, err = s.ParseJWT(tokenStr)
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}
	if err := s.checkTenantActive(ctx, actor.TenantID); err != nil {
		return nil, err
	}
	return actor, nil
}

// ---- API Token ----

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
		u, err := s.users.GetByID(ctx, *rec.UserID)
		if err == nil {
			actor.UserID = u.ID
			actor.Roles = u.Roles
		}
	}
	return &actor, nil
}

func (s *Service) RevokeAPIToken(ctx context.Context, id uint64) error {
	return s.tokens.Revoke(ctx, id)
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
