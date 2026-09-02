package provisioning

import (
	"context"
	"fmt"
	"testing"
	"time"

	"acs/internal/domain"
)

// ---- fakes khusus engine preset ----

type fakePresetRepo struct {
	enforced []domain.Preset
}

func (f *fakePresetRepo) Create(context.Context, *domain.Preset) error { return nil }
func (f *fakePresetRepo) GetByID(context.Context, uint64) (*domain.Preset, error) {
	return nil, domain.ErrNotFound
}
func (f *fakePresetRepo) List(context.Context, *uint64, domain.Pagination) ([]domain.Preset, int, error) {
	return nil, 0, nil
}
func (f *fakePresetRepo) Update(context.Context, *domain.Preset) error { return nil }
func (f *fakePresetRepo) Delete(context.Context, uint64) error         { return nil }
func (f *fakePresetRepo) ListActiveEnforce(context.Context, *uint64) ([]domain.Preset, error) {
	return f.enforced, nil
}

type fakePresetAppRepo struct {
	rows map[string]*domain.PresetApplication // key: "presetID|deviceID"
}

func newFakePresetAppRepo() *fakePresetAppRepo {
	return &fakePresetAppRepo{rows: map[string]*domain.PresetApplication{}}
}
func appKey(p, d uint64) string { return fmt.Sprintf("%d|%d", p, d) }
func (f *fakePresetAppRepo) Get(_ context.Context, presetID, deviceID uint64) (*domain.PresetApplication, error) {
	if a, ok := f.rows[appKey(presetID, deviceID)]; ok {
		return a, nil
	}
	return nil, domain.ErrNotFound
}
func (f *fakePresetAppRepo) Upsert(_ context.Context, a *domain.PresetApplication) error {
	cp := *a
	f.rows[appKey(a.PresetID, a.DeviceID)] = &cp
	return nil
}

// presetEvalService merakit Service hanya dengan dependensi yang dipakai
// EvaluatePresets.
func presetEvalService(presets *fakePresetRepo, apps *fakePresetAppRepo, enq *fakeEnqueuerPV, params map[string]string) *Service {
	valMap := make(map[string]string, len(params))
	for k, v := range params {
		valMap[k] = v
	}
	return &Service{
		deviceParams: &fakeDeviceParamRepoPV{values: valMap},
		enqueuer:     enq,
		activity:     &fakeActivityRepoPV{},
		presets:      presets,
		presetApps:   apps,
	}
}

func enforcedPreset(id uint64, precondition, configs string) domain.Preset {
	return domain.Preset{ID: id, IsActive: true, Enforce: true, Precondition: precondition, Configurations: configs}
}

// ---- tests ----

func TestEvaluatePresets_NilEngineIsNoOp(t *testing.T) {
	s := &Service{} // presets/presetApps nil
	if err := s.EvaluatePresets(context.Background(), domain.Actor{}, &domain.Device{ID: 1}); err != nil {
		t.Fatalf("nil engine harus no-op, got %v", err)
	}
}

func TestEvaluatePresets_DriftEnqueuesSetParameterValues(t *testing.T) {
	dev := &domain.Device{ID: 7}
	enq := &fakeEnqueuerPV{}
	presets := &fakePresetRepo{enforced: []domain.Preset{
		enforcedPreset(1, `{}`, `[{"op":"set_parameter","key":"Device.WiFi.SSID","value":"MyISP"}]`),
	}}
	// device_parameters saat ini: SSID masih nilai lama -> drift
	s := presetEvalService(presets, newFakePresetAppRepo(), enq, map[string]string{
		paramKey(7, "Device.WiFi.SSID"): "OldSSID",
	})

	if err := s.EvaluatePresets(context.Background(), domain.Actor{}, dev); err != nil {
		t.Fatalf("EvaluatePresets: %v", err)
	}
	if enq.setParamsCalls != 1 {
		t.Fatalf("drift harus memicu 1 SetParameterValues, got %d", enq.setParamsCalls)
	}
	if got := enq.lastSetParams["Device.WiFi.SSID"]; got != "MyISP" {
		t.Fatalf("nilai target salah: got %q", got)
	}
}

func TestEvaluatePresets_ConvergedDeviceNoEnqueue(t *testing.T) {
	dev := &domain.Device{ID: 7}
	enq := &fakeEnqueuerPV{}
	presets := &fakePresetRepo{enforced: []domain.Preset{
		enforcedPreset(1, `{}`, `[{"op":"set_parameter","key":"Device.WiFi.SSID","value":"MyISP"}]`),
	}}
	apps := newFakePresetAppRepo()
	s := presetEvalService(presets, apps, enq, map[string]string{
		paramKey(7, "Device.WiFi.SSID"): "MyISP", // sudah sesuai
	})

	if err := s.EvaluatePresets(context.Background(), domain.Actor{}, dev); err != nil {
		t.Fatalf("EvaluatePresets: %v", err)
	}
	if enq.setParamsCalls != 0 {
		t.Fatalf("device konvergen tidak boleh memicu task, got %d", enq.setParamsCalls)
	}
	if a, _ := apps.Get(context.Background(), 1, 7); a == nil || a.Status != domain.PresetApplicationStatusConverged {
		t.Fatalf("status harus CONVERGED, got %+v", a)
	}
}

func TestEvaluatePresets_PendingTaskSkips(t *testing.T) {
	dev := &domain.Device{ID: 7}
	enq := &fakeEnqueuerPV{hasPending: true}
	presets := &fakePresetRepo{enforced: []domain.Preset{
		enforcedPreset(1, `{}`, `[{"op":"set_parameter","key":"Device.WiFi.SSID","value":"MyISP"}]`),
	}}
	s := presetEvalService(presets, newFakePresetAppRepo(), enq, map[string]string{
		paramKey(7, "Device.WiFi.SSID"): "OldSSID",
	})
	if err := s.EvaluatePresets(context.Background(), domain.Actor{}, dev); err != nil {
		t.Fatalf("EvaluatePresets: %v", err)
	}
	if enq.setParamsCalls != 0 {
		t.Fatalf("task in-flight harus menahan preset, got %d", enq.setParamsCalls)
	}
}

func TestEvaluatePresets_PreconditionMismatch(t *testing.T) {
	vid := uint64(3)
	dev := &domain.Device{ID: 7, VendorID: &vid}
	enq := &fakeEnqueuerPV{}
	presets := &fakePresetRepo{enforced: []domain.Preset{
		enforcedPreset(1, `{"vendor_id":99}`, `[{"op":"set_parameter","key":"Device.WiFi.SSID","value":"MyISP"}]`),
	}}
	s := presetEvalService(presets, newFakePresetAppRepo(), enq, map[string]string{
		paramKey(7, "Device.WiFi.SSID"): "OldSSID",
	})
	if err := s.EvaluatePresets(context.Background(), domain.Actor{}, dev); err != nil {
		t.Fatalf("EvaluatePresets: %v", err)
	}
	if enq.setParamsCalls != 0 {
		t.Fatalf("precondition tidak cocok harus 0 task, got %d", enq.setParamsCalls)
	}
}

func TestEvaluatePresets_CooldownSkips(t *testing.T) {
	dev := &domain.Device{ID: 7}
	enq := &fakeEnqueuerPV{}
	presets := &fakePresetRepo{enforced: []domain.Preset{
		enforcedPreset(1, `{}`, `[{"op":"set_parameter","key":"Device.WiFi.SSID","value":"MyISP"}]`),
	}}
	apps := newFakePresetAppRepo()
	recent := time.Now().Add(-1 * time.Minute)
	_ = apps.Upsert(context.Background(), &domain.PresetApplication{
		PresetID: 1, DeviceID: 7, LastAppliedAt: &recent, Status: domain.PresetApplicationStatusPending,
	})
	s := presetEvalService(presets, apps, enq, map[string]string{
		paramKey(7, "Device.WiFi.SSID"): "OldSSID",
	})
	if err := s.EvaluatePresets(context.Background(), domain.Actor{}, dev); err != nil {
		t.Fatalf("EvaluatePresets: %v", err)
	}
	if enq.setParamsCalls != 0 {
		t.Fatalf("cooldown (<15m) harus menahan re-apply, got %d", enq.setParamsCalls)
	}
}

func TestEvaluatePresets_GaveUpAfterMaxFailures(t *testing.T) {
	dev := &domain.Device{ID: 7}
	enq := &fakeEnqueuerPV{}
	presets := &fakePresetRepo{enforced: []domain.Preset{
		enforcedPreset(1, `{}`, `[{"op":"set_parameter","key":"Device.WiFi.SSID","value":"MyISP"}]`),
	}}
	apps := newFakePresetAppRepo()
	old := time.Now().Add(-1 * time.Hour)
	_ = apps.Upsert(context.Background(), &domain.PresetApplication{
		PresetID: 1, DeviceID: 7, LastAppliedAt: &old,
		ConsecutiveFailures: presetMaxConsecutiveFailures, Status: domain.PresetApplicationStatusFailed,
	})
	s := presetEvalService(presets, apps, enq, map[string]string{
		paramKey(7, "Device.WiFi.SSID"): "OldSSID",
	})
	if err := s.EvaluatePresets(context.Background(), domain.Actor{}, dev); err != nil {
		t.Fatalf("EvaluatePresets: %v", err)
	}
	if enq.setParamsCalls != 0 {
		t.Fatalf("preset FAILED (menyerah) tidak boleh dicoba lagi, got %d", enq.setParamsCalls)
	}
}
