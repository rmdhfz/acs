package http

import (
	"net/http"

	"acs/internal/domain"

	"github.com/labstack/echo/v5"
)

// Resource faults (migrations/0038): cwmp:Fault aktif dari CPE. Baca terbuka untuk semua peran staf (scope tenant ditegakkan
// di usecase); menutup fault dibatasi ADMIN/NOC. Identifier di API adalah UUID, PK tidak pernah diekspos.

// GET /faults?device_id=&fault_code=&rpc_method=&search=&tenant_id=
func (r *Router) listFaults(c *echo.Context) error {
	f := domain.FaultFilter{
		DeviceID:  queryUint64(c, "device_id"),
		FaultCode: c.QueryParam("fault_code"),
		RPCMethod: c.QueryParam("rpc_method"),
		Search:    c.QueryParam("search"),
		// tenant_id dari query hanya efektif untuk superadmin (Faults.List menimpanya untuk peran lain).
		TenantID: queryUint64(c, "tenant_id"),
	}
	if err := checkMaxLen(lenRule{"fault_code", f.FaultCode, domain.FaultCodeMaxLen}, lenRule{"rpc_method", f.RPCMethod, 64},
		lenRule{"search", f.Search, 255}); err != nil {
		return err
	}
	rows, total, err := r.Faults.List(c.Request().Context(), ActorFrom(c), f, paginationFromQuery(c))
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, listResponse{Data: rows, Total: total})
}

// GET /faults/:uuid
func (r *Router) getFault(c *echo.Context) error {
	uuid := c.Param("uuid")
	if err := checkMaxLen(lenRule{"uuid", uuid, 36}); err != nil {
		return err
	}
	f, err := r.Faults.Get(c.Request().Context(), ActorFrom(c), uuid)
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, f)
}

// DELETE /faults/:uuid — tutup fault (muncul lagi bila CPE mengulangnya).
func (r *Router) dismissFault(c *echo.Context) error {
	uuid := c.Param("uuid")
	if err := checkMaxLen(lenRule{"uuid", uuid, 36}); err != nil {
		return err
	}
	if err := r.Faults.Dismiss(c.Request().Context(), ActorFrom(c), uuid); err != nil {
		return handleErr(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}
