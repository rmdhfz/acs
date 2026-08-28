package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// InformResponseLatency mengukur durasi dari Inform diterima ACS sampai
// InformResponse selesai ditulis balik ke CPE (TECH.md §10 — observability
// per sesi CWMP), diinstrumentasi REAL-TIME sbg histogram Prometheus biasa
// -- BEDA dari pola "gauge query-on-scrape" yang dipakai Collector di
// collector.go (taskCompletionAvg dkk).
//
// Kenapa beda pola, bukan cuma preferensi: metrik lain di collector.go
// dihitung dari PASANGAN timestamp yang SUDAH tersimpan di DB utk keperluan
// bisnis lain (mis. tasks.created_at & tasks.completed_at, dipakai jauh di
// luar observability) -- query ulang saat scrape itu murah & konsisten dgn
// desain app server stateless (TECH.md §9), tanpa perlu state proses sama
// sekali. Inform -> InformResponse TIDAK py pasangan timestamp yang
// dipersist hari ini: device_sessions.started_at diisi SAAT SESI DIBUAT
// (sebelum body Inform selesai diproses sepenuhnya, lihat
// usecase/session/service.go#resolveSession), BUKAN "kapan InformResponse
// benar-benar dikirim" -- utk itu butuh kolom baru (mis.
// device_sessions.inform_response_sent_at) + migrasi skema, yang SENGAJA
// TIDAK dilakukan di sesi perubahan ini (di luar cakupan/berpotensi bentrok
// dgn perubahan skema paralel lain yang sedang berjalan terhadap
// schema.sql/migrations/). Histogram real-time di request path adalah
// pendekatan paling jujur yang TIDAK butuh perubahan skema sama sekali --
// BUKAN indikasi bahwa histogram "lebih baik" drpd pola gauge Collector,
// murni konsekuensi keterbatasan data yang dipersist saat ini.
//
// Trade-off yang disadari: nilai histogram ini reset tiap kali proses acsd
// restart (state in-memory client_golang biasa), beda dgn gauge Collector
// yang selalu merepresentasikan state DB terkini apa pun kondisi proses.
// Utk deployment multi-instance di belakang load balancer, tiap instance
// py histogram terpisah (Prometheus federation/sum across instances via
// PromQL scrape per-instance, pola standar Prometheus utk metrik demikian
// -- BUKAN kekurangan desain, ini karakteristik normal histogram
// real-time). Kandidat migrasi lanjutan bila suatu saat dibutuhkan agregat
// historis yang query-ulang-dari-DB (survive restart/lintas-instance
// seragam spt metrik lain): tambah kolom
// device_sessions.inform_response_sent_at, lalu pindahkan metrik ini ke
// pola Collector spt metrik lainnya.
var InformResponseLatency = prometheus.NewHistogram(prometheus.HistogramOpts{
	Name: "acs_cwmp_inform_response_latency_seconds",
	Help: "Durasi dari Inform diterima sampai InformResponse dikirim balik ke CPE (detik). " +
		"Histogram real-time per-request (BUKAN query-on-scrape spt metrik lain di package ini, lihat komentar source).",
	Buckets: prometheus.DefBuckets,
})

// ObserveInformResponseLatency mencatat satu durasi Inform->InformResponse.
// Dipanggil delivery/cwmp/handler.go setelah InformResponse berhasil ditulis
// (durasi kegagalan/Unauthorized SENGAJA tidak diobservasi -- metrik ini
// mengukur latensi siklus Inform->InformResponse yang BERHASIL, bukan durasi
// request gagal).
func ObserveInformResponseLatency(d time.Duration) {
	InformResponseLatency.Observe(d.Seconds())
}
