package device

import (
	"context"
	"errors"
	"testing"

	"acs/internal/domain"
)

type delDevices struct {
	domain.DeviceRepository
	byID    map[uint64]*domain.Device
	deleted []uint64
	order   *[]string
}

func (f *delDevices) GetByID(_ context.Context, id uint64) (*domain.Device, error) {
	if d, ok := f.byID[id]; ok {
		return d, nil
	}
	return nil, domain.ErrNotFound
}
func (f *delDevices) SoftDelete(_ context.Context, id, _ uint64) error {
	f.deleted = append(f.deleted, id)
	*f.order = append(*f.order, "softdelete")
	return nil
}

type fakeCanceller struct {
	n     int64
	err   error
	calls []uint64
	order *[]string
}

func (f *fakeCanceller) CancelOpenForDevice(_ context.Context, deviceID uint64, _ *uint64) (int64, error) {
	f.calls = append(f.calls, deviceID)
	*f.order = append(*f.order, "cancel")
	return f.n, f.err
}

type delActivity struct{ got []*domain.ActivityLog }

func (a *delActivity) Record(_ context.Context, l *domain.ActivityLog) error {
	a.got = append(a.got, l)
	return nil
}
func (a *delActivity) ListByEntity(context.Context, string, uint64, domain.Pagination) ([]domain.ActivityLog, int, error) {
	return nil, 0, nil
}
func (a *delActivity) ListByTenant(context.Context, *uint64, domain.ActivityLogFilter, domain.Pagination) ([]domain.ActivityLog, int, error) {
	return nil, 0, nil
}

func delRig() (*Service, *delDevices, *fakeCanceller, *delActivity, *[]string) {
	order := &[]string{}
	devs := &delDevices{order: order, byID: map[uint64]*domain.Device{
		1: {ID: 1, TenantID: ptr(uint64(7)), SerialNumber: "SN-A"},
		2: {ID: 2, TenantID: ptr(uint64(8)), SerialNumber: "SN-B"},
	}}
	can := &fakeCanceller{n: 3, order: order}
	act := &delActivity{}
	return &Service{devices: devs, taskCanceller: can, activity: act}, devs, can, act, order
}

func TestDelete_BatalkanTaskLaluHapusDanAudit(t *testing.T) {
	svc, devs, can, act, order := delRig()
	admin := domain.Actor{UserID: 5, TenantID: ptr(uint64(7)), Roles: []string{domain.RoleAdmin}}

	if err := svc.Delete(context.Background(), admin, 1); err != nil {
		t.Fatal(err)
	}
	if len(*order) != 2 || (*order)[0] != "cancel" || (*order)[1] != "softdelete" {
		t.Fatalf("urutan harus cancel -> softdelete, got %v", *order)
	}
	if len(can.calls) != 1 || can.calls[0] != 1 || len(devs.deleted) != 1 || devs.deleted[0] != 1 {
		t.Fatalf("cancel=%v deleted=%v", can.calls, devs.deleted)
	}
	if len(act.got) != 1 || act.got[0].Action != "DELETE_DEVICE" {
		t.Fatalf("audit trail: %+v", act.got)
	}
	if act.got[0].Details["cancelled_tasks"] != int64(3) || act.got[0].Details["serial_number"] != "SN-A" {
		t.Fatalf("detail audit: %+v", act.got[0].Details)
	}
}

func TestDelete_RBACDanScopeTenant(t *testing.T) {
	svc, devs, can, _, _ := delRig()
	ctx := context.Background()

	noc := domain.Actor{UserID: 5, TenantID: ptr(uint64(7)), Roles: []string{domain.RoleNOC}}
	if err := svc.Delete(ctx, noc, 1); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("NOC: want ErrForbidden, got %v", err)
	}
	adminT7 := domain.Actor{UserID: 5, TenantID: ptr(uint64(7)), Roles: []string{domain.RoleAdmin}}
	if err := svc.Delete(ctx, adminT7, 2); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("ADMIN device tenant lain: want ErrForbidden, got %v", err)
	}
	if err := svc.Delete(ctx, adminT7, 404); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	if len(can.calls) != 0 || len(devs.deleted) != 0 {
		t.Fatalf("penolakan tidak boleh menyentuh task/device: cancel=%v deleted=%v", can.calls, devs.deleted)
	}
	super := domain.Actor{UserID: 1, Roles: []string{domain.RoleSuperadmin}}
	if err := svc.Delete(ctx, super, 2); err != nil {
		t.Fatalf("superadmin lintas tenant: %v", err)
	}
}

func TestDelete_GagalBatalkanTaskTidakMenghapus(t *testing.T) {
	svc, devs, can, _, _ := delRig()
	can.err = errors.New("db down")
	admin := domain.Actor{UserID: 5, TenantID: ptr(uint64(7)), Roles: []string{domain.RoleAdmin}}

	if err := svc.Delete(context.Background(), admin, 1); err == nil {
		t.Fatal("harus error bila pembatalan task gagal")
	}
	if len(devs.deleted) != 0 {
		t.Fatal("device tidak boleh terhapus bila task belum berhasil dibatalkan")
	}
}
