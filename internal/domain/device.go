package domain

import (
	"context"
	"time"
)

// Device adalah inventory CPE. Kredensial connection request & inform auth
// disimpan terenkripsi (*_enc) — lihat pkg/cryptoutil dan CLAUDE.md.
type Device struct {
	ID                    uint64  `db:"id" json:"id"`
	DeviceUUID            string  `db:"device_uuid" json:"device_uuid"`
	TenantID              *uint64 `db:"tenant_id" json:"tenant_id"`
	VendorID              *uint64 `db:"vendor_id" json:"vendor_id"`
	DeviceModelID         *uint64 `db:"device_model_id" json:"device_model_id"`
	DeviceStatusID        uint64  `db:"device_status_id" json:"device_status_id"`
	ProvisioningProfileID *uint64 `db:"provisioning_profile_id" json:"provisioning_profile_id"`
	OUI                   *string `db:"oui" json:"oui"`
	SerialNumber          string  `db:"serial_number" json:"serial_number"`
	ProductClass          *string `db:"product_class" json:"product_class"`
	MACAddress            *string `db:"mac_address" json:"mac_address"`
	SoftwareVersion       *string `db:"software_version" json:"software_version"`
	HardwareVersion       *string `db:"hardware_version" json:"hardware_version"`
	IPAddress             *string `db:"ip_address" json:"ip_address"`
	ConnectionRequestURL  *string `db:"connection_request_url" json:"connection_request_url"`
	// ConnectionRequestUsername -- json:"-" (BUKAN diekspos apa adanya):
	// CLAUDE.md mengelompokkan connection_request_username/password sbg
	// sepasang kredensial yang sama-sama tidak boleh dikembalikan plaintext
	// lewat API manapun, tapi versi sebelumnya cuma menyembunyikan
	// password-nya (ConnectionRequestPasswordEnc di bawah) -- username-nya
	// tetap bocor ke SEMUA role termasuk VIEWER (temuan audit OpenAPI spec).
	// Tidak dipakai di frontend manapun saat ini (dicek langsung, bukan
	// asumsi), jadi menyembunyikannya tidak menghilangkan fungsi UI apa pun.
	ConnectionRequestUsername    *string    `db:"connection_request_username" json:"-"`
	ConnectionRequestPasswordEnc []byte     `db:"connection_request_password_enc" json:"-"`
	InformUsername               *string    `db:"inform_username" json:"inform_username"`
	InformPasswordEnc            []byte     `db:"inform_password_enc" json:"-"`
	LastInformAt                 *time.Time `db:"last_inform_at" json:"last_inform_at"`
	LastBootEventAt              *time.Time `db:"last_boot_event_at" json:"last_boot_event_at"`
	Latitude                     *float64   `db:"latitude" json:"latitude"`
	Longitude                    *float64   `db:"longitude" json:"longitude"`
	Notes                        *string    `db:"notes" json:"notes"`
	Audit
}

type DeviceFilter struct {
	TenantID       *uint64
	VendorID       *uint64
	DeviceModelID  *uint64
	DeviceStatusID *uint64
	Search         string  // cocok ke serial_number/mac_address
	TagID          *uint64 // hanya device yang punya tag ini (device_tags, migrations/0019)
}

// DeviceStatusCount/DeviceVendorCount — hasil agregasi utk dashboard analitik
// (ROADMAP.md Fase 1). VendorID nil berarti device belum ter-resolve vendor-nya.
type DeviceStatusCount struct {
	DeviceStatusID uint64 `db:"device_status_id" json:"device_status_id"`
	Count          int    `db:"cnt" json:"count"`
}

type DeviceVendorCount struct {
	VendorID *uint64 `db:"vendor_id" json:"vendor_id"`
	Count    int     `db:"cnt" json:"count"`
}

type DeviceRepository interface {
	Create(ctx context.Context, d *Device) error
	GetByID(ctx context.Context, id uint64) (*Device, error)
	GetByUUID(ctx context.Context, uuid string) (*Device, error)
	GetByOUISerial(ctx context.Context, oui, serial string) (*Device, error)
	List(ctx context.Context, f DeviceFilter, p Pagination) ([]Device, int, error)
	// CountByStatus/CountByVendor — agregasi GROUP BY untuk dashboard, jauh
	// lebih murah daripada N query List(page_size=1) per status/vendor dari
	// frontend. tenantID nil = agregat lintas tenant (view superadmin).
	CountByStatus(ctx context.Context, tenantID *uint64) ([]DeviceStatusCount, error)
	CountByVendor(ctx context.Context, tenantID *uint64) ([]DeviceVendorCount, error)
	Update(ctx context.Context, d *Device) error
	UpdateStatus(ctx context.Context, id, statusID uint64, updatedBy *uint64) error
	MarkStaleOffline(ctx context.Context, offlineStatusID uint64, staleBefore time.Time) (int64, error)
	SoftDelete(ctx context.Context, id, deletedBy uint64) error
}

// DeviceParameter — EAV nilai parameter TR-069 terkini per device (hasil sync).
type DeviceParameter struct {
	ID              uint64    `db:"id" json:"id"`
	DeviceID        uint64    `db:"device_id" json:"device_id"`
	ParameterName   string    `db:"parameter_name" json:"parameter_name"`
	ParameterValue  *string   `db:"parameter_value" json:"parameter_value"`
	ParameterTypeID *uint64   `db:"parameter_type_id" json:"parameter_type_id"`
	Writable        bool      `db:"writable" json:"writable"`
	CreatedAt       time.Time `db:"created_at" json:"created_at"`
	UpdatedAt       time.Time `db:"updated_at" json:"updated_at"`
}

type DeviceParameterRepository interface {
	Upsert(ctx context.Context, p *DeviceParameter) error
	UpsertBatch(ctx context.Context, ps []DeviceParameter) error
	ListByDevice(ctx context.Context, deviceID uint64, prefix string) ([]DeviceParameter, error)
	Get(ctx context.Context, deviceID uint64, name string) (*DeviceParameter, error)
}

// DeviceConfigSnapshot menyimpan status parameter full pada waktu tertentu
type DeviceConfigSnapshot struct {
	ID           uint64         `db:"id" json:"id"`
	DeviceID     uint64         `db:"device_id" json:"device_id"`
	SnapshotData JSONRawMessage `db:"snapshot_data" json:"snapshot_data"`
	CreatedAt    time.Time      `db:"created_at" json:"created_at"`
}

type DeviceConfigSnapshotRepository interface {
	Create(ctx context.Context, s *DeviceConfigSnapshot) error
	ListByDevice(ctx context.Context, deviceID uint64, p Pagination) ([]DeviceConfigSnapshot, int, error)
	GetByID(ctx context.Context, id uint64) (*DeviceConfigSnapshot, error)
}

// DeviceSession — sesi CWMP aktif/historis. Memungkinkan app server stateless
// (lihat TECH.md §3/§9): instance manapun bisa melanjutkan sesi via session_token.
type DeviceSession struct {
	ID           uint64  `db:"id" json:"id"`
	DeviceID     uint64  `db:"device_id" json:"device_id"`
	SessionToken string  `db:"session_token" json:"session_token"`
	CWMPID       *string `db:"cwmp_id" json:"cwmp_id"`
	// CWMPNamespace -- namespace CWMP yang dideklarasikan CPE pada Inform
	// sesi ini (migrations/0012), dipakai ulang utk SELURUH RPC proaktif
	// yang dikirim ACS sepanjang sesi yang sama (lihat NextRequest) --
	// bukan cuma balasan InformResponse yang sama-request. Lihat komentar
	// lengkap di migrations/0012 soal bug yang diperbaiki ini.
	CWMPNamespace *string    `db:"cwmp_namespace" json:"cwmp_namespace"`
	Status        string     `db:"status" json:"status"` // OPEN, CLOSED, ERROR
	RemoteIP      *string    `db:"remote_ip" json:"remote_ip"`
	StartedAt     time.Time  `db:"started_at" json:"started_at"`
	EndedAt       *time.Time `db:"ended_at" json:"ended_at"`
	CreatedAt     time.Time  `db:"created_at" json:"created_at"`
}

const (
	SessionStatusOpen   = "OPEN"
	SessionStatusClosed = "CLOSED"
	SessionStatusError  = "ERROR"
	// SessionStatusTimeout — sesi yang ditinggalkan CPE tanpa POST-kosong
	// penutup (mis. koneksi putus di tengah sesi), di-reap oleh sweeper
	// periodik di cmd/acsd setelah melewati ambang usia. Dibedakan dari
	// ERROR (fault protokol yang eksplisit) supaya observability bisa
	// memisahkan "CPE menghilang" dari "sesi gagal".
	SessionStatusTimeout = "TIMEOUT"
)

type DeviceSessionRepository interface {
	Create(ctx context.Context, s *DeviceSession) error
	GetByToken(ctx context.Context, token string) (*DeviceSession, error)
	UpdateStatus(ctx context.Context, id uint64, status string, endedAt *time.Time) error
	SetCWMPID(ctx context.Context, id uint64, cwmpID string) error
	// SetCWMPNamespace — dipanggil SEKALI per sesi saat Inform diproses (lihat
	// migrations/0012 & komentar CWMPNamespace di atas).
	SetCWMPNamespace(ctx context.Context, id uint64, namespace string) error
	// CountOpen — jumlah sesi CWMP berstatus OPEN saat ini, lintas seluruh
	// tenant (metrik observability TECH.md §10). device_sessions tidak
	// menyimpan tenant_id langsung dan metrik ini utk operator platform,
	// sehingga sengaja tidak tenant-scoped (beda dgn CountByStatus milik
	// DeviceRepository/TaskRepository yang menerima tenantID nullable).
	CountOpen(ctx context.Context) (int, error)
	// TimeoutStaleOpen menandai semua sesi berstatus OPEN yang started_at-nya
	// lebih lama dari olderThan menjadi TIMEOUT (ended_at = NOW()). Dipakai
	// sweeper periodik cmd/acsd untuk membersihkan sesi yang tidak pernah
	// ditutup CPE (POST-kosong penutup tak pernah datang) — tanpa ini
	// device_sessions.status='OPEN' menumpuk selamanya dan metrik
	// acs_cwmp_sessions_open jadi tidak berguna. Mengembalikan jumlah baris
	// yang diubah. Aman dijalankan dari instance manapun (idempoten, app
	// server stateless — TECH.md §9).
	TimeoutStaleOpen(ctx context.Context, olderThan time.Time) (int64, error)
}

// DeviceEvent — histori event Inform, audit minimal (created_at saja) sesuai
// TECH.md §11 (tabel log volume tinggi, kandidat partisi by month nanti).
type DeviceEvent struct {
	ID          uint64    `db:"id" json:"id"`
	DeviceID    uint64    `db:"device_id" json:"device_id"`
	SessionID   *uint64   `db:"session_id" json:"session_id"`
	EventCodeID uint64    `db:"event_code_id" json:"event_code_id"`
	CommandKey  *string   `db:"command_key" json:"command_key"`
	RawPayload  *string   `db:"raw_payload" json:"raw_payload"`
	OccurredAt  time.Time `db:"occurred_at" json:"occurred_at"`
	CreatedAt   time.Time `db:"created_at" json:"created_at"`
}

type DeviceEventRepository interface {
	Create(ctx context.Context, e *DeviceEvent) error
	ListByDevice(ctx context.Context, deviceID uint64, p Pagination) ([]DeviceEvent, int, error)
}

// DeviceOpticalMetric — metrik optik GPON/EPON, audit minimal (lihat TECH.md §11).
type DeviceOpticalMetric struct {
	ID                 uint64    `db:"id" json:"id"`
	DeviceID           uint64    `db:"device_id" json:"device_id"`
	RxPowerDBM         *float64  `db:"rx_power_dbm" json:"rx_power_dbm"`
	TxPowerDBM         *float64  `db:"tx_power_dbm" json:"tx_power_dbm"`
	Voltage            *float64  `db:"voltage" json:"voltage"`
	BiasCurrentMA      *float64  `db:"bias_current_ma" json:"bias_current_ma"`
	TemperatureCelsius *float64  `db:"temperature_celsius" json:"temperature_celsius"`
	DistanceMeters     *float64  `db:"distance_meters" json:"distance_meters"`
	RecordedAt         time.Time `db:"recorded_at" json:"recorded_at"`
	CreatedAt          time.Time `db:"created_at" json:"created_at"`
}

type DeviceOpticalMetricRepository interface {
	Create(ctx context.Context, m *DeviceOpticalMetric) error
	Latest(ctx context.Context, deviceID uint64) (*DeviceOpticalMetric, error)
	ListByDevice(ctx context.Context, deviceID uint64, p Pagination) ([]DeviceOpticalMetric, int, error)
}
