// Package hotcache membungkus repository yang dibaca pada SETIAP Inform CWMP (aturan ZTP, preset enforce) dengan cache
// TTL pendek di memori proses. Datanya kecil, jarang berubah, dan identik antar-device satu tenant — tanpa cache tiap
// Inform memicu dua SELECT yang hasilnya hampir selalu sama (hot path, TECH.md §9).
//
// Konsistensi: mutasi lewat instance yang sama langsung membuang cache; instance acsd lain baru melihat perubahan
// setelah TTL habis (bila ttl <= 0, cache dimatikan dan semua panggilan diteruskan apa adanya). Aturan/preset yang
// baru dibuat tetap berlaku mulai Inform berikutnya — hanya tertunda paling lama sebesar TTL.
package hotcache

import (
	"context"
	"sync"
	"time"

	"acs/internal/domain"
)

// ttlMap: cache sederhana per kunci tenant (0 = tanpa tenant). Hanya hasil sukses yang disimpan.
type ttlMap[V any] struct {
	ttl time.Duration
	mu  sync.Mutex
	m   map[uint64]entry[V]
}

type entry[V any] struct {
	v   V
	exp time.Time
}

func newTTLMap[V any](ttl time.Duration) *ttlMap[V] {
	return &ttlMap[V]{ttl: ttl, m: make(map[uint64]entry[V])}
}

func tenantKey(tenantID *uint64) uint64 {
	if tenantID == nil {
		return 0
	}
	return *tenantID
}

func (c *ttlMap[V]) get(key uint64) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[key]
	if !ok || time.Now().After(e.exp) {
		var zero V
		return zero, false
	}
	return e.v, true
}

func (c *ttlMap[V]) put(key uint64, v V) {
	c.mu.Lock()
	c.m[key] = entry[V]{v: v, exp: time.Now().Add(c.ttl)}
	c.mu.Unlock()
}

func (c *ttlMap[V]) clear() {
	c.mu.Lock()
	clear(c.m)
	c.mu.Unlock()
}

// ---- Aturan ZTP ----

type zeroTouchRules struct {
	domain.ZeroTouchRuleRepository
	cache *ttlMap[[]domain.ZeroTouchRule]
}

// WrapZeroTouchRules men-cache ListActiveOrdered. ttl <= 0 mengembalikan repo asli.
func WrapZeroTouchRules(inner domain.ZeroTouchRuleRepository, ttl time.Duration) domain.ZeroTouchRuleRepository {
	if ttl <= 0 {
		return inner
	}
	return &zeroTouchRules{ZeroTouchRuleRepository: inner, cache: newTTLMap[[]domain.ZeroTouchRule](ttl)}
}

func (r *zeroTouchRules) ListActiveOrdered(ctx context.Context, tenantID *uint64) ([]domain.ZeroTouchRule, error) {
	key := tenantKey(tenantID)
	if v, ok := r.cache.get(key); ok {
		return append([]domain.ZeroTouchRule(nil), v...), nil
	}
	v, err := r.ZeroTouchRuleRepository.ListActiveOrdered(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	r.cache.put(key, append([]domain.ZeroTouchRule(nil), v...))
	return v, nil
}

func (r *zeroTouchRules) Create(ctx context.Context, x *domain.ZeroTouchRule) error {
	defer r.cache.clear()
	return r.ZeroTouchRuleRepository.Create(ctx, x)
}

func (r *zeroTouchRules) Update(ctx context.Context, x *domain.ZeroTouchRule) error {
	defer r.cache.clear()
	return r.ZeroTouchRuleRepository.Update(ctx, x)
}

func (r *zeroTouchRules) SoftDelete(ctx context.Context, id, deletedBy uint64) error {
	defer r.cache.clear()
	return r.ZeroTouchRuleRepository.SoftDelete(ctx, id, deletedBy)
}

// ---- Preset ----

type presets struct {
	domain.PresetRepository
	cache *ttlMap[[]domain.Preset]
}

// WrapPresets men-cache ListActiveEnforce. ttl <= 0 mengembalikan repo asli.
func WrapPresets(inner domain.PresetRepository, ttl time.Duration) domain.PresetRepository {
	if ttl <= 0 {
		return inner
	}
	return &presets{PresetRepository: inner, cache: newTTLMap[[]domain.Preset](ttl)}
}

func (r *presets) ListActiveEnforce(ctx context.Context, tenantID *uint64) ([]domain.Preset, error) {
	key := tenantKey(tenantID)
	if v, ok := r.cache.get(key); ok {
		return append([]domain.Preset(nil), v...), nil
	}
	v, err := r.PresetRepository.ListActiveEnforce(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	r.cache.put(key, append([]domain.Preset(nil), v...))
	return v, nil
}

func (r *presets) Create(ctx context.Context, x *domain.Preset) error {
	defer r.cache.clear()
	return r.PresetRepository.Create(ctx, x)
}

func (r *presets) Update(ctx context.Context, x *domain.Preset) error {
	defer r.cache.clear()
	return r.PresetRepository.Update(ctx, x)
}

func (r *presets) Delete(ctx context.Context, id uint64) error {
	defer r.cache.clear()
	return r.PresetRepository.Delete(ctx, id)
}
