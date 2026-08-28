// Package domain berisi entities dan interface kontrak (repository & usecase)
// sesuai Clean Architecture yang dipakai proyek ini — lihat TECH.md §2.
package domain

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

var (
	ErrNotFound       = errors.New("domain: data tidak ditemukan")
	ErrConflict       = errors.New("domain: data sudah ada / konflik unique")
	ErrInvalidInput   = errors.New("domain: input tidak valid")
	ErrUnauthorized   = errors.New("domain: tidak terautentikasi")
	ErrForbidden      = errors.New("domain: tidak punya akses")
	ErrNoMatchingRule = errors.New("domain: tidak ada aturan zero-touch yang cocok")
	// ErrQuotaExceeded — kuota tenant sudah tercapai (mis. max_pending_tasks,
	// ROADMAP.md Fase 2). Di-map ke HTTP 429 Too Many Requests (delivery/http/middleware.go),
	// beda dari ErrForbidden (403, soal wewenang) karena ini soal batas resource, bukan izin.
	ErrQuotaExceeded = errors.New("domain: kuota tenant sudah tercapai")
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

// JSONRawMessage sama seperti encoding/json.RawMessage (JSON mentah, di-emit
// verbatim tanpa base64 -- lihat komentar Task.Parameters/Response di
// domain/task.go) TAPI juga mengimplementasikan sql.Scanner/driver.Valuer.
// json.RawMessage bawaan stdlib tidak mengimplementasikan sql.Scanner,
// sehingga sqlx.GetContext/SelectContext gagal dgn "unsupported Scan, storing
// driver.Value type <nil>" saat kolom JSON nullable (tasks.parameters/response,
// device_diagnostics.result) bernilai NULL -- ditemukan saat validasi live
// docker-compose end-to-end sesudah migrasi field-field tsb dari []byte ke
// json.RawMessage biasa.
type JSONRawMessage json.RawMessage

func (m JSONRawMessage) MarshalJSON() ([]byte, error) {
	if len(m) == 0 {
		return []byte("null"), nil
	}
	return []byte(m), nil
}

func (m *JSONRawMessage) UnmarshalJSON(data []byte) error {
	if m == nil {
		return errors.New("domain.JSONRawMessage: UnmarshalJSON on nil pointer")
	}
	*m = append((*m)[0:0], data...)
	return nil
}

func (m *JSONRawMessage) Scan(src interface{}) error {
	if src == nil {
		*m = nil
		return nil
	}
	switch v := src.(type) {
	case []byte:
		*m = append(JSONRawMessage(nil), v...)
	case string:
		*m = JSONRawMessage(v)
	default:
		return fmt.Errorf("domain.JSONRawMessage: tipe Scan tidak didukung %T", src)
	}
	return nil
}

func (m JSONRawMessage) Value() (driver.Value, error) {
	if len(m) == 0 {
		return nil, nil
	}
	return []byte(m), nil
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
	// RefTableZtpTriggerEvent — kapan zero_touch_rules dievaluasi relatif thd
	// event CWMP Inform (migrations/0009).
	RefTableZtpTriggerEvent = "ref_ztp_trigger_event"
	// RefTableFirmwareRolloutStatus — status lifecycle firmware_rollout_batches
	// (migrations/0011).
	RefTableFirmwareRolloutStatus = "ref_firmware_rollout_status"
	// RefTableWebhookEventTypes — jenis event webhook (migrations/0013).
	RefTableWebhookEventTypes = "ref_webhook_event_types"
)

// Kode ref_webhook_event_types (migrations/0013) — jenis event yang dapat
// dikirim ACS sebagai webhook keluar. Hanya kode yang benar-benar sudah
// di-wire di usecase yang ada di sini.
const (
	WebhookEventDeviceFault          = "DEVICE_FAULT"
	WebhookEventParameterValueChange = "PARAMETER_VALUE_CHANGE"
	WebhookEventTaskFailed           = "TASK_FAILED"
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

// MinUserPasswordLen — panjang minimum password akun user aplikasi (login),
// dipakai admin-reset (usecase/iam.ResetUserPassword) DAN ganti password
// sendiri (usecase/auth.ChangeOwnPassword). SATU sumber kebenaran di domain
// (bukan konstanta terpisah di masing-masing package usecase) supaya
// kebijakan tidak bisa diam-diam drift antara dua jalur ubah-password yang
// SEHARUSNYA menegakkan aturan yang sama persis (temuan acs-code-reviewer —
// sebelumnya ada 2 konstanta terpisah bernilai sama, tidak dijaga compiler,
// cuma komentar). BEDA dari shared secret Inform CWMP (lihat
// vendor_parameter_mappings/tenants.cwmp_inform_password_enc) yang minimal
// panjangnya jauh lebih ketat (16) krn itu bukan password login manusia.
const MinUserPasswordLen = 8

const (
	DeviceStatusOnline         = "ONLINE"
	DeviceStatusOffline        = "OFFLINE"
	DeviceStatusProvisioning   = "PROVISIONING"
	DeviceStatusFaulty         = "FAULTY"
	DeviceStatusUnregistered   = "UNREGISTERED"
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
	TaskTypeGetParameterValues     = "GET_PARAMETER_VALUES"
	TaskTypeSetParameterValues     = "SET_PARAMETER_VALUES"
	TaskTypeGetParameterNames      = "GET_PARAMETER_NAMES"
	TaskTypeAddObject              = "ADD_OBJECT"
	TaskTypeDeleteObject           = "DELETE_OBJECT"
	TaskTypeReboot                 = "REBOOT"
	TaskTypeFactoryReset           = "FACTORY_RESET"
	TaskTypeDownload               = "DOWNLOAD"
	TaskTypeUpload                 = "UPLOAD"
	TaskTypeScheduleInform         = "SCHEDULE_INFORM"
	TaskTypeSetParameterAttributes = "SET_PARAMETER_ATTRIBUTES"
	TaskTypeGetParameterAttributes = "GET_PARAMETER_ATTRIBUTES"
)

// Kode ref_ztp_trigger_event (migrations/0009) — kapan sebuah zero_touch_rules
// dievaluasi relatif thd event CWMP Inform. BOOTSTRAP_ONLY adalah nilai
// backfill rule existing (perilaku lama sebelum migrasi ini, jangan diubah
// penulisannya di sini tanpa migrasi data yang sepadan).
const (
	ZtpTriggerEventBootstrapOnly   = "BOOTSTRAP_ONLY"
	ZtpTriggerEventBootstrapOrBoot = "BOOTSTRAP_OR_BOOT"
	ZtpTriggerEventEveryInform     = "EVERY_INFORM"
)

// Kode ref_firmware_rollout_status (migrations/0011) — status lifecycle satu
// firmware_rollout_batches (canary/staged rollout).
const (
	FirmwareRolloutStatusPending                = "PENDING"
	FirmwareRolloutStatusInProgress             = "IN_PROGRESS"
	FirmwareRolloutStatusPausedFailureThreshold = "PAUSED_FAILURE_THRESHOLD"
	FirmwareRolloutStatusCompleted              = "COMPLETED"
	FirmwareRolloutStatusCancelled              = "CANCELLED"
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

// Kode fault CWMP standar (Broadband Forum TR-069 Annex A, dikirim CPE lewat
// cwmp:Fault) yang mendapat penanganan KHUSUS di usecase/session.handleFault
// selain alur retry generik (task.Service.Fail/failOrRetry) — lihat
// TECH.md §3/§4 dan komentar handleFault. Kode fault lain di luar ini TETAP
// lewat alur retry-lalu-gagal generik yang sudah ada, tidak berubah. Ini
// kode fault standar spec (bukan kuirk satu vendor), jadi wajar ditangani di
// sini (bukan internal/vendor_adapter/ yang khusus penyimpangan non-standar
// per CLAUDE.md).
const (
	// FaultCodeInvalidParameterName — 9005: parameter yang diminta memang
	// tidak ada pada device ini. Retry request identik akan gagal identik
	// setiap kali, sehingga task ditandai FAILED segera (task.Service.
	// FailPermanently), TIDAK lewat siklus max_retries seperti fault lain.
	FaultCodeInvalidParameterName = "9005"
	// FaultCodeInvalidArguments — 9003: pada task GET_PARAMETER_VALUES,
	// umumnya berarti CPE menolak krn ParameterNames terlalu panjang (kuirk
	// umum lintas vendor, bukan satu vendor spesifik). Ditangani dgn
	// memecah daftar nama jadi dua task baru (task.Service.
	// SplitGetParameterValuesOnFault) alih-alih retry request identik.
	// Untuk task type lain, 9003 tetap lewat alur retry generik biasa.
	FaultCodeInvalidArguments = "9003"
)
