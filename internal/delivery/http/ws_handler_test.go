package http

import (
	nethttp "net/http"
	"net/http/httptest"
	"testing"
)

func TestBearerFromRequest(t *testing.T) {
	mk := func(hdr map[string][]string) *nethttp.Request {
		r := httptest.NewRequest("GET", "/api/v1/ws", nil)
		for k, v := range hdr {
			for _, x := range v {
				r.Header.Add(k, x)
			}
		}
		return r
	}
	cases := []struct {
		name string
		hdr  map[string][]string
		want string
	}{
		{"header Authorization", map[string][]string{"Authorization": {"Bearer abc.def"}}, "abc.def"},
		{"Authorization bukan Bearer tidak jatuh ke subprotocol", map[string][]string{"Authorization": {"Basic xxx"}, "Upgrade": {"websocket"}, "Sec-Websocket-Protocol": {"acs.bearer, tok"}}, ""},
		{"subprotocol pada upgrade WebSocket", map[string][]string{"Upgrade": {"websocket"}, "Sec-Websocket-Protocol": {"acs.bearer, tok.en-1_x"}}, "tok.en-1_x"},
		{"subprotocol dalam dua header", map[string][]string{"Upgrade": {"websocket"}, "Sec-Websocket-Protocol": {"acs.bearer", "tok"}}, "tok"},
		{"subprotocol TANPA Upgrade diabaikan (REST biasa)", map[string][]string{"Sec-Websocket-Protocol": {"acs.bearer, tok"}}, ""},
		{"penanda tanpa token", map[string][]string{"Upgrade": {"websocket"}, "Sec-Websocket-Protocol": {"acs.bearer"}}, ""},
		{"subprotocol lain", map[string][]string{"Upgrade": {"websocket"}, "Sec-Websocket-Protocol": {"graphql-ws, tok"}}, ""},
		{"kosong", nil, ""},
	}
	for _, c := range cases {
		if got := bearerFromRequest(mk(c.hdr)); got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
}

func TestWSOriginAllowed(t *testing.T) {
	allowed := wsOriginAllowed([]string{"http://10.0.0.5:5173", "http://localhost:5173/"})
	mk := func(origin, host string) *nethttp.Request {
		r := httptest.NewRequest("GET", "http://"+host+"/api/v1/ws", nil)
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		return r
	}
	ok := []struct{ origin, host string }{
		{"", "acs:8080"}, // klien non-peramban
		{"http://10.0.0.5:5173", "10.0.0.5:18080"},    // origin terdaftar
		{"http://localhost:5173", "10.0.0.5:18080"},   // terdaftar (slash akhir dibuang)
		{"http://acs.example.com", "acs.example.com"}, // same-origin lewat proxy
	}
	for _, c := range ok {
		if !allowed(mk(c.origin, c.host)) {
			t.Errorf("harus diizinkan: origin=%q host=%q", c.origin, c.host)
		}
	}
	bad := []struct{ origin, host string }{
		{"http://evil.example", "10.0.0.5:18080"},
		{"http://10.0.0.5:5174", "10.0.0.5:18080"},
		{"null", "10.0.0.5:18080"},
	}
	for _, c := range bad {
		if allowed(mk(c.origin, c.host)) {
			t.Errorf("harus ditolak: origin=%q host=%q", c.origin, c.host)
		}
	}
}
