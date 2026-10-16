package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"

	"acs/internal/domain"
)

// mockIdP — IdP OIDC tiruan lengkap: discovery, JWKS, dan token endpoint yang mengeluarkan
// id_token RS256 sungguhan. Dipakai untuk menguji alur SSO ujung-ke-ujung di sisi ACS
// (verifikasi tanda tangan, issuer/audience, nonce, email_verified, pemetaan akun).
type mockIdP struct {
	srv   *httptest.Server
	key   *rsa.PrivateKey
	claim map[string]any // klaim tambahan/penimpa untuk id_token berikutnya
}

func newMockIdP(t *testing.T) *mockIdP {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	m := &mockIdP{key: key, claim: map[string]any{}}
	mux := http.NewServeMux()
	m.srv = httptest.NewServer(mux)
	t.Cleanup(m.srv.Close)
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer": m.srv.URL, "authorization_endpoint": m.srv.URL + "/authorize", "token_endpoint": m.srv.URL + "/token",
			"jwks_uri": m.srv.URL + "/jwks", "id_token_signing_alg_values_supported": []string{"RS256"},
			"response_types_supported": []string{"code"}, "subject_types_supported": []string{"public"},
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		jwk := jose.JSONWebKey{Key: &key.PublicKey, KeyID: "k1", Algorithm: "RS256", Use: "sig"}
		_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{jwk}})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		sig, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: jose.JSONWebKey{Key: key, KeyID: "k1"}}, (&jose.SignerOptions{}).WithType("JWT"))
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		now := time.Now()
		claims := map[string]any{
			"iss": m.srv.URL, "sub": "idp-user-1", "aud": "acs-client", "exp": now.Add(time.Hour).Unix(), "iat": now.Unix(),
			"nonce": "nonce-1", "email": "sso.user@perusahaan.id", "email_verified": true,
		}
		for k, v := range m.claim {
			claims[k] = v
		}
		raw, err := jwt.Signed(sig).Claims(claims).Serialize()
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "at", "token_type": "Bearer", "id_token": raw, "expires_in": 3600})
	})
	return m
}

func ssoService(t *testing.T, idp *mockIdP, users *fakeUserRepoAuth) *Service {
	t.Helper()
	svc := NewService(users, &fakeTokenRepoAuth{}, &fakeTenantRepoAuthTest{}, &fakeActivityRepoAuth{},
		[]byte("test-secret-at-least-32-bytes!!"), time.Hour, idp.srv.URL, "acs-client", "secret", "http://acs.test/cb")
	if !svc.HasOIDC() {
		t.Fatal("discovery IdP tiruan gagal")
	}
	return svc
}

func TestOIDCLogin_AlurLengkapDenganIdPTiruan(t *testing.T) {
	ctx := context.Background()
	user := &domain.User{ID: 7, Username: "budi", Email: "SSO.User@Perusahaan.id", IsActive: true, Roles: []string{domain.RoleNOC}}
	// Penyerang (ADMIN tenant lain) memakai username = email korban: TIDAK boleh dipetakan.
	decoy := &domain.User{ID: 8, Username: "sso.user@perusahaan.id", Email: "decoy@evil.test", IsActive: true}
	users := &fakeUserRepoAuth{
		byID:       map[uint64]*domain.User{7: user, 8: decoy},
		byUsername: map[string]*domain.User{"budi": user, "sso.user@perusahaan.id": decoy},
	}
	idp := newMockIdP(t)
	svc := ssoService(t, idp, users)

	// sukses: dipetakan lewat EMAIL ke akun 7 (bukan decoy ber-username sama dengan email)
	u, tok, err := svc.OIDCLogin(ctx, "kode", "nonce-1")
	if err != nil || u.ID != 7 || tok == "" {
		t.Fatalf("login SSO sah gagal: u=%v err=%v", u, err)
	}
	if actor, err := svc.ResolveActor(ctx, tok); err != nil || actor.UserID != 7 {
		t.Fatalf("JWT hasil SSO harus valid: %v %v", actor, err)
	}

	// nonce salah (replay)
	if _, _, err := svc.OIDCLogin(ctx, "kode", "nonce-lain"); err == nil {
		t.Fatal("nonce tak cocok harus ditolak")
	}
	// email_verified=false
	idp.claim["email_verified"] = false
	if _, _, err := svc.OIDCLogin(ctx, "kode", "nonce-1"); err == nil {
		t.Fatal("email belum terverifikasi harus ditolak")
	}
	idp.claim["email_verified"] = true
	// email tak terdaftar -> tidak auto-provisioning
	idp.claim["email"] = "orang.baru@perusahaan.id"
	if _, _, err := svc.OIDCLogin(ctx, "kode", "nonce-1"); err == nil || !strings.Contains(err.Error(), "belum terdaftar") {
		t.Fatalf("akun tak terdaftar harus ditolak: %v", err)
	}
	idp.claim["email"] = "sso.user@perusahaan.id"
	// audience salah
	idp.claim["aud"] = "client-lain"
	if _, _, err := svc.OIDCLogin(ctx, "kode", "nonce-1"); err == nil {
		t.Fatal("audience salah harus ditolak")
	}
	delete(idp.claim, "aud")
	// token kedaluwarsa
	idp.claim["exp"] = time.Now().Add(-time.Hour).Unix()
	if _, _, err := svc.OIDCLogin(ctx, "kode", "nonce-1"); err == nil {
		t.Fatal("id_token kedaluwarsa harus ditolak")
	}
	delete(idp.claim, "exp")
	// akun nonaktif
	user.IsActive = false
	if _, _, err := svc.OIDCLogin(ctx, "kode", "nonce-1"); err == nil {
		t.Fatal("akun nonaktif harus ditolak")
	}
}

// IdP belum siap saat startup tidak mematikan SSO permanen: dicoba lagi saat dipakai.
func TestOIDC_DiscoveryDicobaUlang(t *testing.T) {
	idp := newMockIdP(t)
	url := idp.srv.URL
	idp.srv.Close() // IdP mati saat NewService
	svc := NewService(&fakeUserRepoAuth{}, &fakeTokenRepoAuth{}, &fakeTenantRepoAuthTest{}, &fakeActivityRepoAuth{},
		[]byte("test-secret-at-least-32-bytes!!"), time.Hour, url, "acs-client", "secret", "http://acs.test/cb")
	if svc.HasOIDC() {
		t.Fatal("IdP mati: belum boleh siap")
	}
	if !svc.OIDCConfigured() {
		t.Fatal("tetap terkonfigurasi walau IdP belum terjangkau (tombol SSO tampil)")
	}
}
