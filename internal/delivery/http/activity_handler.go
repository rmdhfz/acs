package http

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v5"

	"acs/internal/domain"
	"acs/internal/usecase/auth"
)

// listActivity — audit trail global tenant-scoped (GET /activity).
// Gated ADMIN/SUPERADMIN di router. Superadmin melihat lintas tenant; admin
// dibatasi ke tenant-nya (auth.ScopedTenantFilter). Handler tipis: hanya
// scoping + parsing filter, tanpa logic bisnis — activity_logs belum punya
// lapisan usecase sendiri (debt pra-eksisting, sama seperti catalog_handler).
func (r *Router) listActivity(c *echo.Context) error {
	actor := ActorFrom(c)
	tenantID, err := auth.ScopedTenantFilter(actor)
	if err != nil {
		if errors.Is(err, domain.ErrForbidden) {
			return echo.NewHTTPError(http.StatusForbidden, "akun tanpa tenant_id tidak bisa melihat audit trail")
		}
		return handleErr(c, err)
	}
	f := domain.ActivityLogFilter{
		Action:     c.QueryParam("action"),
		EntityType: c.QueryParam("entity_type"),
	}
	rows, total, err := r.Activity.ListByTenant(c.Request().Context(), tenantID, f, paginationFromQuery(c))
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, listResponse{Data: rows, Total: total})
}
