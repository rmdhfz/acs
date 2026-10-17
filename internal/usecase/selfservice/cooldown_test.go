package selfservice

import (
	"errors"
	"testing"
	"time"

	"acs/internal/domain"
)

// Jeda per akun+perangkat (audit OWASP A04): mencegah banjir task/reboot berulang oleh satu pelanggan.
func TestRequireCooldown(t *testing.T) {
	s := &Service{}
	if err := s.requireCooldown(10, 1); err != nil {
		t.Fatalf("aksi pertama harus lolos: %v", err)
	}
	if err := s.requireCooldown(10, 1); !errors.Is(err, domain.ErrQuotaExceeded) {
		t.Fatalf("aksi kedua dalam jeda harus ditolak: %v", err)
	}
	if err := s.requireCooldown(10, 2); err != nil {
		t.Fatalf("perangkat lain tidak terkena jeda: %v", err)
	}
	if err := s.requireCooldown(11, 1); err != nil {
		t.Fatalf("akun lain tidak terkena jeda: %v", err)
	}
	// setelah jeda berlalu boleh lagi
	s.cdMu.Lock()
	s.cooldown[[2]uint64{10, 1}] = time.Now().Add(-selfServiceCooldown - time.Second)
	s.cdMu.Unlock()
	if err := s.requireCooldown(10, 1); err != nil {
		t.Fatalf("setelah jeda berlalu harus lolos: %v", err)
	}
}
