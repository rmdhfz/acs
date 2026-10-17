package http

import (
	"context"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/labstack/echo/v5"

	"acs/internal/delivery/ws"
	"acs/internal/usecase/auth"
)

// wsOriginAllowed membatasi Origin peramban: harus salah satu origin Console yang
// diizinkan (ACS_CORS_ALLOW_ORIGINS) atau sama dengan Host permintaan. Origin
// kosong (klien non-peramban) diizinkan — autentikasi tetap lewat token.
func wsOriginAllowed(allowed []string) func(*http.Request) bool {
	return func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}
		for _, a := range allowed {
			if strings.EqualFold(strings.TrimRight(a, "/"), origin) {
				return true
			}
		}
		u, err := url.Parse(origin)
		return err == nil && strings.EqualFold(u.Host, r.Host)
	}
}

// serveWs meng-upgrade koneksi ke WebSocket untuk push event realtime ke
// dashboard (device online/offline, task status). Sudah lewat AuthMiddleware
// (lihat router.go) sehingga actor pasti ada.
func (r *Router) serveWs(c *echo.Context) error {
	up := ws.Upgrader
	up.Subprotocols = []string{wsBearerProtocol}
	up.CheckOrigin = wsOriginAllowed(r.WSAllowedOrigins)
	conn, err := up.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		log.Printf("WS Upgrade Error: %v", err)
		return err
	}

	actor := ActorFrom(c)
	// Hub memakai Role hanya untuk membedakan SUPERADMIN (lintas tenant) dari
	// yang lain; jangan bergantung pada urutan Roles.
	role := ""
	if len(actor.Roles) > 0 {
		role = actor.Roles[0]
	}
	for _, rl := range actor.Roles {
		if rl == "SUPERADMIN" {
			role = rl
		}
	}

	token := bearerFromRequest(c.Request())
	client := &ws.Client{
		Hub:      r.WSHub,
		Conn:     conn,
		Send:     make(chan []byte, 256),
		TenantID: actor.TenantID,
		Role:     role,
		// Cek ulang kredensial tiap siklus ping: token kedaluwarsa / user atau tenant dinonaktifkan -> tutup.
		// Galat SISTEM (DB sesaat) tidak memutus koneksi (fail-open); hanya penolakan kredensial yang memutus.
		Revalidate: func() bool {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			a, err := r.Auth.ResolveActor(ctx, token)
			switch {
			case err == nil:
				return a != nil
			case errors.Is(err, auth.ErrInvalidToken), errors.Is(err, auth.ErrTenantInactive):
				return false
			default:
				return true
			}
		},
	}
	client.Hub.Register <- client

	go client.WritePump()
	go client.ReadPump()

	return nil
}
