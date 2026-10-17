// Package signup — pendaftaran tenant mandiri (self-signup) dengan verifikasi email.
//
// Desain anti-penyalahgunaan:
//   - hanya aktif bila dikonfigurasi (ACS_SELF_SIGNUP); bawaan MATI;
//   - tenant & admin TIDAK dibuat saat mendaftar — hanya baris tenant_signups (hash token + hash password) yang
//     menunggu; tenant dibuat setelah tautan di email dipakai, sehingga pendaftaran palsu tidak mengotori data master;
//   - respons pendaftaran selalu seragam (tidak membocorkan apakah email sudah terdaftar) dan Argon2id dihitung SEBELUM
//     percabangan supaya waktu respons juga seragam;
//   - batas per IP, per email, dan plafon harian global; honeypot untuk bot sederhana;
//   - tenant hasil pendaftaran mulai dengan batas perangkat & antrean task bawaan (paket awal), dinaikkan superadmin.
package signup

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"

	"acs/internal/domain"
	"acs/internal/usecase/auth"
	"acs/internal/usecase/iam"
	"acs/internal/usecase/notify"
	"acs/pkg/mailer"
)

var (
	ErrDisabled = errors.New("signup: pendaftaran mandiri tidak diaktifkan")
	ErrClosed   = errors.New("signup: pendaftaran sedang penuh, coba lagi nanti")
	ErrInvalid  = errors.New("signup: tautan verifikasi tidak valid atau sudah kedaluwarsa")
)

const (
	TokenTTL      = 24 * time.Hour
	perIPPerHour  = 5
	perEmailPerDy = 3
)

// Provisioner — bagian iam.Service yang dipakai (antarmuka sempit agar mudah diuji).
type Provisioner interface {
	CreateTenant(ctx context.Context, actor domain.Actor, in iam.CreateTenantInput) (*domain.Tenant, error)
	CreateUser(ctx context.Context, actor domain.Actor, in iam.CreateUserInput) (*domain.User, error)
	SetTaskQuota(ctx context.Context, actor domain.Actor, tenantID uint64, max *uint32) error
	SetDeviceLimit(ctx context.Context, actor domain.Actor, tenantID uint64, max *uint32) error
	DeleteTenant(ctx context.Context, actor domain.Actor, tenantID uint64) error
}

// EmailLookup — pencarian akun berdasarkan email (domain.UserRepository).
type EmailLookup interface {
	GetByEmail(ctx context.Context, email string) (*domain.User, error)
}

type Config struct {
	Enabled         bool
	DailyCap        int    // plafon pendaftaran per 24 jam di seluruh platform
	MaxDevices      uint32 // batas perangkat awal tenant baru (0 = tak dibatasi)
	MaxPendingTasks uint32 // batas antrean task awal (0 = tak dibatasi)
}

type Service struct {
	repo     domain.SignupRepository
	prov     Provisioner
	users    EmailLookup
	activity domain.ActivityLogRepository
	notifier mailer.Notifier
	mail     *notify.Builder
	cfg      Config
	now      func() time.Time

	alerts domain.Notifier // opsional (WithNotifier): notifikasi NEW_TENANT_SIGNUP
}

// WithNotifier memasang notifikasi persisten untuk SUPERADMIN saat tenant hasil pendaftaran mandiri terbentuk.
func (s *Service) WithNotifier(n domain.Notifier) *Service {
	s.alerts = n
	return s
}

func NewService(repo domain.SignupRepository, prov Provisioner, users EmailLookup, activity domain.ActivityLogRepository, notifier mailer.Notifier, mail *notify.Builder, cfg Config) *Service {
	return &Service{repo: repo, prov: prov, users: users, activity: activity, notifier: notifier, mail: mail, cfg: cfg, now: time.Now}
}

func (s *Service) Enabled() bool { return s.cfg.Enabled }

// sysActor — aksi sistem (provisioning tenant hasil pendaftaran): peran SUPERADMIN tanpa user (created_by NULL).
var sysActor = domain.Actor{Roles: []string{domain.RoleSuperadmin}}

type RequestInput struct {
	CompanyName string
	FullName    string
	Email       string
	Password    string
	AcceptTerms bool
	Honeypot    string // field tersembunyi di form; manusia tidak mengisinya
	IP          string
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

func hasControl(s string) bool {
	for _, r := range s {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}

func validate(in *RequestInput) error {
	in.CompanyName = strings.TrimSpace(in.CompanyName)
	in.FullName = strings.TrimSpace(in.FullName)
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	if n := len([]rune(in.CompanyName)); n < 3 || n > 128 || hasControl(in.CompanyName) {
		return fmt.Errorf("%w: nama perusahaan 3–128 karakter", domain.ErrInvalidInput)
	}
	if n := len([]rune(in.FullName)); n < 2 || n > 128 || hasControl(in.FullName) {
		return fmt.Errorf("%w: nama lengkap 2–128 karakter", domain.ErrInvalidInput)
	}
	a, err := mail.ParseAddress(in.Email)
	if err != nil || a.Address != in.Email || len(in.Email) > 191 {
		return fmt.Errorf("%w: email tidak valid", domain.ErrInvalidInput)
	}
	if len(in.Password) < 10 || len(in.Password) > 128 {
		return fmt.Errorf("%w: password 10–128 karakter", domain.ErrInvalidInput)
	}
	low := strings.ToLower(in.Password)
	if low == in.Email || strings.Contains(low, strings.ToLower(strings.ReplaceAll(in.CompanyName, " ", ""))) {
		return fmt.Errorf("%w: password tidak boleh memuat email atau nama perusahaan", domain.ErrInvalidInput)
	}
	if !in.AcceptTerms {
		return fmt.Errorf("%w: ketentuan penggunaan harus disetujui", domain.ErrInvalidInput)
	}
	return nil
}

// tenantCode — slug nama perusahaan + sufiks acak: unik tanpa membuka peluang "squatting" nama merek.
func tenantCode(company string) string {
	slug := strings.Trim(slugRe.ReplaceAllString(strings.ToLower(company), "-"), "-")
	if len(slug) > 20 {
		slug = strings.Trim(slug[:20], "-")
	}
	if slug == "" {
		slug = "tenant"
	}
	b := make([]byte, 3)
	_, _ = rand.Read(b)
	return slug + "-" + hex.EncodeToString(b)
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// Request menerima pendaftaran. Mengembalikan nil untuk kasus yang sengaja diam (bot, email sudah ada, batas per
// email) agar respons seragam; galat hanya untuk validasi, fitur mati, batas IP, dan plafon penuh.
func (s *Service) Request(ctx context.Context, in RequestInput) error {
	if !s.cfg.Enabled {
		return ErrDisabled
	}
	if strings.TrimSpace(in.Honeypot) != "" {
		return nil // bot: seolah sukses, tidak ada yang dibuat/dikirim
	}
	if err := validate(&in); err != nil {
		return err
	}
	now := s.now()

	// Argon2id dihitung SEBELUM cabang apa pun: waktu respons tidak membedakan email baru vs sudah terdaftar.
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		return err
	}

	if in.IP != "" {
		if n, err := s.repo.CountByIPSince(ctx, in.IP, now.Add(-time.Hour)); err != nil {
			return err
		} else if n >= perIPPerHour {
			return fmt.Errorf("%w: terlalu banyak pendaftaran dari alamat ini, coba lagi nanti", domain.ErrQuotaExceeded)
		}
	}
	if s.cfg.DailyCap > 0 {
		if n, err := s.repo.CountSince(ctx, now.Add(-24*time.Hour)); err != nil {
			return err
		} else if n >= s.cfg.DailyCap {
			return ErrClosed
		}
	}
	if n, err := s.repo.CountByEmailSince(ctx, in.Email, now.Add(-24*time.Hour)); err != nil {
		return err
	} else if n >= perEmailPerDy {
		return nil
	}
	if _, err := s.users.GetByEmail(ctx, in.Email); err == nil {
		return nil // sudah terdaftar: tidak membuat apa pun, tidak membocorkan
	} else if !errors.Is(err, domain.ErrNotFound) {
		return err
	}

	tokBytes := make([]byte, 32)
	if _, err := rand.Read(tokBytes); err != nil {
		return err
	}
	token := base64.RawURLEncoding.EncodeToString(tokBytes)
	row := &domain.TenantSignup{
		SignupUUID: uuid.NewString(), CompanyName: in.CompanyName, TenantCode: tenantCode(in.CompanyName), Email: in.Email,
		FullName: in.FullName, PasswordHash: hash, TokenHash: hashToken(token), ExpiresAt: now.Add(TokenTTL),
	}
	if in.IP != "" {
		ip := in.IP
		if len(ip) > 45 {
			ip = ip[:45]
		}
		row.IPAddress = &ip
	}
	if err := s.repo.Create(ctx, row); err != nil {
		return err
	}
	_ = s.activity.Record(ctx, &domain.ActivityLog{Action: "SIGNUP_REQUESTED", EntityType: "signup", EntityID: &row.ID, Details: map[string]any{"company": in.CompanyName}})
	if s.notifier != nil && s.mail != nil {
		s.notifier.Notify(s.mail.SignupVerification(in.Email, in.FullName, in.CompanyName, s.mail.VerifySignupLink(token), TokenTTL))
	}
	return nil
}

type VerifyResult struct {
	TenantCode string
	TenantName string
	Email      string
}

// Verify memakai token: membuat tenant + admin pertama. Sekali pakai (Claim atomik); bila pembuatan gagal di tengah
// jalan, tenant setengah jadi dihapus dan klaim dibatalkan supaya tautan bisa dicoba lagi.
func (s *Service) Verify(ctx context.Context, token string) (*VerifyResult, error) {
	if !s.cfg.Enabled {
		return nil, ErrDisabled
	}
	token = strings.TrimSpace(token)
	if len(token) < 20 || len(token) > 200 {
		return nil, ErrInvalid
	}
	row, err := s.repo.GetByTokenHash(ctx, hashToken(token))
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, ErrInvalid
		}
		return nil, err
	}
	now := s.now()
	if row.VerifiedAt != nil || !row.ExpiresAt.After(now) {
		return nil, ErrInvalid
	}
	ok, err := s.repo.Claim(ctx, row.ID, now)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrInvalid // klik ganda / paralel: yang lain sudah memakainya
	}

	tenant, err := s.createTenant(ctx, row)
	if err != nil {
		_ = s.repo.Unclaim(ctx, row.ID)
		return nil, err
	}
	username := row.Email
	if len(username) > 64 {
		username = "admin-" + strings.TrimPrefix(row.TenantCode, "tenant-")
	}
	if _, err := s.prov.CreateUser(ctx, sysActor, iam.CreateUserInput{
		TenantID: &tenant.ID, Username: username, Email: row.Email, PasswordHash: row.PasswordHash,
		FullName: row.FullName, RoleCodes: []string{domain.RoleAdmin},
	}); err != nil {
		_ = s.prov.DeleteTenant(ctx, sysActor, tenant.ID)
		_ = s.repo.Unclaim(ctx, row.ID)
		if errors.Is(err, domain.ErrConflict) {
			return nil, ErrInvalid // email sudah dipakai akun lain sejak pendaftaran
		}
		return nil, err
	}
	_ = s.activity.Record(ctx, &domain.ActivityLog{TenantID: &tenant.ID, Action: "SIGNUP_VERIFIED", EntityType: "tenant", EntityID: &tenant.ID})
	if s.alerts != nil {
		tid := tenant.ID
		_ = s.alerts.Notify(ctx, domain.NotificationEvent{
			TypeCode: domain.NotifNewTenantSignup, TenantID: &tid,
			Title:      "Tenant baru mendaftar",
			Body:       fmt.Sprintf("Tenant %q (kode %s) selesai mendaftar mandiri dan terverifikasi.", tenant.Name, tenant.Code),
			Link:       "/administration",
			EntityType: "tenant", EntityID: &tid,
		})
	}
	return &VerifyResult{TenantCode: tenant.Code, TenantName: tenant.Name, Email: row.Email}, nil
}

// createTenant membuat tenant (mengulang kode bila bentrok) lalu menerapkan batas paket awal.
func (s *Service) createTenant(ctx context.Context, row *domain.TenantSignup) (*domain.Tenant, error) {
	code := row.TenantCode
	var tenant *domain.Tenant
	var err error
	for attempt := 0; attempt < 4; attempt++ {
		tenant, err = s.prov.CreateTenant(ctx, sysActor, iam.CreateTenantInput{Code: code, Name: row.CompanyName})
		if err == nil {
			break
		}
		if !errors.Is(err, domain.ErrConflict) {
			return nil, err
		}
		code = tenantCode(row.CompanyName)
	}
	if err != nil {
		return nil, err
	}
	rollback := func(e error) (*domain.Tenant, error) {
		_ = s.prov.DeleteTenant(ctx, sysActor, tenant.ID)
		return nil, e
	}
	if s.cfg.MaxPendingTasks > 0 {
		v := s.cfg.MaxPendingTasks
		if err := s.prov.SetTaskQuota(ctx, sysActor, tenant.ID, &v); err != nil {
			return rollback(err)
		}
	}
	if s.cfg.MaxDevices > 0 {
		v := s.cfg.MaxDevices
		if err := s.prov.SetDeviceLimit(ctx, sysActor, tenant.ID, &v); err != nil {
			return rollback(err)
		}
	}
	return tenant, nil
}

// Purge menghapus antrean verifikasi lama (kedaluwarsa > 7 hari, terverifikasi > 30 hari).
func (s *Service) Purge(ctx context.Context) (int64, error) {
	now := s.now()
	return s.repo.PurgeOld(ctx, now.AddDate(0, 0, -7), now.AddDate(0, 0, -30))
}
