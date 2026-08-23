package http

import "github.com/labstack/echo/v5"

// metrics mendelegasikan request ke promhttp.Handler (r.MetricsHandler, lihat
// router.go/cmd/acsd/main.go) yang membungkus internal/metrics.Collector.
//
// Handler ini SENGAJA hanya delegasi murni (tanpa logic, tanpa actor/RBAC) —
// endpoint GET /metrics TIDAK diautentikasi, konsisten dengan konvensi umum
// Prometheus exporter (scraper Prometheus tidak mengirim Bearer token/JWT).
// Ini didaftarkan di grup "api" (bukan "authed") di router.go. Di produksi,
// endpoint ini WAJIB difirewall di level jaringan (mis. hanya reachable dari
// subnet Prometheus scraper), BUKAN di level aplikasi — dicatat eksplisit di
// sini karena ini pengecualian terhadap aturan "tidak ada endpoint tanpa
// autentikasi" (CLAUDE.md), yang berlaku untuk endpoint MUTASI state, bukan
// endpoint read-only agregat lintas-tenant seperti ini.
func (r *Router) metrics(c *echo.Context) error {
	r.MetricsHandler.ServeHTTP(c.Response(), c.Request())
	return nil
}
