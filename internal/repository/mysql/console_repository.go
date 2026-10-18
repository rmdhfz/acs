package mysql

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"

	"acs/internal/domain"
)

// Penyimpanan fitur Console (migrations/0033): layout menu, laporan bug, riwayat impor/ekspor. Hanya query; semua
// keputusan akses (siapa boleh melihat baris siapa) ada di usecase, dan filter kepemilikan/tenant DIKIRIM ke sini
// sebagai kondisi WHERE berparameter -- repository tidak pernah merangkai SQL dari input.

// ---- layout menu ----

type menuLayoutRepository struct{ db *sqlx.DB }

func NewMenuLayoutRepository(db *sqlx.DB) domain.MenuLayoutRepository {
	return &menuLayoutRepository{db: db}
}

func (r *menuLayoutRepository) Get(ctx context.Context, userID uint64) (string, error) {
	var s string
	if err := r.db.GetContext(ctx, &s, `SELECT layout_json FROM user_menu_layouts WHERE user_id = ? AND is_deleted = 0`, userID); err != nil {
		return "", translateErr(err)
	}
	return s, nil
}

// Upsert menghidupkan baris yang sudah di-reset (is_deleted=1) pada kunci unik user_id yang sama.
func (r *menuLayoutRepository) Upsert(ctx context.Context, userID uint64, layoutJSON string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO user_menu_layouts (user_id, layout_json, created_by, updated_by) VALUES (?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE layout_json = VALUES(layout_json), updated_by = VALUES(updated_by),
		                         is_deleted = 0, deleted_at = NULL, deleted_by = NULL`,
		userID, layoutJSON, userID, userID)
	return translateErr(err)
}

func (r *menuLayoutRepository) Reset(ctx context.Context, userID uint64) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE user_menu_layouts SET is_deleted = 1, deleted_at = NOW(), deleted_by = ? WHERE user_id = ? AND is_deleted = 0`, userID, userID)
	return translateErr(err)
}

// ---- laporan bug ----

type bugReportRepository struct{ db *sqlx.DB }

func NewBugReportRepository(db *sqlx.DB) domain.BugReportRepository {
	return &bugReportRepository{db: db}
}

const bugSelect = `SELECT b.id, b.report_uuid, b.tenant_id, t.name AS tenant_name, b.reporter_id,
		COALESCE(u.username, '') AS reporter_name, COALESCE(u.email, '') AS reporter_email,
		(b.impersonator_id IS NOT NULL) AS impersonated,
		b.title, b.description, b.expected_behavior AS expected,
		EXISTS(SELECT 1 FROM bug_report_screenshots sc WHERE sc.bug_report_id = b.id AND sc.is_deleted = 0) AS has_screenshot,
		b.page_path, b.user_agent, b.app_version,
		sv.code AS severity, st.code AS status, b.admin_note, b.emailed_at, b.created_at, b.updated_at
	FROM bug_reports b
	JOIN ref_bug_report_severity sv ON sv.id = b.severity_id
	JOIN ref_bug_report_status st ON st.id = b.status_id
	JOIN users u ON u.id = b.reporter_id
	LEFT JOIN tenants t ON t.id = b.tenant_id
	WHERE b.is_deleted = 0`

func (r *bugReportRepository) Create(ctx context.Context, in domain.BugReportCreate) (*domain.BugReport, error) {
	// Laporan + tangkapan layar atomik: tidak ada laporan setengah jadi bila penyimpanan gambar gagal.
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, translateErr(err)
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx,
		`INSERT INTO bug_reports (report_uuid, tenant_id, reporter_id, impersonator_id, title, description, expected_behavior, page_path, user_agent, app_version,
		                          severity_id, status_id, created_by)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
		         (SELECT id FROM ref_bug_report_severity WHERE code = ?), (SELECT id FROM ref_bug_report_status WHERE code = 'OPEN'), ?)`,
		in.UUID, in.TenantID, in.ReporterID, in.ImpersonatorID, in.Title, in.Description, in.Expected, in.PagePath, in.UserAgent, in.AppVersion,
		in.SeverityCode, in.ReporterID)
	if err != nil {
		return nil, translateErr(err)
	}
	if in.Screenshot != nil {
		id, _ := res.LastInsertId()
		sc := in.Screenshot
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO bug_report_screenshots (bug_report_id, content_type, width, height, size_bytes, sha256, data, created_by)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			id, sc.ContentType, sc.Width, sc.Height, len(sc.Data), sc.SHA256, sc.Data, in.ReporterID); err != nil {
			return nil, translateErr(err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, translateErr(err)
	}
	return r.GetByUUID(ctx, in.UUID)
}

func (r *bugReportRepository) GetScreenshot(ctx context.Context, reportID uint64) (*domain.BugScreenshot, error) {
	var sc domain.BugScreenshot
	row := r.db.QueryRowxContext(ctx, `SELECT content_type, width, height, sha256, data FROM bug_report_screenshots WHERE bug_report_id = ? AND is_deleted = 0`, reportID)
	if err := row.Scan(&sc.ContentType, &sc.Width, &sc.Height, &sc.SHA256, &sc.Data); err != nil {
		return nil, translateErr(err)
	}
	return &sc, nil
}

func (r *bugReportRepository) GetByUUID(ctx context.Context, uuid string) (*domain.BugReport, error) {
	var b domain.BugReport
	if err := r.db.GetContext(ctx, &b, bugSelect+` AND b.report_uuid = ?`, uuid); err != nil {
		return nil, translateErr(err)
	}
	return &b, nil
}

func (r *bugReportRepository) List(ctx context.Context, f domain.BugReportFilter, p domain.Pagination) ([]domain.BugReport, int, error) {
	where, args := ``, []any{}
	if f.ReporterID != nil {
		where += ` AND b.reporter_id = ?`
		args = append(args, *f.ReporterID)
	}
	if f.TenantID != nil {
		where += ` AND b.tenant_id = ?`
		args = append(args, *f.TenantID)
	}
	if f.Status != "" {
		where += ` AND st.code = ?`
		args = append(args, f.Status)
	}
	var total int
	if err := r.db.GetContext(ctx, &total, `SELECT COUNT(*) FROM bug_reports b JOIN ref_bug_report_status st ON st.id = b.status_id WHERE b.is_deleted = 0`+where, args...); err != nil {
		return nil, 0, translateErr(err)
	}
	out := []domain.BugReport{}
	if err := r.db.SelectContext(ctx, &out, bugSelect+where+` ORDER BY b.created_at DESC, b.id DESC LIMIT ? OFFSET ?`,
		append(args, p.Limit(), p.Offset())...); err != nil {
		return nil, 0, translateErr(err)
	}
	return out, total, nil
}

func (r *bugReportRepository) UpdateStatus(ctx context.Context, uuid, statusCode string, note *string, by uint64) error {
	var exists bool
	if err := r.db.GetContext(ctx, &exists, `SELECT EXISTS(SELECT 1 FROM bug_reports WHERE report_uuid = ? AND is_deleted = 0)`, uuid); err != nil {
		return translateErr(err)
	}
	if !exists {
		return domain.ErrNotFound
	}
	_, err := r.db.ExecContext(ctx,
		`UPDATE bug_reports SET status_id = (SELECT id FROM ref_bug_report_status WHERE code = ?), admin_note = ?, updated_by = ?
		 WHERE report_uuid = ? AND is_deleted = 0`, statusCode, note, by, uuid)
	return translateErr(err)
}

func (r *bugReportRepository) MarkEmailed(ctx context.Context, id uint64) error {
	_, err := r.db.ExecContext(ctx, `UPDATE bug_reports SET emailed_at = NOW() WHERE id = ?`, id)
	return translateErr(err)
}

func (r *bugReportRepository) CountByReporterSince(ctx context.Context, reporterID uint64, since time.Time) (int, error) {
	var n int
	err := r.db.GetContext(ctx, &n, `SELECT COUNT(*) FROM bug_reports WHERE reporter_id = ? AND created_at >= ?`, reporterID, since)
	return n, translateErr(err)
}

// ---- riwayat impor/ekspor ----

type dataTransferRepository struct{ db *sqlx.DB }

func NewDataTransferRepository(db *sqlx.DB) domain.DataTransferRepository {
	return &dataTransferRepository{db: db}
}

const transferSelect = `SELECT d.id, d.transfer_uuid, d.tenant_id, t.name AS tenant_name, d.user_id, COALESCE(u.username, '') AS user_name,
		(d.impersonator_id IS NOT NULL) AS impersonated, k.code AS kind, st.code AS status, d.module, d.format, d.file_name,
		d.total_rows, d.success_rows, d.failed_rows, d.detail, d.source, d.emailed_at, d.created_at
	FROM data_transfer_logs d
	JOIN ref_data_transfer_kind k ON k.id = d.kind_id
	JOIN ref_data_transfer_status st ON st.id = d.status_id
	JOIN users u ON u.id = d.user_id
	LEFT JOIN tenants t ON t.id = d.tenant_id
	WHERE d.is_deleted = 0`

func (r *dataTransferRepository) Create(ctx context.Context, in domain.DataTransferCreate) (*domain.DataTransferLog, error) {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO data_transfer_logs (transfer_uuid, tenant_id, user_id, impersonator_id, kind_id, status_id, module, format, file_name,
		                                 total_rows, success_rows, failed_rows, detail, source, created_by)
		 VALUES (?, ?, ?, ?, (SELECT id FROM ref_data_transfer_kind WHERE code = ?), (SELECT id FROM ref_data_transfer_status WHERE code = ?),
		         ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		in.UUID, in.TenantID, in.UserID, in.ImpersonatorID, in.KindCode, in.StatusCode, in.Module, in.Format, in.FileName,
		in.TotalRows, in.SuccessRows, in.FailedRows, in.Detail, in.Source, in.UserID)
	if err != nil {
		return nil, translateErr(err)
	}
	return r.GetByUUID(ctx, in.UUID)
}

func (r *dataTransferRepository) GetByUUID(ctx context.Context, uuid string) (*domain.DataTransferLog, error) {
	var d domain.DataTransferLog
	if err := r.db.GetContext(ctx, &d, transferSelect+` AND d.transfer_uuid = ?`, uuid); err != nil {
		return nil, translateErr(err)
	}
	return &d, nil
}

func (r *dataTransferRepository) List(ctx context.Context, f domain.DataTransferFilter, p domain.Pagination) ([]domain.DataTransferLog, int, error) {
	where, args := ``, []any{}
	if f.TenantID != nil {
		where += ` AND d.tenant_id = ?`
		args = append(args, *f.TenantID)
	}
	if f.Kind != "" {
		where += ` AND k.code = ?`
		args = append(args, f.Kind)
	}
	if f.Module != "" {
		where += ` AND d.module = ?`
		args = append(args, f.Module)
	}
	var total int
	if err := r.db.GetContext(ctx, &total, `SELECT COUNT(*) FROM data_transfer_logs d JOIN ref_data_transfer_kind k ON k.id = d.kind_id WHERE d.is_deleted = 0`+where, args...); err != nil {
		return nil, 0, translateErr(err)
	}
	out := []domain.DataTransferLog{}
	if err := r.db.SelectContext(ctx, &out, transferSelect+where+` ORDER BY d.created_at DESC, d.id DESC LIMIT ? OFFSET ?`,
		append(args, p.Limit(), p.Offset())...); err != nil {
		return nil, 0, translateErr(err)
	}
	return out, total, nil
}

func (r *dataTransferRepository) MarkEmailed(ctx context.Context, id uint64) error {
	_, err := r.db.ExecContext(ctx, `UPDATE data_transfer_logs SET emailed_at = NOW() WHERE id = ?`, id)
	return translateErr(err)
}

func (r *dataTransferRepository) CountEmailedByUserSince(ctx context.Context, userID uint64, since time.Time) (int, error) {
	var n int
	err := r.db.GetContext(ctx, &n, `SELECT COUNT(*) FROM data_transfer_logs WHERE user_id = ? AND emailed_at IS NOT NULL AND emailed_at >= ?`, userID, since)
	return n, translateErr(err)
}
