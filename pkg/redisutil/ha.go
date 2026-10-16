package redisutil

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// Komponen HA multi-instance (lihat docker-compose / RUNBOOK "Beberapa instance acsd"):
//   - TryLock    : kunci leader untuk sweeper berkala (satu instance per tick).
//   - PubSub     : fan-out event WebSocket antar-instance.
//   - RateLimiter: rate limit terdistribusi (satu hitungan untuk semua instance).
// Semuanya fail-open terhadap gangguan Redis: ACS tetap berjalan (perilaku per-instance) alih-alih berhenti.

var releaseScript = redis.NewScript(`
if redis.call("get", KEYS[1]) == ARGV[1] then return redis.call("del", KEYS[1]) else return 0 end`)

// TryLock mencoba mengambil kunci `key` selama ttl. ok=false bila instance lain memegangnya. release
// aman dipanggil (hanya melepas kunci miliknya sendiri). err != nil = Redis bermasalah (pemanggil
// memutuskan; sweeper memilih tetap jalan).
func (c *Client) TryLock(ctx context.Context, key string, ttl time.Duration) (release func(), ok bool, err error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return nil, false, err
	}
	token := hex.EncodeToString(b)
	full := "acs:lock:" + key
	got, err := c.client.SetNX(ctx, full, token, ttl).Result()
	if err != nil {
		return nil, false, err
	}
	if !got {
		return nil, false, nil
	}
	return func() {
		rctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = releaseScript.Run(rctx, c.client, []string{full}, token).Err()
	}, true, nil
}

// PubSub — satu channel Redis untuk publikasi/langganan pesan (dipakai hub WebSocket).
type PubSub struct {
	c       *Client
	channel string
}

func (c *Client) PubSub(channel string) *PubSub { return &PubSub{c: c, channel: channel} }

func (p *PubSub) Publish(ctx context.Context, data []byte) error {
	return p.c.client.Publish(ctx, p.channel, data).Err()
}

// Subscribe memblok sampai ctx selesai; go-redis menyambung ulang otomatis saat koneksi putus.
func (p *PubSub) Subscribe(ctx context.Context, handler func(data []byte)) {
	sub := p.c.client.Subscribe(ctx, p.channel)
	defer sub.Close()
	ch := sub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			handler([]byte(msg.Payload))
		}
	}
}

// Limiter — antarmuka sempit yang sama dengan echo middleware.RateLimiterStore.
type Limiter interface {
	Allow(identifier string) (bool, error)
}

// RateLimiter — rate limit jendela tetap, dibagi lintas instance lewat Redis. Laju >= 1/dtk memakai
// jendela 1 dtk dengan batas ceil(laju); laju < 1/dtk (mis. 0,2) memakai jendela ceil(1/laju) dtk
// dengan batas 1. Bila Redis gagal, jatuh ke fallback (limiter in-memory per instance).
type RateLimiter struct {
	c        *Client
	prefix   string
	limit    int64
	window   time.Duration
	fallback Limiter
}

func (c *Client) NewRateLimiter(prefix string, ratePerSec float64, fallback Limiter) *RateLimiter {
	limit, window := int64(1), time.Second
	if ratePerSec >= 1 {
		limit = int64(math.Ceil(ratePerSec))
	} else if ratePerSec > 0 {
		window = time.Duration(math.Ceil(1/ratePerSec)) * time.Second
	}
	return &RateLimiter{c: c, prefix: prefix, limit: limit, window: window, fallback: fallback}
}

func (r *RateLimiter) Allow(identifier string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	slot := time.Now().UnixNano() / int64(r.window)
	key := fmt.Sprintf("acs:rl:%s:%s:%s", r.prefix, identifier, strconv.FormatInt(slot, 10))
	pipe := r.c.client.Pipeline()
	incr := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, r.window+time.Second)
	if _, err := pipe.Exec(ctx); err != nil {
		if r.fallback != nil {
			return r.fallback.Allow(identifier)
		}
		return true, nil // fail-open: gangguan Redis tidak boleh menutup akses
	}
	return incr.Val() <= r.limit, nil
}
