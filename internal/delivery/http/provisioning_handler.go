package http

import (
	"net/http"

	"github.com/labstack/echo/v5"

	"acs/internal/domain"
	"acs/internal/usecase/provisioning"
)

type profileParamDTO struct {
	ParameterName  string  `json:"parameter_name"`
	ParameterValue *string `json:"parameter_value"`
	ApplyOrder     uint32  `json:"apply_order"`
}

func toDomainParams(in []profileParamDTO) []domain.ProvisioningProfileParameter {
	out := make([]domain.ProvisioningProfileParameter, 0, len(in))
	for _, p := range in {
		out = append(out, domain.ProvisioningProfileParameter{
			ParameterName: p.ParameterName, ParameterValue: p.ParameterValue, ApplyOrder: p.ApplyOrder,
		})
	}
	return out
}

type createProfileRequest struct {
	TenantID      *uint64           `json:"tenant_id"`
	VendorID      *uint64           `json:"vendor_id"`
	DeviceModelID *uint64           `json:"device_model_id"`
	Name          string            `json:"name"`
	Description   *string           `json:"description"`
	IsDefault     bool              `json:"is_default"`
	Parameters    []profileParamDTO `json:"parameters"`
}

func (r *Router) createProfile(c *echo.Context) error {
	actor := ActorFrom(c)
	var req createProfileRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "payload tidak valid")
	}
	if req.Name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name wajib diisi")
	}
	p, err := r.Provisioning.CreateProfile(c.Request().Context(), actor, provisioning.CreateProfileInput{
		TenantID: req.TenantID, VendorID: req.VendorID, DeviceModelID: req.DeviceModelID,
		Name: req.Name, Description: req.Description, IsDefault: req.IsDefault, Parameters: toDomainParams(req.Parameters),
	})
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusCreated, p)
}

func (r *Router) getProfile(c *echo.Context) error {
	actor := ActorFrom(c)
	id, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	p, params, err := r.Provisioning.Get(c.Request().Context(), actor, id)
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"profile": p, "parameters": params})
}

func (r *Router) listProfiles(c *echo.Context) error {
	actor := ActorFrom(c)
	tenantID := actor.TenantID
	if actor.IsSuperadmin() {
		tenantID = queryUint64(c, "tenant_id")
	}
	profiles, total, err := r.Provisioning.List(c.Request().Context(), tenantID, paginationFromQuery(c))
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, listResponse{Data: profiles, Total: total})
}

type updateProfileRequest struct {
	VendorID      *uint64           `json:"vendor_id"`
	DeviceModelID *uint64           `json:"device_model_id"`
	Name          string            `json:"name"`
	Description   *string           `json:"description"`
	IsDefault     bool              `json:"is_default"`
	IsActive      bool              `json:"is_active"`
	Parameters    []profileParamDTO `json:"parameters"`
}

func (r *Router) updateProfile(c *echo.Context) error {
	actor := ActorFrom(c)
	id, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	var req updateProfileRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "payload tidak valid")
	}
	p := &domain.ProvisioningProfile{
		ID: id, VendorID: req.VendorID, DeviceModelID: req.DeviceModelID,
		Name: req.Name, Description: req.Description, IsDefault: req.IsDefault, IsActive: req.IsActive,
	}
	var params []domain.ProvisioningProfileParameter
	if req.Parameters != nil {
		params = toDomainParams(req.Parameters)
	}
	if err := r.Provisioning.UpdateProfile(c.Request().Context(), actor, p, params); err != nil {
		return handleErr(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (r *Router) deleteProfile(c *echo.Context) error {
	actor := ActorFrom(c)
	id, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	if err := r.Provisioning.DeleteProfile(c.Request().Context(), actor, id); err != nil {
		return handleErr(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

// applyProfile SELALU eksplisit — perubahan profile tidak otomatis
// mendorong ulang ke device (FR-18, lihat usecase/provisioning).
func (r *Router) applyProfile(c *echo.Context) error {
	actor := ActorFrom(c)
	deviceID, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id device tidak valid")
	}
	profileID, err := parseUint64Param(c, "profileId")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id profile tidak valid")
	}
	t, err := r.Provisioning.ApplyProfile(c.Request().Context(), actor, deviceID, profileID)
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusCreated, t)
}

// ---- Zero-Touch Rules ----

type ztRuleRequest struct {
	TenantID              *uint64 `json:"tenant_id"`
	VendorID              *uint64 `json:"vendor_id"`
	DeviceModelID         *uint64 `json:"device_model_id"`
	OUI                   *string `json:"oui"`
	SerialPattern         *string `json:"serial_pattern"`
	ProvisioningProfileID uint64  `json:"provisioning_profile_id"`
	Priority              uint32  `json:"priority"`
	IsActive              bool    `json:"is_active"`
}

func (r *Router) listZTRules(c *echo.Context) error {
	actor := ActorFrom(c)
	tenantID := actor.TenantID
	if actor.IsSuperadmin() {
		tenantID = queryUint64(c, "tenant_id")
	}
	rules, err := r.Provisioning.ListZeroTouchRules(c.Request().Context(), tenantID)
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, rules)
}

func (r *Router) createZTRule(c *echo.Context) error {
	actor := ActorFrom(c)
	var req ztRuleRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "payload tidak valid")
	}
	if req.ProvisioningProfileID == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "provisioning_profile_id wajib diisi")
	}
	rule := &domain.ZeroTouchRule{
		TenantID: req.TenantID, VendorID: req.VendorID, DeviceModelID: req.DeviceModelID,
		OUI: req.OUI, SerialPattern: req.SerialPattern, ProvisioningProfileID: req.ProvisioningProfileID,
		Priority: req.Priority, IsActive: true,
	}
	if err := r.Provisioning.CreateZeroTouchRule(c.Request().Context(), actor, rule); err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusCreated, rule)
}

func (r *Router) updateZTRule(c *echo.Context) error {
	actor := ActorFrom(c)
	id, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	var req ztRuleRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "payload tidak valid")
	}
	rule := &domain.ZeroTouchRule{
		ID: id, VendorID: req.VendorID, DeviceModelID: req.DeviceModelID,
		OUI: req.OUI, SerialPattern: req.SerialPattern, ProvisioningProfileID: req.ProvisioningProfileID,
		Priority: req.Priority, IsActive: req.IsActive,
	}
	if err := r.Provisioning.UpdateZeroTouchRule(c.Request().Context(), actor, rule); err != nil {
		return handleErr(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (r *Router) deleteZTRule(c *echo.Context) error {
	actor := ActorFrom(c)
	id, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	if err := r.Provisioning.DeleteZeroTouchRule(c.Request().Context(), actor, id); err != nil {
		return handleErr(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}
