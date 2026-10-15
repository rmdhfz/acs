package mysql

import (
	"context"

	"github.com/jmoiron/sqlx"

	"acs/internal/domain"
)

type openTaskCanceller struct{ db *sqlx.DB }

func NewOpenTaskCanceller(db *sqlx.DB) domain.OpenTaskCanceller { return &openTaskCanceller{db: db} }

// CancelOpenForDevice mengubah semua task PENDING/QUEUED/SENT milik device
// menjadi CANCELLED dalam satu UPDATE atomik; mengembalikan jumlah task yang
// dibatalkan. Task yang sudah terminal (COMPLETED/FAILED/...) tidak disentuh.
func (r *openTaskCanceller) CancelOpenForDevice(ctx context.Context, deviceID uint64, updatedBy *uint64) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`UPDATE tasks t
		   JOIN ref_task_status cur ON cur.id = t.task_status_id
		   JOIN ref_task_status c   ON c.code = ?
		    SET t.task_status_id = c.id, t.updated_by = ?
		  WHERE t.device_id = ? AND cur.code IN (?, ?, ?)`,
		domain.TaskStatusCancelled, updatedBy, deviceID,
		domain.TaskStatusPending, domain.TaskStatusQueued, domain.TaskStatusSent)
	if err != nil {
		return 0, translateErr(err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}
