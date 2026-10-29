package http

import (
	"net/http"

	"github.com/labstack/echo/v5"
)

// deviceDiagnosis — GET /devices/:id/diagnosis: "kenapa offline?" dari bukti sisi ACS (usecase/troubleshoot). Hanya baca; scope tenant
// ditegakkan usecase (perangkat tenant lain = 404, tidak membocorkan keberadaan).
func (r *Router) deviceDiagnosis(c *echo.Context) error {
	id, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	d, err := r.Troubleshoot.Diagnose(c.Request().Context(), ActorFrom(c), id)
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, d)
}
