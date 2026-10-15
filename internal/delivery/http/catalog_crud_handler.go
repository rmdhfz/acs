package http

import (
	"net/http"

	"github.com/labstack/echo/v5"

	"acs/internal/usecase/catalog"
)

// Handler CRUD katalog vendor (get/update/delete). Create & list ada di
// catalog_handler.go. Handler hanya parsing/validasi lalu memanggil
// usecase/catalog — aturan "masih dipakai -> 409" ada di usecase.

func (r *Router) getVendor(c *echo.Context) error {
	id, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	v, err := r.Catalog.GetVendor(c.Request().Context(), id)
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, v)
}

type updateVendorRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	IsActive    *bool   `json:"is_active"`
}

func (r *Router) updateVendor(c *echo.Context) error {
	id, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	var req updateVendorRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "payload tidak valid")
	}
	if req.Name == nil && req.Description == nil && req.IsActive == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "minimal satu field (name, description, is_active) harus diisi")
	}
	if err := checkMaxLen(
		lenRule{"name", optStr(req.Name), maxVendorName},
		lenRule{"description", optStr(req.Description), maxDescription},
	); err != nil {
		return err
	}
	v, err := r.Catalog.UpdateVendor(c.Request().Context(), ActorFrom(c), id, catalog.UpdateVendorInput(req))
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, v)
}

func (r *Router) deleteVendor(c *echo.Context) error {
	id, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	if err := r.Catalog.DeleteVendor(c.Request().Context(), ActorFrom(c), id); err != nil {
		return handleErr(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (r *Router) deleteVendorOUI(c *echo.Context) error {
	vendorID, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	ouiID, err := parseUint64Param(c, "ouiId")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "ouiId tidak valid")
	}
	if err := r.Catalog.DeleteVendorOUI(c.Request().Context(), ActorFrom(c), vendorID, ouiID); err != nil {
		return handleErr(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (r *Router) getDeviceModel(c *echo.Context) error {
	id, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	m, err := r.Catalog.GetDeviceModel(c.Request().Context(), id)
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, m)
}

type updateDeviceModelRequest struct {
	DeviceTypeID       *uint64 `json:"device_type_id"`
	DataModelVersionID *uint64 `json:"data_model_version_id"`
	ProductClass       *string `json:"product_class"`
	ModelName          *string `json:"model_name"`
	Description        *string `json:"description"`
	IsActive           *bool   `json:"is_active"`
}

func (r *Router) updateDeviceModel(c *echo.Context) error {
	id, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	var req updateDeviceModelRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "payload tidak valid")
	}
	if req.DeviceTypeID == nil && req.DataModelVersionID == nil && req.ProductClass == nil &&
		req.ModelName == nil && req.Description == nil && req.IsActive == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "minimal satu field harus diisi")
	}
	if err := checkMaxLen(
		lenRule{"model_name", optStr(req.ModelName), maxModelName},
		lenRule{"product_class", optStr(req.ProductClass), maxProductClass},
		lenRule{"description", optStr(req.Description), maxDescription},
	); err != nil {
		return err
	}
	m, err := r.Catalog.UpdateDeviceModel(c.Request().Context(), ActorFrom(c), id, catalog.UpdateDeviceModelInput(req))
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, m)
}

func (r *Router) deleteDeviceModel(c *echo.Context) error {
	id, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	if err := r.Catalog.DeleteDeviceModel(c.Request().Context(), ActorFrom(c), id); err != nil {
		return handleErr(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (r *Router) getParameterMapping(c *echo.Context) error {
	id, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	m, err := r.Catalog.GetParameterMapping(c.Request().Context(), id)
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, m)
}

func (r *Router) deleteParameterMapping(c *echo.Context) error {
	id, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	if err := r.Catalog.DeleteParameterMapping(c.Request().Context(), ActorFrom(c), id); err != nil {
		return handleErr(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}
