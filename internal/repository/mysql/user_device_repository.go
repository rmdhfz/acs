package mysql

import (
	"context"

	"acs/internal/domain"

	"github.com/jmoiron/sqlx"
)

type userDeviceRepository struct {
	db *sqlx.DB
}

func NewUserDeviceRepository(db *sqlx.DB) domain.UserDeviceRepository {
	return &userDeviceRepository{db: db}
}

func (r *userDeviceRepository) DeviceIDsForUser(ctx context.Context, userID uint64) ([]uint64, error) {
	var ids []uint64
	err := r.db.SelectContext(ctx, &ids,
		`SELECT device_id FROM user_devices WHERE user_id = ? ORDER BY device_id`, userID)
	if err != nil {
		return nil, translateErr(err)
	}
	return ids, nil
}

func (r *userDeviceRepository) IsMapped(ctx context.Context, userID, deviceID uint64) (bool, error) {
	var n int
	err := r.db.GetContext(ctx, &n,
		`SELECT COUNT(*) FROM user_devices WHERE user_id = ? AND device_id = ?`, userID, deviceID)
	if err != nil {
		return false, translateErr(err)
	}
	return n > 0, nil
}

func (r *userDeviceRepository) Map(ctx context.Context, userID, deviceID uint64, createdBy *uint64) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT IGNORE INTO user_devices (user_id, device_id, created_by) VALUES (?, ?, ?)`,
		userID, deviceID, createdBy)
	return translateErr(err)
}

func (r *userDeviceRepository) Unmap(ctx context.Context, userID, deviceID uint64) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM user_devices WHERE user_id = ? AND device_id = ?`, userID, deviceID)
	return translateErr(err)
}
