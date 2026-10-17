package http

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"

	"acs/internal/usecase/signup"
)

// Pendaftaran tenant mandiri (publik, dibatasi laju, bawaan MATI). Seluruh keputusan ada di usecase/signup.

type signupRequest struct {
	CompanyName string `json:"company_name"`
	FullName    string `json:"full_name"`
	Email       string `json:"email"`
	Password    string `json:"password"`
	AcceptTerms bool   `json:"accept_terms"`
	// Website = honeypot: kolom tersembunyi di form; manusia tidak mengisinya, bot sederhana mengisinya.
	Website string `json:"website"`
}

func (r *Router) signupEnabled() bool { return r.Signup != nil && r.Signup.Enabled() }

func (r *Router) signup(c *echo.Context) error {
	if !r.signupEnabled() {
		return echo.NewHTTPError(http.StatusForbidden, "pendaftaran mandiri tidak diaktifkan")
	}
	var req signupRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "payload tidak valid")
	}
	err := r.Signup.Request(c.Request().Context(), signup.RequestInput{
		CompanyName: req.CompanyName, FullName: req.FullName, Email: req.Email, Password: req.Password,
		AcceptTerms: req.AcceptTerms, Honeypot: req.Website, IP: c.RealIP(),
	})
	if err != nil {
		switch {
		case errors.Is(err, signup.ErrClosed):
			return echo.NewHTTPError(http.StatusServiceUnavailable, err.Error())
		case errors.Is(err, signup.ErrDisabled):
			return echo.NewHTTPError(http.StatusForbidden, "pendaftaran mandiri tidak diaktifkan")
		}
		return handleErr(c, err)
	}
	// Respons SERAGAM untuk email baru maupun yang sudah terdaftar (anti-enumerasi akun).
	c.Response().Header().Set("Cache-Control", "no-store")
	return c.JSON(http.StatusAccepted, map[string]string{
		"message": "Jika data valid, kami telah mengirim tautan verifikasi ke email Anda. Tautan berlaku 24 jam.",
	})
}

type verifySignupRequest struct {
	Token string `json:"token"`
}

func (r *Router) verifySignup(c *echo.Context) error {
	if !r.signupEnabled() {
		return echo.NewHTTPError(http.StatusForbidden, "pendaftaran mandiri tidak diaktifkan")
	}
	var req verifySignupRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "payload tidak valid")
	}
	res, err := r.Signup.Verify(c.Request().Context(), req.Token)
	if err != nil {
		switch {
		case errors.Is(err, signup.ErrInvalid):
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		case errors.Is(err, signup.ErrDisabled):
			return echo.NewHTTPError(http.StatusForbidden, "pendaftaran mandiri tidak diaktifkan")
		}
		return handleErr(c, err)
	}
	c.Response().Header().Set("Cache-Control", "no-store")
	return c.JSON(http.StatusOK, map[string]string{"tenant_code": res.TenantCode, "tenant_name": res.TenantName, "email": res.Email})
}
