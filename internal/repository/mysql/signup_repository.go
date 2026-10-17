package mysql

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"

	"acs/internal/domain"
)

type signupRepository struct{ db *sqlx.DB }

func NewSignupRepository(db *sqlx.DB) domain.SignupRepository { return &signupRepository{db: db} }

func (r *signupRepository) Create(ctx context.Context, s *domain.TenantSignup) error {
	s.CreatedAt = time.Now()
	const q = `INSERT INTO tenant_signups
		(signup_uuid, company_name, tenant_code, email, full_name, password_hash, token_hash, ip_address, expires_at, created_at)
		VALUES (:signup_uuid, :company_name, :tenant_code, :email, :full_name, :password_hash, :token_hash, :ip_address, :expires_at, :created_at)`
	res, err := r.db.NamedExecContext(ctx, q, s)
	if err != nil {
		return translateErr(err)
	}
	id, _ := res.LastInsertId()
	s.ID = uint64(id)
	return nil
}

func (r *signupRepository) GetByTokenHash(ctx context.Context, hash string) (*domain.TenantSignup, error) {
	var s domain.TenantSignup
	if err := r.db.GetContext(ctx, &s, `SELECT * FROM tenant_signups WHERE token_hash = ?`, hash); err != nil {
		return nil, translateErr(err)
	}
	return &s, nil
}

func (r *signupRepository) Claim(ctx context.Context, id uint64, now time.Time) (bool, error) {
	res, err := r.db.ExecContext(ctx,
		`UPDATE tenant_signups SET verified_at = ? WHERE id = ? AND verified_at IS NULL AND expires_at > ?`, now, id, now)
	if err != nil {
		return false, translateErr(err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

func (r *signupRepository) Unclaim(ctx context.Context, id uint64) error {
	_, err := r.db.ExecContext(ctx, `UPDATE tenant_signups SET verified_at = NULL WHERE id = ?`, id)
	return translateErr(err)
}

func (r *signupRepository) CountSince(ctx context.Context, since time.Time) (int, error) {
	var n int
	err := r.db.GetContext(ctx, &n, `SELECT COUNT(*) FROM tenant_signups WHERE created_at >= ?`, since)
	return n, translateErr(err)
}

func (r *signupRepository) CountByEmailSince(ctx context.Context, email string, since time.Time) (int, error) {
	var n int
	err := r.db.GetContext(ctx, &n, `SELECT COUNT(*) FROM tenant_signups WHERE email = ? AND created_at >= ?`, email, since)
	return n, translateErr(err)
}

func (r *signupRepository) CountByIPSince(ctx context.Context, ip string, since time.Time) (int, error) {
	var n int
	err := r.db.GetContext(ctx, &n, `SELECT COUNT(*) FROM tenant_signups WHERE ip_address = ? AND created_at >= ?`, ip, since)
	return n, translateErr(err)
}

// PurgeOld — penghapusan fisik yang disengaja (antrean verifikasi berumur pendek; pengecualian aturan soft-delete,
// didokumentasikan di migrations/0028).
func (r *signupRepository) PurgeOld(ctx context.Context, unverifiedBefore, verifiedBefore time.Time) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM tenant_signups WHERE (verified_at IS NULL AND created_at < ?) OR (verified_at IS NOT NULL AND verified_at < ?)`,
		unverifiedBefore, verifiedBefore)
	if err != nil {
		return 0, translateErr(err)
	}
	return res.RowsAffected()
}
