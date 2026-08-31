package redisrepo

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"acs/internal/domain"
	"acs/pkg/redisutil"
)

const (
	sessionKeyPrefix = "acs:cwmp:session:"
	// sessionIDKeyPrefix — indeks balik id->token, supaya mutator yang hanya
	// menerima id (UpdateStatus/SetCWMPID/SetCWMPNamespace) bisa menemukan &
	// menginvalidasi entri cache token-nya.
	sessionIDKeyPrefix = "acs:cwmp:session:byid:"
	// sessionCacheTTL — sesi CWMP normal hanya hidup beberapa detik s/d menit;
	// yang lebih tua dari ini di-reap sweeper cmd/acsd jadi TIMEOUT. TTL pendek
	// (bukan 1 jam) membatasi jendela di mana entri cache bisa basi relatif ke
	// MySQL (sumber kebenaran) untuk jalur yang tidak lewat invalidasi eksplisit
	// (mis. bulk TimeoutStaleOpen).
	sessionCacheTTL = 15 * time.Minute
)

type deviceSessionRepository struct {
	mysqlRepo domain.DeviceSessionRepository
	redis     *redisutil.Client
}

// NewDeviceSessionRepository membungkus MySQL repo eksisting dengan Redis cache
// (Fase 4: Stateless Microservices & Redis, TECH.md §9).
func NewDeviceSessionRepository(mysqlRepo domain.DeviceSessionRepository, redis *redisutil.Client) domain.DeviceSessionRepository {
	return &deviceSessionRepository{
		mysqlRepo: mysqlRepo,
		redis:     redis,
	}
}

// cache menyimpan sesi ke Redis (entri token + indeks balik id->token). Best
// effort — kegagalan Redis tidak menggagalkan operasi (MySQL tetap otoritatif).
func (r *deviceSessionRepository) cache(ctx context.Context, s *domain.DeviceSession) {
	b, err := json.Marshal(s)
	if err != nil {
		return
	}
	_ = r.redis.DB().Set(ctx, sessionKeyPrefix+s.SessionToken, b, sessionCacheTTL).Err()
	_ = r.redis.DB().Set(ctx, sessionIDKeyPrefix+strconv.FormatUint(s.ID, 10), s.SessionToken, sessionCacheTTL).Err()
}

// invalidate menghapus entri cache untuk sesi id (dipanggil setiap mutasi
// lewat id). Tanpa ini, cache bisa terus menyajikan status/namespace/cwmp_id
// lama sampai TTL habis — mis. namespace CWMP yang di-set saat Inform
// (migrations/0012) tidak akan terlihat NextRequest karena membaca salinan
// cache pra-SetCWMPNamespace.
func (r *deviceSessionRepository) invalidate(ctx context.Context, id uint64) {
	idKey := sessionIDKeyPrefix + strconv.FormatUint(id, 10)
	token, err := r.redis.DB().Get(ctx, idKey).Result()
	if err == nil && token != "" {
		_ = r.redis.DB().Del(ctx, sessionKeyPrefix+token).Err()
	}
	_ = r.redis.DB().Del(ctx, idKey).Err()
}

func (r *deviceSessionRepository) Create(ctx context.Context, s *domain.DeviceSession) error {
	// 1. Audit trail ke MySQL (mengisi s.ID)
	if err := r.mysqlRepo.Create(ctx, s); err != nil {
		return err
	}
	// 2. Cache ke Redis
	r.cache(ctx, s)
	return nil
}

func (r *deviceSessionRepository) GetByToken(ctx context.Context, token string) (*domain.DeviceSession, error) {
	// 1. Coba Redis (hot path)
	b, err := r.redis.DB().Get(ctx, sessionKeyPrefix+token).Bytes()
	if err == nil {
		var s domain.DeviceSession
		if err := json.Unmarshal(b, &s); err == nil {
			return &s, nil
		}
	}

	// 2. Fallback ke MySQL + isi ulang cache supaya baca berikutnya dalam sesi
	// yang sama kembali kena hot path.
	s, err := r.mysqlRepo.GetByToken(ctx, token)
	if err != nil {
		return nil, err
	}
	r.cache(ctx, s)
	return s, nil
}

func (r *deviceSessionRepository) UpdateStatus(ctx context.Context, id uint64, status string, endedAt *time.Time) error {
	if err := r.mysqlRepo.UpdateStatus(ctx, id, status, endedAt); err != nil {
		return err
	}
	r.invalidate(ctx, id)
	return nil
}

func (r *deviceSessionRepository) SetCWMPID(ctx context.Context, id uint64, cwmpID string) error {
	if err := r.mysqlRepo.SetCWMPID(ctx, id, cwmpID); err != nil {
		return err
	}
	r.invalidate(ctx, id)
	return nil
}

func (r *deviceSessionRepository) SetCWMPNamespace(ctx context.Context, id uint64, namespace string) error {
	if err := r.mysqlRepo.SetCWMPNamespace(ctx, id, namespace); err != nil {
		return err
	}
	r.invalidate(ctx, id)
	return nil
}

func (r *deviceSessionRepository) CountOpen(ctx context.Context) (int, error) {
	// Karena OPEN sessions disimpan di mysql dengan presisi tinggi, fallback mysql
	return r.mysqlRepo.CountOpen(ctx)
}

// TimeoutStaleOpen — reap sesi OPEN yang basi (sweeper cmd/acsd). Dijalankan di
// MySQL (sumber kebenaran status sesi). Entri cache per-sesi yang di-reap tidak
// diinvalidasi satu per satu di sini (operasi bulk, tanpa daftar id) — aman
// karena sessionCacheTTL (15 mnt) <= ambang reap sweeper, jadi entri cache sesi
// setua itu sudah kedaluwarsa atau segera kedaluwarsa, dan guard
// Status==OPEN di resolveSession/NextRequest membuat sesi mati yang masih
// ter-cache tidak menimbulkan aksi salah.
func (r *deviceSessionRepository) TimeoutStaleOpen(ctx context.Context, olderThan time.Time) (int64, error) {
	return r.mysqlRepo.TimeoutStaleOpen(ctx, olderThan)
}
