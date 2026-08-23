package mysql

import (
	"context"
	"database/sql"
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

func (r *taskRepository) CountPendingForTenant(ctx context.Context, tenantID uint64) (int, error) {
	var count int
	err := r.db.GetContext(ctx, &count,
		`SELECT COUNT(*) FROM tasks t
		 JOIN devices d ON d.id = t.device_id
		 JOIN ref_task_status s ON s.id = t.task_status_id
		 WHERE d.tenant_id = ? AND t.is_deleted = 0 AND s.code IN (?, ?)`,
		tenantID, domain.TaskStatusPending, domain.TaskStatusQueued)
	if err != nil {
		return 0, translateErr(err)
	}
	return count, nil
}

// CountByStatus meng-agregasi jumlah task per status. tasks tidak punya
// tenant_id langsung (task melekat ke device) — JOIN devices dibutuhkan
// hanya saat tenantID != nil untuk membatasi scope tenant (dashboard, FR-26).
func (r *taskRepository) CountByStatus(ctx context.Context, tenantID *uint64) ([]domain.TaskStatusCount, error) {
	where := "t.is_deleted = 0"
	args := []interface{}{}
	joins := ""
	if tenantID != nil {
		joins = "JOIN devices d ON d.id = t.device_id"
		where += " AND d.tenant_id = ?"
		args = append(args, *tenantID)
	}
	var rows []domain.TaskStatusCount
	q := "SELECT t.task_status_id, COUNT(*) AS cnt FROM tasks t " + joins + " WHERE " + where + " GROUP BY t.task_status_id"
	if err := r.db.SelectContext(ctx, &rows, q, args...); err != nil {
		return nil, translateErr(err)
	}
	return rows, nil
}

// AvgCompletionSeconds meng-agregasi rata-rata waktu penyelesaian task
// COMPLETED sejak `since` (metrik observability TECH.md §10). Query-on-scrape
// atas data historis, bukan instrumentasi per-request — lihat komentar
// interface di domain/task.go dan internal/metrics/collector.go.
func (r *taskRepository) AvgCompletionSeconds(ctx context.Context, tenantID *uint64, since time.Time) (*float64, error) {
	where := "t.is_deleted = 0 AND t.completed_at IS NOT NULL AND t.completed_at >= ?"
	args := []interface{}{since}
	joins := ""
	if tenantID != nil {
		joins = "JOIN devices d ON d.id = t.device_id"
		where += " AND d.tenant_id = ?"
		args = append(args, *tenantID)
	}
	q := "SELECT AVG(TIMESTAMPDIFF(SECOND, t.created_at, t.completed_at)) FROM tasks t " + joins + " WHERE " + where
	var avg sql.NullFloat64
	if err := r.db.GetContext(ctx, &avg, q, args...); err != nil {
		return nil, translateErr(err)
	}
	if !avg.Valid {
		return nil, nil
	}
	v := avg.Float64
	return &v, nil
}

// CountFailedByVendor meng-agregasi jumlah task FAILED saat ini per vendor
// (metrik observability TECH.md §10 — indikasi masalah kompatibilitas
// parameter mapping vendor tertentu). JOIN devices dibutuhkan baik untuk
// vendor_id maupun (opsional) tenant_id, beda dengan CountByStatus yang
// hanya JOIN saat tenant-scoped.
func (r *taskRepository) CountFailedByVendor(ctx context.Context, tenantID *uint64) ([]domain.TaskVendorErrorCount, error) {
	where := "t.is_deleted = 0 AND s.code = ?"
	args := []interface{}{domain.TaskStatusFailed}
	if tenantID != nil {
		where += " AND d.tenant_id = ?"
		args = append(args, *tenantID)
	}
	q := `SELECT d.vendor_id AS vendor_id, COUNT(*) AS cnt
		FROM tasks t
		JOIN devices d ON d.id = t.device_id
		JOIN ref_task_status s ON s.id = t.task_status_id
		WHERE ` + where + `
		GROUP BY d.vendor_id`
	var rows []domain.TaskVendorErrorCount
	if err := r.db.SelectContext(ctx, &rows, q, args...); err != nil {
		return nil, translateErr(err)
	}
	return rows, nil
}

func (r *taskRepository) List(ctx context.Context, f domain.TaskFilter, p domain.Pagination) ([]domain.Task, int, error) {
	where := []string{"t.is_deleted = 0"}
	args := []interface{}{}
	joins := []string{}
	if f.DeviceID != nil {
		where = append(where, "t.device_id = ?")
		args = append(args, *f.DeviceID)
	}
	if f.TaskStatusID != nil {
		where = append(where, "t.task_status_id = ?")
		args = append(args, *f.TaskStatusID)
	}
	if f.TaskTypeCode != "" {
		joins = append(joins, "JOIN ref_task_types tt ON tt.id = t.task_type_id")
		where = append(where, "tt.code = ?")
		args = append(args, f.TaskTypeCode)
	}
	if f.TenantID != nil {
		// tasks tidak punya tenant_id langsung — resolve lewat device pemiliknya
		// (RBAC scope tenant wajib, CLAUDE.md).
		joins = append(joins, "JOIN devices d ON d.id = t.device_id")
		where = append(where, "d.tenant_id = ?")
		args = append(args, *f.TenantID)
	}
	joinSQL := strings.Join(joins, " ")
	whereSQL := strings.Join(where, " AND ")

	var total int
	countQ := "SELECT COUNT(*) FROM tasks t " + joinSQL + " WHERE " + whereSQL
	if err := r.db.GetContext(ctx, &total, countQ, args...); err != nil {
		return nil, 0, translateErr(err)
	}

	var rows []domain.Task
	listQ := "SELECT t.* FROM tasks t " + joinSQL + " WHERE " + whereSQL + " ORDER BY t.id DESC LIMIT ? OFFSET ?"
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
