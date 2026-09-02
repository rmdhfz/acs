package provisioning

import (
	"context"
	"errors"
	"testing"
	"time"

	"acs/internal/domain"
)

// ---- fakes (pola sama seperti internal/usecase/task/service_test.go) ----

type fakeProfileRepo struct {
	byID map[uint64]*domain.ProvisioningProfile
}

func (f *fakeProfileRepo) Create(context.Context, *domain.ProvisioningProfile) error { return nil }
func (f *fakeProfileRepo) GetByID(_ context.Context, id uint64) (*domain.ProvisioningProfile, error) {
	if p, ok := f.byID[id]; ok {
		return p, nil
	}
	return nil, domain.ErrNotFound
}
func (f *fakeProfileRepo) GetByUUID(context.Context, string) (*domain.ProvisioningProfile, error) {
	return nil, domain.ErrNotFound
}
func (f *fakeProfileRepo) List(context.Context, *uint64, domain.Pagination) ([]domain.ProvisioningProfile, int, error) {
	return nil, 0, nil
}
func (f *fakeProfileRepo) Update(context.Context, *domain.ProvisioningProfile) error { return nil }
func (f *fakeProfileRepo) SoftDelete(context.Context, uint64, uint64) error          { return nil }

type fakeProfileParamRepo struct {
	byProfile map[uint64][]domain.ProvisioningProfileParameter
}

func (f *fakeProfileParamRepo) Replace(context.Context, uint64, []domain.ProvisioningProfileParameter) error {
	return nil
}
func (f *fakeProfileParamRepo) ListByProfile(_ context.Context, profileID uint64) ([]domain.ProvisioningProfileParameter, error) {
	return f.byProfile[profileID], nil
}

type fakeZTRuleRepo struct{ rules []domain.ZeroTouchRule }

func (f *fakeZTRuleRepo) Create(context.Context, *domain.ZeroTouchRule) error { return nil }
func (f *fakeZTRuleRepo) GetByID(context.Context, uint64) (*domain.ZeroTouchRule, error) {
	return nil, domain.ErrNotFound
}
func (f *fakeZTRuleRepo) ListActiveOrdered(context.Context, *uint64) ([]domain.ZeroTouchRule, error) {
	return f.rules, nil
}
func (f *fakeZTRuleRepo) Update(context.Context, *domain.ZeroTouchRule) error { return nil }
func (f *fakeZTRuleRepo) SoftDelete(context.Context, uint64, uint64) error    { return nil }

type fakeDeviceRepoPV struct{ dev *domain.Device }

func (f *fakeDeviceRepoPV) Create(context.Context, *domain.Device) error { return nil }
func (f *fakeDeviceRepoPV) GetByID(context.Context, uint64) (*domain.Device, error) {
	return f.dev, nil
}
func (f *fakeDeviceRepoPV) GetByUUID(context.Context, string) (*domain.Device, error) {
	return nil, domain.ErrNotFound
}
func (f *fakeDeviceRepoPV) GetByOUISerial(context.Context, string, string) (*domain.Device, error) {
	return nil, domain.ErrNotFound
}
func (f *fakeDeviceRepoPV) List(context.Context, domain.DeviceFilter, domain.Pagination) ([]domain.Device, int, error) {
	return nil, 0, nil
}
func (f *fakeDeviceRepoPV) CountByStatus(context.Context, *uint64) ([]domain.DeviceStatusCount, error) {
	return nil, nil
}
func (f *fakeDeviceRepoPV) CountByVendor(context.Context, *uint64) ([]domain.DeviceVendorCount, error) {
	return nil, nil
}
func (f *fakeDeviceRepoPV) Update(_ context.Context, d *domain.Device) error { f.dev = d; return nil }
func (f *fakeDeviceRepoPV) UpdateStatus(context.Context, uint64, uint64, *uint64) error {
	return nil
}
func (f *fakeDeviceRepoPV) MarkStaleOffline(context.Context, uint64, time.Time) (int64, error) {
	return 0, nil
}
func (f *fakeDeviceRepoPV) SoftDelete(context.Context, uint64, uint64) error { return nil }

// fakeDeviceParamRepoPV — key gabungan deviceID+name, cukup utk uji
// precondition MatchParameterName/MatchParameterValuePattern.
type fakeDeviceParamRepoPV struct {
	values map[string]string // key: fmt.Sprintf("%d|%s", deviceID, name)
}

func paramKey(deviceID uint64, name string) string {
	return fmtUint(deviceID) + "|" + name
}
func fmtUint(v uint64) string {
	// hindari import "fmt" cuma utk satu fungsi kecil di file test
	if v == 0 {
		return "0"
	}
	digits := []byte{}
	for v > 0 {
		digits = append([]byte{byte('0' + v%10)}, digits...)
		v /= 10
	}
	return string(digits)
}
func (f *fakeDeviceParamRepoPV) Upsert(context.Context, *domain.DeviceParameter) error { return nil }
func (f *fakeDeviceParamRepoPV) UpsertBatch(context.Context, []domain.DeviceParameter) error {
	return nil
}
func (f *fakeDeviceParamRepoPV) ListByDevice(context.Context, uint64, string) ([]domain.DeviceParameter, error) {
	return nil, nil
}
func (f *fakeDeviceParamRepoPV) Get(_ context.Context, deviceID uint64, name string) (*domain.DeviceParameter, error) {
	val, ok := f.values[paramKey(deviceID, name)]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return &domain.DeviceParameter{DeviceID: deviceID, ParameterName: name, ParameterValue: &val}, nil
}

// fakeRefRepoPV memetakan kode -> ID tetap per tabel (diasumsikan nama kode
// unik lintas tabel di test ini, cukup sederhana utk kebutuhan pengujian).
type fakeRefRepoPV struct{ ids map[string]uint64 }

func (f *fakeRefRepoPV) GetByCode(_ context.Context, _ string, code string) (domain.RefLookup, error) {
	id, ok := f.ids[code]
	if !ok {
		return domain.RefLookup{}, domain.ErrNotFound
	}
	return domain.RefLookup{ID: id, Code: code}, nil
}
func (f *fakeRefRepoPV) GetByID(context.Context, string, uint64) (domain.RefLookup, error) {
	return domain.RefLookup{}, domain.ErrNotFound
}
func (f *fakeRefRepoPV) List(context.Context, string) ([]domain.RefLookup, error) { return nil, nil }

// fakeEnqueuerPV mencatat pemanggilan EnqueueSetParameterValues/EnqueueReboot,
// dan bisa disetel HasPendingForDevice utk uji pagar anti-reboot-loop.
type fakeEnqueuerPV struct {
	setParamsCalls int
	rebootCalls    int
	hasPending     bool
	lastSetParams  map[string]string
	setParamsErr   error
}

func (f *fakeEnqueuerPV) EnqueueSetParameterValues(_ context.Context, _ domain.Actor, _ uint64, params map[string]string, _ uint8) (*domain.Task, error) {
	f.setParamsCalls++
	f.lastSetParams = params
	if f.setParamsErr != nil {
		return nil, f.setParamsErr
	}
	return &domain.Task{ID: 1}, nil
}
func (f *fakeEnqueuerPV) EnqueueGetParameterNames(context.Context, domain.Actor, uint64, string, bool, uint8) (*domain.Task, error) {
	return &domain.Task{ID: 3}, nil
}
func (f *fakeEnqueuerPV) EnqueueReboot(context.Context, domain.Actor, uint64, uint8) (*domain.Task, error) {
	f.rebootCalls++
	return &domain.Task{ID: 2}, nil
}
func (f *fakeEnqueuerPV) HasPendingForDevice(context.Context, uint64) (bool, error) {
	return f.hasPending, nil
}
func (f *fakeEnqueuerPV) ResolveParameterPath(_ context.Context, _ uint64, key string) (string, error) {
	return key, nil
}

type fakeFirmwareSchedulerPV struct{ calls int }

func (f *fakeFirmwareSchedulerPV) ScheduleUpgrade(context.Context, domain.Actor, uint64, uint64, *time.Time) (*domain.FirmwareUpgradeJob, error) {
	f.calls++
	return &domain.FirmwareUpgradeJob{ID: 1}, nil
}

// fakeActivityRepoPV -- Record menyimpan SEMUA log ke slice (bukan no-op),
// ListByEntity membaca balik dari slice yang sama (terurut terbaru dulu,
// meniru ORDER BY id DESC sungguhan) -- dipakai TestEvaluateZeroTouch_
// ActionCooldown utk memverifikasi withinActionCooldown sungguhan membaca
// hasil Record sebelumnya, bukan cuma unit terisolasi.
type fakeActivityRepoPV struct {
	logs   []domain.ActivityLog
	seeded []domain.ActivityLog // entry tambahan yg "sudah ada" sebelum test mulai
}

func (f *fakeActivityRepoPV) Record(_ context.Context, log *domain.ActivityLog) error {
	if log.CreatedAt.IsZero() {
		log.CreatedAt = time.Now()
	}
	f.logs = append(f.logs, *log)
	return nil
}
func (f *fakeActivityRepoPV) ListByEntity(_ context.Context, entityType string, entityID uint64, p domain.Pagination) ([]domain.ActivityLog, int, error) {
	// Gabung seeded (dianggap paling lama) + logs (dari Record, urutan
	// kemunculan = terlama->terbaru), lalu balik urutan supaya TERBARU dulu
	// (meniru ORDER BY id DESC sungguhan).
	all := append(append([]domain.ActivityLog{}, f.seeded...), f.logs...)
	var matched []domain.ActivityLog
	for i := len(all) - 1; i >= 0; i-- {
		if all[i].EntityType == entityType && all[i].EntityID != nil && *all[i].EntityID == entityID {
			matched = append(matched, all[i])
		}
	}
	limit := p.Limit()
	if limit > len(matched) {
		limit = len(matched)
	}
	return matched[:limit], len(matched), nil
}

// ---- helpers ----

const (
	idBootstrapOnly   = 1
	idBootstrapOrBoot = 2
	idEveryInform     = 3
)

func newTestService(rules []domain.ZeroTouchRule, dev *domain.Device, enqueuer *fakeEnqueuerPV, fwSvc *fakeFirmwareSchedulerPV, params map[string]string) *Service {
	return newTestServiceWithActivity(rules, dev, enqueuer, fwSvc, params, &fakeActivityRepoPV{})
}

func newTestServiceWithActivity(rules []domain.ZeroTouchRule, dev *domain.Device, enqueuer *fakeEnqueuerPV, fwSvc *fakeFirmwareSchedulerPV, params map[string]string, activity *fakeActivityRepoPV) *Service {
	valMap := make(map[string]string, len(params))
	for k, v := range params {
		valMap[k] = v
	}
	return NewService(
		&fakeProfileRepo{byID: map[uint64]*domain.ProvisioningProfile{
			10: {ID: 10, Name: "Test Profile", IsActive: true},
		}},
		&fakeProfileParamRepo{byProfile: map[uint64][]domain.ProvisioningProfileParameter{
			10: {{ProfileID: 10, ParameterName: "wifi.ssid", ParameterValue: strPtr("Test")}},
		}},
		&fakeZTRuleRepo{rules: rules},
		&fakeDeviceRepoPV{dev: dev},
		&fakeDeviceParamRepoPV{values: valMap},
		&fakeRefRepoPV{ids: map[string]uint64{
			domain.ZtpTriggerEventBootstrapOnly:   idBootstrapOnly,
			domain.ZtpTriggerEventBootstrapOrBoot: idBootstrapOrBoot,
			domain.ZtpTriggerEventEveryInform:     idEveryInform,
			domain.TaskStatusPending:              1,
		}},
		enqueuer,
		fwSvc,
		activity,
		nil, // presets — engine preset diuji terpisah (preset_eval_test.go)
		nil, // presetApps
	)
}

func strPtr(s string) *string { return &s }

// ---- tests ----

func TestEvaluateZeroTouch_SoftwareVersionPattern(t *testing.T) {
	sv := "v1.2.3"
	dev := &domain.Device{ID: 1, SerialNumber: "SN1", SoftwareVersion: &sv}
	rules := []domain.ZeroTouchRule{
		{ID: 1, TriggerEventID: idBootstrapOnly, SoftwareVersionPattern: strPtr("v1.%"), ProvisioningProfileID: uint64Ptr(10), IsActive: true},
	}
	enq := &fakeEnqueuerPV{}
	svc := newTestService(rules, dev, enq, &fakeFirmwareSchedulerPV{}, nil)

	// Superadmin actor -- rule ini mengeksekusi aksi ProvisioningProfileID
	// (ApplyProfile), yang menegakkan auth.RequireTenantScope thd device;
	// device di test ini sengaja tanpa TenantID (dev lintas-tenant/belum
	// diketahui), jadi hanya superadmin yang lolos (sama seperti systemActor
	// ZTP asli di usecase/session yang selalu pakai TenantID device apa
	// adanya -- di sini disederhanakan krn yang diuji adalah precondition
	// matching, bukan RBAC ApplyProfile itu sendiri).
	matched, err := svc.EvaluateZeroTouch(context.Background(), domain.Actor{Roles: []string{domain.RoleSuperadmin}}, dev, []string{domain.ZtpTriggerEventBootstrapOnly, domain.ZtpTriggerEventEveryInform})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if matched == nil || matched.ID != 1 {
		t.Fatalf("rule seharusnya cocok via SoftwareVersionPattern, got %+v", matched)
	}
	if enq.setParamsCalls != 1 {
		t.Fatalf("ApplyProfile (EnqueueSetParameterValues) seharusnya terpanggil sekali, got %d", enq.setParamsCalls)
	}
}

func TestEvaluateZeroTouch_SoftwareVersionPattern_NoMatch(t *testing.T) {
	sv := "v2.0.0"
	dev := &domain.Device{ID: 1, SerialNumber: "SN1", SoftwareVersion: &sv}
	rules := []domain.ZeroTouchRule{
		{ID: 1, TriggerEventID: idBootstrapOnly, SoftwareVersionPattern: strPtr("v1.%"), ProvisioningProfileID: uint64Ptr(10), IsActive: true},
	}
	svc := newTestService(rules, dev, &fakeEnqueuerPV{}, &fakeFirmwareSchedulerPV{}, nil)

	_, err := svc.EvaluateZeroTouch(context.Background(), domain.Actor{}, dev, []string{domain.ZtpTriggerEventBootstrapOnly})
	if !errors.Is(err, domain.ErrNoMatchingRule) {
		t.Fatalf("error = %v, want ErrNoMatchingRule (versi tidak cocok pola)", err)
	}
}

func TestEvaluateZeroTouch_MatchParameterNamePair(t *testing.T) {
	dev := &domain.Device{ID: 5, SerialNumber: "SN5"}
	rules := []domain.ZeroTouchRule{
		{
			ID: 1, TriggerEventID: idEveryInform,
			MatchParameterName: strPtr("Device.DeviceInfo.X_OUTDATED"), MatchParameterValuePattern: strPtr("true"),
			PostApplyReboot: true, IsActive: true,
		},
	}
	enq := &fakeEnqueuerPV{}
	svc := newTestService(rules, dev, enq, &fakeFirmwareSchedulerPV{}, map[string]string{
		paramKey(5, "Device.DeviceInfo.X_OUTDATED"): "true",
	})

	matched, err := svc.EvaluateZeroTouch(context.Background(), domain.Actor{}, dev, []string{domain.ZtpTriggerEventEveryInform})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if matched == nil {
		t.Fatal("rule seharusnya cocok via MatchParameterName/ValuePattern")
	}
	if enq.rebootCalls != 1 {
		t.Fatalf("PostApplyReboot seharusnya memicu EnqueueReboot sekali, got %d", enq.rebootCalls)
	}
}

func TestEvaluateZeroTouch_FirmwareFileIDAction(t *testing.T) {
	dev := &domain.Device{ID: 6, SerialNumber: "SN6"}
	rules := []domain.ZeroTouchRule{
		{ID: 1, TriggerEventID: idBootstrapOnly, FirmwareFileID: uint64Ptr(99), IsActive: true},
	}
	fw := &fakeFirmwareSchedulerPV{}
	svc := newTestService(rules, dev, &fakeEnqueuerPV{}, fw, nil)

	_, err := svc.EvaluateZeroTouch(context.Background(), domain.Actor{}, dev, []string{domain.ZtpTriggerEventBootstrapOnly})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fw.calls != 1 {
		t.Fatalf("FirmwareFileID seharusnya memicu ScheduleUpgrade sekali, got %d", fw.calls)
	}
}

func TestEvaluateZeroTouch_BootstrapOnlyGuard_SkipsAlreadyProvisioned(t *testing.T) {
	dev := &domain.Device{ID: 7, SerialNumber: "SN7", ProvisioningProfileID: uint64Ptr(999)}
	rules := []domain.ZeroTouchRule{
		{ID: 1, TriggerEventID: idBootstrapOnly, ProvisioningProfileID: uint64Ptr(10), IsActive: true},
	}
	svc := newTestService(rules, dev, &fakeEnqueuerPV{}, &fakeFirmwareSchedulerPV{}, nil)

	_, err := svc.EvaluateZeroTouch(context.Background(), domain.Actor{}, dev, []string{domain.ZtpTriggerEventBootstrapOnly})
	if !errors.Is(err, domain.ErrNoMatchingRule) {
		t.Fatalf("error = %v, want ErrNoMatchingRule -- rule BOOTSTRAP_ONLY harus di-skip utk device yg sudah terprovisioning", err)
	}
}

func TestEvaluateZeroTouch_NonBootstrapOnly_BypassesAlreadyProvisionedGuard(t *testing.T) {
	dev := &domain.Device{ID: 8, SerialNumber: "SN8", ProvisioningProfileID: uint64Ptr(999)}
	rules := []domain.ZeroTouchRule{
		{ID: 1, TriggerEventID: idEveryInform, PostApplyReboot: true, IsActive: true},
	}
	enq := &fakeEnqueuerPV{}
	svc := newTestService(rules, dev, enq, &fakeFirmwareSchedulerPV{}, nil)

	matched, err := svc.EvaluateZeroTouch(context.Background(), domain.Actor{}, dev, []string{domain.ZtpTriggerEventEveryInform})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if matched == nil {
		t.Fatal("rule EVERY_INFORM seharusnya TETAP dievaluasi walau device sudah terprovisioning")
	}
	if enq.rebootCalls != 1 {
		t.Fatalf("aksi reboot seharusnya tetap terpicu, got %d calls", enq.rebootCalls)
	}
}

func TestEvaluateZeroTouch_TriggerFiltering_BootstrapOnlyNotFiredOnPeriodic(t *testing.T) {
	dev := &domain.Device{ID: 9, SerialNumber: "SN9"}
	rules := []domain.ZeroTouchRule{
		{ID: 1, TriggerEventID: idBootstrapOnly, ProvisioningProfileID: uint64Ptr(10), IsActive: true},
	}
	svc := newTestService(rules, dev, &fakeEnqueuerPV{}, &fakeFirmwareSchedulerPV{}, nil)

	// Inform periodik biasa -- triggerCodes cuma EVERY_INFORM (tidak ada
	// BOOTSTRAP/BOOT), rule ber-trigger BOOTSTRAP_ONLY tidak boleh cocok.
	_, err := svc.EvaluateZeroTouch(context.Background(), domain.Actor{}, dev, []string{domain.ZtpTriggerEventEveryInform})
	if !errors.Is(err, domain.ErrNoMatchingRule) {
		t.Fatalf("error = %v, want ErrNoMatchingRule -- rule BOOTSTRAP_ONLY tidak boleh dievaluasi saat Inform periodik biasa", err)
	}
}

func TestEvaluateZeroTouch_AntiLoopGuard_SkipsWhenDeviceHasPendingTask(t *testing.T) {
	dev := &domain.Device{ID: 10, SerialNumber: "SN10"}
	rules := []domain.ZeroTouchRule{
		{ID: 1, TriggerEventID: idEveryInform, PostApplyReboot: true, IsActive: true},
	}
	enq := &fakeEnqueuerPV{hasPending: true}
	svc := newTestService(rules, dev, enq, &fakeFirmwareSchedulerPV{}, nil)

	_, err := svc.EvaluateZeroTouch(context.Background(), domain.Actor{}, dev, []string{domain.ZtpTriggerEventEveryInform})
	if !errors.Is(err, domain.ErrNoMatchingRule) {
		t.Fatalf("error = %v, want ErrNoMatchingRule -- rule non-bootstrap-only harus di-skip saat device masih py task pending", err)
	}
	if enq.rebootCalls != 0 {
		t.Fatal("EnqueueReboot TIDAK seharusnya terpanggil saat pagar anti-loop aktif")
	}
}

func TestJitterPeriodicInformInterval(t *testing.T) {
	t.Run("nilai non-numerik dikembalikan apa adanya", func(t *testing.T) {
		if got := jitterPeriodicInformInterval(1, "not-a-number"); got != "not-a-number" {
			t.Fatalf("got %q, want unchanged", got)
		}
	})
	t.Run("deterministik -- device yg sama selalu dapat hasil sama", func(t *testing.T) {
		a := jitterPeriodicInformInterval(42, "3600")
		b := jitterPeriodicInformInterval(42, "3600")
		if a != b {
			t.Fatalf("hasil jitter tidak deterministik: %q vs %q", a, b)
		}
	})
	t.Run("hasil dalam rentang ±10%% dari base", func(t *testing.T) {
		base := 3600
		for _, deviceID := range []uint64{1, 2, 3, 100, 99999} {
			got := jitterPeriodicInformInterval(deviceID, "3600")
			gotInt := atoiT(t, got)
			spread := base * 10 / 100
			if gotInt < base-spread || gotInt > base+spread {
				t.Fatalf("device %d: hasil %d di luar rentang [%d,%d]", deviceID, gotInt, base-spread, base+spread)
			}
		}
	})
}

func TestIsPeriodicInformIntervalKey(t *testing.T) {
	cases := map[string]bool{
		"device.periodic_inform_interval":                               true,
		"InternetGatewayDevice.ManagementServer.PeriodicInformInterval": true,
		"Device.ManagementServer.PeriodicInformInterval":                true,
		"wifi.ssid": false,
		"InternetGatewayDevice.ManagementServer.ConnectionRequestURL": false,
	}
	for key, want := range cases {
		if got := isPeriodicInformIntervalKey(key); got != want {
			t.Errorf("isPeriodicInformIntervalKey(%q) = %v, want %v", key, got, want)
		}
	}
}

func uint64Ptr(v uint64) *uint64 { return &v }

// TestEvaluateZeroTouch_ActionCooldown_CrossSessionLoop menguji skenario
// PERSIS yang membuat pagar HasPendingForDevice saja tidak cukup (temuan
// review kode+keamanan independen): rule EVERY_INFORM+PostApplyReboot sudah
// pernah menembak aksi utk device ini BARU-BARU INI (dicatat via
// ZERO_TOUCH_MATCH+rule_id di activity_logs), device TIDAK py task pending
// (skenario: task reboot sebelumnya sudah COMPLETED, device sudah balik
// online lewat sesi/Inform baru -- persis kondisi yang bikin lapis pertama
// "bersih" lagi) -- evaluasi rule KEDUA ini harus tetap ditahan cooldown,
// TIDAK menembak reboot lagi.
func TestEvaluateZeroTouch_ActionCooldown_CrossSessionLoop(t *testing.T) {
	dev := &domain.Device{ID: 11, SerialNumber: "SN11"}
	rule := domain.ZeroTouchRule{ID: 42, TriggerEventID: idEveryInform, PostApplyReboot: true, IsActive: true}
	rules := []domain.ZeroTouchRule{rule}

	t.Run("belum pernah menembak -> aksi jalan normal", func(t *testing.T) {
		enq := &fakeEnqueuerPV{}
		svc := newTestService(rules, dev, enq, &fakeFirmwareSchedulerPV{}, nil)
		matched, err := svc.EvaluateZeroTouch(context.Background(), domain.Actor{}, dev, []string{domain.ZtpTriggerEventEveryInform})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if matched == nil || enq.rebootCalls != 1 {
			t.Fatalf("tembakan PERTAMA seharusnya berhasil, matched=%+v rebootCalls=%d", matched, enq.rebootCalls)
		}
	})

	t.Run("sudah pernah menembak dalam cooldown, device TIDAK py pending task -> tetap ditahan", func(t *testing.T) {
		activity := &fakeActivityRepoPV{
			seeded: []domain.ActivityLog{{
				EntityType: "device", EntityID: &dev.ID, Action: "ZERO_TOUCH_MATCH",
				Description: strPtr("rule_id=42"), CreatedAt: time.Now().Add(-10 * time.Minute), // 10 menit lalu, < cooldown 1 jam
			}},
		}
		// hasPending=false -- meniru task reboot sebelumnya SUDAH COMPLETED &
		// device sudah balik online lewat sesi baru (lapis pertama "bersih").
		enq := &fakeEnqueuerPV{hasPending: false}
		svc := newTestServiceWithActivity(rules, dev, enq, &fakeFirmwareSchedulerPV{}, nil, activity)

		_, err := svc.EvaluateZeroTouch(context.Background(), domain.Actor{}, dev, []string{domain.ZtpTriggerEventEveryInform})
		if !errors.Is(err, domain.ErrNoMatchingRule) {
			t.Fatalf("error = %v, want ErrNoMatchingRule -- cooldown seharusnya menahan tembakan kedua", err)
		}
		if enq.rebootCalls != 0 {
			t.Fatal("EnqueueReboot TIDAK seharusnya terpanggil selagi dalam cooldown -- inilah reboot-loop yang harus dicegah")
		}
	})

	t.Run("cooldown sudah lewat -> boleh menembak lagi", func(t *testing.T) {
		activity := &fakeActivityRepoPV{
			seeded: []domain.ActivityLog{{
				EntityType: "device", EntityID: &dev.ID, Action: "ZERO_TOUCH_MATCH",
				Description: strPtr("rule_id=42"), CreatedAt: time.Now().Add(-2 * time.Hour), // 2 jam lalu, > cooldown 1 jam
			}},
		}
		enq := &fakeEnqueuerPV{hasPending: false}
		svc := newTestServiceWithActivity(rules, dev, enq, &fakeFirmwareSchedulerPV{}, nil, activity)

		matched, err := svc.EvaluateZeroTouch(context.Background(), domain.Actor{}, dev, []string{domain.ZtpTriggerEventEveryInform})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if matched == nil || enq.rebootCalls != 1 {
			t.Fatalf("cooldown sudah lewat, aksi seharusnya boleh jalan lagi, matched=%+v rebootCalls=%d", matched, enq.rebootCalls)
		}
	})
}

func atoiT(t *testing.T, s string) int {
	t.Helper()
	n := 0
	neg := false
	for i, r := range s {
		if i == 0 && r == '-' {
			neg = true
			continue
		}
		if r < '0' || r > '9' {
			t.Fatalf("bukan angka: %q", s)
		}
		n = n*10 + int(r-'0')
	}
	if neg {
		n = -n
	}
	return n
}
