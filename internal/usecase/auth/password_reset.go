package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	"acs/internal/domain"
)

// ErrInvalidResetToken — tautan reset tidak valid, sudah dipakai, atau
// kedaluwarsa. SATU pesan untuk ketiganya agar klien tidak bisa membedakan.
var ErrInvalidResetToken = errors.New("auth: tautan reset tidak valid atau sudah kedaluwarsa")

// ErrPasswordResetDisabled — email/reset belum dikonfigurasi di instalasi ini.
var ErrPasswordResetDisabled = errors.New("auth: reset password lewat email tidak aktif")

// lastResetCleanup — waktu (UnixNano) pembersihan token kedaluwarsa terakhir.
var lastResetCleanup atomic.Int64

const (
	// maxResetRequestsPerHour membatasi permintaan reset PER AKUN (selain rate
	// limit per-IP di router) — mencegah akun korban dibanjiri email.
	maxResetRequestsPerHour = 3
	resetTokenBytes         = 32
	maxEmailLen             = 191 // sama dgn kolom users.email
)

func hashResetToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// PasswordResetEnabled melaporkan apakah alur reset lewat email aktif.
func (s *Service) PasswordResetEnabled() bool {
	return s.resets != nil && s.mail != nil && s.notifier != nil
}

// RequestPasswordReset memproses "lupa password". SELALU tampak berhasil bagi
// pemanggil (nil) — baik email terdaftar atau tidak, baik terkena rate limit
// atau tidak — supaya endpoint tidak bisa dipakai menebak email yang terdaftar
// (OWASP: user enumeration). Kegagalan internal hanya dicatat ke log
// (TANPA token/tautan/alamat lengkap).
func (s *Service) RequestPasswordReset(ctx context.Context, email, requestIP string) {
	if !s.PasswordResetEnabled() {
		return
	}
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || len(email) > maxEmailLen || !strings.Contains(email, "@") {
		return
	}
	now := time.Now()
	// Retensi: buang token kedaluwarsa > 24 jam — dibatasi paling sering sekali per
	// 10 menit supaya permintaan anonim tidak masing-masing memicu DELETE.
	if last := lastResetCleanup.Load(); now.UnixNano()-last > int64(10*time.Minute) && lastResetCleanup.CompareAndSwap(last, now.UnixNano()) {
		_, _ = s.resets.DeleteExpired(ctx, now.Add(-24*time.Hour))
	}

	u, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		if !errors.Is(err, domain.ErrNotFound) {
			slog.Warn("auth: reset password — lookup user gagal", "error", err)
		}
		return
	}
	if !u.IsActive {
		return
	}
	if err := s.checkTenantActive(ctx, u.TenantID); err != nil {
		return
	}
	n, err := s.resets.CountRecentByUser(ctx, u.ID, now.Add(-time.Hour))
	if err != nil {
		slog.Warn("auth: reset password — hitung permintaan gagal", "error", err)
		return
	}
	if n >= maxResetRequestsPerHour {
		return
	}

	raw := make([]byte, resetTokenBytes)
	if _, err := rand.Read(raw); err != nil {
		slog.Error("auth: reset password — gagal membangkitkan token", "error", err)
		return
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	if err := s.resets.Create(ctx, u.ID, hashResetToken(token), now.Add(s.resetTTL), requestIP); err != nil {
		slog.Warn("auth: reset password — simpan token gagal", "error", err)
		return
	}
	_ = s.activity.Record(ctx, &domain.ActivityLog{UserID: &u.ID, TenantID: u.TenantID, Action: "REQUEST_PASSWORD_RESET", EntityType: "user", EntityID: &u.ID, IPAddress: ptr(requestIP)})
	s.notify(s.mail.PasswordReset(u.Email, u.FullName, s.mail.ResetLink(token), s.resetTTL))
}

// ResetPassword menukar token reset dengan password baru. Token sekali pakai;
// semua token lain milik user ikut dimatikan; lockout dibersihkan; pemilik
// akun diberi tahu lewat email.
func (s *Service) ResetPassword(ctx context.Context, token, newPassword string) error {
	if !s.PasswordResetEnabled() {
		return ErrPasswordResetDisabled
	}
	token = strings.TrimSpace(token)
	if token == "" || len(token) > 128 {
		return ErrInvalidResetToken
	}
	// Validasi password SEBELUM mengonsumsi token — agar pengguna yang salah
	// ketik (terlalu pendek) tidak kehilangan tautannya.
	if len(newPassword) < domain.MinUserPasswordLen {
		return fmt.Errorf("%w: password minimal %d karakter", domain.ErrInvalidInput, domain.MinUserPasswordLen)
	}
	// Urutan sengaja: (1) Peek murah — token ngawur ditolak TANPA hashing Argon2
	// (anti-DoS dari pemanggil anonim); (2) pastikan akun aktif SEBELUM token
	// dihanguskan; (3) baru hash + Consume atomik + simpan.
	tokenHash := hashResetToken(token)
	now := time.Now()
	userID, err := s.resets.Peek(ctx, tokenHash, now)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return ErrInvalidResetToken
		}
		return err
	}
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return ErrInvalidResetToken
		}
		return err
	}
	if !u.IsActive {
		return ErrInvalidResetToken
	}
	hash, err := HashPassword(newPassword)
	if err != nil {
		return err
	}
	// Consume atomik: bila request paralel lain sudah memakainya, kalah di sini.
	if _, err := s.resets.Consume(ctx, tokenHash, now); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return ErrInvalidResetToken
		}
		return err
	}
	if err := s.users.UpdatePassword(ctx, u.ID, hash, &u.ID); err != nil {
		return err
	}
	_ = s.users.ResetLoginLockout(ctx, u.ID)
	_ = s.resets.InvalidateAllForUser(ctx, u.ID, now)
	_ = s.activity.Record(ctx, &domain.ActivityLog{UserID: &u.ID, TenantID: u.TenantID, Action: "RESET_PASSWORD_VIA_EMAIL", EntityType: "user", EntityID: &u.ID})
	s.notify(s.mail.PasswordChanged(u.Email, u.FullName))
	return nil
}

func ptr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
