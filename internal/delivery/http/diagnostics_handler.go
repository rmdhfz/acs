package http

import (
	"net/http"

	"github.com/labstack/echo/v5"
)

type triggerDiagnosticRequest struct {
	DiagnosticType string            `json:"diagnostic_type"`
	Parameters     map[string]string `json:"parameters"`
}

func (r *Router) triggerDiagnostic(c *echo.Context) error {
	_, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	var req triggerDiagnosticRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "payload tidak valid")
	}
	if req.DiagnosticType == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "diagnostic_type wajib diisi")
	}
	// Diagnostics (ping/traceroute) BELUM berfungsi penuh: tidak ada pemetaan
	// parameter diagnostik per data model, event "8 DIAGNOSTICS COMPLETE" belum
	// diproses, dan hasil tak pernah diambil. Sebelumnya endpoint ini membuat task
	// SET_PARAMETER_VALUES dengan parameters bebas (celah bagi NOC) yang pasti
	// gagal 9005. Ditolak jujur sampai fiturnya selesai.
	return echo.NewHTTPError(http.StatusNotImplemented, "Diagnostics belum didukung pada versi ini")
}

func (r *Router) listDiagnostics(c *echo.Context) error {
	actor := ActorFrom(c)
	deviceID, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	diags, total, err := r.Diagnostics.ListByDevice(c.Request().Context(), actor, deviceID, paginationFromQuery(c))
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, listResponse{Data: diags, Total: total})
}
