package http

import (
	"log"

	"github.com/labstack/echo/v5"

	"acs/internal/delivery/ws"
)

// serveWs meng-upgrade koneksi ke WebSocket untuk push event realtime ke
// dashboard (device online/offline, task status). Sudah lewat AuthMiddleware
// (lihat router.go) sehingga actor pasti ada.
func (r *Router) serveWs(c *echo.Context) error {
	conn, err := ws.Upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		log.Printf("WS Upgrade Error: %v", err)
		return err
	}

	actor := ActorFrom(c)
	role := ""
	if len(actor.Roles) > 0 {
		role = actor.Roles[0]
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
