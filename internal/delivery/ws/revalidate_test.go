package ws

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// serveTestClient menjalankan satu Client di server uji dengan Revalidate yang diberikan.
func serveTestClient(t *testing.T, reval func() bool) *websocket.Conn {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := Upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		c := &Client{Conn: conn, Send: make(chan []byte, 4), Revalidate: reval, Hub: &Hub{Unregister: make(chan *Client, 1)}}
		go c.WritePump()
		go c.ReadPump()
	}))
	t.Cleanup(srv.Close)
	cl, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cl.Close() })
	return cl
}

func withFastPing(t *testing.T) {
	old := pingPeriod
	pingPeriod = 30 * time.Millisecond
	t.Cleanup(func() { pingPeriod = old })
}

// Kredensial tak berlaku lagi -> server menutup koneksi dengan close frame "policy violation".
func TestRevalidate_KredensialDicabutMenutupKoneksi(t *testing.T) {
	withFastPing(t)
	var calls atomic.Int32
	cl := serveTestClient(t, func() bool { return calls.Add(1) < 2 }) // siklus 1 lolos, siklus 2 dicabut
	cl.SetReadDeadline(time.Now().Add(3 * time.Second))
	for {
		_, _, err := cl.ReadMessage()
		if err == nil {
			continue
		}
		ce, ok := err.(*websocket.CloseError)
		if !ok || ce.Code != websocket.ClosePolicyViolation {
			t.Fatalf("harus ditutup dengan policy violation, dapat %v", err)
		}
		break
	}
	if calls.Load() < 2 {
		t.Fatalf("Revalidate harus dipanggil ulang tiap siklus: %d", calls.Load())
	}
}

// Kredensial masih valid -> koneksi tetap hidup lintas beberapa siklus ping.
func TestRevalidate_MasihValidKoneksiTetapHidup(t *testing.T) {
	withFastPing(t)
	var calls atomic.Int32
	cl := serveTestClient(t, func() bool { calls.Add(1); return true })
	cl.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	_, _, err := cl.ReadMessage() // hanya ping masuk (ditangani otomatis); harus habis waktu, bukan ditutup
	if ce, ok := err.(*websocket.CloseError); ok {
		t.Fatalf("koneksi tidak boleh ditutup: %v", ce)
	}
	if calls.Load() < 3 {
		t.Fatalf("seharusnya sudah beberapa siklus: %d", calls.Load())
	}
}
