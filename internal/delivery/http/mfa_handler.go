package http

import (
	"errors"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	"acs/internal/usecase/auth"
	"acs/pkg/passwordhash"
)

// MFA TOTP (migrations/0027). Semua logika di usecase/auth; handler hanya parsing & pemetaan error.

type mfaVerifyRequest struct {
	MFAToken string `json:"mfa_token"`
	Code     string `json:"code"`
}

// mfaVerify — tahap kedua login (PUBLIK dengan rate limit; kredensialnya adalah mfa_token hasil
// POST /auth/login + kode TOTP/pemulihan).
func (r *Router) mfaVerify(c *echo.Context) error {
	var req mfaVerifyRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "payload tidak valid")
	}
	if strings.TrimSpace(req.MFAToken) == "" || strings.TrimSpace(req.Code) == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "mfa_token dan code wajib diisi")
	}
	u, token, err := r.Auth.MFAVerifyLogin(c.Request().Context(), req.MFAToken, req.Code)
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrInvalidMFACode), errors.Is(err, auth.ErrInvalidCredentials),
			errors.Is(err, auth.ErrInvalidToken), errors.Is(err, auth.ErrTenantInactive):
			return echo.NewHTTPError(http.StatusUnauthorized, err.Error())
		default:
			return handleErr(c, err)
		}
	}
	return c.JSON(http.StatusOK, map[string]interface{}{
		"access_token": token,
		"token_type":   "Bearer",
		"user": map[string]interface{}{
			"id": u.ID, "uuid": u.UserUUID, "username": u.Username, "roles": u.Roles, "tenant_id": u.TenantID,
		},
	})
}

// requireSessionToken — pengelolaan MFA hanya lewat sesi login (JWT), bukan API token.
func requireSessionToken(c *echo.Context) error {
	if err := denyIfImpersonating(c); err != nil {
		return err
	}
	if strings.HasPrefix(bearerFromRequest(c.Request()), "acs_") {
		return echo.NewHTTPError(http.StatusForbidden, "API token tidak dapat mengelola MFA; gunakan sesi login")
	}
	return nil
}

func (r *Router) mfaStatus(c *echo.Context) error {
	if err := requireSessionToken(c); err != nil {
		return err
	}
	enabled, left, err := r.Auth.MFAStatus(c.Request().Context(), ActorFrom(c))
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, map[string]interface{}{
		"available": r.Auth.MFAEnabledFeature(), "enabled": enabled, "recovery_codes_left": left,
	})
}

func (r *Router) mfaSetup(c *echo.Context) error {
	if err := requireSessionToken(c); err != nil {
		return err
	}
	secret, uri, err := r.Auth.MFASetup(c.Request().Context(), ActorFrom(c))
	if err != nil {
		return handleErr(c, err)
	}
	c.Response().Header().Set("Cache-Control", "no-store")
	return c.JSON(http.StatusOK, map[string]string{"secret": secret, "otpauth_uri": uri})
}

type mfaEnableRequest struct {
	Code string `json:"code"`
}

func (r *Router) mfaEnable(c *echo.Context) error {
	if err := requireSessionToken(c); err != nil {
		return err
	}
	var req mfaEnableRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "payload tidak valid")
	}
	if strings.TrimSpace(req.Code) == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "code wajib diisi")
	}
	codes, err := r.Auth.MFAEnable(c.Request().Context(), ActorFrom(c), req.Code)
	if err != nil {
		return handleErr(c, err)
	}
	c.Response().Header().Set("Cache-Control", "no-store")
	return c.JSON(http.StatusOK, map[string]interface{}{"recovery_codes": codes})
}

type mfaDisableRequest struct {
	Password string `json:"password"`
	Code     string `json:"code"`
}

func (r *Router) mfaDisable(c *echo.Context) error {
	if err := requireSessionToken(c); err != nil {
		return err
	}
	var req mfaDisableRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "payload tidak valid")
	}
	if req.Password == "" || strings.TrimSpace(req.Code) == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "password dan code wajib diisi")
	}
	token, err := r.Auth.MFADisable(c.Request().Context(), ActorFrom(c), req.Password, req.Code)
	if err != nil {
		if errors.Is(err, passwordhash.ErrBusy) {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "server sedang sibuk, coba lagi sebentar")
		}
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, map[string]string{"access_token": token, "token_type": "Bearer"})
}

// adminResetMFA — ADMIN/SUPERADMIN melepas MFA akun lain (perangkat & kode pemulihan hilang).
func (r *Router) adminResetMFA(c *echo.Context) error {
	id, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	if err := r.Auth.MFAAdminReset(c.Request().Context(), ActorFrom(c), id); err != nil {
		return handleErr(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}
