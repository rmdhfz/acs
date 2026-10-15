package mysql

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"

	"acs/internal/domain"
)

type passwordResetRepository struct{ db *sqlx.DB }

// NewPasswordResetRepository — token reset password (migrations/0023). Hanya
// query, tanpa keputusan bisnis (aturan TTL/rate limit ada di usecase/auth).
func NewPasswordResetRepository(db *sqlx.DB) domain.PasswordResetRepository {
	return &passwordResetRepository{db: db}
}

func (r *passwordResetRepository) Create(ctx context.Context, userID uint64, tokenHash string, expiresAt time.Time, requestedIP string) error {
	var ip *string
	if requestedIP != "" {
		ip = &requestedIP
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO password_reset_tokens (user_id, token_hash, expires_at, requested_ip, created_at) VALUES (?, ?, ?, ?, ?)`,
		userID, tokenHash, expiresAt, ip, time.Now())
	return translateErr(err)
}

// Consume: satu UPDATE bersyarat (belum dipakai & belum kedaluwarsa) — hanya
// SATU dari sekian request paralel yang melihat RowsAffected = 1.
func (r *passwordResetRepository) Consume(ctx context.Context, tokenHash string, now time.Time) (uint64, error) {
	res, err := r.db.ExecContext(ctx,
		`UPDATE password_reset_tokens SET used_at = ? WHERE token_hash = ? AND used_at IS NULL AND expires_at > ?`,
		now, tokenHash, now)
	if err != nil {
		return 0, translateErr(err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return 0, domain.ErrNotFound
	}
	var userID uint64
	if err := r.db.GetContext(ctx, &userID, `SELECT user_id FROM password_reset_tokens WHERE token_hash = ?`, tokenHash); err != nil {
		return 0, translateErr(err)
	}
	return userID, nil
}

// Peek — baca-saja: token valid (belum dipakai & belum kedaluwarsa)?
func (r *passwordResetRepository) Peek(ctx context.Context, tokenHash string, now time.Time) (uint64, error) {
	var userID uint64
	err := r.db.GetContext(ctx, &userID,
		`SELECT user_id FROM password_reset_tokens WHERE token_hash = ? AND used_at IS NULL AND expires_at > ?`, tokenHash, now)
	if err != nil {
		return 0, translateErr(err)
	}
	return userID, nil
}

func (r *passwordResetRepository) CountRecentByUser(ctx context.Context, userID uint64, since time.Time) (int, error) {
	var n int
	if err := r.db.GetContext(ctx, &n,
		`SELECT COUNT(*) FROM password_reset_tokens WHERE user_id = ? AND created_at >= ?`, userID, since); err != nil {
		return 0, translateErr(err)
	}
	return n, nil
}

func (r *passwordResetRepository) InvalidateAllForUser(ctx context.Context, userID uint64, now time.Time) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE password_reset_tokens SET used_at = ? WHERE user_id = ? AND used_at IS NULL`, now, userID)
	return translateErr(err)
}

func (r *passwordResetRepository) DeleteExpired(ctx context.Context, before time.Time) (int64, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM password_reset_tokens WHERE expires_at < ?`, before)
	if err != nil {
		return 0, translateErr(err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}
