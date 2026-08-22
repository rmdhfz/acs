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
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"acs/internal/domain"
)

var ErrInvalidCredentials = errors.New("auth: username atau password salah")

type Claims struct {
	UserID   uint64   `json:"uid"`
	TenantID *uint64  `json:"tid,omitempty"`
	Roles    []string `json:"roles"`
	jwt.RegisteredClaims
}

type Service struct {
	users     domain.UserRepository
	tokens    domain.APITokenRepository
	activity  domain.ActivityLogRepository
	jwtSecret []byte
	jwtExpiry time.Duration
}

func NewService(users domain.UserRepository, tokens domain.APITokenRepository, activity domain.ActivityLogRepository, jwtSecret []byte, jwtExpiry time.Duration) *Service {
	return &Service{users: users, tokens: tokens, activity: activity, jwtSecret: jwtSecret, jwtExpiry: jwtExpiry}
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
	if !u.IsActive {
		return nil, "", ErrInvalidCredentials
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return nil, "", ErrInvalidCredentials
	}
	token, err := s.issueJWT(u)
	if err != nil {
		return nil, "", err
	}
	_ = s.users.TouchLastLogin(ctx, u.ID, time.Now())
	_ = s.activity.Record(ctx, &domain.ActivityLog{UserID: &u.ID, TenantID: u.TenantID, Action: "LOGIN", EntityType: "user", EntityID: &u.ID})
	return u, token, nil
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
