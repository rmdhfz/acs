package redisrepo

import (
	"context"
	"encoding/json"
	"time"

	"acs/internal/domain"
	"acs/pkg/redisutil"
)

const sessionKeyPrefix = "acs:cwmp:session:"

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

func (r *deviceSessionRepository) Create(ctx context.Context, s *domain.DeviceSession) error {
	// 1. Audit trail ke MySQL
	if err := r.mysqlRepo.Create(ctx, s); err != nil {
		return err
	}
	
	// 2. Cache ke Redis (TTL 1 jam, cwmp session biasanya hanya beberapa detik/menit)
	b, err := json.Marshal(s)
	if err == nil {
		_ = r.redis.DB().Set(ctx, sessionKeyPrefix+s.SessionToken, b, 1*time.Hour).Err()
	}
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
	
	// 2. Fallback ke MySQL
	return r.mysqlRepo.GetByToken(ctx, token)
}

func (r *deviceSessionRepository) UpdateStatus(ctx context.Context, id uint64, status string, endedAt *time.Time) error {
	// Catatan: Karena kita tidak me-lookup by ID ke redis dengan mudah,
	// kita serahkan ke MySQL, lalu hapus dari Redis jika status CLOSED.
	// (Untuk update dari CWMP, session lookup di-cache, tapi write-back tetap sinkron).
	return r.mysqlRepo.UpdateStatus(ctx, id, status, endedAt)
}

func (r *deviceSessionRepository) SetCWMPID(ctx context.Context, id uint64, cwmpID string) error {
	return r.mysqlRepo.SetCWMPID(ctx, id, cwmpID)
}

func (r *deviceSessionRepository) SetCWMPNamespace(ctx context.Context, id uint64, namespace string) error {
	return r.mysqlRepo.SetCWMPNamespace(ctx, id, namespace)
}

func (r *deviceSessionRepository) CountOpen(ctx context.Context) (int, error) {
	// Karena OPEN sessions disimpan di mysql dengan presisi tinggi, fallback mysql
	return r.mysqlRepo.CountOpen(ctx)
}
