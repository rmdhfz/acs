package http

import (
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	"acs/internal/domain"
	"acs/internal/usecase/auth"
)

const actorContextKey = "actor"

// AuthMiddleware mewajibkan Bearer token (JWT sesi user atau API token
// "acs_...") pada setiap request — tidak ada endpoint mutasi tanpa
// autentikasi (CLAUDE.md).
func AuthMiddleware(authSvc *auth.Service) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			h := c.Request().Header.Get("Authorization")
			if h == "" || !strings.HasPrefix(h, "Bearer ") {
				return echo.NewHTTPError(http.StatusUnauthorized, "missing bearer token")
			}
			tokenStr := strings.TrimPrefix(h, "Bearer ")

			// ResolveActor menyatukan parsing JWT/API token DAN cek tenant aktif
			// (auth.ErrTenantInactive) -- lihat komentar lengkap di
			// usecase/auth.Service.ResolveActor soal kenapa ini dicek di SETIAP
			// request, bukan cuma saat login.
			actor, err := authSvc.ResolveActor(c.Request().Context(), tokenStr)
			if err != nil {
				switch {
				case errors.Is(err, auth.ErrTenantInactive):
					return echo.NewHTTPError(http.StatusUnauthorized, err.Error())
				case errors.Is(err, auth.ErrInvalidToken):
					return echo.NewHTTPError(http.StatusUnauthorized, "invalid token")
				default:
					// Error SISTEM (mis. DB timeout saat checkTenantActive),
					// BUKAN token yang salah -- jangan disamarkan jadi 401
					// "invalid token" (temuan review: itu menyulitkan on-call
					// membedakan insiden nyata dari gangguan infrastruktur
					// sesaat). Di-log server-side, klien dapat 500 generik
					// tanpa detail error internal.
					log.Printf("auth middleware: gagal resolve actor: %v", err)
					return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
				}
			}
			if actor == nil {
				return echo.NewHTTPError(http.StatusUnauthorized, "invalid token")
			}
			c.Set(actorContextKey, *actor)
			return next(c)
		}
	}
}

func ActorFrom(c *echo.Context) domain.Actor {
	if a, ok := c.Get(actorContextKey).(domain.Actor); ok {
		return a
	}
	return domain.Actor{}
}

// RequireRoles menolak request bila actor tidak superadmin dan tidak punya
// salah satu role yang diizinkan (RBAC scope, CLAUDE.md).
func RequireRoles(roles ...string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			if err := auth.RequireRole(ActorFrom(c), roles...); err != nil {
				return echo.NewHTTPError(http.StatusForbidden, "insufficient role")
			}
			return next(c)
		}
	}
}

func handleErr(c *echo.Context, err error) error {
	switch {
	// auth.ErrInvalidCredentials -- dipakai auth.Service.ChangeOwnPassword saat
	// current_password tidak cocok (bukan cuma di Login). Bukan domain.Err*
	// krn ini spesifik ke package auth (dan supaya pesan bisa disamakan dgn
	// pesan login gagal biasa) -- dicek terpisah di sini, sebelum default 500.
	case errors.Is(err, auth.ErrInvalidCredentials):
		return echo.NewHTTPError(http.StatusUnauthorized, err.Error())
	case errors.Is(err, domain.ErrNotFound):
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	case errors.Is(err, domain.ErrForbidden):
		return echo.NewHTTPError(http.StatusForbidden, err.Error())
	case errors.Is(err, domain.ErrUnauthorized):
		return echo.NewHTTPError(http.StatusUnauthorized, err.Error())
	case errors.Is(err, domain.ErrConflict):
		return echo.NewHTTPError(http.StatusConflict, err.Error())
	case errors.Is(err, domain.ErrInvalidInput):
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	case errors.Is(err, domain.ErrQuotaExceeded):
		return echo.NewHTTPError(http.StatusTooManyRequests, err.Error())
	default:
		// Error sistem (DB timeout, query error, dsb.) — JANGAN di-expose
		// ke client (bisa bocor detail internal: query SQL, stack trace,
		// nama tabel, dsb.). Di-log server-side dengan detail penuh, client
		// hanya dapat pesan generik. Berbeda dari domain error di atas yang
		// memang dirancang untuk dikembalikan ke client (domain.ErrNotFound
		// dsb. tidak mengandung detail implementasi).
		log.Printf("handleErr: unhandled error: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}
}
