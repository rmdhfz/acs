package http

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v5"
)

// Capture sesi CWMP per perangkat (migrations/0037): hanya ADMIN/SUPERADMIN pada perangkat tenantnya (ditegakkan di usecase,
// rute juga dibatasi). Isi pesan sudah disamarkan sebelum disimpan; ekspor NDJSON dipakai membuat fixture uji.

type captureRequest struct {
	Minutes int `json:"minutes"`
}

// GET /devices/:id/capture
func (r *Router) getCapture(c *echo.Context) error {
	id, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	st, err := r.Capture.Status(c.Request().Context(), ActorFrom(c), id)
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, st)
}

// POST /devices/:id/capture {minutes}
func (r *Router) enableCapture(c *echo.Context) error {
	id, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	var req captureRequest
	if err := decodeStrictJSON(c, &req); err != nil {
		return err
	}
	st, err := r.Capture.Enable(c.Request().Context(), ActorFrom(c), id, req.Minutes)
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, st)
}

// DELETE /devices/:id/capture
func (r *Router) disableCapture(c *echo.Context) error {
	id, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	if err := r.Capture.Disable(c.Request().Context(), ActorFrom(c), id); err != nil {
		return handleErr(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

// GET /devices/:id/captures
func (r *Router) listCaptures(c *echo.Context) error {
	id, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	rows, total, err := r.Capture.List(c.Request().Context(), ActorFrom(c), id, paginationFromQuery(c))
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, listResponse{Data: rows, Total: total})
}

// GET /devices/:id/captures/export — NDJSON (satu pesan per baris, urut waktu naik) sebagai unduhan.
// Rute statis ini didaftarkan SEBELUM /captures/:uuid.
func (r *Router) exportCaptures(c *echo.Context) error {
	id, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	rows, err := r.Capture.Export(c.Request().Context(), ActorFrom(c), id)
	if err != nil {
		return handleErr(c, err)
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	for i := range rows {
		if err := enc.Encode(rows[i]); err != nil {
			return err
		}
	}
	c.Response().Header().Set("Content-Disposition", `attachment; filename="capture-device-`+strconv.FormatUint(id, 10)+`.ndjson"`)
	return c.Blob(http.StatusOK, "application/x-ndjson", buf.Bytes())
}

// GET /devices/:id/captures/:uuid
func (r *Router) getCaptureMessage(c *echo.Context) error {
	id, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	uuid := c.Param("uuid")
	if err := checkMaxLen(lenRule{"uuid", uuid, 36}); err != nil {
		return err
	}
	m, err := r.Capture.Get(c.Request().Context(), ActorFrom(c), id, uuid)
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, m)
}
