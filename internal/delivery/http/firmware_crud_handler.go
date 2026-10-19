package http

import (
	"net/http"

	"github.com/labstack/echo/v5"

	"acs/internal/usecase/firmware"
)

// Handler get/update/delete firmware file (upload & list ada di
// firmware_handler.go). RBAC superadmin dan guard "masih dipakai -> 409" ada
// di usecase/firmware.

// maxFirmwareVersion — firmware_files.version VARCHAR(64) NOT NULL.
const maxFirmwareVersion = 64

// maxFirmwareReleaseNotes — firmware_files.release_notes TEXT (kolom tanpa
// batas praktis), tapi dibatasi di aplikasi agar konsisten dgn uploadFirmware
// (lihat firmware_handler.go) — sebelumnya PATCH tidak membatasi sama sekali
// sementara upload membatasi 4000, jadi edit bisa menyimpan nilai yang
// upload tidak akan pernah terima.
const maxFirmwareReleaseNotes = 4000

func (r *Router) getFirmware(c *echo.Context) error {
	id, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	f, err := r.Firmware.Get(c.Request().Context(), id)
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, f)
}

type updateFirmwareRequest struct {
	Version      *string `json:"version"`
	ReleaseNotes *string `json:"release_notes"`
	IsActive     *bool   `json:"is_active"`
}

func (r *Router) updateFirmware(c *echo.Context) error {
	id, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	var req updateFirmwareRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "payload tidak valid")
	}
	if req.Version == nil && req.ReleaseNotes == nil && req.IsActive == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "minimal satu field (version, release_notes, is_active) harus diisi")
	}
	if err := checkMaxLen(
		lenRule{"version", optStr(req.Version), maxFirmwareVersion},
		lenRule{"release_notes", optStr(req.ReleaseNotes), maxFirmwareReleaseNotes},
	); err != nil {
		return err
	}
	f, err := r.Firmware.UpdateFirmware(c.Request().Context(), ActorFrom(c), id, firmware.UpdateFirmwareInput(req))
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, f)
}

func (r *Router) deleteFirmware(c *echo.Context) error {
	id, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	if err := r.Firmware.DeleteFirmware(c.Request().Context(), ActorFrom(c), id); err != nil {
		return handleErr(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}
