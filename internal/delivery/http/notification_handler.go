package http

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	"acs/internal/usecase/notification"
)

// ---- Notifikasi persisten (migrations/0030) ----
// Handler tipis: parsing + validasi bentuk; semua keputusan (kepemilikan, preferensi, RBAC pengumuman) di
// usecase/notification. Notifikasi selalu milik pengguna yang sedang login; "id" di API = notification_uuid.

// listNotifications — GET /notifications?unread=true&page=&page_size=
func (r *Router) listNotifications(c *echo.Context) error {
	p := paginationFromQuery(c)
	unread := strings.EqualFold(c.QueryParam("unread"), "true") || c.QueryParam("unread") == "1"
	items, total, unreadCount, err := r.Notifications.List(c.Request().Context(), ActorFrom(c), unread, p)
	if err != nil {
		return handleErr(c, err)
	}
	page := p.Page
	if page < 1 {
		page = 1
	}
	return c.JSON(http.StatusOK, map[string]any{
		"data": items,
		"meta": map[string]int{"total": total, "unread": unreadCount, "page": page, "limit": p.Limit()},
	})
}

// notificationUnreadCount — GET /notifications/unread-count
func (r *Router) notificationUnreadCount(c *echo.Context) error {
	n, err := r.Notifications.UnreadCount(c.Request().Context(), ActorFrom(c))
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, map[string]int{"count": n})
}

type markNotificationsReadRequest struct {
	IDs []string `json:"ids"`
	All bool     `json:"all"`
}

// markNotificationsRead — POST /notifications/read
func (r *Router) markNotificationsRead(c *echo.Context) error {
	var req markNotificationsReadRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "payload tidak valid")
	}
	if err := r.Notifications.MarkRead(c.Request().Context(), ActorFrom(c), req.IDs, req.All); err != nil {
		return handleErr(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

// listNotificationPreferences — GET /notifications/preferences
func (r *Router) listNotificationPreferences(c *echo.Context) error {
	prefs, err := r.Notifications.ListPreferences(c.Request().Context(), ActorFrom(c))
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{"data": prefs})
}

type notificationPreferenceItem struct {
	Type  string `json:"type"`
	InApp bool   `json:"in_app"`
	Email bool   `json:"email"`
}

type updateNotificationPreferencesRequest struct {
	Items []notificationPreferenceItem `json:"items"`
}

// updateNotificationPreferences — PUT /notifications/preferences
func (r *Router) updateNotificationPreferences(c *echo.Context) error {
	var req updateNotificationPreferencesRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "payload tidak valid")
	}
	in := make([]notification.PreferenceInput, 0, len(req.Items))
	for _, it := range req.Items {
		if err := checkMaxLen(lenRule{"items[].type", it.Type, maxNotificationTypeCode}); err != nil {
			return err
		}
		in = append(in, notification.PreferenceInput{Type: it.Type, InApp: it.InApp, Email: it.Email})
	}
	if err := r.Notifications.UpdatePreferences(c.Request().Context(), ActorFrom(c), in); err != nil {
		return handleErr(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

type announceRequest struct {
	Title    string  `json:"title"`
	Body     string  `json:"body"`
	Link     *string `json:"link"`
	TenantID *uint64 `json:"tenant_id"`
	Email    bool    `json:"email"`
}

// announceNotification — POST /notifications/announce (SUPERADMIN; dicek di usecase juga).
func (r *Router) announceNotification(c *echo.Context) error {
	var req announceRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "payload tidak valid")
	}
	if err := checkMaxLen(
		lenRule{"title", req.Title, notification.MaxTitleLen},
		lenRule{"body", req.Body, notification.MaxBodyLen},
		lenRule{"link", optStr(req.Link), notification.MaxLinkLen},
	); err != nil {
		return err
	}
	n, err := r.Notifications.Announce(c.Request().Context(), ActorFrom(c), notification.AnnounceInput{
		Title: req.Title, Body: req.Body, Link: optStr(req.Link), TenantID: req.TenantID, Email: req.Email,
	})
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusAccepted, map[string]int{"recipients": n})
}
