package http

import (
	"net/http"

	"acs/internal/domain"
	"acs/internal/usecase/vparam"

	"github.com/labstack/echo/v5"
)

// Virtual parameter (migrations/0039, pkg/vexpr): definisi ekspresi per tenant + nilai turunan per perangkat. Baca terbuka untuk
// semua peran staf (scope tenant di usecase); mutasi & uji ekspresi ADMIN/SUPERADMIN. Identifier di API = UUID.

type vparamRequest struct {
	TenantID    *uint64 `json:"tenant_id"` // hanya efektif untuk SUPERADMIN (null = global)
	Name        string  `json:"name"`
	Label       string  `json:"label"`
	Description *string `json:"description"`
	Expression  string  `json:"expression"`
	Unit        *string `json:"unit"`
	IsActive    *bool   `json:"is_active"`
}

type vparamPatch struct {
	Name        *string `json:"name"`
	Label       *string `json:"label"`
	Description *string `json:"description"`
	Expression  *string `json:"expression"`
	Unit        *string `json:"unit"`
	IsActive    *bool   `json:"is_active"`
}

func (q vparamRequest) lens() []lenRule {
	return []lenRule{
		{"name", q.Name, domain.VParamNameMaxLen}, {"label", q.Label, domain.VParamLabelMaxLen},
		{"description", optStr(q.Description), domain.VParamDescriptionMaxLen}, {"expression", q.Expression, 1000},
		{"unit", optStr(q.Unit), domain.VParamUnitMaxLen},
	}
}

// GET /virtual-parameters?tenant_id=
func (r *Router) listVirtualParameters(c *echo.Context) error {
	rows, total, err := r.VParams.List(c.Request().Context(), ActorFrom(c), queryUint64(c, "tenant_id"), paginationFromQuery(c))
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, listResponse{Data: rows, Total: total})
}

// GET /virtual-parameters/:uuid
func (r *Router) getVirtualParameter(c *echo.Context) error {
	uuid := c.Param("uuid")
	if err := checkMaxLen(lenRule{"uuid", uuid, 36}); err != nil {
		return err
	}
	v, err := r.VParams.Get(c.Request().Context(), ActorFrom(c), uuid)
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, v)
}

// POST /virtual-parameters
func (r *Router) createVirtualParameter(c *echo.Context) error {
	var req vparamRequest
	if err := decodeStrictJSON(c, &req); err != nil {
		return err
	}
	if err := checkMaxLen(req.lens()...); err != nil {
		return err
	}
	v, err := r.VParams.Create(c.Request().Context(), ActorFrom(c), vparam.Input{
		TenantID: req.TenantID, Name: req.Name, Label: req.Label, Description: req.Description,
		Expression: req.Expression, Unit: req.Unit, IsActive: req.IsActive,
	})
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusCreated, v)
}

// PATCH /virtual-parameters/:uuid
func (r *Router) updateVirtualParameter(c *echo.Context) error {
	uuid := c.Param("uuid")
	if err := checkMaxLen(lenRule{"uuid", uuid, 36}); err != nil {
		return err
	}
	var req vparamPatch
	if err := decodeStrictJSON(c, &req); err != nil {
		return err
	}
	if err := checkMaxLen(
		lenRule{"name", optStr(req.Name), domain.VParamNameMaxLen}, lenRule{"label", optStr(req.Label), domain.VParamLabelMaxLen},
		lenRule{"description", optStr(req.Description), domain.VParamDescriptionMaxLen}, lenRule{"expression", optStr(req.Expression), 1000},
		lenRule{"unit", optStr(req.Unit), domain.VParamUnitMaxLen},
	); err != nil {
		return err
	}
	v, err := r.VParams.Update(c.Request().Context(), ActorFrom(c), uuid, vparam.UpdateInput{
		Name: req.Name, Label: req.Label, Description: req.Description, Expression: req.Expression, Unit: req.Unit, IsActive: req.IsActive,
	})
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, v)
}

// DELETE /virtual-parameters/:uuid
func (r *Router) deleteVirtualParameter(c *echo.Context) error {
	uuid := c.Param("uuid")
	if err := checkMaxLen(lenRule{"uuid", uuid, 36}); err != nil {
		return err
	}
	if err := r.VParams.Delete(c.Request().Context(), ActorFrom(c), uuid); err != nil {
		return handleErr(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

type vparamTestRequest struct {
	Expression string `json:"expression"`
	DeviceID   uint64 `json:"device_id"`
}

// POST /virtual-parameters/test {expression, device_id} — coba ekspresi pada satu perangkat tanpa menyimpan apa pun.
func (r *Router) testVirtualParameter(c *echo.Context) error {
	var req vparamTestRequest
	if err := decodeStrictJSON(c, &req); err != nil {
		return err
	}
	if req.DeviceID == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "device_id wajib diisi")
	}
	if err := checkMaxLen(lenRule{"expression", req.Expression, 1000}); err != nil {
		return err
	}
	res, err := r.VParams.Test(c.Request().Context(), ActorFrom(c), req.Expression, req.DeviceID)
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, res)
}

// GET /devices/:id/virtual-parameters — nilai semua virtual parameter aktif untuk perangkat ini.
func (r *Router) deviceVirtualParameters(c *echo.Context) error {
	id, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	vals, err := r.VParams.ForDevice(c.Request().Context(), ActorFrom(c), id)
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, vals)
}
