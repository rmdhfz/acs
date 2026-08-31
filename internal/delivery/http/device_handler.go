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
		TagID:          queryUint64(c, "tag_id"),
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

func (r *Router) countOpenSessions(c *echo.Context) error {
	// Endpoint ini lintas-tenant karena melacak load balancer CWMP
	// secara global. Hanya admin/noc (sudah digate di router).
	count, err := r.Sessions.CountOpenSessions(c.Request().Context())
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, map[string]int{"count": count})
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
	Notes                     *string  `json:"notes"`
	ConnectionRequestURL      *string  `json:"connection_request_url"`
	ConnectionRequestUsername *string  `json:"connection_request_username"`
	ConnectionRequestPassword *string  `json:"connection_request_password"`
	Latitude                  *float64 `json:"latitude"`
	Longitude                 *float64 `json:"longitude"`
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
		Latitude:                  req.Latitude,
		Longitude:                 req.Longitude,
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

func (r *Router) listConfigSnapshots(c *echo.Context) error {
	actor := ActorFrom(c)
	id, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	snaps, total, err := r.Devices.ListConfigSnapshots(c.Request().Context(), actor, id, paginationFromQuery(c))
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, listResponse{Data: snaps, Total: total})
}

func (r *Router) createConfigSnapshot(c *echo.Context) error {
	actor := ActorFrom(c)
	id, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}

	// ensure they can view device first
	if _, err := r.Devices.Get(c.Request().Context(), actor, id); err != nil {
		return handleErr(c, err)
	}

	if err := r.Devices.CreateConfigSnapshot(c.Request().Context(), id); err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, map[string]string{"message": "snapshot created"})
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

func (r *Router) listDeviceActivity(c *echo.Context) error {
	actor := ActorFrom(c)
	id, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	logs, total, err := r.Devices.ListActivity(c.Request().Context(), actor, id, paginationFromQuery(c))
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, listResponse{Data: logs, Total: total})
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

// triggerConnectionRequest mengirim HTTP GET ke connection_request_url CPE\
// (TR-069 §3.2.2) untuk memaksa device segera Inform ke ACS. Berguna untuk\
// membangunkan device offline tanpa menunggu periodic inform interval.\
// Hanya berhasil bila device mempunyai connection_request_url terkonfigurasi\
// dan dapat dijangkau dari ACS (tidak di balik NAT tanpa port forward).
func (r *Router) triggerConnectionRequest(c *echo.Context) error {
	actor := ActorFrom(c)
	id, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	if err := r.Devices.TriggerConnectionRequest(c.Request().Context(), actor, id); err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusAccepted, map[string]string{
		"message": "Connection request terkirim. Device akan Inform dalam waktu singkat.",
	})
}

func (r *Router) rebootDevice(c *echo.Context) error {
	actor := ActorFrom(c)
	id, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	if err := r.Devices.Reboot(c.Request().Context(), actor, id); err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusAccepted, map[string]string{
		"message": "Task reboot berhasil ditambahkan ke antrean.",
	})
}

func (r *Router) factoryResetDevice(c *echo.Context) error {
	actor := ActorFrom(c)
	id, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	if err := r.Devices.FactoryReset(c.Request().Context(), actor, id); err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusAccepted, map[string]string{
		"message": "Task factory reset berhasil ditambahkan ke antrean.",
	})
}

type pushFileRequest struct {
	FileID uint64 `json:"file_id"`
}

func (r *Router) pushFileToDevice(c *echo.Context) error {
	actor := ActorFrom(c)
	id, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	var req pushFileRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "payload tidak valid")
	}
	if req.FileID == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "file_id wajib diisi")
	}
	if err := r.Devices.PushFile(c.Request().Context(), actor, id, req.FileID); err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusAccepted, map[string]string{
		"message": "Task download (push file) berhasil ditambahkan ke antrean.",
	})
}

type addObjectRequest struct {
	ObjectName string `json:"object_name"`
}

func (r *Router) addObjectDevice(c *echo.Context) error {
	actor := ActorFrom(c)
	id, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	var req addObjectRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "payload tidak valid")
	}
	if req.ObjectName == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "object_name wajib diisi")
	}
	if err := r.Devices.AddObject(c.Request().Context(), actor, id, req.ObjectName); err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusAccepted, map[string]string{
		"message": "Task AddObject berhasil ditambahkan ke antrean.",
	})
}

type deleteObjectRequest struct {
	ObjectName string `json:"object_name"`
}

func (r *Router) deleteObjectDevice(c *echo.Context) error {
	actor := ActorFrom(c)
	id, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	var req deleteObjectRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "payload tidak valid")
	}
	if req.ObjectName == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "object_name wajib diisi")
	}
	if err := r.Devices.DeleteObject(c.Request().Context(), actor, id, req.ObjectName); err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusAccepted, map[string]string{
		"message": "Task DeleteObject berhasil ditambahkan ke antrean.",
	})
}

type getParameterNamesRequest struct {
	Path      string `json:"path"`
	NextLevel bool   `json:"next_level"`
}

func (r *Router) getParameterNamesDevice(c *echo.Context) error {
	actor := ActorFrom(c)
	id, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	var req getParameterNamesRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "payload tidak valid")
	}
	if req.Path == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "path wajib diisi")
	}
	if err := r.Devices.GetParameterNames(c.Request().Context(), actor, id, req.Path, req.NextLevel); err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusAccepted, map[string]string{
		"message": "Task GetParameterNames berhasil ditambahkan ke antrean.",
	})
}

type getParameterValuesRequest struct {
	Names []string `json:"names"`
}

func (r *Router) getParameterValuesDevice(c *echo.Context) error {
	actor := ActorFrom(c)
	id, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	var req getParameterValuesRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "payload tidak valid")
	}
	if len(req.Names) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "names wajib diisi minimal 1")
	}
	if err := r.Devices.GetParameterValues(c.Request().Context(), actor, id, req.Names); err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusAccepted, map[string]string{
		"message": "Task GetParameterValues berhasil ditambahkan ke antrean.",
	})
}

type setParameterValuesRequest struct {
	Values map[string]string `json:"values"`
}

func (r *Router) setParameterValuesDevice(c *echo.Context) error {
	actor := ActorFrom(c)
	id, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	var req setParameterValuesRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "payload tidak valid")
	}
	if len(req.Values) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "values wajib diisi minimal 1")
	}
	if err := r.Devices.SetParameterValues(c.Request().Context(), actor, id, req.Values); err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusAccepted, map[string]string{
		"message": "Task SetParameterValues berhasil ditambahkan ke antrean.",
	})
}
