package http

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/labstack/echo/v5"
)

// ReadinessCheck — satu dependensi yang diperiksa /readyz. Critical=true: bila gagal, ACS TIDAK siap
// (orkestrator berhenti mengarahkan trafik); Critical=false: hanya dilaporkan (mis. Redis, cache opsional).
type ReadinessCheck struct {
	Name     string
	Critical bool
	Fn       func(ctx context.Context) error
}

const readinessTimeout = 2 * time.Second

// healthz — liveness: proses hidup dan event loop HTTP merespons. Tanpa dependensi sama sekali
// supaya orkestrator tidak me-restart ACS hanya karena DB sedang bermasalah.
func (r *Router) healthz(c *echo.Context) error {
	c.Response().Header().Set("Cache-Control", "no-store")
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// readyz — readiness: memeriksa dependensi paralel dengan batas waktu. Respons hanya memuat status
// per dependensi (ok/down), TIDAK pernah teks error (bisa memuat host/DSN). Endpoint publik read-only
// tanpa data pengguna, sengaja tanpa auth agar bisa dipanggil probe orkestrator/load balancer.
func (r *Router) readyz(c *echo.Context) error {
	ctx, cancel := context.WithTimeout(c.Request().Context(), readinessTimeout)
	defer cancel()

	statuses := make([]string, len(r.Readiness))
	var wg sync.WaitGroup
	for i, chk := range r.Readiness {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := chk.Fn(ctx); err != nil {
				statuses[i] = "down"
			} else {
				statuses[i] = "ok"
			}
		}()
	}
	wg.Wait()

	ready := true
	checks := make(map[string]string, len(r.Readiness))
	for i, chk := range r.Readiness {
		checks[chk.Name] = statuses[i]
		if statuses[i] != "ok" && chk.Critical {
			ready = false
		}
	}
	code, status := http.StatusOK, "ready"
	if !ready {
		code, status = http.StatusServiceUnavailable, "not_ready"
	}
	c.Response().Header().Set("Cache-Control", "no-store")
	return c.JSON(code, map[string]any{"status": status, "checks": checks})
}
