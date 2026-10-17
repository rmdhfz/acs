package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"acs/internal/domain"
)

// Impersonate — SUPERADMIN "masuk sebagai" pengguna tenant untuk dukungan/diagnosis (SaaS support).
//
// Kontrol keamanan:
//   - hanya SUPERADMIN asli (bukan sesi impersonate lain, bukan API token);
//   - target tidak boleh SUPERADMIN (tak ada eskalasi/lateral), harus aktif & tenant-nya aktif;
//   - token pendek (maks. impersonateTTL), membawa klaim imp = id SUPERADMIN, sehingga TIDAK bisa diperpanjang;
//   - token mati seketika bila SUPERADMIN asli dinonaktifkan/dicabut sesinya (lihat checkImpersonator);
//   - handler memblokir aksi sensitif selama impersonate (ganti password, MFA, token API, impersonate lagi);
//   - mulai & akhir tercatat di audit trail, dan setiap aksi audit selama impersonate memuat impersonated_by.
const impersonateTTL = 30 * time.Minute

var ErrImpersonationForbidden = errors.New("auth: impersonate tidak diizinkan untuk akun ini")

type ImpersonationResult struct {
	Token     string
	User      *domain.User
	ExpiresAt time.Time
}

func (s *Service) Impersonate(ctx context.Context, actor domain.Actor, targetID uint64) (*ImpersonationResult, error) {
	if !actor.IsSuperadmin() || actor.ImpersonatorID != 0 {
		return nil, domain.ErrForbidden
	}
	if targetID == actor.UserID {
		return nil, fmt.Errorf("%w: tidak dapat meng-impersonate diri sendiri", domain.ErrInvalidInput)
	}
	target, err := s.users.GetByID(ctx, targetID)
	if err != nil {
		return nil, err
	}
	roles, err := s.users.RolesByUserID(ctx, targetID)
	if err != nil {
		return nil, err
	}
	for _, r := range roles {
		if r == domain.RoleSuperadmin {
			return nil, ErrImpersonationForbidden
		}
	}
	if !target.IsActive || target.TenantID == nil {
		return nil, ErrImpersonationForbidden
	}
	if err := s.checkTenantActive(ctx, target.TenantID); err != nil {
		return nil, err
	}
	target.Roles = roles

	ttl := impersonateTTL
	if s.jwtExpiry < ttl {
		ttl = s.jwtExpiry
	}
	now := time.Now()
	exp := now.Add(ttl)
	claims := Claims{
		UserID: target.ID, TenantID: target.TenantID, Roles: roles, Imp: actor.UserID,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   target.UserUUID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
		},
	}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.jwtSecret)
	if err != nil {
		return nil, err
	}
	_ = s.activity.Record(ctx, &domain.ActivityLog{
		UserID: actor.UserIDPtr(), TenantID: target.TenantID, Action: "IMPERSONATE_START", EntityType: "user", EntityID: &target.ID,
		Details: map[string]any{"target_username": target.Username, "ttl_minutes": int(ttl.Minutes())},
	})
	return &ImpersonationResult{Token: tok, User: target, ExpiresAt: exp}, nil
}

// EndImpersonation mencatat akhir sesi impersonate (token dibuang klien; tidak ada state server).
func (s *Service) EndImpersonation(ctx context.Context, actor domain.Actor) error {
	if actor.ImpersonatorID == 0 {
		return domain.ErrForbidden
	}
	imp := actor.ImpersonatorID
	_ = s.activity.Record(ctx, &domain.ActivityLog{
		UserID: &imp, TenantID: actor.TenantID, Action: "IMPERSONATE_END", EntityType: "user", EntityID: &actor.UserID,
	})
	return nil
}

// checkImpersonator — token impersonate hanya sah selama SUPERADMIN asli masih aktif dan sesinya belum dicabut.
func (s *Service) checkImpersonator(ctx context.Context, claims *Claims) error {
	st, err := s.users.AuthState(ctx, claims.Imp)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return errors.New("impersonator tidak ada")
		}
		return fmt.Errorf("%w: %v", errSessionLookup, err)
	}
	if !st.IsActive {
		return errors.New("impersonator nonaktif")
	}
	if st.PasswordChangedAt != nil && claims.IssuedAt != nil &&
		claims.IssuedAt.Time.Before(st.PasswordChangedAt.Truncate(time.Second)) {
		return errors.New("sesi impersonator dicabut")
	}
	return nil
}
