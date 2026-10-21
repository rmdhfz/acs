package task

import (
	"context"
	"errors"
	"strings"
	"testing"

	"acs/internal/domain"
)

// repo device in-memory: id → device.
type summaryDevices struct {
	domain.DeviceRepository
	byID map[uint64]*domain.Device
}

func (r *summaryDevices) GetByID(_ context.Context, id uint64) (*domain.Device, error) {
	if d, ok := r.byID[id]; ok {
		return d, nil
	}
	return nil, domain.ErrNotFound
}

// parameter device: prefix data model yang "pernah dilaporkan" per device.
type summaryParams struct {
	domain.DeviceParameterRepository
	prefixByDevice map[uint64]string
}

func (r *summaryParams) HasPrefix(_ context.Context, id uint64, prefix string) (bool, error) {
	return r.prefixByDevice[id] == prefix, nil
}

type taskCall struct {
	Device uint64
	Names  []string
}

// RefreshDeviceSummary membuat task lewat CreateTask; supaya terisolasi, pakai Service yang CreateTask-nya
// memakai fake repos minimal (sama pola service_test.go): kita periksa task yang tersimpan di fakeTaskRepo.created.
func newSummarySvc(devs map[uint64]*domain.Device, prefixes map[uint64]string, pending int) (*Service, *fakeTaskRepo) {
	tasks := &fakeTaskRepo{pendingCountForTenant: pending}
	return &Service{
		tasks:        tasks,
		devices:      &summaryDevices{byID: devs},
		deviceParams: &summaryParams{prefixByDevice: prefixes},
		refs:         &fakeRefRepo{ids: map[string]uint64{domain.TaskStatusPending: 1, domain.TaskTypeGetParameterValues: 20}},
		activity:     &fakeActivityRepo{},
	}, tasks
}

func TestRefreshDeviceSummary_ScopeDataModelDanBatas(t *testing.T) {
	t7, t8 := uint64(7), uint64(8)
	devs := map[uint64]*domain.Device{
		1: {ID: 1, TenantID: &t7}, // TR-098
		2: {ID: 2, TenantID: &t7}, // TR-181
		3: {ID: 3, TenantID: &t7}, // data model belum diketahui
		4: {ID: 4, TenantID: &t8}, // tenant lain
	}
	prefixes := map[uint64]string{1: "InternetGatewayDevice.", 2: "Device."}
	admin := domain.Actor{UserID: 9, TenantID: &t7, Roles: []string{domain.RoleAdmin}}

	s, repo := newSummarySvc(devs, prefixes, 0)
	res, err := s.RefreshDeviceSummary(context.Background(), admin, []uint64{1, 2, 3, 4, 99, 1})
	if err != nil {
		t.Fatal(err)
	}
	if res.Queued != 2 {
		t.Fatalf("queued = %d, mau 2 (device 1 & 2; duplikat id 1 hanya sekali)", res.Queued)
	}
	reasons := map[uint64]string{}
	for _, sk := range res.Skipped {
		reasons[sk.DeviceID] = sk.Reason
	}
	if len(reasons) != 3 || !strings.Contains(reasons[3], "data model") || !strings.Contains(reasons[4], "bukan milik tenant") || !strings.Contains(reasons[99], "tidak ditemukan") {
		t.Fatalf("skipped salah: %+v", res.Skipped)
	}
	// Cabang yang dibaca sesuai data model.
	var got []string
	for _, tk := range repo.created {
		got = append(got, string(tk.Parameters))
	}
	joined := strings.Join(got, "|")
	if !strings.Contains(joined, "InternetGatewayDevice.WANDevice.1.WANConnectionDevice.") || !strings.Contains(joined, "Device.PPP.Interface.") {
		t.Fatalf("cabang PPP per data model tidak benar: %s", joined)
	}
}

func TestRefreshDeviceSummary_ValidasiDanSuperadmin(t *testing.T) {
	devs := map[uint64]*domain.Device{1: {ID: 1}}
	s, _ := newSummarySvc(devs, map[uint64]string{1: "InternetGatewayDevice."}, 0)
	super := domain.Actor{UserID: 1, Roles: []string{domain.RoleSuperadmin}}
	if _, err := s.RefreshDeviceSummary(context.Background(), super, nil); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("kosong harus ErrInvalidInput: %v", err)
	}
	many := make([]uint64, MaxSummaryRefreshDevices+1)
	for i := range many {
		many[i] = uint64(i + 1)
	}
	if _, err := s.RefreshDeviceSummary(context.Background(), super, many); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("melebihi batas harus ErrInvalidInput: %v", err)
	}
	res, err := s.RefreshDeviceSummary(context.Background(), super, []uint64{1})
	if err != nil || res.Queued != 1 {
		t.Fatalf("superadmin lintas tenant: res=%+v err=%v", res, err)
	}
}
