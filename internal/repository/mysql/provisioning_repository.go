package mysql

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"

	"acs/internal/domain"
)

// ---- ProvisioningProfile ----

type provisioningProfileRepository struct{ db *sqlx.DB }

func NewProvisioningProfileRepository(db *sqlx.DB) domain.ProvisioningProfileRepository {
	return &provisioningProfileRepository{db: db}
}

func (r *provisioningProfileRepository) Create(ctx context.Context, p *domain.ProvisioningProfile) error {
	now := time.Now()
	p.CreatedAt, p.UpdatedAt = now, now
	const q = `INSERT INTO provisioning_profiles
		(profile_uuid, tenant_id, vendor_id, device_model_id, name, description, is_default, is_active, created_at, updated_at, created_by)
		VALUES (:profile_uuid, :tenant_id, :vendor_id, :device_model_id, :name, :description, :is_default, :is_active, :created_at, :updated_at, :created_by)`
	res, err := r.db.NamedExecContext(ctx, q, p)
	if err != nil {
		return translateErr(err)
	}
	id, _ := res.LastInsertId()
	p.ID = uint64(id)
	return nil
}

func (r *provisioningProfileRepository) GetByID(ctx context.Context, id uint64) (*domain.ProvisioningProfile, error) {
	var p domain.ProvisioningProfile
	if err := r.db.GetContext(ctx, &p, `SELECT * FROM provisioning_profiles WHERE id = ? AND is_deleted = 0`, id); err != nil {
		return nil, translateErr(err)
	}
	return &p, nil
}

func (r *provisioningProfileRepository) GetByUUID(ctx context.Context, uuid string) (*domain.ProvisioningProfile, error) {
	var p domain.ProvisioningProfile
	err := r.db.GetContext(ctx, &p, `SELECT * FROM provisioning_profiles WHERE profile_uuid = ? AND is_deleted = 0`, uuid)
	if err != nil {
		return nil, translateErr(err)
	}
	return &p, nil
}

func (r *provisioningProfileRepository) List(ctx context.Context, tenantID *uint64, p domain.Pagination) ([]domain.ProvisioningProfile, int, error) {
	where := "is_deleted = 0"
	args := []interface{}{}
	if tenantID != nil {
		where += " AND (tenant_id = ? OR tenant_id IS NULL)"
		args = append(args, *tenantID)
	}
	var total int
	if err := r.db.GetContext(ctx, &total, "SELECT COUNT(*) FROM provisioning_profiles WHERE "+where, args...); err != nil {
		return nil, 0, translateErr(err)
	}
	var rows []domain.ProvisioningProfile
	args = append(args, p.Limit(), p.Offset())
	err := r.db.SelectContext(ctx, &rows,
		"SELECT * FROM provisioning_profiles WHERE "+where+" ORDER BY id DESC LIMIT ? OFFSET ?", args...)
	if err != nil {
		return nil, 0, translateErr(err)
	}
	return rows, total, nil
}

func (r *provisioningProfileRepository) Update(ctx context.Context, p *domain.ProvisioningProfile) error {
	const q = `UPDATE provisioning_profiles SET vendor_id = :vendor_id, device_model_id = :device_model_id,
		name = :name, description = :description, is_default = :is_default, is_active = :is_active,
		updated_by = :updated_by WHERE id = :id AND is_deleted = 0`
	_, err := r.db.NamedExecContext(ctx, q, p)
	return translateErr(err)
}

func (r *provisioningProfileRepository) SoftDelete(ctx context.Context, id, deletedBy uint64) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE provisioning_profiles SET is_deleted = 1, deleted_at = ?, deleted_by = ? WHERE id = ?`,
		time.Now(), deletedBy, id)
	return translateErr(err)
}

// ---- ProvisioningProfileParameter ----

type provisioningProfileParameterRepository struct{ db *sqlx.DB }

func NewProvisioningProfileParameterRepository(db *sqlx.DB) domain.ProvisioningProfileParameterRepository {
	return &provisioningProfileParameterRepository{db: db}
}

// Replace mengganti seluruh parameter milik profile dalam satu transaksi
// (form edit profile mengirim daftar lengkap, bukan patch parsial).
func (r *provisioningProfileParameterRepository) Replace(ctx context.Context, profileID uint64, params []domain.ProvisioningProfileParameter) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return translateErr(err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM provisioning_profile_parameters WHERE profile_id = ?`, profileID); err != nil {
		_ = tx.Rollback()
		return translateErr(err)
	}
	const q = `INSERT INTO provisioning_profile_parameters
		(profile_id, parameter_name, parameter_value, parameter_type_id, apply_order)
		VALUES (:profile_id, :parameter_name, :parameter_value, :parameter_type_id, :apply_order)`
	for i := range params {
		params[i].ProfileID = profileID
		if _, err := tx.NamedExecContext(ctx, q, &params[i]); err != nil {
			_ = tx.Rollback()
			return translateErr(err)
		}
	}
	return tx.Commit()
}

func (r *provisioningProfileParameterRepository) ListByProfile(ctx context.Context, profileID uint64) ([]domain.ProvisioningProfileParameter, error) {
	var rows []domain.ProvisioningProfileParameter
	err := r.db.SelectContext(ctx, &rows,
		`SELECT * FROM provisioning_profile_parameters WHERE profile_id = ? ORDER BY apply_order, id`, profileID)
	if err != nil {
		return nil, translateErr(err)
	}
	return rows, nil
}

// ---- ZeroTouchRule ----

type zeroTouchRuleRepository struct{ db *sqlx.DB }

func NewZeroTouchRuleRepository(db *sqlx.DB) domain.ZeroTouchRuleRepository {
	return &zeroTouchRuleRepository{db: db}
}

func (r *zeroTouchRuleRepository) Create(ctx context.Context, ru *domain.ZeroTouchRule) error {
	now := time.Now()
	ru.CreatedAt, ru.UpdatedAt = now, now
	const q = `INSERT INTO zero_touch_rules
		(tenant_id, vendor_id, device_model_id, oui, serial_pattern, provisioning_profile_id, priority, is_active, created_at, updated_at, created_by)
		VALUES (:tenant_id, :vendor_id, :device_model_id, :oui, :serial_pattern, :provisioning_profile_id, :priority, :is_active, :created_at, :updated_at, :created_by)`
	res, err := r.db.NamedExecContext(ctx, q, ru)
	if err != nil {
		return translateErr(err)
	}
	id, _ := res.LastInsertId()
	ru.ID = uint64(id)
	return nil
}

func (r *zeroTouchRuleRepository) GetByID(ctx context.Context, id uint64) (*domain.ZeroTouchRule, error) {
	var ru domain.ZeroTouchRule
	if err := r.db.GetContext(ctx, &ru, `SELECT * FROM zero_touch_rules WHERE id = ? AND is_deleted = 0`, id); err != nil {
		return nil, translateErr(err)
	}
	return &ru, nil
}

// ListActiveOrdered: rule milik tenant + rule global (tenant_id NULL), priority ASC
// (angka lebih kecil dievaluasi lebih dulu — lihat schema.sql).
func (r *zeroTouchRuleRepository) ListActiveOrdered(ctx context.Context, tenantID *uint64) ([]domain.ZeroTouchRule, error) {
	var rows []domain.ZeroTouchRule
	var err error
	if tenantID != nil {
		err = r.db.SelectContext(ctx, &rows,
			`SELECT * FROM zero_touch_rules WHERE is_deleted = 0 AND is_active = 1
			 AND (tenant_id = ? OR tenant_id IS NULL) ORDER BY priority ASC, id ASC`, *tenantID)
	} else {
		err = r.db.SelectContext(ctx, &rows,
			`SELECT * FROM zero_touch_rules WHERE is_deleted = 0 AND is_active = 1
			 AND tenant_id IS NULL ORDER BY priority ASC, id ASC`)
	}
	if err != nil {
		return nil, translateErr(err)
	}
	return rows, nil
}

func (r *zeroTouchRuleRepository) Update(ctx context.Context, ru *domain.ZeroTouchRule) error {
	const q = `UPDATE zero_touch_rules SET vendor_id = :vendor_id, device_model_id = :device_model_id,
		oui = :oui, serial_pattern = :serial_pattern, provisioning_profile_id = :provisioning_profile_id,
		priority = :priority, is_active = :is_active, updated_by = :updated_by WHERE id = :id AND is_deleted = 0`
	_, err := r.db.NamedExecContext(ctx, q, ru)
	return translateErr(err)
}

func (r *zeroTouchRuleRepository) SoftDelete(ctx context.Context, id, deletedBy uint64) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE zero_touch_rules SET is_deleted = 1, deleted_at = ?, deleted_by = ? WHERE id = ?`,
		time.Now(), deletedBy, id)
	return translateErr(err)
}
