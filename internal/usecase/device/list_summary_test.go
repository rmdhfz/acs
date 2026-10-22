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
	// proyeksi ?params=
	vals     map[uint64]map[string]string
	valNames []string
}

func (r *lsParams) PPPoEUsernames(context.Context, []uint64) (map[uint64]string, error) {
	return r.m, r.err
}

func (r *lsParams) ValuesFor(_ context.Context, _ []uint64, names []string) (map[uint64]map[string]string, error) {
	r.valNames = names
	return r.vals, r.err
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

// Query bebas: parameter rahasia (password/PSK/...) tidak boleh dipakai sebagai filter atau diminta oleh peran tanpa hak melihat
// rahasia — kalau tidak, filter eq/prefix menjadi oracle untuk menebak nilainya walau respons menyamarkan.
func TestList_ParameterRahasiaDitolakUntukPeranTanpaHak(t *testing.T) {
	tenant := uint64(7)
	psk := "InternetGatewayDevice.LANDevice.1.WLANConfiguration.1.PreSharedKey.1.KeyPassphrase"
	viewer := domain.Actor{UserID: 1, TenantID: &tenant, Roles: []string{domain.RoleViewer}}
	admin := domain.Actor{UserID: 2, TenantID: &tenant, Roles: []string{domain.RoleAdmin}}

	for name, f := range map[string]domain.DeviceFilter{
		"filter":   {Params: []domain.ParamCond{{Name: psk, Op: domain.ParamOpPrefix, Value: "a"}}},
		"proyeksi": {ProjectParams: []string{psk}},
	} {
		s := &Service{devices: &lsDevices{}}
		if _, _, err := s.ListWithSummary(context.Background(), viewer, f, domain.Pagination{}); !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("%s oleh VIEWER harus ErrInvalidInput, got %v", name, err)
		}
		if _, _, err := s.ListWithSummary(context.Background(), admin, f, domain.Pagination{}); err != nil {
			t.Errorf("%s oleh ADMIN (boleh melihat rahasia) harus lolos, got %v", name, err)
		}
	}
}

func TestList_BatasQueryBebas(t *testing.T) {
	tenant := uint64(7)
	actor := domain.Actor{UserID: 1, TenantID: &tenant, Roles: []string{domain.RoleAdmin}}
	s := &Service{devices: &lsDevices{}}
	many := make([]domain.ParamCond, domain.MaxParamConds+1)
	for i := range many {
		many[i] = domain.ParamCond{Name: "A.B", Op: domain.ParamOpExists}
	}
	if _, _, err := s.List(context.Background(), actor, domain.DeviceFilter{Params: many}, domain.Pagination{}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("terlalu banyak kondisi harus ditolak, got %v", err)
	}
	names := make([]string, domain.MaxProjectParams+1)
	for i := range names {
		names[i] = "A.B"
	}
	if _, _, err := s.List(context.Background(), actor, domain.DeviceFilter{ProjectParams: names}, domain.Pagination{}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("terlalu banyak proyeksi harus ditolak, got %v", err)
	}
}

// Proyeksi params= memuat nilai per device (tanpa N+1: satu panggilan batch); device tanpa nilai = tanpa field parameters.
func TestListWithSummary_ProyeksiParameter(t *testing.T) {
	tenant := uint64(7)
	ssid := "InternetGatewayDevice.LANDevice.1.WLANConfiguration.1.SSID"
	params := &lsParams{vals: map[uint64]map[string]string{1: {ssid: "Rumah"}}}
	s := &Service{
		devices:      &lsDevices{rows: []domain.Device{{ID: 1, TenantID: &tenant}, {ID: 2, TenantID: &tenant}}},
		deviceParams: params,
	}
	actor := domain.Actor{UserID: 1, TenantID: &tenant, Roles: []string{domain.RoleViewer}}
	items, _, err := s.ListWithSummary(context.Background(), actor, domain.DeviceFilter{ProjectParams: []string{ssid}}, domain.Pagination{})
	if err != nil {
		t.Fatal(err)
	}
	if items[0].Parameters[ssid] != "Rumah" || items[1].Parameters != nil {
		t.Fatalf("proyeksi salah: %+v / %+v", items[0].Parameters, items[1].Parameters)
	}
	if len(params.valNames) != 1 || params.valNames[0] != ssid {
		t.Fatalf("nama yang diminta tidak diteruskan: %v", params.valNames)
	}
}
