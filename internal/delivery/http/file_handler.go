package http

import (
	"net/http"
	"path"
	"strconv"
	"strings"

	"acs/internal/domain"
	"acs/internal/usecase/file"

	"github.com/labstack/echo/v5"
)

func (r *Router) uploadFile(c *echo.Context) error {
	actor := ActorFrom(c)
	// Batas body HARUS dipasang sebelum FormValue/FormFile (keduanya mem-parse multipart ke disk sementara);
	// BodyLimit global dikecualikan untuk route ini. Tolak cepat bila Content-Length sudah melewati batas.
	maxBody := domain.MaxFirmwareFileSizeBytes + (1 << 20)
	if cl := c.Request().ContentLength; cl > maxBody {
		return echo.NewHTTPError(http.StatusRequestEntityTooLarge, "file terlalu besar")
	}
	c.Request().Body = http.MaxBytesReader(c.Response(), c.Request().Body, maxBody)
	fileTypeStr := c.FormValue("file_type")
	vendorIDStr := c.FormValue("vendor_id")
	modelIDStr := c.FormValue("device_model_id")
	version := c.FormValue("version")
	fileName := c.FormValue("file_name")

	var vendorID *uint64
	if vendorIDStr != "" {
		vid, err := strconv.ParseUint(vendorIDStr, 10, 64)
		if err != nil || vid == 0 {
			return echo.NewHTTPError(http.StatusBadRequest, "vendor_id tidak valid")
		}
		vendorID = &vid
	}

	var modelID *uint64
	if modelIDStr != "" {
		mid, err := strconv.ParseUint(modelIDStr, 10, 64)
		if err != nil || mid == 0 {
			return echo.NewHTTPError(http.StatusBadRequest, "device_model_id tidak valid")
		}
		modelID = &mid
	}

	var verPtr *string
	if version != "" {
		verPtr = &version
	}

	if err := checkMaxLen(lenRule{"file_name", fileName, 255}, lenRule{"version", version, 100}); err != nil {
		return err
	}
	if err := validateUploadName(fileName); err != nil {
		return err
	}
	if err := rejectControl("version", version); err != nil {
		return err
	}
	fh, err := c.FormFile("file")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "file part is missing or invalid")
	}

	if fh.Size > domain.MaxFirmwareFileSizeBytes {
		return echo.NewHTTPError(http.StatusBadRequest, "file too large")
	}

	f, err := fh.Open()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "cannot read uploaded file")
	}
	defer f.Close()

	if fileName == "" {
		// nama dari klien: ambil basename saja (buang direktori gaya Unix/Windows)
		fileName = path.Base(strings.ReplaceAll(fh.Filename, "\\", "/"))
	}
	if err := checkMaxLen(lenRule{"file_name", fileName, 255}); err != nil {
		return err
	}
	if err := validateUploadName(fileName); err != nil {
		return err
	}

	in := file.UploadFileInput{
		TenantID:      actor.TenantID,
		VendorID:      vendorID,
		DeviceModelID: modelID,
		FileType:      domain.FileType(fileTypeStr),
		Version:       verPtr,
		FileName:      fileName,
		File:          f,
		FileSize:      fh.Size,
		ContentType:   fh.Header.Get("Content-Type"),
	}

	res, err := r.Files.UploadFile(c.Request().Context(), actor, in)
	if err != nil {
		return handleErr(c, err)
	}

	return c.JSON(http.StatusCreated, res)
}

func (r *Router) listFiles(c *echo.Context) error {
	actor := ActorFrom(c)
	p := paginationFromQuery(c)

	files, total, err := r.Files.List(c.Request().Context(), actor, p)
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, map[string]interface{}{
		"data": files,
		"meta": map[string]interface{}{
			"total": total,
			"page":  p.Page,
			"limit": p.PageSize,
		},
	})
}

func (r *Router) deleteFile(c *echo.Context) error {
	actor := ActorFrom(c)
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid ID")
	}
	if err := r.Files.Delete(c.Request().Context(), actor, id); err != nil {
		return handleErr(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}
