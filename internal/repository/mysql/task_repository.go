package mysql

import (
	"context"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"acs/internal/domain"
)

type taskRepository struct{ db *sqlx.DB }

func NewTaskRepository(db *sqlx.DB) domain.TaskRepository { return &taskRepository{db: db} }

func (r *taskRepository) Create(ctx context.Context, t *domain.Task) error {
	now := time.Now()
	t.CreatedAt, t.UpdatedAt = now, now
	const q = `INSERT INTO tasks
		(task_uuid, device_id, task_type_id, task_status_id, priority, parameters, retry_count, max_retries,
		 scheduled_at, expires_at, created_at, updated_at, created_by)
		VALUES
		(:task_uuid, :device_id, :task_type_id, :task_status_id, :priority, :parameters, :retry_count, :max_retries,
		 :scheduled_at, :expires_at, :created_at, :updated_at, :created_by)`
	res, err := r.db.NamedExecContext(ctx, q, t)
	if err != nil {
		return translateErr(err)
	}
	id, _ := res.LastInsertId()
	t.ID = uint64(id)
	return nil
}

func (r *taskRepository) GetByID(ctx context.Context, id uint64) (*domain.Task, error) {
	var t domain.Task
	if err := r.db.GetContext(ctx, &t, `SELECT * FROM tasks WHERE id = ? AND is_deleted = 0`, id); err != nil {
		return nil, translateErr(err)
	}
	return &t, nil
}

func (r *taskRepository) GetByUUID(ctx context.Context, uuid string) (*domain.Task, error) {
	var t domain.Task
	if err := r.db.GetContext(ctx, &t, `SELECT * FROM tasks WHERE task_uuid = ? AND is_deleted = 0`, uuid); err != nil {
		return nil, translateErr(err)
	}
	return &t, nil
}

// NextForDevice: priority ASC (1 = tertinggi dieksekusi duluan, lihat schema.sql
// komentar kolom tasks.priority), created_at ASC, hanya task yang sudah boleh
// dieksekusi (scheduled_at NULL atau sudah lewat) dan belum kedaluwarsa.
func (r *taskRepository) NextForDevice(ctx context.Context, deviceID uint64) (*domain.Task, error) {
	var t domain.Task
	err := r.db.GetContext(ctx, &t,
		`SELECT t.* FROM tasks t
		 JOIN ref_task_status s ON s.id = t.task_status_id
		 WHERE t.device_id = ? AND t.is_deleted = 0 AND s.code = ?
		   AND (t.scheduled_at IS NULL OR t.scheduled_at <= NOW())
		   AND (t.expires_at IS NULL OR t.expires_at > NOW())
		 ORDER BY t.priority ASC, t.created_at ASC
		 LIMIT 1`,
		deviceID, domain.TaskStatusPending)
	if err != nil {
		return nil, translateErr(err)
	}
	return &t, nil
}

func (r *taskRepository) GetSentForDevice(ctx context.Context, deviceID uint64) (*domain.Task, error) {
	var t domain.Task
	err := r.db.GetContext(ctx, &t,
		`SELECT t.* FROM tasks t JOIN ref_task_status s ON s.id = t.task_status_id
		 WHERE t.device_id = ? AND t.is_deleted = 0 AND s.code = ?
		 ORDER BY t.sent_at DESC LIMIT 1`,
		deviceID, domain.TaskStatusSent)
	if err != nil {
		return nil, translateErr(err)
	}
	return &t, nil
}

func (r *taskRepository) ListStaleSent(ctx context.Context, staleBefore time.Time) ([]domain.Task, error) {
	var rows []domain.Task
	err := r.db.SelectContext(ctx, &rows,
		`SELECT t.* FROM tasks t JOIN ref_task_status s ON s.id = t.task_status_id
		 WHERE s.code = ? AND t.is_deleted = 0 AND t.sent_at IS NOT NULL AND t.sent_at < ?`,
		domain.TaskStatusSent, staleBefore)
	if err != nil {
		return nil, translateErr(err)
	}
	return rows, nil
}

func (r *taskRepository) HasPendingForDevice(ctx context.Context, deviceID uint64) (bool, error) {
	var count int
	err := r.db.GetContext(ctx, &count,
		`SELECT COUNT(*) FROM tasks t JOIN ref_task_status s ON s.id = t.task_status_id
		 WHERE t.device_id = ? AND t.is_deleted = 0 AND s.code IN (?, ?)`,
		deviceID, domain.TaskStatusPending, domain.TaskStatusQueued)
	if err != nil {
		return false, translateErr(err)
	}
	return count > 0, nil
}

func (r *taskRepository) List(ctx context.Context, f domain.TaskFilter, p domain.Pagination) ([]domain.Task, int, error) {
	where := []string{"t.is_deleted = 0"}
	args := []interface{}{}
	joins := ""
	if f.DeviceID != nil {
		where = append(where, "t.device_id = ?")
		args = append(args, *f.DeviceID)
	}
	if f.TaskStatusID != nil {
		where = append(where, "t.task_status_id = ?")
		args = append(args, *f.TaskStatusID)
	}
	if f.TaskTypeCode != "" {
		joins = "JOIN ref_task_types tt ON tt.id = t.task_type_id"
		where = append(where, "tt.code = ?")
		args = append(args, f.TaskTypeCode)
	}
	whereSQL := strings.Join(where, " AND ")

	var total int
	countQ := "SELECT COUNT(*) FROM tasks t " + joins + " WHERE " + whereSQL
	if err := r.db.GetContext(ctx, &total, countQ, args...); err != nil {
		return nil, 0, translateErr(err)
	}

	var rows []domain.Task
	listQ := "SELECT t.* FROM tasks t " + joins + " WHERE " + whereSQL + " ORDER BY t.id DESC LIMIT ? OFFSET ?"
	args = append(args, p.Limit(), p.Offset())
	if err := r.db.SelectContext(ctx, &rows, listQ, args...); err != nil {
		return nil, 0, translateErr(err)
	}
	return rows, total, nil
}

func (r *taskRepository) UpdateStatus(ctx context.Context, id, statusID uint64, updatedBy *uint64) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE tasks SET task_status_id = ?, updated_by = ? WHERE id = ?`, statusID, updatedBy, id)
	return translateErr(err)
}

func (r *taskRepository) MarkSent(ctx context.Context, id uint64, sentAt time.Time) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE tasks t JOIN ref_task_status s ON s.code = ? SET t.task_status_id = s.id, t.sent_at = ? WHERE t.id = ?`,
		domain.TaskStatusSent, sentAt, id)
	return translateErr(err)
}

func (r *taskRepository) MarkCompleted(ctx context.Context, id uint64, response []byte, completedAt time.Time) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE tasks t JOIN ref_task_status s ON s.code = ? SET t.task_status_id = s.id, t.response = ?, t.completed_at = ? WHERE t.id = ?`,
		domain.TaskStatusCompleted, response, completedAt, id)
	return translateErr(err)
}

func (r *taskRepository) MarkFailed(ctx context.Context, id uint64, statusID uint64, errMsg string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE tasks SET task_status_id = ?, error_message = ? WHERE id = ?`, statusID, errMsg, id)
	return translateErr(err)
}

func (r *taskRepository) SetErrorMessage(ctx context.Context, id uint64, errMsg string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE tasks SET error_message = ? WHERE id = ?`, errMsg, id)
	return translateErr(err)
}

func (r *taskRepository) IncrementRetry(ctx context.Context, id uint64) error {
	_, err := r.db.ExecContext(ctx, `UPDATE tasks SET retry_count = retry_count + 1 WHERE id = ?`, id)
	return translateErr(err)
}

func (r *taskRepository) Cancel(ctx context.Context, id uint64, updatedBy *uint64) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE tasks t JOIN ref_task_status s ON s.code = ? SET t.task_status_id = s.id, t.updated_by = ? WHERE t.id = ?`,
		domain.TaskStatusCancelled, updatedBy, id)
	return translateErr(err)
}
