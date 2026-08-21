package domain

import (
	"context"
	"time"
)

// Task adalah satu item antrean RPC CWMP untuk sebuah device (lihat TECH.md §4).
type Task struct {
	ID            uint64     `db:"id" json:"id"`
	TaskUUID      string     `db:"task_uuid" json:"task_uuid"`
	DeviceID      uint64     `db:"device_id" json:"device_id"`
	TaskTypeID    uint64     `db:"task_type_id" json:"task_type_id"`
	TaskStatusID  uint64     `db:"task_status_id" json:"task_status_id"`
	Priority      uint8      `db:"priority" json:"priority"` // 1 = tertinggi, 9 = terendah (lihat schema.sql)
	Parameters    []byte     `db:"parameters" json:"parameters"`
	Response      []byte     `db:"response" json:"response"`
	ErrorMessage  *string    `db:"error_message" json:"error_message"`
	RetryCount    uint32     `db:"retry_count" json:"retry_count"`
	MaxRetries    uint32     `db:"max_retries" json:"max_retries"`
	ScheduledAt   *time.Time `db:"scheduled_at" json:"scheduled_at"`
	ExpiresAt     *time.Time `db:"expires_at" json:"expires_at"`
	SentAt        *time.Time `db:"sent_at" json:"sent_at"`
	CompletedAt   *time.Time `db:"completed_at" json:"completed_at"`
	Audit
}

type TaskFilter struct {
	DeviceID     *uint64
	TaskStatusID *uint64
	TaskTypeCode string
}

type TaskRepository interface {
	Create(ctx context.Context, t *Task) error
	GetByID(ctx context.Context, id uint64) (*Task, error)
	GetByUUID(ctx context.Context, uuid string) (*Task, error)
	// NextForDevice mengambil task PENDING milik device, terurut priority ASC
	// (1 = tertinggi dieksekusi lebih dulu), created_at ASC.
	NextForDevice(ctx context.Context, deviceID uint64) (*Task, error)
	HasPendingForDevice(ctx context.Context, deviceID uint64) (bool, error)
	// GetSentForDevice mengembalikan task SENT paling baru milik device —
	// dipakai usecase/session untuk mengorelasikan respons RPC CPE ke task
	// yang dikirim, karena dalam satu sesi hanya ada satu task in-flight
	// per device (dikirim serial, tunggu respons, baru kirim berikutnya).
	GetSentForDevice(ctx context.Context, deviceID uint64) (*Task, error)
	// ListStaleSent mengembalikan task berstatus SENT yang sudah lebih lama
	// dari staleBefore tanpa respons CPE — dipakai sweeper timeout periodik
	// (lihat cmd/acsd, pola sama dengan DeviceRepository.MarkStaleOffline).
	ListStaleSent(ctx context.Context, staleBefore time.Time) ([]Task, error)
	List(ctx context.Context, f TaskFilter, p Pagination) ([]Task, int, error)
	UpdateStatus(ctx context.Context, id, statusID uint64, updatedBy *uint64) error
	MarkSent(ctx context.Context, id uint64, sentAt time.Time) error
	MarkCompleted(ctx context.Context, id uint64, response []byte, completedAt time.Time) error
	MarkFailed(ctx context.Context, id uint64, statusID uint64, errMsg string) error
	SetErrorMessage(ctx context.Context, id uint64, errMsg string) error
	IncrementRetry(ctx context.Context, id uint64) error
	Cancel(ctx context.Context, id uint64, updatedBy *uint64) error
}

// TaskEnqueuer dipakai usecase/provisioning untuk mengantre task
// SetParameterValues tanpa bergantung langsung pada package usecase/task
// (menghindari import cycle — lihat usecase/task/service.go).
type TaskEnqueuer interface {
	EnqueueSetParameterValues(ctx context.Context, actor Actor, deviceID uint64, params map[string]string, priority uint8) (*Task, error)
}

// CreateTaskInput adalah payload umum pembuatan task, didefinisikan di domain
// (bukan di usecase/task) agar usecase lain (firmware, diagnostics) bisa
// bergantung pada TaskCreator tanpa import cycle ke usecase/task.
type CreateTaskInput struct {
	DeviceID    uint64
	TaskType    string // kode ref_task_types
	Priority    uint8  // 1 = tertinggi, default 5 bila 0
	Parameters  map[string]interface{}
	MaxRetries  uint32 // default 3 bila 0
	ScheduledAt *time.Time
	ExpiresAt   *time.Time
}

type TaskCreator interface {
	CreateTask(ctx context.Context, actor Actor, in CreateTaskInput) (*Task, error)
}
