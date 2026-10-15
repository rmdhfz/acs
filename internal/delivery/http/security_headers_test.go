package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
)

func serveWithHeaders(t *testing.T, mutate func(*http.Request)) *httptest.ResponseRecorder {
	t.Helper()
	e := echo.New()
	e.Use(SecurityHeaders())
	e.GET("/x", func(c *echo.Context) error { return c.String(http.StatusOK, "ok") })
	e.GET("/err", func(c *echo.Context) error { return echo.NewHTTPError(http.StatusForbidden, "no") })
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	if mutate != nil {
		mutate(req)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func TestSecurityHeaders_SelaluTerpasang(t *testing.T) {
	rec := serveWithHeaders(t, nil)
	want := map[string]string{
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":         "DENY",
		"Referrer-Policy":         "no-referrer",
		"Content-Security-Policy": "default-src 'none'; frame-ancestors 'none'",
		"Cache-Control":           "no-store",
	}
	for k, v := range want {
		if got := rec.Header().Get(k); got != v {
			t.Errorf("%s = %q, ingin %q", k, got, v)
		}
	}
	if hsts := rec.Header().Get("Strict-Transport-Security"); hsts != "" {
		t.Errorf("HSTS tidak boleh dikirim lewat http polos: %q", hsts)
	}
}

func TestSecurityHeaders_HSTSHanyaUntukHTTPS(t *testing.T) {
	rec := serveWithHeaders(t, func(r *http.Request) { r.Header.Set("X-Forwarded-Proto", "https") })
	if hsts := rec.Header().Get("Strict-Transport-Security"); hsts == "" {
		t.Fatal("HSTS harus dikirim bila request https (X-Forwarded-Proto)")
	}
}

// Header harus ikut terpasang pada respons ERROR juga (di sanalah kebocoran
// biasanya terjadi), bukan hanya jalur sukses.
func TestSecurityHeaders_JugaPadaRespons4xx(t *testing.T) {
	e := echo.New()
	e.Use(SecurityHeaders())
	e.GET("/err", func(c *echo.Context) error { return echo.NewHTTPError(http.StatusForbidden, "no") })
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/err", nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status %d", rec.Code)
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("header keamanan hilang pada respons 403")
	}
}
