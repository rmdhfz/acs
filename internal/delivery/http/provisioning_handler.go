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
	if err := checkMaxLen(
		lenRule{"name", req.Name, maxProfileName},
		lenRule{"description", optStr(req.Description), maxDescription},
	); err != nil {
		return err
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
	profiles, total, err := r.Provisioning.List(c.Request().Context(), actor, queryUint64(c, "tenant_id"), paginationFromQuery(c))
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
	if err := checkMaxLen(
		lenRule{"name", req.Name, maxProfileName},
		lenRule{"description", optStr(req.Description), maxDescription},
	); err != nil {
		return err
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

// ztRuleRequest — DTO request Zero-Touch Rule. ProvisioningProfileID sekarang
// OPSIONAL (migrations/0009): rule boleh hanya memicu PostApplyReboot dan/atau
// FirmwareFileID tanpa menerapkan profile parameter apa pun -- lihat
// domain.ZeroTouchRule. TriggerEventID WAJIB diisi klien (raw FK id ke
// ref_ztp_trigger_event, mengikuti pola VendorID/DeviceModelID di struct ini
// yang juga raw id, bukan resolusi dari kode string -- klien mengambil id
// via endpoint generik GET /refs/ref_ztp_trigger_event).
type ztRuleRequest struct {
	TenantID                   *uint64 `json:"tenant_id"`
	VendorID                   *uint64 `json:"vendor_id"`
	DeviceModelID              *uint64 `json:"device_model_id"`
	OUI                        *string `json:"oui"`
	SerialPattern              *string `json:"serial_pattern"`
	SoftwareVersionPattern     *string `json:"software_version_pattern"`
	MatchParameterName         *string `json:"match_parameter_name"`
	MatchParameterValuePattern *string `json:"match_parameter_value_pattern"`
	ProvisioningProfileID      *uint64 `json:"provisioning_profile_id"`
	PostApplyReboot            bool    `json:"post_apply_reboot"`
	FirmwareFileID             *uint64 `json:"firmware_file_id"`
	TriggerEventID             uint64  `json:"trigger_event_id"`
	Priority                   uint32  `json:"priority"`
	IsActive                   bool    `json:"is_active"`
}

// checkZTRuleLen — batas panjang kolom zero_touch_rules (schema.sql), dipakai
// createZTRule dan updateZTRule supaya keduanya tidak bisa menyimpang.
func checkZTRuleLen(req ztRuleRequest) error {
	return checkMaxLen(
		lenRule{"oui", optStr(req.OUI), maxOUI},
		lenRule{"serial_pattern", optStr(req.SerialPattern), maxSerialPattern},
		lenRule{"software_version_pattern", optStr(req.SoftwareVersionPattern), maxSoftwareVerPatt},
		lenRule{"match_parameter_name", optStr(req.MatchParameterName), maxMatchParamName},
		lenRule{"match_parameter_value_pattern", optStr(req.MatchParameterValuePattern), maxMatchParamValPatt},
	)
}

func (r *Router) listZTRules(c *echo.Context) error {
	actor := ActorFrom(c)
	rules, err := r.Provisioning.ListZeroTouchRules(c.Request().Context(), actor, queryUint64(c, "tenant_id"))
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
	// ProvisioningProfileID sekarang OPSIONAL (migrations/0009) -- rule boleh
	// hanya memicu reboot/firmware, jadi TIDAK divalidasi wajib diisi di sini
	// lagi. TriggerEventID TETAP wajib (kolom NOT NULL, FK ref_ztp_trigger_event).
	if req.TriggerEventID == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "trigger_event_id wajib diisi")
	}
	if err := checkZTRuleLen(req); err != nil {
		return err
	}
	rule := &domain.ZeroTouchRule{
		TenantID: req.TenantID, VendorID: req.VendorID, DeviceModelID: req.DeviceModelID,
		OUI: req.OUI, SerialPattern: req.SerialPattern, SoftwareVersionPattern: req.SoftwareVersionPattern,
		MatchParameterName: req.MatchParameterName, MatchParameterValuePattern: req.MatchParameterValuePattern,
		ProvisioningProfileID: req.ProvisioningProfileID, PostApplyReboot: req.PostApplyReboot,
		FirmwareFileID: req.FirmwareFileID, TriggerEventID: req.TriggerEventID,
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
	if req.TriggerEventID == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "trigger_event_id wajib diisi")
	}
	if err := checkZTRuleLen(req); err != nil {
		return err
	}
	rule := &domain.ZeroTouchRule{
		ID: id, VendorID: req.VendorID, DeviceModelID: req.DeviceModelID,
		OUI: req.OUI, SerialPattern: req.SerialPattern, SoftwareVersionPattern: req.SoftwareVersionPattern,
		MatchParameterName: req.MatchParameterName, MatchParameterValuePattern: req.MatchParameterValuePattern,
		ProvisioningProfileID: req.ProvisioningProfileID, PostApplyReboot: req.PostApplyReboot,
		FirmwareFileID: req.FirmwareFileID, TriggerEventID: req.TriggerEventID,
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
