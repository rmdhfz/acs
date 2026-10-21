package device

import (
	"context"
	"errors"
	"testing"
	"time"

	"acs/internal/domain"
)

type lsDevices struct {
	domain.DeviceRepository
	rows []domain.Device
	last domain.DeviceFilter
}

func (r *lsDevices) List(_ context.Context, f domain.DeviceFilter, _ domain.Pagination) ([]domain.Device, int, error) {
	r.last = f
	return r.rows, len(r.rows), nil
}

type lsOptical struct {
	domain.DeviceOpticalMetricRepository
	m   map[uint64]domain.DeviceOpticalMetric
	err error
}

func (r *lsOptical) LatestForDevices(context.Context, []uint64) (map[uint64]domain.DeviceOpticalMetric, error) {
	return r.m, r.err
}

type lsParams struct {
	domain.DeviceParameterRepository
	m   map[uint64]string
	err error
}

func (r *lsParams) PPPoEUsernames(context.Context, []uint64) (map[uint64]string, error) {
	return r.m, r.err
}

func f64(v float64) *float64 { return &v }

// Daftar perangkat membawa redaman optik terakhir & username PPPoE; device tanpa data memberi null (bukan nol).
func TestListWithSummary_MenggabungkanOptikDanPPPoE(t *testing.T) {
	at := time.Date(2026, 10, 21, 3, 0, 0, 0, time.UTC)
	tenant := uint64(7)
	devs := &lsDevices{rows: []domain.Device{{ID: 1, TenantID: &tenant}, {ID: 2, TenantID: &tenant}, {ID: 3, TenantID: &tenant}}}
	s := &Service{
		devices:        devs,
		opticalMetrics: &lsOptical{m: map[uint64]domain.DeviceOpticalMetric{1: {DeviceID: 1, RxPowerDBM: f64(-26.2), TxPowerDBM: f64(2.1), RecordedAt: at}}},
		deviceParams:   &lsParams{m: map[uint64]string{2: "001003000206@dukodu.id"}},
	}
	actor := domain.Actor{UserID: 1, TenantID: &tenant, Roles: []string{domain.RoleAdmin}}
	items, total, err := s.ListWithSummary(context.Background(), actor, domain.DeviceFilter{}, domain.Pagination{Page: 1, PageSize: 50})
	if err != nil || total != 3 || len(items) != 3 {
		t.Fatalf("items=%d total=%d err=%v", len(items), total, err)
	}
	if items[0].RxPowerDBM == nil || *items[0].RxPowerDBM != -26.2 || items[0].OpticalAt == nil || !items[0].OpticalAt.Equal(at) || items[0].PPPoEUsername != nil {
		t.Fatalf("device 1 (hanya optik) salah: %+v", items[0])
	}
	if items[1].PPPoEUsername == nil || *items[1].PPPoEUsername != "001003000206@dukodu.id" || items[1].RxPowerDBM != nil {
		t.Fatalf("device 2 (hanya PPPoE) salah: %+v", items[1])
	}
	if items[2].RxPowerDBM != nil || items[2].PPPoEUsername != nil || items[2].OpticalAt != nil {
		t.Fatalf("device 3 tanpa data harus null semua: %+v", items[2])
	}
	if devs.last.TenantID == nil || *devs.last.TenantID != tenant {
		t.Fatalf("scope tenant actor tidak diterapkan: %+v", devs.last)
	}
}

// Ringkasan opsional: kegagalan membacanya tidak boleh menggagalkan daftar.
func TestListWithSummary_KegagalanRingkasanTidakMenggagalkanDaftar(t *testing.T) {
	tenant := uint64(7)
	s := &Service{
		devices:        &lsDevices{rows: []domain.Device{{ID: 1, TenantID: &tenant}}},
		opticalMetrics: &lsOptical{err: errors.New("db down")},
		deviceParams:   &lsParams{err: errors.New("db down")},
	}
	actor := domain.Actor{UserID: 1, TenantID: &tenant, Roles: []string{domain.RoleAdmin}}
	items, _, err := s.ListWithSummary(context.Background(), actor, domain.DeviceFilter{}, domain.Pagination{})
	if err != nil || len(items) != 1 || items[0].RxPowerDBM != nil || items[0].PPPoEUsername != nil {
		t.Fatalf("daftar harus tetap tampil: items=%+v err=%v", items, err)
	}
}
