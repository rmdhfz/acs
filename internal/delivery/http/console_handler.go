package http

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	"acs/internal/domain"
	"acs/internal/usecase/console"
)

// ---- Fitur Console (migrations/0033): layout menu, laporan bug, riwayat impor/ekspor ----
// Handler tipis: parsing + batas panjang; semua keputusan akses (siapa boleh melihat baris siapa) di usecase/console.
// Identitas pelaku & tenant SELALU dari token (ActorFrom), tidak pernah dari body/query. "id" publik = UUID.

const (
	maxBugTitleBody = 120
	maxBugDescBody  = 4000
	maxBugExpBody   = 2000
	// Batas total body multipart laporan bug (gambar maks 2 MiB + field teks).
	maxBugMultipart = 2<<20 + 256<<10
	maxBugUABody    = 512 // dipotong usecase ke 255; batas kasar agar payload tak membengkak
	maxPathBody     = 512
	maxVersionBody  = 32
	maxNoteBody     = 500
	maxTransferFile = 255
	maxTransferDet  = 500
)

func (r *Router) mountConsoleRoutes(authedSelf, authed *echo.Group) {
	if r.Menu != nil {
		authedSelf.GET("/auth/menu-layout", r.getMenuLayout)
		authedSelf.PUT("/auth/menu-layout", r.putMenuLayout, denyImpersonation)
		authedSelf.DELETE("/auth/menu-layout", r.resetMenuLayout, denyImpersonation)
	}
	if r.Bugs != nil {
		// Semua peran masuk (termasuk ENDUSER) boleh melapor; batas laju per pengguna ada di usecase.
		authedSelf.POST("/bug-reports", r.createBugReport, r.limiter("bug-report", 5))
		authedSelf.GET("/bug-reports", r.listBugReports)
		authedSelf.GET("/bug-reports/:uuid", r.getBugReport)
		authedSelf.GET("/bug-reports/:uuid/screenshot", r.getBugReportScreenshot)
		authedSelf.PATCH("/bug-reports/:uuid", r.updateBugReport, RequireRoles(domain.RoleSuperadmin))
	}
	if r.Transfers != nil {
		authed.GET("/data-transfers", r.listDataTransfers)
		authed.GET("/data-transfers/:uuid", r.getDataTransfer)
		authed.POST("/data-transfers", r.createDataTransfer, r.limiter("data-transfer", 2))
	}
}

// ---- layout menu ----

func (r *Router) getMenuLayout(c *echo.Context) error {
	raw, err := r.Menu.Get(c.Request().Context(), ActorFrom(c))
	if err != nil {
		return handleErr(c, err)
	}
	c.Response().Header().Set("Cache-Control", "no-store")
	if raw == nil {
		return c.JSON(http.StatusOK, map[string]any{"layout": nil})
	}
	return c.JSON(http.StatusOK, map[string]any{"layout": raw})
}

func (r *Router) putMenuLayout(c *echo.Context) error {
	req := c.Request()
	req.Body = http.MaxBytesReader(c.Response(), req.Body, 9<<10)
	body, err := io.ReadAll(req.Body)
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			return echo.NewHTTPError(http.StatusRequestEntityTooLarge, "layout menu terlalu besar")
		}
		return echo.NewHTTPError(http.StatusBadRequest, "payload tidak valid")
	}
	norm, err := r.Menu.Put(req.Context(), ActorFrom(c), body)
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{"layout": json.RawMessage(norm)})
}

func (r *Router) resetMenuLayout(c *echo.Context) error {
	if err := r.Menu.Reset(c.Request().Context(), ActorFrom(c)); err != nil {
		return handleErr(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

// ---- laporan bug ----

type createBugReportRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"` // apa yang terjadi
	Expected    string `json:"expected"`    // apa yang diharapkan
	Severity    string `json:"severity"`
	PagePath    string `json:"page_path"`
	UserAgent   string `json:"user_agent"`
	AppVersion  string `json:"app_version"`
}

// bugFormFields = satu-satunya field teks yang diterima pada multipart (field lain ditolak: tak ada mass assignment).
var bugFormFields = map[string]bool{"title": true, "description": true, "expected": true, "severity": true, "page_path": true, "user_agent": true, "app_version": true}

// readBugRequest menerima JSON (tanpa gambar) ATAU multipart/form-data (field teks + berkas "screenshot" opsional).
func readBugRequest(c *echo.Context) (createBugReportRequest, []byte, error) {
	var req createBugReportRequest
	if !strings.HasPrefix(strings.ToLower(c.Request().Header.Get("Content-Type")), "multipart/form-data") {
		return req, nil, decodeStrictJSON(c, &req)
	}
	hr := c.Request()
	hr.Body = http.MaxBytesReader(c.Response(), hr.Body, maxBugMultipart)
	if err := hr.ParseMultipartForm(1 << 20); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			return req, nil, echo.NewHTTPError(http.StatusRequestEntityTooLarge, "payload terlalu besar (tangkapan layar maks. 2 MB)")
		}
		return req, nil, echo.NewHTTPError(http.StatusBadRequest, "payload tidak valid")
	}
	defer func() {
		if hr.MultipartForm != nil {
			_ = hr.MultipartForm.RemoveAll()
		}
	}()
	for k := range hr.MultipartForm.Value {
		if !bugFormFields[k] {
			return req, nil, echo.NewHTTPError(http.StatusBadRequest, "field tidak dikenal: "+k)
		}
	}
	for k := range hr.MultipartForm.File {
		if k != "screenshot" {
			return req, nil, echo.NewHTTPError(http.StatusBadRequest, "berkas tidak dikenal: "+k)
		}
	}
	val := func(k string) string {
		if v := hr.MultipartForm.Value[k]; len(v) > 0 {
			return v[0]
		}
		return ""
	}
	req = createBugReportRequest{
		Title: val("title"), Description: val("description"), Expected: val("expected"), Severity: val("severity"),
		PagePath: val("page_path"), UserAgent: val("user_agent"), AppVersion: val("app_version"),
	}
	var shot []byte
	if fhs := hr.MultipartForm.File["screenshot"]; len(fhs) > 0 {
		if len(fhs) > 1 {
			return req, nil, echo.NewHTTPError(http.StatusBadRequest, "hanya satu tangkapan layar")
		}
		fh := fhs[0]
		if fh.Size > 2<<20 {
			return req, nil, echo.NewHTTPError(http.StatusRequestEntityTooLarge, "tangkapan layar maks. 2 MB")
		}
		f, err := fh.Open()
		if err != nil {
			return req, nil, echo.NewHTTPError(http.StatusBadRequest, "tangkapan layar tidak dapat dibaca")
		}
		defer f.Close()
		if shot, err = io.ReadAll(io.LimitReader(f, 2<<20+1)); err != nil {
			return req, nil, echo.NewHTTPError(http.StatusBadRequest, "tangkapan layar tidak dapat dibaca")
		}
	}
	return req, shot, nil
}

func (r *Router) createBugReport(c *echo.Context) error {
	req, shot, err := readBugRequest(c)
	if err != nil {
		return err
	}
	if err := checkMaxLen(
		lenRule{"title", req.Title, maxBugTitleBody}, lenRule{"description", req.Description, maxBugDescBody},
		lenRule{"expected", req.Expected, maxBugExpBody},
		lenRule{"severity", req.Severity, 16}, lenRule{"page_path", req.PagePath, maxPathBody},
		lenRule{"user_agent", req.UserAgent, maxBugUABody}, lenRule{"app_version", req.AppVersion, maxVersionBody},
	); err != nil {
		return err
	}
	rep, err := r.Bugs.Create(c.Request().Context(), ActorFrom(c), console.CreateBugInput{
		Title: req.Title, Description: req.Description, Expected: req.Expected, Severity: req.Severity,
		PagePath: req.PagePath, UserAgent: req.UserAgent, AppVersion: req.AppVersion, Screenshot: shot,
	})
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusCreated, rep)
}

// getBugReportScreenshot — GET /bug-reports/:uuid/screenshot: gambar hanya untuk pelapor sendiri atau SUPERADMIN.
// Jenis konten ditetapkan server (PNG/JPEG hasil validasi); nosniff + no-store.
func (r *Router) getBugReportScreenshot(c *echo.Context) error {
	sc, err := r.Bugs.GetScreenshot(c.Request().Context(), ActorFrom(c), c.Param("uuid"))
	if err != nil {
		return handleErr(c, err)
	}
	h := c.Response().Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", "private, no-store")
	h.Set("Content-Disposition", "inline")
	return c.Blob(http.StatusOK, sc.ContentType, sc.Data)
}

func (r *Router) listBugReports(c *echo.Context) error {
	tid, err := queryTenantID(c)
	if err != nil {
		return err
	}
	status := strings.ToUpper(strings.TrimSpace(c.QueryParam("status")))
	p := paginationFromQuery(c)
	rows, total, err := r.Bugs.List(c.Request().Context(), ActorFrom(c), tid, status, p)
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, pagedBody(rows, total, p))
}

func (r *Router) getBugReport(c *echo.Context) error {
	rep, err := r.Bugs.Get(c.Request().Context(), ActorFrom(c), c.Param("uuid"))
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, rep)
}

type updateBugReportRequest struct {
	Status string  `json:"status"`
	Note   *string `json:"note"`
}

func (r *Router) updateBugReport(c *echo.Context) error {
	var req updateBugReportRequest
	if err := decodeStrictJSON(c, &req); err != nil {
		return err
	}
	if err := checkMaxLen(lenRule{"status", req.Status, 16}, lenRule{"note", optStr(req.Note), maxNoteBody}); err != nil {
		return err
	}
	rep, err := r.Bugs.UpdateStatus(c.Request().Context(), ActorFrom(c), c.Param("uuid"), strings.ToUpper(strings.TrimSpace(req.Status)), req.Note)
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, rep)
}

// ---- riwayat impor/ekspor ----

type createDataTransferRequest struct {
	Kind        string `json:"kind"`
	Status      string `json:"status"`
	Module      string `json:"module"`
	Format      string `json:"format"`
	FileName    string `json:"file_name"`
	TotalRows   int64  `json:"total_rows"`
	SuccessRows int64  `json:"success_rows"`
	FailedRows  int64  `json:"failed_rows"`
	Detail      string `json:"detail"`
}

func (r *Router) createDataTransfer(c *echo.Context) error {
	var req createDataTransferRequest
	if err := decodeStrictJSON(c, &req); err != nil {
		return err
	}
	if err := checkMaxLen(
		lenRule{"kind", req.Kind, 16}, lenRule{"status", req.Status, 16}, lenRule{"module", req.Module, 48},
		lenRule{"format", req.Format, 16}, lenRule{"file_name", req.FileName, maxTransferFile}, lenRule{"detail", req.Detail, maxTransferDet},
	); err != nil {
		return err
	}
	rec, err := r.Transfers.RecordClient(c.Request().Context(), ActorFrom(c), console.RecordInput{
		Kind: strings.ToUpper(req.Kind), Status: strings.ToUpper(req.Status), Module: strings.ToLower(req.Module), Format: req.Format,
		FileName: req.FileName, TotalRows: req.TotalRows, SuccessRows: req.SuccessRows, FailedRows: req.FailedRows, Detail: req.Detail,
	})
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusCreated, rec)
}

func (r *Router) listDataTransfers(c *echo.Context) error {
	tid, err := queryTenantID(c)
	if err != nil {
		return err
	}
	kind := strings.ToUpper(strings.TrimSpace(c.QueryParam("kind")))
	module := strings.ToLower(strings.TrimSpace(c.QueryParam("module")))
	p := paginationFromQuery(c)
	rows, total, err := r.Transfers.List(c.Request().Context(), ActorFrom(c), tid, kind, module, p)
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, pagedBody(rows, total, p))
}

func (r *Router) getDataTransfer(c *echo.Context) error {
	rec, err := r.Transfers.Get(c.Request().Context(), ActorFrom(c), c.Param("uuid"))
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, rec)
}

// pagedBody = bentuk daftar standar ({data, meta}) seperti notifikasi/billing.
func pagedBody(rows any, total int, p domain.Pagination) map[string]any {
	page := p.Page
	if page < 1 {
		page = 1
	}
	return map[string]any{"data": rows, "meta": map[string]int{"total": total, "page": page, "limit": p.Limit()}}
}
