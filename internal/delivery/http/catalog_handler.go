package http

import (
	"net/http"

	"github.com/labstack/echo/v5"

	"acs/internal/domain"
)

func (r *Router) listVendors(c *echo.Context) error {
	vendors, total, err := r.Vendors.List(c.Request().Context(), paginationFromQuery(c))
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, listResponse{Data: vendors, Total: total})
}

type createVendorRequest struct {
	Code        string  `json:"code"`
	Name        string  `json:"name"`
	Description *string `json:"description"`
}

func (r *Router) createVendor(c *echo.Context) error {
	actor := ActorFrom(c)
	var req createVendorRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "payload tidak valid")
	}
	if req.Code == "" || req.Name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "code dan name wajib diisi")
	}
	if err := checkMaxLen(
		lenRule{"code", req.Code, maxVendorCode},
		lenRule{"name", req.Name, maxVendorName},
		lenRule{"description", optStr(req.Description), maxDescription},
	); err != nil {
		return err
	}
	v := &domain.Vendor{
		Code: req.Code, Name: req.Name, Description: req.Description, IsActive: true,
		Audit: domain.Audit{CreatedBy: actor.UserIDPtr()},
	}
	if err := r.Vendors.Create(c.Request().Context(), v); err != nil {
		return handleErr(c, err)
	}
	_ = r.Activity.Record(c.Request().Context(), &domain.ActivityLog{
		UserID: actor.UserIDPtr(), Action: "CREATE_VENDOR", EntityType: "vendor", EntityID: &v.ID,
	})
	return c.JSON(http.StatusCreated, v)
}

type addOUIRequest struct {
	OUI   string  `json:"oui"`
	Notes *string `json:"notes"`
}

func (r *Router) addVendorOUI(c *echo.Context) error {
	actor := ActorFrom(c)
	vendorID, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	var req addOUIRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "payload tidak valid")
	}
	if req.OUI == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "oui wajib diisi")
	}
	if err := checkMaxLen(
		lenRule{"oui", req.OUI, maxOUI},
		lenRule{"notes", optStr(req.Notes), maxOUINotes},
	); err != nil {
		return err
	}
	o := &domain.VendorOUI{
		VendorID: vendorID, OUI: req.OUI, Notes: req.Notes, Audit: domain.Audit{CreatedBy: actor.UserIDPtr()},
	}
	if err := r.VendorOUIs.Create(c.Request().Context(), o); err != nil {
		return handleErr(c, err)
	}
	_ = r.Activity.Record(c.Request().Context(), &domain.ActivityLog{
		UserID: actor.UserIDPtr(), Action: "ADD_VENDOR_OUI", EntityType: "vendor", EntityID: &vendorID,
	})
	return c.JSON(http.StatusCreated, o)
}

func (r *Router) listVendorOUIs(c *echo.Context) error {
	vendorID, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	ouis, err := r.VendorOUIs.ListByVendor(c.Request().Context(), vendorID)
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, ouis)
}

func (r *Router) listDeviceModels(c *echo.Context) error {
	vendorID := queryUint64(c, "vendor_id")
	if vendorID == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "vendor_id wajib diisi")
	}
	models, err := r.DeviceModels.ListByVendor(c.Request().Context(), *vendorID)
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, models)
}

type createDeviceModelRequest struct {
	VendorID           uint64  `json:"vendor_id"`
	DeviceTypeID       uint64  `json:"device_type_id"`
	DataModelVersionID uint64  `json:"data_model_version_id"`
	ProductClass       *string `json:"product_class"`
	ModelName          string  `json:"model_name"`
	Description        *string `json:"description"`
}

func (r *Router) createDeviceModel(c *echo.Context) error {
	actor := ActorFrom(c)
	var req createDeviceModelRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "payload tidak valid")
	}
	if req.VendorID == 0 || req.ModelName == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "vendor_id dan model_name wajib diisi")
	}
	if err := checkMaxLen(
		lenRule{"model_name", req.ModelName, maxModelName},
		lenRule{"product_class", optStr(req.ProductClass), maxProductClass},
		lenRule{"description", optStr(req.Description), maxDescription},
	); err != nil {
		return err
	}
	m := &domain.DeviceModel{
		VendorID: req.VendorID, DeviceTypeID: req.DeviceTypeID, DataModelVersionID: req.DataModelVersionID,
		ProductClass: req.ProductClass, ModelName: req.ModelName, Description: req.Description, IsActive: true,
		Audit: domain.Audit{CreatedBy: actor.UserIDPtr()},
	}
	if err := r.DeviceModels.Create(c.Request().Context(), m); err != nil {
		return handleErr(c, err)
	}
	_ = r.Activity.Record(c.Request().Context(), &domain.ActivityLog{
		UserID: actor.UserIDPtr(), Action: "CREATE_DEVICE_MODEL", EntityType: "device_model", EntityID: &m.ID,
	})
	return c.JSON(http.StatusCreated, m)
}

// listParameterMappings — daftar mapping milik satu vendor untuk Catalog UI.
// Read-only dan dibuka ke semua role terautentikasi, konsisten dgn
// GET /vendors dan GET /device-models: vendor_parameter_mappings adalah data
// referensi GLOBAL lintas tenant (tidak py tenant_id), jadi tidak ada
// filter tenant di sini — yang dibatasi superadmin adalah MUTASInya (POST).
func (r *Router) listParameterMappings(c *echo.Context) error {
	vendorID := queryUint64(c, "vendor_id")
	if vendorID == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "vendor_id wajib diisi")
	}
	mappings, err := r.ParamMappings.ListByVendor(c.Request().Context(), *vendorID)
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, mappings)
}

type upsertMappingRequest struct {
	VendorID           uint64  `json:"vendor_id"`
	DataModelVersionID uint64  `json:"data_model_version_id"`
	DeviceModelID      *uint64 `json:"device_model_id"`
	// SoftwareVersionPattern (migrations/0010) — pola SQL LIKE opsional thd
	// devices.software_version, dievaluasi gaya sama seperti
	// zero_touch_rules.serial_pattern. NULL = mapping generik lintas versi.
	SoftwareVersionPattern *string `json:"software_version_pattern"`
	LogicalKey             string  `json:"logical_key"`
	TR069Path              string  `json:"tr069_path"`
	ParameterTypeID        *uint64 `json:"parameter_type_id"`
	Description            *string `json:"description"`
}

// upsertParameterMapping — menambah vendor/model baru adalah operasi data
// (CLAUDE.md); endpoint ini bagian dari operasi data tsb, dibatasi superadmin
// karena vendor_parameter_mappings adalah data referensi global lintas tenant.
func (r *Router) upsertParameterMapping(c *echo.Context) error {
	actor := ActorFrom(c)
	var req upsertMappingRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "payload tidak valid")
	}
	if req.VendorID == 0 || req.LogicalKey == "" || req.TR069Path == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "vendor_id, logical_key, tr069_path wajib diisi")
	}
	if err := checkMaxLen(
		lenRule{"logical_key", req.LogicalKey, maxLogicalKey},
		lenRule{"tr069_path", req.TR069Path, maxTR069Path},
		lenRule{"software_version_pattern", optStr(req.SoftwareVersionPattern), maxSoftwareVerPatt},
		lenRule{"description", optStr(req.Description), maxDescription},
	); err != nil {
		return err
	}
	m := &domain.VendorParameterMapping{
		VendorID: req.VendorID, DataModelVersionID: req.DataModelVersionID, DeviceModelID: req.DeviceModelID,
		SoftwareVersionPattern: req.SoftwareVersionPattern,
		LogicalKey:             req.LogicalKey, TR069Path: req.TR069Path, ParameterTypeID: req.ParameterTypeID, Description: req.Description,
		Audit: domain.Audit{CreatedBy: actor.UserIDPtr()},
	}
	if err := r.ParamMappings.Upsert(c.Request().Context(), m); err != nil {
		return handleErr(c, err)
	}
	_ = r.Activity.Record(c.Request().Context(), &domain.ActivityLog{
		UserID: actor.UserIDPtr(), Action: "UPSERT_VENDOR_PARAMETER_MAPPING", EntityType: "vendor_parameter_mapping", EntityID: &m.ID,
	})
	return c.JSON(http.StatusOK, m)
}
