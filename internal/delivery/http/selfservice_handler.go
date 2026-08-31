package http

import (
	"net/http"

	"acs/internal/domain"

	"github.com/labstack/echo/v5"
)

// mountSelfServiceRoutes memasang endpoint portal self-service pelanggan
// (role ENDUSER). Semua endpoint di sini di-scope ke device yang dipetakan
// ke akun ENDUSER lewat tabel user_devices (migrations/0020).
func (r *Router) mountSelfServiceRoutes(api *echo.Group) {
	ss := api.Group("/self-service", AuthMiddleware(r.Auth), RequireRoles(domain.RoleEndUser))
	ss.GET("/devices", r.listMyDevices)
	ss.GET("/devices/:id", r.getMyDevice)
	ss.PATCH("/devices/:id/wifi", r.changeMyWiFi)
	ss.POST("/devices/:id/reboot", r.rebootMyDevice)
}

func (r *Router) listMyDevices(c *echo.Context) error {
	actor := ActorFrom(c)
	devices, err := r.SelfService.ListMyDevices(c.Request().Context(), actor)
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, listResponse{Data: devices, Total: len(devices)})
}

func (r *Router) getMyDevice(c *echo.Context) error {
	actor := ActorFrom(c)
	id, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	dev, err := r.SelfService.GetMyDevice(c.Request().Context(), actor, id)
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, dev)
}

type changeWiFiReq struct {
	SSID       string `json:"ssid"`
	Passphrase string `json:"passphrase"`
	Band       string `json:"band"` // "2g" | "5g" | "" (dua-duanya)
}

func (r *Router) changeMyWiFi(c *echo.Context) error {
	actor := ActorFrom(c)
	id, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	var req changeWiFiReq
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "payload tidak valid")
	}
	task, err := r.SelfService.ChangeMyWiFi(c.Request().Context(), actor, id, domain.SelfServiceWiFiChange{
		SSID:       req.SSID,
		Passphrase: req.Passphrase,
		Band:       req.Band,
	})
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusAccepted, task)
}

func (r *Router) rebootMyDevice(c *echo.Context) error {
	actor := ActorFrom(c)
	id, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	if err := r.SelfService.RebootMyDevice(c.Request().Context(), actor, id); err != nil {
		return handleErr(c, err)
	}
	return c.NoContent(http.StatusAccepted)
}
