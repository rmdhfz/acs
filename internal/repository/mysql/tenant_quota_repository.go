package mysql

import (
	"context"

	"github.com/jmoiron/sqlx"

	"acs/internal/domain"
)

// tenantQuotaRepository — batas perangkat per tenant (migrations/0028). Terpisah dari TenantRepository agar
// antarmuka tenant yang sudah ada (dan fake-nya di banyak tes) tidak berubah.
type tenantQuotaRepository struct{ db *sqlx.DB }

func NewTenantQuotaRepository(db *sqlx.DB) domain.TenantQuotaRepository {
	return &tenantQuotaRepository{db: db}
}

func (r *tenantQuotaRepository) SetMaxDevices(ctx context.Context, id uint64, max *uint32, updatedBy *uint64) error {
	// Existence dicek terpisah (RowsAffected tidak menghitung baris yang nilainya tak berubah).
	var exists bool
	if err := r.db.GetContext(ctx, &exists, `SELECT EXISTS(SELECT 1 FROM tenants WHERE id = ? AND is_deleted = 0)`, id); err != nil {
		return translateErr(err)
	}
	if !exists {
		return domain.ErrNotFound
	}
	_, err := r.db.ExecContext(ctx, `UPDATE tenants SET max_devices = ?, updated_by = ? WHERE id = ? AND is_deleted = 0`, max, updatedBy, id)
	return translateErr(err)
}

func (r *tenantQuotaRepository) DeviceCapacity(ctx context.Context, tenantID uint64) (*uint32, int, error) {
	var row struct {
		Max *uint32 `db:"max_devices"`
		Cur int     `db:"cur"`
	}
	err := r.db.GetContext(ctx, &row, `SELECT t.max_devices,
		(SELECT COUNT(*) FROM devices d WHERE d.tenant_id = t.id AND d.is_deleted = 0) AS cur
		FROM tenants t WHERE t.id = ? AND t.is_deleted = 0`, tenantID)
	if err != nil {
		return nil, 0, translateErr(err)
	}
	return row.Max, row.Cur, nil
}
