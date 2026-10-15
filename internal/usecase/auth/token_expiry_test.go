package auth

import (
	"errors"
	"testing"
	"time"

	"acs/internal/domain"
)

func TestResolveAPITokenExpiry(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	at := func(d time.Duration) *time.Time { v := now.Add(d); return &v }

	t.Run("tidak diisi -> default maksimum (bukan abadi)", func(t *testing.T) {
		got, err := resolveAPITokenExpiry(now, nil)
		if err != nil || got == nil || !got.Equal(now.Add(MaxAPITokenTTL)) {
			t.Fatalf("got %v err=%v", got, err)
		}
	})
	t.Run("30 hari lolos apa adanya", func(t *testing.T) {
		req := at(30 * 24 * time.Hour)
		got, err := resolveAPITokenExpiry(now, req)
		if err != nil || !got.Equal(*req) {
			t.Fatalf("got %v err=%v", got, err)
		}
	})
	t.Run("tepat di batas maksimum lolos", func(t *testing.T) {
		if _, err := resolveAPITokenExpiry(now, at(MaxAPITokenTTL)); err != nil {
			t.Fatalf("boundary harus lolos: %v", err)
		}
	})
	t.Run("sedetik di atas maksimum ditolak", func(t *testing.T) {
		_, err := resolveAPITokenExpiry(now, at(MaxAPITokenTTL+time.Second))
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("want ErrInvalidInput, got %v", err)
		}
	})
	t.Run("masa lalu dan 'sekarang' ditolak", func(t *testing.T) {
		for _, d := range []time.Duration{-time.Hour, 0} {
			if _, err := resolveAPITokenExpiry(now, at(d)); !errors.Is(err, domain.ErrInvalidInput) {
				t.Fatalf("offset %v: want ErrInvalidInput, got %v", d, err)
			}
		}
	})
}
