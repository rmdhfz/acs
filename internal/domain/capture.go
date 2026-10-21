package domain

import (
	"context"
	"time"
)

// ---- Capture sesi CWMP per perangkat (migrations/0037) ----

// Batas capture (satu sumber untuk usecase, repository, dan UI).
const (
	CaptureMaxMinutes    = 60         // lama maksimum satu kali menyalakan capture
	CaptureMaxBodyBytes  = 128 * 1024 // pesan lebih besar dipotong (body_truncated)
	CaptureKeepPerDevice = 200        // pesan terbaru yang disimpan per perangkat
	CaptureMaxAge        = 7 * 24 * time.Hour
)

// CaptureStatus = keadaan saklar capture satu perangkat.
type CaptureStatus struct {
	Active bool       `json:"active"`
	Until  *time.Time `json:"until"`
}

// SessionCapture = satu pesan SOAP hasil capture. ID numerik tidak pernah diekspos (UUID publik).
type SessionCapture struct {
	ID            uint64    `db:"id" json:"-"`
	UUID          string    `db:"capture_uuid" json:"id"`
	DeviceID      uint64    `db:"device_id" json:"-"`
	SessionRef    *string   `db:"session_ref" json:"session_ref"`
	FromCPE       bool      `db:"from_cpe" json:"from_cpe"`
	RPCName       string    `db:"rpc_name" json:"rpc_name"`
	HTTPStatus    *uint16   `db:"http_status" json:"http_status"`
	Body          string    `db:"body" json:"body,omitempty"`
	BodyBytes     uint32    `db:"body_bytes" json:"body_bytes"`
	BodyTruncated bool      `db:"body_truncated" json:"body_truncated"`
	CreatedAt     time.Time `db:"created_at" json:"created_at"`
}

// CaptureRepository — hanya query, tanpa keputusan (batas & redaksi ada di usecase/capture).
type CaptureRepository interface {
	// EnableUntil menyalakan/memperpanjang saklar (upsert per perangkat).
	EnableUntil(ctx context.Context, deviceID uint64, until time.Time, by *uint64) error
	// Disable mematikan saklar seketika (expires_at = now); tidak ada baris = tidak apa-apa.
	Disable(ctx context.Context, deviceID uint64, now time.Time) error
	Status(ctx context.Context, deviceID uint64, now time.Time) (*CaptureStatus, error)
	// ActiveDeviceIDs: perangkat yang capture-nya masih berlaku pada `now`.
	ActiveDeviceIDs(ctx context.Context, now time.Time) ([]uint64, error)
	// Insert menyimpan pesan (mengisi ID/UUID/CreatedAt) lalu memangkas pesan lama perangkat itu: hanya `keep` terbaru
	// yang tersisa dan yang lebih tua dari `maxAge` dibuang.
	Insert(ctx context.Context, c *SessionCapture, keep int, maxAge time.Duration) error
	// List: tanpa body (ringan), terbaru dulu.
	List(ctx context.Context, deviceID uint64, p Pagination) ([]SessionCapture, int, error)
	// GetByUUID: dengan body; ErrNotFound bila bukan milik perangkat itu.
	GetByUUID(ctx context.Context, deviceID uint64, uuid string) (*SessionCapture, error)
	// ListAll: semua pesan perangkat dengan body, urut waktu naik (untuk ekspor), maks `limit`.
	ListAll(ctx context.Context, deviceID uint64, limit int) ([]SessionCapture, error)
}
