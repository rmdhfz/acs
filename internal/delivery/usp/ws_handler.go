package usp

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"acs/internal/usecase/usp_session"
	"acs/pkg/usp"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v5"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Di production harus divalidasi
	},
	// MTP USP menggunakan subprotokol khusus v1.usp
	Subprotocols: []string{"v1.usp"},
}

type WsConnection struct {
	conn *websocket.Conn
}

func (c *WsConnection) Send(msg *usp.Msg) error {
	rec := usp.Record{
		Version: "1.2", // TR-369 v1.2
		NoMAC:   &usp.NoMAC{},
	}
	payload, _ := json.Marshal(msg)
	rec.NoMAC.Payload = payload

	return c.conn.WriteJSON(rec)
}

func (c *WsConnection) Close() error {
	return c.conn.Close()
}

type Handler struct {
	logger *slog.Logger
	svc    *usp_session.Service
}

func NewHandler(logger *slog.Logger, svc *usp_session.Service) *Handler {
	return &Handler{
		logger: logger,
		svc:    svc,
	}
}

func (h *Handler) Mount(e *echo.Echo) {
	e.GET("/usp", h.handleWebSocket)
}

func (h *Handler) handleWebSocket(c *echo.Context) error {
	conn, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		h.logger.Error("Gagal upgrade ke websocket", "error", err)
		return err
	}

	// USP Endpoint ID biasanya dikirim via header atau param
	// Fallback ke RemoteAddr untuk mockup
	endpointID := c.Request().Header.Get("USP-Endpoint-ID")
	if endpointID == "" {
		endpointID = c.Request().RemoteAddr
	}
	endpointID = strings.Split(endpointID, ":")[0]

	wsConn := &WsConnection{conn: conn}
	h.svc.HandleConnect(c.Request().Context(), endpointID, wsConn)
	defer h.svc.HandleDisconnect(endpointID)
	defer conn.Close()

	for {
		var rec usp.Record
		err := conn.ReadJSON(&rec)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				h.logger.Error("USP WS read error", "error", err)
			}
			break
		}

		if rec.NoMAC != nil && rec.NoMAC.Payload != nil {
			var msg usp.Msg
			if err := json.Unmarshal(rec.NoMAC.Payload, &msg); err == nil {
				_ = h.svc.HandleMessage(context.Background(), endpointID, &msg)
			}
		}
	}

	return nil
}
