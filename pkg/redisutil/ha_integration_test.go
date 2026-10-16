package redisutil

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// Integrasi Redis NYATA. Dilewati tanpa ACS_TEST_REDIS (mis. ACS_TEST_REDIS=127.0.0.1:16379).
func testClient(t *testing.T) *Client {
	t.Helper()
	addr := os.Getenv("ACS_TEST_REDIS")
	if addr == "" {
		t.Skip("ACS_TEST_REDIS tidak diisi — test integrasi Redis dilewati")
	}
	c, err := NewClient(addr, os.Getenv("ACS_TEST_REDIS_PASSWORD"), slog.Default())
	if err != nil {
		t.Fatalf("redis: %v", err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func TestTryLock_HanyaSatuPemenang(t *testing.T) {
	c := testClient(t)
	ctx := context.Background()
	key := "uji-" + time.Now().Format("150405.000000")

	var wins int32
	var wg sync.WaitGroup
	var releases []func()
	var mu sync.Mutex
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rel, ok, err := c.TryLock(ctx, key, 5*time.Second)
			if err != nil {
				t.Errorf("TryLock: %v", err)
				return
			}
			if ok {
				atomic.AddInt32(&wins, 1)
				mu.Lock()
				releases = append(releases, rel)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if wins != 1 {
		t.Fatalf("pemenang kunci = %d, harus tepat 1", wins)
	}
	// release milik pemenang -> kunci bisa diambil lagi
	releases[0]()
	rel, ok, _ := c.TryLock(ctx, key, time.Second)
	if !ok {
		t.Fatal("kunci harus bisa diambil setelah dilepas")
	}
	// release basi (token lama) tidak boleh melepas kunci pemegang baru
	releases[0]()
	if _, ok2, _ := c.TryLock(ctx, key, time.Second); ok2 {
		t.Fatal("release basi tidak boleh melepas kunci milik pemegang baru")
	}
	rel()
}

func TestTryLock_KedaluwarsaOtomatis(t *testing.T) {
	c := testClient(t)
	key := "ttl-" + time.Now().Format("150405.000000")
	if _, ok, _ := c.TryLock(context.Background(), key, 300*time.Millisecond); !ok {
		t.Fatal("kunci pertama harus didapat")
	}
	time.Sleep(500 * time.Millisecond)
	if _, ok, _ := c.TryLock(context.Background(), key, time.Second); !ok {
		t.Fatal("kunci pemegang yang mati harus kedaluwarsa sendiri (instance crash tidak menahan sweeper selamanya)")
	}
}

func TestPubSub_AntarKlien(t *testing.T) {
	a, b := testClient(t), testClient(t)
	ch := "uji-ps-" + time.Now().Format("150405.000000")
	got := make(chan string, 4)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go b.PubSub(ch).Subscribe(ctx, func(d []byte) { got <- string(d) })
	time.Sleep(300 * time.Millisecond) // beri waktu langganan terpasang
	if err := a.PubSub(ch).Publish(ctx, []byte("halo")); err != nil {
		t.Fatal(err)
	}
	select {
	case m := <-got:
		if m != "halo" {
			t.Fatalf("pesan = %q", m)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("pesan tidak sampai ke pelanggan di klien lain")
	}
}

type countingFallback struct{ n int32 }

func (f *countingFallback) Allow(string) (bool, error) { atomic.AddInt32(&f.n, 1); return true, nil }

func TestRateLimiter_TerdistribusiDanFallback(t *testing.T) {
	a, b := testClient(t), testClient(t) // dua "instance"
	id := "ip-" + time.Now().Format("150405.000000")
	// 3 req/dtk dibagi dua instance: total yang lolos <= 3 per jendela (boleh 6 bila melintasi batas detik)
	la := a.NewRateLimiter("uji", 3, nil)
	lb := b.NewRateLimiter("uji", 3, nil)
	// tunggu awal detik agar jendela tidak terbelah di tengah tes
	time.Sleep(time.Until(time.Now().Truncate(time.Second).Add(1010 * time.Millisecond)))
	allowed := 0
	for i := 0; i < 10; i++ {
		l := la
		if i%2 == 1 {
			l = lb
		}
		if ok, _ := l.Allow(id); ok {
			allowed++
		}
	}
	if allowed != 3 {
		t.Fatalf("lolos = %d dari 10, harus tepat 3 (batas dibagi lintas instance)", allowed)
	}
	// laju < 1/dtk: 1 per jendela
	slow := a.NewRateLimiter("uji-slow", 0.5, nil)
	sid := "x-" + time.Now().Format("150405.000000")
	ok1, _ := slow.Allow(sid)
	ok2, _ := slow.Allow(sid)
	if !ok1 || ok2 {
		t.Fatalf("laju 0,5/dtk: pertama lolos (%v), kedua ditolak (%v)", ok1, !ok2)
	}
	// Redis mati -> fallback dipakai; tanpa fallback -> fail-open (gangguan Redis tidak boleh menutup akses)
	dead := &Client{client: redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", DialTimeout: 100 * time.Millisecond, MaxRetries: -1}), logger: slog.Default()}
	defer dead.Close()
	fb := &countingFallback{}
	if ok, _ := dead.NewRateLimiter("x", 5, fb).Allow("a"); !ok || atomic.LoadInt32(&fb.n) != 1 {
		t.Fatalf("Redis mati harus memakai fallback (n=%d)", fb.n)
	}
	if ok, _ := dead.NewRateLimiter("x", 5, nil).Allow("a"); !ok {
		t.Fatal("tanpa fallback harus fail-open")
	}
}
