package mysql

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"acs/internal/domain"
)

type captureRepository struct{ db *sqlx.DB }

func NewCaptureRepository(db *sqlx.DB) domain.CaptureRepository { return &captureRepository{db: db} }

func (r *captureRepository) EnableUntil(ctx context.Context, deviceID uint64, until time.Time, by *uint64) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO device_capture_switches (device_id, expires_at, enabled_by) VALUES (?, ?, ?)
		 ON DUPLICATE KEY UPDATE expires_at = VALUES(expires_at), enabled_by = VALUES(enabled_by)`,
		deviceID, until, by)
	return translateErr(err)
}

func (r *captureRepository) Disable(ctx context.Context, deviceID uint64, now time.Time) error {
	_, err := r.db.ExecContext(ctx, `UPDATE device_capture_switches SET expires_at = ? WHERE device_id = ? AND expires_at > ?`, now, deviceID, now)
	return translateErr(err)
}

func (r *captureRepository) Status(ctx context.Context, deviceID uint64, now time.Time) (*domain.CaptureStatus, error) {
	var until *time.Time
	if err := r.db.GetContext(ctx, &until, `SELECT expires_at FROM device_capture_switches WHERE device_id = ?`, deviceID); err != nil {
		if translateErr(err) == domain.ErrNotFound {
			return &domain.CaptureStatus{}, nil
		}
		return nil, translateErr(err)
	}
	return &domain.CaptureStatus{Active: until != nil && until.After(now), Until: until}, nil
}

func (r *captureRepository) ActiveDeviceIDs(ctx context.Context, now time.Time) ([]uint64, error) {
	var ids []uint64
	if err := r.db.SelectContext(ctx, &ids, `SELECT device_id FROM device_capture_switches WHERE expires_at > ?`, now); err != nil {
		return nil, translateErr(err)
	}
	return ids, nil
}

func (r *captureRepository) Insert(ctx context.Context, c *domain.SessionCapture, keep int, maxAge time.Duration) error {
	c.UUID = uuid.NewString()
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO device_session_captures (capture_uuid, device_id, session_ref, from_cpe, rpc_name, http_status, body, body_bytes, body_truncated)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.UUID, c.DeviceID, c.SessionRef, c.FromCPE, c.RPCName, c.HTTPStatus, c.Body, c.BodyBytes, c.BodyTruncated)
	if err != nil {
		return translateErr(err)
	}
	id, _ := res.LastInsertId()
	c.ID = uint64(id)
	// Pemangkasan: log volume tinggi dibatasi (pengecualian soft-delete yang diizinkan untuk tabel log, migrations/0037).
	if keep > 0 {
		_, _ = r.db.ExecContext(ctx,
			`DELETE FROM device_session_captures WHERE device_id = ? AND id < (
			   SELECT m FROM (SELECT MIN(id) AS m FROM (SELECT id FROM device_session_captures WHERE device_id = ? ORDER BY id DESC LIMIT ?) t) x)`,
			c.DeviceID, c.DeviceID, keep)
	}
	if maxAge > 0 {
		// Umur dihitung dengan jam DATABASE (created_at memakai CURRENT_TIMESTAMP server), bukan jam Go: zona waktu bisa berbeda.
		_, _ = r.db.ExecContext(ctx, `DELETE FROM device_session_captures WHERE device_id = ? AND created_at < NOW(3) - INTERVAL ? SECOND`, c.DeviceID, int64(maxAge/time.Second))
	}
	return nil
}

const captureListCols = `id, capture_uuid, device_id, session_ref, from_cpe, rpc_name, http_status, '' AS body, body_bytes, body_truncated, created_at`
const captureFullCols = `id, capture_uuid, device_id, session_ref, from_cpe, rpc_name, http_status, body, body_bytes, body_truncated, created_at`

func (r *captureRepository) List(ctx context.Context, deviceID uint64, p domain.Pagination) ([]domain.SessionCapture, int, error) {
	var total int
	if err := r.db.GetContext(ctx, &total, `SELECT COUNT(*) FROM device_session_captures WHERE device_id = ?`, deviceID); err != nil {
		return nil, 0, translateErr(err)
	}
	rows := []domain.SessionCapture{}
	if err := r.db.SelectContext(ctx, &rows,
		`SELECT `+captureListCols+` FROM device_session_captures WHERE device_id = ? ORDER BY id DESC LIMIT ? OFFSET ?`,
		deviceID, p.Limit(), p.Offset()); err != nil {
		return nil, 0, translateErr(err)
	}
	return rows, total, nil
}

func (r *captureRepository) GetByUUID(ctx context.Context, deviceID uint64, uuid string) (*domain.SessionCapture, error) {
	var c domain.SessionCapture
	if err := r.db.GetContext(ctx, &c, `SELECT `+captureFullCols+` FROM device_session_captures WHERE device_id = ? AND capture_uuid = ?`, deviceID, uuid); err != nil {
		return nil, translateErr(err)
	}
	return &c, nil
}

func (r *captureRepository) ListAll(ctx context.Context, deviceID uint64, limit int) ([]domain.SessionCapture, error) {
	rows := []domain.SessionCapture{}
	if err := r.db.SelectContext(ctx, &rows,
		`SELECT `+captureFullCols+` FROM device_session_captures WHERE device_id = ? ORDER BY id ASC LIMIT ?`, deviceID, limit); err != nil {
		return nil, translateErr(err)
	}
	return rows, nil
}
