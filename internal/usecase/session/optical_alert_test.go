package session

import (
	"context"
	"strings"
	"sync"
	"testing"

	"acs/internal/domain"
)

type captureNotifier struct {
	mu  sync.Mutex
	got []domain.NotificationEvent
}

func (c *captureNotifier) Notify(_ context.Context, ev domain.NotificationEvent) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.got = append(c.got, ev)
	return nil
}

type captureWebhook struct{ events []string }

func (c *captureWebhook) Enqueue(_ context.Context, code string, _ *uint64, _ any) error {
	c.events = append(c.events, code)
	return nil
}

func TestAlertOptical_LevelDanKanal(t *testing.T) {
	tenant := uint64(7)
	dev := &domain.Device{ID: 42, TenantID: &tenant, SerialNumber: "ZTEGCD775EE7"}
	cases := []struct {
		name         string
		rx           float64
		wantNotify   bool
		wantSeverity string
		wantWebhook  bool
	}{
		{"normal", -20, false, "", false},
		{"batas waspada masih normal", -25, false, "", false},
		{"waspada: notifikasi saja", -26, true, domain.NotificationSeverityWarning, false},
		{"kritis lemah: notifikasi + webhook", -28.5, true, domain.NotificationSeverityCritical, true},
		{"kritis overload", -5, true, domain.NotificationSeverityCritical, true},
		{"mentah per 0,01 dBm", -2850, true, domain.NotificationSeverityCritical, true},
		{"mentah per 0,001 dBm normal", -23500, false, "", false},
		{"satuan tak jelas: diabaikan", -5000000, false, "", false},
	}
	for _, c := range cases {
		n, w := &captureNotifier{}, &captureWebhook{}
		s := &Service{notifier: n, webhookEnq: w}
		s.alertOptical(context.Background(), dev, c.rx, "X_Optical.RxPower")
		if (len(n.got) == 1) != c.wantNotify || len(n.got) > 1 {
			t.Fatalf("%s: notifikasi=%d, mau %v", c.name, len(n.got), c.wantNotify)
		}
		if (len(w.events) == 1) != c.wantWebhook {
			t.Fatalf("%s: webhook=%v, mau %v", c.name, w.events, c.wantWebhook)
		}
		if !c.wantNotify {
			continue
		}
		ev := n.got[0]
		if ev.TypeCode != domain.NotifOpticalAlert || ev.SeverityOverride != c.wantSeverity || ev.TenantID == nil || *ev.TenantID != tenant {
			t.Fatalf("%s: event salah: %+v", c.name, ev)
		}
		if !strings.Contains(ev.Body, "ZTEGCD775EE7") || ev.Link != "/devices/42" || ev.DedupeWindow == 0 || !strings.HasPrefix(ev.DedupeKey, "optical:42:") {
			t.Fatalf("%s: isi/dedupe salah: %+v", c.name, ev)
		}
	}
}

// Naik level (waspada -> kritis) memakai kunci dedupe berbeda sehingga tetap dikabarkan.
func TestAlertOptical_KunciDedupeBedaPerLevel(t *testing.T) {
	tenant := uint64(7)
	dev := &domain.Device{ID: 1, TenantID: &tenant, SerialNumber: "S"}
	n := &captureNotifier{}
	s := &Service{notifier: n}
	s.alertOptical(context.Background(), dev, -26, "p")
	s.alertOptical(context.Background(), dev, -29, "p")
	if len(n.got) != 2 || n.got[0].DedupeKey == n.got[1].DedupeKey {
		t.Fatalf("kunci dedupe harus beda per level: %+v", n.got)
	}
}

func TestAlertOptical_TanpaNotifierAtauTenantTidakPanik(t *testing.T) {
	tenant := uint64(7)
	(&Service{}).alertOptical(context.Background(), &domain.Device{ID: 1, TenantID: &tenant}, -30, "p")
	n := &captureNotifier{}
	(&Service{notifier: n}).alertOptical(context.Background(), &domain.Device{ID: 1}, -30, "p") // tanpa tenant
	if len(n.got) != 0 {
		t.Fatal("perangkat tanpa tenant tidak boleh memicu notifikasi")
	}
}
