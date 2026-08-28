package mysql

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"

	"acs/internal/domain"
)

// ---- WebhookSubscription ----

type webhookSubscriptionRepository struct{ db *sqlx.DB }

func NewWebhookSubscriptionRepository(db *sqlx.DB) domain.WebhookSubscriptionRepository {
	return &webhookSubscriptionRepository{db: db}
}

func (r *webhookSubscriptionRepository) Create(ctx context.Context, s *domain.WebhookSubscription) error {
	now := time.Now()
	s.CreatedAt, s.UpdatedAt = now, now
	const q = `INSERT INTO webhook_subscriptions
		(subscription_uuid, tenant_id, event_type_id, name, target_url, secret_enc, is_active, description,
		 created_at, updated_at, created_by)
		VALUES (:subscription_uuid, :tenant_id, :event_type_id, :name, :target_url, :secret_enc, :is_active, :description,
		 :created_at, :updated_at, :created_by)`
	res, err := r.db.NamedExecContext(ctx, q, s)
	if err != nil {
		return translateErr(err)
	}
	id, _ := res.LastInsertId()
	s.ID = uint64(id)
	return nil
}

func (r *webhookSubscriptionRepository) GetByID(ctx context.Context, id uint64) (*domain.WebhookSubscription, error) {
	var s domain.WebhookSubscription
	err := r.db.GetContext(ctx, &s,
		`SELECT * FROM webhook_subscriptions WHERE id = ? AND is_deleted = 0`, id)
	if err != nil {
		return nil, translateErr(err)
	}
	return &s, nil
}

func (r *webhookSubscriptionRepository) List(ctx context.Context, tenantID *uint64, p domain.Pagination) ([]domain.WebhookSubscription, int, error) {
	where := "is_deleted = 0"
	args := []any{}
	if tenantID != nil {
		where += " AND tenant_id = ?"
		args = append(args, *tenantID)
	}
	var total int
	if err := r.db.GetContext(ctx, &total, "SELECT COUNT(*) FROM webhook_subscriptions WHERE "+where, args...); err != nil {
		return nil, 0, translateErr(err)
	}
	rows := []domain.WebhookSubscription{}
	args = append(args, p.Limit(), p.Offset())
	if err := r.db.SelectContext(ctx, &rows,
		"SELECT * FROM webhook_subscriptions WHERE "+where+" ORDER BY id DESC LIMIT ? OFFSET ?", args...); err != nil {
		return nil, 0, translateErr(err)
	}
	return rows, total, nil
}

func (r *webhookSubscriptionRepository) ListActiveForEvent(ctx context.Context, eventTypeID uint64, tenantID *uint64) ([]domain.WebhookSubscription, error) {
	rows := []domain.WebhookSubscription{}
	// Subscription global (tenant_id IS NULL) selalu ikut; subscription
	// bertenant hanya ikut bila tenant-nya sama dgn event.
	var err error
	if tenantID != nil {
		err = r.db.SelectContext(ctx, &rows,
			`SELECT * FROM webhook_subscriptions
			 WHERE event_type_id = ? AND is_active = 1 AND is_deleted = 0
			   AND (tenant_id = ? OR tenant_id IS NULL)`,
			eventTypeID, *tenantID)
	} else {
		err = r.db.SelectContext(ctx, &rows,
			`SELECT * FROM webhook_subscriptions
			 WHERE event_type_id = ? AND is_active = 1 AND is_deleted = 0 AND tenant_id IS NULL`,
			eventTypeID)
	}
	if err != nil {
		return nil, translateErr(err)
	}
	return rows, nil
}

func (r *webhookSubscriptionRepository) Update(ctx context.Context, s *domain.WebhookSubscription) error {
	const q = `UPDATE webhook_subscriptions
		SET name = :name, target_url = :target_url, is_active = :is_active, description = :description,
		    updated_by = :updated_by
		WHERE id = :id AND is_deleted = 0`
	_, err := r.db.NamedExecContext(ctx, q, s)
	return translateErr(err)
}

func (r *webhookSubscriptionRepository) SoftDelete(ctx context.Context, id, deletedBy uint64) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE webhook_subscriptions SET is_deleted = 1, deleted_at = NOW(), deleted_by = ?
		 WHERE id = ? AND is_deleted = 0`, deletedBy, id)
	return translateErr(err)
}

// ---- WebhookDelivery ----

type webhookDeliveryRepository struct{ db *sqlx.DB }

func NewWebhookDeliveryRepository(db *sqlx.DB) domain.WebhookDeliveryRepository {
	return &webhookDeliveryRepository{db: db}
}

func (r *webhookDeliveryRepository) Create(ctx context.Context, d *domain.WebhookDelivery) error {
	now := time.Now()
	d.CreatedAt, d.UpdatedAt = now, now
	const q = `INSERT INTO webhook_deliveries
		(delivery_uuid, subscription_id, event_type_id, payload, status, attempt_count, max_attempts,
		 next_attempt_at, created_at, updated_at)
		VALUES (:delivery_uuid, :subscription_id, :event_type_id, :payload, :status, :attempt_count, :max_attempts,
		 :next_attempt_at, :created_at, :updated_at)`
	res, err := r.db.NamedExecContext(ctx, q, d)
	if err != nil {
		return translateErr(err)
	}
	id, _ := res.LastInsertId()
	d.ID = uint64(id)
	return nil
}

func (r *webhookDeliveryRepository) GetByID(ctx context.Context, id uint64) (*domain.WebhookDelivery, error) {
	var d domain.WebhookDelivery
	err := r.db.GetContext(ctx, &d, `SELECT * FROM webhook_deliveries WHERE id = ?`, id)
	if err != nil {
		return nil, translateErr(err)
	}
	return &d, nil
}

func (r *webhookDeliveryRepository) ListBySubscription(ctx context.Context, subscriptionID uint64, p domain.Pagination) ([]domain.WebhookDelivery, int, error) {
	var total int
	if err := r.db.GetContext(ctx, &total,
		`SELECT COUNT(*) FROM webhook_deliveries WHERE subscription_id = ?`, subscriptionID); err != nil {
		return nil, 0, translateErr(err)
	}
	rows := []domain.WebhookDelivery{}
	if err := r.db.SelectContext(ctx, &rows,
		`SELECT * FROM webhook_deliveries WHERE subscription_id = ? ORDER BY id DESC LIMIT ? OFFSET ?`,
		subscriptionID, p.Limit(), p.Offset()); err != nil {
		return nil, 0, translateErr(err)
	}
	return rows, total, nil
}

// ClaimDue — lihat komentar di domain.WebhookDeliveryRepository. Pendekatan
// sederhana: SELECT baris jatuh-tempo, worker memproses & MarkResult
// (guarded UPDATE). Pada deployment multi-instance ini BISA menghasilkan
// pengiriman ganda sesekali — receiver WAJIB idempotent (dedup via header
// `X-ACS-Delivery-Id`). Trade-off yang disadari & didokumentasikan, konsisten
// dgn gaya trade-off lain di codebase ini (mis. TOCTOU kuota task queue).
func (r *webhookDeliveryRepository) ClaimDue(ctx context.Context, now time.Time, limit int) ([]domain.WebhookDelivery, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows := []domain.WebhookDelivery{}
	err := r.db.SelectContext(ctx, &rows,
		`SELECT * FROM webhook_deliveries
		 WHERE status = 'PENDING' AND (next_attempt_at IS NULL OR next_attempt_at <= ?)
		 ORDER BY id LIMIT ?`, now, limit)
	if err != nil {
		return nil, translateErr(err)
	}
	return rows, nil
}

func (r *webhookDeliveryRepository) MarkResult(ctx context.Context, id uint64, status string, attemptCount uint32, responseStatus *int, errMsg *string, nextAttemptAt, deliveredAt *time.Time) error {
	// Guarded: hanya ubah baris yang MASIH PENDING — bila instance lain sudah
	// menuntaskan, UPDATE ini kena 0 baris & itu tidak apa-apa.
	_, err := r.db.ExecContext(ctx,
		`UPDATE webhook_deliveries
		 SET status = ?, attempt_count = ?, response_status = ?, error_message = ?,
		     next_attempt_at = ?, delivered_at = ?, updated_at = NOW()
		 WHERE id = ? AND status = 'PENDING'`,
		status, attemptCount, responseStatus, errMsg, nextAttemptAt, deliveredAt, id)
	return translateErr(err)
}

func (r *webhookDeliveryRepository) CountFailed(ctx context.Context, tenantID *uint64) (int, error) {
	var count int
	var err error
	if tenantID != nil {
		err = r.db.GetContext(ctx, &count,
			`SELECT COUNT(*) FROM webhook_deliveries d
			 INNER JOIN webhook_subscriptions s ON d.subscription_id = s.id
			 WHERE d.status = 'FAILED' AND s.tenant_id = ? AND s.is_deleted = 0`, *tenantID)
	} else {
		err = r.db.GetContext(ctx, &count,
			`SELECT COUNT(*) FROM webhook_deliveries WHERE status = 'FAILED'`)
	}
	if err != nil {
		return 0, translateErr(err)
	}
	return count, nil
}

