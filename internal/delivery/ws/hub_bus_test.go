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

func addUserClient(h *Hub, userID uint64) *Client {
	c := &Client{Hub: h, Send: make(chan []byte, 8), UserID: userID, Role: "NOC"}
	h.mu.Lock()
	h.Clients[c] = true
	h.mu.Unlock()
	return c
}

// BroadcastToUser hanya sampai ke koneksi milik pengguna itu (semua tabnya), termasuk lintas instance via bus;
// pengguna lain — juga SUPERADMIN dan satu tenant — TIDAK menerima.
func TestHubBus_BroadcastToUser(t *testing.T) {
	bus := &memBus{}
	h1, h2 := newTestHub(t, bus), newTestHub(t, bus)
	time.Sleep(50 * time.Millisecond)

	tab1, tab2 := addUserClient(h1, 7), addUserClient(h1, 7)
	remote := addUserClient(h2, 7)
	other := addUserClient(h2, 8)
	super := addClient(h2, nil, "SUPERADMIN")
	sameTenant := addClient(h1, u64(1), "NOC")

	h1.BroadcastToUser(7, "NOTIFICATION", map[string]any{"id": "x"})

	for name, c := range map[string]*Client{"tab1": tab1, "tab2": tab2, "instance lain": remote} {
		if m, ok := recv(c, time.Second); !ok || m == "" {
			t.Fatalf("%s harus menerima notifikasi pengguna", name)
		}
	}
	if _, ok := recv(tab1, 200*time.Millisecond); ok {
		t.Fatal("tidak boleh duplikat (gema bus) di instance pengirim")
	}
	for name, c := range map[string]*Client{"pengguna lain": other, "superadmin": super, "tenant sama": sameTenant} {
		if _, ok := recv(c, 300*time.Millisecond); ok {
			t.Fatalf("%s TIDAK boleh menerima notifikasi pribadi", name)
		}
	}
}

func TestHub_BroadcastToUser_TanpaBusDanUserNol(t *testing.T) {
	h := newTestHub(t, nil)
	c := addUserClient(h, 3)
	anon := addUserClient(h, 0)
	h.BroadcastToUser(3, "NOTIFICATION", nil)
	if _, ok := recv(c, time.Second); !ok {
		t.Fatal("tanpa bus harus tetap mengirim lokal")
	}
	h.BroadcastToUser(0, "NOTIFICATION", nil)
	if _, ok := recv(anon, 200*time.Millisecond); ok {
		t.Fatal("userID 0 tidak boleh menjangkau koneksi tanpa identitas")
	}
}
