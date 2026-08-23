// Package domain berisi entities dan interface kontrak (repository & usecase)
// sesuai Clean Architecture yang dipakai proyek ini — lihat TECH.md §2.
package domain

import (
	"context"
	"errors"
	"time"
)

var (
	ErrNotFound         = errors.New("domain: data tidak ditemukan")
	ErrConflict          = errors.New("domain: data sudah ada / konflik unique")
	ErrInvalidInput      = errors.New("domain: input tidak valid")
	ErrUnauthorized      = errors.New("domain: tidak terautentikasi")
	ErrForbidden         = errors.New("domain: tidak punya akses")
	ErrNoMatchingRule    = errors.New("domain: tidak ada aturan zero-touch yang cocok")
	// ErrQuotaExceeded — kuota tenant sudah tercapai (mis. max_pending_tasks,
	// ROADMAP.md Fase 2). Di-map ke HTTP 429 Too Many Requests (delivery/http/middleware.go),
	// beda dari ErrForbidden (403, soal wewenang) karena ini soal batas resource, bukan izin.
	ErrQuotaExceeded     = errors.New("domain: kuota tenant sudah tercapai")
)

// Audit adalah 7 kolom audit standar (lihat CLAUDE.md - Konvensi Skema Database #2).
type Audit struct {
	CreatedAt time.Time  `db:"created_at" json:"created_at"`
	CreatedBy *uint64    `db:"created_by" json:"created_by"`
	UpdatedAt time.Time  `db:"updated_at" json:"updated_at"`
	UpdatedBy *uint64    `db:"updated_by" json:"updated_by"`
	DeletedAt *time.Time `db:"deleted_at" json:"deleted_at"`
	DeletedBy *uint64    `db:"deleted_by" json:"deleted_by"`
	IsDeleted bool       `db:"is_deleted" json:"is_deleted"`
}

// Actor merepresentasikan siapa yang melakukan aksi (dipakai untuk kolom
// created_by/updated_by dan activity_logs).
type Actor struct {
	UserID   uint64
	TenantID *uint64
	Roles    []string
}

func (a Actor) HasRole(role string) bool {
	for _, r := range a.Roles {
		if r == role {
			return true
		}
	}
	return false
}

func (a Actor) IsSuperadmin() bool {
	return a.HasRole(RoleSuperadmin)
}

// UserIDPtr mengembalikan pointer ke UserID, atau nil bila UserID kosong
// (aksi sistem seperti evaluasi ZTP otomatis saat BOOTSTRAP, bukan dipicu
// user tertentu) — penting agar kolom created_by/user_id (FK ke users) tidak
// pernah diisi 0 (yang akan melanggar foreign key / mencatat user palsu).
func (a Actor) UserIDPtr() *uint64 {
	if a.UserID == 0 {
		return nil
	}
	return &a.UserID
}

// Pagination adalah parameter halaman generik untuk endpoint listing.
type Pagination struct {
	Page     int
	PageSize int
}

func (p Pagination) Limit() int {
	if p.PageSize <= 0 || p.PageSize > 200 {
		return 50
	}
	return p.PageSize
}

func (p Pagination) Offset() int {
	if p.Page <= 1 {
		return 0
	}
	return (p.Page - 1) * p.Limit()
}

// RefLookup adalah baris generik dari salah satu tabel ref_* (lihat schema.sql §1).
type RefLookup struct {
	ID   uint64 `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
}

// Nama tabel ref_* yang di-whitelist — lihat repository/mysql/ref_repository.go.
const (
	RefTableVendors           = "ref_vendors"
	RefTableDeviceTypes       = "ref_device_types"
	RefTableDataModelVersions = "ref_data_model_versions"
	RefTableEventCodes        = "ref_event_codes"
	RefTableTaskTypes         = "ref_task_types"
	RefTableTaskStatus        = "ref_task_status"
	RefTableDeviceStatus      = "ref_device_status"
	RefTableParameterTypes    = "ref_parameter_types"
	RefTableRoles             = "ref_roles"
)

// RefRepository adalah akses generik ke tabel ref_* — menghindari 9 repository
// nyaris identik untuk tabel lookup sederhana. Nama tabel divalidasi lewat
// whitelist di implementasi, bukan diteruskan mentah ke SQL.
type RefRepository interface {
	GetByCode(ctx context.Context, table, code string) (RefLookup, error)
	GetByID(ctx context.Context, table string, id uint64) (RefLookup, error)
	List(ctx context.Context, table string) ([]RefLookup, error)
}

const (
	RoleSuperadmin = "SUPERADMIN"
	RoleAdmin      = "ADMIN"
	RoleNOC        = "NOC"
	RoleViewer     = "VIEWER"
)

const (
	DeviceStatusOnline        = "ONLINE"
	DeviceStatusOffline       = "OFFLINE"
	DeviceStatusProvisioning  = "PROVISIONING"
	DeviceStatusFaulty        = "FAULTY"
	DeviceStatusUnregistered  = "UNREGISTERED"
	DeviceStatusDecommissioned = "DECOMMISSIONED"
)

const (
	TaskStatusPending   = "PENDING"
	TaskStatusQueued    = "QUEUED"
	TaskStatusSent      = "SENT"
	TaskStatusCompleted = "COMPLETED"
	TaskStatusFailed    = "FAILED"
	TaskStatusCancelled = "CANCELLED"
	TaskStatusTimeout   = "TIMEOUT"
)

const (
	TaskTypeGetParameterValues      = "GET_PARAMETER_VALUES"
	TaskTypeSetParameterValues      = "SET_PARAMETER_VALUES"
	TaskTypeGetParameterNames       = "GET_PARAMETER_NAMES"
	TaskTypeAddObject               = "ADD_OBJECT"
	TaskTypeDeleteObject            = "DELETE_OBJECT"
	TaskTypeReboot                  = "REBOOT"
	TaskTypeFactoryReset            = "FACTORY_RESET"
	TaskTypeDownload                = "DOWNLOAD"
	TaskTypeUpload                  = "UPLOAD"
	TaskTypeScheduleInform          = "SCHEDULE_INFORM"
	TaskTypeSetParameterAttributes  = "SET_PARAMETER_ATTRIBUTES"
	TaskTypeGetParameterAttributes  = "GET_PARAMETER_ATTRIBUTES"
)

// Event code standar CWMP (Broadband Forum) — lihat CLAUDE.md, jangan diubah penulisannya.
const (
	EventCodeBootstrap                  = "0 BOOTSTRAP"
	EventCodeBoot                       = "1 BOOT"
	EventCodePeriodic                   = "2 PERIODIC"
	EventCodeScheduled                  = "3 SCHEDULED"
	EventCodeValueChange                = "4 VALUE CHANGE"
	EventCodeKicked                     = "5 KICKED"
	EventCodeConnectionRequest          = "6 CONNECTION REQUEST"
	EventCodeTransferComplete           = "7 TRANSFER COMPLETE"
	EventCodeDiagnosticsComplete        = "8 DIAGNOSTICS COMPLETE"
	EventCodeRequestDownload            = "9 REQUEST DOWNLOAD"
	EventCodeAutonomousTransferComplete = "10 AUTONOMOUS TRANSFER COMPLETE"
	EventCodeMReboot                    = "M Reboot"
	EventCodeMScheduleInform            = "M ScheduleInform"
	EventCodeMDownload                  = "M Download"
	EventCodeMUpload                    = "M Upload"
)
