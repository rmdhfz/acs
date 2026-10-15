package http

import (
	"net/http"

	"github.com/labstack/echo/v5"
)

// Handler get/delete untuk tenant dan user yang melengkapi CRUD IAM
// (create/list/update ada di iam_handler.go). Aturan RBAC, scope tenant, dan
// guard "masih punya user/device aktif" ada di usecase/iam, bukan di sini.

func (r *Router) getTenant(c *echo.Context) error {
	id, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	t, err := r.IAM.GetTenant(c.Request().Context(), ActorFrom(c), id)
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, t)
}

func (r *Router) deleteTenant(c *echo.Context) error {
	id, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	if err := r.IAM.DeleteTenant(c.Request().Context(), ActorFrom(c), id); err != nil {
		return handleErr(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (r *Router) getUser(c *echo.Context) error {
	id, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	u, err := r.IAM.GetUser(c.Request().Context(), ActorFrom(c), id)
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, u)
}
