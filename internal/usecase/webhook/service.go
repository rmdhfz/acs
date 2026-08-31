// Package webhook mengelola langganan webhook keluar (typed event) dan
// pengirimannya ke sistem pihak ketiga (BSS/OSS/NMS) — `/goal` directive #3,
// PRD.md §4.1. Event yang didukung: DEVICE_FAULT, PARAMETER_VALUE_CHANGE,
// TASK_FAILED (lihat domain.WebhookEvent*).
//
// Alur:
//   - usecase lain (session, task) memanggil Service.Enqueue (via
//     domain.WebhookEnqueuer) — NON-BLOCKING, hanya menulis baris
//     webhook_deliveries berstatus PENDING per subscription yang cocok.
//   - Worker periodik cmd/acsd memanggil Service.DispatchDue — mengambil
//     baris jatuh tempo, POST JSON ber-HMAC ke target_url, update status,
//     retry berjenjang (backoff eksponensial) sampai max_attempts.
package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"

	"acs/internal/domain"
	"acs/internal/usecase/auth"
	"acs/pkg/cryptoutil"
)

const (
	defaultMaxAttempts   = 6
	deliveryHTTPTimeout  = 10 * time.Second
	maxBackoff           = 30 * time.Minute
	dispatchBatchPerTick = 100
)

type Service struct {
	subs       domain.WebhookSubscriptionRepository
	deliveries domain.WebhookDeliveryRepository
	refs       domain.RefRepository
	activity   domain.ActivityLogRepository
	enc        *cryptoutil.Encryptor
	http       *http.Client
	logger     *slog.Logger
}

func NewService(
	subs domain.WebhookSubscriptionRepository,
	deliveries domain.WebhookDeliveryRepository,
	refs domain.RefRepository,
	activity domain.ActivityLogRepository,
	enc *cryptoutil.Encryptor,
	logger *slog.Logger,
) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		subs: subs, deliveries: deliveries, refs: refs, activity: activity, enc: enc,
		http:   &http.Client{Timeout: deliveryHTTPTimeout},
		logger: logger,
	}
}

// ---- Subscription CRUD ----

type CreateSubscriptionInput struct {
	TenantID    *uint64
	EventCode   string
	Name        string
	TargetURL   string
	Description *string
}

// CreateSubscription membuat langganan + men-generate secret HMAC acak
// (dikembalikan plaintext SEKALI di field Secret). TenantID nil (global) hanya
// boleh SUPERADMIN — pola sama dgn resource lintas-tenant lain (mis. rollout
// batch global, provisioning profile global).
func (s *Service) CreateSubscription(ctx context.Context, actor domain.Actor, in CreateSubscriptionInput) (*domain.WebhookSubscription, error) {
	if in.TenantID == nil {
		if !actor.IsSuperadmin() {
			return nil, domain.ErrForbidden
		}
	} else if err := auth.RequireTenantScope(actor, in.TenantID); err != nil {
		return nil, err
	}
	if in.Name == "" {
		return nil, fmt.Errorf("%w: name wajib diisi", domain.ErrInvalidInput)
	}
	if err := validateTargetURL(in.TargetURL); err != nil {
		return nil, err
	}
	evt, err := s.refs.GetByCode(ctx, domain.RefTableWebhookEventTypes, in.EventCode)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, fmt.Errorf("%w: event_type %q tidak dikenal", domain.ErrInvalidInput, in.EventCode)
		}
		return nil, err
	}

	secret, err := randomSecret()
	if err != nil {
		return nil, err
	}
	secretEnc, err := s.enc.Encrypt(secret)
	if err != nil {
		return nil, err
	}

	sub := &domain.WebhookSubscription{
		SubscriptionUUID: uuid.NewString(),
		TenantID:         in.TenantID,
		EventTypeID:      evt.ID,
		Name:             in.Name,
		TargetURL:        in.TargetURL,
		SecretEnc:        secretEnc,
		IsActive:         true,
		Description:      in.Description,
		Audit:            domain.Audit{CreatedBy: actor.UserIDPtr()},
	}
	if err := s.subs.Create(ctx, sub); err != nil {
		return nil, err
	}
	_ = s.activity.Record(ctx, &domain.ActivityLog{
		UserID: actor.UserIDPtr(), TenantID: actor.TenantID,
		Action: "CREATE_WEBHOOK_SUBSCRIPTION", EntityType: "webhook_subscription", EntityID: &sub.ID,
	})
	sub.Secret = secret // dikirim sekali ke client
	return sub, nil
}

func (s *Service) GetSubscription(ctx context.Context, actor domain.Actor, id uint64) (*domain.WebhookSubscription, error) {
	sub, err := s.subs.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.requireScope(actor, sub); err != nil {
		return nil, err
	}
	return sub, nil
}

// ListSubscriptions — queryTenantID hanya efektif untuk SUPERADMIN;
// non-superadmin selalu dipaksa ke tenant sendiri (ditolak bila TenantID nil).
func (s *Service) ListSubscriptions(ctx context.Context, actor domain.Actor, queryTenantID *uint64, p domain.Pagination) ([]domain.WebhookSubscription, int, error) {
	tenantID := queryTenantID
	if !actor.IsSuperadmin() {
		scoped, err := auth.ScopedTenantFilter(actor)
		if err != nil {
			return nil, 0, err
		}
		tenantID = scoped
	}
	return s.subs.List(ctx, tenantID, p)
}

type UpdateSubscriptionInput struct {
	Name      string
	TargetURL string
	// IsActive pointer: nil = tidak diubah, false = nonaktifkan, true = aktifkan.
	// Menggunakan *bool supaya klien bisa secara eksplisit menonaktifkan
	// webhook (kalau bool biasa, false tidak bisa dibedakan dari "tidak diisi").
	IsActive    *bool
	Description *string
}

func (s *Service) UpdateSubscription(ctx context.Context, actor domain.Actor, id uint64, in UpdateSubscriptionInput) error {
	sub, err := s.subs.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if err := s.requireScope(actor, sub); err != nil {
		return err
	}
	if in.Name == "" {
		return fmt.Errorf("%w: name wajib diisi", domain.ErrInvalidInput)
	}
	if err := validateTargetURL(in.TargetURL); err != nil {
		return err
	}
	sub.Name = in.Name
	sub.TargetURL = in.TargetURL
	if in.IsActive != nil {
		sub.IsActive = *in.IsActive
	}
	sub.Description = in.Description
	sub.UpdatedBy = actor.UserIDPtr()
	if err := s.subs.Update(ctx, sub); err != nil {
		return err
	}
	_ = s.activity.Record(ctx, &domain.ActivityLog{
		UserID: actor.UserIDPtr(), TenantID: actor.TenantID,
		Action: "UPDATE_WEBHOOK_SUBSCRIPTION", EntityType: "webhook_subscription", EntityID: &id,
	})
	return nil
}

func (s *Service) DeleteSubscription(ctx context.Context, actor domain.Actor, id uint64) error {
	sub, err := s.subs.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if err := s.requireScope(actor, sub); err != nil {
		return err
	}
	uid := uint64(0)
	if actor.UserIDPtr() != nil {
		uid = *actor.UserIDPtr()
	}
	if err := s.subs.SoftDelete(ctx, id, uid); err != nil {
		return err
	}
	_ = s.activity.Record(ctx, &domain.ActivityLog{
		UserID: actor.UserIDPtr(), TenantID: actor.TenantID,
		Action: "DELETE_WEBHOOK_SUBSCRIPTION", EntityType: "webhook_subscription", EntityID: &id,
	})
	return nil
}

func (s *Service) ListDeliveries(ctx context.Context, actor domain.Actor, subID uint64, p domain.Pagination) ([]domain.WebhookDelivery, int, error) {
	if _, err := s.GetSubscription(ctx, actor, subID); err != nil {
		return nil, 0, err
	}
	return s.deliveries.ListBySubscription(ctx, subID, p)
}

// TestSubscription mengantre satu delivery uji (payload sintetis) ke
// subscription — supaya operator bisa memverifikasi endpoint mereka menerima
// & memvalidasi signature dengan benar sebelum menunggu event nyata.
func (s *Service) TestSubscription(ctx context.Context, actor domain.Actor, id uint64) (*domain.WebhookDelivery, error) {
	sub, err := s.GetSubscription(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{
		"event": "webhook.test",
		"note":  "Delivery uji dari ACS — endpoint & verifikasi signature Anda berfungsi.",
		"at":    time.Now().UTC().Format(time.RFC3339),
	}
	return s.createDelivery(ctx, sub, payload)
}

func (s *Service) CountFailedDeliveries(ctx context.Context, actor domain.Actor) (int, error) {
	if !actor.HasRole(domain.RoleSuperadmin) && actor.TenantID == nil {
		return 0, domain.ErrUnauthorized
	}
	// Superadmin get all tenants failures if tenantID is nil, else scoped to tenant
	var filterTenant *uint64
	if !actor.HasRole(domain.RoleSuperadmin) {
		filterTenant = actor.TenantID
	}
	return s.deliveries.CountFailed(ctx, filterTenant)
}

// ---- Enqueue (domain.WebhookEnqueuer) ----

// Enqueue mem-fan-out satu event ke semua subscription yang cocok. Dipanggil
// dari usecase/session & usecase/task. TIDAK melakukan I/O jaringan; error
// dikembalikan tapi pemanggil WAJIB tidak menggagalkan alurnya karena ini.
func (s *Service) Enqueue(ctx context.Context, eventCode string, tenantID *uint64, payload any) error {
	evt, err := s.refs.GetByCode(ctx, domain.RefTableWebhookEventTypes, eventCode)
	if err != nil {
		return fmt.Errorf("webhook: event code %q: %w", eventCode, err)
	}
	subs, err := s.subs.ListActiveForEvent(ctx, evt.ID, tenantID)
	if err != nil {
		return err
	}
	if len(subs) == 0 {
		return nil
	}
	envelope := map[string]any{
		"event":       eventCode,
		"tenant_id":   tenantID,
		"occurred_at": time.Now().UTC().Format(time.RFC3339),
		"data":        payload,
	}
	var firstErr error
	for i := range subs {
		if _, err := s.createDelivery(ctx, &subs[i], envelope); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (s *Service) createDelivery(ctx context.Context, sub *domain.WebhookSubscription, payload any) (*domain.WebhookDelivery, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("webhook: marshal payload: %w", err)
	}
	d := &domain.WebhookDelivery{
		DeliveryUUID:   uuid.NewString(),
		SubscriptionID: sub.ID,
		EventTypeID:    sub.EventTypeID,
		Payload:        domain.JSONRawMessage(raw),
		Status:         domain.WebhookDeliveryStatusPending,
		AttemptCount:   0,
		MaxAttempts:    defaultMaxAttempts,
	}
	if err := s.deliveries.Create(ctx, d); err != nil {
		return nil, err
	}
	return d, nil
}

// ---- Dispatch worker ----

// DispatchDue diproses worker periodik cmd/acsd. Mengembalikan jumlah delivery
// yang diproses (sukses + gagal + dijadwalkan ulang) pada tick ini.
func (s *Service) DispatchDue(ctx context.Context) (int, error) {
	due, err := s.deliveries.ClaimDue(ctx, time.Now(), dispatchBatchPerTick)
	if err != nil {
		return 0, err
	}
	for i := range due {
		s.attemptDelivery(ctx, &due[i])
	}
	return len(due), nil
}

func (s *Service) attemptDelivery(ctx context.Context, d *domain.WebhookDelivery) {
	sub, err := s.subs.GetByID(ctx, d.SubscriptionID)
	if err != nil {
		// Subscription hilang/terhapus — delivery tidak akan pernah sukses.
		msg := "subscription tidak ditemukan / terhapus"
		_ = s.deliveries.MarkResult(ctx, d.ID, domain.WebhookDeliveryStatusFailed, d.AttemptCount+1, nil, &msg, nil, nil)
		return
	}
	secret, err := s.enc.Decrypt(sub.SecretEnc)
	if err != nil {
		msg := "gagal mendekripsi secret subscription"
		s.logger.Error("webhook: "+msg, "subscription_id", sub.ID, "error", err)
		_ = s.deliveries.MarkResult(ctx, d.ID, domain.WebhookDeliveryStatusFailed, d.AttemptCount+1, nil, &msg, nil, nil)
		return
	}

	attempt := d.AttemptCount + 1
	status, respStatus, errMsg := s.postDelivery(ctx, sub, d, []byte(d.Payload), secret)

	if status == domain.WebhookDeliveryStatusDelivered {
		now := time.Now()
		_ = s.deliveries.MarkResult(ctx, d.ID, status, attempt, respStatus, nil, nil, &now)
		s.logger.Info("webhook: delivery sukses", "delivery_id", d.ID, "subscription_id", sub.ID, "attempt", attempt, "response_status", derefInt(respStatus))
		return
	}

	if attempt >= d.MaxAttempts {
		_ = s.deliveries.MarkResult(ctx, d.ID, domain.WebhookDeliveryStatusFailed, attempt, respStatus, errMsg, nil, nil)
		s.logger.Warn("webhook: delivery GAGAL permanen (habis attempt)", "delivery_id", d.ID, "subscription_id", sub.ID, "attempt", attempt, "error", derefStr(errMsg))
		return
	}
	next := time.Now().Add(backoff(attempt))
	_ = s.deliveries.MarkResult(ctx, d.ID, domain.WebhookDeliveryStatusPending, attempt, respStatus, errMsg, &next, nil)
	s.logger.Info("webhook: delivery gagal, dijadwalkan ulang", "delivery_id", d.ID, "subscription_id", sub.ID, "attempt", attempt, "next_attempt_at", next.Format(time.RFC3339), "error", derefStr(errMsg))
}

// postDelivery melakukan POST HTTP tunggal. Mengembalikan (statusDelivery,
// responseStatus, errMsg). statusDelivery = DELIVERED bila 2xx, selain itu
// PENDING (dipetakan pemanggil ke retry / FAILED).
func (s *Service) postDelivery(ctx context.Context, sub *domain.WebhookSubscription, d *domain.WebhookDelivery, body []byte, secret string) (string, *int, *string) {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sub.TargetURL, bytes.NewReader(body))
	if err != nil {
		m := "gagal membangun request: " + err.Error()
		return domain.WebhookDeliveryStatusPending, nil, &m
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "ACS-Webhook/1.0")
	req.Header.Set("X-ACS-Delivery-Id", d.DeliveryUUID)
	req.Header.Set("X-ACS-Signature", sig)

	resp, err := s.http.Do(req)
	if err != nil {
		m := "koneksi gagal: " + err.Error()
		return domain.WebhookDeliveryStatusPending, nil, &m
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))

	code := resp.StatusCode
	if code >= 200 && code < 300 {
		return domain.WebhookDeliveryStatusDelivered, &code, nil
	}
	m := fmt.Sprintf("target membalas HTTP %d", code)
	return domain.WebhookDeliveryStatusPending, &code, &m
}

// ---- helpers ----

func (s *Service) requireScope(actor domain.Actor, sub *domain.WebhookSubscription) error {
	if sub.TenantID == nil {
		if !actor.IsSuperadmin() {
			return domain.ErrForbidden
		}
		return nil
	}
	return auth.RequireTenantScope(actor, sub.TenantID)
}

func validateTargetURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return fmt.Errorf("%w: target_url harus URL http(s) absolut yang valid", domain.ErrInvalidInput)
	}
	if len(raw) > 500 {
		return fmt.Errorf("%w: target_url maksimum 500 karakter", domain.ErrInvalidInput)
	}
	return nil
}

func randomSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("webhook: gagal generate secret: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// backoff — eksponensial: attempt 1 -> 1m, 2 -> 2m, 3 -> 4m, ... dibatasi
// maxBackoff. attempt di sini adalah nomor percobaan yang BARU SAJA gagal.
func backoff(attempt uint32) time.Duration {
	d := time.Minute
	for i := uint32(1); i < attempt && d < maxBackoff; i++ {
		d *= 2
	}
	if d > maxBackoff {
		d = maxBackoff
	}
	return d
}

func derefInt(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
