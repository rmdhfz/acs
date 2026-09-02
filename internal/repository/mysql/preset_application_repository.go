package mysql

import (
	"context"

	"acs/internal/domain"

	"github.com/jmoiron/sqlx"
)

type presetApplicationRepository struct {
	db *sqlx.DB
}

func NewPresetApplicationRepository(db *sqlx.DB) domain.PresetApplicationRepository {
	return &presetApplicationRepository{db: db}
}

func (r *presetApplicationRepository) Get(ctx context.Context, presetID, deviceID uint64) (*domain.PresetApplication, error) {
	var a domain.PresetApplication
	err := r.db.GetContext(ctx, &a,
		`SELECT preset_id, device_id, last_applied_at, last_drift_hash, consecutive_failures, status, updated_at
		 FROM preset_applications WHERE preset_id = ? AND device_id = ?`, presetID, deviceID)
	if err != nil {
		return nil, translateErr(err)
	}
	return &a, nil
}

// Upsert menulis ulang baris (preset_id, device_id). Semua field non-kunci
// diambil dari `a` apa adanya — pemanggil (EvaluatePresets) yang menentukan
// nilai akhir (mis. reset consecutive_failures ke 0 saat sukses, atau
// increment saat gagal).
func (r *presetApplicationRepository) Upsert(ctx context.Context, a *domain.PresetApplication) error {
	_, err := r.db.NamedExecContext(ctx,
		`INSERT INTO preset_applications
			(preset_id, device_id, last_applied_at, last_drift_hash, consecutive_failures, status)
		 VALUES (:preset_id, :device_id, :last_applied_at, :last_drift_hash, :consecutive_failures, :status)
		 ON DUPLICATE KEY UPDATE
			last_applied_at = VALUES(last_applied_at),
			last_drift_hash = VALUES(last_drift_hash),
			consecutive_failures = VALUES(consecutive_failures),
			status = VALUES(status)`, a)
	return translateErr(err)
}
