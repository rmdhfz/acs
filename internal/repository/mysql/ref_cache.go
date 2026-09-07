package mysql

import (
	"context"
	"sync"
	"time"

	"acs/internal/domain"
)

// immutableRefTables — tabel ref_* yang isinya di-seed migrasi dan TIDAK
// PERNAH dimutasi aplikasi (enum standar: event code Broadband Forum, status
// task/device/rollout, trigger ZTP, role, tipe parameter). Aman di-cache
// selamanya (TTL hanya backstop). ref_vendors / ref_device_types /
// ref_data_model_versions SENGAJA dikecualikan — tumbuh lewat operasi katalog.
var immutableRefTables = map[string]bool{
	domain.RefTableEventCodes:            true,
	domain.RefTableTaskTypes:             true,
	domain.RefTableTaskStatus:            true,
	domain.RefTableDeviceStatus:          true,
	domain.RefTableZtpTriggerEvent:       true,
	domain.RefTableFirmwareRolloutStatus: true,
	domain.RefTableRoles:                 true,
	domain.RefTableParameterTypes:        true,
}

const refCacheTTL = 10 * time.Minute

// cachedRefRepository membungkus RefRepository dengan cache in-memory untuk
// GetByCode/GetByID pada tabel imutabel — menghilangkan ~6 query DB per Inform
// (event code, trigger ZTP ×2, status device, task type, task status) yang
// selama ini menembak DB tiap kali padahal isinya konstan (temuan
// perf-scale-auditor). List() TIDAK di-cache (dipakai UI, boleh kena tabel
// mutable).
type cachedRefRepository struct {
	inner domain.RefRepository
	mu    sync.RWMutex
	byKey map[string]refCacheEntry // "table\x00code" atau "table\x00#id"
}

type refCacheEntry struct {
	val domain.RefLookup
	exp time.Time
}

// NewCachedRefRepository membungkus repo ref dengan cache. Aman dipakai dari
// banyak goroutine (jalur Inform concurrent).
func NewCachedRefRepository(inner domain.RefRepository) domain.RefRepository {
	return &cachedRefRepository{inner: inner, byKey: map[string]refCacheEntry{}}
}

func (c *cachedRefRepository) get(key string) (domain.RefLookup, bool) {
	c.mu.RLock()
	e, ok := c.byKey[key]
	c.mu.RUnlock()
	if !ok || time.Now().After(e.exp) {
		return domain.RefLookup{}, false
	}
	return e.val, true
}

func (c *cachedRefRepository) put(key string, val domain.RefLookup) {
	c.mu.Lock()
	c.byKey[key] = refCacheEntry{val: val, exp: time.Now().Add(refCacheTTL)}
	c.mu.Unlock()
}

func (c *cachedRefRepository) GetByCode(ctx context.Context, table, code string) (domain.RefLookup, error) {
	if !immutableRefTables[table] {
		return c.inner.GetByCode(ctx, table, code)
	}
	key := table + "\x00" + code
	if v, ok := c.get(key); ok {
		return v, nil
	}
	v, err := c.inner.GetByCode(ctx, table, code)
	if err != nil {
		return v, err
	}
	c.put(key, v)
	c.put(table+"\x00#"+itoa(v.ID), v) // isi juga cache by-id
	return v, nil
}

func (c *cachedRefRepository) GetByID(ctx context.Context, table string, id uint64) (domain.RefLookup, error) {
	if !immutableRefTables[table] {
		return c.inner.GetByID(ctx, table, id)
	}
	key := table + "\x00#" + itoa(id)
	if v, ok := c.get(key); ok {
		return v, nil
	}
	v, err := c.inner.GetByID(ctx, table, id)
	if err != nil {
		return v, err
	}
	c.put(key, v)
	c.put(table+"\x00"+v.Code, v)
	return v, nil
}

// List tidak di-cache: dipakai UI (GET /refs/:table) dan bisa menyasar tabel
// mutable (ref_vendors, dst).
func (c *cachedRefRepository) List(ctx context.Context, table string) ([]domain.RefLookup, error) {
	return c.inner.List(ctx, table)
}

func itoa(v uint64) string {
	if v == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
	}
	return string(b[i:])
}
