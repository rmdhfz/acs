package domain

import (
	"context"
	"math"
	"time"
)

// ---- OLT Controller, irisan B: ONU TERDAFTAR di OLT (migrations/0047) ----
// Read-only terhadap OLT. Bentuk datanya NETRAL (keadaan dinormalisasi driver; nilai asli vendor disimpan terpisah): driver nyata
// tinggal memetakan keluaran perangkatnya ke ReportedONU. Metode ini opsional (OLTInventoryDriver) agar driver yang hanya bisa
// membaca antrean ONU belum terdaftar tidak dipaksa mengimplementasikannya.

// Kode ref_olt_onu_state — keadaan ONU menurut OLT, dinormalisasi oleh driver. UNKNOWN = tak terpetakan (jangan ditebak).
const (
	OLTStateUnknown   = "UNKNOWN"
	OLTStateOnline    = "ONLINE"
	OLTStateOffline   = "OFFLINE"
	OLTStateLOS       = "LOS"
	OLTStateDyingGasp = "DYING_GASP"
)

// ValidOLTONUState: kode keadaan yang dikenal.
func ValidOLTONUState(code string) bool {
	switch code {
	case OLTStateUnknown, OLTStateOnline, OLTStateOffline, OLTStateLOS, OLTStateDyingGasp:
		return true
	}
	return false
}

// Batas (dicerminkan dari lebar kolom migrations/0047).
const (
	ONUIndexMaxLen       = 16
	ONUVendorStateMaxLen = 64
	// OLTMaxRegisteredONUs membatasi baris ONU terdaftar per OLT (128 port x 128 ONU = 16384 pada OLT besar). Daftar berasal dari perangkat luar;
	// bila terlampaui polling TIDAK diterapkan (daftar terpotong akan membuat ONU di luar batas dianggap hilang).
	OLTMaxRegisteredONUs = 20000
	// OLTONURxMinDBM/MaxDBM = rentang masuk akal daya terima sisi OLT; di luarnya dibuang (bukan disimpan sebagai angka palsu).
	OLTONURxMinDBM = -80.0
	OLTONURxMaxDBM = 30.0
)

// ValidOLTRx: daya dBm hingga dan masuk akal.
func ValidOLTRx(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= OLTONURxMinDBM && v <= OLTONURxMaxDBM
}

// ReportedONU = satu ONU terdaftar yang dilaporkan OLT. State sudah dinormalisasi driver (kode OLTState*); VendorState = nilai asli.
type ReportedONU struct {
	PonPort      string
	ONUIndex     string
	SerialNumber string
	ONUType      string
	State        string
	VendorState  string
	RxPowerDBM   *float64
}

// OLTInventoryDriver = kemampuan OPSIONAL driver: membaca ONU yang sudah terdaftar. Dicek lewat type assertion pada OLTDriver.
type OLTInventoryDriver interface {
	ListRegisteredONUs(ctx context.Context, c OLTConnection) ([]ReportedONU, error)
}

// RegisteredONU = baris olt_registered_onus. Kolom tautan (DeviceID, DeviceStatus) berasal dari join ke devices (serial + tenant sama)
// dan hanya terisi pada query daftar/pencarian; nil = tak ada perangkat ACS dengan serial itu di tenant OLT.
type RegisteredONU struct {
	ID                uint64    `db:"id" json:"-"`
	UUID              string    `db:"onu_uuid" json:"id"`
	OLTID             uint64    `db:"olt_id" json:"-"`
	OLTUUID           string    `db:"olt_uuid" json:"olt_id"`
	OLTName           string    `db:"olt_name" json:"olt_name"`
	TenantID          uint64    `db:"tenant_id" json:"tenant_id"`
	PonPort           string    `db:"pon_port" json:"pon_port"`
	ONUIndex          *string   `db:"onu_index" json:"onu_index"`
	SerialNumber      string    `db:"serial_number" json:"serial_number"`
	ONUType           *string   `db:"onu_type" json:"onu_type"`
	StateID           uint64    `db:"state_id" json:"-"`
	State             string    `db:"state" json:"state"` // kode ref_olt_onu_state (join)
	VendorState       *string   `db:"vendor_state" json:"vendor_state"`
	RxPowerDBM        *float64  `db:"rx_power_dbm" json:"rx_power_dbm"`
	FirstSeenAt       time.Time `db:"first_seen_at" json:"first_seen_at"`
	LastSeenAt        time.Time `db:"last_seen_at" json:"last_seen_at"`
	LastStateChangeAt time.Time `db:"last_state_change_at" json:"last_state_change_at"`
	// Tautan ke ACS.
	DeviceID     *uint64 `db:"device_id" json:"device_id"`
	DeviceStatus *string `db:"device_status" json:"device_status"`
	// OLTLastPollAt = waktu polling terakhir OLT pemilik; menandai seberapa basi keadaan di atas.
	OLTLastPollAt *time.Time `db:"olt_last_poll_at" json:"olt_last_poll_at"`
}

// RegisteredONUFilter — TenantID wajib diisi usecase untuk non-superadmin.
type RegisteredONUFilter struct {
	TenantID *uint64
	OLTID    *uint64
	PonPort  string
	StateID  *uint64
	Search   string // serial atau port (LIKE)
}

// PONPortSummary = ringkasan satu port PON dari ONU terdaftar. Port diturunkan dari data ONU (bukan tabel port): status admin port
// (aktif/nonaktif) tidak diketahui tanpa driver nyata, jadi tidak ditampilkan.
type PONPortSummary struct {
	PonPort    string     `db:"pon_port" json:"pon_port"`
	Total      int        `db:"total" json:"total"`
	Online     int        `db:"n_online" json:"online"`
	Offline    int        `db:"n_offline" json:"offline"`
	LOS        int        `db:"n_los" json:"los"`
	DyingGasp  int        `db:"n_dying_gasp" json:"dying_gasp"`
	Unknown    int        `db:"n_unknown" json:"unknown"`
	Linked     int        `db:"linked" json:"linked"` // di antaranya yang tertaut ke perangkat ACS
	MinRxDBM   *float64   `db:"min_rx_dbm" json:"min_rx_dbm"`
	AvgRxDBM   *float64   `db:"avg_rx_dbm" json:"avg_rx_dbm"`
	LastSeenAt *time.Time `db:"last_seen_at" json:"last_seen_at"`
}

// OLTInventoryRepository — hanya query, tanpa keputusan (rekonsiliasi diputuskan usecase/olt). Antarmuka terpisah dari OLTRepository
// supaya kontrak OLTRepository tidak melebar.
type OLTInventoryRepository interface {
	// ListRegisteredByOLT mengembalikan seluruh ONU aktif sebuah OLT untuk rekonsiliasi (dibatasi OLTMaxRegisteredONUs + 1).
	ListRegisteredByOLT(ctx context.Context, oltID uint64) ([]RegisteredONU, error)
	// UpsertRegisteredONUs menyisipkan/memperbarui per (olt_id, serial_number). onu_uuid dipertahankan; first_seen_at dan
	// last_state_change_at diatur ulang bila baris sebelumnya dihapus-lunak, last_state_change_at hanya maju bila keadaan berubah.
	UpsertRegisteredONUs(ctx context.Context, oltID uint64, rows []RegisteredONU) error
	// RemoveRegisteredONUs menghapus-lunak ONU yang tidak lagi dilaporkan OLT.
	RemoveRegisteredONUs(ctx context.Context, ids []uint64, by *uint64) error
	ListRegisteredONUs(ctx context.Context, f RegisteredONUFilter, p Pagination) ([]RegisteredONU, int, error)
	PortSummaries(ctx context.Context, oltID uint64) ([]PONPortSummary, error)
	// FindRegisteredBySerial mencari ONU (dicocokkan tanpa peka huruf besar/kecil) di OLT milik tenant itu; yang terakhir terlihat menang.
	FindRegisteredBySerial(ctx context.Context, tenantID uint64, serial string) (*RegisteredONU, error)
	// CountActiveOLTs = jumlah OLT aktif tenant (dasar menyatakan "tenant punya OLT tetapi ONU ini tak ditemukan").
	CountActiveOLTs(ctx context.Context, tenantID uint64) (int, error)
	// ListOLTsDueForPoll = OLT aktif yang belum pernah dipoll atau terakhir dipoll sebelum `before` (terlama dulu), lintas tenant.
	ListOLTsDueForPoll(ctx context.Context, before time.Time, limit int) ([]OLT, error)
}
