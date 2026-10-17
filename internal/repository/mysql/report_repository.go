package mysql

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"

	"acs/internal/domain"
)

// reportRepository — query agregat laporan SaaS (superadmin). Hanya SELECT berparameter; tanpa logika keputusan.
type reportRepository struct{ db *sqlx.DB }

func NewReportRepository(db *sqlx.DB) domain.ReportRepository { return &reportRepository{db: db} }

func (r *reportRepository) Platform(ctx context.Context, now time.Time) (domain.PlatformStats, error) {
	var out domain.PlatformStats
	d7, d30, h24 := now.AddDate(0, 0, -7), now.AddDate(0, 0, -30), now.Add(-24*time.Hour)
	err := r.db.GetContext(ctx, &out, `SELECT
 (SELECT COUNT(*) FROM tenants WHERE is_deleted = 0) AS tenants_total,
 (SELECT COUNT(*) FROM tenants WHERE is_deleted = 0 AND is_active = 1) AS tenants_active,
 (SELECT COUNT(*) FROM tenants WHERE is_deleted = 0 AND created_at >= ?) AS tenants_new_30d,
 (SELECT COUNT(*) FROM devices WHERE is_deleted = 0) AS devices_total,
 (SELECT COUNT(*) FROM devices d JOIN ref_device_status s ON s.id = d.device_status_id WHERE d.is_deleted = 0 AND s.code = ?) AS devices_online,
 (SELECT COUNT(*) FROM devices WHERE is_deleted = 0 AND created_at >= ?) AS devices_new_7d,
 (SELECT COUNT(*) FROM devices WHERE is_deleted = 0 AND created_at >= ?) AS devices_new_30d,
 (SELECT COUNT(*) FROM users WHERE is_deleted = 0) AS users_total,
 (SELECT COUNT(*) FROM users WHERE is_deleted = 0 AND last_login_at >= ?) AS users_active_30d,
 (SELECT COUNT(*) FROM tasks WHERE is_deleted = 0 AND created_at >= ?) AS tasks_30d,
 (SELECT COUNT(*) FROM tasks t JOIN ref_task_status s ON s.id = t.task_status_id WHERE t.is_deleted = 0 AND t.created_at >= ? AND s.code = ?) AS tasks_failed_30d,
 (SELECT COUNT(*) FROM device_sessions WHERE started_at >= ?) AS sessions_24h,
 (SELECT COUNT(*) FROM activity_logs WHERE action = 'LOGIN_FAILED' AND created_at >= ?) AS failed_logins_24h,
 (SELECT COUNT(*) FROM webhook_subscriptions WHERE is_deleted = 0 AND is_active = 1) AS webhooks_active,
 (SELECT COUNT(*) FROM firmware_files WHERE is_deleted = 0) AS firmware_files`,
		d30, domain.DeviceStatusOnline, d7, d30, d30, d30, d30, domain.TaskStatusFailed, h24, h24)
	return out, translateErr(err)
}

func (r *reportRepository) Tenants(ctx context.Context, now time.Time) ([]domain.TenantReport, error) {
	d30 := now.AddDate(0, 0, -30)

	var base []struct {
		ID         uint64    `db:"id"`
		Code       string    `db:"code"`
		Name       string    `db:"name"`
		IsActive   bool      `db:"is_active"`
		CreatedAt  time.Time `db:"created_at"`
		MaxPending *uint32   `db:"max_pending_tasks"`
	}
	if err := r.db.SelectContext(ctx, &base,
		`SELECT id, code, name, is_active, created_at, max_pending_tasks FROM tenants WHERE is_deleted = 0 ORDER BY name`); err != nil {
		return nil, translateErr(err)
	}
	rows := make(map[uint64]*domain.TenantReport, len(base))
	out := make([]domain.TenantReport, len(base))
	for i, b := range base {
		out[i] = domain.TenantReport{TenantID: b.ID, Code: b.Code, Name: b.Name, IsActive: b.IsActive, CreatedAt: b.CreatedAt, MaxPending: b.MaxPending, HealthReasons: []string{}}
		rows[b.ID] = &out[i]
	}

	var dev []struct {
		TenantID uint64     `db:"tenant_id"`
		Total    int        `db:"total"`
		Online   int        `db:"online"`
		LastInf  *time.Time `db:"last_inform"`
	}
	if err := r.db.SelectContext(ctx, &dev, `SELECT d.tenant_id, COUNT(*) AS total, COALESCE(SUM(s.code = ?), 0) AS online, MAX(d.last_inform_at) AS last_inform
		FROM devices d LEFT JOIN ref_device_status s ON s.id = d.device_status_id
		WHERE d.is_deleted = 0 AND d.tenant_id IS NOT NULL GROUP BY d.tenant_id`, domain.DeviceStatusOnline); err != nil {
		return nil, translateErr(err)
	}
	for _, x := range dev {
		if t := rows[x.TenantID]; t != nil {
			t.Devices, t.DevicesOnline, t.LastInformAt = x.Total, x.Online, x.LastInf
		}
	}

	var usr []struct {
		TenantID uint64     `db:"tenant_id"`
		Total    int        `db:"total"`
		Active   int        `db:"active"`
		LastLog  *time.Time `db:"last_login"`
	}
	if err := r.db.SelectContext(ctx, &usr, `SELECT tenant_id, COUNT(*) AS total, COALESCE(SUM(last_login_at >= ?), 0) AS active, MAX(last_login_at) AS last_login
		FROM users WHERE is_deleted = 0 AND tenant_id IS NOT NULL GROUP BY tenant_id`, d30); err != nil {
		return nil, translateErr(err)
	}
	for _, x := range usr {
		if t := rows[x.TenantID]; t != nil {
			t.Users, t.UsersActive30d, t.LastLoginAt = x.Total, x.Active, x.LastLog
		}
	}

	var tsk []struct {
		TenantID uint64 `db:"tenant_id"`
		Total    int    `db:"total"`
		Failed   int    `db:"failed"`
	}
	if err := r.db.SelectContext(ctx, &tsk, `SELECT d.tenant_id, COUNT(*) AS total, COALESCE(SUM(s.code = ?), 0) AS failed
		FROM tasks t JOIN devices d ON d.id = t.device_id JOIN ref_task_status s ON s.id = t.task_status_id
		WHERE t.is_deleted = 0 AND t.created_at >= ? AND d.tenant_id IS NOT NULL GROUP BY d.tenant_id`, domain.TaskStatusFailed, d30); err != nil {
		return nil, translateErr(err)
	}
	for _, x := range tsk {
		if t := rows[x.TenantID]; t != nil {
			t.Tasks30d, t.TasksFailed30d = x.Total, x.Failed
		}
	}

	var pend []struct {
		TenantID uint64 `db:"tenant_id"`
		N        int    `db:"n"`
	}
	if err := r.db.SelectContext(ctx, &pend, `SELECT d.tenant_id, COUNT(*) AS n
		FROM tasks t JOIN devices d ON d.id = t.device_id JOIN ref_task_status s ON s.id = t.task_status_id
		WHERE t.is_deleted = 0 AND s.code IN (?, ?) AND d.tenant_id IS NOT NULL GROUP BY d.tenant_id`, domain.TaskStatusPending, domain.TaskStatusQueued); err != nil {
		return nil, translateErr(err)
	}
	for _, x := range pend {
		if t := rows[x.TenantID]; t != nil {
			t.PendingTasks = x.N
		}
	}

	var ses []struct {
		TenantID uint64 `db:"tenant_id"`
		N        int    `db:"n"`
	}
	if err := r.db.SelectContext(ctx, &ses, `SELECT d.tenant_id, COUNT(*) AS n FROM device_sessions ds JOIN devices d ON d.id = ds.device_id
		WHERE ds.started_at >= ? AND d.tenant_id IS NOT NULL GROUP BY d.tenant_id`, d30); err != nil {
		return nil, translateErr(err)
	}
	for _, x := range ses {
		if t := rows[x.TenantID]; t != nil {
			t.Sessions30d = x.N
		}
	}

	var wh []struct {
		TenantID uint64 `db:"tenant_id"`
		N        int    `db:"n"`
	}
	if err := r.db.SelectContext(ctx, &wh, `SELECT tenant_id, COUNT(*) AS n FROM webhook_subscriptions
		WHERE is_deleted = 0 AND is_active = 1 AND tenant_id IS NOT NULL GROUP BY tenant_id`); err != nil {
		return nil, translateErr(err)
	}
	for _, x := range wh {
		if t := rows[x.TenantID]; t != nil {
			t.Webhooks = x.N
		}
	}
	return out, nil
}

func (r *reportRepository) Vendors(ctx context.Context) ([]domain.VendorStat, error) {
	var out []domain.VendorStat
	err := r.db.SelectContext(ctx, &out, `SELECT COALESCE(v.name, 'Tidak diketahui') AS vendor, COUNT(*) AS devices
		FROM devices d LEFT JOIN ref_vendors v ON v.id = d.vendor_id
		WHERE d.is_deleted = 0 GROUP BY v.id, v.name ORDER BY devices DESC LIMIT 20`)
	return out, translateErr(err)
}

func (r *reportRepository) Daily(ctx context.Context, since time.Time) (map[string]domain.DailyPoint, error) {
	out := map[string]domain.DailyPoint{}
	type kv struct {
		Day string `db:"day"`
		N   int    `db:"n"`
	}
	collect := func(query string, args []any, set func(p *domain.DailyPoint, n int)) error {
		var rows []kv
		if err := r.db.SelectContext(ctx, &rows, query, args...); err != nil {
			return translateErr(err)
		}
		for _, x := range rows {
			p := out[x.Day]
			p.Date = x.Day
			set(&p, x.N)
			out[x.Day] = p
		}
		return nil
	}
	if err := collect(`SELECT DATE_FORMAT(created_at, '%Y-%m-%d') AS day, COUNT(*) AS n FROM devices WHERE is_deleted = 0 AND created_at >= ? GROUP BY day`,
		[]any{since}, func(p *domain.DailyPoint, n int) { p.NewDevices = n }); err != nil {
		return nil, err
	}
	if err := collect(`SELECT DATE_FORMAT(started_at, '%Y-%m-%d') AS day, COUNT(*) AS n FROM device_sessions WHERE started_at >= ? GROUP BY day`,
		[]any{since}, func(p *domain.DailyPoint, n int) { p.Sessions = n }); err != nil {
		return nil, err
	}
	if err := collect(`SELECT DATE_FORMAT(created_at, '%Y-%m-%d') AS day, COUNT(*) AS n FROM tasks WHERE is_deleted = 0 AND created_at >= ? GROUP BY day`,
		[]any{since}, func(p *domain.DailyPoint, n int) { p.Tasks = n }); err != nil {
		return nil, err
	}
	if err := collect(`SELECT DATE_FORMAT(t.created_at, '%Y-%m-%d') AS day, COUNT(*) AS n FROM tasks t JOIN ref_task_status s ON s.id = t.task_status_id
		WHERE t.is_deleted = 0 AND t.created_at >= ? AND s.code = ? GROUP BY day`,
		[]any{since, domain.TaskStatusFailed}, func(p *domain.DailyPoint, n int) { p.TasksFail = n }); err != nil {
		return nil, err
	}
	return out, nil
}
