package domain

import "time"

// ---- Diagnosis perangkat: "kenapa offline?" dari bukti yang ACS miliki ----
//
// Hasilnya HIPOTESIS berbukti, bukan vonis: tiap temuan menyebut fakta terukur (Evidence) dan langkah saran (Hint). Penyebab fisik yang
// hanya terlihat dari sisi OLT (LOS, dying gasp, port mati) tidak dapat disimpulkan dari TR-069; itu dinyatakan di DiagnosisLimits.

// Verdict diagnosis.
const (
	VerdictNeverConnected = "NEVER_CONNECTED" // belum pernah Inform
	VerdictReachable      = "REACHABLE"       // diam masih dalam ambang offline perangkat ini
	VerdictOffline        = "OFFLINE"         // diam melewati ambang offline perangkat ini
)

// Keparahan temuan (urut dari paling parah).
const (
	SeverityCritical = "critical"
	SeverityWarning  = "warning"
	SeverityInfo     = "info"
)

// Kode aksi yang disarankan; UI memetakannya ke endpoint yang sudah ada (hak akses tetap ditegakkan endpoint itu).
const (
	DiagActionConnectionRequest  = "CONNECTION_REQUEST" // POST /devices/{id}/connection-request
	DiagActionReadInformInterval = "READ_INFORM_INTERVAL"
	DiagActionOpenOptical        = "OPEN_OPTICAL" // lihat riwayat redaman optik
)

// DiagnosisLimits menyatakan batas diagnosis ini; selalu disertakan agar tidak dibaca sebagai kepastian.
const DiagnosisLimits = "Diagnosis ini hanya memakai bukti dari sisi ACS (TR-069: Inform, event, redaman optik yang dilaporkan perangkat, fault, task). " +
	"Penyebab di jaringan fisik (fiber putus, port PON, LOS/dying gasp, OLT) hanya terlihat dari sisi OLT dan belum tersedia."

// DiagnosisLimitsWithOLT = batas bila data OLT ikut dipakai: penyebab fisik yang DILAPORKAN OLT kini terlihat, tetapi datanya berasal dari polling
// terakhir dan penyebab di luar yang dilaporkan ACS maupun OLT tetap tidak terlihat.
const DiagnosisLimitsWithOLT = "Diagnosis ini memakai bukti dari sisi ACS (TR-069: Inform, event, redaman optik yang dilaporkan perangkat, fault, task) dan data OLT " +
	"dari polling terakhir (keadaan bisa sudah berubah sejak itu). Penyebab di luar yang dilaporkan keduanya tidak terlihat."

// DiagnosisOLT = posisi perangkat di jaringan OLT yang dipakai sebagai bukti; nil pada DeviceDiagnosis bila ONU-nya tidak ditemukan di OLT tenant.
type DiagnosisOLT struct {
	OLTName           string     `json:"olt_name"`
	PonPort           string     `json:"pon_port"`
	ONUIndex          *string    `json:"onu_index"`
	State             string     `json:"state"` // kode ref_olt_onu_state
	VendorState       *string    `json:"vendor_state"`
	RxPowerDBM        *float64   `json:"rx_power_dbm"`
	LastSeenAt        time.Time  `json:"last_seen_at"`
	LastStateChangeAt time.Time  `json:"last_state_change_at"`
	OLTPolledAt       *time.Time `json:"olt_polled_at"`
}

type DiagnosisFinding struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Title    string `json:"title"`
	Evidence string `json:"evidence"` // fakta terukur yang mendasari temuan
	Hint     string `json:"hint"`     // langkah saran
}

type DeviceDiagnosis struct {
	DeviceID uint64 `json:"device_id"`
	Status   string `json:"status"` // kode ref_device_status saat ini
	Verdict  string `json:"verdict"`
	// Summary = satu kalimat dugaan utama (dari temuan terparah) atau pernyataan jujur bila tak ada bukti.
	Summary      string     `json:"summary"`
	LastInformAt *time.Time `json:"last_inform_at"`
	// SilentSeconds = lamanya diam sejak Inform terakhir; nil bila belum pernah Inform.
	SilentSeconds *int64 `json:"silent_seconds"`
	// InformIntervalSeconds = interval Periodic Inform yang dilaporkan perangkat; nil = belum diketahui.
	InformIntervalSeconds *uint32 `json:"inform_interval_seconds"`
	// InformIntervalParameter = nama parameter lengkap untuk membaca interval Inform pada perangkat ini (sesuai model data TR-098/TR-181);
	// kosong bila model datanya belum diketahui. Dipakai UI untuk aksi READ_INFORM_INTERVAL tanpa menebak root.
	InformIntervalParameter string `json:"inform_interval_parameter"`
	OfflineThresholdSeconds int64  `json:"offline_threshold_seconds"`
	// OLT = bukti dari sisi OLT (nil bila tidak ada: tenant tanpa OLT, ONU tak ditemukan, atau inventaris tidak aktif).
	OLT         *DiagnosisOLT      `json:"olt"`
	Findings    []DiagnosisFinding `json:"findings"`
	Actions     []string           `json:"actions"`
	Limits      string             `json:"limits"`
	GeneratedAt time.Time          `json:"generated_at"`
}
