package selfservice

import (
	"context"
	"errors"
	"testing"

	"acs/internal/domain"
)

type fakeLinks struct {
	domain.UserDeviceRepository
	m map[[2]uint64]bool
}

func (f *fakeLinks) IsMapped(_ context.Context, u, d uint64) (bool, error) {
	return f.m[[2]uint64{u, d}], nil
}
func (f *fakeLinks) Map(_ context.Context, u, d uint64, _ *uint64) error {
	f.m[[2]uint64{u, d}] = true
	return nil
}
func (f *fakeLinks) Unmap(_ context.Context, u, d uint64) error {
	delete(f.m, [2]uint64{u, d})
	return nil
}
func (f *fakeLinks) DeviceIDsForUser(_ context.Context, u uint64) ([]uint64, error) {
	var out []uint64
	for k := range f.m {
		if k[0] == u {
			out = append(out, k[1])
		}
	}
	return out, nil
}

type fakeUsers struct {
	domain.UserRepository
	byID  map[uint64]*domain.User
	roles map[uint64][]string
}

func (f fakeUsers) GetByID(_ context.Context, id uint64) (*domain.User, error) {
	if u, ok := f.byID[id]; ok {
		return u, nil
	}
	return nil, domain.ErrNotFound
}
func (f fakeUsers) RolesByUserID(_ context.Context, id uint64) ([]string, error) {
	return f.roles[id], nil
}

type noActivity struct{ domain.ActivityLogRepository }

func (noActivity) Record(context.Context, *domain.ActivityLog) error { return nil }

func linkSvc() (*Service, *fakeLinks) {
	links := &fakeLinks{m: map[[2]uint64]bool{}}
	svc := &Service{
		userDevices: links,
		devices: fakeDevs{byID: map[uint64]*domain.Device{
			1: {ID: 1, TenantID: tid(7), SerialNumber: "A1"},
			2: {ID: 2, TenantID: tid(8), SerialNumber: "B1"},
			3: {ID: 3, TenantID: nil, SerialNumber: "N1"},
		}},
		users: fakeUsers{
			byID: map[uint64]*domain.User{
				10: {ID: 10, TenantID: tid(7)}, // pelanggan tenant 7
				11: {ID: 11, TenantID: tid(7)}, // staf tenant 7 (bukan ENDUSER)
				12: {ID: 12, TenantID: tid(8)}, // pelanggan tenant 8
			},
			roles: map[uint64][]string{10: {domain.RoleEndUser}, 11: {domain.RoleNOC}, 12: {domain.RoleEndUser}},
		},
		activity: noActivity{},
	}
	return svc, links
}

func TestLinkDevice_Aturan(t *testing.T) {
	ctx := context.Background()
	adm7 := domain.Actor{UserID: 1, TenantID: tid(7), Roles: []string{domain.RoleAdmin}}
	super := domain.Actor{UserID: 2, Roles: []string{domain.RoleSuperadmin}}
	noc7 := domain.Actor{UserID: 3, TenantID: tid(7), Roles: []string{domain.RoleNOC}}

	cases := []struct {
		name  string
		actor domain.Actor
		user  uint64
		dev   uint64
		want  error
	}{
		{"ADMIN tenant sama berhasil", adm7, 10, 1, nil},
		{"SUPERADMIN berhasil", super, 10, 1, nil},
		{"NOC tidak boleh", noc7, 10, 1, domain.ErrForbidden},
		{"ADMIN tenant lain: akun tenant 8 ditolak", adm7, 12, 2, domain.ErrForbidden},
		{"ADMIN: perangkat tenant lain ditolak", adm7, 10, 2, domain.ErrForbidden},
		{"akun & perangkat beda tenant (superadmin)", super, 10, 2, domain.ErrInvalidInput},
		{"perangkat tanpa tenant", super, 10, 3, domain.ErrInvalidInput},
		{"akun bukan ENDUSER", adm7, 11, 1, domain.ErrInvalidInput},
		{"akun tidak ada", adm7, 99, 1, domain.ErrNotFound},
		{"perangkat tidak ada", adm7, 10, 99, domain.ErrNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, links := linkSvc()
			err := svc.LinkDevice(ctx, tc.actor, tc.user, tc.dev)
			if tc.want == nil {
				if err != nil || !links.m[[2]uint64{tc.user, tc.dev}] {
					t.Fatalf("harus tertaut: err=%v", err)
				}
				return
			}
			if !errors.Is(err, tc.want) || len(links.m) != 0 {
				t.Fatalf("want %v tanpa tautan, dapat err=%v links=%v", tc.want, err, links.m)
			}
		})
	}
}

func TestLinkDevice_KuotaIdempotenDanLepas(t *testing.T) {
	ctx := context.Background()
	svc, links := linkSvc()
	adm := domain.Actor{UserID: 1, TenantID: tid(7), Roles: []string{domain.RoleAdmin}}
	// idempoten: menautkan dua kali tidak error & tidak menggandakan
	for i := 0; i < 2; i++ {
		if err := svc.LinkDevice(ctx, adm, 10, 1); err != nil {
			t.Fatal(err)
		}
	}
	if len(links.m) != 1 {
		t.Fatalf("tautan ganda: %v", links.m)
	}
	// batas per akun: isi sampai penuh, tautan baru ditolak, tautan yang sudah ada tetap idempoten
	for id := uint64(100); len(links.m) < maxLinkedDevicesPerUser; id++ {
		links.m[[2]uint64{10, id}] = true
	}
	svc.devices = fakeDevs{byID: map[uint64]*domain.Device{1: {ID: 1, TenantID: tid(7)}, 5: {ID: 5, TenantID: tid(7)}}}
	if err := svc.LinkDevice(ctx, adm, 10, 5); !errors.Is(err, domain.ErrQuotaExceeded) {
		t.Fatalf("melewati batas harus ErrQuotaExceeded: %v", err)
	}
	if err := svc.LinkDevice(ctx, adm, 10, 1); err != nil {
		t.Fatalf("menautkan ulang yang sudah ada tetap boleh: %v", err)
	}
	// lepas: idempoten; admin tenant lain tidak bisa
	other := domain.Actor{UserID: 9, TenantID: tid(8), Roles: []string{domain.RoleAdmin}}
	if err := svc.UnlinkDevice(ctx, other, 10, 1); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("admin tenant lain tidak boleh melepas: %v", err)
	}
	if err := svc.UnlinkDevice(ctx, adm, 10, 1); err != nil || links.m[[2]uint64{10, 1}] {
		t.Fatalf("lepas gagal: %v", err)
	}
	if err := svc.UnlinkDevice(ctx, adm, 10, 1); err != nil {
		t.Fatalf("lepas ulang harus idempoten: %v", err)
	}
}

type fakeRefs struct{ domain.RefRepository }

func (fakeRefs) GetByID(context.Context, string, uint64) (domain.RefLookup, error) {
	return domain.RefLookup{}, domain.ErrNotFound
}

func TestListUserDevices_HanyaSeTenant(t *testing.T) {
	ctx := context.Background()
	svc, links := linkSvc()
	svc.refs = fakeRefs{}
	links.m[[2]uint64{10, 1}] = true  // se-tenant
	links.m[[2]uint64{10, 2}] = true  // sisa tautan lama ke perangkat tenant lain
	links.m[[2]uint64{10, 77}] = true // perangkat sudah dihapus
	adm := domain.Actor{UserID: 1, TenantID: tid(7), Roles: []string{domain.RoleAdmin}}
	got, err := svc.ListUserDevices(ctx, adm, 10)
	if err != nil || len(got) != 1 || got[0].SerialNumber != "A1" {
		t.Fatalf("hanya perangkat se-tenant yang masih ada yang tampil: %+v err=%v", got, err)
	}
	if _, err := svc.ListUserDevices(ctx, domain.Actor{UserID: 9, TenantID: tid(8), Roles: []string{domain.RoleAdmin}}, 10); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("admin tenant lain tidak boleh melihat: %v", err)
	}
}
