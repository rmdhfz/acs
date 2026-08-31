package http

import (
	"net/http"

	"github.com/labstack/echo/v5"

	"acs/internal/usecase/webhook"
)

// ---- Webhook subscriptions (migrations/0013) ----
// Handler tipis: parsing + validasi bentuk saja; RBAC/tenant-scope/validasi
// bisnis ada di usecase/webhook (CLAUDE.md).

type createWebhookRequest struct {
	TenantID    *uint64 `json:"tenant_id"`
	EventType   string  `json:"event_type"`
	Name        string  `json:"name"`
	TargetURL   string  `json:"target_url"`
	Description *string `json:"description"`
}

func (r *Router) createWebhook(c *echo.Context) error {
	actor := ActorFrom(c)
	var req createWebhookRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "payload tidak valid")
	}
	if req.EventType == "" || req.Name == "" || req.TargetURL == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "event_type, name, target_url wajib diisi")
	}
	sub, err := r.Webhooks.CreateSubscription(c.Request().Context(), actor, webhook.CreateSubscriptionInput{
		TenantID:    req.TenantID,
		EventCode:   req.EventType,
		Name:        req.Name,
		TargetURL:   req.TargetURL,
		Description: req.Description,
	})
	if err != nil {
		return handleErr(c, err)
	}
	// Response memuat `secret` plaintext — SATU-SATUNYA kesempatan client
	// menyimpannya (kolom DB terenkripsi, tidak pernah dikembalikan lagi).
	return c.JSON(http.StatusCreated, sub)
}

func (r *Router) listWebhooks(c *echo.Context) error {
	actor := ActorFrom(c)
	subs, total, err := r.Webhooks.ListSubscriptions(c.Request().Context(), actor, queryUint64(c, "tenant_id"), paginationFromQuery(c))
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, listResponse{Data: subs, Total: total})
}

func (r *Router) getWebhook(c *echo.Context) error {
	actor := ActorFrom(c)
	id, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	sub, err := r.Webhooks.GetSubscription(c.Request().Context(), actor, id)
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, sub)
}

type updateWebhookRequest struct {
	Name      string `json:"name"`
	TargetURL string `json:"target_url"`
	// is_active: true/false wajib diisi eksplisit. Bila tidak diisi (null),
	// status aktif subscription tidak berubah — konsisten dgn pola PATCH partial
	// update di endpoint lain. *bool supaya false bisa dibedakan dari "tidak ada".
	IsActive    *bool   `json:"is_active"`
	Description *string `json:"description"`
}

func (r *Router) updateWebhook(c *echo.Context) error {
	actor := ActorFrom(c)
	id, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	var req updateWebhookRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "payload tidak valid")
	}
	if err := r.Webhooks.UpdateSubscription(c.Request().Context(), actor, id, webhook.UpdateSubscriptionInput{
		Name: req.Name, TargetURL: req.TargetURL, IsActive: req.IsActive, Description: req.Description,
	}); err != nil {
		return handleErr(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (r *Router) deleteWebhook(c *echo.Context) error {
	actor := ActorFrom(c)
	id, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	if err := r.Webhooks.DeleteSubscription(c.Request().Context(), actor, id); err != nil {
		return handleErr(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (r *Router) listWebhookDeliveries(c *echo.Context) error {
	actor := ActorFrom(c)
	id, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	rows, total, err := r.Webhooks.ListDeliveries(c.Request().Context(), actor, id, paginationFromQuery(c))
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, listResponse{Data: rows, Total: total})
}

func (r *Router) testWebhook(c *echo.Context) error {
	actor := ActorFrom(c)
	id, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	d, err := r.Webhooks.TestSubscription(c.Request().Context(), actor, id)
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusAccepted, d)
}

func (r *Router) countFailedDeliveries(c *echo.Context) error {
	actor := ActorFrom(c)
	count, err := r.Webhooks.CountFailedDeliveries(c.Request().Context(), actor)
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, map[string]int{"count": count})
}
