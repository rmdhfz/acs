package olt

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"acs/internal/domain"
)

type recNotifier struct {
	mu  sync.Mutex
	evs []domain.NotificationEvent
	err error
}

func (r *recNotifier) Notify(_ context.Context, ev domain.NotificationEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.evs = append(r.evs, ev)
	return r.err
}

type whCall struct {
	event  string
	tenant *uint64
	data   map[string]interface{}
}

type recWebhook struct {
	mu    sync.Mutex
	calls []whCall
	err   error
}

func (r *recWebhook) Enqueue(_ context.Context, code string, tenantID *uint64, payload any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	m, _ := payload.(map[string]interface{})
	r.calls = append(r.calls, whCall{code, tenantID, m})
	return r.err
}

func onu(serial, port, state string) domain.ReportedONU {
	return domain.ReportedONU{PonPort: port, SerialNumber: serial, State: state}
}

func pollTwice(t *testing.T, first, second []domain.ReportedONU, wh *recWebhook, rec *recNotifier) {
	t.Helper()
	d := &scriptedInv{states: [][]domain.ReportedONU{first, second}}
	r := newInvRig(t, d)
	r.svc.WithWebhooks(wh).WithNotifier(rec)
	o := mkWith(t, r, d.Code())
	for i := 0; i < 2; i++ {
		if _, err := r.svc.Poll(context.Background(), nocA, o.UUID); err != nil {
			t.Fatalf("poll %d: %v", i+1, err)
		}
		if i == 0 && len(wh.calls)+len(rec.evs) != 0 {
			t.Fatalf("polling pertama (semua ONU baru) tak boleh memberitahu: %+v %+v", wh.calls, rec.evs)
		}
	}
}

// ONU masuk LOS: satu webhook per polling (payload tanpa PK) + satu notifikasi ONU_STATE_ALERT WARNING ke tenant OLT.
func TestPoll_ONUMasukLOS_WebhookDanNotifikasi(t *testing.T) {
	wh, rec := &recWebhook{}, &recNotifier{}
	pollTwice(t,
		[]domain.ReportedONU{onu("ZTEG0001", "1/1/1", domain.OLTStateOnline), onu("ZTEG0002", "1/1/1", domain.OLTStateOnline)},
		[]domain.ReportedONU{onu("ZTEG0001", "1/1/1", domain.OLTStateLOS), onu("ZTEG0002", "1/1/1", domain.OLTStateOnline)},
		wh, rec)

	if len(wh.calls) != 1 {
		t.Fatalf("webhook = %d, mau 1: %+v", len(wh.calls), wh.calls)
	}
	c := wh.calls[0]
	if c.event != domain.WebhookEventONUStateChanged || c.tenant == nil || *c.tenant != 7 {
		t.Fatalf("event/tenant salah: %+v", c)
	}
	if c.data["changed"] != 1 || c.data["truncated"] != false || c.data["olt_name"] != "o-scripted" || c.data["olt_id"] == "" {
		t.Errorf("payload = %+v", c.data)
	}
	if ch, _ := c.data["changes"].([]StateChange); len(ch) != 1 || ch[0] != (StateChange{SerialNumber: "ZTEG0001", PonPort: "1/1/1", From: "ONLINE", To: "LOS"}) {
		t.Errorf("changes = %+v", c.data["changes"])
	}

	if len(rec.evs) != 1 {
		t.Fatalf("notifikasi = %d, mau 1: %+v", len(rec.evs), rec.evs)
	}
	ev := rec.evs[0]
	if ev.TypeCode != domain.NotifONUStateAlert || *ev.TenantID != 7 || ev.SeverityOverride != domain.NotificationSeverityWarning || ev.Link != "/olts?olt="+c.data["olt_id"].(string)+"&problem=1" {
		t.Errorf("notifikasi = %+v", ev)
	}
	if !strings.Contains(ev.Body, "ZTEG0001 (1/1/1, LOS)") || strings.Contains(ev.Body, "ZTEG0002") {
		t.Errorf("isi harus menyebut hanya ONU yang masuk LOS: %q", ev.Body)
	}
}

// Perubahan yang bukan LOS/dying gasp (offline, pulih) tetap dikabarkan lewat webhook tetapi TIDAK menjadi notifikasi in-app.
func TestPoll_PerubahanBiasa_WebhookTanpaNotifikasi(t *testing.T) {
	wh, rec := &recWebhook{}, &recNotifier{}
	pollTwice(t,
		[]domain.ReportedONU{onu("A", "1/1/1", domain.OLTStateOnline), onu("B", "1/1/1", domain.OLTStateLOS)},
		[]domain.ReportedONU{onu("A", "1/1/1", domain.OLTStateOffline), onu("B", "1/1/1", domain.OLTStateOnline)},
		wh, rec)
	if len(wh.calls) != 1 || wh.calls[0].data["changed"] != 2 {
		t.Fatalf("webhook harus tetap satu dengan 2 perubahan: %+v", wh.calls)
	}
	if len(rec.evs) != 0 {
		t.Errorf("offline/pulih bukan LOS/dying gasp, tak boleh jadi notifikasi: %+v", rec.evs)
	}
}

// Webhook memuat maks 50 rincian tetapi jumlah total benar dan `truncated` menyatakan pemotongan; notifikasi menyebut 5 ONU + sisa.
func TestPoll_BanyakPerubahan_DipotongTapiJumlahBenar(t *testing.T) {
	var first, second []domain.ReportedONU
	for i := 0; i < 60; i++ {
		s := fmt.Sprintf("SN%03d", i)
		first = append(first, onu(s, "1/1/1", domain.OLTStateOnline))
		second = append(second, onu(s, "1/1/1", domain.OLTStateLOS))
	}
	wh, rec := &recWebhook{}, &recNotifier{}
	pollTwice(t, first, second, wh, rec)

	c := wh.calls[0]
	ch, _ := c.data["changes"].([]StateChange)
	if c.data["changed"] != 60 || len(ch) != maxStateChangesInEvent || c.data["truncated"] != true {
		t.Fatalf("changed=%v rincian=%d truncated=%v", c.data["changed"], len(ch), c.data["truncated"])
	}
	ev := rec.evs[0]
	if ev.SeverityOverride != domain.NotificationSeverityCritical {
		t.Errorf("60 ONU LOS di satu OLT harus CRITICAL, dapat %q", ev.SeverityOverride)
	}
	if !strings.Contains(ev.Title, "60 ONU") || !strings.Contains(ev.Body, "dan 55 lainnya") {
		t.Errorf("notifikasi = %q / %q", ev.Title, ev.Body)
	}
}

// Galat webhook/notifikasi tak menggagalkan polling; tanpa keduanya (nil) polling tetap berjalan.
func TestPoll_GalatPemberitahuanTidakMenggagalkanPolling(t *testing.T) {
	wh, rec := &recWebhook{err: errors.New("antrean rusak")}, &recNotifier{err: errors.New("notifikasi rusak")}
	pollTwice(t,
		[]domain.ReportedONU{onu("A", "1/1/1", domain.OLTStateOnline)},
		[]domain.ReportedONU{onu("A", "1/1/1", domain.OLTStateDyingGasp)}, wh, rec)
	if len(wh.calls) != 1 || len(rec.evs) != 1 {
		t.Fatalf("keduanya tetap dicoba: %d %d", len(wh.calls), len(rec.evs))
	}

	d := &scriptedInv{states: [][]domain.ReportedONU{{onu("A", "1/1/1", domain.OLTStateOnline)}, {onu("A", "1/1/1", domain.OLTStateLOS)}}}
	r := newInvRig(t, d) // tanpa WithNotifier/WithWebhooks
	o := mkWith(t, r, d.Code())
	for i := 0; i < 2; i++ {
		if _, err := r.svc.Poll(context.Background(), nocA, o.UUID); err != nil {
			t.Fatalf("tanpa notifier/webhook poll %d: %v", i+1, err)
		}
	}
}

func TestOnuStateAlert_AmbangDanIsi(t *testing.T) {
	o := &domain.OLT{TenantID: 3, Name: "OLT-X"}
	mk := func(to string, n int) []StateChange {
		var cs []StateChange
		for i := 0; i < n; i++ {
			cs = append(cs, StateChange{SerialNumber: fmt.Sprintf("S%d", i), PonPort: "1/1/1", From: domain.OLTStateOnline, To: to})
		}
		return cs
	}
	if _, ok := onuStateAlert(o, mk(domain.OLTStateOffline, 4)); ok {
		t.Error("OFFLINE bukan alarm")
	}
	if _, ok := onuStateAlert(o, mk(domain.OLTStateOnline, 4)); ok {
		t.Error("pulih ke ONLINE bukan alarm")
	}
	if _, ok := onuStateAlert(o, mk(domain.OLTStateUnknown, 4)); ok {
		t.Error("UNKNOWN (tak terpetakan) bukan alarm")
	}
	cases := []struct {
		to   string
		n    int
		want string
	}{
		{domain.OLTStateLOS, 1, domain.NotificationSeverityWarning},
		{domain.OLTStateLOS, onuLOSCriticalCount - 1, domain.NotificationSeverityWarning},
		{domain.OLTStateLOS, onuLOSCriticalCount, domain.NotificationSeverityCritical},
		{domain.OLTStateDyingGasp, 50, domain.NotificationSeverityWarning}, // dying gasp massal = listrik padam, bukan putus kabel
	}
	for _, c := range cases {
		ev, ok := onuStateAlert(o, mk(c.to, c.n))
		if !ok || ev.SeverityOverride != c.want || *ev.TenantID != 3 {
			t.Errorf("%s x%d -> ok=%v sev=%q (mau %q)", c.to, c.n, ok, ev.SeverityOverride, c.want)
		}
	}
	// campuran: hanya LOS/dying gasp yang dihitung, dan hanya LOS yang menentukan CRITICAL
	mixed := append(mk(domain.OLTStateOffline, 10), mk(domain.OLTStateDyingGasp, 2)...)
	ev, ok := onuStateAlert(o, mixed)
	if !ok || !strings.HasPrefix(ev.Title, "2 ONU") || ev.SeverityOverride != domain.NotificationSeverityWarning {
		t.Errorf("campuran: ok=%v %+v", ok, ev)
	}
}
