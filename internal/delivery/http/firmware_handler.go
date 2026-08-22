package http

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v5"

	"acs/internal/usecase/firmware"
)

type uploadFirmwareRequest struct {
	VendorID       uint64  `json:"vendor_id"`
	DeviceModelID  *uint64 `json:"device_model_id"`
	Version        string  `json:"version"`
	FileName       string  `json:"file_name"`
	FilePath       string  `json:"file_path"`
	FileSizeBytes  *uint64 `json:"file_size_bytes"`
	ChecksumSHA256 *string `json:"checksum_sha256"`
	ReleaseNotes   *string `json:"release_notes"`
}

// uploadFirmware mencatat metadata firmware yang file-nya sudah tersedia di
// path yang dikirim (strategi object storage vs filesystem belum diputuskan,
// lihat TECH.md §12 — upload file fisik di luar cakupan endpoint ini).
func (r *Router) uploadFirmware(c *echo.Context) error {
	actor := ActorFrom(c)
	var req uploadFirmwareRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "payload tidak valid")
	}
	f, err := r.Firmware.UploadFirmware(c.Request().Context(), actor, firmware.UploadFirmwareInput{
		VendorID: req.VendorID, DeviceModelID: req.DeviceModelID, Version: req.Version,
		FileName: req.FileName, FilePath: req.FilePath, FileSizeBytes: req.FileSizeBytes,
		ChecksumSHA256: req.ChecksumSHA256, ReleaseNotes: req.ReleaseNotes,
	})
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusCreated, f)
}

func (r *Router) listFirmware(c *echo.Context) error {
	vendorID := queryUint64(c, "vendor_id")
	if vendorID == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "vendor_id wajib diisi")
	}
	files, total, err := r.Firmware.ListByVendor(c.Request().Context(), *vendorID, paginationFromQuery(c))
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, listResponse{Data: files, Total: total})
}

type scheduleUpgradeRequest struct {
	FirmwareID  uint64     `json:"firmware_id"`
	ScheduledAt *time.Time `json:"scheduled_at"`
}

func (r *Router) scheduleFirmwareUpgrade(c *echo.Context) error {
	actor := ActorFrom(c)
	deviceID, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	var req scheduleUpgradeRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "payload tidak valid")
	}
	if req.FirmwareID == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "firmware_id wajib diisi")
	}
	job, err := r.Firmware.ScheduleUpgrade(c.Request().Context(), actor, deviceID, req.FirmwareID, req.ScheduledAt)
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusCreated, job)
}

func (r *Router) listFirmwareJobs(c *echo.Context) error {
	actor := ActorFrom(c)
	deviceID, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	jobs, total, err := r.Firmware.ListJobsByDevice(c.Request().Context(), actor, deviceID, paginationFromQuery(c))
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, listResponse{Data: jobs, Total: total})
}
