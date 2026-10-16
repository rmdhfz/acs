package http

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
)

func serveHealth(t *testing.T, checks []ReadinessCheck, path string) (int, string) {
	t.Helper()
	e := echo.New()
	r := &Router{Readiness: checks}
	e.GET("/healthz", r.healthz)
	e.GET("/readyz", r.readyz)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec.Code, rec.Body.String()
}

func TestHealthz_SelaluOkTanpaDependensi(t *testing.T) {
	code, body := serveHealth(t, []ReadinessCheck{{Name: "database", Critical: true, Fn: func(context.Context) error { return errors.New("db mati") }}}, "/healthz")
	if code != 200 || !strings.Contains(body, `"ok"`) {
		t.Fatalf("healthz harus 200 walau DB mati: %d %s", code, body)
	}
}

func TestReadyz(t *testing.T) {
	ok := func(context.Context) error { return nil }
	bad := func(context.Context) error { return errors.New("dial tcp 10.0.0.5:3306: secret-host refused") }

	if code, body := serveHealth(t, []ReadinessCheck{{"database", true, ok}, {"redis", false, ok}}, "/readyz"); code != 200 || !strings.Contains(body, `"ready"`) {
		t.Fatalf("semua sehat: %d %s", code, body)
	}
	// dependensi kritis mati -> 503
	code, body := serveHealth(t, []ReadinessCheck{{"database", true, bad}, {"redis", false, ok}}, "/readyz")
	if code != 503 || !strings.Contains(body, `"not_ready"`) || !strings.Contains(body, `"database":"down"`) {
		t.Fatalf("DB mati harus 503: %d %s", code, body)
	}
	// teks error (host/port internal) TIDAK boleh bocor
	if strings.Contains(body, "10.0.0.5") || strings.Contains(body, "secret-host") {
		t.Fatalf("readyz membocorkan detail error: %s", body)
	}
	// dependensi opsional mati -> tetap siap, tetapi dilaporkan
	code, body = serveHealth(t, []ReadinessCheck{{"database", true, ok}, {"redis", false, bad}}, "/readyz")
	if code != 200 || !strings.Contains(body, `"redis":"down"`) {
		t.Fatalf("redis opsional mati harus tetap 200 + dilaporkan: %d %s", code, body)
	}
}

func TestReadyz_TanpaCheckDianggapSiap(t *testing.T) {
	if code, _ := serveHealth(t, nil, "/readyz"); code != 200 {
		t.Fatalf("tanpa check: %d", code)
	}
}
