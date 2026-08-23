// Package iam menangani tenant dan user management dasar (FR-26/27/28) —
// dibutuhkan agar sistem punya jalur untuk membuat tenant/user pertama kali,
// di luar 6 usecase inti yang didaftarkan TECH.md §2.
package iam

import (
	"context"
	"fmt"
	"net/url"
	"regexp"

	"github.com/google/uuid"

	"acs/internal/domain"
	"acs/internal/usecase/auth"
	"acs/pkg/cryptoutil"
)

// minCWMPInformPasswordLen — shared secret Inform CWMP dipakai lewat HTTP
// Basic Auth tanpa batas percobaan bawaan CWMP itu sendiri (rate limiting
// ada di layer server, bukan di sini) — panjang minimum mengurangi risiko
// brute-force pada endpoint publik /cwmp (TECH.md/PRD.md §8).
const minCWMPInformPasswordLen = 16

type Service struct {
	tenants  domain.TenantRepository
	users    domain.UserRepository
	refs     domain.RefRepository
	activity domain.ActivityLogRepository
	enc      *cryptoutil.Encryptor
}

func NewService(tenants domain.TenantRepository, users domain.UserRepository, refs domain.RefRepository, activity domain.ActivityLogRepository, enc *cryptoutil.Encryptor) *Service {
	return &Service{tenants: tenants, users: users, refs: refs, activity: activity, enc: enc}
}

type CreateTenantInput struct {
	Code string
	Name string
	// CWMPInformUsername/Password (opsional) — shared secret Inform CWMP
	// tenant ini, dipakai memvalidasi device baru sebelum ia punya kredensial
	// sendiri (lihat usecase/session). Password disimpan terenkripsi.
	CWMPInformUsername *string
	CWMPInformPassword *string
}

func (s *Service) CreateTenant(ctx context.Context, actor domain.Actor, in CreateTenantInput) (*domain.Tenant, error) {
	if err := auth.RequireRole(actor, domain.RoleSuperadmin); err != nil {
		return nil, err
	}
	var passwordEnc []byte
	if in.CWMPInformPassword != nil {
		if len(*in.CWMPInformPassword) < minCWMPInformPasswordLen {
			return nil, fmt.Errorf("%w: password Inform CWMP minimal %d karakter", domain.ErrInvalidInput, minCWMPInformPasswordLen)
		}
		enc, err := s.enc.Encrypt(*in.CWMPInformPassword)
		if err != nil {
			return nil, err
		}
		passwordEnc = enc
	}
	t := &domain.Tenant{
		TenantUUID: uuid.NewString(), Code: in.Code, Name: in.Name, IsActive: true,
		CWMPInformUsername: in.CWMPInformUsername, CWMPInformPasswordEnc: passwordEnc,
		Audit: domain.Audit{CreatedBy: actor.UserIDPtr()},
	}
	if err := s.tenants.Create(ctx, t); err != nil {
		return nil, err
	}
	_ = s.activity.Record(ctx, &domain.ActivityLog{UserID: actor.UserIDPtr(), Action: "CREATE_TENANT", EntityType: "tenant", EntityID: &t.ID})
	return t, nil
}

// SetCWMPInformCredentials meng-set/rotate shared secret Inform CWMP tenant
// (superadmin only — kredensial sensitif lintas seluruh device tenant ini).
func (s *Service) SetCWMPInformCredentials(ctx context.Context, actor domain.Actor, tenantID uint64, username, password string) error {
	if err := auth.RequireRole(actor, domain.RoleSuperadmin); err != nil {
		return err
	}
	if len(password) < minCWMPInformPasswordLen {
		return fmt.Errorf("%w: password Inform CWMP minimal %d karakter", domain.ErrInvalidInput, minCWMPInformPasswordLen)
	}
	passwordEnc, err := s.enc.Encrypt(password)
	if err != nil {
		return err
	}
	if err := s.tenants.SetCWMPInformCredentials(ctx, tenantID, username, passwordEnc, actor.UserIDPtr()); err != nil {
		return err
	}
	_ = s.activity.Record(ctx, &domain.ActivityLog{UserID: actor.UserIDPtr(), Action: "SET_TENANT_CWMP_CREDENTIALS", EntityType: "tenant", EntityID: &tenantID})
	return nil
}

func (s *Service) ListTenants(ctx context.Context, actor domain.Actor, p domain.Pagination) ([]domain.Tenant, int, error) {
	if err := auth.RequireRole(actor, domain.RoleSuperadmin); err != nil {
		return nil, 0, err
	}
	return s.tenants.List(ctx, p)
}

// GetCurrentTenant — dipanggil frontend (semua role) utk resolve branding
// (ROADMAP.md Fase 2) yang harus ditampilkan. Actor tanpa tenant (superadmin
// global) mengembalikan nil, nil — frontend fallback ke branding default
// "ACS Console", bukan error.
func (s *Service) GetCurrentTenant(ctx context.Context, actor domain.Actor) (*domain.Tenant, error) {
	if actor.TenantID == nil {
		return nil, nil
	}
	return s.tenants.GetByID(ctx, *actor.TenantID)
}

var hexColorRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// validateBranding — kolom ini bebas diisi lewat API langsung (bukan cuma
// lewat form frontend yang punya guard-nya sendiri), jadi divalidasi lagi di
// sini. Tanpa ini, nilai yang melebihi lebar kolom (primary_color CHAR(7),
// logo_url VARCHAR(512)) jatuh sbg ER_DATA_TOO_LONG dari MariaDB yang tidak
// dipetakan translateErr -> balik sbg 500 generik, bukan 400 informatif.
func validateBranding(brandName, logoURL, primaryColor *string) error {
	if brandName != nil && len(*brandName) > 128 {
		return fmt.Errorf("%w: nama brand maksimal 128 karakter", domain.ErrInvalidInput)
	}
	if logoURL != nil {
		u, err := url.Parse(*logoURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return fmt.Errorf("%w: URL logo harus http/https yang valid", domain.ErrInvalidInput)
		}
		if len(*logoURL) > 512 {
			return fmt.Errorf("%w: URL logo maksimal 512 karakter", domain.ErrInvalidInput)
		}
	}
	if primaryColor != nil && !hexColorRe.MatchString(*primaryColor) {
		return fmt.Errorf("%w: warna aksen harus format hex #rrggbb", domain.ErrInvalidInput)
	}
	return nil
}

// UpdateBranding — superadmin bisa utk tenant manapun; ADMIN hanya utk
// tenant-nya sendiri (self-service, ROADMAP.md Fase 2, pola sama dgn
// CreateUser). Field nil berarti "tidak diubah" tidak berlaku di sini --
// ini full replace (form kirim state lengkap); nil eksplisit = kembali ke
// default (kosongkan kolom).
func (s *Service) UpdateBranding(ctx context.Context, actor domain.Actor, tenantID uint64, brandName, logoURL, primaryColor *string) error {
	if err := auth.RequireRole(actor, domain.RoleAdmin); err != nil {
		return err
	}
	if err := auth.RequireTenantScope(actor, &tenantID); err != nil {
		return err
	}
	if err := validateBranding(brandName, logoURL, primaryColor); err != nil {
		return err
	}
	if err := s.tenants.UpdateBranding(ctx, tenantID, brandName, logoURL, primaryColor, actor.UserIDPtr()); err != nil {
		return err
	}
	_ = s.activity.Record(ctx, &domain.ActivityLog{UserID: actor.UserIDPtr(), Action: "UPDATE_TENANT_BRANDING", EntityType: "tenant", EntityID: &tenantID})
	return nil
}

type CreateUserInput struct {
	TenantID  *uint64
	Username  string
	Email     string
	Password  string
	FullName  string
	RoleCodes []string
}

// CreateUser: superadmin bisa membuat user di tenant manapun (atau lintas
// tenant untuk sesama superadmin); admin tenant hanya bisa membuat user di
// tenant-nya sendiri (RBAC scope tenant, CLAUDE.md).
func (s *Service) CreateUser(ctx context.Context, actor domain.Actor, in CreateUserInput) (*domain.User, error) {
	if !actor.IsSuperadmin() {
		if err := auth.RequireRole(actor, domain.RoleAdmin); err != nil {
			return nil, err
		}
		if in.TenantID == nil || actor.TenantID == nil || *in.TenantID != *actor.TenantID {
			return nil, domain.ErrForbidden
		}
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		return nil, err
	}
	u := &domain.User{
		UserUUID: uuid.NewString(), TenantID: in.TenantID, Username: in.Username, Email: in.Email,
		PasswordHash: hash, FullName: in.FullName, IsActive: true, Audit: domain.Audit{CreatedBy: actor.UserIDPtr()},
	}
	if err := s.users.Create(ctx, u); err != nil {
		return nil, err
	}
	for _, code := range in.RoleCodes {
		role, err := s.refs.GetByCode(ctx, domain.RefTableRoles, code)
		if err != nil {
			continue
		}
		_ = s.users.AssignRole(ctx, u.ID, role.ID, actor.UserIDPtr())
	}
	u.Roles = in.RoleCodes
	_ = s.activity.Record(ctx, &domain.ActivityLog{UserID: actor.UserIDPtr(), TenantID: in.TenantID, Action: "CREATE_USER", EntityType: "user", EntityID: &u.ID})
	return u, nil
}

func (s *Service) ListUsers(ctx context.Context, actor domain.Actor, tenantID *uint64, p domain.Pagination) ([]domain.User, int, error) {
	if !actor.IsSuperadmin() {
		tenantID = actor.TenantID
	}
	return s.users.List(ctx, tenantID, p)
}
