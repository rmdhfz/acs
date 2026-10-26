// Package alerts menerima alert dari Alertmanager (payload webhook versi 4) dan mengubahnya menjadi notifikasi PLATFORM_ALERT
// untuk operator platform (SUPERADMIN): in-app, push WebSocket, dan email lewat usecase/notification. Sebelumnya Alertmanager
// diarahkan ke endpoint yang tidak ada, sehingga alert tidak pernah sampai ke siapa pun.
//
// Teks alert bersumber dari aturan Prometheus tetapi memuat nilai label metrik (mis. kode vendor dari database), jadi diperlakukan
// sebagai tak tepercaya: dipangkas, dibersihkan, dan jumlahnya dibatasi per permintaan.
package alerts

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"acs/internal/domain"
)

const (
	// MaxAlertsPerRequest membatasi alert yang diproses per permintaan (Alertmanager sendiri memotong lewat max_alerts).
	MaxAlertsPerRequest = 100
	// firingDedupe: alert yang masih firing diingatkan ulang Alertmanager tiap repeat_interval; satu notifikasi per jendela ini
	// cukup, sisanya hanya pengulangan.
	firingDedupe = 6 * time.Hour
	// resolvedDedupe: kabar "pulih" cukup sekali per jendela ini.
	resolvedDedupe = time.Hour

	maxLabelsShown  = 8
	maxLabelValue   = 64
	maxNameLen      = 128
	maxSummaryLen   = 200
	maxDescription  = 600
	maxTimestampLen = 40
)

// Alert = satu alert pada payload webhook Alertmanager (hanya field yang dipakai).
type Alert struct {
	Status      string            `json:"status"`
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	StartsAt    string            `json:"startsAt"`
	EndsAt      string            `json:"endsAt"`
}

// Payload = badan webhook Alertmanager (https://prometheus.io/docs/alerting/latest/configuration/#webhook_config).
type Payload struct {
	Version string  `json:"version"`
	Alerts  []Alert `json:"alerts"`
}

// Result = ringkasan satu permintaan.
type Result struct {
	Accepted int `json:"accepted"` // diteruskan ke notifikasi (termasuk yang ditahan dedupe)
	Skipped  int `json:"skipped"`  // tidak valid atau melebihi batas
}

type Service struct {
	notifier domain.Notifier
	log      *slog.Logger
}

func NewService(n domain.Notifier, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{notifier: n, log: log}
}

// Ingest memproses satu payload. Galat dikembalikan bila ada notifikasi yang gagal dibuat: Alertmanager mencoba ulang payload yang
// sama, dan notifikasi yang sudah berhasil tidak terduplikasi karena dedupe.
func (s *Service) Ingest(ctx context.Context, p Payload) (Result, error) {
	if p.Version != "4" {
		return Result{}, fmt.Errorf("%w: versi payload Alertmanager %q tidak didukung (harus \"4\")", domain.ErrInvalidInput, truncate(p.Version, 16))
	}
	if len(p.Alerts) == 0 {
		return Result{}, fmt.Errorf("%w: alerts kosong", domain.ErrInvalidInput)
	}
	var res Result
	failed := 0
	for i, a := range p.Alerts {
		if i >= MaxAlertsPerRequest {
			res.Skipped += len(p.Alerts) - i
			break
		}
		ev, ok := buildEvent(a)
		if !ok {
			res.Skipped++
			continue
		}
		if err := s.notifier.Notify(ctx, ev); err != nil {
			failed++
			s.log.Warn("alerts: gagal membuat notifikasi", "alertname", clean(a.Labels["alertname"], maxNameLen), "error", err)
			continue
		}
		res.Accepted++
	}
	if failed > 0 {
		return res, fmt.Errorf("alerts: %d dari %d notifikasi gagal dibuat", failed, res.Accepted+failed)
	}
	return res, nil
}

// buildEvent mengubah satu alert menjadi NotificationEvent; ok=false bila alert tidak valid (tanpa alertname, status tak dikenal).
func buildEvent(a Alert) (domain.NotificationEvent, bool) {
	status := strings.ToLower(strings.TrimSpace(a.Status))
	if status != "firing" && status != "resolved" {
		return domain.NotificationEvent{}, false
	}
	name := clean(a.Labels["alertname"], maxNameLen)
	if name == "" {
		return domain.NotificationEvent{}, false
	}

	summary := clean(a.Annotations["summary"], maxSummaryLen)
	if summary == "" {
		summary = name
	}
	title, severity, window := "[ALERT] "+summary, severityFor(a.Labels["severity"]), firingDedupe
	if status == "resolved" {
		title, severity, window = "[PULIH] "+summary, domain.NotificationSeverityInfo, resolvedDedupe
	}

	var b strings.Builder
	if d := clean(a.Annotations["description"], maxDescription); d != "" {
		b.WriteString(d)
		b.WriteString("\n")
	}
	if l := labelLine(a.Labels); l != "" {
		b.WriteString("Label: " + l + "\n")
	}
	if t := clean(a.StartsAt, maxTimestampLen); t != "" {
		b.WriteString("Mulai: " + t + "\n")
	}
	if status == "resolved" {
		if t := clean(a.EndsAt, maxTimestampLen); t != "" {
			b.WriteString("Selesai: " + t + "\n")
		}
	}
	body := strings.TrimSpace(b.String())
	if body == "" {
		body = summary
	}

	return domain.NotificationEvent{
		TypeCode:         domain.NotifPlatformAlert, // audiens bawaan: SUPERADMIN
		Title:            title,
		Body:             body,
		Link:             "/dashboard",
		EntityType:       "alert", // jenis entitas saja; alert tidak punya baris database untuk dirujuk
		SeverityOverride: severity,
		DedupeKey:        "alert:" + fingerprint(name, a.Labels) + ":" + status,
		DedupeWindow:     window,
	}, true
}

// severityFor memetakan label severity Prometheus ke keparahan notifikasi; yang tak dikenal dianggap WARNING.
func severityFor(label string) string {
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "critical", "page", "error":
		return domain.NotificationSeverityCritical
	case "info", "none":
		return domain.NotificationSeverityInfo
	}
	return domain.NotificationSeverityWarning
}

// fingerprint = pengenal stabil (alertname + label terurut) untuk dedupe, dibatasi 16 heksadesimal. Sengaja tidak memakai fingerprint
// bawaan Alertmanager: itu bergantung pada versi, sedangkan kunci ini bergantung hanya pada isi alert.
func fingerprint(name string, labels map[string]string) string {
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	h := sha256.New()
	h.Write([]byte(name))
	for _, k := range keys {
		h.Write([]byte{0})
		h.Write([]byte(k))
		h.Write([]byte{0})
		h.Write([]byte(labels[k]))
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// labelLine menampilkan label yang informatif (tanpa alertname/severity yang sudah ada di judul/keparahan), terurut, dan dibatasi.
func labelLine(labels map[string]string) string {
	keys := make([]string, 0, len(labels))
	for k := range labels {
		if k != "alertname" && k != "severity" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		if len(parts) == maxLabelsShown {
			parts = append(parts, "…")
			break
		}
		parts = append(parts, clean(k, 32)+"="+clean(labels[k], maxLabelValue))
	}
	return strings.Join(parts, ", ")
}

// clean membuang karakter kontrol/format (termasuk baris baru: teks ini masuk judul dan email), memangkas spasi, dan membatasi
// panjang dalam karakter.
func clean(s string, max int) string {
	if !utf8.ValidString(s) {
		s = strings.ToValidUTF8(s, "")
	}
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return ' '
		}
		return r
	}, s)
	return truncate(strings.Join(strings.Fields(s), " "), max)
}

func truncate(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	return string([]rune(s)[:max])
}
