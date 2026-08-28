package domain

import (
	"context"
	"time"
)

// WebhookSubscription — langganan webhook untuk SATU jenis event pada SATU
// URL target (migrations/0013). Secret HMAC tidak pernah dikembalikan setelah
// create (field `Secret` di bawah hanya terisi saat create untuk dikirim
// sekali ke client; kolom DB `secret_enc` terenkripsi AES-GCM).
type WebhookSubscription struct {
	ID               uint64  `db:"id" json:"id"`
	SubscriptionUUID string  `db:"subscription_uuid" json:"subscription_uuid"`
	TenantID         *uint64 `db:"tenant_id" json:"tenant_id"` // NULL = global (SUPERADMIN)
	EventTypeID      uint64  `db:"event_type_id" json:"event_type_id"`
	Name             string  `db:"name" json:"name"`
	TargetURL        string  `db:"target_url" json:"target_url"`
	// SecretEnc — ciphertext AES-GCM secret HMAC. `json:"-"` — tidak pernah
	// bocor ke response (pola sama seperti kredensial lain di codebase ini).
	SecretEnc   []byte  `db:"secret_enc" json:"-"`
	IsActive    bool    `db:"is_active" json:"is_active"`
	Description *string `db:"description" json:"description"`
	Audit

	// Secret — plaintext, HANYA diisi pada response create (lihat
	// webhook.Service.CreateSubscription). Tidak pernah dibaca dari DB.
	Secret string `db:"-" json:"secret,omitempty"`
}

// WebhookDelivery — satu percobaan (atau antrean percobaan) pengiriman satu
// event ke satu subscription (migrations/0013). Tabel log volume tinggi:
// audit minimal, tanpa soft-delete.
type WebhookDelivery struct {
	ID             uint64         `db:"id" json:"id"`
	DeliveryUUID   string         `db:"delivery_uuid" json:"delivery_uuid"`
	SubscriptionID uint64         `db:"subscription_id" json:"subscription_id"`
	EventTypeID    uint64         `db:"event_type_id" json:"event_type_id"`
	Payload        JSONRawMessage `db:"payload" json:"payload"`
	Status         string         `db:"status" json:"status"` // PENDING, DELIVERED, FAILED
	AttemptCount   uint32         `db:"attempt_count" json:"attempt_count"`
	MaxAttempts    uint32         `db:"max_attempts" json:"max_attempts"`
	ResponseStatus *int           `db:"response_status" json:"response_status"`
	ErrorMessage   *string        `db:"error_message" json:"error_message"`
	NextAttemptAt  *time.Time     `db:"next_attempt_at" json:"next_attempt_at"`
	DeliveredAt    *time.Time     `db:"delivered_at" json:"delivered_at"`
	CreatedAt      time.Time      `db:"created_at" json:"created_at"`
	UpdatedAt      time.Time      `db:"updated_at" json:"updated_at"`
}

const (
	WebhookDeliveryStatusPending   = "PENDING"
	WebhookDeliveryStatusDelivered = "DELIVERED"
	WebhookDeliveryStatusFailed    = "FAILED"
)

type WebhookSubscriptionRepository interface {
	Create(ctx context.Context, s *WebhookSubscription) error
	GetByID(ctx context.Context, id uint64) (*WebhookSubscription, error)
	// List — tenantID nil untuk SUPERADMIN berarti semua tenant + global;
	// untuk non-superadmin, pemanggil WAJIB sudah memaksa tenantID ke tenant
	// actor (lihat auth.ScopedTenantFilter).
	List(ctx context.Context, tenantID *uint64, p Pagination) ([]WebhookSubscription, int, error)
	// ListActiveForEvent — subscription aktif (is_active=1, is_deleted=0) yang
	// cocok untuk sebuah event: event_type_id sama DAN (tenant_id = tenantID
	// ATAU tenant_id IS NULL / global). tenantID nil (event tanpa tenant —
	// seharusnya jarang) hanya cocok subscription global.
	ListActiveForEvent(ctx context.Context, eventTypeID uint64, tenantID *uint64) ([]WebhookSubscription, error)
	Update(ctx context.Context, s *WebhookSubscription) error
	SoftDelete(ctx context.Context, id, deletedBy uint64) error
}

type WebhookDeliveryRepository interface {
	Create(ctx context.Context, d *WebhookDelivery) error
	GetByID(ctx context.Context, id uint64) (*WebhookDelivery, error)
	ListBySubscription(ctx context.Context, subscriptionID uint64, p Pagination) ([]WebhookDelivery, int, error)
	// ClaimDue — ambil sampai `limit` baris berstatus PENDING yang
	// next_attempt_at <= now (atau NULL), untuk diproses worker. Implementasi
	// memakai `FOR UPDATE SKIP LOCKED` (MariaDB 10.6+) supaya beberapa
	// instance acsd tidak memproses baris yang sama (app server stateless,
	// TECH.md §9).
	ClaimDue(ctx context.Context, now time.Time, limit int) ([]WebhookDelivery, error)
	// MarkResult menyimpan hasil satu percobaan: status baru, attempt_count,
	// response_status, error, next_attempt_at (nil bila terminal), delivered_at.
	MarkResult(ctx context.Context, id uint64, status string, attemptCount uint32, responseStatus *int, errMsg *string, nextAttemptAt, deliveredAt *time.Time) error
	CountFailed(ctx context.Context, tenantID *uint64) (int, error)
}

// WebhookEnqueuer dipakai usecase lain (session, task) untuk mem-fan-out
// sebuah event ke semua subscription yang cocok TANPA bergantung langsung
// pada package usecase/webhook (menghindari import cycle — pola sama seperti
// domain.TaskEnqueuer / domain.FirmwareScheduler). Implementasi:
// usecase/webhook.Service.Enqueue.
//
// eventCode adalah salah satu konstanta WebhookEvent* di common.go.
// tenantID boleh nil (event tanpa tenant). payload harus JSON-marshalable.
// Enqueue TIDAK memblokir untuk I/O jaringan — hanya menulis baris
// webhook_deliveries berstatus PENDING; pengiriman HTTP sesungguhnya
// dilakukan worker periodik (cmd/acsd). Error dari Enqueue TIDAK boleh
// menggagalkan alur pemanggil (sesi CWMP, task) — pemanggil me-log & lanjut.
type WebhookEnqueuer interface {
	Enqueue(ctx context.Context, eventCode string, tenantID *uint64, payload any) error
}
