package domain

import (
	"context"
	"time"
)

const (
	DiagnosticTypePing         = "PING"
	DiagnosticTypeTraceroute   = "TRACEROUTE"
	DiagnosticTypeWiFiScan     = "WIFI_SCAN"
	DiagnosticTypeOpticalPower = "OPTICAL_POWER"
	DiagnosticTypeSpeedTest    = "SPEED_TEST"

	DiagnosticStatusPending   = "PENDING"
	DiagnosticStatusRunning   = "RUNNING"
	DiagnosticStatusCompleted = "COMPLETED"
	DiagnosticStatusFailed    = "FAILED"
)

type DeviceDiagnostic struct {
	ID             uint64  `db:"id" json:"id"`
	DeviceID       uint64  `db:"device_id" json:"device_id"`
	TaskID         *uint64 `db:"task_id" json:"task_id"`
	DiagnosticType string  `db:"diagnostic_type" json:"diagnostic_type"`
	Status         string  `db:"status" json:"status"`
	// Result adalah domain.JSONRawMessage, BUKAN []byte -- isinya SELALU JSON
	// (hasil GetParameterValues yg di-json.Marshal, lihat cwmp/handler.go),
	// pola sama seperti Task.Parameters/Response (lihat komentar lengkap di
	// domain/task.go dan domain/common.go soal kenapa []byte biasa maupun
	// json.RawMessage stdlib polos salah -- base64 di response API / gagal
	// scan NULL).
	Result     JSONRawMessage `db:"result" json:"result"`
	ExecutedAt *time.Time     `db:"executed_at" json:"executed_at"`
	CreatedAt  time.Time      `db:"created_at" json:"created_at"`
}

type DeviceDiagnosticRepository interface {
	Create(ctx context.Context, d *DeviceDiagnostic) error
	GetByID(ctx context.Context, id uint64) (*DeviceDiagnostic, error)
	GetByTaskID(ctx context.Context, taskID uint64) (*DeviceDiagnostic, error)
	ListByDevice(ctx context.Context, deviceID uint64, p Pagination) ([]DeviceDiagnostic, int, error)
	UpdateResult(ctx context.Context, id uint64, status string, result JSONRawMessage, executedAt time.Time) error
}
