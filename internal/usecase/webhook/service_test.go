package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"acs/internal/domain"
	"acs/pkg/cryptoutil"
)

// ---- fakes ----

type fakeSubRepo struct {
	mu     sync.Mutex
	nextID uint64
	rows   map[uint64]*domain.WebhookSubscription
}

func newFakeSubRepo() *fakeSubRepo {
	return &fakeSubRepo{rows: map[uint64]*domain.WebhookSubscription{}}
}

func (r *fakeSubRepo) Create(_ context.Context, s *domain.WebhookSubscription) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextID++
	s.ID = r.nextID
	cp := *s
	r.rows[s.ID] = &cp
	return nil
}

func (r *fakeSubRepo) GetByID(_ context.Context, id uint64) (*domain.WebhookSubscription, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.rows[id]
	if !ok || s.IsDeleted {
		return nil, domain.ErrNotFound
	}
	cp := *s
	return &cp, nil
}

func (r *fakeSubRepo) List(_ context.Context, tenantID *uint64, _ domain.Pagination) ([]domain.WebhookSubscription, int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []domain.WebhookSubscription
	for _, s := range r.rows {
		if s.IsDeleted {
			continue
		}
		if tenantID != nil && (s.TenantID == nil || *s.TenantID != *tenantID) {
			continue
		}
		out = append(out, *s)
	}
	return out, len(out), nil
}

func (r *fakeSubRepo) ListActiveForEvent(_ context.Context, eventTypeID uint64, tenantID *uint64) ([]domain.WebhookSubscription, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []domain.WebhookSubscription
	for _, s := range r.rows {
		if s.IsDeleted || !s.IsActive || s.EventTypeID != eventTypeID {
			continue
		}
		matchTenant := s.TenantID == nil || (tenantID != nil && *s.TenantID == *tenantID)
		if !matchTenant {
			continue
		}
		out = append(out, *s)
	}
	return out, nil
}

func (r *fakeSubRepo) Update(_ context.Context, s *domain.WebhookSubscription) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.rows[s.ID]; !ok {
		return domain.ErrNotFound
	}
	cp := *s
	r.rows[s.ID] = &cp
	return nil
}

func (r *fakeSubRepo) SoftDelete(_ context.Context, id, _ uint64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.rows[id]
	if !ok {
		return domain.ErrNotFound
	}
	s.IsDeleted = true
	return nil
}

type fakeDeliveryRepo struct {
	mu     sync.Mutex
	nextID uint64
	rows   map[uint64]*domain.WebhookDelivery
}

func newFakeDeliveryRepo() *fakeDeliveryRepo {
	return &fakeDeliveryRepo{rows: map[uint64]*domain.WebhookDelivery{}}
}

func (r *fakeDeliveryRepo) Create(_ context.Context, d *domain.WebhookDelivery) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextID++
	d.ID = r.nextID
	cp := *d
	r.rows[d.ID] = &cp
	return nil
}

func (r *fakeDeliveryRepo) GetByID(_ context.Context, id uint64) (*domain.WebhookDelivery, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	d, ok := r.rows[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *d
	return &cp, nil
}

func (r *fakeDeliveryRepo) ListBySubscription(_ context.Context, subID uint64, _ domain.Pagination) ([]domain.WebhookDelivery, int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []domain.WebhookDelivery
	for _, d := range r.rows {
		if d.SubscriptionID == subID {
			out = append(out, *d)
		}
	}
	return out, len(out), nil
}

func (r *fakeDeliveryRepo) ClaimDue(_ context.Context, now time.Time, limit int) ([]domain.WebhookDelivery, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []domain.WebhookDelivery
	for _, d := range r.rows {
		if d.Status != domain.WebhookDeliveryStatusPending {
			continue
		}
		if d.NextAttemptAt != nil && d.NextAttemptAt.After(now) {
			continue
		}
		out = append(out, *d)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (r *fakeDeliveryRepo) MarkResult(_ context.Context, id uint64, status string, attemptCount uint32, responseStatus *int, errMsg *string, nextAttemptAt, deliveredAt *time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	d, ok := r.rows[id]
	if !ok || d.Status != domain.WebhookDeliveryStatusPending {
		return nil
	}
	d.Status = status
	d.AttemptCount = attemptCount
	d.ResponseStatus = responseStatus
	d.ErrorMessage = errMsg
	d.NextAttemptAt = nextAttemptAt
	d.DeliveredAt = deliveredAt
	return nil
}

func (r *fakeDeliveryRepo) CountFailed(_ context.Context, tenantID *uint64) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, d := range r.rows {
		if d.Status == domain.WebhookDeliveryStatusFailed {
			n++
		}
	}
	return n, nil
}

type fakeRefRepo struct{ byCode map[string]domain.RefLookup }

func (r *fakeRefRepo) GetByCode(_ context.Context, _, code string) (domain.RefLookup, error) {
	v, ok := r.byCode[code]
	if !ok {
		return domain.RefLookup{}, domain.ErrNotFound
	}
	return v, nil
}
func (r *fakeRefRepo) GetByID(_ context.Context, _ string, _ uint64) (domain.RefLookup, error) {
	return domain.RefLookup{}, domain.ErrNotFound
}
func (r *fakeRefRepo) List(_ context.Context, _ string) ([]domain.RefLookup, error) { return nil, nil }

type fakeActivityRepo struct{}

func (fakeActivityRepo) Record(_ context.Context, _ *domain.ActivityLog) error { return nil }
func (fakeActivityRepo) ListByEntity(_ context.Context, _ string, _ uint64, _ domain.Pagination) ([]domain.ActivityLog, int, error) {
	return nil, 0, nil
}

// ListByTenant tidak dipakai test di paket ini — cukup penuhi kontrak
// domain.ActivityLogRepository.
func (fakeActivityRepo) ListByTenant(_ context.Context, _ *uint64, _ domain.ActivityLogFilter, _ domain.Pagination) ([]domain.ActivityLog, int, error) {
	return nil, 0, nil
}

func newTestService(t *testing.T) (*Service, *fakeSubRepo, *fakeDeliveryRepo) {
	t.Helper()
	key := make([]byte, 32)
	enc, err := cryptoutil.NewEncryptor(key)
	if err != nil {
		t.Fatalf("encryptor: %v", err)
	}
	subs := newFakeSubRepo()
	del := newFakeDeliveryRepo()
	refs := &fakeRefRepo{byCode: map[string]domain.RefLookup{
		domain.WebhookEventDeviceFault:          {ID: 1, Code: domain.WebhookEventDeviceFault},
		domain.WebhookEventParameterValueChange: {ID: 2, Code: domain.WebhookEventParameterValueChange},
		domain.WebhookEventTaskFailed:           {ID: 3, Code: domain.WebhookEventTaskFailed},
	}}
	svc := NewService(subs, del, refs, fakeActivityRepo{}, enc, nil)
	// Test memakai URL httptest (loopback) & domain ".test" yang tidak
	// resolvable — lewati SSRF guard di sini. Guard-nya sendiri diuji di
	// pkg/netguard.
	svc.guardURL = func(string) error { return nil }
	return svc, subs, del
}

func ptr[T any](v T) *T { return &v }

// ---- tests ----

func TestCreateSubscription_TenantScope(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()

	admin := domain.Actor{UserID: 5, TenantID: ptr(uint64(1)), Roles: []string{domain.RoleAdmin}}
	superadmin := domain.Actor{UserID: 1, Roles: []string{domain.RoleSuperadmin}}

	// ADMIN tidak boleh membuat subscription global.
	if _, err := svc.CreateSubscription(ctx, admin, CreateSubscriptionInput{
		EventCode: domain.WebhookEventDeviceFault, Name: "g", TargetURL: "https://x.test/h",
	}); err != domain.ErrForbidden {
		t.Fatalf("ADMIN global: want ErrForbidden, got %v", err)
	}

	// ADMIN tidak boleh membuat untuk tenant lain.
	if _, err := svc.CreateSubscription(ctx, admin, CreateSubscriptionInput{
		TenantID: ptr(uint64(2)), EventCode: domain.WebhookEventDeviceFault, Name: "x", TargetURL: "https://x.test/h",
	}); err != domain.ErrForbidden {
		t.Fatalf("ADMIN cross-tenant: want ErrForbidden, got %v", err)
	}

	// ADMIN boleh untuk tenant sendiri.
	sub, err := svc.CreateSubscription(ctx, admin, CreateSubscriptionInput{
		TenantID: ptr(uint64(1)), EventCode: domain.WebhookEventDeviceFault, Name: "own", TargetURL: "https://x.test/h",
	})
	if err != nil {
		t.Fatalf("ADMIN own-tenant: %v", err)
	}
	if sub.Secret == "" || len(sub.Secret) != 64 {
		t.Fatalf("expected 64-hex secret returned once, got %q", sub.Secret)
	}
	if sub.SecretEnc == nil {
		t.Fatalf("expected secret_enc persisted")
	}

	// Superadmin boleh global.
	if _, err := svc.CreateSubscription(ctx, superadmin, CreateSubscriptionInput{
		EventCode: domain.WebhookEventTaskFailed, Name: "g", TargetURL: "http://x.test/h",
	}); err != nil {
		t.Fatalf("superadmin global: %v", err)
	}
}

func TestCreateSubscription_ValidatesInput(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	sa := domain.Actor{UserID: 1, Roles: []string{domain.RoleSuperadmin}}

	for _, tc := range []struct{ name, url, event string }{
		{"bad-scheme", "ftp://x.test/h", domain.WebhookEventDeviceFault},
		{"no-host", "https:///h", domain.WebhookEventDeviceFault},
		{"garbage", "not a url", domain.WebhookEventDeviceFault},
	} {
		if _, err := svc.CreateSubscription(ctx, sa, CreateSubscriptionInput{
			EventCode: tc.event, Name: "n", TargetURL: tc.url,
		}); err == nil {
			t.Fatalf("%s: expected validation error for url %q", tc.name, tc.url)
		}
	}
	if _, err := svc.CreateSubscription(ctx, sa, CreateSubscriptionInput{
		EventCode: "NOPE", Name: "n", TargetURL: "https://x.test/h",
	}); err == nil {
		t.Fatalf("unknown event_type: expected error")
	}
}

func TestEnqueue_FanOutMatchingOnly(t *testing.T) {
	svc, subs, del := newTestService(t)
	ctx := context.Background()
	sa := domain.Actor{UserID: 1, Roles: []string{domain.RoleSuperadmin}}
	t1 := ptr(uint64(1))

	mk := func(in CreateSubscriptionInput) *domain.WebhookSubscription {
		s, err := svc.CreateSubscription(ctx, sa, in)
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		return s
	}
	wantHit := mk(CreateSubscriptionInput{TenantID: t1, EventCode: domain.WebhookEventDeviceFault, Name: "match-tenant", TargetURL: "https://a.test/h"})
	wantGlobal := mk(CreateSubscriptionInput{EventCode: domain.WebhookEventDeviceFault, Name: "match-global", TargetURL: "https://b.test/h"})
	_ = mk(CreateSubscriptionInput{TenantID: ptr(uint64(2)), EventCode: domain.WebhookEventDeviceFault, Name: "other-tenant", TargetURL: "https://c.test/h"})
	_ = mk(CreateSubscriptionInput{TenantID: t1, EventCode: domain.WebhookEventTaskFailed, Name: "other-event", TargetURL: "https://d.test/h"})
	inactive := mk(CreateSubscriptionInput{TenantID: t1, EventCode: domain.WebhookEventDeviceFault, Name: "inactive", TargetURL: "https://e.test/h"})
	_ = svc.UpdateSubscription(ctx, sa, inactive.ID, UpdateSubscriptionInput{Name: "inactive", TargetURL: "https://e.test/h", IsActive: ptr(false)})

	if err := svc.Enqueue(ctx, domain.WebhookEventDeviceFault, t1, map[string]any{"x": 1}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	del.mu.Lock()
	got := map[uint64]bool{}
	for _, d := range del.rows {
		got[d.SubscriptionID] = true
		if d.Status != domain.WebhookDeliveryStatusPending {
			t.Fatalf("delivery %d: status = %s, want PENDING", d.ID, d.Status)
		}
	}
	del.mu.Unlock()

	if len(got) != 2 || !got[wantHit.ID] || !got[wantGlobal.ID] {
		t.Fatalf("fan-out mismatch: got subs %v, want exactly {%d, %d}", got, wantHit.ID, wantGlobal.ID)
	}
	_ = subs
}

func TestDispatch_SuccessSignsPayload(t *testing.T) {
	svc, _, del := newTestService(t)
	ctx := context.Background()
	sa := domain.Actor{UserID: 1, Roles: []string{domain.RoleSuperadmin}}

	var gotSig, gotDeliveryID string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSig = r.Header.Get("X-ACS-Signature")
		gotDeliveryID = r.Header.Get("X-ACS-Delivery-Id")
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	sub, err := svc.CreateSubscription(ctx, sa, CreateSubscriptionInput{
		EventCode: domain.WebhookEventDeviceFault, Name: "n", TargetURL: srv.URL,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := svc.Enqueue(ctx, domain.WebhookEventDeviceFault, nil, map[string]any{"hello": "world"}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	n, err := svc.DispatchDue(ctx)
	if err != nil || n != 1 {
		t.Fatalf("dispatch: n=%d err=%v", n, err)
	}

	del.mu.Lock()
	var d *domain.WebhookDelivery
	for _, row := range del.rows {
		d = row
	}
	del.mu.Unlock()
	if d.Status != domain.WebhookDeliveryStatusDelivered {
		t.Fatalf("status = %s, want DELIVERED", d.Status)
	}
	if d.DeliveredAt == nil || d.ResponseStatus == nil || *d.ResponseStatus != 200 {
		t.Fatalf("delivered_at/response_status not set: %+v", d)
	}
	if gotDeliveryID != d.DeliveryUUID {
		t.Fatalf("X-ACS-Delivery-Id = %q, want %q", gotDeliveryID, d.DeliveryUUID)
	}

	// Verifikasi signature: sha256=hmac(secret, body).
	mac := hmac.New(sha256.New, []byte(sub.Secret))
	mac.Write(gotBody)
	want := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if gotSig != want {
		t.Fatalf("signature mismatch:\n got  %s\n want %s", gotSig, want)
	}
}

// TestDispatch_RetryThenPermanentFail — skenario retry + max_attempts tercapai
// (wajib per CLAUDE.md untuk kode yang menyentuh retry).
func TestDispatch_RetryThenPermanentFail(t *testing.T) {
	svc, _, del := newTestService(t)
	ctx := context.Background()
	sa := domain.Actor{UserID: 1, Roles: []string{domain.RoleSuperadmin}}

	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	if _, err := svc.CreateSubscription(ctx, sa, CreateSubscriptionInput{
		EventCode: domain.WebhookEventTaskFailed, Name: "n", TargetURL: srv.URL,
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := svc.Enqueue(ctx, domain.WebhookEventTaskFailed, nil, map[string]any{"k": "v"}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	// id delivery satu-satunya.
	del.mu.Lock()
	var did uint64
	for id := range del.rows {
		did = id
	}
	del.mu.Unlock()

	// Attempt 1..5: gagal 5xx -> tetap PENDING, next_attempt_at di masa depan,
	// attempt_count naik. Kita "buka gerbang" waktu dgn menyetel next_attempt_at
	// ke masa lalu sebelum tiap tick (mensimulasikan berlalunya backoff).
	for i := 1; i < defaultMaxAttempts; i++ {
		del.mu.Lock()
		del.rows[did].NextAttemptAt = ptr(time.Now().Add(-time.Second))
		del.mu.Unlock()

		if _, err := svc.DispatchDue(ctx); err != nil {
			t.Fatalf("dispatch tick %d: %v", i, err)
		}
		del.mu.Lock()
		d := *del.rows[did]
		del.mu.Unlock()
		if d.AttemptCount != uint32(i) {
			t.Fatalf("tick %d: attempt_count = %d, want %d", i, d.AttemptCount, i)
		}
		if i < defaultMaxAttempts-1 {
			if d.Status != domain.WebhookDeliveryStatusPending {
				t.Fatalf("tick %d: status = %s, want PENDING", i, d.Status)
			}
			if d.NextAttemptAt == nil || !d.NextAttemptAt.After(time.Now()) {
				t.Fatalf("tick %d: next_attempt_at not scheduled forward", i)
			}
		}
	}

	// Attempt terakhir (ke-6) -> FAILED permanen.
	del.mu.Lock()
	del.rows[did].NextAttemptAt = ptr(time.Now().Add(-time.Second))
	del.mu.Unlock()
	if _, err := svc.DispatchDue(ctx); err != nil {
		t.Fatalf("final dispatch: %v", err)
	}
	del.mu.Lock()
	final := *del.rows[did]
	del.mu.Unlock()
	if final.Status != domain.WebhookDeliveryStatusFailed {
		t.Fatalf("final status = %s, want FAILED", final.Status)
	}
	if final.AttemptCount != defaultMaxAttempts {
		t.Fatalf("final attempt_count = %d, want %d", final.AttemptCount, defaultMaxAttempts)
	}
	if final.NextAttemptAt != nil {
		t.Fatalf("FAILED delivery should have next_attempt_at = nil")
	}
	if hits != defaultMaxAttempts {
		t.Fatalf("target hit %d times, want %d", hits, defaultMaxAttempts)
	}

	// Delivery FAILED tidak diproses lagi di tick berikutnya.
	del.mu.Lock()
	del.rows[did].NextAttemptAt = ptr(time.Now().Add(-time.Second))
	del.mu.Unlock()
	if n, _ := svc.DispatchDue(ctx); n != 0 {
		t.Fatalf("FAILED delivery re-dispatched (n=%d)", n)
	}
}

func TestBackoffMonotonicCapped(t *testing.T) {
	var prev time.Duration
	for a := uint32(1); a <= 12; a++ {
		d := backoff(a)
		if d < prev {
			t.Fatalf("backoff not monotonic at attempt %d: %v < %v", a, d, prev)
		}
		if d > maxBackoff {
			t.Fatalf("backoff exceeds cap at attempt %d: %v", a, d)
		}
		prev = d
	}
	if backoff(99) != maxBackoff {
		t.Fatalf("backoff(99) = %v, want cap %v", backoff(99), maxBackoff)
	}
}
