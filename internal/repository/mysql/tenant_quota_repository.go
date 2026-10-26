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

// NewTenantStorageQuotaRepository — batas total ukuran berkas per tenant (migrations/0043); memakai struct yang sama.
func NewTenantStorageQuotaRepository(db *sqlx.DB) domain.TenantStorageQuotaRepository {
	return &tenantQuotaRepository{db: db}
}

func (r *tenantQuotaRepository) SetMaxStorageBytes(ctx context.Context, id uint64, max *uint64, updatedBy *uint64) error {
	var exists bool
	if err := r.db.GetContext(ctx, &exists, `SELECT EXISTS(SELECT 1 FROM tenants WHERE id = ? AND is_deleted = 0)`, id); err != nil {
		return translateErr(err)
	}
	if !exists {
		return domain.ErrNotFound
	}
	_, err := r.db.ExecContext(ctx, `UPDATE tenants SET max_storage_bytes = ?, updated_by = ? WHERE id = ? AND is_deleted = 0`, max, updatedBy, id)
	return translateErr(err)
}

func (r *tenantQuotaRepository) StorageUsage(ctx context.Context, tenantID uint64) (*uint64, uint64, error) {
	var max *uint64
	if err := r.db.GetContext(ctx, &max, `SELECT max_storage_bytes FROM tenants WHERE id = ? AND is_deleted = 0`, tenantID); err != nil {
		return nil, 0, translateErr(err)
	}
	// Tenant tanpa batas tidak perlu SUM (lihat DeviceCapacity). Memakai idx_files_tenant_type (tenant_id, file_type).
	if max == nil {
		return nil, 0, nil
	}
	var used uint64
	if err := r.db.GetContext(ctx, &used, `SELECT CAST(COALESCE(SUM(file_size_bytes), 0) AS UNSIGNED) FROM files WHERE tenant_id = ? AND is_deleted = 0`, tenantID); err != nil {
		return nil, 0, translateErr(err)
	}
	return max, used, nil
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
