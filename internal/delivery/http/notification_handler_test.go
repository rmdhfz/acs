package http

import (
	"errors"
	nethttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"acs/internal/domain"
	"acs/internal/usecase/notification"
)

// Validasi bentuk di handler + penolakan RBAC di usecase, tanpa menyentuh repository (semua kasus di bawah gagal
// sebelum query pertama, sehingga repo nil aman).
func callNotif(t *testing.T, h func(*echo.Context) error, actor domain.Actor, body string) int {
	t.Helper()
	req := httptest.NewRequest(nethttp.MethodPost, "/api/v1/notifications/x", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set(actorContextKey, actor)
	err := h(c)
	if err == nil {
		return rec.Code
	}
	var he *echo.HTTPError
	if errors.As(err, &he) {
		return he.Code
	}
	t.Fatalf("error non-HTTP: %v", err)
	return 0
}

func TestNotificationHandler_Validasi(t *testing.T) {
	r := &Router{Notifications: notification.NewService(nil, nil, nil, notification.Config{}, nil)}
	super := domain.Actor{UserID: 1, Roles: []string{domain.RoleSuperadmin}}
	admin := domain.Actor{UserID: 2, Roles: []string{domain.RoleAdmin}}

	cases := []struct {
		name  string
		h     func(*echo.Context) error
		actor domain.Actor
		body  string
		want  int
	}{
		{"announce judul 161 karakter", r.announceNotification, super, `{"title":"` + strings.Repeat("a", 161) + `","body":"x"}`, 400},
		{"announce isi 1001 karakter", r.announceNotification, super, `{"title":"j","body":"` + strings.Repeat("a", 1001) + `"}`, 400},
		{"announce link berbahaya //", r.announceNotification, super, `{"title":"j","body":"x","link":"//evil.test"}`, 400},
		{"announce link berskema", r.announceNotification, super, `{"title":"j","body":"x","link":"https://evil.test"}`, 400},
		{"announce judul kosong", r.announceNotification, super, `{"title":"","body":"x"}`, 400},
		{"announce JSON rusak", r.announceNotification, super, `{`, 400},
		{"announce oleh ADMIN ditolak", r.announceNotification, admin, `{"title":"j","body":"x"}`, 403},
		{"mark-read id bukan uuid", r.markNotificationsRead, admin, `{"ids":["1"]}`, 400},
		{"mark-read kosong", r.markNotificationsRead, admin, `{}`, 400},
		{"mark-read tanpa identitas", r.markNotificationsRead, domain.Actor{}, `{"all":true}`, 403},
		{"preferensi kosong", r.updateNotificationPreferences, admin, `{"items":[]}`, 400},
		{"preferensi kode terlalu panjang", r.updateNotificationPreferences, admin, `{"items":[{"type":"` + strings.Repeat("A", 49) + `","in_app":true}]}`, 400},
	}
	for _, c := range cases {
		if got := callNotif(t, c.h, c.actor, c.body); got != c.want {
			t.Errorf("%s: status %d, mau %d", c.name, got, c.want)
		}
	}
}
