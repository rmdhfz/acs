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
	// BrandName/LogoURL/PrimaryColor — white-labeling (ROADMAP.md Fase 2).
	// LogoURL adalah URL eksternal (bukan upload lewat ACS).
	BrandName    *string `db:"brand_name" json:"brand_name"`
	LogoURL      *string `db:"logo_url" json:"logo_url"`
	PrimaryColor *string `db:"primary_color" json:"primary_color"`
	// MaxPendingTasks — kuota task queue per tenant (ROADMAP.md Fase 2, migrations/0005).
	// NULL = tidak dibatasi. HANYA membatasi jumlah task PENDING, bukan rate
	// limit koneksi/sesi CWMP (lihat usecase/task.Service.CreateTask).
	MaxPendingTasks *uint32 `db:"max_pending_tasks" json:"max_pending_tasks"`
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
	UpdateBranding(ctx context.Context, id uint64, brandName, logoURL, primaryColor *string, updatedBy *uint64) error
	// SetTaskQuota mengubah batas task PENDING tenant (nil = tidak dibatasi).
	// Kebijakan platform-level, superadmin only (lihat usecase/iam.SetTaskQuota).
	SetTaskQuota(ctx context.Context, id uint64, maxPendingTasks *uint32, updatedBy *uint64) error
	SoftDelete(ctx context.Context, id, deletedBy uint64) error
}

type User struct {
	ID           uint64     `db:"id" json:"id"`
	UserUUID     string     `db:"user_uuid" json:"user_uuid"`
	TenantID     *uint64    `db:"tenant_id" json:"tenant_id"`
	Username     string     `db:"username" json:"username"`
	Email        string     `db:"email" json:"email"`
	PasswordHash string     `db:"password_hash" json:"-"`
	FullName     string     `db:"full_name" json:"full_name"`
	IsActive     bool       `db:"is_active" json:"is_active"`
	LastLoginAt  *time.Time `db:"last_login_at" json:"last_login_at"`
	// FailedLoginAttempts/LockedUntil -- proteksi brute-force login
	// (migrations/0008, usecase/auth.Service.Login). Tidak diekspos ke JSON
	// (json:"-"): ini state keamanan internal, bukan sesuatu yang perlu
	// ditampilkan lewat API listing user manapun.
	FailedLoginAttempts uint32     `db:"failed_login_attempts" json:"-"`
	LockedUntil         *time.Time `db:"locked_until" json:"-"`
	Roles               []string   `db:"-" json:"roles"`
	Audit
}

type UserRepository interface {
	Create(ctx context.Context, u *User) error
	GetByID(ctx context.Context, id uint64) (*User, error)
	GetByUsername(ctx context.Context, username string) (*User, error)
	List(ctx context.Context, tenantID *uint64, p Pagination) ([]User, int, error)
	Update(ctx context.Context, u *User) error
	// UpdatePassword mengganti password_hash user (admin-reset atau ganti
	// password sendiri) — terpisah dari Update karena Update tidak menyentuh
	// kolom password_hash (lihat repository/mysql/tenancy_repository.go).
	UpdatePassword(ctx context.Context, id uint64, passwordHash string, updatedBy *uint64) error
	SoftDelete(ctx context.Context, id, deletedBy uint64) error
	TouchLastLogin(ctx context.Context, id uint64, at time.Time) error
	RolesByUserID(ctx context.Context, userID uint64) ([]string, error)
	AssignRole(ctx context.Context, userID, roleID uint64, createdBy *uint64) error
	RevokeRole(ctx context.Context, userID, roleID uint64) error
	// RecordFailedLogin menaikkan failed_login_attempts DAN (kalau nilai
	// setelah increment >= maxAttempts) sekaligus mengunci akun sampai
	// `lockUntil`, sebagai SATU pernyataan UPDATE atomik. SENGAJA digabung,
	// bukan dua method terpisah (increment lalu lock) -- versi dua-langkah
	// sebelumnya py celah TOCTOU nyata: beberapa request login paralel utk
	// username yang sama semuanya bisa membaca `locked_until` yang masih
	// kosong sebelum salah satu sempat menerapkan lock, sehingga penyerang
	// efektif dapat lebih dari `maxAttempts` percobaan gratis per batch
	// paralel (ditemukan acs-security-reviewer). UPDATE tunggal ini
	// memanfaatkan row-level locking MariaDB: increment & keputusan lock
	// dihitung dari nilai baris SETELAH lock diperoleh, bukan dari snapshot
	// baca terpisah sebelumnya.
	RecordFailedLogin(ctx context.Context, id uint64, maxAttempts uint32, lockUntil time.Time) error
	// ResetLoginLockout mengembalikan failed_login_attempts ke 0 dan
	// mengosongkan locked_until -- dipanggil saat login berhasil ATAU saat
	// admin mereset password user lain (ResetUserPassword, usecase/iam):
	// reset password = kasih kesempatan baru, jadi lockout lama tidak relevan lagi.
	ResetLoginLockout(ctx context.Context, id uint64) error
}

// APIToken adalah token API internal (lihat CLAUDE.md - tidak ada endpoint
// mutasi tanpa autentikasi). TokenHash disimpan hash-nya saja (bukan token asli).
//
// UpdatedAt/UpdatedBy WAJIB ada di sini krn kolomnya memang ada di tabel
// api_tokens (schema.sql) -- tanpa keduanya, SEMUA query `SELECT *` di
// apiTokenRepository (termasuk GetByHash yang dipakai jalur autentikasi API
// token pada SETIAP request) gagal dgn "missing destination name updated_at",
// membuat autentikasi via API token acs_... TIDAK PERNAH berfungsi sama
// sekali terhadap skema nyata -- baru ketahuan saat live-test end-to-end
// (ResolveActor membungkus error ini jadi generik "invalid token" 401,
// menutupi akar masalahnya).
//
// Scopes bertipe JSONRawMessage (bukan []byte polos) mengikuti pola
// domain.JSONRawMessage (lihat domain/common.go) -- kolomnya JSON di skema,
// sama seperti Task.Parameters/Response, meski saat ini belum ada jalur yang
// mengisinya (IssueAPIToken belum expose scopes granular).
type APIToken struct {
	ID        uint64         `db:"id" json:"id"`
	TokenUUID string         `db:"token_uuid" json:"token_uuid"`
	UserID    *uint64        `db:"user_id" json:"user_id"`
	TenantID  *uint64        `db:"tenant_id" json:"tenant_id"`
	Name      string         `db:"name" json:"name"`
	TokenHash string         `db:"token_hash" json:"-"`
	Scopes    JSONRawMessage `db:"scopes" json:"scopes"`
	ExpiresAt *time.Time     `db:"expires_at" json:"expires_at"`
	RevokedAt *time.Time     `db:"revoked_at" json:"revoked_at"`
	CreatedAt time.Time      `db:"created_at" json:"created_at"`
	CreatedBy *uint64        `db:"created_by" json:"created_by"`
	UpdatedAt time.Time      `db:"updated_at" json:"updated_at"`
	UpdatedBy *uint64        `db:"updated_by" json:"updated_by"`
}

type APITokenRepository interface {
	Create(ctx context.Context, t *APIToken) error
	GetByHash(ctx context.Context, tokenHash string) (*APIToken, error)
	// GetByID dipakai RevokeAPIToken utk resolve tenant pemilik token SEBELUM
	// mencabutnya (RBAC scope tenant, sama pola dgn requireUserMutationScope
	// di usecase/iam) -- BEDA dari GetByHash yg dipakai jalur autentikasi
	// (memfilter revoked_at/expires_at), GetByID sengaja tidak memfilter itu
	// supaya token yang sudah revoked/expired tetap bisa dilihat/diaudit.
	GetByID(ctx context.Context, id uint64) (*APIToken, error)
	Revoke(ctx context.Context, id uint64) error
	ListByUser(ctx context.Context, userID uint64) ([]APIToken, error)
	// List — utk GET /auth/tokens: tenantID nil = lintas seluruh tenant (HANYA
	// valid dipanggil dgn actor superadmin, lihat auth.ScopedTenantFilter),
	// non-nil = scoped ke satu tenant (ADMIN cuma lihat token tenant sendiri,
	// bukan cuma token yang dia terbitkan sendiri -- supaya ADMIN bisa cabut
	// token yang diterbitkan ADMIN lain di tenant yang sama, mis. saat
	// karyawan yang menerbitkan token tsb sudah resign).
	List(ctx context.Context, tenantID *uint64, p Pagination) ([]APIToken, int, error)
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
