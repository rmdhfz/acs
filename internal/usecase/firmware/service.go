// Package firmware menangani upload metadata firmware dan penjadwalan
// upgrade (TECH.md §7, FR-19/FR-20/FR-21). Upload file fisik (object storage
// vs filesystem, lihat TECH.md §12) belum diputuskan — usecase ini hanya
// mencatat metadata (path/checksum) yang sudah tersedia dari layer HTTP.
package firmware

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"acs/internal/domain"
)

type Service struct {
	files    domain.FirmwareFileRepository
	jobs     domain.FirmwareUpgradeJobRepository
	devices  domain.DeviceRepository
	tasks    domain.TaskCreator
	refs     domain.RefRepository
	activity domain.ActivityLogRepository
}

func NewService(
	files domain.FirmwareFileRepository,
	jobs domain.FirmwareUpgradeJobRepository,
	devices domain.DeviceRepository,
	tasks domain.TaskCreator,
	refs domain.RefRepository,
	activity domain.ActivityLogRepository,
) *Service {
	return &Service{files: files, jobs: jobs, devices: devices, tasks: tasks, refs: refs, activity: activity}
}

// requireDeviceTenantScope — RBAC scope tenant (CLAUDE.md), dipakai tiap kali
// usecase ini mengakses data ter-scope ke satu device.
func requireDeviceTenantScope(actor domain.Actor, deviceTenantID *uint64) error {
	if actor.IsSuperadmin() {
		return nil
	}
	if deviceTenantID == nil || actor.TenantID == nil || *actor.TenantID != *deviceTenantID {
		return domain.ErrForbidden
	}
	return nil
}

type UploadFirmwareInput struct {
	VendorID       uint64
	DeviceModelID  *uint64
	Version        string
	FileName       string
	FilePath       string
	FileSizeBytes  *uint64
	ChecksumSHA256 *string
	ReleaseNotes   *string
}

func (s *Service) UploadFirmware(ctx context.Context, actor domain.Actor, in UploadFirmwareInput) (*domain.FirmwareFile, error) {
	if in.ChecksumSHA256 == nil || *in.ChecksumSHA256 == "" {
		return nil, fmt.Errorf("firmware: checksum_sha256 wajib diisi untuk validasi integritas (FR-19)")
	}
	f := &domain.FirmwareFile{
		FirmwareUUID:   uuid.NewString(),
		VendorID:       in.VendorID,
		DeviceModelID:  in.DeviceModelID,
		Version:        in.Version,
		FileName:       in.FileName,
		FilePath:       in.FilePath,
		FileSizeBytes:  in.FileSizeBytes,
		ChecksumSHA256: in.ChecksumSHA256,
		ReleaseNotes:   in.ReleaseNotes,
		IsActive:       true,
		Audit:          domain.Audit{CreatedBy: actor.UserIDPtr()},
	}
	if err := s.files.Create(ctx, f); err != nil {
		return nil, err
	}
	_ = s.activity.Record(ctx, &domain.ActivityLog{
		UserID: actor.UserIDPtr(), TenantID: actor.TenantID, Action: "UPLOAD_FIRMWARE", EntityType: "firmware_file", EntityID: &f.ID,
	})
	return f, nil
}

func (s *Service) Get(ctx context.Context, id uint64) (*domain.FirmwareFile, error) {
	return s.files.GetByID(ctx, id)
}

func (s *Service) ListByVendor(ctx context.Context, vendorID uint64, p domain.Pagination) ([]domain.FirmwareFile, int, error) {
	return s.files.ListByVendor(ctx, vendorID, p)
}

// ScheduleUpgrade membuat firmware_upgrade_jobs + task Download (RPC CWMP
// standar FileType=1 Firmware Upgrade Image, lihat TECH.md §7).
func (s *Service) ScheduleUpgrade(ctx context.Context, actor domain.Actor, deviceID, firmwareID uint64, scheduledAt *time.Time) (*domain.FirmwareUpgradeJob, error) {
	dev, err := s.devices.GetByID(ctx, deviceID)
	if err != nil {
		return nil, err
	}
	if err := requireDeviceTenantScope(actor, dev.TenantID); err != nil {
		return nil, err
	}
	fw, err := s.files.GetByID(ctx, firmwareID)
	if err != nil {
		return nil, err
	}

	downloadURL := fmt.Sprintf("/firmware/download/%s", fw.FirmwareUUID)
	t, err := s.tasks.CreateTask(ctx, actor, domain.CreateTaskInput{
		DeviceID: deviceID,
		TaskType: domain.TaskTypeDownload,
		Priority: 3,
		Parameters: map[string]interface{}{
			"file_type":        "1 Firmware Upgrade Image",
			"url":              downloadURL,
			"checksum_sha256":  fw.ChecksumSHA256,
			"target_file_name": fw.FileName,
		},
		ScheduledAt: scheduledAt,
	})
	if err != nil {
		return nil, err
	}

	pendingStatus, err := s.refs.GetByCode(ctx, domain.RefTableTaskStatus, domain.TaskStatusPending)
	if err != nil {
		return nil, err
	}

	job := &domain.FirmwareUpgradeJob{
		JobUUID:      uuid.NewString(),
		DeviceID:     deviceID,
		FirmwareID:   firmwareID,
		TaskID:       &t.ID,
		TaskStatusID: pendingStatus.ID,
		FromVersion:  dev.SoftwareVersion,
		ToVersion:    &fw.Version,
		ScheduledAt:  scheduledAt,
		CreatedBy:    actor.UserIDPtr(),
	}
	if err := s.jobs.Create(ctx, job); err != nil {
		return nil, err
	}

	_ = s.activity.Record(ctx, &domain.ActivityLog{
		UserID: actor.UserIDPtr(), TenantID: actor.TenantID,
		Action: "SCHEDULE_FIRMWARE_UPGRADE", EntityType: "firmware_upgrade_job", EntityID: &job.ID,
	})
	return job, nil
}

func (s *Service) ListJobsByDevice(ctx context.Context, actor domain.Actor, deviceID uint64, p domain.Pagination) ([]domain.FirmwareUpgradeJob, int, error) {
	dev, err := s.devices.GetByID(ctx, deviceID)
	if err != nil {
		return nil, 0, err
	}
	if err := requireDeviceTenantScope(actor, dev.TenantID); err != nil {
		return nil, 0, err
	}
	return s.jobs.ListByDevice(ctx, deviceID, p)
}

// HandleTransferComplete di-panggil usecase/session saat CPE mengirim RPC
// TransferComplete (event 7) berkorelasi dengan task Download milik job ini
// (TECH.md §7) — meng-update status job & software_version device bila sukses.
func (s *Service) HandleTransferComplete(ctx context.Context, taskID uint64, success bool, errMsg string) error {
	job, err := s.jobs.GetByTaskID(ctx, taskID)
	if err != nil {
		return err
	}
	if success {
		completedStatus, err := s.refs.GetByCode(ctx, domain.RefTableTaskStatus, domain.TaskStatusCompleted)
		if err != nil {
			return err
		}
		now := time.Now()
		toVersion := ""
		if job.ToVersion != nil {
			toVersion = *job.ToVersion
		}
		if err := s.jobs.MarkCompleted(ctx, job.ID, toVersion, now); err != nil {
			return err
		}
		if err := s.jobs.UpdateStatus(ctx, job.ID, completedStatus.ID, nil); err != nil {
			return err
		}
		if dev, err := s.devices.GetByID(ctx, job.DeviceID); err == nil {
			dev.SoftwareVersion = job.ToVersion
			_ = s.devices.Update(ctx, dev)
		}
		return nil
	}
	failedStatus, err := s.refs.GetByCode(ctx, domain.RefTableTaskStatus, domain.TaskStatusFailed)
	if err != nil {
		return err
	}
	return s.jobs.UpdateStatus(ctx, job.ID, failedStatus.ID, &errMsg)
}
