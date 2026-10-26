package domain

import (
	"context"
	"fmt"
	"time"
)

// ---- OLT Controller, irisan 1: inventaris OLT + antrean ONU belum terdaftar (migrations/0044) ----
// Rancangan dan batas kerja: docs/arsitektur/OLT_CONTROLLER_DESIGN.md. Read-only: tidak ada antarmuka untuk menulis ke OLT.

// Kode ref_olt_poll_status.
const (
	OLTPollUnknown     = "UNKNOWN"
	OLTPollOK          = "OK"
	OLTPollUnreachable = "UNREACHABLE"
	OLTPollAuthFailed  = "AUTH_FAILED"
	OLTPollError       = "ERROR"
)

// Kode ref_onu_discovery_status.
const (
	ONUStatusPending  = "PENDING"
	ONUStatusIgnored  = "IGNORED"
	ONUStatusResolved = "RESOLVED"
)

// Batas masukan (dicerminkan dari lebar kolom di migrations/0044 dan frontend/src/lib/limits.ts).
const (
	OLTNameMaxLen       = 128
	OLTHostMaxLen       = 255
	OLTNotesMaxLen      = 255
	OLTCredentialMaxLen = 128
	OLTDriverCodeMaxLen = 32
	ONUSerialMaxLen     = 32
	ONUPortMaxLen       = 32
	ONUTypeMaxLen       = 64
	OLTInfoMaxLen       = 128
	OLTErrorMaxLen      = 255
	// OLTMaxUnregisteredONUs membatasi baris antrean per OLT: daftar berasal dari perangkat luar, jadi tanpa batas OLT yang rusak
	// (atau jahat) dapat menumbuhkan tabel tanpa henti.
	OLTMaxUnregisteredONUs = 5000
	// OLTKeepResolvedONUs = riwayat entri RESOLVED yang dipertahankan per OLT (yang lebih lama dihapus-lunak saat polling).
	OLTKeepResolvedONUs = 200
)

// ErrOLTUnreachable / ErrOLTAuthFailed dikembalikan driver; keduanya membungkus ErrUpstreamUnavailable (HTTP 502). Teks dari
// perangkat tidak pernah ikut diteruskan ke klien.
var (
	ErrOLTUnreachable = fmt.Errorf("%w: OLT tidak dapat dijangkau", ErrUpstreamUnavailable)
	ErrOLTAuthFailed  = fmt.Errorf("%w: OLT menolak autentikasi", ErrUpstreamUnavailable)
)

// OLT = satu perangkat OLT milik satu tenant. Kredensial terenkripsi (json:"-"); API hanya menampilkan penandanya.
type OLT struct {
	ID               uint64     `db:"id" json:"-"`
	UUID             string     `db:"olt_uuid" json:"id"`
	TenantID         uint64     `db:"tenant_id" json:"tenant_id"`
	Name             string     `db:"name" json:"name"`
	DriverCode       string     `db:"driver_code" json:"driver_code"`
	Host             string     `db:"host" json:"host"`
	SNMPPort         uint16     `db:"snmp_port" json:"snmp_port"`
	SSHPort          uint16     `db:"ssh_port" json:"ssh_port"`
	SNMPCommunityEnc []byte     `db:"snmp_community_enc" json:"-"`
	SSHUsername      *string    `db:"ssh_username" json:"ssh_username"`
	SSHPasswordEnc   []byte     `db:"ssh_password_enc" json:"-"`
	Notes            *string    `db:"notes" json:"notes"`
	IsActive         bool       `db:"is_active" json:"is_active"`
	PollStatusID     uint64     `db:"poll_status_id" json:"-"`
	PollStatus       string     `db:"poll_status" json:"poll_status"` // kode ref_olt_poll_status (join)
	LastPollAt       *time.Time `db:"last_poll_at" json:"last_poll_at"`
	LastPollError    *string    `db:"last_poll_error" json:"last_poll_error"`
	ReportedName     *string    `db:"reported_name" json:"reported_name"`
	ReportedVersion  *string    `db:"reported_version" json:"reported_version"`
	// Penanda keberadaan kredensial (diisi usecase dari kolom terenkripsi), supaya UI bisa menampilkan "tersimpan" tanpa nilainya.
	HasSNMPCommunity bool `db:"-" json:"has_snmp_community"`
	HasSSHPassword   bool `db:"-" json:"has_ssh_password"`
	Audit
}

// OLTFilter — TenantID wajib diisi usecase untuk non-superadmin.
type OLTFilter struct {
	TenantID *uint64
	Search   string // nama atau host (LIKE)
}

// OLTPollResult = hasil satu percobaan koneksi/polling yang dicatat pada baris OLT.
type OLTPollResult struct {
	StatusID uint64
	Error    *string // sudah dibersihkan
	Info     *OLTInfo
}

// DiscoveredONU = satu entri antrean ONU belum terdaftar. ID numerik tidak pernah diekspos; TenantID dari join olts (scope tenant).
type DiscoveredONU struct {
	ID           uint64     `db:"id" json:"-"`
	UUID         string     `db:"onu_uuid" json:"id"`
	OLTID        uint64     `db:"olt_id" json:"-"`
	OLTUUID      string     `db:"olt_uuid" json:"olt_id"`
	OLTName      string     `db:"olt_name" json:"olt_name"`
	TenantID     uint64     `db:"tenant_id" json:"tenant_id"`
	PonPort      string     `db:"pon_port" json:"pon_port"`
	SerialNumber string     `db:"serial_number" json:"serial_number"`
	ONUType      *string    `db:"onu_type" json:"onu_type"`
	StatusID     uint64     `db:"status_id" json:"-"`
	Status       string     `db:"status" json:"status"` // kode ref_onu_discovery_status (join)
	FirstSeenAt  time.Time  `db:"first_seen_at" json:"first_seen_at"`
	LastSeenAt   time.Time  `db:"last_seen_at" json:"last_seen_at"`
	ResolvedAt   *time.Time `db:"resolved_at" json:"resolved_at"`
}

// ONUFilter — TenantID wajib diisi usecase untuk non-superadmin.
type ONUFilter struct {
	TenantID *uint64
	OLTID    *uint64
	StatusID *uint64
	Search   string // serial atau port (LIKE)
}

// OLTRepository — hanya query, tanpa keputusan (rekonsiliasi status diputuskan usecase/olt).
type OLTRepository interface {
	Create(ctx context.Context, o *OLT) error
	GetByUUID(ctx context.Context, uuid string) (*OLT, error)
	List(ctx context.Context, f OLTFilter, p Pagination) ([]OLT, int, error)
	// Update menyimpan field yang dapat diubah operator (nama, driver, host, port, kredensial, catatan, aktif).
	Update(ctx context.Context, o *OLT) error
	// SoftDelete menghapus OLT beserta antrean ONU-nya.
	SoftDelete(ctx context.Context, id, by uint64) error
	// RecordPoll mencatat hasil koneksi: status, waktu, galat, dan (bila ada) identitas yang dilaporkan OLT.
	RecordPoll(ctx context.Context, id uint64, r OLTPollResult) error

	// ListONUsByOLT mengembalikan seluruh antrean aktif sebuah OLT (dibatasi OLTMaxUnregisteredONUs + selisih) untuk rekonsiliasi.
	ListONUsByOLT(ctx context.Context, oltID uint64) ([]DiscoveredONU, error)
	// UpsertONUs menyisipkan/memperbarui entri per (olt_id, serial_number): port, tipe, status, last_seen_at, resolved_at.
	// first_seen_at dan onu_uuid baris yang sudah ada dipertahankan.
	UpsertONUs(ctx context.Context, oltID uint64, rows []DiscoveredONU) error
	// SetONUsStatus mengubah status banyak entri sekaligus; resolvedAt non-nil mengisi resolved_at, nil mengosongkannya.
	// by mengisi updated_by (nil = perubahan sistem, mis. polling).
	SetONUsStatus(ctx context.Context, ids []uint64, statusID uint64, resolvedAt *time.Time, by *uint64) error
	// PruneResolvedONUs menghapus-lunak entri berstatus resolvedStatusID yang lebih lama dari `keep` entri terbaru milik OLT itu,
	// supaya riwayat RESOLVED tidak tumbuh tanpa henti. Mengembalikan jumlah baris.
	PruneResolvedONUs(ctx context.Context, oltID, resolvedStatusID uint64, keep int) (int64, error)
	ListONUs(ctx context.Context, f ONUFilter, p Pagination) ([]DiscoveredONU, int, error)
	GetONUByUUID(ctx context.Context, uuid string) (*DiscoveredONU, error)
}

// ---- Antarmuka driver ----

// OLTConnection = parameter koneksi dengan kredensial PLAINTEXT yang hanya hidup di memori (dibuka dari kolom terenkripsi tepat
// sebelum dipakai). Jangan pernah dilog.
type OLTConnection struct {
	Host          string
	SNMPPort      uint16
	SSHPort       uint16
	SNMPCommunity string
	SSHUsername   string
	SSHPassword   string
}

// OLTInfo = identitas yang dilaporkan OLT saat Probe. Teks dari perangkat tak tepercaya: usecase memangkasnya.
type OLTInfo struct {
	Name    string
	Version string
}

// UnregisteredONU = satu ONU yang dilaporkan OLT sebagai belum terdaftar, apa adanya dari perangkat.
type UnregisteredONU struct {
	PonPort      string
	SerialNumber string
	ONUType      string
}

// OLTDriver = adapter satu keluarga OLT (pola adapter, CLAUDE.md: kuirk vendor di adapter, bukan percabangan tersebar).
// Antarmuka sengaja kecil: metode optik, status port, dan penulisan ditambahkan hanya setelah keluaran nyata dari perangkat
// menentukan bentuknya. Galat harus dibungkus ErrOLTUnreachable/ErrOLTAuthFailed bila relevan; teks perangkat tidak boleh bocor.
type OLTDriver interface {
	Code() string
	Probe(ctx context.Context, c OLTConnection) (OLTInfo, error)
	ListUnregisteredONUs(ctx context.Context, c OLTConnection) ([]UnregisteredONU, error)
}

// OLTDriverRegistry mencari driver berdasarkan kode. Implementasi: internal/oltdriver.
type OLTDriverRegistry interface {
	Get(code string) (OLTDriver, bool)
	Codes() []string
}
