// Package billing -- pembayaran langganan SaaS lewat gateway (Xendit Invoice API), migrations/0032.
//
// Prinsip keamanan ("jangan percaya input"):
//   - NOMINAL tidak pernah berasal dari klien: invoice dibuat dari paket aktif di DB dan nominalnya disalin ke
//     billing_invoices.amount_idr (sumber kebenaran); klien hanya mengirim kode paket (dinormalisasi, lalu dicocokkan
//     dengan whitelist DB);
//   - callback (webhook) TIDAK dipercaya: setelah token X-CALLBACK-TOKEN cocok (waktu-konstan), status dan nominal
//     DIAMBIL ULANG dari gateway (GetInvoice) dan harus cocok dengan baris kita; isi payload hanya dipakai untuk
//     menemukan invoice;
//   - idempoten: kunci event (SHA-256 "<id gateway>|<status>") + transisi status atomik di repository; callback ganda
//     atau paralel hanya memperpanjang langganan SEKALI;
//   - URL pembayaran dari gateway divalidasi (https + host xendit.co) sebelum disimpan/ditampilkan (anti open-redirect);
//   - galat gateway tidak dibocorkan ke klien (502 umum); log tidak memuat rahasia;
//   - isolasi tenant ketat: ADMIN hanya melihat/men-sync invoice tenantnya (tenant lain -> 404).
//
// KEPUTUSAN BISNIS: layanan TIDAK diputus otomatis saat langganan berakhir. Evaluator hanya menandai status
// EXPIRED dan mengirim notifikasi; kebijakan pembatasan/penangguhan adalah keputusan pemilik produk.
package billing

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/mail"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"

	"acs/internal/domain"
	"acs/internal/usecase/notify"
	"acs/pkg/mailer"
)

var (
	// ErrDisabled -- billing dimatikan (ACS_BILLING_ENABLED=false). 409 dengan pesan jelas.
	ErrDisabled = fmt.Errorf("%w: pembayaran langganan belum diaktifkan di server ini", domain.ErrConflict)
	// ErrInvalidToken -- token callback salah. 401.
	ErrInvalidToken = fmt.Errorf("%w: token callback tidak valid", domain.ErrUnauthorized)
	// ErrGateway -- gateway pembayaran gagal; pesan SENGAJA umum (detail hanya di log server). 502.
	ErrGateway = fmt.Errorf("%w: layanan pembayaran sedang tidak tersedia, coba lagi nanti", domain.ErrUpstreamUnavailable)
)

// Batas dan konstanta.
const (
	MaxPendingInvoices = 5                // maks invoice PENDING (belum kedaluwarsa) per tenant
	syncMinInterval    = 10 * time.Second // jarak minimum sinkronisasi per invoice
	expiringWindow     = 7 * 24 * time.Hour
	expiredLookback    = 30 * 24 * time.Hour // evaluator hanya memantau langganan yang berakhir <= 30 hari lalu
	maxWebhookBody     = 64 << 10
	maxAdminContacts   = 20
	sweepBatch         = 500

	MaxPlanPriceIDR   = 1_000_000_000
	MaxPlanDevices    = 10_000_000
	MaxPlanPeriodDays = 366
	maxIdentLen       = 128 // batas id/external_id pada payload webhook
)

var planCodeRe = regexp.MustCompile(`^[A-Z][A-Z0-9_]{1,31}$`)

// TenantReader / UserReader = bagian repository tenant/user yang dipakai (antarmuka sempit).
type TenantReader interface {
	GetByID(ctx context.Context, id uint64) (*domain.Tenant, error)
}
type UserReader interface {
	GetByID(ctx context.Context, id uint64) (*domain.User, error)
}

// Config konfigurasi usecase.
type Config struct {
	Enabled       bool
	CallbackToken string        // rahasia: tidak pernah dicatat/dikembalikan
	Invoice       time.Duration // masa berlaku invoice
	PublicURL     string        // origin Console untuk URL redirect sukses/gagal
	// AllowedPaymentHosts = akhiran host tambahan yang dipercaya untuk payment_url (selain xendit.co), mis. host base
	// URL non-produksi. Tetap wajib https.
	AllowedPaymentHosts []string
}

type Service struct {
	repo     domain.BillingRepository
	tenants  TenantReader
	users    UserReader
	gw       domain.PaymentGateway
	activity domain.ActivityLogRepository
	cfg      Config
	log      *slog.Logger
	now      func() time.Time

	alerts  domain.Notifier // opsional: notifikasi in-app (+ email via kanal notifikasi)
	mail    mailer.Notifier // opsional: email tanda terima
	builder *notify.Builder

	tokenHash [32]byte // SHA-256 token callback (dibandingkan waktu-konstan)

	syncMu sync.Mutex
	syncAt map[string]time.Time
}

func NewService(repo domain.BillingRepository, tenants TenantReader, users UserReader, gw domain.PaymentGateway, activity domain.ActivityLogRepository, cfg Config, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	if cfg.Invoice <= 0 {
		cfg.Invoice = 24 * time.Hour
	}
	cfg.PublicURL = strings.TrimRight(cfg.PublicURL, "/")
	s := &Service{repo: repo, tenants: tenants, users: users, gw: gw, activity: activity, cfg: cfg, log: log, now: time.Now, syncAt: map[string]time.Time{}}
	// Token kosong tidak boleh pernah cocok dengan header kosong: tanpa token, billing dianggap mati.
	if cfg.CallbackToken == "" {
		s.cfg.Enabled = false
	}
	s.tokenHash = sha256.Sum256([]byte(cfg.CallbackToken))
	s.cfg.CallbackToken = "" // jangan simpan teks asli; cukup hash
	return s
}

// WithNotifier memasang notifikasi persisten (nil-safe).
func (s *Service) WithNotifier(n domain.Notifier) *Service { s.alerts = n; return s }

// WithEmail mengaktifkan email tanda terima (nil/SMTP mati = lewati).
func (s *Service) WithEmail(m mailer.Notifier, b *notify.Builder) *Service {
	s.mail, s.builder = m, b
	return s
}

// Enabled -- billing aktif dan gateway tersedia.
func (s *Service) Enabled() bool { return s.cfg.Enabled && s.gw != nil }

// ---- peran & validasi ----

func isStaff(a domain.Actor) bool {
	return a.IsSuperadmin() || a.HasRole(domain.RoleAdmin) || a.HasRole(domain.RoleNOC) || a.HasRole(domain.RoleViewer)
}

// canManage = boleh membuat/men-sync invoice: ADMIN tenant atau SUPERADMIN.
func canManage(a domain.Actor) bool { return a.IsSuperadmin() || a.HasRole(domain.RoleAdmin) }

// resolveTenant menentukan tenant sasaran. SUPERADMIN wajib menyebut tenant_id; selain itu hanya tenant sendiri
// (tenant_id yang berbeda -> 403).
func resolveTenant(a domain.Actor, requested *uint64) (uint64, error) {
	if a.IsSuperadmin() {
		if requested != nil {
			return *requested, nil
		}
		if a.TenantID != nil {
			return *a.TenantID, nil
		}
		return 0, fmt.Errorf("%w: tenant_id wajib diisi untuk SUPERADMIN", domain.ErrInvalidInput)
	}
	if a.TenantID == nil {
		return 0, domain.ErrForbidden
	}
	if requested != nil && *requested != *a.TenantID {
		return 0, domain.ErrForbidden
	}
	return *a.TenantID, nil
}

func hasControl(s string) bool {
	for _, r := range s {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}

// cleanText memangkas spasi tepi lalu memvalidasi panjang (rune) dan karakter kontrol.
func cleanText(field, v string, min, max int) (string, error) {
	v = strings.TrimSpace(v)
	n := utf8.RuneCountInString(v)
	if n < min || n > max || hasControl(v) || !utf8.ValidString(v) {
		if min == 0 {
			return "", fmt.Errorf("%w: %s maksimal %d karakter, tanpa karakter kontrol", domain.ErrInvalidInput, field, max)
		}
		return "", fmt.Errorf("%w: %s harus %d-%d karakter, tanpa karakter kontrol", domain.ErrInvalidInput, field, min, max)
	}
	return v, nil
}

// NormalizePlanCode: trim + huruf besar + pola ^[A-Z][A-Z0-9_]{1,31}$.
func NormalizePlanCode(code string) (string, error) {
	c := strings.ToUpper(strings.TrimSpace(code))
	if !planCodeRe.MatchString(c) {
		return "", fmt.Errorf("%w: kode paket harus 2-32 karakter (huruf besar, angka, garis bawah; diawali huruf)", domain.ErrInvalidInput)
	}
	return c, nil
}

// ---- paket ----

// ListPlans: SUPERADMIN semua paket; staf lain hanya yang aktif. ENDUSER ditolak.
func (s *Service) ListPlans(ctx context.Context, actor domain.Actor) ([]domain.BillingPlan, error) {
	if !isStaff(actor) {
		return nil, domain.ErrForbidden
	}
	return s.repo.ListPlans(ctx, !actor.IsSuperadmin())
}

// CreatePlanInput -- semua angka bertipe int64 agar nilai negatif/di luar rentang bisa ditolak dengan pesan jelas.
type CreatePlanInput struct {
	Code        string
	Name        string
	Description string
	MaxDevices  *int64
	PriceIDR    int64
	PeriodDays  int64
	IsActive    bool
	SortOrder   int64
}

func validateMaxDevices(v *int64) (*uint32, error) {
	if v == nil {
		return nil, nil
	}
	if *v < 1 || *v > MaxPlanDevices {
		return nil, fmt.Errorf("%w: max_devices harus kosong (tak dibatasi) atau 1-%d", domain.ErrInvalidInput, MaxPlanDevices)
	}
	n := uint32(*v)
	return &n, nil
}

func validatePrice(v int64) (uint64, error) {
	if v < 0 || v > MaxPlanPriceIDR {
		return 0, fmt.Errorf("%w: price_idr harus bilangan bulat 0-%d", domain.ErrInvalidInput, MaxPlanPriceIDR)
	}
	return uint64(v), nil
}

func validatePeriod(v int64) (uint16, error) {
	if v < 1 || v > MaxPlanPeriodDays {
		return 0, fmt.Errorf("%w: period_days harus 1-%d", domain.ErrInvalidInput, MaxPlanPeriodDays)
	}
	return uint16(v), nil
}

func validateSort(v int64) (int, error) {
	if v < -100000 || v > 100000 {
		return 0, fmt.Errorf("%w: sort_order harus -100000..100000", domain.ErrInvalidInput)
	}
	return int(v), nil
}

// CreatePlan (SUPERADMIN) -- validasi ketat; kode ganda -> 409.
func (s *Service) CreatePlan(ctx context.Context, actor domain.Actor, in CreatePlanInput) (*domain.BillingPlan, error) {
	if !actor.IsSuperadmin() {
		return nil, domain.ErrForbidden
	}
	var p domain.BillingPlan
	var err error
	if p.Code, err = NormalizePlanCode(in.Code); err != nil {
		return nil, err
	}
	if p.Name, err = cleanText("name", in.Name, 2, 64); err != nil {
		return nil, err
	}
	if p.Description, err = cleanText("description", in.Description, 0, 255); err != nil {
		return nil, err
	}
	if p.MaxDevices, err = validateMaxDevices(in.MaxDevices); err != nil {
		return nil, err
	}
	if p.PriceIDR, err = validatePrice(in.PriceIDR); err != nil {
		return nil, err
	}
	if p.PeriodDays, err = validatePeriod(in.PeriodDays); err != nil {
		return nil, err
	}
	if p.SortOrder, err = validateSort(in.SortOrder); err != nil {
		return nil, err
	}
	p.IsActive = in.IsActive
	if err := s.repo.CreatePlan(ctx, &p, actor.UserIDPtr()); err != nil {
		if errors.Is(err, domain.ErrConflict) {
			return nil, fmt.Errorf("%w: kode paket %s sudah ada", domain.ErrConflict, p.Code)
		}
		return nil, err
	}
	s.auditPlan(ctx, actor, "BILLING_PLAN_CREATED", &p)
	return s.repo.GetPlanByID(ctx, p.ID)
}

// OptionalInt64 membedakan field "tidak dikirim" (Set=false) dari "null" (Set=true, Value=nil).
type OptionalInt64 struct {
	Set   bool
	Value *int64
}

// UpdatePlanInput -- semua field opsional (PATCH). Kode paket tidak dapat diubah.
type UpdatePlanInput struct {
	Name        *string
	Description *string
	MaxDevices  OptionalInt64
	PriceIDR    *int64
	PeriodDays  *int64
	IsActive    *bool
	SortOrder   *int64
}

// UpdatePlan (SUPERADMIN). Perubahan HANYA berlaku untuk invoice BARU: nominal/periode invoice yang sudah dibuat
// tersalin di billing_invoices dan tidak berubah; tenant yang sudah berlangganan tidak diubah batasnya.
func (s *Service) UpdatePlan(ctx context.Context, actor domain.Actor, code string, in UpdatePlanInput) (*domain.BillingPlan, error) {
	if !actor.IsSuperadmin() {
		return nil, domain.ErrForbidden
	}
	code, err := NormalizePlanCode(code)
	if err != nil {
		return nil, err
	}
	p, err := s.repo.GetPlanByCode(ctx, code)
	if err != nil {
		return nil, err
	}
	if in.Name != nil {
		if p.Name, err = cleanText("name", *in.Name, 2, 64); err != nil {
			return nil, err
		}
	}
	if in.Description != nil {
		if p.Description, err = cleanText("description", *in.Description, 0, 255); err != nil {
			return nil, err
		}
	}
	if in.MaxDevices.Set {
		if p.MaxDevices, err = validateMaxDevices(in.MaxDevices.Value); err != nil {
			return nil, err
		}
	}
	if in.PriceIDR != nil {
		if p.PriceIDR, err = validatePrice(*in.PriceIDR); err != nil {
			return nil, err
		}
	}
	if in.PeriodDays != nil {
		if p.PeriodDays, err = validatePeriod(*in.PeriodDays); err != nil {
			return nil, err
		}
	}
	if in.IsActive != nil {
		p.IsActive = *in.IsActive
	}
	if in.SortOrder != nil {
		if p.SortOrder, err = validateSort(*in.SortOrder); err != nil {
			return nil, err
		}
	}
	if err := s.repo.UpdatePlan(ctx, p, actor.UserIDPtr()); err != nil {
		return nil, err
	}
	s.auditPlan(ctx, actor, "BILLING_PLAN_UPDATED", p)
	return s.repo.GetPlanByID(ctx, p.ID)
}

// DeletePlan (SUPERADMIN) menghapus paket dari katalog. Ditolak (409) selama masih dipakai tenant atau punya invoice
// PENDING — nonaktifkan saja (is_active=false) bila ingin menutup penjualan tanpa menghapus. Riwayat invoice utuh.
func (s *Service) DeletePlan(ctx context.Context, actor domain.Actor, code string) error {
	if !actor.IsSuperadmin() {
		return domain.ErrForbidden
	}
	code, err := NormalizePlanCode(code)
	if err != nil {
		return err
	}
	p, err := s.repo.GetPlanByCode(ctx, code)
	if err != nil {
		return err
	}
	uid := actor.UserIDPtr()
	if uid == nil {
		return domain.ErrForbidden
	}
	if err := s.repo.SoftDeletePlan(ctx, p.ID, *uid); err != nil {
		if errors.Is(err, domain.ErrConflict) {
			return fmt.Errorf("%w: paket masih dipakai tenant atau punya invoice menunggu pembayaran; nonaktifkan saja", domain.ErrConflict)
		}
		return err
	}
	s.auditPlan(ctx, actor, "BILLING_PLAN_DELETED", p)
	return nil
}

// ---- langganan ----

// SubscriptionView = status langganan tenant.
type SubscriptionView struct {
	Enabled        bool                   `json:"enabled"`
	Status         string                 `json:"status"` // NONE | ACTIVE | EXPIRED
	Plan           *domain.BillingPlan    `json:"plan"`
	ExpiresAt      *time.Time             `json:"expires_at"`
	DaysLeft       *int                   `json:"days_left"`
	MaxDevices     *uint32                `json:"max_devices"`
	PendingInvoice *domain.BillingInvoice `json:"pending_invoice"`
}

// Subscription -- status langganan tenant actor (SUPERADMIN wajib menyebut tenant_id). Semua staf boleh membaca;
// pending_invoice (memuat URL bayar) hanya untuk ADMIN/SUPERADMIN.
func (s *Service) Subscription(ctx context.Context, actor domain.Actor, tenantID *uint64) (*SubscriptionView, error) {
	if !isStaff(actor) {
		return nil, domain.ErrForbidden
	}
	tid, err := resolveTenant(actor, tenantID)
	if err != nil {
		return nil, err
	}
	t, err := s.tenants.GetByID(ctx, tid)
	if err != nil {
		return nil, err
	}
	now := s.now()
	v := &SubscriptionView{Enabled: s.Enabled(), Status: domain.SubscriptionNone, MaxDevices: t.MaxDevices}
	if t.BillingPlanID != nil {
		if p, err := s.repo.GetPlanByID(ctx, *t.BillingPlanID); err == nil {
			v.Plan = p
		} else if !errors.Is(err, domain.ErrNotFound) {
			return nil, err
		}
	}
	if t.PlanExpiresAt != nil {
		exp := *t.PlanExpiresAt
		v.ExpiresAt = &exp
		days := 0
		if exp.After(now) {
			v.Status = domain.SubscriptionActive
			days = int((exp.Sub(now) + 24*time.Hour - 1) / (24 * time.Hour)) // pembulatan ke atas
		} else {
			v.Status = domain.SubscriptionExpired
		}
		v.DaysLeft = &days
	}
	if canManage(actor) {
		pend, err := s.repo.ListPendingByTenant(ctx, tid, now)
		if err != nil {
			return nil, err
		}
		for i := range pend {
			if pend[i].PaymentURL != nil { // invoice yang sudah punya URL bayar
				inv := s.scrub(pend[i], now)
				v.PendingInvoice = &inv
				break
			}
		}
	}
	return v, nil
}

// scrub: payment_url hanya untuk invoice PENDING yang belum kedaluwarsa.
func (s *Service) scrub(inv domain.BillingInvoice, now time.Time) domain.BillingInvoice {
	if inv.Status != domain.InvoicePending || !inv.ExpiresAt.After(now) {
		inv.PaymentURL = nil
	}
	return inv
}

// ---- invoice ----

// ListInvoices: ADMIN tenant sendiri; SUPERADMIN semua tenant (tenantID nil) atau satu tenant. NOC/VIEWER/ENDUSER ditolak.
func (s *Service) ListInvoices(ctx context.Context, actor domain.Actor, tenantID *uint64, p domain.Pagination) ([]domain.BillingInvoice, int, error) {
	if !canManage(actor) {
		return nil, 0, domain.ErrForbidden
	}
	var filter *uint64
	if actor.IsSuperadmin() {
		filter = tenantID
	} else {
		tid, err := resolveTenant(actor, tenantID)
		if err != nil {
			return nil, 0, err
		}
		filter = &tid
	}
	rows, total, err := s.repo.ListInvoices(ctx, filter, p)
	if err != nil {
		return nil, 0, err
	}
	now := s.now()
	for i := range rows {
		rows[i] = s.scrub(rows[i], now)
	}
	return rows, total, nil
}

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// validPaymentURL: https, tanpa userinfo, port kosong/443, host = xendit.co atau subdomainnya (atau akhiran tambahan
// yang dikonfigurasi). Mencegah open-redirect bila respons gateway dimanipulasi.
func (s *Service) validPaymentURL(raw string) bool {
	if raw == "" || len(raw) > 512 || hasControl(raw) || strings.ContainsAny(raw, " \\") {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Host == "" {
		return false
	}
	if p := u.Port(); p != "" && p != "443" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	suffixes := append([]string{"xendit.co"}, s.cfg.AllowedPaymentHosts...)
	for _, suf := range suffixes {
		suf = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(suf), "."))
		if suf != "" && (host == suf || strings.HasSuffix(host, "."+suf)) {
			return true
		}
	}
	return false
}

// payerEmail: email actor (bila valid), cadangan: ADMIN pertama tenant.
func (s *Service) payerEmail(ctx context.Context, actor domain.Actor, tenantID uint64) (string, error) {
	if actor.UserID != 0 && s.users != nil {
		if u, err := s.users.GetByID(ctx, actor.UserID); err == nil {
			if a, perr := mail.ParseAddress(strings.TrimSpace(u.Email)); perr == nil && len(a.Address) <= 191 {
				return a.Address, nil
			}
		}
	}
	if cs, err := s.repo.ListTenantAdminContacts(ctx, tenantID, 1); err == nil && len(cs) > 0 {
		if a, perr := mail.ParseAddress(cs[0].Email); perr == nil {
			return a.Address, nil
		}
	}
	return "", fmt.Errorf("%w: tidak ada email penagihan yang valid pada akun", domain.ErrInvalidInput)
}

// Checkout membuat invoice langganan. NOMINAL HANYA dari DB (paket aktif), tidak pernah dari klien.
// Idempoten: invoice PENDING yang masih berlaku untuk paket yang sama dikembalikan, bukan dibuat ganda.
// Catatan: tidak ada kunci lintas-permintaan; dua checkout paralel pertama kali dapat membuat dua invoice (aman,
// hanya lebih banyak PENDING; batas MaxPendingInvoices tetap ditegakkan pada percobaan berikutnya).
func (s *Service) Checkout(ctx context.Context, actor domain.Actor, planCode string, tenantID *uint64) (*domain.BillingInvoice, error) {
	if !s.Enabled() {
		return nil, ErrDisabled
	}
	if !canManage(actor) {
		return nil, domain.ErrForbidden
	}
	tid, err := resolveTenant(actor, tenantID)
	if err != nil {
		return nil, err
	}
	code, err := NormalizePlanCode(planCode)
	if err != nil {
		return nil, fmt.Errorf("%w: paket tidak tersedia", domain.ErrInvalidInput)
	}
	t, err := s.tenants.GetByID(ctx, tid)
	if err != nil {
		return nil, err
	}
	if !t.IsActive {
		return nil, fmt.Errorf("%w: tenant tidak aktif", domain.ErrConflict)
	}
	plan, err := s.repo.GetPlanByCode(ctx, code)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, fmt.Errorf("%w: paket tidak tersedia", domain.ErrInvalidInput)
		}
		return nil, err
	}
	if !plan.IsActive {
		return nil, fmt.Errorf("%w: paket tidak tersedia", domain.ErrInvalidInput)
	}
	if plan.PriceIDR == 0 {
		return nil, fmt.Errorf("%w: paket ini tidak memerlukan pembayaran", domain.ErrInvalidInput)
	}

	now := s.now()
	pend, err := s.repo.ListPendingByTenant(ctx, tid, now)
	if err != nil {
		return nil, err
	}
	for _, p := range pend {
		if p.PlanID == plan.ID && p.PaymentURL != nil && p.XenditInvoiceID != nil {
			inv := s.scrub(p, now)
			return &inv, nil
		}
	}
	if len(pend) >= MaxPendingInvoices {
		return nil, fmt.Errorf("%w: terlalu banyak invoice menunggu pembayaran (maks %d); bayar atau tunggu hingga kedaluwarsa", domain.ErrQuotaExceeded, MaxPendingInvoices)
	}
	email, err := s.payerEmail(ctx, actor, tid)
	if err != nil {
		return nil, err
	}

	inv := &domain.BillingInvoice{
		InvoiceUUID:   uuid.NewString(),
		InvoiceNumber: fmt.Sprintf("ACS-%d-%d-%s", tid, now.UnixNano(), randHex(4)),
		TenantID:      tid, PlanID: plan.ID, PlanCode: plan.Code, PlanName: plan.Name,
		AmountIDR: plan.PriceIDR, PeriodDays: plan.PeriodDays,
		ExpiresAt: now.Add(s.cfg.Invoice), CreatedAt: now, CreatedBy: actor.UserIDPtr(),
	}
	if err := s.repo.CreateInvoice(ctx, inv); err != nil {
		return nil, err
	}

	g, gerr := s.gw.CreateInvoice(ctx, domain.GatewayInvoiceInput{
		ExternalID: inv.InvoiceNumber, AmountIDR: inv.AmountIDR, PayerEmail: email,
		Description:        fmt.Sprintf("Langganan %s (%d hari)", plan.Name, plan.PeriodDays),
		DurationSeconds:    int(s.cfg.Invoice.Seconds()),
		SuccessRedirectURL: s.cfg.PublicURL + "/billing?status=success",
		FailureRedirectURL: s.cfg.PublicURL + "/billing?status=failed",
	})
	if gerr == nil {
		gerr = s.checkCreated(inv, g)
	}
	if gerr != nil {
		// Detail hanya di log server (galat gateway sudah disanitasi klien); klien menerima galat umum.
		s.log.Warn("billing: pembuatan invoice di gateway gagal", "invoice", inv.InvoiceNumber, "tenant_id", tid, "error", gerr.Error())
		if _, merr := s.repo.MarkInvoiceFailed(ctx, inv.ID); merr != nil {
			s.log.Error("billing: gagal menandai invoice FAILED", "invoice", inv.InvoiceNumber, "error", merr.Error())
		}
		s.audit(ctx, &actor, tid, "BILLING_CHECKOUT_FAILED", inv, nil)
		return nil, ErrGateway
	}

	expires := inv.ExpiresAt
	if g.ExpiryDate != nil {
		expires = *g.ExpiryDate
	}
	if err := s.repo.SetInvoiceGateway(ctx, inv.ID, g.ID, g.InvoiceURL, expires); err != nil {
		_, _ = s.repo.MarkInvoiceFailed(ctx, inv.ID) // invoice di gateway menjadi yatim (kedaluwarsa sendiri); jangan biarkan PENDING tanpa URL
		return nil, err
	}
	s.audit(ctx, &actor, tid, "BILLING_CHECKOUT", inv, nil)
	out, err := s.repo.GetInvoiceByUUID(ctx, inv.InvoiceUUID)
	if err != nil {
		return nil, err
	}
	res := s.scrub(*out, now)
	return &res, nil
}

// checkCreated memvalidasi respons pembuatan invoice: id, external_id, nominal, dan URL bayar.
func (s *Service) checkCreated(inv *domain.BillingInvoice, g *domain.GatewayInvoice) error {
	switch {
	case g == nil || g.ID == "" || len(g.ID) > 64:
		return errors.New("respons gateway tanpa id invoice")
	case g.ExternalID != "" && g.ExternalID != inv.InvoiceNumber:
		return errors.New("external_id respons gateway tidak cocok")
	case g.AmountIDR != inv.AmountIDR:
		return errors.New("nominal respons gateway tidak cocok dengan nominal paket")
	case !s.validPaymentURL(g.InvoiceURL):
		return errors.New("URL pembayaran dari gateway ditolak (bukan https/host tepercaya)")
	}
	return nil
}

// ---- sinkronisasi & webhook ----

// Sync mengambil status terbaru dari gateway dan menerapkannya (jalur yang sama dengan webhook). Untuk tombol
// "Periksa status" bila webhook terlambat. Maks 1 panggilan gateway per invoice per 10 detik.
func (s *Service) Sync(ctx context.Context, actor domain.Actor, invoiceUUID string) (*domain.BillingInvoice, error) {
	if !s.Enabled() {
		return nil, ErrDisabled
	}
	if !canManage(actor) {
		return nil, domain.ErrForbidden
	}
	id, err := parseUUID(invoiceUUID)
	if err != nil {
		return nil, err
	}
	inv, err := s.repo.GetInvoiceByUUID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !actor.IsSuperadmin() && (actor.TenantID == nil || *actor.TenantID != inv.TenantID) {
		return nil, domain.ErrNotFound // jangan bocorkan keberadaan invoice tenant lain
	}
	// Hanya PENDING (dan EXPIRED: pembayaran bisa masuk tepat di batas waktu) yang punya id gateway layak ditanyakan.
	if (inv.Status == domain.InvoicePending || inv.Status == domain.InvoiceExpired) && inv.XenditInvoiceID != nil {
		if !s.allowSync(inv.InvoiceUUID) {
			return nil, fmt.Errorf("%w: tunggu %d detik sebelum memeriksa status lagi", domain.ErrQuotaExceeded, int(syncMinInterval.Seconds()))
		}
		g, gerr := s.gw.GetInvoice(ctx, *inv.XenditInvoiceID)
		if gerr != nil {
			s.log.Warn("billing: sinkronisasi ke gateway gagal", "invoice", inv.InvoiceNumber, "error", gerr.Error())
			return nil, ErrGateway
		}
		if _, err := s.applyGateway(ctx, inv, g, &actor); err != nil {
			return nil, err
		}
	}
	fresh, err := s.repo.GetInvoiceByUUID(ctx, id)
	if err != nil {
		return nil, err
	}
	out := s.scrub(*fresh, s.now())
	return &out, nil
}

func (s *Service) allowSync(key string) bool {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	now := s.now()
	if len(s.syncAt) > 2000 { // pangkas entri lama agar peta tidak tumbuh tanpa batas
		for k, t := range s.syncAt {
			if now.Sub(t) > syncMinInterval {
				delete(s.syncAt, k)
			}
		}
	}
	if t, ok := s.syncAt[key]; ok && now.Sub(t) < syncMinInterval {
		return false
	}
	s.syncAt[key] = now
	return true
}

func parseUUID(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	id, err := uuid.Parse(raw)
	if err != nil || len(raw) != 36 {
		return "", fmt.Errorf("%w: id invoice tidak valid", domain.ErrInvalidInput)
	}
	return id.String(), nil
}

// Hasil pemrosesan webhook/sinkronisasi (untuk log dan tes).
const (
	OutcomePaid         = "paid"
	OutcomeDuplicate    = "duplicate"
	OutcomeExpired      = "expired"
	OutcomePending      = "pending"
	OutcomeIgnored      = "ignored"
	OutcomeMismatch     = "mismatch"
	OutcomeUnderpaid    = "underpaid"
	OutcomeNoop         = "noop"
	OutcomeTenantMissed = "tenant_missing"
	OutcomeUnknown      = "unknown_status"
)

type webhookPayload struct {
	ID         string `json:"id"`
	ExternalID string `json:"external_id"`
	// status/amount pada payload SENGAJA tidak dibaca: kebenaran diambil ulang dari gateway.
}

// HandleWebhook memproses callback Xendit. Mengembalikan outcome; galat hanya untuk token salah (ErrInvalidToken),
// payload rusak (ErrInvalidInput), dan galat sistem (pemanggil membalas 5xx agar Xendit mengulang).
func (s *Service) HandleWebhook(ctx context.Context, token string, body []byte) (string, error) {
	if !s.Enabled() {
		return "", domain.ErrNotFound
	}
	// 1. Token: bandingkan hash (panjang sama) dengan ConstantTimeCompare -- tidak bocor lewat waktu/panjang.
	got := sha256.Sum256([]byte(token))
	if subtle.ConstantTimeCompare(got[:], s.tokenHash[:]) != 1 {
		return "", ErrInvalidToken
	}
	// 2. Body & JSON ketat.
	if len(body) > maxWebhookBody {
		return "", fmt.Errorf("%w: payload terlalu besar", domain.ErrInvalidInput)
	}
	var p webhookPayload
	dec := json.NewDecoder(strings.NewReader(string(body)))
	if err := dec.Decode(&p); err != nil {
		return "", fmt.Errorf("%w: payload bukan JSON objek yang valid", domain.ErrInvalidInput)
	}
	if _, err := dec.Token(); err != io.EOF {
		return "", fmt.Errorf("%w: payload memuat data tambahan setelah JSON", domain.ErrInvalidInput)
	}
	for _, v := range []string{p.ID, p.ExternalID} {
		if len(v) > maxIdentLen || hasControl(v) || !utf8.ValidString(v) {
			return "", fmt.Errorf("%w: id/external_id tidak valid", domain.ErrInvalidInput)
		}
	}
	if p.ID == "" && p.ExternalID == "" {
		return "", fmt.Errorf("%w: id atau external_id wajib ada", domain.ErrInvalidInput)
	}
	// 3. Cari invoice kita. Tidak dikenal -> abaikan (200 tanpa efek, tanpa bocorkan keberadaan).
	var inv *domain.BillingInvoice
	var err error
	if p.ExternalID != "" {
		inv, err = s.repo.GetInvoiceByNumber(ctx, p.ExternalID)
		if err != nil && !errors.Is(err, domain.ErrNotFound) {
			return "", err
		}
	}
	if inv == nil && p.ID != "" {
		inv, err = s.repo.GetInvoiceByXenditID(ctx, p.ID)
		if err != nil && !errors.Is(err, domain.ErrNotFound) {
			return "", err
		}
	}
	if inv == nil || inv.XenditInvoiceID == nil {
		return OutcomeIgnored, nil
	}
	if inv.Status == domain.InvoicePaid {
		return OutcomeDuplicate, nil // sudah lunas: tidak perlu menanyakan gateway lagi
	}
	// 4. Jangan percaya payload: tanyakan gateway memakai id yang TERSIMPAN di baris kita.
	g, gerr := s.gw.GetInvoice(ctx, *inv.XenditInvoiceID)
	if gerr != nil {
		s.log.Warn("billing: verifikasi callback ke gateway gagal", "invoice", inv.InvoiceNumber, "error", gerr.Error())
		return "", ErrGateway // 502 -> Xendit mengulang callback
	}
	return s.applyGateway(ctx, inv, g, nil)
}

func eventKey(gatewayID, status string) string {
	sum := sha256.Sum256([]byte(gatewayID + "|" + status))
	return hex.EncodeToString(sum[:])
}

// applyGateway menerapkan status KEBENARAN dari gateway ke invoice kita (dipakai webhook dan Sync). actor nil = sistem.
func (s *Service) applyGateway(ctx context.Context, inv *domain.BillingInvoice, g *domain.GatewayInvoice, actor *domain.Actor) (string, error) {
	if g == nil || inv.XenditInvoiceID == nil || g.ID != *inv.XenditInvoiceID || g.ExternalID != inv.InvoiceNumber || g.AmountIDR != inv.AmountIDR {
		s.log.Warn("billing: data gateway tidak cocok dengan invoice kita; diabaikan", "invoice", inv.InvoiceNumber)
		s.audit(ctx, actor, inv.TenantID, "BILLING_MISMATCH", inv, nil)
		return OutcomeMismatch, nil
	}
	now := s.now()
	status := strings.ToUpper(strings.TrimSpace(g.Status))
	switch status {
	case domain.GatewayStatusPaid, domain.GatewayStatusSettled:
		paid := g.PaidAmountIDR
		if paid == 0 {
			paid = g.AmountIDR // ASUMSI: paid_amount absen pada invoice lunas -> sama dengan amount
		}
		if paid < inv.AmountIDR {
			s.log.Warn("billing: nominal terbayar kurang dari tagihan; tidak dilunasi", "invoice", inv.InvoiceNumber)
			s.audit(ctx, actor, inv.TenantID, "BILLING_UNDERPAID", inv, map[string]any{"paid_amount_idr": paid})
			return OutcomeUnderpaid, nil
		}
		paidAt := now
		if g.PaidAt != nil && !g.PaidAt.After(now) {
			paidAt = *g.PaidAt
		}
		res, err := s.repo.ApplyPayment(ctx, domain.BillingPaymentInput{
			InvoiceID: inv.ID, EventKey: eventKey(g.ID, status), PaidAt: paidAt, PaidAmountIDR: paid, Now: now,
		})
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				s.log.Error("billing: pembayaran diterima tetapi tenant sudah tidak ada; perlu penanganan manual", "invoice", inv.InvoiceNumber)
				s.audit(ctx, actor, inv.TenantID, "BILLING_TENANT_MISSING", inv, nil)
				return OutcomeTenantMissed, nil
			}
			return "", err
		}
		if res.Duplicate {
			return OutcomeDuplicate, nil
		}
		if !res.Applied {
			return OutcomeNoop, nil
		}
		s.afterPaid(ctx, actor, inv, paid, paidAt, res.PlanExpiresAt)
		return OutcomePaid, nil
	case domain.GatewayStatusExpired:
		changed, err := s.repo.MarkInvoiceExpired(ctx, inv.ID)
		if err != nil {
			return "", err
		}
		if !changed {
			return OutcomeNoop, nil
		}
		_, _ = s.repo.RecordWebhookEvent(ctx, eventKey(g.ID, status), &inv.ID, domain.InvoiceExpired)
		s.afterExpired(ctx, actor, inv)
		return OutcomeExpired, nil
	case domain.GatewayStatusPending:
		return OutcomePending, nil
	}
	s.log.Warn("billing: status gateway tidak dikenal; diabaikan", "invoice", inv.InvoiceNumber)
	return OutcomeUnknown, nil
}

// afterPaid = efek samping SETELAH transaksi lunas: audit, notifikasi, email tanda terima. Kegagalan di sini tidak
// membatalkan pembayaran yang sudah tercatat.
func (s *Service) afterPaid(ctx context.Context, actor *domain.Actor, inv *domain.BillingInvoice, paid uint64, paidAt, activeUntil time.Time) {
	s.audit(ctx, actor, inv.TenantID, "BILLING_PAID", inv, map[string]any{"paid_amount_idr": paid})
	tid, iid := inv.TenantID, inv.ID
	if s.alerts != nil {
		_ = s.alerts.Notify(ctx, domain.NotificationEvent{
			TypeCode: domain.NotifBillingPaid, TenantID: &tid,
			Title:      "Pembayaran langganan berhasil",
			Body:       fmt.Sprintf("Pembayaran invoice %s (paket %s, %s) diterima. Langganan aktif hingga %s.", inv.InvoiceNumber, inv.PlanName, notify.FormatRupiah(paid), activeUntil.Format("02-01-2006")),
			Link:       "/billing",
			EntityType: "billing_invoice", EntityID: &iid,
			DedupeKey: "paid:" + inv.InvoiceUUID, DedupeWindow: 365 * 24 * time.Hour,
			NoEmail: true, // email tanda terima dikirim terpisah di bawah
		})
	}
	if s.mail == nil || s.builder == nil {
		return
	}
	tenantName := ""
	if t, err := s.tenants.GetByID(ctx, inv.TenantID); err == nil {
		tenantName = t.Name
	}
	contacts, err := s.repo.ListTenantAdminContacts(ctx, inv.TenantID, maxAdminContacts)
	if err != nil {
		s.log.Warn("billing: gagal mengambil penerima tanda terima", "invoice", inv.InvoiceNumber, "error", err.Error())
		return
	}
	for _, c := range contacts {
		s.mail.Notify(s.builder.PaymentReceipt(c.Email, c.FullName, tenantName, inv.PlanName, inv.InvoiceNumber, paid, paidAt, activeUntil))
	}
}

func (s *Service) afterExpired(ctx context.Context, actor *domain.Actor, inv *domain.BillingInvoice) {
	s.audit(ctx, actor, inv.TenantID, "BILLING_INVOICE_EXPIRED", inv, nil)
	if s.alerts == nil {
		return
	}
	tid, iid := inv.TenantID, inv.ID
	_ = s.alerts.Notify(ctx, domain.NotificationEvent{
		TypeCode: domain.NotifBillingFailed, TenantID: &tid,
		Title:      "Invoice langganan kedaluwarsa",
		Body:       fmt.Sprintf("Invoice %s (paket %s) kedaluwarsa tanpa pembayaran. Buat invoice baru bila Anda masih ingin berlangganan.", inv.InvoiceNumber, inv.PlanName),
		Link:       "/billing",
		EntityType: "billing_invoice", EntityID: &iid,
		DedupeKey: "invoice:" + inv.InvoiceUUID, DedupeWindow: 365 * 24 * time.Hour,
	})
}

// ---- evaluator periodik ----

// Evaluate dipanggil sweeper (tiap jam, di bawah kunci leader). Tugas:
//  1. invoice PENDING yang lewat expires_at -> EXPIRED (+ notifikasi BILLING_FAILED);
//  2. langganan yang berakhir <= 7 hari -> BILLING_EXPIRING; yang sudah lewat -> BILLING_EXPIRED
//     (dedupe: 1 per tenant per periode, kunci memuat tanggal berakhir).
//
// TIDAK ADA pemutusan layanan otomatis -- itu keputusan bisnis pemilik; di sini hanya status + notifikasi.
// Mengembalikan jumlah tindakan (invoice kedaluwarsa + kejadian langganan yang dievaluasi).
func (s *Service) Evaluate(ctx context.Context) (int, error) {
	if !s.Enabled() {
		return 0, nil
	}
	now := s.now()
	acted := 0
	expired, err := s.repo.ListPendingExpired(ctx, now, sweepBatch)
	if err != nil {
		return 0, err
	}
	for i := range expired {
		inv := expired[i]
		changed, err := s.repo.MarkInvoiceExpired(ctx, inv.ID)
		if err != nil {
			s.log.Warn("billing: gagal menandai invoice kedaluwarsa", "invoice", inv.InvoiceNumber, "error", err.Error())
			continue
		}
		if changed {
			s.afterExpired(ctx, nil, &inv)
			acted++
		}
	}
	if s.alerts == nil {
		return acted, nil
	}
	subs, err := s.repo.ListSubscriptionsBetween(ctx, now.Add(-expiredLookback), now.Add(expiringWindow))
	if err != nil {
		return acted, err
	}
	for _, sub := range subs {
		tid := sub.TenantID
		plan := "langganan"
		if sub.PlanCode != nil {
			plan = "paket " + *sub.PlanCode
		}
		end := sub.PlanExpiresAt.Format("02-01-2006")
		if sub.PlanExpiresAt.After(now) {
			_ = s.alerts.Notify(ctx, domain.NotificationEvent{
				TypeCode: domain.NotifBillingExpiring, TenantID: &tid,
				Title:      "Langganan segera berakhir",
				Body:       fmt.Sprintf("Masa berlaku %s tenant %s berakhir pada %s. Perpanjang agar tidak terputus.", plan, sub.TenantName, end),
				Link:       "/billing",
				EntityType: "tenant", EntityID: &tid,
				DedupeKey: fmt.Sprintf("expiring:%d", sub.PlanExpiresAt.Unix()), DedupeWindow: 30 * 24 * time.Hour,
			})
		} else {
			_ = s.alerts.Notify(ctx, domain.NotificationEvent{
				TypeCode: domain.NotifBillingExpired, TenantID: &tid,
				Title:      "Langganan telah berakhir",
				Body:       fmt.Sprintf("Masa berlaku %s tenant %s berakhir pada %s. Perpanjang di menu Langganan.", plan, sub.TenantName, end),
				Link:       "/billing",
				EntityType: "tenant", EntityID: &tid,
				DedupeKey: fmt.Sprintf("expired:%d", sub.PlanExpiresAt.Unix()), DedupeWindow: 400 * 24 * time.Hour,
			})
		}
		acted++
	}
	return acted, nil
}

// ---- audit ----

// audit mencatat aksi billing ke activity_logs. Detail HANYA nomor invoice, kode paket, dan nominal -- tidak ada
// URL pembayaran, token, atau rahasia. Selama sesi impersonate, repository menambahkan impersonated_by otomatis.
func (s *Service) audit(ctx context.Context, actor *domain.Actor, tenantID uint64, action string, inv *domain.BillingInvoice, extra map[string]any) {
	if s.activity == nil {
		return
	}
	d := map[string]any{"invoice": inv.InvoiceNumber, "plan": inv.PlanCode, "amount_idr": inv.AmountIDR}
	for k, v := range extra {
		d[k] = v
	}
	tid, iid := tenantID, inv.ID
	l := &domain.ActivityLog{TenantID: &tid, Action: action, EntityType: "billing_invoice", EntityID: &iid, Details: d}
	if actor != nil {
		l.UserID = actor.UserIDPtr()
	}
	_ = s.activity.Record(ctx, l)
}

func (s *Service) auditPlan(ctx context.Context, actor domain.Actor, action string, p *domain.BillingPlan) {
	if s.activity == nil {
		return
	}
	pid := p.ID
	_ = s.activity.Record(ctx, &domain.ActivityLog{
		UserID: actor.UserIDPtr(), Action: action, EntityType: "billing_plan", EntityID: &pid,
		Details: map[string]any{"code": p.Code, "price_idr": p.PriceIDR, "is_active": p.IsActive},
	})
}
