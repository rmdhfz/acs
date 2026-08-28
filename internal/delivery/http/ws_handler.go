package http

import (
	"log"

	"github.com/labstack/echo/v5"

	"acs/internal/delivery/ws"
	"acs/internal/domain"
)

func (r *Router) serveWs(c echo.Context) error {
	conn, err := ws.Upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		log.Printf("WS Upgrade Error: %v", err)
		return err
	}

	actor, ok := c.Get("actor").(*domain.Actor)
	if !ok {
		// Harusnya tidak mungkin karena lewat AuthMiddleware
		conn.Close()
		return nil
	}

	client := &ws.Client{
		Hub:      r.WSHub,
		Conn:     conn,
		Send:     make(chan []byte, 256),
		TenantID: actor.TenantID,
		Role:     actor.Roles[0], // Simplified, assumes at least one role
	}

	client.Hub.Register <- client

	// Allow collection of memory referenced by the caller by doing all work in
	// new goroutines.
	go client.WritePump()
	go client.ReadPump()

	return nil
}
