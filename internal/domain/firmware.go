package domain

import (
	"context"
	"time"
)

type FirmwareFile struct {
	ID              uint64  `db:"id" json:"id"`
	FirmwareUUID    string  `db:"firmware_uuid" json:"firmware_uuid"`
	VendorID        uint64  `db:"vendor_id" json:"vendor_id"`
	DeviceModelID   *uint64 `db:"device_model_id" json:"device_model_id"`
	Version         string  `db:"version" json:"version"`
	FileName        string  `db:"file_name" json:"file_name"`
	FilePath        string  `db:"file_path" json:"file_path"`
	FileSizeBytes   *uint64 `db:"file_size_bytes" json:"file_size_bytes"`
	ChecksumSHA256  *string `db:"checksum_sha256" json:"checksum_sha256"`
	ReleaseNotes    *string `db:"release_notes" json:"release_notes"`
	IsActive        bool    `db:"is_active" json:"is_active"`
	Audit
}

type FirmwareFileRepository interface {
	Create(ctx context.Context, f *FirmwareFile) error
	GetByID(ctx context.Context, id uint64) (*FirmwareFile, error)
	GetByUUID(ctx context.Context, uuid string) (*FirmwareFile, error)
	ListByVendor(ctx context.Context, vendorID uint64, p Pagination) ([]FirmwareFile, int, error)
	SoftDelete(ctx context.Context, id, deletedBy uint64) error
}

// FirmwareUpgradeJob menghubungkan firmware ke device dan task Download-nya
// (lihat TECH.md §7).
type FirmwareUpgradeJob struct {
	ID           uint64     `db:"id" json:"id"`
	JobUUID      string     `db:"job_uuid" json:"job_uuid"`
	DeviceID     uint64     `db:"device_id" json:"device_id"`
	FirmwareID   uint64     `db:"firmware_id" json:"firmware_id"`
	TaskID       *uint64    `db:"task_id" json:"task_id"`
	TaskStatusID uint64     `db:"task_status_id" json:"task_status_id"`
	FromVersion  *string    `db:"from_version" json:"from_version"`
	ToVersion    *string    `db:"to_version" json:"to_version"`
	ScheduledAt  *time.Time `db:"scheduled_at" json:"scheduled_at"`
	StartedAt    *time.Time `db:"started_at" json:"started_at"`
	CompletedAt  *time.Time `db:"completed_at" json:"completed_at"`
	ErrorMessage *string    `db:"error_message" json:"error_message"`
	CreatedAt    time.Time  `db:"created_at" json:"created_at"`
	CreatedBy    *uint64    `db:"created_by" json:"created_by"`
	UpdatedAt    time.Time  `db:"updated_at" json:"updated_at"`
	UpdatedBy    *uint64    `db:"updated_by" json:"updated_by"`
}

type FirmwareUpgradeJobRepository interface {
	Create(ctx context.Context, j *FirmwareUpgradeJob) error
	GetByID(ctx context.Context, id uint64) (*FirmwareUpgradeJob, error)
	GetByTaskID(ctx context.Context, taskID uint64) (*FirmwareUpgradeJob, error)
	ListByDevice(ctx context.Context, deviceID uint64, p Pagination) ([]FirmwareUpgradeJob, int, error)
	UpdateStatus(ctx context.Context, id, taskStatusID uint64, errMsg *string) error
	MarkStarted(ctx context.Context, id uint64, startedAt time.Time) error
	MarkCompleted(ctx context.Context, id uint64, toVersion string, completedAt time.Time) error
}
