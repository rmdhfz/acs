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
	InformIntervalParameter string             `json:"inform_interval_parameter"`
	OfflineThresholdSeconds int64              `json:"offline_threshold_seconds"`
	Findings                []DiagnosisFinding `json:"findings"`
	Actions                 []string           `json:"actions"`
	Limits                  string             `json:"limits"`
	GeneratedAt             time.Time          `json:"generated_at"`
}
