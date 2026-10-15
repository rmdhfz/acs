package device

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"acs/pkg/digestauth"
)

func noRedirectClient() *http.Client {
	return &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

// cpeServer meniru endpoint Connection Request CPE. mode: "digest", "basic", "none", "unknown".
func cpeServer(t *testing.T, mode, user, pass string, hits *int32, sawAuthOnFirst *int32) *httptest.Server {
	t.Helper()
	const realm, nonce = "IGD", "srv-nonce-1"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(hits, 1)
		authz := r.Header.Get("Authorization")
		if n == 1 && authz != "" {
			atomic.StoreInt32(sawAuthOnFirst, 1)
		}
		switch mode {
		case "none":
			w.WriteHeader(http.StatusOK)
		case "digest":
			if authz != "" && digestauth.Verify(r.Method, authz, user, pass, realm, nonce) {
				w.WriteHeader(http.StatusOK)
				return
			}
			w.Header().Set("WWW-Authenticate", digestauth.NewChallengeHeader(realm, nonce))
			w.WriteHeader(http.StatusUnauthorized)
		case "basic":
			if u, p, ok := r.BasicAuth(); ok && u == user && p == pass {
				w.WriteHeader(http.StatusOK)
				return
			}
			w.Header().Set("WWW-Authenticate", `Basic realm="IGD"`)
			w.WriteHeader(http.StatusUnauthorized)
		default:
			w.Header().Set("WWW-Authenticate", `Negotiate`)
			w.WriteHeader(http.StatusUnauthorized)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestDoConnectionRequest(t *testing.T) {
	cases := []struct {
		name       string
		mode       string
		user, pass string // kredensial yang dikirim ACS
		wantStatus int
		wantHits   int32
	}{
		{"digest benar", "digest", "acs", "secret", http.StatusOK, 2},
		{"digest password salah", "digest", "acs", "salah", http.StatusUnauthorized, 2},
		{"basic benar (CPE hanya Basic)", "basic", "acs", "secret", http.StatusOK, 2},
		{"basic password salah", "basic", "acs", "salah", http.StatusUnauthorized, 2},
		{"tanpa auth di CPE", "none", "acs", "secret", http.StatusOK, 1},
		{"tanpa kredensial di ACS tidak menjawab tantangan", "digest", "", "", http.StatusUnauthorized, 1},
		{"skema tak dikenal dikembalikan apa adanya", "unknown", "acs", "secret", http.StatusUnauthorized, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var hits, sawAuth int32
			srv := cpeServer(t, tc.mode, "acs", "secret", &hits, &sawAuth)
			resp, err := doConnectionRequest(context.Background(), noRedirectClient(), srv.URL+"/cr?x=1", tc.user, tc.pass)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != tc.wantStatus {
				t.Errorf("status = %d, mau %d", resp.StatusCode, tc.wantStatus)
			}
			if hits != tc.wantHits {
				t.Errorf("jumlah request ke CPE = %d, mau %d", hits, tc.wantHits)
			}
			if sawAuth != 0 {
				t.Error("request pertama tidak boleh membawa kredensial (tidak ada Basic preemptive)")
			}
		})
	}
}

// Nonce kedaluwarsa (stale=true) → ACS menjawab sekali lagi dengan nonce baru.
func TestDoConnectionRequest_StaleNonceRetry(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		switch n {
		case 1: // tantangan awal
			w.Header().Set("WWW-Authenticate", digestauth.NewChallengeHeader("IGD", "old"))
			w.WriteHeader(http.StatusUnauthorized)
		case 2: // nonce lama sudah basi
			w.Header().Set("WWW-Authenticate", digestauth.NewChallengeHeader("IGD", "fresh")+", stale=true")
			w.WriteHeader(http.StatusUnauthorized)
		default:
			if digestauth.Verify(r.Method, r.Header.Get("Authorization"), "acs", "secret", "IGD", "fresh") {
				w.WriteHeader(http.StatusOK)
				return
			}
			w.WriteHeader(http.StatusUnauthorized)
		}
	}))
	defer srv.Close()

	resp, err := doConnectionRequest(context.Background(), noRedirectClient(), srv.URL+"/cr", "acs", "secret")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || hits != 3 {
		t.Errorf("status=%d hits=%d, mau 200 dan 3", resp.StatusCode, hits)
	}
}
