package mysql

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"

	"acs/internal/domain"
)

type deviceDiagnosticRepository struct{ db *sqlx.DB }

func NewDeviceDiagnosticRepository(db *sqlx.DB) domain.DeviceDiagnosticRepository {
	return &deviceDiagnosticRepository{db: db}
}

func (r *deviceDiagnosticRepository) Create(ctx context.Context, d *domain.DeviceDiagnostic) error {
	d.CreatedAt = time.Now()
	const q = `INSERT INTO device_diagnostics (device_id, task_id, diagnostic_type, status, created_at)
		VALUES (:device_id, :task_id, :diagnostic_type, :status, :created_at)`
	res, err := r.db.NamedExecContext(ctx, q, d)
	if err != nil {
		return translateErr(err)
	}
	id, _ := res.LastInsertId()
	d.ID = uint64(id)
	return nil
}

func (r *deviceDiagnosticRepository) GetByID(ctx context.Context, id uint64) (*domain.DeviceDiagnostic, error) {
	var d domain.DeviceDiagnostic
	if err := r.db.GetContext(ctx, &d, `SELECT * FROM device_diagnostics WHERE id = ?`, id); err != nil {
		return nil, translateErr(err)
	}
	return &d, nil
}

func (r *deviceDiagnosticRepository) GetByTaskID(ctx context.Context, taskID uint64) (*domain.DeviceDiagnostic, error) {
	var d domain.DeviceDiagnostic
	if err := r.db.GetContext(ctx, &d, `SELECT * FROM device_diagnostics WHERE task_id = ?`, taskID); err != nil {
		return nil, translateErr(err)
	}
	return &d, nil
}

func (r *deviceDiagnosticRepository) ListByDevice(ctx context.Context, deviceID uint64, p domain.Pagination) ([]domain.DeviceDiagnostic, int, error) {
	var total int
	if err := r.db.GetContext(ctx, &total, `SELECT COUNT(*) FROM device_diagnostics WHERE device_id = ?`, deviceID); err != nil {
		return nil, 0, translateErr(err)
	}
	var rows []domain.DeviceDiagnostic
	err := r.db.SelectContext(ctx, &rows,
		`SELECT * FROM device_diagnostics WHERE device_id = ? ORDER BY id DESC LIMIT ? OFFSET ?`,
		deviceID, p.Limit(), p.Offset())
	if err != nil {
		return nil, 0, translateErr(err)
	}
	return rows, total, nil
}

func (r *deviceDiagnosticRepository) UpdateResult(ctx context.Context, id uint64, status string, result domain.JSONRawMessage, executedAt time.Time) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE device_diagnostics SET status = ?, result = ?, executed_at = ? WHERE id = ?`,
		status, result, executedAt, id)
	return translateErr(err)
}
