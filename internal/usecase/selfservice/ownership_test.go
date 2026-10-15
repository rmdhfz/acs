package selfservice

import (
	"context"
	"errors"
	"testing"

	"acs/internal/domain"
)

type fakeMap struct {
	domain.UserDeviceRepository
	mapped map[[2]uint64]bool
}

func (f fakeMap) IsMapped(_ context.Context, user, device uint64) (bool, error) {
	return f.mapped[[2]uint64{user, device}], nil
}

type fakeDevs struct {
	domain.DeviceRepository
	byID map[uint64]*domain.Device
}

func (f fakeDevs) GetByID(_ context.Context, id uint64) (*domain.Device, error) {
	if d, ok := f.byID[id]; ok {
		return d, nil
	}
	return nil, domain.ErrNotFound
}

func tid(v uint64) *uint64 { return &v }

// Kepemilikan = pemetaan user_devices DAN tenant yang sama. Pemetaan lama yang
// tersisa setelah device berpindah tenant tidak boleh memberi akses.
func TestRequireOwnership(t *testing.T) {
	svc := &Service{
		userDevices: fakeMap{mapped: map[[2]uint64]bool{{10, 1}: true, {10, 2}: true, {10, 3}: true}},
		devices: fakeDevs{byID: map[uint64]*domain.Device{
			1: {ID: 1, TenantID: tid(7)},
			2: {ID: 2, TenantID: tid(8)}, // pemetaan lama ke device yang kini milik tenant lain
			3: {ID: 3, TenantID: nil},    // device tanpa tenant
		}},
	}
	ctx := context.Background()
	cust := domain.Actor{UserID: 10, TenantID: tid(7), Roles: []string{domain.RoleEndUser}}

	cases := []struct {
		name  string
		actor domain.Actor
		dev   uint64
		want  error
	}{
		{"dipetakan & tenant sama", cust, 1, nil},
		{"dipetakan tetapi device pindah tenant", cust, 2, domain.ErrNotFound},
		{"dipetakan tetapi device tanpa tenant", cust, 3, domain.ErrNotFound},
		{"tidak dipetakan", cust, 99, domain.ErrNotFound},
		{"akun tanpa tenant", domain.Actor{UserID: 10, Roles: []string{domain.RoleEndUser}}, 1, domain.ErrNotFound},
		{"UserID 0", domain.Actor{TenantID: tid(7)}, 1, domain.ErrForbidden},
		{"akun lain tidak dipetakan", domain.Actor{UserID: 11, TenantID: tid(7)}, 1, domain.ErrNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, err := svc.requireOwnership(ctx, tc.actor, tc.dev)
			if tc.want == nil {
				if err != nil || d == nil || d.ID != tc.dev {
					t.Fatalf("d=%v err=%v", d, err)
				}
				return
			}
			if !errors.Is(err, tc.want) || d != nil {
				t.Fatalf("want %v, got d=%v err=%v", tc.want, d, err)
			}
		})
	}
}
