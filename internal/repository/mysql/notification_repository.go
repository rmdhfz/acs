package mysql

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"

	"acs/internal/domain"
)

// notificationRepository -- query notifikasi persisten (migrations/0030). Hanya query berparameter; semua
// keputusan (penerima, preferensi, dedupe, throttle) ada di usecase/notification.
type notificationRepository struct{ db *sqlx.DB }

func NewNotificationRepository(db *sqlx.DB) domain.NotificationRepository {
	return &notificationRepository{db: db}
}

func (r *notificationRepository) InsertBatch(ctx context.Context, items []*domain.Notification) error {
	if len(items) == 0 {
		return nil
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return translateErr(err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, n := range items {
		res, err := tx.ExecContext(ctx, `INSERT INTO notifications
			(notification_uuid, tenant_id, user_id, type_id, severity_id, title, body, link, entity_type, entity_id, dedupe_key, read_at, emailed_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			n.NotificationUUID, n.TenantID, n.UserID, n.TypeID, n.SeverityID, n.Title, n.Body, n.Link, n.EntityType, n.EntityID,
			n.DedupeKey, n.ReadAt, n.EmailedAt)
		if err != nil {
			return translateErr(err)
		}
		id, _ := res.LastInsertId()
		n.ID = uint64(id)
	}
	if err := tx.Commit(); err != nil {
		return translateErr(err)
	}
	// created_at diisi DEFAULT CURRENT_TIMESTAMP oleh DB; pemanggil hanya butuh ID/UUID.
	return nil
}

func (r *notificationRepository) List(ctx context.Context, userID uint64, unreadOnly bool, p domain.Pagination) ([]domain.NotificationItem, int, int, error) {
	where := "n.user_id = ?"
	if unreadOnly {
		where += " AND n.read_at IS NULL"
	}
	var total int
	if err := r.db.GetContext(ctx, &total, "SELECT COUNT(*) FROM notifications n WHERE "+where, userID); err != nil {
		return nil, 0, 0, translateErr(err)
	}
	unread, err := r.CountUnread(ctx, userID)
	if err != nil {
		return nil, 0, 0, err
	}
	items := []domain.NotificationItem{}
	err = r.db.SelectContext(ctx, &items, `SELECT n.notification_uuid, t.code AS type_code, t.name AS type_name, s.code AS severity,
			n.title, n.body, n.link, n.entity_type, n.entity_id, n.read_at, n.created_at
		FROM notifications n
		JOIN ref_notification_types t ON t.id = n.type_id
		JOIN ref_notification_severity s ON s.id = n.severity_id
		WHERE `+where+` ORDER BY n.id DESC LIMIT ? OFFSET ?`, userID, p.Limit(), p.Offset())
	if err != nil {
		return nil, 0, 0, translateErr(err)
	}
	return items, total, unread, nil
}

func (r *notificationRepository) CountUnread(ctx context.Context, userID uint64) (int, error) {
	var n int
	err := r.db.GetContext(ctx, &n, `SELECT COUNT(*) FROM notifications WHERE user_id = ? AND read_at IS NULL`, userID)
	return n, translateErr(err)
}

func (r *notificationRepository) MarkRead(ctx context.Context, userID uint64, uuids []string, at time.Time) (int64, error) {
	if len(uuids) == 0 {
		return 0, nil
	}
	q, args, err := sqlx.In(`UPDATE notifications SET read_at = ? WHERE user_id = ? AND read_at IS NULL AND notification_uuid IN (?)`, at, userID, uuids)
	if err != nil {
		return 0, translateErr(err)
	}
	res, err := r.db.ExecContext(ctx, r.db.Rebind(q), args...)
	if err != nil {
		return 0, translateErr(err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

func (r *notificationRepository) MarkAllRead(ctx context.Context, userID uint64, at time.Time) (int64, error) {
	res, err := r.db.ExecContext(ctx, `UPDATE notifications SET read_at = ? WHERE user_id = ? AND read_at IS NULL`, at, userID)
	if err != nil {
		return 0, translateErr(err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

const notificationTypeSelect = `SELECT t.id, t.code, t.name, t.description, t.severity_id, s.code AS severity,
	t.default_in_app, t.default_email, t.is_mandatory
	FROM ref_notification_types t JOIN ref_notification_severity s ON s.id = t.severity_id`

func (r *notificationRepository) ListTypes(ctx context.Context) ([]domain.NotificationType, error) {
	out := []domain.NotificationType{}
	err := r.db.SelectContext(ctx, &out, notificationTypeSelect+` WHERE t.is_deleted = 0 ORDER BY t.id`)
	return out, translateErr(err)
}

func (r *notificationRepository) GetTypeByCode(ctx context.Context, code string) (*domain.NotificationType, error) {
	var t domain.NotificationType
	if err := r.db.GetContext(ctx, &t, notificationTypeSelect+` WHERE t.code = ? AND t.is_deleted = 0`, code); err != nil {
		return nil, translateErr(err)
	}
	return &t, nil
}

func (r *notificationRepository) SeverityIDByCode(ctx context.Context, code string) (uint64, error) {
	var id uint64
	err := r.db.GetContext(ctx, &id, `SELECT id FROM ref_notification_severity WHERE code = ? AND is_deleted = 0`, code)
	return id, translateErr(err)
}

func (r *notificationRepository) ListPreferences(ctx context.Context, userID uint64) ([]domain.NotificationPreference, error) {
	out := []domain.NotificationPreference{}
	err := r.db.SelectContext(ctx, &out, `SELECT user_id, type_id, in_app, email FROM notification_preferences WHERE user_id = ?`, userID)
	return out, translateErr(err)
}

func (r *notificationRepository) ListPreferencesForUsers(ctx context.Context, typeID uint64, userIDs []uint64) ([]domain.NotificationPreference, error) {
	out := []domain.NotificationPreference{}
	if len(userIDs) == 0 {
		return out, nil
	}
	q, args, err := sqlx.In(`SELECT user_id, type_id, in_app, email FROM notification_preferences WHERE type_id = ? AND user_id IN (?)`, typeID, userIDs)
	if err != nil {
		return nil, translateErr(err)
	}
	err = r.db.SelectContext(ctx, &out, r.db.Rebind(q), args...)
	return out, translateErr(err)
}

func (r *notificationRepository) UpsertPreferences(ctx context.Context, userID uint64, prefs []domain.NotificationPreference) error {
	if len(prefs) == 0 {
		return nil
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return translateErr(err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, p := range prefs {
		if _, err := tx.ExecContext(ctx, `INSERT INTO notification_preferences (user_id, type_id, in_app, email) VALUES (?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE in_app = VALUES(in_app), email = VALUES(email)`, userID, p.TypeID, p.InApp, p.Email); err != nil {
			return translateErr(err)
		}
	}
	return translateErr(tx.Commit())
}

func (r *notificationRepository) ExistsDedupe(ctx context.Context, typeID uint64, tenantID *uint64, key string, since time.Time) (bool, error) {
	var ok bool
	err := r.db.GetContext(ctx, &ok, `SELECT EXISTS(SELECT 1 FROM notifications
		WHERE type_id = ? AND tenant_id <=> ? AND dedupe_key = ? AND created_at >= ?)`, typeID, tenantID, key, since)
	return ok, translateErr(err)
}

func (r *notificationRepository) EmailedUsersSince(ctx context.Context, typeID uint64, userIDs []uint64, since time.Time) (map[uint64]bool, error) {
	out := map[uint64]bool{}
	if len(userIDs) == 0 {
		return out, nil
	}
	q, args, err := sqlx.In(`SELECT DISTINCT user_id FROM notifications WHERE type_id = ? AND emailed_at >= ? AND user_id IN (?)`, typeID, since, userIDs)
	if err != nil {
		return nil, translateErr(err)
	}
	var ids []uint64
	if err := r.db.SelectContext(ctx, &ids, r.db.Rebind(q), args...); err != nil {
		return nil, translateErr(err)
	}
	for _, id := range ids {
		out[id] = true
	}
	return out, nil
}

func (r *notificationRepository) ListRecipients(ctx context.Context, tenantID *uint64, roles []string) ([]domain.NotificationRecipient, error) {
	out := []domain.NotificationRecipient{}
	if len(roles) == 0 {
		return out, nil
	}
	query := `SELECT DISTINCT u.id, u.tenant_id, u.email, u.full_name
		FROM users u
		JOIN user_roles ur ON ur.user_id = u.id
		JOIN ref_roles rr ON rr.id = ur.role_id AND rr.is_deleted = 0
		LEFT JOIN tenants t ON t.id = u.tenant_id
		WHERE u.is_deleted = 0 AND u.is_active = 1 AND rr.code IN (?)
		  AND (u.tenant_id IS NULL OR (t.is_active = 1 AND t.is_deleted = 0))`
	args := []interface{}{roles}
	if tenantID != nil {
		query += ` AND u.tenant_id = ?`
		args = append(args, *tenantID)
	}
	q, a, err := sqlx.In(query+` ORDER BY u.id`, args...)
	if err != nil {
		return nil, translateErr(err)
	}
	if err := r.db.SelectContext(ctx, &out, r.db.Rebind(q), a...); err != nil {
		return nil, translateErr(err)
	}
	return out, nil
}

func (r *notificationRepository) GetRecipient(ctx context.Context, userID uint64) (*domain.NotificationRecipient, error) {
	var rc domain.NotificationRecipient
	err := r.db.GetContext(ctx, &rc, `SELECT u.id, u.tenant_id, u.email, u.full_name FROM users u
		LEFT JOIN tenants t ON t.id = u.tenant_id
		WHERE u.id = ? AND u.is_deleted = 0 AND u.is_active = 1
		  AND (u.tenant_id IS NULL OR (t.is_active = 1 AND t.is_deleted = 0))`, userID)
	if err != nil {
		return nil, translateErr(err)
	}
	if err := r.db.SelectContext(ctx, &rc.Roles, `SELECT rr.code FROM user_roles ur
		JOIN ref_roles rr ON rr.id = ur.role_id AND rr.is_deleted = 0 WHERE ur.user_id = ?`, userID); err != nil {
		return nil, translateErr(err)
	}
	return &rc, nil
}

func (r *notificationRepository) TenantDeviceHealth(ctx context.Context) ([]domain.TenantDeviceHealth, error) {
	out := []domain.TenantDeviceHealth{}
	err := r.db.SelectContext(ctx, &out, `SELECT d.tenant_id, COUNT(*) AS total, CAST(COALESCE(SUM(s.code = ?), 0) AS SIGNED) AS offline
		FROM devices d
		JOIN ref_device_status s ON s.id = d.device_status_id
		JOIN tenants t ON t.id = d.tenant_id AND t.is_active = 1 AND t.is_deleted = 0
		WHERE d.is_deleted = 0 AND d.last_inform_at IS NOT NULL
		GROUP BY d.tenant_id`, domain.DeviceStatusOffline)
	return out, translateErr(err)
}

func (r *notificationRepository) FailedTasksSince(ctx context.Context, since time.Time) ([]domain.TenantCount, error) {
	out := []domain.TenantCount{}
	err := r.db.SelectContext(ctx, &out, `SELECT d.tenant_id, COUNT(*) AS n
		FROM tasks k
		JOIN ref_task_status s ON s.id = k.task_status_id AND s.code = ?
		JOIN devices d ON d.id = k.device_id
		JOIN tenants t ON t.id = d.tenant_id AND t.is_active = 1 AND t.is_deleted = 0
		WHERE k.is_deleted = 0 AND k.updated_at >= ?
		GROUP BY d.tenant_id`, domain.TaskStatusFailed, since)
	return out, translateErr(err)
}
