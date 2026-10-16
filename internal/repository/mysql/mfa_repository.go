package mysql

import (
	"context"

	"github.com/jmoiron/sqlx"

	"acs/internal/domain"
)

type mfaRepository struct{ db *sqlx.DB }

func NewMFARepository(db *sqlx.DB) domain.MFARepository { return &mfaRepository{db: db} }

func (r *mfaRepository) SaveSecret(ctx context.Context, userID uint64, secretEnc []byte) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE users SET totp_secret_enc = ?, totp_enabled = 0, totp_last_step = NULL WHERE id = ? AND is_deleted = 0`,
		secretEnc, userID)
	if err != nil {
		return translateErr(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *mfaRepository) Enable(ctx context.Context, userID, step uint64, codeHashes []string) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return translateErr(err)
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx,
		`UPDATE users SET totp_enabled = 1, totp_last_step = ? WHERE id = ? AND is_deleted = 0 AND totp_secret_enc IS NOT NULL AND totp_enabled = 0`,
		step, userID)
	if err != nil {
		return translateErr(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.ErrConflict
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM user_recovery_codes WHERE user_id = ?`, userID); err != nil {
		return translateErr(err)
	}
	for _, h := range codeHashes {
		if _, err := tx.ExecContext(ctx, `INSERT INTO user_recovery_codes (user_id, code_hash) VALUES (?, ?)`, userID, h); err != nil {
			return translateErr(err)
		}
	}
	return translateErr(tx.Commit())
}

func (r *mfaRepository) Disable(ctx context.Context, userID uint64) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return translateErr(err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx,
		`UPDATE users SET totp_secret_enc = NULL, totp_enabled = 0, totp_last_step = NULL WHERE id = ? AND is_deleted = 0`, userID); err != nil {
		return translateErr(err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM user_recovery_codes WHERE user_id = ?`, userID); err != nil {
		return translateErr(err)
	}
	return translateErr(tx.Commit())
}

func (r *mfaRepository) AdvanceStep(ctx context.Context, userID, step uint64) (bool, error) {
	res, err := r.db.ExecContext(ctx,
		`UPDATE users SET totp_last_step = ? WHERE id = ? AND is_deleted = 0 AND (totp_last_step IS NULL OR totp_last_step < ?)`,
		step, userID, step)
	if err != nil {
		return false, translateErr(err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

func (r *mfaRepository) UseRecoveryCode(ctx context.Context, userID uint64, codeHash string) (bool, error) {
	res, err := r.db.ExecContext(ctx,
		`UPDATE user_recovery_codes SET used_at = NOW() WHERE user_id = ? AND code_hash = ? AND used_at IS NULL`, userID, codeHash)
	if err != nil {
		return false, translateErr(err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

func (r *mfaRepository) CountRecoveryCodes(ctx context.Context, userID uint64) (int, error) {
	var n int
	err := r.db.GetContext(ctx, &n, `SELECT COUNT(*) FROM user_recovery_codes WHERE user_id = ? AND used_at IS NULL`, userID)
	return n, translateErr(err)
}
