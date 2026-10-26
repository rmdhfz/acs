package alerts

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"acs/internal/domain"
)

type fakeNotifier struct {
	events []domain.NotificationEvent
	failOn func(domain.NotificationEvent) bool
}

func (f *fakeNotifier) Notify(_ context.Context, ev domain.NotificationEvent) error {
	if f.failOn != nil && f.failOn(ev) {
		return errors.New("db mati")
	}
	f.events = append(f.events, ev)
	return nil
}

func firing(labels map[string]string, ann map[string]string) Alert {
	return Alert{Status: "firing", Labels: labels, Annotations: ann, StartsAt: "2026-10-26T08:00:00Z"}
}

func ingest(t *testing.T, n *fakeNotifier, alerts ...Alert) (Result, error) {
	t.Helper()
	return NewService(n, nil).Ingest(context.Background(), Payload{Version: "4", Alerts: alerts})
}

func TestIngest_PayloadTidakValidDitolakTanpaNotifikasi(t *testing.T) {
	for name, p := range map[string]Payload{
		"versi kosong":   {Alerts: []Alert{firing(map[string]string{"alertname": "A"}, nil)}},
		"versi lain":     {Version: "3", Alerts: []Alert{firing(map[string]string{"alertname": "A"}, nil)}},
		"alerts kosong":  {Version: "4"},
		"alerts nil sah": {Version: "4", Alerts: []Alert{}},
	} {
		n := &fakeNotifier{}
		if _, err := NewService(n, nil).Ingest(context.Background(), p); !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("%s: harus ErrInvalidInput: %v", name, err)
		}
		if len(n.events) != 0 {
			t.Errorf("%s: tidak boleh ada notifikasi: %d", name, len(n.events))
		}
	}
}

func TestIngest_FiringDanResolved(t *testing.T) {
	n := &fakeNotifier{}
	labels := map[string]string{"alertname": "ACSTaskErrorRateByVendor", "severity": "warning", "vendor": "ZTE", "job": "acsd"}
	ann := map[string]string{"summary": "Task FAILED tinggi untuk vendor ZTE (12 task)", "description": "Lebih dari 10 selama 5 menit."}
	resolved := firing(labels, ann)
	resolved.Status, resolved.EndsAt = "resolved", "2026-10-26T09:00:00Z"

	res, err := ingest(t, n, firing(labels, ann), resolved)
	if err != nil || res.Accepted != 2 || res.Skipped != 0 {
		t.Fatalf("hasil: %+v %v", res, err)
	}
	f, r := n.events[0], n.events[1]

	if f.TypeCode != domain.NotifPlatformAlert || f.Audience.Kind != domain.AudienceDefault {
		t.Fatalf("jenis/audiens: %+v", f)
	}
	if f.Title != "[ALERT] Task FAILED tinggi untuk vendor ZTE (12 task)" || f.SeverityOverride != domain.NotificationSeverityWarning || f.DedupeWindow != 6*time.Hour {
		t.Fatalf("firing: %+v", f)
	}
	for _, want := range []string{"Lebih dari 10 selama 5 menit.", "Label: job=acsd, vendor=ZTE", "Mulai: 2026-10-26T08:00:00Z"} {
		if !strings.Contains(f.Body, want) {
			t.Fatalf("body firing tanpa %q: %q", want, f.Body)
		}
	}
	if strings.Contains(f.Body, "alertname") || strings.Contains(f.Body, "severity") || strings.Contains(f.Body, "Selesai") {
		t.Fatalf("body firing memuat yang tak seharusnya: %q", f.Body)
	}
	if r.Title != "[PULIH] Task FAILED tinggi untuk vendor ZTE (12 task)" || r.SeverityOverride != domain.NotificationSeverityInfo || r.DedupeWindow != time.Hour || !strings.Contains(r.Body, "Selesai: 2026-10-26T09:00:00Z") {
		t.Fatalf("resolved: %+v", r)
	}
	if f.DedupeKey == r.DedupeKey || !strings.HasSuffix(f.DedupeKey, ":firing") || !strings.HasSuffix(r.DedupeKey, ":resolved") {
		t.Fatalf("kunci dedupe firing vs resolved harus beda: %q %q", f.DedupeKey, r.DedupeKey)
	}
	if f.Link != "/dashboard" || f.EntityType != "alert" || f.EntityID != nil {
		t.Fatalf("tautan/entitas: %+v", f)
	}
}

func TestIngest_PemetaanKeparahan(t *testing.T) {
	for label, want := range map[string]string{
		"critical": domain.NotificationSeverityCritical, "CRITICAL": domain.NotificationSeverityCritical, "page": domain.NotificationSeverityCritical,
		"warning": domain.NotificationSeverityWarning, "": domain.NotificationSeverityWarning, "aneh": domain.NotificationSeverityWarning,
		"info": domain.NotificationSeverityInfo, "none": domain.NotificationSeverityInfo,
	} {
		n := &fakeNotifier{}
		if _, err := ingest(t, n, firing(map[string]string{"alertname": "A", "severity": label}, nil)); err != nil {
			t.Fatal(err)
		}
		if got := n.events[0].SeverityOverride; got != want {
			t.Errorf("severity %q -> %s, harapan %s", label, got, want)
		}
	}
}

// Kunci dedupe stabil untuk alert yang sama, berbeda bila label berbeda (vendor lain = masalah lain), dan tidak bergantung urutan.
func TestIngest_KunciDedupe(t *testing.T) {
	n := &fakeNotifier{}
	a := map[string]string{"alertname": "A", "vendor": "ZTE", "job": "acsd"}
	same := map[string]string{"job": "acsd", "vendor": "ZTE", "alertname": "A"}
	other := map[string]string{"alertname": "A", "vendor": "HUAWEI", "job": "acsd"}
	diffName := map[string]string{"alertname": "B", "vendor": "ZTE", "job": "acsd"}
	if _, err := ingest(t, n, firing(a, nil), firing(same, nil), firing(other, nil), firing(diffName, nil)); err != nil {
		t.Fatal(err)
	}
	k := []string{n.events[0].DedupeKey, n.events[1].DedupeKey, n.events[2].DedupeKey, n.events[3].DedupeKey}
	if k[0] != k[1] {
		t.Fatalf("alert yang sama harus berkunci sama: %q %q", k[0], k[1])
	}
	if k[0] == k[2] || k[0] == k[3] || k[2] == k[3] {
		t.Fatalf("label/nama berbeda harus berkunci beda: %v", k)
	}
	for _, key := range k {
		if len(key) > 40 {
			t.Fatalf("kunci dedupe terlalu panjang: %q", key)
		}
	}
}

func TestIngest_AlertTidakValidDilewati(t *testing.T) {
	n := &fakeNotifier{}
	ok := firing(map[string]string{"alertname": "OK"}, nil)
	res, err := ingest(t, n,
		ok,
		firing(map[string]string{"severity": "warning"}, nil),                     // tanpa alertname
		firing(map[string]string{"alertname": "  \n "}, nil),                      // alertname hanya spasi
		Alert{Status: "pending", Labels: map[string]string{"alertname": "X"}},     // status tak dikenal
		Alert{Status: "", Labels: map[string]string{"alertname": "X"}},            // status kosong
		Alert{Status: "FIRING ", Labels: map[string]string{"alertname": "Huruf"}}, // dinormalkan, sah
	)
	if err != nil || res.Accepted != 2 || res.Skipped != 4 {
		t.Fatalf("hasil: %+v %v", res, err)
	}
	if n.events[1].Title != "[ALERT] Huruf" {
		t.Fatalf("summary kosong harus jatuh ke alertname: %q", n.events[1].Title)
	}
}

func TestIngest_BatasJumlahPerPermintaan(t *testing.T) {
	n := &fakeNotifier{}
	var as []Alert
	for i := 0; i < MaxAlertsPerRequest+50; i++ {
		as = append(as, firing(map[string]string{"alertname": fmt.Sprintf("A%d", i)}, nil))
	}
	res, err := ingest(t, n, as...)
	if err != nil || res.Accepted != MaxAlertsPerRequest || res.Skipped != 50 {
		t.Fatalf("hasil: %+v %v", res, err)
	}
	if len(n.events) != MaxAlertsPerRequest {
		t.Fatalf("notifikasi: %d", len(n.events))
	}
}

// Teks alert memuat nilai label dari database: karakter kontrol/baris baru dibuang (judul dan email), nilai dipangkas, label dibatasi.
func TestIngest_TeksDibersihkan(t *testing.T) {
	n := &fakeNotifier{}
	labels := map[string]string{"alertname": "A\x00B\nC", "vendor": "ZTE\r\nBcc: x@y"}
	for i := 0; i < 12; i++ {
		labels[fmt.Sprintf("l%02d", i)] = strings.Repeat("v", 200)
	}
	ann := map[string]string{"summary": "Judul\nbaris dua\t\x07 " + strings.Repeat("s", 500), "description": "Baris 1\nBaris 2‮ terbalik"}
	if _, err := ingest(t, n, firing(labels, ann)); err != nil {
		t.Fatal(err)
	}
	ev := n.events[0]
	for _, s := range []string{ev.Title, ev.Body} {
		if strings.ContainsAny(strings.ReplaceAll(s, "\n", ""), "\x00\r\t\x07‮") || !utf8.ValidString(s) {
			t.Fatalf("karakter kontrol lolos: %q", s)
		}
	}
	if strings.Contains(ev.Title, "\n") {
		t.Fatalf("judul tidak boleh multibaris: %q", ev.Title)
	}
	if utf8.RuneCountInString(ev.Title) > len("[ALERT] ")+maxSummaryLen {
		t.Fatalf("judul tidak dipangkas: %d", utf8.RuneCountInString(ev.Title))
	}
	line := ""
	for _, l := range strings.Split(ev.Body, "\n") {
		if strings.HasPrefix(l, "Label: ") {
			line = l
		}
	}
	if strings.Count(line, "=") != maxLabelsShown || !strings.HasSuffix(line, "…") {
		t.Fatalf("label harus dibatasi %d + elipsis: %q", maxLabelsShown, line)
	}
	if strings.Contains(line, strings.Repeat("v", maxLabelValue+1)) {
		t.Fatalf("nilai label tidak dipangkas: %q", line)
	}
	if strings.Contains(ev.Body, "Bcc:") && strings.Contains(ev.Body, "\r\nBcc:") {
		t.Fatalf("injeksi header lolos: %q", ev.Body)
	}
}

// Notifikasi yang gagal dibuat membuat Ingest mengembalikan galat (Alertmanager mencoba ulang); yang berhasil tetap tercatat dan
// pengulangannya tidak menggandakan karena dedupe di usecase notifikasi.
func TestIngest_GalatNotifikasiDikembalikan(t *testing.T) {
	n := &fakeNotifier{failOn: func(ev domain.NotificationEvent) bool { return strings.Contains(ev.Title, "RUSAK") }}
	res, err := ingest(t, n,
		firing(map[string]string{"alertname": "BAIK"}, map[string]string{"summary": "baik"}),
		firing(map[string]string{"alertname": "RUSAK"}, map[string]string{"summary": "RUSAK"}),
	)
	if err == nil {
		t.Fatal("harus galat agar Alertmanager mencoba ulang")
	}
	if res.Accepted != 1 || len(n.events) != 1 {
		t.Fatalf("yang berhasil tetap diteruskan: %+v events=%d", res, len(n.events))
	}
	if errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("galat sistem tidak boleh tampak sebagai masukan salah: %v", err)
	}
}
