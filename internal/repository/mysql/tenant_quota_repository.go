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
	var max *uint32
	if err := r.db.GetContext(ctx, &max, `SELECT max_devices FROM tenants WHERE id = ? AND is_deleted = 0`, tenantID); err != nil {
		return nil, 0, translateErr(err)
	}
	// Tenant tanpa batas (NULL) tidak perlu hitungan: COUNT(*) per registrasi itu O(jumlah perangkat tenant), jadi onboarding
	// massal menjadi O(n^2). Hitung hanya bila ada batas yang harus ditegakkan.
	if max == nil {
		return nil, 0, nil
	}
	var cur int
	if err := r.db.GetContext(ctx, &cur, `SELECT COUNT(*) FROM devices WHERE tenant_id = ? AND is_deleted = 0`, tenantID); err != nil {
		return nil, 0, translateErr(err)
	}
	return max, cur, nil
}
