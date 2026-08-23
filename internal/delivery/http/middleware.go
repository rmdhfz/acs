package http

import (
	"errors"
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

			var actor *domain.Actor
			var err error
			if strings.HasPrefix(tokenStr, "acs_") {
				actor, err = authSvc.AuthenticateAPIToken(c.Request().Context(), tokenStr)
			} else {
				actor, err = authSvc.ParseJWT(tokenStr)
			}
			if err != nil || actor == nil {
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
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
}
