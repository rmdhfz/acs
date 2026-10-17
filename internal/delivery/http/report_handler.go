package http

import (
	"encoding/csv"
	"net/http"
	"time"

	"github.com/labstack/echo/v5"

	"acs/internal/usecase/report"
)

// getSaaSReport — GET /reports/saas (SUPERADMIN): ukuran pelanggan, pemakaian, kesehatan, tren 30 hari.
func (r *Router) getSaaSReport(c *echo.Context) error {
	rep, err := r.Reports.Build(c.Request().Context(), ActorFrom(c))
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, rep)
}

// exportSaaSTenantsCSV — GET /reports/saas/tenants.csv: tabel tenant untuk manajemen (sel dinetralkan dari rumus).
func (r *Router) exportSaaSTenantsCSV(c *echo.Context) error {
	rep, err := r.Reports.Build(c.Request().Context(), ActorFrom(c))
	if err != nil {
		return handleErr(c, err)
	}
	w := c.Response()
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="laporan-saas-tenant-`+time.Now().Format("20060102")+`.csv"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("\xEF\xBB\xBF")) // BOM: Excel membaca UTF-8 dengan benar
	cw := csv.NewWriter(w)
	if err := cw.WriteAll(report.TenantsCSV(rep)); err != nil {
		return err
	}
	return nil
}
