package firmware

import (
	"context"
	"errors"
	"testing"

	"acs/internal/domain"
)

type recNotifierFW struct {
	evs []domain.NotificationEvent
	err error
}

func (r *recNotifierFW) Notify(_ context.Context, ev domain.NotificationEvent) error {
	r.evs = append(r.evs, ev)
	return r.err
}

// Rollout dijeda karena ambang gagal terlampaui -> ROLLOUT_PAUSED ke tenant batch; alur utama tidak terpengaruh
// walau notifier gagal.
func TestAdvanceRollout_FailureRate_MemberiTahu(t *testing.T) {
	tenantID := uint64(4)
	svc, rollouts, jobs, devices := newRolloutTestService()
	rec := &recNotifierFW{err: errors.New("notifikasi rusak")}
	svc.WithNotifier(rec)
	rollouts.batch = &domain.FirmwareRolloutBatch{ID: 9, TenantID: &tenantID, FirmwareFileID: 1, WavePercentage: 50, MaxFailureRatePercent: 30, CurrentWave: 1, StatusID: rtStatusInProg}
	wave := uint32(1)
	jobs.jobs = []domain.FirmwareUpgradeJob{
		{ID: 1, DeviceID: 10, RolloutBatchID: uint64Ptr(9), WaveNumber: &wave, TaskStatusID: taskStatusFailed},
		{ID: 2, DeviceID: 11, RolloutBatchID: uint64Ptr(9), WaveNumber: &wave, TaskStatusID: taskStatusCompleted},
	}
	devices.population = []domain.Device{{ID: 10}, {ID: 11}}

	got, err := svc.AdvanceRollout(context.Background(), 9)
	if err != nil || got.StatusID != rtStatusPaused {
		t.Fatalf("rollout harus tetap dijeda walau notifier gagal: %v status=%d", err, got.StatusID)
	}
	if len(rec.evs) != 1 {
		t.Fatalf("harus 1 notifikasi, dapat %d", len(rec.evs))
	}
	ev := rec.evs[0]
	if ev.TypeCode != domain.NotifRolloutPaused || ev.TenantID == nil || *ev.TenantID != 4 || ev.Link != "/firmware" ||
		ev.EntityType != "firmware_rollout_batch" || ev.EntityID == nil || *ev.EntityID != 9 {
		t.Fatalf("event salah: %+v", ev)
	}
}

// Wave yang masih berjalan / lanjut normal tidak menotifikasi; 0 device terjadwal (jeda sistemik) menotifikasi.
func TestAdvanceRollout_TanpaJeda_TidakMenotifikasi(t *testing.T) {
	svc, rollouts, jobs, devices := newRolloutTestService()
	rec := &recNotifierFW{}
	svc.WithNotifier(rec)
	rollouts.batch = &domain.FirmwareRolloutBatch{ID: 1, FirmwareFileID: 1, WavePercentage: 50, MaxFailureRatePercent: 30, CurrentWave: 1, StatusID: rtStatusInProg}
	wave := uint32(1)
	jobs.jobs = []domain.FirmwareUpgradeJob{{ID: 1, DeviceID: 10, RolloutBatchID: uint64Ptr(1), WaveNumber: &wave, TaskStatusID: taskStatusSent}}
	devices.population = []domain.Device{{ID: 10}, {ID: 11}}
	if _, err := svc.AdvanceRollout(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	if len(rec.evs) != 0 {
		t.Fatal("wave berjalan tidak boleh menotifikasi")
	}
}

func TestAdvanceRollout_NolTerjadwal_MemberiTahu(t *testing.T) {
	tenantID := uint64(1)
	svc, rollouts, _, devices := newRolloutTestService()
	rec := &recNotifierFW{}
	svc.WithNotifier(rec)
	rollouts.batch = &domain.FirmwareRolloutBatch{ID: 1, TenantID: &tenantID, FirmwareFileID: 1, WavePercentage: 100, MaxFailureRatePercent: 30, StatusID: rtStatusPending}
	other := uint64(2)
	devices.population = []domain.Device{{ID: 10, TenantID: &other}}
	devices.dev = &domain.Device{VendorID: uint64Ptr(1), ID: 10, TenantID: &other}
	if _, err := svc.AdvanceRollout(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	if len(rec.evs) != 1 || rec.evs[0].TypeCode != domain.NotifRolloutPaused {
		t.Fatalf("jeda karena 0 terjadwal harus menotifikasi: %+v", rec.evs)
	}
}
