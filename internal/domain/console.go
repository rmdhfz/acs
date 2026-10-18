package domain

import (
	"context"
	"time"
)

// ---- Fitur Console: layout menu, laporan bug, riwayat impor/ekspor (migrations/0033) ----

// Kode ref_bug_report_status.
const (
	BugStatusOpen       = "OPEN"
	BugStatusInProgress = "IN_PROGRESS"
	BugStatusResolved   = "RESOLVED"
	BugStatusWontFix    = "WONTFIX"
)

// Kode ref_bug_report_severity.
const (
	BugSeverityMinor   = "MINOR"
	BugSeverityMajor   = "MAJOR"
	BugSeverityBlocker = "BLOCKER"
)

// Kode ref_data_transfer_kind / ref_data_transfer_status.
const (
	TransferKindImport = "IMPORT"
	TransferKindExport = "EXPORT"

	TransferStatusCompleted = "COMPLETED"
	TransferStatusPartial   = "PARTIAL"
	TransferStatusFailed    = "FAILED"
)

// Sumber pencatatan transfer: SERVER = dicatat server saat memproses sendiri; CLIENT = dilaporkan Console.
const (
	TransferSourceServer = "SERVER"
	TransferSourceClient = "CLIENT"
)

// Jenis notifikasi (ref_notification_types): laporan baru untuk operator platform, dan perubahan status untuk pelapor.
const (
	NotifBugReport       = "BUG_REPORT"
	NotifBugReportUpdate = "BUG_REPORT_UPDATE"
)

// MenuLayoutRepository menyimpan susunan menu sidebar PER PENGGUNA. Semua metode dikunci ke userID pemilik: tidak ada
// jalur baca/tulis layout milik pengguna lain.
type MenuLayoutRepository interface {
	// Get mengembalikan layout JSON, atau ErrNotFound bila belum pernah disimpan / sudah di-reset.
	Get(ctx context.Context, userID uint64) (string, error)
	Upsert(ctx context.Context, userID uint64, layoutJSON string) error
	// Reset membuang layout (soft delete). Idempoten.
	Reset(ctx context.Context, userID uint64) error
}

// BugReport = laporan bug. PK numerik tidak pernah keluar API; "id" publik = report_uuid.
type BugReport struct {
	ID            uint64     `db:"id" json:"-"`
	UUID          string     `db:"report_uuid" json:"id"`
	TenantID      *uint64    `db:"tenant_id" json:"tenant_id"`
	TenantName    *string    `db:"tenant_name" json:"tenant_name"`
	ReporterID    uint64     `db:"reporter_id" json:"-"`
	ReporterName  string     `db:"reporter_name" json:"reporter"`
	ReporterEmail string     `db:"reporter_email" json:"reporter_email,omitempty"` // hanya diisi untuk SUPERADMIN (usecase)
	Impersonated  bool       `db:"impersonated" json:"impersonated"`
	Title         string     `db:"title" json:"title"`
	Description   string     `db:"description" json:"description"` // apa yang terjadi
	Expected      string     `db:"expected" json:"expected"`       // apa yang diharapkan
	HasScreenshot bool       `db:"has_screenshot" json:"has_screenshot"`
	PagePath      string     `db:"page_path" json:"page_path"`
	UserAgent     string     `db:"user_agent" json:"user_agent"`
	AppVersion    string     `db:"app_version" json:"app_version"`
	Severity      string     `db:"severity" json:"severity"`
	Status        string     `db:"status" json:"status"`
	AdminNote     *string    `db:"admin_note" json:"admin_note"`
	EmailedAt     *time.Time `db:"emailed_at" json:"emailed_at"`
	CreatedAt     time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt     time.Time  `db:"updated_at" json:"updated_at"`
}

// BugScreenshot = satu tangkapan layar (PNG/JPEG) yang sudah divalidasi usecase.
type BugScreenshot struct {
	ContentType string // image/png | image/jpeg
	Width       int
	Height      int
	SHA256      string
	Data        []byte
}

// BugReportCreate = masukan pembuatan baris (kode severity/status di-resolve repository ke id).
type BugReportCreate struct {
	UUID           string
	TenantID       *uint64
	ReporterID     uint64
	ImpersonatorID *uint64
	Title          string
	Description    string
	Expected       string
	Screenshot     *BugScreenshot
	PagePath       string
	UserAgent      string
	AppVersion     string
	SeverityCode   string
}

type BugReportFilter struct {
	ReporterID *uint64 // non-nil = hanya laporan pelapor ini
	TenantID   *uint64 // non-nil = hanya tenant ini (SUPERADMIN)
	Status     string  // kode; kosong = semua
}

type BugReportRepository interface {
	Create(ctx context.Context, in BugReportCreate) (*BugReport, error)
	GetByUUID(ctx context.Context, uuid string) (*BugReport, error)
	List(ctx context.Context, f BugReportFilter, p Pagination) ([]BugReport, int, error)
	// UpdateStatus mengubah status + catatan; ErrNotFound bila uuid tidak ada.
	UpdateStatus(ctx context.Context, uuid, statusCode string, note *string, by uint64) error
	// GetScreenshot mengembalikan tangkapan layar laporan, atau ErrNotFound bila tidak ada.
	GetScreenshot(ctx context.Context, reportID uint64) (*BugScreenshot, error)
	MarkEmailed(ctx context.Context, id uint64) error
	CountByReporterSince(ctx context.Context, reporterID uint64, since time.Time) (int, error)
}

// DataTransferLog = satu proses impor/ekspor. "id" publik = transfer_uuid.
type DataTransferLog struct {
	ID           uint64     `db:"id" json:"-"`
	UUID         string     `db:"transfer_uuid" json:"id"`
	TenantID     *uint64    `db:"tenant_id" json:"tenant_id"`
	TenantName   *string    `db:"tenant_name" json:"tenant_name"`
	UserID       uint64     `db:"user_id" json:"-"`
	UserName     string     `db:"user_name" json:"user"`
	Impersonated bool       `db:"impersonated" json:"impersonated"`
	Kind         string     `db:"kind" json:"kind"`
	Status       string     `db:"status" json:"status"`
	Module       string     `db:"module" json:"module"`
	Format       string     `db:"format" json:"format"`
	FileName     *string    `db:"file_name" json:"file_name"`
	TotalRows    uint32     `db:"total_rows" json:"total_rows"`
	SuccessRows  uint32     `db:"success_rows" json:"success_rows"`
	FailedRows   uint32     `db:"failed_rows" json:"failed_rows"`
	Detail       *string    `db:"detail" json:"detail"`
	Source       string     `db:"source" json:"source"`
	EmailedAt    *time.Time `db:"emailed_at" json:"emailed_at"`
	CreatedAt    time.Time  `db:"created_at" json:"created_at"`
}

type DataTransferCreate struct {
	UUID           string
	TenantID       *uint64
	UserID         uint64
	ImpersonatorID *uint64
	KindCode       string
	StatusCode     string
	Module         string
	Format         string
	FileName       *string
	TotalRows      uint32
	SuccessRows    uint32
	FailedRows     uint32
	Detail         *string
	Source         string
}

type DataTransferFilter struct {
	TenantID *uint64 // WAJIB diisi untuk non-SUPERADMIN (usecase); nil = semua tenant (hanya SUPERADMIN)
	Kind     string  // kode; kosong = semua
	Module   string  // kosong = semua
}

type DataTransferRepository interface {
	Create(ctx context.Context, in DataTransferCreate) (*DataTransferLog, error)
	GetByUUID(ctx context.Context, uuid string) (*DataTransferLog, error)
	List(ctx context.Context, f DataTransferFilter, p Pagination) ([]DataTransferLog, int, error)
	MarkEmailed(ctx context.Context, id uint64) error
	CountEmailedByUserSince(ctx context.Context, userID uint64, since time.Time) (int, error)
}
