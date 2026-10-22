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
	ConnectionRequestUsername    *string `db:"connection_request_username" json:"-"`
	ConnectionRequestPasswordEnc []byte  `db:"connection_request_password_enc" json:"-"`
	// CRCredentialsSet — TURUNAN (bukan kolom; diisi Service.Get): username & password Connection Request sudah diisi.
	// Hanya penanda untuk UI; kredensialnya sendiri tidak pernah diekspos.
	CRCredentialsSet  bool       `db:"-" json:"connection_request_credentials_set"`
	InformUsername    *string    `db:"inform_username" json:"inform_username"`
	InformPasswordEnc []byte     `db:"inform_password_enc" json:"-"`
	LastInformAt      *time.Time `db:"last_inform_at" json:"last_inform_at"`
	LastBootEventAt   *time.Time `db:"last_boot_event_at" json:"last_boot_event_at"`
	Latitude          *float64   `db:"latitude" json:"latitude"`
	Longitude         *float64   `db:"longitude" json:"longitude"`
	Notes             *string    `db:"notes" json:"notes"`
	Audit
}

type DeviceFilter struct {
	TenantID       *uint64
	VendorID       *uint64
	DeviceModelID  *uint64
	DeviceStatusID *uint64
	Search         string // cocok ke serial_number/mac_address
	// SerialNumber: cocok PERSIS (indeks idx_devices_serial, migrations/0041) — pola integrasi BSS; beda dari Search yang memindai.
	SerialNumber string
	TagID        *uint64 // hanya device yang punya tag ini (device_tags, migrations/0019)
	// OpticalLevel menyaring berdasarkan RX terakhir (kosong = semua): OpticalOK/Warn/Bad/Unknown.
	OpticalLevel string
	// NoVendor — hanya device yang vendornya belum terpetakan (vendor_id NULL, tampil "Belum dikenal").
	NoVendor bool
	// Params: kondisi pada nilai device_parameters (semua harus terpenuhi / AND). Lihat ParseParamCond.
	Params []ParamCond
	// InformBefore/InformAfter membatasi last_inform_at (mis. "tidak Inform sejak kemarin" = InformBefore).
	InformBefore, InformAfter *time.Time
	// Sort: salah satu DeviceSortFields (kosong = id); SortDesc membalik arah.
	Sort     string
	SortDesc bool
	// ProjectParams: nama parameter yang nilainya ikut dikirim pada baris daftar (DeviceListItem.Parameters).
	ProjectParams []string
}

// Level redaman optik (RX) — satu sumber ambang untuk filter daftar & peringatan; frontend mencerminkannya di lib/optical.ts.
const (
	OpticalOK      = "ok"
	OpticalWarn    = "warn"
	OpticalBad     = "bad"
	OpticalUnknown = "unknown" // belum ada pembacaan RX

	OpticalRXWarnBelowDBM     = -25.0 // lebih lemah dari ini = waspada
	OpticalRXBadBelowDBM      = -27.0 // lebih lemah dari ini = kritis (dekat sensitivitas penerima)
	OpticalRXOverloadAboveDBM = -8.0  // lebih kuat dari ini = kritis (receiver overload)
)

// OpticalLevelOf mengklasifikasi RX (dBm); nil = OpticalUnknown.
func OpticalLevelOf(rx *float64) string {
	switch {
	case rx == nil:
		return OpticalUnknown
	case *rx < OpticalRXBadBelowDBM || *rx > OpticalRXOverloadAboveDBM:
		return OpticalBad
	case *rx < OpticalRXWarnBelowDBM:
		return OpticalWarn
	default:
		return OpticalOK
	}
}

// NormalizeRXDBM menebak satuan pembacaan RX mentah yang belum dikonversi: dBm langsung (-23,5), per 0,01 dBm (-2350)
// atau per 0,001 dBm (-23500, mis. TR-181 OpticalSignalLevel). Memakai skala pertama yang masuk akal secara fisik
// (-60..+15 dBm); ok=false bila tak ada — jangan membunyikan alarm dari satuan yang tidak jelas.
func NormalizeRXDBM(raw float64) (dbm float64, ok bool) {
	for _, scale := range []float64{1, 0.01, 0.001} {
		if v := raw * scale; v >= -60 && v <= 15 {
			return v, true
		}
	}
	return 0, false
}

// ValidOpticalLevel — nilai filter yang diterima (kosong tidak termasuk).
func ValidOpticalLevel(v string) bool {
	return v == OpticalOK || v == OpticalWarn || v == OpticalBad || v == OpticalUnknown
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
	// SoftDelete menandai device terhapus. Create() meng-"revive" baris terhapus
	// dengan (oui, serial) yang sama bila CPE itu Inform lagi (unique key
	// uq_devices_oui_serial mencakup baris terhapus) — lihat implementasinya.
	SoftDelete(ctx context.Context, id, deletedBy uint64) error
}

// OpenTaskCanceller membatalkan task yang belum terminal (PENDING/QUEUED/SENT)
// milik satu device. Dipakai saat device dihapus supaya task lama tidak
// "bangkit" menyerang device yang kelak di-revive. Interface terpisah (bukan
// method TaskRepository) agar kontrak TaskRepository tidak melebar.
type OpenTaskCanceller interface {
	CancelOpenForDevice(ctx context.Context, deviceID uint64, updatedBy *uint64) (int64, error)
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
	// HasPrefix: apakah device punya parameter berawalan prefix (deteksi root data model
	// TR-098/TR-181 dari parameter yang pernah dilaporkan CPE).
	HasPrefix(ctx context.Context, deviceID uint64, prefix string) (bool, error)
	// UpsertNames mencatat NAMA parameter (+ writable) hasil GetParameterNames TANPA menimpa
	// nilai yang sudah ada (INSERT baru bernilai NULL).
	UpsertNames(ctx context.Context, ps []DeviceParameter) error
	ListByDevice(ctx context.Context, deviceID uint64, prefix string) ([]DeviceParameter, error)
	Get(ctx context.Context, deviceID uint64, name string) (*DeviceParameter, error)
	// PPPoEUsernames: username PPPoE (parameter standar TR-098 WANPPPConnection.N.Username / TR-181 PPP.Interface.N.Username,
	// instance mana pun) untuk banyak device sekaligus; hanya yang terisi. Dipakai kolom daftar perangkat.
	PPPoEUsernames(ctx context.Context, deviceIDs []uint64) (map[uint64]string, error)
	// ValuesFor: nilai parameter bernama `names` (persis) untuk banyak device sekaligus; hanya yang terisi. Dipakai proyeksi
	// ?params= pada daftar perangkat. Pemanggil bertanggung jawab menyaring nama rahasia.
	ValuesFor(ctx context.Context, deviceIDs []uint64, names []string) (map[uint64]map[string]string, error)
	// GetBySuffix: parameter terbaru milik device yang namanya berakhiran suffix (mis. ".ManagementServer.ConnectionRequestURL"
	// — lintas TR-098/TR-181); ErrNotFound bila tak ada.
	GetBySuffix(ctx context.Context, deviceID uint64, suffix string) (*DeviceParameter, error)
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
	// CountOpenForTenant — sama, tetapi hanya sesi milik perangkat tenant tsb (untuk ADMIN/NOC tenant:
	// jumlah sesi seluruh platform adalah metadata tenant lain; audit OWASP A01).
	CountOpenForTenant(ctx context.Context, tenantID uint64) (int, error)
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
	// LatestForDevices: metrik terbaru per device untuk banyak device sekaligus (tanpa N+1); device tanpa data tidak ada di map.
	LatestForDevices(ctx context.Context, deviceIDs []uint64) (map[uint64]DeviceOpticalMetric, error)
}

// DeviceListItem = Device + ringkasan yang dibutuhkan baris daftar perangkat (redaman optik terakhir & username PPPoE).
// Nilai kosong (null) berarti belum pernah terbaca dari perangkat — bukan nol.
type DeviceListItem struct {
	Device
	RxPowerDBM    *float64   `json:"rx_power_dbm"`
	TxPowerDBM    *float64   `json:"tx_power_dbm"`
	OpticalAt     *time.Time `json:"optical_at"`
	PPPoEUsername *string    `json:"pppoe_username"`
	// Parameters: nilai parameter yang diminta lewat ?params= (tidak diminta = tidak ikut dikirim).
	Parameters map[string]string `json:"parameters,omitempty"`
	// Virtual: nilai virtual parameter yang diminta lewat ?vparams= (nama -> nilai; yang belum dapat dihitung tidak muncul).
	Virtual map[string]string `json:"virtual,omitempty"`
}
