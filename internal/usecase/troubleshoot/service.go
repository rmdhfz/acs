// Package troubleshoot mengumpulkan bukti sisi ACS tentang satu perangkat dan menjalankan aturan diagnosis "kenapa offline?"
// (lihat Diagnose). Hanya membaca: tidak mengubah perangkat, tidak mengantre task. Akses mengikuti usecase/device (scope tenant).
package troubleshoot

import (
	"context"
	"time"

	"acs/internal/domain"
)

// DeviceReader dipenuhi usecase/device.Service: pembacaan perangkat yang sudah menegakkan scope tenant/RBAC.
type DeviceReader interface {
	Get(ctx context.Context, actor domain.Actor, id uint64) (*domain.Device, error)
}

// ParamPrefixChecker dipenuhi domain.DeviceParameterRepository: apakah perangkat punya parameter berawalan tertentu (menentukan root TR-098/TR-181).
type ParamPrefixChecker interface {
	HasPrefix(ctx context.Context, deviceID uint64, prefix string) (bool, error)
}

const (
	eventsLookback  = 100
	opticalLookback = 10
	faultsLookback  = 20
	tasksLookback   = 50
)

type Service struct {
	devices      DeviceReader
	events       domain.DeviceEventRepository
	optical      domain.DeviceOpticalMetricRepository
	faults       domain.FaultRepository
	tasks        domain.TaskRepository
	refs         domain.RefRepository
	params       ParamPrefixChecker
	minThreshold time.Duration
	now          func() time.Time
}

func NewService(devices DeviceReader, events domain.DeviceEventRepository, optical domain.DeviceOpticalMetricRepository,
	faults domain.FaultRepository, tasks domain.TaskRepository, refs domain.RefRepository, params ParamPrefixChecker, minThreshold time.Duration) *Service {
	return &Service{devices: devices, events: events, optical: optical, faults: faults, tasks: tasks, refs: refs, params: params, minThreshold: minThreshold, now: time.Now}
}

// Diagnose mengumpulkan bukti dan menjalankan aturan. Perangkat milik tenant lain = domain.ErrNotFound/ErrForbidden dari usecase/device.
// Sumber bukti yang gagal dibaca dicatat sebagai celah (Evidence.Gaps) dan tidak menggagalkan diagnosis.
func (s *Service) Diagnose(ctx context.Context, actor domain.Actor, deviceID uint64) (*domain.DeviceDiagnosis, error) {
	dev, err := s.devices.Get(ctx, actor, deviceID)
	if err != nil {
		return nil, err
	}
	ev := Evidence{Now: s.now(), Device: *dev, MinThreshold: s.minThreshold}
	if ref, err := s.refs.GetByID(ctx, domain.RefTableDeviceStatus, dev.DeviceStatusID); err == nil {
		ev.StatusCode = ref.Code
	}

	for _, root := range []string{"InternetGatewayDevice.", "Device."} {
		if ok, err := s.params.HasPrefix(ctx, deviceID, root); err == nil && ok {
			ev.DataModelRoot = root
			break
		}
	}

	if rows, _, err := s.events.ListByDevice(ctx, deviceID, domain.Pagination{Page: 1, PageSize: eventsLookback}); err != nil {
		ev.Gaps = append(ev.Gaps, "riwayat event")
	} else {
		codes := map[uint64]string{}
		for _, r := range rows {
			code, ok := codes[r.EventCodeID]
			if !ok {
				if ref, err := s.refs.GetByID(ctx, domain.RefTableEventCodes, r.EventCodeID); err == nil {
					code = ref.Code
				}
				codes[r.EventCodeID] = code
			}
			ev.Events = append(ev.Events, EventAt{Code: code, At: r.OccurredAt})
		}
	}

	if rows, _, err := s.optical.ListByDevice(ctx, deviceID, domain.Pagination{Page: 1, PageSize: opticalLookback}); err != nil {
		ev.Gaps = append(ev.Gaps, "riwayat redaman optik")
	} else {
		for _, r := range rows {
			ev.Optical = append(ev.Optical, OpticalAt{RXDBM: r.RxPowerDBM, At: r.RecordedAt})
		}
	}

	if rows, _, err := s.faults.List(ctx, domain.FaultFilter{DeviceID: &deviceID}, domain.Pagination{Page: 1, PageSize: faultsLookback}); err != nil {
		ev.Gaps = append(ev.Gaps, "fault aktif")
	} else {
		ev.Faults = rows
	}

	if rows, _, err := s.tasks.List(ctx, domain.TaskFilter{DeviceID: &deviceID}, domain.Pagination{Page: 1, PageSize: tasksLookback}); err != nil {
		ev.Gaps = append(ev.Gaps, "antrean task")
	} else {
		statuses := map[uint64]string{}
		for _, r := range rows {
			code, ok := statuses[r.TaskStatusID]
			if !ok {
				if ref, err := s.refs.GetByID(ctx, domain.RefTableTaskStatus, r.TaskStatusID); err == nil {
					code = ref.Code
				}
				statuses[r.TaskStatusID] = code
			}
			ev.Tasks = append(ev.Tasks, TaskInfo{Status: code, CreatedAt: r.CreatedAt})
		}
	}

	d := Diagnose(ev)
	return &d, nil
}
