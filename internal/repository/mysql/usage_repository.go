package mysql

import (
	"context"

	"github.com/jmoiron/sqlx"

	"acs/internal/domain"
)

// usageRepository menghitung referensi aktif ke entitas master (guard hapus).
// Semua query murni COUNT berparameter; tabel yang dihitung hanya baris
// is_deleted = 0 (baris yang sudah soft-delete tidak lagi "memakai" entitas).
type usageRepository struct{ db *sqlx.DB }

func NewUsageRepository(db *sqlx.DB) domain.UsageRepository { return &usageRepository{db: db} }

// usageProbe: satu subquery COUNT bernama. arg dipakai berulang untuk tiap probe.
type usageProbe struct {
	kind string
	sql  string // harus berupa SELECT COUNT(*) ... dengan tepat satu placeholder '?'
}

func (r *usageRepository) run(ctx context.Context, id uint64, probes []usageProbe) ([]domain.UsageCount, error) {
	out := make([]domain.UsageCount, 0, len(probes))
	for _, p := range probes {
		var n int
		if err := r.db.GetContext(ctx, &n, p.sql, id); err != nil {
			return nil, translateErr(err)
		}
		if n > 0 {
			out = append(out, domain.UsageCount{Kind: p.kind, Count: n})
		}
	}
	return out, nil
}

func (r *usageRepository) VendorUsage(ctx context.Context, vendorID uint64) ([]domain.UsageCount, error) {
	return r.run(ctx, vendorID, []usageProbe{
		{"device", `SELECT COUNT(*) FROM devices WHERE vendor_id = ? AND is_deleted = 0`},
		{"device_model", `SELECT COUNT(*) FROM device_models WHERE vendor_id = ? AND is_deleted = 0`},
		{"parameter_mapping", `SELECT COUNT(*) FROM vendor_parameter_mappings WHERE vendor_id = ? AND is_deleted = 0`},
		{"firmware", `SELECT COUNT(*) FROM firmware_files WHERE vendor_id = ? AND is_deleted = 0`},
		{"provisioning_profile", `SELECT COUNT(*) FROM provisioning_profiles WHERE vendor_id = ? AND is_deleted = 0`},
		{"zero_touch_rule", `SELECT COUNT(*) FROM zero_touch_rules WHERE vendor_id = ? AND is_deleted = 0`},
		{"rollout_batch", `SELECT COUNT(*) FROM firmware_rollout_batches WHERE vendor_id = ? AND is_deleted = 0`},
	})
}

func (r *usageRepository) DeviceModelUsage(ctx context.Context, deviceModelID uint64) ([]domain.UsageCount, error) {
	return r.run(ctx, deviceModelID, []usageProbe{
		{"device", `SELECT COUNT(*) FROM devices WHERE device_model_id = ? AND is_deleted = 0`},
		{"parameter_mapping", `SELECT COUNT(*) FROM vendor_parameter_mappings WHERE device_model_id = ? AND is_deleted = 0`},
		{"firmware", `SELECT COUNT(*) FROM firmware_files WHERE device_model_id = ? AND is_deleted = 0`},
		{"provisioning_profile", `SELECT COUNT(*) FROM provisioning_profiles WHERE device_model_id = ? AND is_deleted = 0`},
		{"zero_touch_rule", `SELECT COUNT(*) FROM zero_touch_rules WHERE device_model_id = ? AND is_deleted = 0`},
		{"rollout_batch", `SELECT COUNT(*) FROM firmware_rollout_batches WHERE device_model_id = ? AND is_deleted = 0`},
	})
}

func (r *usageRepository) FirmwareUsage(ctx context.Context, firmwareID uint64) ([]domain.UsageCount, error) {
	return r.run(ctx, firmwareID, []usageProbe{
		{"zero_touch_rule", `SELECT COUNT(*) FROM zero_touch_rules WHERE firmware_file_id = ? AND is_deleted = 0`},
		// Batch belum terminal = status bukan COMPLETED/CANCELLED.
		{"rollout_batch_aktif", `SELECT COUNT(*) FROM firmware_rollout_batches b
			JOIN ref_firmware_rollout_status s ON s.id = b.status_id
			WHERE b.firmware_file_id = ? AND b.is_deleted = 0 AND s.code NOT IN ('COMPLETED', 'CANCELLED')`},
		// Job upgrade yang task-nya belum terminal (PENDING/QUEUED/SENT).
		{"upgrade_job_berjalan", `SELECT COUNT(*) FROM firmware_upgrade_jobs j
			JOIN tasks t ON t.id = j.task_id
			JOIN ref_task_status ts ON ts.id = t.task_status_id
			WHERE j.firmware_id = ? AND ts.code IN ('PENDING', 'QUEUED', 'SENT')`},
	})
}

func (r *usageRepository) TenantUsage(ctx context.Context, tenantID uint64) ([]domain.UsageCount, error) {
	return r.run(ctx, tenantID, []usageProbe{
		{"user", `SELECT COUNT(*) FROM users WHERE tenant_id = ? AND is_deleted = 0`},
		{"device", `SELECT COUNT(*) FROM devices WHERE tenant_id = ? AND is_deleted = 0`},
	})
}
