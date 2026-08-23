package mysql

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"

	"acs/internal/domain"
)

// ---- Tenant ----

type tenantRepository struct{ db *sqlx.DB }

func NewTenantRepository(db *sqlx.DB) domain.TenantRepository { return &tenantRepository{db: db} }

func (r *tenantRepository) Create(ctx context.Context, t *domain.Tenant) error {
	now := time.Now()
	t.CreatedAt, t.UpdatedAt = now, now
	const q = `INSERT INTO tenants (tenant_uuid, code, name, is_active, cwmp_inform_username, cwmp_inform_password_enc, created_at, updated_at, created_by)
		VALUES (:tenant_uuid, :code, :name, :is_active, :cwmp_inform_username, :cwmp_inform_password_enc, :created_at, :updated_at, :created_by)`
	res, err := r.db.NamedExecContext(ctx, q, t)
	if err != nil {
		return translateErr(err)
	}
	id, _ := res.LastInsertId()
	t.ID = uint64(id)
	return nil
}

func (r *tenantRepository) GetByID(ctx context.Context, id uint64) (*domain.Tenant, error) {
	var t domain.Tenant
	err := r.db.GetContext(ctx, &t, `SELECT * FROM tenants WHERE id = ? AND is_deleted = 0`, id)
	if err != nil {
		return nil, translateErr(err)
	}
	return &t, nil
}

func (r *tenantRepository) GetByUUID(ctx context.Context, uuid string) (*domain.Tenant, error) {
	var t domain.Tenant
	err := r.db.GetContext(ctx, &t, `SELECT * FROM tenants WHERE tenant_uuid = ? AND is_deleted = 0`, uuid)
	if err != nil {
		return nil, translateErr(err)
	}
	return &t, nil
}

func (r *tenantRepository) GetByCWMPInformUsername(ctx context.Context, username string) (*domain.Tenant, error) {
	var t domain.Tenant
	err := r.db.GetContext(ctx, &t,
		`SELECT * FROM tenants WHERE cwmp_inform_username = ? AND is_deleted = 0 AND is_active = 1`, username)
	if err != nil {
		return nil, translateErr(err)
	}
	return &t, nil
}

func (r *tenantRepository) SetCWMPInformCredentials(ctx context.Context, id uint64, username string, passwordEnc []byte, updatedBy *uint64) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE tenants SET cwmp_inform_username = ?, cwmp_inform_password_enc = ?, updated_by = ? WHERE id = ? AND is_deleted = 0`,
		username, passwordEnc, updatedBy, id)
	return translateErr(err)
}

func (r *tenantRepository) UpdateBranding(ctx context.Context, id uint64, brandName, logoURL, primaryColor *string, updatedBy *uint64) error {
	// Existence dicek terpisah, bukan lewat RowsAffected dari UPDATE di bawah:
	// driver mysql hanya menghitung baris yang NILAINYA berubah (bukan yang
	// match WHERE) kecuali clientFoundRows diaktifkan di DSN -- submit ulang
	// branding yang sama persis (no-op) akan salah dianggap "tenant tidak
	// ditemukan" kalau kita pakai RowsAffected di sini.
	var exists bool
	if err := r.db.GetContext(ctx, &exists, `SELECT EXISTS(SELECT 1 FROM tenants WHERE id = ? AND is_deleted = 0)`, id); err != nil {
		return translateErr(err)
	}
	if !exists {
		return domain.ErrNotFound
	}
	_, err := r.db.ExecContext(ctx,
		`UPDATE tenants SET brand_name = ?, logo_url = ?, primary_color = ?, updated_by = ? WHERE id = ? AND is_deleted = 0`,
		brandName, logoURL, primaryColor, updatedBy, id)
	return translateErr(err)
}

func (r *tenantRepository) SetTaskQuota(ctx context.Context, id uint64, maxPendingTasks *uint32, updatedBy *uint64) error {
	// Existence dicek terpisah, bukan lewat RowsAffected -- pola sama seperti
	// UpdateBranding di atas: driver mysql hanya menghitung baris yang
	// NILAINYA berubah, jadi submit ulang kuota yang sama persis (no-op) akan
	// salah dianggap "tenant tidak ditemukan" kalau kita pakai RowsAffected.
	var exists bool
	if err := r.db.GetContext(ctx, &exists, `SELECT EXISTS(SELECT 1 FROM tenants WHERE id = ? AND is_deleted = 0)`, id); err != nil {
		return translateErr(err)
	}
	if !exists {
		return domain.ErrNotFound
	}
	_, err := r.db.ExecContext(ctx,
		`UPDATE tenants SET max_pending_tasks = ?, updated_by = ? WHERE id = ? AND is_deleted = 0`,
		maxPendingTasks, updatedBy, id)
	return translateErr(err)
}

func (r *tenantRepository) List(ctx context.Context, p domain.Pagination) ([]domain.Tenant, int, error) {
	var total int
	if err := r.db.GetContext(ctx, &total, `SELECT COUNT(*) FROM tenants WHERE is_deleted = 0`); err != nil {
		return nil, 0, translateErr(err)
	}
	var rows []domain.Tenant
	err := r.db.SelectContext(ctx, &rows,
		`SELECT * FROM tenants WHERE is_deleted = 0 ORDER BY id DESC LIMIT ? OFFSET ?`,
		p.Limit(), p.Offset())
	if err != nil {
		return nil, 0, translateErr(err)
	}
	return rows, total, nil
}

func (r *tenantRepository) Update(ctx context.Context, t *domain.Tenant) error {
	const q = `UPDATE tenants SET code = :code, name = :name, is_active = :is_active, updated_by = :updated_by
		WHERE id = :id AND is_deleted = 0`
	_, err := r.db.NamedExecContext(ctx, q, t)
	return translateErr(err)
}

func (r *tenantRepository) SoftDelete(ctx context.Context, id, deletedBy uint64) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE tenants SET is_deleted = 1, deleted_at = ?, deleted_by = ? WHERE id = ?`,
		time.Now(), deletedBy, id)
	return translateErr(err)
}

// ---- User ----

type userRepository struct{ db *sqlx.DB }

func NewUserRepository(db *sqlx.DB) domain.UserRepository { return &userRepository{db: db} }

func (r *userRepository) Create(ctx context.Context, u *domain.User) error {
	now := time.Now()
	u.CreatedAt, u.UpdatedAt = now, now
	const q = `INSERT INTO users (user_uuid, tenant_id, username, email, password_hash, full_name, is_active, created_at, updated_at, created_by)
		VALUES (:user_uuid, :tenant_id, :username, :email, :password_hash, :full_name, :is_active, :created_at, :updated_at, :created_by)`
	res, err := r.db.NamedExecContext(ctx, q, u)
	if err != nil {
		return translateErr(err)
	}
	id, _ := res.LastInsertId()
	u.ID = uint64(id)
	return nil
}

func (r *userRepository) GetByID(ctx context.Context, id uint64) (*domain.User, error) {
	var u domain.User
	if err := r.db.GetContext(ctx, &u, `SELECT * FROM users WHERE id = ? AND is_deleted = 0`, id); err != nil {
		return nil, translateErr(err)
	}
	roles, err := r.RolesByUserID(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	u.Roles = roles
	return &u, nil
}

func (r *userRepository) GetByUsername(ctx context.Context, username string) (*domain.User, error) {
	var u domain.User
	if err := r.db.GetContext(ctx, &u, `SELECT * FROM users WHERE username = ? AND is_deleted = 0`, username); err != nil {
		return nil, translateErr(err)
	}
	roles, err := r.RolesByUserID(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	u.Roles = roles
	return &u, nil
}

func (r *userRepository) List(ctx context.Context, tenantID *uint64, p domain.Pagination) ([]domain.User, int, error) {
	where := "is_deleted = 0"
	args := []interface{}{}
	if tenantID != nil {
		where += " AND tenant_id = ?"
		args = append(args, *tenantID)
	}
	var total int
	if err := r.db.GetContext(ctx, &total, "SELECT COUNT(*) FROM users WHERE "+where, args...); err != nil {
		return nil, 0, translateErr(err)
	}
	var rows []domain.User
	args = append(args, p.Limit(), p.Offset())
	err := r.db.SelectContext(ctx, &rows,
		"SELECT * FROM users WHERE "+where+" ORDER BY id DESC LIMIT ? OFFSET ?", args...)
	if err != nil {
		return nil, 0, translateErr(err)
	}
	if err := r.attachRoles(ctx, rows); err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// attachRoles mengisi field Roles utk sekumpulan user sekaligus lewat SATU
// query IN(...), menggantikan RolesByUserID dipanggil per baris (N+1 --
// audit performa: List() dgn page_size besar sebelumnya memicu 1+N query
// roundtrip terpisah alih-alih satu JOIN).
func (r *userRepository) attachRoles(ctx context.Context, rows []domain.User) error {
	if len(rows) == 0 {
		return nil
	}
	ids := make([]uint64, len(rows))
	for i, u := range rows {
		ids[i] = u.ID
	}
	query, args, err := sqlx.In(
		`SELECT ur.user_id, rr.code FROM user_roles ur
		 JOIN ref_roles rr ON rr.id = ur.role_id AND rr.is_deleted = 0
		 WHERE ur.user_id IN (?)`, ids)
	if err != nil {
		return translateErr(err)
	}
	query = r.db.Rebind(query)
	var pairs []struct {
		UserID uint64 `db:"user_id"`
		Code   string `db:"code"`
	}
	if err := r.db.SelectContext(ctx, &pairs, query, args...); err != nil {
		return translateErr(err)
	}
	byUser := make(map[uint64][]string, len(rows))
	for _, p := range pairs {
		byUser[p.UserID] = append(byUser[p.UserID], p.Code)
	}
	for i := range rows {
		rows[i].Roles = byUser[rows[i].ID]
	}
	return nil
}

func (r *userRepository) Update(ctx context.Context, u *domain.User) error {
	const q = `UPDATE users SET username = :username, email = :email, full_name = :full_name,
		is_active = :is_active, updated_by = :updated_by WHERE id = :id AND is_deleted = 0`
	_, err := r.db.NamedExecContext(ctx, q, u)
	return translateErr(err)
}

func (r *userRepository) UpdatePassword(ctx context.Context, id uint64, passwordHash string, updatedBy *uint64) error {
	// Existence dicek terpisah (bukan lewat RowsAffected) — pola sama seperti
	// tenantRepository.UpdateBranding: driver mysql tidak menghitung baris yang
	// nilainya tidak berubah, dan di sini kita juga sengaja tidak pernah
	// membandingkan hash lama/baru (hash bcrypt selalu berbeda per pemanggilan
	// meski password sama, jadi RowsAffected>0 bukan masalah nyata di sini,
	// tapi existence-check tetap dipertahankan agar konsisten & agar caller
	// dapat ErrNotFound yang jelas alih-alih sukses semu pada id yang tidak ada).
	var exists bool
	if err := r.db.GetContext(ctx, &exists, `SELECT EXISTS(SELECT 1 FROM users WHERE id = ? AND is_deleted = 0)`, id); err != nil {
		return translateErr(err)
	}
	if !exists {
		return domain.ErrNotFound
	}
	_, err := r.db.ExecContext(ctx,
		`UPDATE users SET password_hash = ?, updated_by = ? WHERE id = ? AND is_deleted = 0`,
		passwordHash, updatedBy, id)
	return translateErr(err)
}

func (r *userRepository) SoftDelete(ctx context.Context, id, deletedBy uint64) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE users SET is_deleted = 1, deleted_at = ?, deleted_by = ? WHERE id = ?`,
		time.Now(), deletedBy, id)
	return translateErr(err)
}

func (r *userRepository) TouchLastLogin(ctx context.Context, id uint64, at time.Time) error {
	_, err := r.db.ExecContext(ctx, `UPDATE users SET last_login_at = ? WHERE id = ?`, at, id)
	return translateErr(err)
}

// RecordFailedLogin -- SATU UPDATE atomik (increment + keputusan lock
// dihitung dari nilai baris SETELAH row-lock diperoleh MariaDB), bukan
// increment-lalu-SELECT-lalu-UPDATE terpisah seperti versi sebelumnya --
// versi lama py celah TOCTOU nyata (lihat komentar interface
// domain.UserRepository.RecordFailedLogin): request paralel bisa lolos
// lockout krn semuanya membaca counter lama sebelum salah satu sempat commit.
func (r *userRepository) RecordFailedLogin(ctx context.Context, id uint64, maxAttempts uint32, lockUntil time.Time) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE users SET
			failed_login_attempts = failed_login_attempts + 1,
			locked_until = CASE WHEN failed_login_attempts + 1 >= ? THEN ? ELSE locked_until END
		WHERE id = ? AND is_deleted = 0`,
		maxAttempts, lockUntil, id)
	return translateErr(err)
}

func (r *userRepository) ResetLoginLockout(ctx context.Context, id uint64) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE users SET failed_login_attempts = 0, locked_until = NULL WHERE id = ? AND is_deleted = 0`, id)
	return translateErr(err)
}

func (r *userRepository) RolesByUserID(ctx context.Context, userID uint64) ([]string, error) {
	var roles []string
	err := r.db.SelectContext(ctx, &roles,
		`SELECT rr.code FROM user_roles ur
		 JOIN ref_roles rr ON rr.id = ur.role_id AND rr.is_deleted = 0
		 WHERE ur.user_id = ?`, userID)
	if err != nil {
		return nil, translateErr(err)
	}
	return roles, nil
}

func (r *userRepository) AssignRole(ctx context.Context, userID, roleID uint64, createdBy *uint64) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT IGNORE INTO user_roles (user_id, role_id, created_by) VALUES (?, ?, ?)`,
		userID, roleID, createdBy)
	return translateErr(err)
}

func (r *userRepository) RevokeRole(ctx context.Context, userID, roleID uint64) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM user_roles WHERE user_id = ? AND role_id = ?`, userID, roleID)
	return translateErr(err)
}

// ---- APIToken ----

type apiTokenRepository struct{ db *sqlx.DB }

func NewAPITokenRepository(db *sqlx.DB) domain.APITokenRepository { return &apiTokenRepository{db: db} }

func (r *apiTokenRepository) Create(ctx context.Context, t *domain.APIToken) error {
	t.CreatedAt = time.Now()
	const q = `INSERT INTO api_tokens (token_uuid, user_id, tenant_id, name, token_hash, scopes, expires_at, created_at, created_by)
		VALUES (:token_uuid, :user_id, :tenant_id, :name, :token_hash, :scopes, :expires_at, :created_at, :created_by)`
	res, err := r.db.NamedExecContext(ctx, q, t)
	if err != nil {
		return translateErr(err)
	}
	id, _ := res.LastInsertId()
	t.ID = uint64(id)
	return nil
}

func (r *apiTokenRepository) GetByHash(ctx context.Context, tokenHash string) (*domain.APIToken, error) {
	var t domain.APIToken
	err := r.db.GetContext(ctx, &t,
		`SELECT * FROM api_tokens WHERE token_hash = ? AND revoked_at IS NULL
		 AND (expires_at IS NULL OR expires_at > NOW())`, tokenHash)
	if err != nil {
		return nil, translateErr(err)
	}
	return &t, nil
}

func (r *apiTokenRepository) Revoke(ctx context.Context, id uint64) error {
	_, err := r.db.ExecContext(ctx, `UPDATE api_tokens SET revoked_at = ? WHERE id = ?`, time.Now(), id)
	return translateErr(err)
}

func (r *apiTokenRepository) ListByUser(ctx context.Context, userID uint64) ([]domain.APIToken, error) {
	var rows []domain.APIToken
	err := r.db.SelectContext(ctx, &rows, `SELECT * FROM api_tokens WHERE user_id = ? ORDER BY id DESC`, userID)
	if err != nil {
		return nil, translateErr(err)
	}
	return rows, nil
}

// ---- ActivityLog ----

type activityLogRepository struct{ db *sqlx.DB }

func NewActivityLogRepository(db *sqlx.DB) domain.ActivityLogRepository {
	return &activityLogRepository{db: db}
}

func (r *activityLogRepository) Record(ctx context.Context, log *domain.ActivityLog) error {
	const q = `INSERT INTO activity_logs (user_id, tenant_id, action, entity_type, entity_id, description, ip_address)
		VALUES (:user_id, :tenant_id, :action, :entity_type, :entity_id, :description, :ip_address)`
	_, err := r.db.NamedExecContext(ctx, q, log)
	return translateErr(err)
}

func (r *activityLogRepository) ListByEntity(ctx context.Context, entityType string, entityID uint64, p domain.Pagination) ([]domain.ActivityLog, int, error) {
	var total int
	err := r.db.GetContext(ctx, &total,
		`SELECT COUNT(*) FROM activity_logs WHERE entity_type = ? AND entity_id = ?`, entityType, entityID)
	if err != nil {
		return nil, 0, translateErr(err)
	}
	var rows []domain.ActivityLog
	err = r.db.SelectContext(ctx, &rows,
		`SELECT al.*, u.username AS username FROM activity_logs al
		 LEFT JOIN users u ON u.id = al.user_id
		 WHERE al.entity_type = ? AND al.entity_id = ? ORDER BY al.id DESC LIMIT ? OFFSET ?`,
		entityType, entityID, p.Limit(), p.Offset())
	if err != nil {
		return nil, 0, translateErr(err)
	}
	return rows, total, nil
}
