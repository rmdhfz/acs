package domain

import (
	"context"
	"time"
)

// ---- Faults: cwmp:Fault dari CPE (migrations/0038) ----

// Batas teks fault yang disimpan (FaultString dari CPE tak tepercaya panjangnya).
const (
	FaultStringMaxLen = 1024
	FaultCodeMaxLen   = 16
	// FaultMaxActivePerDevice membatasi baris fault aktif per perangkat: fault_code berasal dari CPE, jadi tanpa batas CPE yang mengirim kode acak
	// dapat menumbuhkan tabel tanpa henti (audit keamanan 22 Okt 2026).
	FaultMaxActivePerDevice = 50
)

// DeviceFault = satu masalah AKTIF: CPE menolak RPC tertentu dengan kode fault tertentu. Kejadian berulang menaikkan
// OccurrenceCount. ID numerik tidak pernah diekspos (UUID publik); DeviceTenantID dari join devices (scope tenant).
type DeviceFault struct {
	ID              uint64    `db:"id" json:"-"`
	UUID            string    `db:"fault_uuid" json:"id"`
	DeviceID        uint64    `db:"device_id" json:"device_id"`
	DeviceSerial    string    `db:"device_serial" json:"device_serial"`
	DeviceTenantID  *uint64   `db:"device_tenant_id" json:"tenant_id"`
	TaskID          *uint64   `db:"task_id" json:"task_id"`
	RPCMethod       string    `db:"rpc_method" json:"rpc_method"`
	FaultCode       string    `db:"fault_code" json:"fault_code"`
	FaultString     string    `db:"fault_string" json:"fault_string"`
	OccurrenceCount uint32    `db:"occurrence_count" json:"occurrence_count"`
	FirstSeenAt     time.Time `db:"first_seen_at" json:"first_seen_at"`
	LastSeenAt      time.Time `db:"last_seen_at" json:"last_seen_at"`
}

// FaultFilter — TenantID wajib diisi usecase untuk non-superadmin (scope tenant lewat devices.tenant_id).
type FaultFilter struct {
	TenantID  *uint64
	DeviceID  *uint64
	FaultCode string
	RPCMethod string
	// Search mencocokkan serial perangkat atau teks fault (LIKE).
	Search string
}

// FaultRepository — hanya query, tanpa keputusan (dedup/resolusi diputuskan usecase/fault).
type FaultRepository interface {
	// Upsert mencatat satu kejadian: bila sudah ada fault aktif (device, rpc_method, fault_code) -> occurrence_count+1,
	// last_seen_at/fault_string/task_id diperbarui; selain itu baris baru.
	Upsert(ctx context.Context, f *DeviceFault) error
	// ResolveActive menutup (soft delete) fault aktif perangkat untuk rpc_method itu; mengembalikan jumlah baris.
	ResolveActive(ctx context.Context, deviceID uint64, rpcMethod string) (int64, error)
	List(ctx context.Context, f FaultFilter, p Pagination) ([]DeviceFault, int, error)
	GetByUUID(ctx context.Context, uuid string) (*DeviceFault, error)
	// Dismiss menutup satu fault (soft delete) oleh operator.
	Dismiss(ctx context.Context, id, by uint64) error
}
