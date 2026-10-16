package http

import (
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/labstack/echo/v5"

	"acs/internal/delivery/ws"
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

	client := &ws.Client{
		Hub:      r.WSHub,
		Conn:     conn,
		Send:     make(chan []byte, 256),
		TenantID: actor.TenantID,
		Role:     role,
	}
	client.Hub.Register <- client

	go client.WritePump()
	go client.ReadPump()

	return nil
}
