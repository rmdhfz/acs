package http

import (
	"net/http"

	"acs/internal/domain"
	"acs/internal/usecase/automation"

	"github.com/labstack/echo/v5"
)

// Aturan otomasi kondisi-aksi (migrations/0040, pkg/vexpr). Baca (aturan + riwayat) ADMIN/NOC; mutasi, uji, dan reset ADMIN;
// scope tenant ditegakkan di usecase. Identifier di API = UUID.

type automationRequest struct {
	TenantID            *uint64                   `json:"tenant_id"` // wajib bagi SUPERADMIN; diabaikan untuk peran lain
	Name                string                    `json:"name"`
	Description         *string                   `json:"description"`
	TriggerEvent        string                    `json:"trigger_event"`
	Condition           string                    `json:"condition"`
	Actions             []domain.AutomationAction `json:"actions"`
	CooldownSeconds     uint32                    `json:"cooldown_seconds"`
	MaxConsecutiveFires uint8                     `json:"max_consecutive_fires"`
	Priority            *uint32                   `json:"priority"`
	IsActive            *bool                     `json:"is_active"`
}

type automationPatch struct {
	Name                *string                    `json:"name"`
	Description         *string                    `json:"description"`
	TriggerEvent        *string                    `json:"trigger_event"`
	Condition           *string                    `json:"condition"`
	Actions             *[]domain.AutomationAction `json:"actions"`
	CooldownSeconds     *uint32                    `json:"cooldown_seconds"`
	MaxConsecutiveFires *uint8                     `json:"max_consecutive_fires"`
	Priority            *uint32                    `json:"priority"`
	IsActive            *bool                      `json:"is_active"`
}

// actionRules membatasi panjang teks tiap aksi (selain itu ditegakkan usecase).
func actionRules(actions []domain.AutomationAction) []lenRule {
	var out []lenRule
	for _, a := range actions {
		out = append(out, lenRule{"actions[].name", a.Name, domain.MaxParamNameLen}, lenRule{"actions[].value", a.Value, domain.AutomationConditionMaxLen})
		for _, n := range a.Names {
			out = append(out, lenRule{"actions[].names[]", n, domain.MaxParamNameLen})
		}
	}
	return out
}

// GET /automation-rules?tenant_id=
func (r *Router) listAutomationRules(c *echo.Context) error {
	rows, total, err := r.Automation.List(c.Request().Context(), ActorFrom(c), queryUint64(c, "tenant_id"), paginationFromQuery(c))
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, listResponse{Data: rows, Total: total})
}

// GET /automation-rules/:uuid
func (r *Router) getAutomationRule(c *echo.Context) error {
	uuid := c.Param("uuid")
	if err := checkMaxLen(lenRule{"uuid", uuid, 36}); err != nil {
		return err
	}
	rule, err := r.Automation.Get(c.Request().Context(), ActorFrom(c), uuid)
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, rule)
}

// POST /automation-rules
func (r *Router) createAutomationRule(c *echo.Context) error {
	var req automationRequest
	if err := decodeStrictJSON(c, &req); err != nil {
		return err
	}
	rules := append([]lenRule{
		{"name", req.Name, domain.AutomationNameMaxLen}, {"description", optStr(req.Description), domain.AutomationDescriptionMaxLen},
		{"condition", req.Condition, domain.AutomationConditionMaxLen},
	}, actionRules(req.Actions)...)
	if err := checkMaxLen(rules...); err != nil {
		return err
	}
	rule, err := r.Automation.Create(c.Request().Context(), ActorFrom(c), automation.Input{
		TenantID: req.TenantID, Name: req.Name, Description: req.Description, TriggerEvent: req.TriggerEvent, Condition: req.Condition,
		Actions: req.Actions, CooldownSeconds: req.CooldownSeconds, MaxConsecutiveFires: req.MaxConsecutiveFires, Priority: req.Priority, IsActive: req.IsActive,
	})
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusCreated, rule)
}

// PATCH /automation-rules/:uuid
func (r *Router) updateAutomationRule(c *echo.Context) error {
	uuid := c.Param("uuid")
	if err := checkMaxLen(lenRule{"uuid", uuid, 36}); err != nil {
		return err
	}
	var req automationPatch
	if err := decodeStrictJSON(c, &req); err != nil {
		return err
	}
	rules := []lenRule{
		{"name", optStr(req.Name), domain.AutomationNameMaxLen}, {"description", optStr(req.Description), domain.AutomationDescriptionMaxLen},
		{"condition", optStr(req.Condition), domain.AutomationConditionMaxLen},
	}
	if req.Actions != nil {
		rules = append(rules, actionRules(*req.Actions)...)
	}
	if err := checkMaxLen(rules...); err != nil {
		return err
	}
	rule, err := r.Automation.Update(c.Request().Context(), ActorFrom(c), uuid, automation.UpdateInput{
		Name: req.Name, Description: req.Description, TriggerEvent: req.TriggerEvent, Condition: req.Condition, Actions: req.Actions,
		CooldownSeconds: req.CooldownSeconds, MaxConsecutiveFires: req.MaxConsecutiveFires, Priority: req.Priority, IsActive: req.IsActive,
	})
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, rule)
}

// DELETE /automation-rules/:uuid
func (r *Router) deleteAutomationRule(c *echo.Context) error {
	uuid := c.Param("uuid")
	if err := checkMaxLen(lenRule{"uuid", uuid, 36}); err != nil {
		return err
	}
	if err := r.Automation.Delete(c.Request().Context(), ActorFrom(c), uuid); err != nil {
		return handleErr(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

// POST /automation-rules/:uuid/reset?device_id= — buka kembali aturan yang di-suspend (satu perangkat, atau semua bila tanpa device_id).
func (r *Router) resetAutomationRule(c *echo.Context) error {
	uuid := c.Param("uuid")
	if err := checkMaxLen(lenRule{"uuid", uuid, 36}); err != nil {
		return err
	}
	n, err := r.Automation.ResetSuspension(c.Request().Context(), ActorFrom(c), uuid, queryUint64(c, "device_id"))
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, map[string]int64{"reset": n})
}

type automationTestRequest struct {
	Condition string                    `json:"condition"`
	Actions   []domain.AutomationAction `json:"actions"`
	Events    []string                  `json:"events"` // opsional: mensimulasikan {@events}
	DeviceID  uint64                    `json:"device_id"`
}

// POST /automation-rules/test — coba kondisi + rencana aksi pada satu perangkat TANPA mengeksekusi apa pun.
func (r *Router) testAutomationRule(c *echo.Context) error {
	var req automationTestRequest
	if err := decodeStrictJSON(c, &req); err != nil {
		return err
	}
	if req.DeviceID == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "device_id wajib diisi")
	}
	if len(req.Events) > 10 {
		return echo.NewHTTPError(http.StatusBadRequest, "events maksimal 10")
	}
	rules := append([]lenRule{{"condition", req.Condition, domain.AutomationConditionMaxLen}}, actionRules(req.Actions)...)
	for _, e := range req.Events {
		rules = append(rules, lenRule{"events[]", e, 32})
	}
	if err := checkMaxLen(rules...); err != nil {
		return err
	}
	res, err := r.Automation.Test(c.Request().Context(), ActorFrom(c), automation.TestInput{Condition: req.Condition, Actions: req.Actions, Events: req.Events}, req.DeviceID)
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, res)
}

// GET /automation-runs?rule_uuid=<uuid>&device_id=
func (r *Router) listAutomationRuns(c *echo.Context) error {
	f := domain.AutomationRunFilter{RuleUUID: c.QueryParam("rule_uuid"), DeviceID: queryUint64(c, "device_id")}
	if err := checkMaxLen(lenRule{"rule_uuid", f.RuleUUID, 36}); err != nil {
		return err
	}
	rows, total, err := r.Automation.ListRuns(c.Request().Context(), ActorFrom(c), f, paginationFromQuery(c))
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, listResponse{Data: rows, Total: total})
}
