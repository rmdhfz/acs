package mysql

import (
	"context"
	"time"

	"acs/internal/domain"
)

func (r *deviceDiagnosticRepository) UpdateTask(ctx context.Context, id, taskID uint64, status string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE device_diagnostics SET task_id = ?, status = ? WHERE id = ?`, taskID, status, id)
	return translateErr(err)
}

func (r *deviceDiagnosticRepository) UpdateStatus(ctx context.Context, id uint64, status string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE device_diagnostics SET status = ? WHERE id = ?`, status, id)
	return translateErr(err)
}

func (r *deviceDiagnosticRepository) ListOpenByDevice(ctx context.Context, deviceID uint64) ([]domain.DeviceDiagnostic, error) {
	var rows []domain.DeviceDiagnostic
	err := r.db.SelectContext(ctx, &rows,
		`SELECT * FROM device_diagnostics WHERE device_id = ? AND status IN (?, ?) ORDER BY id`,
		deviceID, domain.DiagnosticStatusPending, domain.DiagnosticStatusRunning)
	return rows, translateErr(err)
}

func (r *deviceDiagnosticRepository) TimeoutStale(ctx context.Context, before time.Time) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`UPDATE device_diagnostics SET status = ?, result = JSON_OBJECT('error', 'timeout: perangkat tidak menyelesaikan diagnostik'), executed_at = NOW()
		  WHERE status IN (?, ?) AND created_at < ?`,
		domain.DiagnosticStatusFailed, domain.DiagnosticStatusPending, domain.DiagnosticStatusRunning, before)
	if err != nil {
		return 0, translateErr(err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}
