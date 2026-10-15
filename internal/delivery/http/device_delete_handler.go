package http

import (
	"net/http"

	"github.com/labstack/echo/v5"
)

// deleteDevice — soft delete device. RBAC (ADMIN+), scope tenant, dan
// pembatalan task terbuka ada di usecase/device.Delete.
func (r *Router) deleteDevice(c *echo.Context) error {
	id, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	if err := r.Devices.Delete(c.Request().Context(), ActorFrom(c), id); err != nil {
		return handleErr(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}
