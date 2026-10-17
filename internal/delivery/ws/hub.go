package ws

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Event struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}

type Client struct {
	Hub      *Hub
	Conn     *websocket.Conn
	Send     chan []byte
	TenantID *uint64
	Role     string
	// Revalidate (opsional) dipanggil tiap siklus ping. false = kredensial sudah tak berlaku (token
	// kedaluwarsa, user/tenant dinonaktifkan) -> koneksi ditutup. Audit OWASP A07: tanpa ini koneksi
	// WebSocket tetap menerima event tenant setelah sesi HTTP-nya berakhir.
	Revalidate func() bool
}

type Hub struct {
	Clients    map[*Client]bool
	Broadcast  chan []byte
	Register   chan *Client
	Unregister chan *Client
	mu         sync.RWMutex

	// Fan-out antar-instance (opsional, lihat WithBus): event yang dihasilkan instance A harus sampai
	// ke browser yang tersambung ke instance B.
	bus    Bus
	origin string
}

// Bus — transport pesan antar-instance (Redis pub/sub di produksi). Publish gagal tidak boleh
// mengganggu pengiriman lokal.
type Bus interface {
	Publish(ctx context.Context, data []byte) error
	Subscribe(ctx context.Context, handler func(data []byte))
}

// busMessage — amplop di Bus. Origin dipakai agar instance tidak mengirim ulang pesannya sendiri.
type busMessage struct {
	Origin string          `json:"o"`
	Scope  string          `json:"s"` // "all" | "tenant"
	Tenant uint64          `json:"t,omitempty"`
	Data   json.RawMessage `json:"d"`
}

// WithBus mengaktifkan fan-out antar-instance dan mulai mendengarkan pesan instance lain sampai ctx selesai.
func (h *Hub) WithBus(ctx context.Context, bus Bus) *Hub {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	h.bus, h.origin = bus, hex.EncodeToString(b)
	go bus.Subscribe(ctx, h.deliverRemote)
	return h
}

func (h *Hub) publish(scope string, tenant uint64, data []byte) {
	if h.bus == nil {
		return
	}
	msg, err := json.Marshal(busMessage{Origin: h.origin, Scope: scope, Tenant: tenant, Data: data})
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	if err := h.bus.Publish(ctx, msg); err != nil {
		log.Printf("WS bus publish: %v (event hanya sampai ke klien instance ini)", err)
	}
}

func (h *Hub) deliverRemote(raw []byte) {
	var m busMessage
	if json.Unmarshal(raw, &m) != nil || m.Origin == h.origin || len(m.Data) == 0 {
		return
	}
	switch m.Scope {
	case "all":
		h.Broadcast <- []byte(m.Data)
	case "tenant":
		h.deliverToTenant(m.Tenant, []byte(m.Data))
	}
}

// deliverToTenant mengirim ke klien lokal: SUPERADMIN atau TenantID yang cocok.
func (h *Hub) deliverToTenant(tenantID uint64, data []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for client := range h.Clients {
		if client.Role == "SUPERADMIN" || (client.TenantID != nil && *client.TenantID == tenantID) {
			select {
			case client.Send <- data:
			default:
				// Buffer penuh atau client stuck, kita tidak close di sini karena
				// block ini di dalam RLock, menghindari deadlock map iteration
			}
		}
	}
}

var Upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Boleh dilonggarkan karena di belakang middleware auth
	},
}

func NewHub() *Hub {
	return &Hub{
		Broadcast:  make(chan []byte),
		Register:   make(chan *Client),
		Unregister: make(chan *Client),
		Clients:    make(map[*Client]bool),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.Register:
			h.mu.Lock()
			h.Clients[client] = true
			h.mu.Unlock()
		case client := <-h.Unregister:
			h.mu.Lock()
			if _, ok := h.Clients[client]; ok {
				delete(h.Clients, client)
				close(client.Send)
			}
			h.mu.Unlock()
		case message := <-h.Broadcast:
			// Lock penuh (bukan RLock): klien lambat di-delete dari map di sini.
			h.mu.Lock()
			for client := range h.Clients {
				select {
				case client.Send <- message:
				default:
					close(client.Send)
					delete(h.Clients, client)
				}
			}
			h.mu.Unlock()
		}
	}
}

func (h *Hub) BroadcastEvent(eventType string, payload interface{}) {
	event := Event{
		Type:    eventType,
		Payload: payload,
	}
	data, err := json.Marshal(event)
	if err != nil {
		log.Printf("WS Broadcast Error: %v", err)
		return
	}
	h.Broadcast <- data
	h.publish("all", 0, data)
}

// BroadcastToTenant mengirim event hanya ke client yang terhubung dengan TenantID yang sesuai (atau superadmin)
func (h *Hub) BroadcastToTenant(tenantID uint64, eventType string, payload interface{}) {
	event := Event{
		Type:    eventType,
		Payload: payload,
	}
	data, err := json.Marshal(event)
	if err != nil {
		log.Printf("WS Broadcast Error: %v", err)
		return
	}

	h.deliverToTenant(tenantID, data)
	h.publish("tenant", tenantID, data)
}

// pingPeriod — jeda ping sekaligus siklus Revalidate. Variabel agar bisa dipercepat di test.
var pingPeriod = 54 * time.Second

func (c *Client) WritePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()
	for {
		select {
		case message, ok := <-c.Send:
			c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.Conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// Add queued chat messages to the current websocket message.
			n := len(c.Send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.Send)
			}

			if err := w.Close(); err != nil {
				return
			}
		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if c.Revalidate != nil && !c.Revalidate() {
				c.Conn.WriteMessage(websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "sesi berakhir"))
				return
			}
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *Client) ReadPump() {
	defer func() {
		c.Hub.Unregister <- c
		c.Conn.Close()
	}()
	c.Conn.SetReadLimit(512)
	c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.Conn.SetPongHandler(func(string) error { c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second)); return nil })
	for {
		_, _, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("error: %v", err)
			}
			break
		}
	}
}
