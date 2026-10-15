package http

import (
	"net/http"

	"github.com/labstack/echo/v5"

	"acs/internal/usecase/device"
)

// createDeviceRequest — pra-registrasi perangkat (POST /devices) dan satu baris
// impor (POST /devices/import). Handler hanya mem-parse; validasi & aturan
// tenant ada di usecase/device.
type createDeviceRequest struct {
	TenantID                  *uint64  `json:"tenant_id"`
	OUI                       string   `json:"oui"`
	SerialNumber              string   `json:"serial_number"`
	ProductClass              *string  `json:"product_class"`
	MACAddress                *string  `json:"mac_address"`
	Notes                     *string  `json:"notes"`
	VendorID                  *uint64  `json:"vendor_id"`
	DeviceModelID             *uint64  `json:"device_model_id"`
	Latitude                  *float64 `json:"latitude"`
	Longitude                 *float64 `json:"longitude"`
	ConnectionRequestURL      *string  `json:"connection_request_url"`
	ConnectionRequestUsername *string  `json:"connection_request_username"`
	ConnectionRequestPassword *string  `json:"connection_request_password"`
}

func (q createDeviceRequest) toInput() device.CreateDeviceInput {
	return device.CreateDeviceInput{
		TenantID: q.TenantID, OUI: q.OUI, SerialNumber: q.SerialNumber,
		ProductClass: q.ProductClass, MACAddress: q.MACAddress, Notes: q.Notes,
		VendorID: q.VendorID, DeviceModelID: q.DeviceModelID,
		Latitude: q.Latitude, Longitude: q.Longitude,
		ConnectionRequestURL: q.ConnectionRequestURL, ConnectionRequestUsername: q.ConnectionRequestUsername,
		ConnectionRequestPassword: q.ConnectionRequestPassword,
	}
}

func (r *Router) createDevice(c *echo.Context) error {
	actor := ActorFrom(c)
	var req createDeviceRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "payload tidak valid")
	}
	d, err := r.Devices.Create(c.Request().Context(), actor, req.toInput())
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusCreated, d)
}

type importDevicesRequest struct {
	Mode string                `json:"mode"`
	Rows []createDeviceRequest `json:"rows"`
}

// importDevices — impor massal; `?dry_run=true` hanya mengklasifikasi baris
// (create/update/unchanged/skip/error) tanpa menulis apa pun, dipakai untuk
// pratinjau sebelum eksekusi. Maks. device.MaxImportRows baris per permintaan.
func (r *Router) importDevices(c *echo.Context) error {
	actor := ActorFrom(c)
	var req importDevicesRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "payload tidak valid")
	}
	rows := make([]device.CreateDeviceInput, len(req.Rows))
	for i, row := range req.Rows {
		rows[i] = row.toInput()
	}
	dry := c.QueryParam("dry_run") == "true" || c.QueryParam("dry_run") == "1"
	res, err := r.Devices.Import(c.Request().Context(), actor, device.ImportInput{Mode: req.Mode, DryRun: dry, Rows: rows})
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, res)
}
