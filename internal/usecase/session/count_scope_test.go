package session

import (
	"context"
	"errors"
	"testing"

	"acs/internal/domain"
)

type countRepo struct {
	domain.DeviceSessionRepository
	global    int
	perTenant map[uint64]int
}

func (r countRepo) CountOpen(context.Context) (int, error) { return r.global, nil }
func (r countRepo) CountOpenForTenant(_ context.Context, id uint64) (int, error) {
	return r.perTenant[id], nil
}

// Jumlah sesi terbuka mengikuti cakupan actor: tenant tidak melihat beban platform/tenant lain (OWASP A01).
func TestCountOpenSessionsFor_CakupanActor(t *testing.T) {
	s := &Service{sessions: countRepo{global: 50, perTenant: map[uint64]int{1: 3, 2: 7}}}
	ctx := context.Background()
	t1, t2 := uint64(1), uint64(2)

	if n, err := s.CountOpenSessionsFor(ctx, domain.Actor{Roles: []string{domain.RoleSuperadmin}}); err != nil || n != 50 {
		t.Fatalf("superadmin harus melihat seluruh platform: %d %v", n, err)
	}
	if n, err := s.CountOpenSessionsFor(ctx, domain.Actor{TenantID: &t1, Roles: []string{domain.RoleAdmin}}); err != nil || n != 3 {
		t.Fatalf("admin tenant 1 hanya melihat miliknya: %d %v", n, err)
	}
	if n, err := s.CountOpenSessionsFor(ctx, domain.Actor{TenantID: &t2, Roles: []string{domain.RoleNOC}}); err != nil || n != 7 {
		t.Fatalf("NOC tenant 2 hanya melihat miliknya: %d %v", n, err)
	}
	if _, err := s.CountOpenSessionsFor(ctx, domain.Actor{Roles: []string{domain.RoleAdmin}}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("non-superadmin tanpa tenant harus ditolak: %v", err)
	}
}
