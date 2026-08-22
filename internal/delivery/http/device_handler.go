package http

import (
	"net/http"

	"github.com/labstack/echo/v5"

	"acs/internal/domain"
	"acs/internal/usecase/device"
)

func (r *Router) listDevices(c *echo.Context) error {
	actor := ActorFrom(c)
	f := domain.DeviceFilter{
		Search:         c.QueryParam("search"),
		VendorID:       queryUint64(c, "vendor_id"),
		DeviceModelID:  queryUint64(c, "device_model_id"),
		DeviceStatusID: queryUint64(c, "device_status_id"),
		TenantID:       queryUint64(c, "tenant_id"),
	}
	devices, total, err := r.Devices.List(c.Request().Context(), actor, f, paginationFromQuery(c))
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, listResponse{Data: devices, Total: total})
}

func (r *Router) deviceStats(c *echo.Context) error {
	actor := ActorFrom(c)
	stats, err := r.Devices.Stats(c.Request().Context(), actor)
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, stats)
}

func (r *Router) getDevice(c *echo.Context) error {
	actor := ActorFrom(c)
	id, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	d, err := r.Devices.Get(c.Request().Context(), actor, id)
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, d)
}

type updateDeviceRequest struct {
	Notes                     *string `json:"notes"`
	ConnectionRequestURL      *string `json:"connection_request_url"`
	ConnectionRequestUsername *string `json:"connection_request_username"`
	ConnectionRequestPassword *string `json:"connection_request_password"`
}

func (r *Router) updateDevice(c *echo.Context) error {
	actor := ActorFrom(c)
	id, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	var req updateDeviceRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "payload tidak valid")
	}
	d, err := r.Devices.Update(c.Request().Context(), actor, id, device.UpdateDeviceInput{
		Notes:                     req.Notes,
		ConnectionRequestURL:      req.ConnectionRequestURL,
		ConnectionRequestUsername: req.ConnectionRequestUsername,
		ConnectionRequestPassword: req.ConnectionRequestPassword,
	})
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, d)
}

func (r *Router) listDeviceParameters(c *echo.Context) error {
	actor := ActorFrom(c)
	id, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	params, err := r.Devices.ListParameters(c.Request().Context(), actor, id, c.QueryParam("prefix"))
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, params)
}

func (r *Router) listDeviceEvents(c *echo.Context) error {
	actor := ActorFrom(c)
	id, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	events, total, err := r.Devices.ListEvents(c.Request().Context(), actor, id, paginationFromQuery(c))
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, listResponse{Data: events, Total: total})
}

func (r *Router) listOpticalMetrics(c *echo.Context) error {
	actor := ActorFrom(c)
	id, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	metrics, total, err := r.Devices.ListOpticalMetrics(c.Request().Context(), actor, id, paginationFromQuery(c))
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, listResponse{Data: metrics, Total: total})
}
