package domain

import (
	"context"
	"time"
)

// Task adalah satu item antrean RPC CWMP untuk sebuah device (lihat TECH.md §4).
type Task struct {
	ID           uint64 `db:"id" json:"id"`
	TaskUUID     string `db:"task_uuid" json:"task_uuid"`
	DeviceID     uint64 `db:"device_id" json:"device_id"`
	TaskTypeID   uint64 `db:"task_type_id" json:"task_type_id"`
	TaskStatusID uint64 `db:"task_status_id" json:"task_status_id"`
	Priority     uint8  `db:"priority" json:"priority"` // 1 = tertinggi, 9 = terendah (lihat schema.sql)
	// Parameters/Response bertipe domain.JSONRawMessage, BUKAN []byte -- keduanya
	// SELALU berisi JSON (lihat komentar OutboundRPC di usecase/session dan
	// cwmp/handler.go json.Marshal ke RawResponse). encoding/json men-
	// treat []byte biasa sbg data BINER dan base64-encode saat serialisasi,
	// sehingga POST /tasks menerima `parameters` sbg objek JSON biasa tapi
	// GET /tasks mengembalikannya sbg string base64 -- asimetri request/
	// response yang membingungkan klien (ditemukan saat audit OpenAPI spec).
	// Dipakai JSONRawMessage (bukan json.RawMessage polos) krn kolomnya JSON
	// NULL di schema.sql -- json.RawMessage stdlib tidak punya sql.Scanner
	// sehingga gagal scan NULL (lihat komentar lengkap di domain/common.go).
	Parameters   JSONRawMessage `db:"parameters" json:"parameters"`
	Response     JSONRawMessage `db:"response" json:"response"`
	ErrorMessage *string        `db:"error_message" json:"error_message"`
	RetryCount   uint32         `db:"retry_count" json:"retry_count"`
	MaxRetries   uint32         `db:"max_retries" json:"max_retries"`
	ScheduledAt  *time.Time     `db:"scheduled_at" json:"scheduled_at"`
	ExpiresAt    *time.Time     `db:"expires_at" json:"expires_at"`
	SentAt       *time.Time     `db:"sent_at" json:"sent_at"`
	CompletedAt  *time.Time     `db:"completed_at" json:"completed_at"`
	Audit
	// TaskTypeCode — kode ref_task_types (mis. GET_PARAMETER_NAMES), hasil
	// JOIN di GetByID/List. Kosong pada query yang tidak ikut men-JOIN
	// ref_task_types (mis. SELECT * polos untuk sweeper internal).
	TaskTypeCode string `db:"task_type_code" json:"task_type_code,omitempty"`
}

type TaskFilter struct {
	DeviceID     *uint64
	TaskStatusID *uint64
	TaskTypeCode string
	// TenantID — RBAC scope tenant (CLAUDE.md). Tasks tidak punya tenant_id
	// langsung, di-resolve via JOIN ke devices di level repository.
	TenantID *uint64
}

// TaskStatusCount — agregasi untuk dashboard analitik (ROADMAP.md Fase 1).
type TaskStatusCount struct {
	TaskStatusID uint64 `db:"task_status_id" json:"task_status_id"`
	Count        int    `db:"cnt" json:"count"`
}

// TaskVendorErrorCount — agregasi jumlah task FAILED SAAT INI per vendor
// (ROADMAP.md Fase 2, metrik observability TECH.md §10: "error rate per
// vendor" mengindikasikan masalah kompatibilitas parameter mapping). VendorID
// nil berarti device pemilik task belum ter-resolve vendor-nya (mis. belum
// pernah Inform/OUI tidak dikenal).
type TaskVendorErrorCount struct {
	VendorID *uint64 `db:"vendor_id" json:"vendor_id"`
	Count    int     `db:"cnt" json:"count"`
}

type TaskRepository interface {
	Create(ctx context.Context, t *Task) error
	GetByID(ctx context.Context, id uint64) (*Task, error)
	GetByUUID(ctx context.Context, uuid string) (*Task, error)
	// CountByStatus — agregasi GROUP BY, tenantID nil = lintas tenant
	// (superadmin). Butuh JOIN devices karena tasks tidak punya tenant_id
	// langsung.
	CountByStatus(ctx context.Context, tenantID *uint64) ([]TaskStatusCount, error)
	// AvgCompletionSeconds — rata-rata TIMESTAMPDIFF(SECOND, created_at,
	// completed_at) untuk task COMPLETED yang completed_at-nya >= since
	// (metrik observability TECH.md §10 "rata-rata waktu penyelesaian task").
	// Sengaja diagregasi via AVG SQL atas data historis di DB, BUKAN
	// histogram real-time — lihat internal/metrics/collector.go untuk alasan
	// lengkap kenapa ini gauge, bukan histogram/summary Prometheus asli.
	// Mengembalikan nil bila tidak ada task selesai dalam window tsb.
	AvgCompletionSeconds(ctx context.Context, tenantID *uint64, since time.Time) (*float64, error)
	// CountFailedByVendor — agregasi GROUP BY vendor_id utk task berstatus
	// FAILED saat ini (snapshot state, bukan counter kumulatif). Butuh JOIN
	// devices (sama seperti CountByStatus) utk resolve vendor_id & tenant_id.
	CountFailedByVendor(ctx context.Context, tenantID *uint64) ([]TaskVendorErrorCount, error)
	// NextForDevice mengambil task PENDING milik device, terurut priority ASC
	// (1 = tertinggi dieksekusi lebih dulu), created_at ASC.
	NextForDevice(ctx context.Context, deviceID uint64) (*Task, error)
	HasPendingForDevice(ctx context.Context, deviceID uint64) (bool, error)
	// CountPendingForTenant — hitung task PENDING+QUEUED (sama seperti
	// HasPendingForDevice) milik SATU tenant, lewat JOIN devices (tasks tidak
	// punya tenant_id langsung). Dipakai kuota task queue per tenant
	// (enforceTenantTaskQuota, ROADMAP.md Fase 2) — query COUNT murni,
	// sengaja BUKAN List() yang juga fetch baris lengkap (termasuk kolom
	// JSON parameters) padahal cuma butuh angkanya di jalur panas ini.
	CountPendingForTenant(ctx context.Context, tenantID uint64) (int, error)
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
	MarkCompleted(ctx context.Context, id uint64, response JSONRawMessage, completedAt time.Time) error
	MarkFailed(ctx context.Context, id uint64, statusID uint64, errMsg string) error
	SetErrorMessage(ctx context.Context, id uint64, errMsg string) error
	IncrementRetry(ctx context.Context, id uint64) error
	Cancel(ctx context.Context, id uint64, updatedBy *uint64) error
}

// TaskEnqueuer dipakai usecase/provisioning untuk mengantre task
// SetParameterValues/Reboot tanpa bergantung langsung pada package
// usecase/task (menghindari import cycle — lihat usecase/task/service.go).
type TaskEnqueuer interface {
	EnqueueSetParameterValues(ctx context.Context, actor Actor, deviceID uint64, params map[string]string, priority uint8) (*Task, error)
	EnqueueGetParameterNames(ctx context.Context, actor Actor, deviceID uint64, path string, nextLevel bool, priority uint8) (*Task, error)
	// EnqueueReboot — dipakai aksi PostApplyReboot pada ZeroTouchRule
	// (migrations/0009, usecase/provisioning.EvaluateZeroTouch).
	EnqueueReboot(ctx context.Context, actor Actor, deviceID uint64, priority uint8) (*Task, error)
	// HasPendingForDevice — dipakai EvaluateZeroTouch sbg pagar longgar
	// terhadap rule ber-trigger BOOTSTRAP_OR_BOOT/EVERY_INFORM yang aksinya
	// bisa berulang tanpa henti (mis. PostApplyReboot dipasangkan EVERY_INFORM
	// -> reboot -> event BOOT -> Inform baru -> cocok lagi -> reboot lagi).
	// TIDAK menjamin mencegah loop sepenuhnya (device bisa saja benar-benar
	// kosong dari task lain), hanya memperlambat -- lihat komentar lengkap di
	// EvaluateZeroTouch.
	HasPendingForDevice(ctx context.Context, deviceID uint64) (bool, error)
	// ResolveParameterPath menerjemahkan logical key -> raw TR-069 path sesuai
	// vendor/model device (FR-11). Raw path yang sudah eksplisit diteruskan apa
	// adanya. Dipakai usecase/provisioning.EvaluatePresets untuk drift-check
	// (bandingkan nilai target preset dgn device_parameters yang di-key oleh
	// raw path). task.Service sudah mengimplementasikannya.
	ResolveParameterPath(ctx context.Context, deviceID uint64, key string) (string, error)
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
