package http

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v5"

	"acs/internal/domain"
	"acs/internal/usecase/firmware"
)

// maxFirmwareRequestBytes adalah batas ukuran BODY REQUEST multipart secara
// keseluruhan (bukan cuma field file) — diberi margin di atas
// domain.MaxFirmwareFileSizeBytes untuk field metadata form lain & overhead
// boundary multipart. Dicek SEBELUM body dibaca (http.MaxBytesReader) supaya
// upload raksasa tidak bisa dipakai untuk resource exhaustion.
const maxFirmwareRequestBytes = domain.MaxFirmwareFileSizeBytes + 1<<20

// uploadFirmware menerima file firmware sungguhan (multipart/form-data,
// bukan lagi JSON metadata murni — ROADMAP.md Fase 2, strategi object
// storage MinIO). Handler HANYA parsing/validasi bentuk request lalu
// meneruskan io.Reader mentah ke usecase; checksum, penamaan object key, dan
// upload ke MinIO adalah business logic milik firmware.Service (CLAUDE.md).
func (r *Router) uploadFirmware(c *echo.Context) error {
	actor := ActorFrom(c)

	req := c.Request()
	if req.ContentLength > 0 && req.ContentLength > maxFirmwareRequestBytes {
		return echo.NewHTTPError(http.StatusRequestEntityTooLarge, "request melebihi batas ukuran maksimum")
	}
	req.Body = http.MaxBytesReader(c.Response(), req.Body, maxFirmwareRequestBytes)

	vendorID, err := strconv.ParseUint(strings.TrimSpace(c.FormValue("vendor_id")), 10, 64)
	if err != nil || vendorID == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "vendor_id wajib diisi dan valid")
	}
	var deviceModelID *uint64
	if v := strings.TrimSpace(c.FormValue("device_model_id")); v != "" {
		id, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "device_model_id tidak valid")
		}
		deviceModelID = &id
	}
	version := strings.TrimSpace(c.FormValue("version"))
	if version == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "version wajib diisi")
	}
	var releaseNotes *string
	if v := c.FormValue("release_notes"); strings.TrimSpace(v) != "" {
		releaseNotes = &v
	}

	fileHeader, err := c.FormFile("file")
	if err != nil {
		if strings.Contains(err.Error(), "too large") {
			return echo.NewHTTPError(http.StatusRequestEntityTooLarge, "file firmware melebihi batas ukuran maksimum")
		}
		return echo.NewHTTPError(http.StatusBadRequest, "field file (multipart) wajib diisi")
	}
	if fileHeader.Size <= 0 || fileHeader.Size > domain.MaxFirmwareFileSizeBytes {
		return echo.NewHTTPError(http.StatusRequestEntityTooLarge, "ukuran file firmware tidak valid atau melebihi batas maksimum")
	}
	file, err := fileHeader.Open()
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "gagal membaca file yang diupload")
	}
	defer file.Close()

	contentType := fileHeader.Header.Get("Content-Type")

	f, err := r.Firmware.UploadFirmware(c.Request().Context(), actor, firmware.UploadFirmwareInput{
		VendorID:      vendorID,
		DeviceModelID: deviceModelID,
		Version:       version,
		FileName:      fileHeader.Filename,
		File:          file,
		FileSize:      fileHeader.Size,
		ContentType:   contentType,
		ReleaseNotes:  releaseNotes,
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
