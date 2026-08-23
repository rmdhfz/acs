// Package diagnostics menangani trigger diagnostic test TR-069 standar
// (ping, traceroute, WiFi scan) dan penyimpanan hasilnya (FR-24).
package diagnostics

import (
	"context"
	"time"

	"acs/internal/domain"
)

type Service struct {
	diagnostics domain.DeviceDiagnosticRepository
	devices     domain.DeviceRepository
	tasks       domain.TaskCreator
	activity    domain.ActivityLogRepository
}

func NewService(
	diagnostics domain.DeviceDiagnosticRepository,
	devices domain.DeviceRepository,
	tasks domain.TaskCreator,
	activity domain.ActivityLogRepository,
) *Service {
	return &Service{diagnostics: diagnostics, devices: devices, tasks: tasks, activity: activity}
}

// Trigger mengantre task SetParameterValues yang menyalakan object diagnostic
// TR-069 terkait (path-nya berasal dari logical key/raw path yang dikirim
// caller di params, diresolve oleh usecase/task) lalu mencatat DeviceDiagnostic
// berstatus PENDING. Hasil final diambil terpisah lewat GetParameterValues
// setelah CPE mengirim event 8 DIAGNOSTICS COMPLETE (lihat HandleResult).
func (s *Service) Trigger(ctx context.Context, actor domain.Actor, deviceID uint64, diagnosticType string, params map[string]string) (*domain.DeviceDiagnostic, error) {
	dev, err := s.devices.GetByID(ctx, deviceID)
	if err != nil {
		return nil, err
	}
	if !actor.IsSuperadmin() {
		if dev.TenantID == nil || actor.TenantID == nil || *dev.TenantID != *actor.TenantID {
			return nil, domain.ErrForbidden
		}
	}

	t, err := s.tasks.CreateTask(ctx, actor, domain.CreateTaskInput{
		DeviceID:   deviceID,
		TaskType:   domain.TaskTypeSetParameterValues,
		Priority:   4,
		Parameters: map[string]interface{}{"values": params},
	})
	if err != nil {
		return nil, err
	}

	d := &domain.DeviceDiagnostic{
		DeviceID:       deviceID,
		TaskID:         &t.ID,
		DiagnosticType: diagnosticType,
		Status:         domain.DiagnosticStatusPending,
	}
	if err := s.diagnostics.Create(ctx, d); err != nil {
		return nil, err
	}

	_ = s.activity.Record(ctx, &domain.ActivityLog{
		UserID: actor.UserIDPtr(), TenantID: actor.TenantID,
		Action: "TRIGGER_DIAGNOSTIC", EntityType: "device_diagnostic", EntityID: &d.ID,
	})
	return d, nil
}

// HandleResult di-panggil usecase/session saat task GetParameterValues yang
// mengambil hasil diagnostic (berkorelasi via TaskID) selesai atau gagal.
func (s *Service) HandleResult(ctx context.Context, taskID uint64, result domain.JSONRawMessage, success bool) error {
	d, err := s.diagnostics.GetByTaskID(ctx, taskID)
	if err != nil {
		return err
	}
	status := domain.DiagnosticStatusCompleted
	if !success {
		status = domain.DiagnosticStatusFailed
	}
	return s.diagnostics.UpdateResult(ctx, d.ID, status, result, time.Now())
}

func (s *Service) Get(ctx context.Context, actor domain.Actor, id uint64) (*domain.DeviceDiagnostic, error) {
	d, err := s.diagnostics.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	dev, err := s.devices.GetByID(ctx, d.DeviceID)
	if err != nil {
		return nil, err
	}
	if !actor.IsSuperadmin() {
		if dev.TenantID == nil || actor.TenantID == nil || *dev.TenantID != *actor.TenantID {
			return nil, domain.ErrForbidden
		}
	}
	return d, nil
}

func (s *Service) ListByDevice(ctx context.Context, actor domain.Actor, deviceID uint64, p domain.Pagination) ([]domain.DeviceDiagnostic, int, error) {
	dev, err := s.devices.GetByID(ctx, deviceID)
	if err != nil {
		return nil, 0, err
	}
	if !actor.IsSuperadmin() {
		if dev.TenantID == nil || actor.TenantID == nil || *dev.TenantID != *actor.TenantID {
			return nil, 0, domain.ErrForbidden
		}
	}
	return s.diagnostics.ListByDevice(ctx, deviceID, p)
}
