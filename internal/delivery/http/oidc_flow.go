package http

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	"acs/internal/domain"
)

// Perlindungan login OIDC (OWASP A07): state acak mencegah login-CSRF (penyerang
// menyuntikkan kode otorisasinya ke browser korban), nonce mengikat id_token ke
// browser yang memulai alur (anti-replay). Keduanya disimpan di cookie
// HttpOnly + SameSite=Lax yang hanya berlaku untuk jalur /auth/oidc dan hanya
// 10 menit; sekali pakai (dihapus di callback, berhasil maupun gagal).
const (
	oidcStateCookie = "acs_oidc_state"
	oidcNonceCookie = "acs_oidc_nonce"
	oidcCookiePath  = "/api/v1/auth/oidc"
	oidcCookieTTL   = 600 // detik
)

// randomToken menghasilkan 256 bit acak (base64url, 43 karakter).
func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// requestIsSecure — true bila request datang lewat TLS langsung atau lewat
// reverse proxy yang menandainya https. Hanya dipakai untuk menentukan flag
// Secure pada cookie (menyalakannya terlalu dini tidak berbahaya).
func requestIsSecure(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func setOIDCCookie(w http.ResponseWriter, r *http.Request, name, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name: name, Value: value, Path: oidcCookiePath, MaxAge: maxAge,
		HttpOnly: true, Secure: requestIsSecure(r), SameSite: http.SameSiteLaxMode,
	})
}

// startOIDCFlow membuat state+nonce acak, mengikatnya ke browser lewat cookie,
// dan mengembalikan keduanya untuk disisipkan ke URL otorisasi IdP.
func startOIDCFlow(c *echo.Context) (state, nonce string, err error) {
	if state, err = randomToken(); err != nil {
		return "", "", err
	}
	if nonce, err = randomToken(); err != nil {
		return "", "", err
	}
	setOIDCCookie(c.Response(), c.Request(), oidcStateCookie, state, oidcCookieTTL)
	setOIDCCookie(c.Response(), c.Request(), oidcNonceCookie, nonce, oidcCookieTTL)
	return state, nonce, nil
}

// finishOIDCFlow memverifikasi parameter state dari IdP terhadap cookie yang
// dipasang startOIDCFlow (perbandingan konstan-waktu) dan mengembalikan nonce
// untuk diperiksa terhadap id_token. Cookie SELALU dihapus (sekali pakai),
// termasuk saat verifikasi gagal. Cookie hilang/kedaluwarsa/tidak cocok =
// ErrInvalidInput (400) tanpa membedakan penyebabnya ke pemanggil.
func finishOIDCFlow(c *echo.Context) (nonce string, err error) {
	r, w := c.Request(), c.Response()
	defer func() {
		setOIDCCookie(w, r, oidcStateCookie, "", -1)
		setOIDCCookie(w, r, oidcNonceCookie, "", -1)
	}()

	stateCk, e1 := r.Cookie(oidcStateCookie)
	nonceCk, e2 := r.Cookie(oidcNonceCookie)
	got := c.QueryParam("state")
	if e1 != nil || e2 != nil || stateCk.Value == "" || nonceCk.Value == "" || got == "" ||
		subtle.ConstantTimeCompare([]byte(stateCk.Value), []byte(got)) != 1 {
		return "", fmt.Errorf("%w: state OIDC tidak valid atau kedaluwarsa, ulangi login", domain.ErrInvalidInput)
	}
	return nonceCk.Value, nil
}
