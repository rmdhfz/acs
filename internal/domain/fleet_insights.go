package domain

import (
	"context"
	"time"
)

// ---- Wawasan armada untuk dashboard (distribusi mode PON, uptime, redaman RX, suhu ONU, lama offline) ----

// Ambang suhu ONU (°C). Satu sumber untuk dashboard; sesuaikan bila kebijakan jaringan berbeda.
const (
	TempWarnAtC     = 60.0 // >= ini = peringatan
	TempHighAtC     = 70.0 // >= ini = tinggi
	TempCriticalAtC = 80.0 // >= ini = kritis
)

// Kunci keranjang (bucket) distribusi — dipetakan ke label & warna di Console.
const (
	InsightUnknown = "unknown"

	PonGPON = "gpon"
	PonEPON = "epon"

	UptimeLT1D   = "lt1d"
	Uptime1to7D  = "d1_7"
	Uptime7to30D = "d7_30"
	UptimeGT30D  = "gt30d"

	OpticalOverload = "overload" // RX > -8 dBm (receiver overload); lemah/kritis = OpticalBad

	TempNormal   = "normal"
	TempWarning  = "warning"
	TempHigh     = "high"
	TempCritical = "critical"

	OfflineLT1D   = "lt1d"
	Offline1to3D  = "d1_3"
	Offline3to7D  = "d3_7"
	Offline7to30D = "d7_30"
	OfflineGT30D  = "gt30d"
)

// BucketCount = jumlah perangkat pada satu keranjang distribusi.
type BucketCount struct {
	Key   string `json:"key"`
	Count int    `json:"count"`
}

// FleetInsights = distribusi armada (tenant actor; SUPERADMIN tanpa filter = seluruh platform).
// Redaman, suhu, dan uptime dihitung dari perangkat ONLINE; lama offline dari perangkat OFFLINE.
// Urutan keranjang tetap (yang tak berisi tetap ada dengan count 0) agar grafik stabil.
type FleetInsights struct {
	Online      int           `json:"online"`
	Offline     int           `json:"offline"`
	Other       int           `json:"other"` // status lain (belum terdaftar, bermasalah, dst.)
	PonMode     []BucketCount `json:"pon_mode"`
	Uptime      []BucketCount `json:"uptime"`
	OpticalRX   []BucketCount `json:"optical_rx"`
	Temperature []BucketCount `json:"temperature"`
	OfflineAge  []BucketCount `json:"offline_age"`
}

// FleetRow = satu perangkat mentah untuk dihitung di usecase (repository hanya query, tanpa keputusan).
type FleetRow struct {
	Status       string     `db:"status"` // kode ref_device_status
	LastInformAt *time.Time `db:"last_inform_at"`
	RxDBM        *float64   `db:"rx"`
	TempC        *float64   `db:"temp"`
	UptimeRaw    *string    `db:"uptime"` // detik, apa adanya dari parameter DeviceInfo.UpTime
	Gpon         bool       `db:"gpon"`
	Epon         bool       `db:"epon"`
}

// FleetInsightsRepository — pembacaan baris mentah per perangkat (tenantID nil = lintas tenant).
type FleetInsightsRepository interface {
	FleetRows(ctx context.Context, tenantID *uint64) ([]FleetRow, error)
}
