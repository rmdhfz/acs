package ws

import (
	"context"
	"sync"
	"testing"
	"time"
)

// memBus — bus in-memory yang menyiarkan ke semua pelanggan (termasuk publisher, seperti Redis pub/sub).
type memBus struct {
	mu   sync.Mutex
	subs []func([]byte)
}

func (b *memBus) Publish(_ context.Context, data []byte) error {
	b.mu.Lock()
	subs := append([]func([]byte){}, b.subs...)
	b.mu.Unlock()
	for _, s := range subs {
		s(append([]byte(nil), data...))
	}
	return nil
}

func (b *memBus) Subscribe(ctx context.Context, h func([]byte)) {
	b.mu.Lock()
	b.subs = append(b.subs, h)
	b.mu.Unlock()
	<-ctx.Done()
}

func newTestHub(t *testing.T, bus Bus) *Hub {
	t.Helper()
	h := NewHub()
	go h.Run()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	if bus != nil {
		h.WithBus(ctx, bus)
	}
	return h
}

func addClient(h *Hub, tenant *uint64, role string) *Client {
	c := &Client{Hub: h, Send: make(chan []byte, 8), TenantID: tenant, Role: role}
	h.mu.Lock()
	h.Clients[c] = true
	h.mu.Unlock()
	return c
}

func recv(c *Client, wait time.Duration) (string, bool) {
	select {
	case m := <-c.Send:
		return string(m), true
	case <-time.After(wait):
		return "", false
	}
}

func u64(v uint64) *uint64 { return &v }

func TestHubBus_FanOutAntarInstance(t *testing.T) {
	bus := &memBus{}
	h1, h2 := newTestHub(t, bus), newTestHub(t, bus)
	time.Sleep(50 * time.Millisecond) // langganan terpasang

	local := addClient(h1, u64(1), "NOC")       // tenant 1 di instance pengirim
	remote := addClient(h2, u64(1), "NOC")      // tenant 1 di instance LAIN
	otherTenant := addClient(h2, u64(2), "NOC") // tenant lain di instance lain
	super := addClient(h2, nil, "SUPERADMIN")   // superadmin di instance lain

	h1.BroadcastToTenant(1, "TASK_CREATED", map[string]any{"id": 7})

	if _, ok := recv(local, time.Second); !ok {
		t.Fatal("klien lokal harus menerima")
	}
	if _, ok := recv(local, 200*time.Millisecond); ok {
		t.Fatal("klien lokal TIDAK boleh menerima duplikat (gema dari bus)")
	}
	if m, ok := recv(remote, time.Second); !ok || m == "" {
		t.Fatal("klien tenant sama di instance lain harus menerima lewat bus")
	}
	if _, ok := recv(super, time.Second); !ok {
		t.Fatal("SUPERADMIN di instance lain harus menerima")
	}
	if _, ok := recv(otherTenant, 300*time.Millisecond); ok {
		t.Fatal("tenant lain di instance lain TIDAK boleh menerima (isolasi tenant lintas instance)")
	}
}

func TestHubBus_BroadcastSemua(t *testing.T) {
	bus := &memBus{}
	h1, h2 := newTestHub(t, bus), newTestHub(t, bus)
	time.Sleep(50 * time.Millisecond)
	a, b := addClient(h1, u64(1), "NOC"), addClient(h2, u64(2), "NOC")
	h1.BroadcastEvent("SYSTEM", map[string]any{"x": 1})
	if _, ok := recv(a, time.Second); !ok {
		t.Fatal("lokal")
	}
	if _, ok := recv(b, time.Second); !ok {
		t.Fatal("instance lain")
	}
	if _, ok := recv(a, 200*time.Millisecond); ok {
		t.Fatal("duplikat di instance pengirim")
	}
}

// Tanpa bus (satu instance) perilaku lama tidak berubah.
func TestHub_TanpaBus(t *testing.T) {
	h := newTestHub(t, nil)
	c := addClient(h, u64(1), "NOC")
	h.BroadcastToTenant(1, "X", nil)
	if _, ok := recv(c, time.Second); !ok {
		t.Fatal("tanpa bus harus tetap mengirim lokal")
	}
}

// Pesan bus rusak/tak dikenal diabaikan, tidak panik.
func TestHubBus_PesanRusakDiabaikan(t *testing.T) {
	h := newTestHub(t, &memBus{})
	c := addClient(h, u64(1), "NOC")
	h.deliverRemote([]byte("bukan json"))
	h.deliverRemote([]byte(`{"o":"lain","s":"aneh","d":{"a":1}}`))
	h.deliverRemote([]byte(`{"o":"lain","s":"tenant","t":1}`)) // tanpa data
	if _, ok := recv(c, 200*time.Millisecond); ok {
		t.Fatal("pesan rusak tidak boleh dikirim ke klien")
	}
}
