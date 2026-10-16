package main

import (
	"net"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
)

func TestRedactURI(t *testing.T) {
	cases := map[string]string{
		"/api/v1/devices?page=1":              "/api/v1/devices?page=1",
		"/api/v1/ws":                          "/api/v1/ws",
		"/api/v1/ws?token=eyJhbGciOi.abc.def": "/api/v1/ws?token=%2A%2A%2A",
		"/cb?code=XYZ&state=S1&x=1":           "/cb?code=%2A%2A%2A&state=%2A%2A%2A&x=1",
	}
	for in, want := range cases {
		got := redactURI(in)
		if got != want {
			t.Errorf("redactURI(%q) = %q, want %q", in, got, want)
		}
		if strings.Contains(got, "eyJ") || strings.Contains(got, "XYZ") {
			t.Errorf("rahasia bocor: %q", got)
		}
	}
}

func TestClientIPExtractor(t *testing.T) {
	// tanpa proxy tepercaya: XFF diabaikan (tak bisa dipalsukan)
	e := echo.New()
	setClientIPExtractor(e, nil)
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "203.0.113.7:5555"
	r.Header.Set("X-Forwarded-For", "1.2.3.4")
	if got := e.IPExtractor(r); got != "203.0.113.7" {
		t.Errorf("tanpa proxy: got %s, want 203.0.113.7", got)
	}

	// dengan proxy tepercaya 10.0.0.0/8: XFF dipakai hanya bila datang dari proxy
	_, n, _ := net.ParseCIDR("10.0.0.0/8")
	e2 := echo.New()
	setClientIPExtractor(e2, []*net.IPNet{n})
	r2 := httptest.NewRequest("GET", "/", nil)
	r2.RemoteAddr = "10.1.1.1:4000"
	r2.Header.Set("X-Forwarded-For", "198.51.100.9")
	if got := e2.IPExtractor(r2); got != "198.51.100.9" {
		t.Errorf("dari proxy tepercaya: got %s, want 198.51.100.9", got)
	}
	r3 := httptest.NewRequest("GET", "/", nil)
	r3.RemoteAddr = "203.0.113.7:5555" // bukan proxy
	r3.Header.Set("X-Forwarded-For", "1.2.3.4")
	if got := e2.IPExtractor(r3); got != "203.0.113.7" {
		t.Errorf("klien langsung memalsukan XFF: got %s, want 203.0.113.7", got)
	}
}
