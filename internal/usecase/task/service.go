// Package task mengorkestrasi antrean RPC CWMP (lihat TECH.md §4) — enqueue,
// dequeue per device (dipanggil dari usecase/session), retry dengan
// max_retries, dan resolusi logical key -> raw TR-069 path (TECH.md §5).
package task

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"acs/internal/domain"
)

type Service struct {
	tasks         domain.TaskRepository
	devices       domain.DeviceRepository
	deviceModels  domain.DeviceModelRepository
	paramMappings domain.VendorParameterMappingRepository
	refs          domain.RefRepository
	activity      domain.ActivityLogRepository
}

func NewService(
	tasks domain.TaskRepository,
	devices domain.DeviceRepository,
	deviceModels domain.DeviceModelRepository,
	paramMappings domain.VendorParameterMappingRepository,
	refs domain.RefRepository,
	activity domain.ActivityLogRepository,
) *Service {
	return &Service{
		tasks: tasks, devices: devices, deviceModels: deviceModels,
		paramMappings: paramMappings, refs: refs, activity: activity,
	}
}

// ResolveParameterPath menerjemahkan logical key (mis. "wifi.5g.ssid") ke raw
// path TR-069 sesuai vendor/model device. Raw path yang sudah eksplisit
// (diawali root object TR-098/TR-181) diteruskan apa adanya (FR-12).
func (s *Service) ResolveParameterPath(ctx context.Context, deviceID uint64, key string) (string, error) {
	if looksLikeRawPath(key) {
		return key, nil
	}
	dev, err := s.devices.GetByID(ctx, deviceID)
	if err != nil {
		return "", err
	}
	if dev.VendorID == nil {
		return "", fmt.Errorf("task: device belum diketahui vendor-nya, tidak bisa resolve logical key %q", key)
	}
	if dev.DeviceModelID == nil {
		return "", fmt.Errorf("task: device belum diketahui model-nya, tidak bisa resolve logical key %q", key)
	}
	dm, err := s.deviceModels.GetByID(ctx, *dev.DeviceModelID)
	if err != nil {
		return "", err
	}
	m, err := s.paramMappings.Resolve(ctx, *dev.VendorID, dm.DataModelVersionID, dev.DeviceModelID, key)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return "", fmt.Errorf("task: tidak ada vendor_parameter_mappings untuk logical key %q pada device ini (kirim raw TR-069 path bila belum dipetakan, lihat FR-12)", key)
		}
		return "", err
	}
	return m.TR069Path, nil
}

func looksLikeRawPath(key string) bool {
	return strings.HasPrefix(key, "InternetGatewayDevice.") || strings.HasPrefix(key, "Device.")
}

func (s *Service) CreateTask(ctx context.Context, actor domain.Actor, in domain.CreateTaskInput) (*domain.Task, error) {
	dev, err := s.devices.GetByID(ctx, in.DeviceID)
	if err != nil {
		return nil, err
	}
	if !actor.IsSuperadmin() {
		if dev.TenantID == nil || actor.TenantID == nil || *dev.TenantID != *actor.TenantID {
			return nil, domain.ErrForbidden
		}
	}
	taskType, err := s.refs.GetByCode(ctx, domain.RefTableTaskTypes, in.TaskType)
	if err != nil {
		return nil, fmt.Errorf("task: tipe task tidak dikenal %q: %w", in.TaskType, err)
	}
	pendingStatus, err := s.refs.GetByCode(ctx, domain.RefTableTaskStatus, domain.TaskStatusPending)
	if err != nil {
		return nil, err
	}
	paramsJSON, err := json.Marshal(in.Parameters)
	if err != nil {
		return nil, fmt.Errorf("task: parameters tidak valid: %w", err)
	}

	maxRetries := in.MaxRetries
	if maxRetries == 0 {
		maxRetries = 3
	}
	priority := in.Priority
	if priority == 0 {
		priority = 5
	}

	t := &domain.Task{
		TaskUUID:     uuid.NewString(),
		DeviceID:     in.DeviceID,
		TaskTypeID:   taskType.ID,
		TaskStatusID: pendingStatus.ID,
		Priority:     priority,
		Parameters:   paramsJSON,
		MaxRetries:   maxRetries,
		ScheduledAt:  in.ScheduledAt,
		ExpiresAt:    in.ExpiresAt,
		Audit:        domain.Audit{CreatedBy: actor.UserIDPtr()},
	}
	if err := s.tasks.Create(ctx, t); err != nil {
		return nil, err
	}
	_ = s.activity.Record(ctx, &domain.ActivityLog{
		UserID: actor.UserIDPtr(), TenantID: actor.TenantID,
		Action: "CREATE_TASK", EntityType: "task", EntityID: &t.ID,
	})
	return t, nil
}

// EnqueueSetParameterValues mengimplementasikan domain.TaskEnqueuer — dipakai
// usecase/provisioning untuk menerapkan profile/ZTP tanpa usecase/task perlu
// bergantung balik pada usecase/provisioning (lihat domain/task.go).
func (s *Service) EnqueueSetParameterValues(ctx context.Context, actor domain.Actor, deviceID uint64, params map[string]string, priority uint8) (*domain.Task, error) {
	resolved := make(map[string]string, len(params))
	for k, v := range params {
		path, err := s.ResolveParameterPath(ctx, deviceID, k)
		if err != nil {
			return nil, err
		}
		resolved[path] = v
	}
	return s.CreateTask(ctx, actor, domain.CreateTaskInput{
		DeviceID:   deviceID,
		TaskType:   domain.TaskTypeSetParameterValues,
		Priority:   priority,
		Parameters: map[string]interface{}{"values": resolved},
	})
}

// requireTaskTenantScope memastikan actor non-superadmin hanya mengakses
// task milik device yang tenant-nya sama dengan tenant actor (RBAC scope
// tenant, CLAUDE.md) — pola sama seperti device.Service.requireTenantScope,
// diduplikasi kecil di sini karena task->tenant harus di-resolve lewat
// device pemiliknya dulu (tasks tidak punya tenant_id langsung).
func (s *Service) requireTaskTenantScope(ctx context.Context, actor domain.Actor, t *domain.Task) error {
	if actor.IsSuperadmin() {
		return nil
	}
	dev, err := s.devices.GetByID(ctx, t.DeviceID)
	if err != nil {
		return err
	}
	if dev.TenantID == nil || actor.TenantID == nil || *actor.TenantID != *dev.TenantID {
		return domain.ErrForbidden
	}
	return nil
}

func (s *Service) Get(ctx context.Context, actor domain.Actor, id uint64) (*domain.Task, error) {
	t, err := s.tasks.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.requireTaskTenantScope(ctx, actor, t); err != nil {
		return nil, err
	}
	return t, nil
}

func (s *Service) List(ctx context.Context, actor domain.Actor, f domain.TaskFilter, p domain.Pagination) ([]domain.Task, int, error) {
	if !actor.IsSuperadmin() {
		f.TenantID = actor.TenantID
	}
	return s.tasks.List(ctx, f, p)
}

// Stats — agregat untuk dashboard analitik (ROADMAP.md Fase 1), tenant-scoped
// sama seperti List/Get di atas.
func (s *Service) Stats(ctx context.Context, actor domain.Actor) ([]domain.TaskStatusCount, error) {
	var tenantID *uint64
	if !actor.IsSuperadmin() {
		tenantID = actor.TenantID
	}
	return s.tasks.CountByStatus(ctx, tenantID)
}

func (s *Service) Cancel(ctx context.Context, actor domain.Actor, id uint64) error {
	t, err := s.tasks.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if err := s.requireTaskTenantScope(ctx, actor, t); err != nil {
		return err
	}
	return s.tasks.Cancel(ctx, id, actor.UserIDPtr())
}

// ---- Dipanggil dari usecase/session selama sesi CWMP berlangsung ----

func (s *Service) NextForDevice(ctx context.Context, deviceID uint64) (*domain.Task, error) {
	return s.tasks.NextForDevice(ctx, deviceID)
}

func (s *Service) HasPendingForDevice(ctx context.Context, deviceID uint64) (bool, error) {
	return s.tasks.HasPendingForDevice(ctx, deviceID)
}

func (s *Service) GetSentForDevice(ctx context.Context, deviceID uint64) (*domain.Task, error) {
	return s.tasks.GetSentForDevice(ctx, deviceID)
}

func (s *Service) GetByUUID(ctx context.Context, uuid string) (*domain.Task, error) {
	return s.tasks.GetByUUID(ctx, uuid)
}

func (s *Service) MarkSent(ctx context.Context, taskID uint64) error {
	return s.tasks.MarkSent(ctx, taskID, time.Now())
}

func (s *Service) Complete(ctx context.Context, taskID uint64, response []byte) error {
	return s.tasks.MarkCompleted(ctx, taskID, response, time.Now())
}

// Fail menandai task gagal (dari cwmp:Fault CPE) dengan retry hingga max_retries.
func (s *Service) Fail(ctx context.Context, t *domain.Task, errMsg string) error {
	return s.failOrRetry(ctx, t, domain.TaskStatusFailed, errMsg)
}

// Timeout menandai task time-out menunggu respons CPE, dengan retry hingga max_retries.
func (s *Service) Timeout(ctx context.Context, t *domain.Task) error {
	return s.failOrRetry(ctx, t, domain.TaskStatusTimeout, "timeout menunggu respons CPE")
}

// TimeoutStaleSent men-timeout-kan task SENT yang sudah lebih lama dari
// threshold tanpa respons CPE (mis. koneksi CPE putus di tengah sesi).
// Dipanggil periodik dari goroutine di cmd/acsd — aman dijalankan dari
// instance manapun karena app server stateless (TECH.md §9).
func (s *Service) TimeoutStaleSent(ctx context.Context, threshold time.Duration) (int, error) {
	stale, err := s.tasks.ListStaleSent(ctx, time.Now().Add(-threshold))
	if err != nil {
		return 0, err
	}
	count := 0
	for i := range stale {
		if err := s.Timeout(ctx, &stale[i]); err == nil {
			count++
		}
	}
	return count, nil
}

// failOrRetry: retry_count bertambah 1; jika sudah mencapai/melewati
// max_retries task ditandai gagal permanen (finalStatusCode), selain itu
// dikembalikan ke PENDING agar dicoba lagi pada sesi berikutnya (TECH.md §4).
func (s *Service) failOrRetry(ctx context.Context, t *domain.Task, finalStatusCode, errMsg string) error {
	if err := s.tasks.IncrementRetry(ctx, t.ID); err != nil {
		return err
	}
	if err := s.tasks.SetErrorMessage(ctx, t.ID, errMsg); err != nil {
		return err
	}
	statusCode := domain.TaskStatusPending
	if t.RetryCount+1 >= t.MaxRetries {
		statusCode = finalStatusCode
	}
	status, err := s.refs.GetByCode(ctx, domain.RefTableTaskStatus, statusCode)
	if err != nil {
		return err
	}
	return s.tasks.UpdateStatus(ctx, t.ID, status.ID, nil)
}
