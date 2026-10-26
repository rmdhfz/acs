package http

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"acs/internal/domain"
	"acs/internal/usecase/alerts"

	"github.com/labstack/echo/v5"
)

const testAlertToken = "tok-alertmanager-abcdefghijklmnop-1234"

type recNotifier struct {
	events []domain.NotificationEvent
	err    error
}

func (n *recNotifier) Notify(_ context.Context, ev domain.NotificationEvent) error {
	if n.err != nil {
		return n.err
	}
	n.events = append(n.events, ev)
	return nil
}

func alertRig(token string, n *recNotifier) *echo.Echo {
	r := &Router{Alerts: alerts.NewService(n, nil), AlertmanagerToken: token}
	e := echo.New()
	e.POST("/api/v1/alertmanager-webhook", r.alertmanagerWebhook)
	return e
}

const validPayload = `{
  "receiver": "acsd", "status": "firing", "version": "4", "groupKey": "{}:{alertname=\"ACSTaskQueueBacklogPending\"}",
  "truncatedAlerts": 0, "externalURL": "http://alertmanager:9093", "fieldBaruDariVersiMendatang": {"x": 1},
  "alerts": [{
    "status": "firing", "labels": {"alertname": "ACSTaskQueueBacklogPending", "severity": "warning", "status": "PENDING"},
    "annotations": {"summary": "Backlog task PENDING menumpuk (60 task)", "description": "Lebih dari 50 selama 5 menit."},
    "startsAt": "2026-10-26T08:00:00Z", "endsAt": "0001-01-01T00:00:00Z", "generatorURL": "http://prometheus/graph", "fingerprint": "abc123"
  }]
}`

func post(e *echo.Echo, auth, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/alertmanager-webhook", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func TestAlertmanagerWebhook_Autentikasi(t *testing.T) {
	for name, auth := range map[string]string{
		"tanpa header":         "",
		"skema salah":          "Basic " + testAlertToken,
		"tanpa skema":          testAlertToken,
		"token salah":          "Bearer tok-salah-abcdefghijklmnopqrstuvwxyz",
		"token lebih pendek":   "Bearer tok",
		"token berlebihan":     "Bearer " + strings.Repeat("a", maxBearerToken+1),
		"Bearer tanpa token":   "Bearer ",
		"awalan sama":          "Bearer " + testAlertToken + "x",
		"awalan lebih pendek":  "Bearer " + testAlertToken[:len(testAlertToken)-1],
		"huruf besar/kecil":    "Bearer " + strings.ToUpper(testAlertToken),
		"spasi di akhir token": "Bearer " + testAlertToken + " ",
	} {
		n := &recNotifier{}
		rec := post(alertRig(testAlertToken, n), auth, validPayload)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: HTTP %d, harusnya 401", name, rec.Code)
		}
		if len(n.events) != 0 {
			t.Errorf("%s: tidak boleh ada notifikasi tanpa autentikasi: %d", name, len(n.events))
		}
		if rec.Code == http.StatusUnauthorized && !strings.Contains(rec.Header().Get("WWW-Authenticate"), "Bearer") {
			t.Errorf("%s: 401 harus menyertakan WWW-Authenticate", name)
		}
	}
	// token terkonfigurasi kosong tidak pernah mengizinkan apa pun (rute pun tidak dipasang; ini lapisan kedua)
	n := &recNotifier{}
	if rec := post(alertRig("", n), "Bearer ", validPayload); rec.Code != http.StatusUnauthorized {
		t.Errorf("token kosong: HTTP %d, harusnya 401", rec.Code)
	}
	if rec := post(alertRig("", n), "", validPayload); rec.Code != http.StatusUnauthorized || len(n.events) != 0 {
		t.Errorf("tanpa token terkonfigurasi: HTTP %d events=%d", rec.Code, len(n.events))
	}
}

func TestAlertmanagerWebhook_PayloadSah(t *testing.T) {
	n := &recNotifier{}
	rec := post(alertRig(testAlertToken, n), "Bearer "+testAlertToken, validPayload)
	if rec.Code != http.StatusOK {
		t.Fatalf("HTTP %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"accepted":1`) || !strings.Contains(rec.Body.String(), `"skipped":0`) {
		t.Fatalf("ringkasan: %s", rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("Cache-Control: %q", rec.Header().Get("Cache-Control"))
	}
	if len(n.events) != 1 || n.events[0].TypeCode != domain.NotifPlatformAlert || n.events[0].Title != "[ALERT] Backlog task PENDING menumpuk (60 task)" {
		t.Fatalf("notifikasi: %+v", n.events)
	}
	// field asing (fieldBaruDariVersiMendatang, generatorURL, ...) tidak boleh menggagalkan: tanpa strict-decode
}

func TestAlertmanagerWebhook_MasukanBurukDanGalat(t *testing.T) {
	big := `{"version":"4","alerts":[{"status":"firing","labels":{"alertname":"A","x":"` + strings.Repeat("a", maxAlertmanagerBody) + `"}}]}`
	for name, c := range map[string]struct {
		body string
		want int
	}{
		"bukan JSON":    {"{bukan json", http.StatusBadRequest},
		"kosong":        {"", http.StatusBadRequest},
		"versi lain":    {`{"version":"3","alerts":[{"status":"firing","labels":{"alertname":"A"}}]}`, http.StatusBadRequest},
		"alerts kosong": {`{"version":"4","alerts":[]}`, http.StatusBadRequest},
		"terlalu besar": {big, http.StatusRequestEntityTooLarge},
	} {
		n := &recNotifier{}
		if rec := post(alertRig(testAlertToken, n), "Bearer "+testAlertToken, c.body); rec.Code != c.want {
			t.Errorf("%s: HTTP %d, harusnya %d (%s)", name, rec.Code, c.want, rec.Body.String())
		}
		if len(n.events) != 0 {
			t.Errorf("%s: tidak boleh ada notifikasi: %d", name, len(n.events))
		}
	}

	// galat sistem -> 500 generik tanpa membocorkan penyebab (Alertmanager mencoba ulang)
	n := &recNotifier{err: errors.New("dial tcp 10.9.9.9:3306: password=hunter2")}
	rec := post(alertRig(testAlertToken, n), "Bearer "+testAlertToken, validPayload)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("galat notifikasi: HTTP %d, harusnya 500", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "hunter2") || strings.Contains(rec.Body.String(), "10.9.9.9") {
		t.Fatalf("500 membocorkan penyebab: %s", rec.Body.String())
	}
}
