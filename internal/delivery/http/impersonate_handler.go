package http

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"

	"acs/internal/usecase/auth"
)

// denyIfImpersonating menolak aksi sensitif akun selama sesi impersonate (ganti password, MFA, token API):
// SUPERADMIN yang menyamar tidak boleh mengambil alih kredensial/faktor milik akun yang disamarkan.
func denyIfImpersonating(c *echo.Context) error {
	if ActorFrom(c).ImpersonatorID != 0 {
		return echo.NewHTTPError(http.StatusForbidden, "aksi ini tidak diizinkan selama sesi impersonate")
	}
	return nil
}

// denyImpersonation — middleware rute: akun/kredensial/peran pengguna tidak boleh diubah dari sesi impersonate
// (reset password, ubah email → reset via email, buat ADMIN baru, ubah peran, hapus, lepas MFA = pengambilalihan akun).
func denyImpersonation(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		if err := denyIfImpersonating(c); err != nil {
			return err
		}
		return next(c)
	}
}

// impersonateUser — POST /users/:id/impersonate (SUPERADMIN): token pendek untuk "masuk sebagai" pengguna tenant.
func (r *Router) impersonateUser(c *echo.Context) error {
	if err := requireSessionToken(c); err != nil {
		return err
	}
	id, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	res, err := r.Auth.Impersonate(c.Request().Context(), ActorFrom(c), id)
	if err != nil {
		if errors.Is(err, auth.ErrImpersonationForbidden) {
			return echo.NewHTTPError(http.StatusForbidden, err.Error())
		}
		if errors.Is(err, auth.ErrTenantInactive) {
			return echo.NewHTTPError(http.StatusConflict, err.Error())
		}
		return handleErr(c, err)
	}
	c.Response().Header().Set("Cache-Control", "no-store")
	return c.JSON(http.StatusOK, map[string]interface{}{
		"access_token":    res.Token,
		"token_type":      "Bearer",
		"expires_at":      res.ExpiresAt,
		"impersonator_id": ActorFrom(c).UserID,
		"user": map[string]interface{}{
			"id": res.User.ID, "uuid": res.User.UserUUID, "username": res.User.Username,
			"roles": res.User.Roles, "tenant_id": res.User.TenantID,
		},
	})
}

// endImpersonation — POST /auth/impersonation/end: mencatat akhir sesi impersonate (klien membuang tokennya).
func (r *Router) endImpersonation(c *echo.Context) error {
	if err := r.Auth.EndImpersonation(c.Request().Context(), ActorFrom(c)); err != nil {
		return handleErr(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}
