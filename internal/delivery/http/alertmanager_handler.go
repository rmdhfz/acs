package http

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"acs/internal/usecase/alerts"

	"github.com/labstack/echo/v5"
)

// Penerima webhook Alertmanager (migrations/0045). Alertmanager tidak punya sesi pengguna, jadi autentikasinya token Bearer bersama
// (ACS_ALERTMANAGER_WEBHOOK_TOKEN, dibandingkan waktu-konstan lewat hash agar panjang tidak bocor). Tanpa token terkonfigurasi rute
// tidak dipasang sama sekali (404). Body dibatasi, dan pembatas laju khusus dipasang di router.

const (
	// maxAlertmanagerBody: Alertmanager memotong lewat max_alerts (bawaan tanpa batas; config repo memakai 50); 512 KiB longgar.
	maxAlertmanagerBody = 512 << 10
	maxBearerToken      = 512
)

// alertmanagerAuthorized membandingkan header Authorization "Bearer <token>" dengan token terkonfigurasi.
func (r *Router) alertmanagerAuthorized(header string) bool {
	token, ok := strings.CutPrefix(header, "Bearer ")
	if !ok || token == "" || len(token) > maxBearerToken || r.AlertmanagerToken == "" {
		return false
	}
	a, b := sha256.Sum256([]byte(token)), sha256.Sum256([]byte(r.AlertmanagerToken))
	return subtle.ConstantTimeCompare(a[:], b[:]) == 1
}

// POST /alertmanager-webhook — payload webhook Alertmanager versi 4. Badan dibaca TANPA strict-decode: Alertmanager menambah field
// seiring versi, dan field asing tidak boleh membuat alert hilang. 2xx menghentikan percobaan ulang; galat memicunya.
func (r *Router) alertmanagerWebhook(c *echo.Context) error {
	if !r.alertmanagerAuthorized(c.Request().Header.Get("Authorization")) {
		c.Response().Header().Set("WWW-Authenticate", `Bearer realm="acs-alertmanager"`)
		return echo.NewHTTPError(http.StatusUnauthorized, "token tidak valid")
	}
	var p alerts.Payload
	dec := json.NewDecoder(http.MaxBytesReader(c.Response(), c.Request().Body, maxAlertmanagerBody))
	if err := dec.Decode(&p); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			return echo.NewHTTPError(http.StatusRequestEntityTooLarge, "payload terlalu besar")
		}
		return echo.NewHTTPError(http.StatusBadRequest, "payload tidak valid")
	}
	res, err := r.Alerts.Ingest(c.Request().Context(), p)
	if err != nil {
		// Masukan salah -> 400 (jangan dicoba ulang); galat sistem -> 500 generik (Alertmanager mencoba ulang).
		return handleErr(c, err)
	}
	c.Response().Header().Set("Cache-Control", "no-store")
	return c.JSON(http.StatusOK, res)
}
