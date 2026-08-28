package domain

import (
	"context"
	"time"
)

type FileType string

const (
	FileTypeFirmware       FileType = "1 Firmware Upgrade Image"
	FileTypeWebContent     FileType = "2 Web Content"
	FileTypeVendorConfig   FileType = "3 Vendor Configuration File"
	FileTypeTone           FileType = "4 Tone File"
	FileTypeRinger         FileType = "5 Ringer Melody File"
	FileTypeVendorLog      FileType = "Vendor Log File" // TR-069 Upload
)

// File merepresentasikan berkas generik (Konfigurasi, Log, Firmware dll)
// yang bisa diunduh atau diunggah oleh CPE.
type File struct {
	ID            uint64   `db:"id" json:"id"`
	FileUUID      string   `db:"file_uuid" json:"file_uuid"`
	TenantID      *uint64  `db:"tenant_id" json:"tenant_id"`
	FileType      FileType `db:"file_type" json:"file_type"`
	VendorID      *uint64  `db:"vendor_id" json:"vendor_id"`
	DeviceModelID *uint64  `db:"device_model_id" json:"device_model_id"`
	Version       *string  `db:"version" json:"version"` // Opsional
	FileName      string   `db:"file_name" json:"file_name"`
	StorageKey    string   `db:"storage_key" json:"storage_key"`
	FileSizeBytes uint64   `db:"file_size_bytes" json:"file_size_bytes"`
	Audit
}

type FileRepository interface {
	Create(ctx context.Context, f *File) error
	GetByID(ctx context.Context, id uint64) (*File, error)
	GetByUUID(ctx context.Context, uuid string) (*File, error)
	List(ctx context.Context, tenantID *uint64, p Pagination) ([]File, int, error)
	Delete(ctx context.Context, id uint64) error
}
