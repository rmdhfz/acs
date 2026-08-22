package domain

import (
	"context"
	"time"
)

type Tenant struct {
	ID         uint64 `db:"id" json:"id"`
	TenantUUID string `db:"tenant_uuid" json:"tenant_uuid"`
	Code       string `db:"code" json:"code"`
	Name       string `db:"name" json:"name"`
	IsActive   bool   `db:"is_active" json:"is_active"`
	// CWMPInformUsername/PasswordEnc adalah shared secret Inform CWMP tenant
	// ini — dipakai memvalidasi Inform dari device yang belum pernah tercatat
	// (lihat usecase/session). PasswordEnc tidak pernah diekspos ke JSON.
	CWMPInformUsername    *string `db:"cwmp_inform_username" json:"cwmp_inform_username"`
	CWMPInformPasswordEnc []byte  `db:"cwmp_inform_password_enc" json:"-"`
	Audit
}

type TenantRepository interface {
	Create(ctx context.Context, t *Tenant) error
	GetByID(ctx context.Context, id uint64) (*Tenant, error)
	GetByUUID(ctx context.Context, uuid string) (*Tenant, error)
	// GetByCWMPInformUsername dipakai usecase/session untuk resolve tenant
	// dari shared secret Inform saat device belum dikenal.
	GetByCWMPInformUsername(ctx context.Context, username string) (*Tenant, error)
	List(ctx context.Context, p Pagination) ([]Tenant, int, error)
	Update(ctx context.Context, t *Tenant) error
	SetCWMPInformCredentials(ctx context.Context, id uint64, username string, passwordEnc []byte, updatedBy *uint64) error
	SoftDelete(ctx context.Context, id, deletedBy uint64) error
}

type User struct {
	ID           uint64   `db:"id" json:"id"`
	UserUUID     string   `db:"user_uuid" json:"user_uuid"`
	TenantID     *uint64  `db:"tenant_id" json:"tenant_id"`
	Username     string   `db:"username" json:"username"`
	Email        string   `db:"email" json:"email"`
	PasswordHash string   `db:"password_hash" json:"-"`
	FullName     string   `db:"full_name" json:"full_name"`
	IsActive     bool     `db:"is_active" json:"is_active"`
	LastLoginAt  *time.Time `db:"last_login_at" json:"last_login_at"`
	Roles        []string `db:"-" json:"roles"`
	Audit
}

type UserRepository interface {
	Create(ctx context.Context, u *User) error
	GetByID(ctx context.Context, id uint64) (*User, error)
	GetByUsername(ctx context.Context, username string) (*User, error)
	List(ctx context.Context, tenantID *uint64, p Pagination) ([]User, int, error)
	Update(ctx context.Context, u *User) error
	SoftDelete(ctx context.Context, id, deletedBy uint64) error
	TouchLastLogin(ctx context.Context, id uint64, at time.Time) error
	RolesByUserID(ctx context.Context, userID uint64) ([]string, error)
	AssignRole(ctx context.Context, userID, roleID uint64, createdBy *uint64) error
	RevokeRole(ctx context.Context, userID, roleID uint64) error
}

// APIToken adalah token API internal (lihat CLAUDE.md - tidak ada endpoint
// mutasi tanpa autentikasi). TokenHash disimpan hash-nya saja (bukan token asli).
type APIToken struct {
	ID        uint64     `db:"id" json:"id"`
	TokenUUID string     `db:"token_uuid" json:"token_uuid"`
	UserID    *uint64    `db:"user_id" json:"user_id"`
	TenantID  *uint64    `db:"tenant_id" json:"tenant_id"`
	Name      string     `db:"name" json:"name"`
	TokenHash string     `db:"token_hash" json:"-"`
	Scopes    []byte     `db:"scopes" json:"scopes"`
	ExpiresAt *time.Time `db:"expires_at" json:"expires_at"`
	RevokedAt *time.Time `db:"revoked_at" json:"revoked_at"`
	CreatedAt time.Time  `db:"created_at" json:"created_at"`
	CreatedBy *uint64    `db:"created_by" json:"created_by"`
}

type APITokenRepository interface {
	Create(ctx context.Context, t *APIToken) error
	GetByHash(ctx context.Context, tokenHash string) (*APIToken, error)
	Revoke(ctx context.Context, id uint64) error
	ListByUser(ctx context.Context, userID uint64) ([]APIToken, error)
}

type ActivityLog struct {
	ID          uint64    `db:"id" json:"id"`
	UserID      *uint64   `db:"user_id" json:"user_id"`
	TenantID    *uint64   `db:"tenant_id" json:"tenant_id"`
	Action      string    `db:"action" json:"action"`
	EntityType  string    `db:"entity_type" json:"entity_type"`
	EntityID    *uint64   `db:"entity_id" json:"entity_id"`
	Description *string   `db:"description" json:"description"`
	IPAddress   *string   `db:"ip_address" json:"ip_address"`
	CreatedAt   time.Time `db:"created_at" json:"created_at"`
	// Username — hasil LEFT JOIN users di ListByEntity (bukan kolom asli
	// activity_logs), nil bila UserID nil (aksi sistem, mis. evaluasi ZTP
	// otomatis) atau user-nya sudah dihapus.
	Username *string `db:"username" json:"username"`
}

type ActivityLogRepository interface {
	Record(ctx context.Context, log *ActivityLog) error
	ListByEntity(ctx context.Context, entityType string, entityID uint64, p Pagination) ([]ActivityLog, int, error)
}
