package http

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"acs/internal/domain"
)

func newCtx(method, target string, cookies ...*http.Cookie) (*echo.Context, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(method, target, nil)
	for _, ck := range cookies {
		req.AddCookie(ck)
	}
	rec := httptest.NewRecorder()
	return echo.New().NewContext(req, rec), rec
}

func cookiesByName(rec *httptest.ResponseRecorder) map[string]*http.Cookie {
	m := map[string]*http.Cookie{}
	for _, ck := range rec.Result().Cookies() {
		m[ck.Name] = ck
	}
	return m
}

func TestRandomToken_AcakDanPanjang(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		tok, err := randomToken()
		if err != nil {
			t.Fatal(err)
		}
		if len(tok) != 43 { // 32 byte -> base64url tanpa padding
			t.Fatalf("panjang token %d, ingin 43 (256 bit)", len(tok))
		}
		if seen[tok] {
			t.Fatal("token berulang — bukan acak")
		}
		seen[tok] = true
	}
}

func TestStartOIDCFlow_CookieAmanDanTerikat(t *testing.T) {
	c, rec := newCtx(http.MethodGet, "/api/v1/auth/oidc/login")
	state, nonce, err := startOIDCFlow(c)
	if err != nil {
		t.Fatal(err)
	}
	if state == nonce || state == "" || nonce == "" {
		t.Fatalf("state dan nonce harus acak & berbeda: %q %q", state, nonce)
	}
	if state == "random-state" {
		t.Fatal("state tidak boleh konstan (regresi TODO lama)")
	}
	cks := cookiesByName(rec)
	for name, want := range map[string]string{oidcStateCookie: state, oidcNonceCookie: nonce} {
		ck := cks[name]
		if ck == nil {
			t.Fatalf("cookie %s tidak dipasang", name)
		}
		if ck.Value != want || !ck.HttpOnly || ck.SameSite != http.SameSiteLaxMode ||
			ck.Path != oidcCookiePath || ck.MaxAge != oidcCookieTTL {
			t.Fatalf("cookie %s salah atribut: %+v", name, ck)
		}
		if ck.Secure {
			t.Fatalf("cookie %s Secure padahal request bukan https", name)
		}
	}
}

func TestStartOIDCFlow_SecureBilaHTTPS(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oidc/login", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()
	if _, _, err := startOIDCFlow(echo.New().NewContext(req, rec)); err != nil {
		t.Fatal(err)
	}
	for _, ck := range rec.Result().Cookies() {
		if !ck.Secure {
			t.Fatalf("cookie %s harus Secure di belakang https", ck.Name)
		}
	}
}

func TestFinishOIDCFlow(t *testing.T) {
	good := func(v string) *http.Cookie { return &http.Cookie{Name: oidcStateCookie, Value: v} }
	nonce := func(v string) *http.Cookie { return &http.Cookie{Name: oidcNonceCookie, Value: v} }

	tests := []struct {
		name    string
		url     string
		cookies []*http.Cookie
		wantOK  bool
	}{
		{"cocok", "/cb?state=S1&code=x", []*http.Cookie{good("S1"), nonce("N1")}, true},
		{"state beda (login-CSRF)", "/cb?state=EVIL&code=x", []*http.Cookie{good("S1"), nonce("N1")}, false},
		{"tanpa parameter state", "/cb?code=x", []*http.Cookie{good("S1"), nonce("N1")}, false},
		{"tanpa cookie state", "/cb?state=S1&code=x", []*http.Cookie{nonce("N1")}, false},
		{"tanpa cookie nonce", "/cb?state=S1&code=x", []*http.Cookie{good("S1")}, false},
		{"tanpa cookie sama sekali (kode dari browser lain)", "/cb?state=S1&code=x", nil, false},
		{"cookie state kosong & state kosong", "/cb?state=&code=x", []*http.Cookie{good(""), nonce("N1")}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, rec := newCtx(http.MethodGet, tc.url, tc.cookies...)
			n, err := finishOIDCFlow(c)
			if tc.wantOK {
				if err != nil || n != "N1" {
					t.Fatalf("want nonce N1, got %q err=%v", n, err)
				}
			} else {
				if !errors.Is(err, domain.ErrInvalidInput) {
					t.Fatalf("want ErrInvalidInput (400), got %v", err)
				}
				if n != "" {
					t.Fatalf("nonce tidak boleh bocor saat gagal: %q", n)
				}
			}
			// Sekali pakai: cookie SELALU dihapus, berhasil maupun gagal.
			cks := cookiesByName(rec)
			for _, name := range []string{oidcStateCookie, oidcNonceCookie} {
				if ck := cks[name]; ck == nil || ck.MaxAge >= 0 {
					t.Fatalf("cookie %s harus dihapus (MaxAge<0) setelah callback: %+v", name, ck)
				}
			}
		})
	}
}

func TestFinishOIDCFlow_PesanTidakMembedakanPenyebab(t *testing.T) {
	c1, _ := newCtx(http.MethodGet, "/cb?state=A")
	c2, _ := newCtx(http.MethodGet, "/cb?state=A",
		&http.Cookie{Name: oidcStateCookie, Value: "B"}, &http.Cookie{Name: oidcNonceCookie, Value: "N"})
	_, e1 := finishOIDCFlow(c1)
	_, e2 := finishOIDCFlow(c2)
	if e1 == nil || e2 == nil || e1.Error() != e2.Error() {
		t.Fatalf("pesan harus sama untuk cookie hilang vs tidak cocok: %v | %v", e1, e2)
	}
	if strings.Contains(e1.Error(), "A") && strings.Contains(e1.Error(), "B") {
		t.Fatal("pesan tidak boleh memuat nilai state")
	}
}
