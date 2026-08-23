package mysql

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"

	"acs/internal/domain"
)

// ---- FirmwareFile ----

type firmwareFileRepository struct{ db *sqlx.DB }

func NewFirmwareFileRepository(db *sqlx.DB) domain.FirmwareFileRepository {
	return &firmwareFileRepository{db: db}
}

func (r *firmwareFileRepository) Create(ctx context.Context, f *domain.FirmwareFile) error {
	now := time.Now()
	f.CreatedAt, f.UpdatedAt = now, now
	const q = `INSERT INTO firmware_files
		(firmware_uuid, vendor_id, device_model_id, version, file_name, storage_key, file_size_bytes,
		 checksum_sha256, release_notes, is_active, created_at, updated_at, created_by)
		VALUES (:firmware_uuid, :vendor_id, :device_model_id, :version, :file_name, :storage_key, :file_size_bytes,
		 :checksum_sha256, :release_notes, :is_active, :created_at, :updated_at, :created_by)`
	res, err := r.db.NamedExecContext(ctx, q, f)
	if err != nil {
		return translateErr(err)
	}
	id, _ := res.LastInsertId()
	f.ID = uint64(id)
	return nil
}

func (r *firmwareFileRepository) GetByID(ctx context.Context, id uint64) (*domain.FirmwareFile, error) {
	var f domain.FirmwareFile
	if err := r.db.GetContext(ctx, &f, `SELECT * FROM firmware_files WHERE id = ? AND is_deleted = 0`, id); err != nil {
		return nil, translateErr(err)
	}
	return &f, nil
}

func (r *firmwareFileRepository) GetByUUID(ctx context.Context, uuid string) (*domain.FirmwareFile, error) {
	var f domain.FirmwareFile
	err := r.db.GetContext(ctx, &f, `SELECT * FROM firmware_files WHERE firmware_uuid = ? AND is_deleted = 0`, uuid)
	if err != nil {
		return nil, translateErr(err)
	}
	return &f, nil
}

func (r *firmwareFileRepository) ListByVendor(ctx context.Context, vendorID uint64, p domain.Pagination) ([]domain.FirmwareFile, int, error) {
	var total int
	if err := r.db.GetContext(ctx, &total, `SELECT COUNT(*) FROM firmware_files WHERE vendor_id = ? AND is_deleted = 0`, vendorID); err != nil {
		return nil, 0, translateErr(err)
	}
	var rows []domain.FirmwareFile
	err := r.db.SelectContext(ctx, &rows,
		`SELECT * FROM firmware_files WHERE vendor_id = ? AND is_deleted = 0 ORDER BY id DESC LIMIT ? OFFSET ?`,
		vendorID, p.Limit(), p.Offset())
	if err != nil {
		return nil, 0, translateErr(err)
	}
	return rows, total, nil
}

func (r *firmwareFileRepository) SoftDelete(ctx context.Context, id, deletedBy uint64) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE firmware_files SET is_deleted = 1, deleted_at = ?, deleted_by = ? WHERE id = ?`,
		time.Now(), deletedBy, id)
	return translateErr(err)
}

// ---- FirmwareUpgradeJob ----

type firmwareUpgradeJobRepository struct{ db *sqlx.DB }

func NewFirmwareUpgradeJobRepository(db *sqlx.DB) domain.FirmwareUpgradeJobRepository {
	return &firmwareUpgradeJobRepository{db: db}
}

func (r *firmwareUpgradeJobRepository) Create(ctx context.Context, j *domain.FirmwareUpgradeJob) error {
	now := time.Now()
	j.CreatedAt, j.UpdatedAt = now, now
	const q = `INSERT INTO firmware_upgrade_jobs
		(job_uuid, device_id, firmware_id, task_id, task_status_id, from_version, to_version, scheduled_at, created_at, updated_at, created_by)
		VALUES (:job_uuid, :device_id, :firmware_id, :task_id, :task_status_id, :from_version, :to_version, :scheduled_at, :created_at, :updated_at, :created_by)`
	res, err := r.db.NamedExecContext(ctx, q, j)
	if err != nil {
		return translateErr(err)
	}
	id, _ := res.LastInsertId()
	j.ID = uint64(id)
	return nil
}

func (r *firmwareUpgradeJobRepository) GetByID(ctx context.Context, id uint64) (*domain.FirmwareUpgradeJob, error) {
	var j domain.FirmwareUpgradeJob
	if err := r.db.GetContext(ctx, &j, `SELECT * FROM firmware_upgrade_jobs WHERE id = ?`, id); err != nil {
		return nil, translateErr(err)
	}
	return &j, nil
}

func (r *firmwareUpgradeJobRepository) GetByTaskID(ctx context.Context, taskID uint64) (*domain.FirmwareUpgradeJob, error) {
	var j domain.FirmwareUpgradeJob
	if err := r.db.GetContext(ctx, &j, `SELECT * FROM firmware_upgrade_jobs WHERE task_id = ?`, taskID); err != nil {
		return nil, translateErr(err)
	}
	return &j, nil
}

func (r *firmwareUpgradeJobRepository) ListByDevice(ctx context.Context, deviceID uint64, p domain.Pagination) ([]domain.FirmwareUpgradeJob, int, error) {
	var total int
	if err := r.db.GetContext(ctx, &total, `SELECT COUNT(*) FROM firmware_upgrade_jobs WHERE device_id = ?`, deviceID); err != nil {
		return nil, 0, translateErr(err)
	}
	var rows []domain.FirmwareUpgradeJob
	err := r.db.SelectContext(ctx, &rows,
		`SELECT * FROM firmware_upgrade_jobs WHERE device_id = ? ORDER BY id DESC LIMIT ? OFFSET ?`,
		deviceID, p.Limit(), p.Offset())
	if err != nil {
		return nil, 0, translateErr(err)
	}
	return rows, total, nil
}

func (r *firmwareUpgradeJobRepository) UpdateStatus(ctx context.Context, id, taskStatusID uint64, errMsg *string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE firmware_upgrade_jobs SET task_status_id = ?, error_message = ? WHERE id = ?`, taskStatusID, errMsg, id)
	return translateErr(err)
}

func (r *firmwareUpgradeJobRepository) MarkStarted(ctx context.Context, id uint64, startedAt time.Time) error {
	_, err := r.db.ExecContext(ctx, `UPDATE firmware_upgrade_jobs SET started_at = ? WHERE id = ?`, startedAt, id)
	return translateErr(err)
}

func (r *firmwareUpgradeJobRepository) MarkCompleted(ctx context.Context, id uint64, toVersion string, completedAt time.Time) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE firmware_upgrade_jobs SET to_version = ?, completed_at = ? WHERE id = ?`, toVersion, completedAt, id)
	return translateErr(err)
}
