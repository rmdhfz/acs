package provisioning

import (
	"context"
	"errors"
	"testing"

	"acs/internal/domain"
)

type fakeZTRules struct {
	domain.ZeroTouchRuleRepository
	byID map[uint64]*domain.ZeroTouchRule
}

func (f *fakeZTRules) GetByID(_ context.Context, id uint64) (*domain.ZeroTouchRule, error) {
	if r, ok := f.byID[id]; ok {
		return r, nil
	}
	return nil, domain.ErrNotFound
}

func tid(v uint64) *uint64 { return &v }

func TestGetZeroTouchRule_Scope(t *testing.T) {
	svc := &Service{rules: &fakeZTRules{byID: map[uint64]*domain.ZeroTouchRule{
		1: {ID: 1, TenantID: tid(7)},
		2: {ID: 2, TenantID: tid(8)},
		3: {ID: 3, TenantID: nil}, // rule global — ikut tampil di list, harus bisa dibaca
	}}}
	ctx := context.Background()
	adminT7 := domain.Actor{UserID: 5, TenantID: tid(7), Roles: []string{domain.RoleAdmin}}
	super := domain.Actor{UserID: 1, Roles: []string{domain.RoleSuperadmin}}
	orphan := domain.Actor{UserID: 9, Roles: []string{domain.RoleAdmin}}

	for _, tc := range []struct {
		name  string
		actor domain.Actor
		id    uint64
		want  error
	}{
		{"milik tenant sendiri", adminT7, 1, nil},
		{"rule global bisa dibaca", adminT7, 3, nil},
		{"tenant lain ditolak", adminT7, 2, domain.ErrForbidden},
		{"superadmin lintas tenant", super, 2, nil},
		{"tidak ada", adminT7, 404, domain.ErrNotFound},
		{"akun tanpa tenant ditolak untuk rule tenant", orphan, 1, domain.ErrForbidden},
	} {
		_, err := svc.GetZeroTouchRule(ctx, tc.actor, tc.id)
		if tc.want == nil && err != nil || tc.want != nil && !errors.Is(err, tc.want) {
			t.Errorf("%s: want %v, got %v", tc.name, tc.want, err)
		}
	}
}
