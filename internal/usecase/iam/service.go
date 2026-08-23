// Package iam menangani tenant dan user management dasar (FR-26/27/28) —
// dibutuhkan agar sistem punya jalur untuk membuat tenant/user pertama kali,
// di luar 6 usecase inti yang didaftarkan TECH.md §2.
package iam

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"

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

// UpdateTenantInput -- partial update (field nil = tidak diubah), pola sama
// dgn UpdateUserInput. IsActive adalah field utama motivasi endpoint ini
// (ROADMAP.md: "tidak ada endpoint untuk menonaktifkan tenant") -- Code/Name
// ikut diekspos sekalian krn TenantRepository.Update yang sudah ada memang
// full-column, bukan krn diminta terpisah.
type UpdateTenantInput struct {
	Code     *string
	Name     *string
	IsActive *bool
}

// UpdateTenant -- superadmin only (kebijakan platform-level, sama seperti
// CreateTenant/SetTaskQuota; BEDA dari UpdateBranding yang admin tenant boleh
// self-service). PENTING soal dampak menonaktifkan tenant: user milik tenant
// yang di-set IsActive=false TIDAK BISA login lagi setelah ini (usecase/auth.
// Service.checkTenantActive, dipanggil dari Login DAN ResolveActor) -- bearer
// token/JWT yang SUDAH diterbitkan sebelumnya pun langsung ditolak di request
// berikutnya (ResolveActor dipanggil di setiap request lewat AuthMiddleware),
// bukan menunggu sampai token itu expire. Lihat komentar lengkap di
// checkTenantActive/ResolveActor soal trade-off query tambahan per request.
func (s *Service) UpdateTenant(ctx context.Context, actor domain.Actor, tenantID uint64, in UpdateTenantInput) (*domain.Tenant, error) {
	if err := auth.RequireRole(actor, domain.RoleSuperadmin); err != nil {
		return nil, err
	}
	// createTenant menolak code/name kosong (lihat CreateTenant di file ini);
	// UpdateTenant harus konsisten -- tanpa ini, PATCH {"code":""} lolos ke
	// UPDATE (kolom NOT NULL tapi VARCHAR kosong tetap valid secara SQL,
	// bukan error) dan mengosongkan data secara diam-diam (temuan
	// acs-code-reviewer).
	if in.Code != nil && *in.Code == "" {
		return nil, fmt.Errorf("%w: code tidak boleh kosong", domain.ErrInvalidInput)
	}
	if in.Name != nil && *in.Name == "" {
		return nil, fmt.Errorf("%w: name tidak boleh kosong", domain.ErrInvalidInput)
	}
	t, err := s.tenants.GetByID(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if in.Code != nil {
		t.Code = *in.Code
	}
	if in.Name != nil {
		t.Name = *in.Name
	}
	if in.IsActive != nil {
		t.IsActive = *in.IsActive
	}
	t.UpdatedBy = actor.UserIDPtr()
	if err := s.tenants.Update(ctx, t); err != nil {
		return nil, err
	}
	_ = s.activity.Record(ctx, &domain.ActivityLog{UserID: actor.UserIDPtr(), Action: "UPDATE_TENANT", EntityType: "tenant", EntityID: &t.ID})
	return t, nil
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

// SetTaskQuota mengubah batas task PENDING sebuah tenant (nil = tidak
// dibatasi). BEDA dari UpdateBranding: ini kebijakan platform-level (kuota
// resource bersama, ROADMAP.md Fase 2), bukan self-service tenant -- hanya
// superadmin yang boleh, ADMIN tenant sendiri sekalipun tidak bisa menaikkan
// kuotanya sendiri lewat endpoint ini.
func (s *Service) SetTaskQuota(ctx context.Context, actor domain.Actor, tenantID uint64, maxPendingTasks *uint32) error {
	if err := auth.RequireRole(actor, domain.RoleSuperadmin); err != nil {
		return err
	}
	if err := s.tenants.SetTaskQuota(ctx, tenantID, maxPendingTasks, actor.UserIDPtr()); err != nil {
		return err
	}
	_ = s.activity.Record(ctx, &domain.ActivityLog{UserID: actor.UserIDPtr(), Action: "SET_TENANT_TASK_QUOTA", EntityType: "tenant", EntityID: &tenantID})
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

// validateRoleCodesAssignable menolak actor non-superadmin yang mencoba
// memberikan role SUPERADMIN ke user manapun (dirinya sendiri atau orang
// lain) — hanya superadmin yang boleh membuat/menetapkan superadmin lain.
// Dipakai bersama oleh CreateUser dan ReplaceUserRoles supaya guard privilege-
// escalation ini tidak terduplikasi dan tidak bisa lolos lewat salah satu
// jalur (temuan audit keamanan: CreateUser sebelumnya tidak memvalidasi
// RoleCodes sama sekali terhadap level actor, ADMIN tenant mana pun bisa
// membuat akun SUPERADMIN permanen via role_codes saat POST /users).
func validateRoleCodesAssignable(actor domain.Actor, roleCodes []string) error {
	if actor.IsSuperadmin() {
		return nil
	}
	// strings.EqualFold, BUKAN slices.Contains (exact-case) -- kolom
	// ref_roles.code memakai kolasi default tabel (utf8mb4_unicode_ci),
	// case-INSENSITIVE. GetByCode akan tetap me-resolve "superadmin"/
	// "SuperAdmin" ke role SUPERADMIN yang sama; exact-case check di sini
	// gampang dilewati cuma dengan ganti casing di request JSON (celah
	// ditemukan acs-security-reviewer -- exact-case check sebelumnya
	// memberi rasa aman palsu, bukan proteksi nyata).
	for _, code := range roleCodes {
		if strings.EqualFold(code, domain.RoleSuperadmin) {
			return fmt.Errorf("%w: hanya superadmin yang boleh memberikan role superadmin", domain.ErrForbidden)
		}
	}
	return nil
}

// CreateUser: superadmin bisa membuat user di tenant manapun (atau lintas
// tenant untuk sesama superadmin); admin tenant hanya bisa membuat user di
// tenant-nya sendiri (RBAC scope tenant, CLAUDE.md). ADMIN non-superadmin juga
// tidak boleh membuat user dengan role SUPERADMIN (lihat validateRoleCodesAssignable).
func (s *Service) CreateUser(ctx context.Context, actor domain.Actor, in CreateUserInput) (*domain.User, error) {
	if !actor.IsSuperadmin() {
		if err := auth.RequireRole(actor, domain.RoleAdmin); err != nil {
			return nil, err
		}
		if in.TenantID == nil || actor.TenantID == nil || *in.TenantID != *actor.TenantID {
			return nil, domain.ErrForbidden
		}
	}
	if err := validateRoleCodesAssignable(actor, in.RoleCodes); err != nil {
		return nil, err
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
		tid, err := auth.ScopedTenantFilter(actor)
		if err != nil {
			return nil, 0, err
		}
		tenantID = tid
	}
	return s.users.List(ctx, tenantID, p)
}

// requireUserMutationScope resolve user target lalu memastikan actor boleh
// memutasi-nya: superadmin bebas; ADMIN hanya ke user satu tenant dengan
// dirinya (pola sama dgn CreateUser/UpdateBranding, CLAUDE.md RBAC scope
// tenant). Dipakai bersama oleh UpdateUser/ResetUserPassword/ReplaceUserRoles/
// DeleteUser agar keempatnya konsisten (existence-check dulu baru scope,
// sama seperti provisioning.UpdateProfile).
func (s *Service) requireUserMutationScope(ctx context.Context, actor domain.Actor, targetID uint64) (*domain.User, error) {
	target, err := s.users.GetByID(ctx, targetID)
	if err != nil {
		return nil, err
	}
	if !actor.IsSuperadmin() {
		if err := auth.RequireRole(actor, domain.RoleAdmin); err != nil {
			return nil, err
		}
		if err := auth.RequireTenantScope(actor, target.TenantID); err != nil {
			return nil, err
		}
	}
	return target, nil
}

type UpdateUserInput struct {
	FullName *string
	Email    *string
	IsActive *bool
}

// UpdateUser — partial update (field nil = tidak diubah). RBAC sama dengan
// CreateUser. Guard tambahan: actor tidak boleh menonaktifkan akun dirinya
// sendiri lewat endpoint ini (self-lockout) — kalau itu satu-satunya ADMIN
// aktif di tenant, tenant itu kehilangan seluruh akses admin tanpa jalan
// keluar selain intervensi superadmin/DB langsung.
func (s *Service) UpdateUser(ctx context.Context, actor domain.Actor, targetID uint64, in UpdateUserInput) (*domain.User, error) {
	target, err := s.requireUserMutationScope(ctx, actor, targetID)
	if err != nil {
		return nil, err
	}
	if targetID == actor.UserID && in.IsActive != nil && !*in.IsActive {
		return nil, fmt.Errorf("%w: tidak bisa menonaktifkan akun sendiri", domain.ErrForbidden)
	}
	if in.FullName != nil {
		target.FullName = *in.FullName
	}
	if in.Email != nil {
		target.Email = *in.Email
	}
	if in.IsActive != nil {
		target.IsActive = *in.IsActive
	}
	target.UpdatedBy = actor.UserIDPtr()
	if err := s.users.Update(ctx, target); err != nil {
		return nil, err
	}
	_ = s.activity.Record(ctx, &domain.ActivityLog{UserID: actor.UserIDPtr(), TenantID: target.TenantID, Action: "UPDATE_USER", EntityType: "user", EntityID: &target.ID})
	return target, nil
}

// ResetUserPassword — admin-reset password user lain (bukan "ganti password
// sendiri via password lama", itu di luar scope ini). RBAC sama dengan
// UpdateUser. Password baru tidak pernah dicatat ke activity_logs (hanya aksi
// + entity, CLAUDE.md §8 — kredensial tidak boleh muncul di log).
func (s *Service) ResetUserPassword(ctx context.Context, actor domain.Actor, targetID uint64, newPassword string) error {
	target, err := s.requireUserMutationScope(ctx, actor, targetID)
	if err != nil {
		return err
	}
	if len(newPassword) < domain.MinUserPasswordLen {
		return fmt.Errorf("%w: password minimal %d karakter", domain.ErrInvalidInput, domain.MinUserPasswordLen)
	}
	hash, err := auth.HashPassword(newPassword)
	if err != nil {
		return err
	}
	if err := s.users.UpdatePassword(ctx, target.ID, hash, actor.UserIDPtr()); err != nil {
		return err
	}
	// Reset lockout brute-force (failed_login_attempts/locked_until) sekalian
	// -- admin mereset password = kasih kesempatan baru, jadi lockout lama
	// (kalau ada) tidak masuk akal dipertahankan (CLAUDE.md instruksi item 1.4).
	if err := s.users.ResetLoginLockout(ctx, target.ID); err != nil {
		return err
	}
	_ = s.activity.Record(ctx, &domain.ActivityLog{UserID: actor.UserIDPtr(), TenantID: target.TenantID, Action: "RESET_USER_PASSWORD", EntityType: "user", EntityID: &target.ID})
	return nil
}

// ReplaceUserRoles — full-replace: user akan PERSIS punya role-role di
// roleCodes setelah ini (role lain yang tidak disebut dicabut). RBAC sama
// dengan UpdateUser, plus dua guard privilege-escalation/self-lockout:
//  1. ADMIN non-superadmin tidak boleh assign role SUPERADMIN ke siapa pun —
//     hanya superadmin yang boleh membuat superadmin lain.
//  2. ADMIN tidak boleh mencabut role ADMIN dari dirinya sendiri
//     (self-lockout, sama seperti guard IsActive di UpdateUser).
func (s *Service) ReplaceUserRoles(ctx context.Context, actor domain.Actor, targetID uint64, roleCodes []string) (*domain.User, error) {
	target, err := s.requireUserMutationScope(ctx, actor, targetID)
	if err != nil {
		return nil, err
	}
	if err := validateRoleCodesAssignable(actor, roleCodes); err != nil {
		return nil, err
	}

	// Resolve semua role KE BENTUK KANONIK dari DB dulu (fail-fast bila ada
	// code yang tidak dikenal), SEBELUM guard self-lockout & diff assign/
	// revoke di bawah. Wajib dinormalisasi -- ref_roles.code pakai kolasi
	// case-INSENSITIVE (utf8mb4_unicode_ci), jadi "admin" dan "ADMIN" resolve
	// ke role yang sama. Tanpa normalisasi ini, guard self-lockout dan diff
	// assign/revoke di bawah membandingkan `target.Roles` (selalu kanonik
	// dari DB) terhadap `roleCodes` (casing bebas dari request) secara
	// exact-case -- ditemukan saat memperbaiki celah privilege-escalation di
	// validateRoleCodesAssignable: mengirim role yang SAMA cuma beda casing
	// (mis. mempertahankan "admin" padahal DB simpan "ADMIN") membuat diff
	// keliru mengira role itu "dicabut" lalu benar-benar memanggil
	// RevokeRole -- role hilang tanpa sengaja, bukan cuma masalah kosmetik.
	roleIDByCanonicalCode := make(map[string]uint64, len(roleCodes))
	canonicalCodes := make([]string, 0, len(roleCodes))
	for _, code := range roleCodes {
		role, err := s.refs.GetByCode(ctx, domain.RefTableRoles, code)
		if err != nil {
			return nil, fmt.Errorf("%w: role_code %q tidak dikenal", domain.ErrInvalidInput, code)
		}
		if _, seen := roleIDByCanonicalCode[role.Code]; !seen {
			canonicalCodes = append(canonicalCodes, role.Code)
		}
		roleIDByCanonicalCode[role.Code] = role.ID
	}

	if !actor.IsSuperadmin() {
		if targetID == actor.UserID && slices.Contains(target.Roles, domain.RoleAdmin) && !slices.Contains(canonicalCodes, domain.RoleAdmin) {
			return nil, fmt.Errorf("%w: tidak bisa mencabut role admin dari akun sendiri", domain.ErrForbidden)
		}
	}

	for code, id := range roleIDByCanonicalCode {
		if slices.Contains(target.Roles, code) {
			continue
		}
		if err := s.users.AssignRole(ctx, target.ID, id, actor.UserIDPtr()); err != nil {
			return nil, err
		}
	}
	for _, code := range target.Roles {
		if slices.Contains(canonicalCodes, code) {
			continue
		}
		role, err := s.refs.GetByCode(ctx, domain.RefTableRoles, code)
		if err != nil {
			continue
		}
		if err := s.users.RevokeRole(ctx, target.ID, role.ID); err != nil {
			return nil, err
		}
	}

	target.Roles = canonicalCodes
	_ = s.activity.Record(ctx, &domain.ActivityLog{UserID: actor.UserIDPtr(), TenantID: target.TenantID, Action: "REPLACE_USER_ROLES", EntityType: "user", EntityID: &target.ID})
	return target, nil
}

// DeleteUser — soft delete (SoftDelete repo, is_deleted=1). RBAC sama dengan
// UpdateUser, plus self-lockout: actor tidak boleh soft-delete akun sendiri.
func (s *Service) DeleteUser(ctx context.Context, actor domain.Actor, targetID uint64) error {
	target, err := s.requireUserMutationScope(ctx, actor, targetID)
	if err != nil {
		return err
	}
	if targetID == actor.UserID {
		return fmt.Errorf("%w: tidak bisa menghapus akun sendiri", domain.ErrForbidden)
	}
	if err := s.users.SoftDelete(ctx, target.ID, actor.UserID); err != nil {
		return err
	}
	_ = s.activity.Record(ctx, &domain.ActivityLog{UserID: actor.UserIDPtr(), TenantID: target.TenantID, Action: "DELETE_USER", EntityType: "user", EntityID: &target.ID})
	return nil
}
