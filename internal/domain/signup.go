package domain

import (
	"context"
	"time"
)

// TenantSignup — pendaftaran tenant mandiri yang menunggu verifikasi email (migrations/0028). Tenant & admin baru
// dibuat setelah tautan verifikasi dipakai; sebelum itu hanya baris ini yang ada (berumur pendek, dipangkas).
type TenantSignup struct {
	ID           uint64     `db:"id"`
	SignupUUID   string     `db:"signup_uuid"`
	CompanyName  string     `db:"company_name"`
	TenantCode   string     `db:"tenant_code"`
	Email        string     `db:"email"`
	FullName     string     `db:"full_name"`
	PasswordHash string     `db:"password_hash"`
	TokenHash    string     `db:"token_hash"`
	IPAddress    *string    `db:"ip_address"`
	ExpiresAt    time.Time  `db:"expires_at"`
	VerifiedAt   *time.Time `db:"verified_at"`
	CreatedAt    time.Time  `db:"created_at"`
}

// SignupRepository — hanya query; keputusan (batas, validasi) ada di usecase/signup.
type SignupRepository interface {
	Create(ctx context.Context, s *TenantSignup) error
	GetByTokenHash(ctx context.Context, hash string) (*TenantSignup, error)
	// Claim menandai terverifikasi bila belum & belum kedaluwarsa (compare-and-swap, aman untuk klik ganda/paralel).
	Claim(ctx context.Context, id uint64, now time.Time) (bool, error)
	// Unclaim membatalkan Claim saat pembuatan tenant gagal supaya tautan bisa dicoba lagi.
	Unclaim(ctx context.Context, id uint64) error
	CountSince(ctx context.Context, since time.Time) (int, error)
	CountByEmailSince(ctx context.Context, email string, since time.Time) (int, error)
	CountByIPSince(ctx context.Context, ip string, since time.Time) (int, error)
	// PurgeOld menghapus pendaftaran kedaluwarsa tak terverifikasi (sebelum `unverifiedBefore`) dan yang sudah terverifikasi
	// lama (sebelum `verifiedBefore`) — antrean verifikasi, bukan data master (lihat migrations/0028).
	PurgeOld(ctx context.Context, unverifiedBefore, verifiedBefore time.Time) (int64, error)
}
