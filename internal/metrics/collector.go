// Package metrics berisi custom Prometheus Collector untuk endpoint
// GET /api/v1/metrics (ROADMAP.md Fase 2, TECH.md §10 Observability).
//
// Pendekatan yang dipakai: Collector query DB ON-SCRAPE (dipanggil setiap
// kali Prometheus scrape /metrics), BUKAN background goroutine/ticker yang
// terus-menerus meng-update gauge. Ini konsisten dengan desain app server
// stateless (TECH.md §9) dan menghindari goroutine tambahan yang perlu
// dikelola lifecycle-nya (start/stop bersama graceful shutdown, dsb).
// Trade-off yang disadari: setiap scrape Prometheus = beberapa query DB
// tambahan. Untuk interval scrape wajar (>=15s, lihat deploy/prometheus.yml)
// ini diterima karena semua query di bawah adalah agregasi ringan
// (COUNT/AVG ... GROUP BY dengan index yang sudah ada untuk dashboard
// /devices/stats dan /tasks/stats yang sudah lebih dulu ada).
//
// Catatan layering (CLAUDE.md §"handler hanya panggil usecase"): Collector
// ini BUKAN HTTP handler — dia tidak parsing request/response HTTP, dan
// tidak didaftarkan sebagai route handler biasa (lihat
// internal/delivery/http/metrics_handler.go yang hanya mendelegasikan
// promhttp.Handler ke Echo). Collector adalah adapter infrastruktur
// observability yang memanggil usecase yang SAMA PERSIS dipakai handler REST
// lain (device.Service.Stats, task.Service.Stats/AvgCompletionSeconds/
// ErrorCountsByVendor, session.Service.CountOpenSessions) — TIDAK ADA logic
// bisnis/agregasi baru yang ditulis di sini selain pemetaan ID->kode label
// dan serialisasi ke tipe metric Prometheus, yang memang pekerjaan lapisan
// observability, bukan usecase. Kalau ragu-ragu soal keputusan ini, itu
// pantas didiskusikan lebih lanjut — didokumentasikan eksplisit di sini
// sesuai arahan sesi ini.
package metrics

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"acs/internal/domain"
	"acs/internal/usecase/device"
	"acs/internal/usecase/session"
	"acs/internal/usecase/task"
)

// completionWindow adalah rentang waktu untuk menghitung rata-rata waktu
// penyelesaian task (TECH.md §10). 1 jam dipilih sebagai window "kesehatan
// saat ini" yang cukup representatif tanpa full-scan histori task lama.
const completionWindow = 1 * time.Hour

// scrapeTimeout membatasi durasi seluruh query on-scrape, supaya request
// Prometheus scraper tidak menggantung tanpa batas kalau DB lambat/down.
const scrapeTimeout = 10 * time.Second

// Collector mengimplementasikan prometheus.Collector.
type Collector struct {
	devices  *device.Service
	tasks    *task.Service
	sessions *session.Service
	refs     domain.RefRepository

	devicesByStatus    *prometheus.Desc
	devicesByVendor    *prometheus.Desc
	taskQueueDepth     *prometheus.Desc
	taskCompletionAvg  *prometheus.Desc
	taskErrorsByVendor *prometheus.Desc
	cwmpSessionsOpen   *prometheus.Desc
}

func NewCollector(devices *device.Service, tasks *task.Service, sessions *session.Service, refs domain.RefRepository) *Collector {
	return &Collector{
		devices:  devices,
		tasks:    tasks,
		sessions: sessions,
		refs:     refs,

		devicesByStatus: prometheus.NewDesc(
			"acs_devices_by_status",
			"Jumlah device per status saat ini, lintas seluruh tenant.",
			[]string{"status"}, nil),
		devicesByVendor: prometheus.NewDesc(
			"acs_devices_by_vendor",
			"Jumlah device per vendor saat ini, lintas seluruh tenant.",
			[]string{"vendor"}, nil),
		taskQueueDepth: prometheus.NewDesc(
			"acs_task_queue_depth",
			"Kedalaman antrean task per status saat ini, lintas seluruh tenant.",
			[]string{"status"}, nil),
		// Gauge, bukan histogram/summary Prometheus asli: nilai ini hasil
		// query AVG SQL atas data historis (tasks.completed_at 1 jam
		// terakhir), bukan instrumentasi observasi individual per-request.
		// Histogram/summary cocok untuk mengukur durasi tiap kejadian
		// real-time; di sini yang dibutuhkan cuma satu angka rata-rata dari
		// state DB saat scrape, sehingga gauge jauh lebih sederhana dan
		// jujur merepresentasikan cara datanya didapat.
		taskCompletionAvg: prometheus.NewDesc(
			"acs_task_completion_seconds_avg",
			"Rata-rata waktu penyelesaian task COMPLETED dalam 1 jam terakhir (detik). Tidak ada sample jika belum ada task selesai dalam window tsb.",
			nil, nil),
		// SENGAJA BUKAN diberi suffix "_total" (konvensi Prometheus utk
		// counter monotonic) walau TECH.md §10 menyebutnya "error rate per
		// vendor" -- ini gauge: snapshot "jumlah task berstatus FAILED SAAT
		// INI per vendor" hasil query-on-scrape dari state DB (bisa naik
		// ATAU turun antar-scrape, mis. task di-retry/di-cancel), bukan
		// counter yang terus naik sejak proses start. PromQL tidak
		// type-check gauge vs counter di level nama -- kalau nama ini pakai
		// "_total", siapa pun yang bikin dashboard baru bisa kepancing pakai
		// rate()/increase() dan dapat angka salah tanpa error apa pun
		// (temuan acs-code-reviewer). "_current" dipakai sbg penanda gauge
		// yang jujur di level nama, bukan cuma di komentar/HELP text.
		taskErrorsByVendor: prometheus.NewDesc(
			"acs_task_failed_current",
			"Jumlah task berstatus FAILED SAAT INI per vendor (gauge/snapshot state DB, JANGAN dipakai dgn rate()/increase() -- ini bukan counter monotonic).",
			[]string{"vendor"}, nil),
		cwmpSessionsOpen: prometheus.NewDesc(
			"acs_cwmp_sessions_open",
			"Jumlah sesi CWMP yang masih berstatus OPEN saat ini.",
			nil, nil),
	}
}

func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.devicesByStatus
	ch <- c.devicesByVendor
	ch <- c.taskQueueDepth
	ch <- c.taskCompletionAvg
	ch <- c.taskErrorsByVendor
	ch <- c.cwmpSessionsOpen
}

// Collect dipanggil promhttp setiap kali /metrics di-scrape. Setiap family
// metrik diquery independen — kegagalan satu query (mis. tabel ref_vendors
// belum ke-seed) hanya melewatkan family tsb (di-log ke stderr), tidak
// membuat seluruh scrape gagal.
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), scrapeTimeout)
	defer cancel()

	deviceStatusCodes := c.refCodeMap(ctx, domain.RefTableDeviceStatus)
	taskStatusCodes := c.refCodeMap(ctx, domain.RefTableTaskStatus)
	vendorCodes := c.refCodeMap(ctx, domain.RefTableVendors)

	c.collectDeviceStats(ctx, ch, deviceStatusCodes, vendorCodes)
	c.collectTaskQueueDepth(ctx, ch, taskStatusCodes)
	c.collectTaskCompletionAvg(ctx, ch)
	c.collectTaskErrorsByVendor(ctx, ch, vendorCodes)
	c.collectOpenSessions(ctx, ch)
}

func (c *Collector) collectDeviceStats(ctx context.Context, ch chan<- prometheus.Metric, statusCodes, vendorCodes map[uint64]string) {
	stats, err := c.devices.PlatformStats(ctx)
	if err != nil {
		log.Printf("metrics: device stats gagal: %v", err)
		return
	}
	for _, sc := range stats.ByStatus {
		ch <- prometheus.MustNewConstMetric(c.devicesByStatus, prometheus.GaugeValue,
			float64(sc.Count), labelFromMap(statusCodes, sc.DeviceStatusID))
	}
	for _, vc := range stats.ByVendor {
		ch <- prometheus.MustNewConstMetric(c.devicesByVendor, prometheus.GaugeValue,
			float64(vc.Count), vendorLabel(vendorCodes, vc.VendorID))
	}
}

func (c *Collector) collectTaskQueueDepth(ctx context.Context, ch chan<- prometheus.Metric, statusCodes map[uint64]string) {
	stats, err := c.tasks.PlatformStats(ctx)
	if err != nil {
		log.Printf("metrics: task stats gagal: %v", err)
		return
	}
	for _, sc := range stats {
		ch <- prometheus.MustNewConstMetric(c.taskQueueDepth, prometheus.GaugeValue,
			float64(sc.Count), labelFromMap(statusCodes, sc.TaskStatusID))
	}
}

func (c *Collector) collectTaskCompletionAvg(ctx context.Context, ch chan<- prometheus.Metric) {
	avg, err := c.tasks.AvgCompletionSeconds(ctx, completionWindow)
	if err != nil {
		log.Printf("metrics: avg completion seconds gagal: %v", err)
		return
	}
	if avg == nil {
		// Tidak ada task COMPLETED dalam window — sengaja tidak emit sample
		// (absennya data point di sini bermakna "belum ada data", bukan 0
		// detik yang menyesatkan).
		return
	}
	ch <- prometheus.MustNewConstMetric(c.taskCompletionAvg, prometheus.GaugeValue, *avg)
}

func (c *Collector) collectTaskErrorsByVendor(ctx context.Context, ch chan<- prometheus.Metric, vendorCodes map[uint64]string) {
	counts, err := c.tasks.ErrorCountsByVendor(ctx)
	if err != nil {
		log.Printf("metrics: error counts by vendor gagal: %v", err)
		return
	}
	for _, ec := range counts {
		ch <- prometheus.MustNewConstMetric(c.taskErrorsByVendor, prometheus.GaugeValue,
			float64(ec.Count), vendorLabel(vendorCodes, ec.VendorID))
	}
}

func (c *Collector) collectOpenSessions(ctx context.Context, ch chan<- prometheus.Metric) {
	n, err := c.sessions.CountOpenSessions(ctx)
	if err != nil {
		log.Printf("metrics: count open sessions gagal: %v", err)
		return
	}
	ch <- prometheus.MustNewConstMetric(c.cwmpSessionsOpen, prometheus.GaugeValue, float64(n))
}

// refCodeMap membangun map id->code dari salah satu tabel ref_* (dipakai
// untuk label metrik yang manusiawi, mis. "ONLINE" alih-alih device_status_id
// mentah). Kegagalan load dicatat & mengembalikan map kosong — pemanggil
// akan jatuh ke label "unknown_<id>" lewat labelFromMap.
func (c *Collector) refCodeMap(ctx context.Context, table string) map[uint64]string {
	rows, err := c.refs.List(ctx, table)
	if err != nil {
		log.Printf("metrics: ref list %s gagal: %v", table, err)
		return nil
	}
	m := make(map[uint64]string, len(rows))
	for _, row := range rows {
		m[row.ID] = row.Code
	}
	return m
}

func labelFromMap(m map[uint64]string, id uint64) string {
	if code, ok := m[id]; ok {
		return code
	}
	return fmt.Sprintf("unknown_%d", id)
}

// vendorLabel menangani VendorID nullable (device yang vendor-nya belum
// ter-resolve, mis. OUI tidak dikenal) — beda dari status yang selalu NOT
// NULL di schema.sql.
func vendorLabel(m map[uint64]string, id *uint64) string {
	if id == nil {
		return "unknown"
	}
	return labelFromMap(m, *id)
}
