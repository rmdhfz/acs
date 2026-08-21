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
	actor := ActorFrom(c)
	deviceID, err := parseUint64Param(c, "id")
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
	d, err := r.Diagnostics.Trigger(c.Request().Context(), actor, deviceID, req.DiagnosticType, req.Parameters)
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusCreated, d)
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
