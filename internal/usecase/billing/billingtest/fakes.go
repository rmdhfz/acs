// Package billingtest berisi fake in-memory untuk menguji usecase/billing dan handler billing tanpa MariaDB/Xendit.
// Hanya dipakai oleh tes (tidak diimpor kode produksi). Perilaku atomik repository MariaDB yang SEBENARNYA diuji
// di tes integrasi repository/mysql (billing_integration_test.go).
package billingtest

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"acs/internal/domain"
	"acs/pkg/mailer"
)

// Repo = BillingRepository + pembaca tenant/user in-memory.
type Repo struct {
	mu       sync.Mutex
	Plans    map[uint64]*domain.BillingPlan
	Invoices map[uint64]*domain.BillingInvoice
	Events   map[string]uint64
	Tenants  map[uint64]*domain.Tenant
	Users    map[uint64]*domain.User
	Admins   map[uint64][]domain.BillingContact

	nextPlan, nextInv uint64
	// ApplyCalls = berapa kali ApplyPayment dipanggil (untuk memastikan jalur dilewati/tidak).
	ApplyCalls int
}

func NewRepo() *Repo {
	return &Repo{
		Plans: map[uint64]*domain.BillingPlan{}, Invoices: map[uint64]*domain.BillingInvoice{},
		Events: map[string]uint64{}, Tenants: map[uint64]*domain.Tenant{}, Users: map[uint64]*domain.User{},
		Admins: map[uint64][]domain.BillingContact{},
	}
}

// AddPlan menambah paket (ID otomatis).
func (r *Repo) AddPlan(code string, maxDevices *uint32, price uint64, period uint16, active bool) *domain.BillingPlan {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextPlan++
	p := &domain.BillingPlan{ID: r.nextPlan, Code: code, Name: code + " plan", MaxDevices: maxDevices, PriceIDR: price, PeriodDays: period, IsActive: active, SortOrder: int(r.nextPlan)}
	r.Plans[p.ID] = p
	return p
}

func (r *Repo) AddTenant(id uint64, name string, active bool) *domain.Tenant {
	r.mu.Lock()
	defer r.mu.Unlock()
	t := &domain.Tenant{ID: id, Code: fmt.Sprintf("t%d", id), Name: name, IsActive: active}
	r.Tenants[id] = t
	return t
}

func (r *Repo) AddUser(id uint64, tenantID *uint64, email string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Users[id] = &domain.User{ID: id, TenantID: tenantID, Email: email, FullName: "User " + email, IsActive: true}
}

// Tenant mengembalikan salinan keadaan tenant saat ini.
func (r *Repo) Tenant(id uint64) domain.Tenant {
	r.mu.Lock()
	defer r.mu.Unlock()
	return *r.Tenants[id]
}

// InvoiceByNumber mengembalikan salinan invoice (nil bila tak ada).
func (r *Repo) InvoiceByNumber(n string) *domain.BillingInvoice {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, i := range r.Invoices {
		if i.InvoiceNumber == n {
			c := *i
			return &c
		}
	}
	return nil
}

// AllInvoices = salinan semua invoice, urut ID.
func (r *Repo) AllInvoices() []domain.BillingInvoice {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.sorted(func(*domain.BillingInvoice) bool { return true })
}

func (r *Repo) sorted(keep func(*domain.BillingInvoice) bool) []domain.BillingInvoice {
	out := []domain.BillingInvoice{}
	for _, i := range r.Invoices {
		if keep(i) {
			out = append(out, *i)
		}
	}
	sort.Slice(out, func(a, b int) bool { return out[a].ID > out[b].ID })
	return out
}

// SetInvoiceExpiry menggeser expires_at (untuk menguji kedaluwarsa).
func (r *Repo) SetInvoiceExpiry(number string, t time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, i := range r.Invoices {
		if i.InvoiceNumber == number {
			i.ExpiresAt = t
		}
	}
}

// ---- TenantReader / UserReader ----

func (r *Repo) GetByID(_ context.Context, id uint64) (*domain.Tenant, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.Tenants[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	c := *t
	return &c, nil
}

// UserReader terpisah agar GetByID tidak bentrok nama.
type Users struct{ R *Repo }

func (u Users) GetByID(_ context.Context, id uint64) (*domain.User, error) {
	u.R.mu.Lock()
	defer u.R.mu.Unlock()
	x, ok := u.R.Users[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	c := *x
	return &c, nil
}

// ---- BillingRepository ----

func (r *Repo) ListPlans(_ context.Context, activeOnly bool) ([]domain.BillingPlan, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []domain.BillingPlan{}
	for _, p := range r.Plans {
		if !activeOnly || p.IsActive {
			out = append(out, *p)
		}
	}
	sort.Slice(out, func(a, b int) bool { return out[a].SortOrder < out[b].SortOrder })
	return out, nil
}

func (r *Repo) GetPlanByCode(_ context.Context, code string) (*domain.BillingPlan, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, p := range r.Plans {
		if p.Code == code {
			c := *p
			return &c, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (r *Repo) GetPlanByID(_ context.Context, id uint64) (*domain.BillingPlan, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if p, ok := r.Plans[id]; ok {
		c := *p
		return &c, nil
	}
	return nil, domain.ErrNotFound
}

func (r *Repo) CreatePlan(_ context.Context, p *domain.BillingPlan, _ *uint64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, x := range r.Plans {
		if x.Code == p.Code {
			return domain.ErrConflict
		}
	}
	r.nextPlan++
	p.ID = r.nextPlan
	c := *p
	r.Plans[p.ID] = &c
	return nil
}

func (r *Repo) UpdatePlan(_ context.Context, p *domain.BillingPlan, _ *uint64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	x, ok := r.Plans[p.ID]
	if !ok {
		return domain.ErrNotFound
	}
	x.Name, x.Description, x.MaxDevices, x.PriceIDR, x.PeriodDays, x.IsActive, x.SortOrder =
		p.Name, p.Description, p.MaxDevices, p.PriceIDR, p.PeriodDays, p.IsActive, p.SortOrder
	return nil
}

func (r *Repo) CreateInvoice(_ context.Context, inv *domain.BillingInvoice) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, x := range r.Invoices {
		if x.InvoiceNumber == inv.InvoiceNumber || x.InvoiceUUID == inv.InvoiceUUID {
			return domain.ErrConflict
		}
	}
	p, ok := r.Plans[inv.PlanID]
	if !ok {
		return domain.ErrInvalidInput
	}
	r.nextInv++
	inv.ID = r.nextInv
	inv.Status = domain.InvoicePending
	c := *inv
	c.PlanCode, c.PlanName = p.Code, p.Name
	c.XenditInvoiceID, c.PaymentURL = nil, nil
	r.Invoices[inv.ID] = &c
	return nil
}

func (r *Repo) find(match func(*domain.BillingInvoice) bool) (*domain.BillingInvoice, error) {
	for _, i := range r.Invoices {
		if match(i) {
			c := *i
			return &c, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (r *Repo) GetInvoiceByUUID(_ context.Context, u string) (*domain.BillingInvoice, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.find(func(i *domain.BillingInvoice) bool { return i.InvoiceUUID == u })
}

func (r *Repo) GetInvoiceByNumber(_ context.Context, n string) (*domain.BillingInvoice, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.find(func(i *domain.BillingInvoice) bool { return i.InvoiceNumber == n })
}

func (r *Repo) GetInvoiceByXenditID(_ context.Context, x string) (*domain.BillingInvoice, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.find(func(i *domain.BillingInvoice) bool { return i.XenditInvoiceID != nil && *i.XenditInvoiceID == x })
}

func (r *Repo) SetInvoiceGateway(_ context.Context, id uint64, xid, url string, exp time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	i, ok := r.Invoices[id]
	if !ok || i.Status != domain.InvoicePending {
		return domain.ErrNotFound
	}
	i.XenditInvoiceID, i.PaymentURL, i.ExpiresAt = &xid, &url, exp
	return nil
}

func (r *Repo) ListInvoices(_ context.Context, tenantID *uint64, p domain.Pagination) ([]domain.BillingInvoice, int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	all := r.sorted(func(i *domain.BillingInvoice) bool { return tenantID == nil || i.TenantID == *tenantID })
	total := len(all)
	lo := p.Offset()
	if lo > total {
		lo = total
	}
	hi := lo + p.Limit()
	if hi > total {
		hi = total
	}
	return all[lo:hi], total, nil
}

func (r *Repo) ListPendingByTenant(_ context.Context, tenantID uint64, now time.Time) ([]domain.BillingInvoice, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.sorted(func(i *domain.BillingInvoice) bool {
		return i.TenantID == tenantID && i.Status == domain.InvoicePending && i.ExpiresAt.After(now)
	}), nil
}

func (r *Repo) ListPendingExpired(_ context.Context, now time.Time, limit int) ([]domain.BillingInvoice, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := r.sorted(func(i *domain.BillingInvoice) bool {
		return i.Status == domain.InvoicePending && !i.ExpiresAt.After(now)
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (r *Repo) transition(id uint64, to string) bool {
	i, ok := r.Invoices[id]
	if !ok || i.Status != domain.InvoicePending {
		return false
	}
	i.Status = to
	return true
}

func (r *Repo) MarkInvoiceFailed(_ context.Context, id uint64) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.transition(id, domain.InvoiceFailed), nil
}

func (r *Repo) MarkInvoiceExpired(_ context.Context, id uint64) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.transition(id, domain.InvoiceExpired), nil
}

func (r *Repo) ApplyPayment(_ context.Context, in domain.BillingPaymentInput) (domain.BillingPaymentResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ApplyCalls++
	var out domain.BillingPaymentResult
	if _, dup := r.Events[in.EventKey]; dup {
		out.Duplicate = true
		return out, nil
	}
	i, ok := r.Invoices[in.InvoiceID]
	if !ok || (i.Status != domain.InvoicePending && i.Status != domain.InvoiceExpired) {
		return out, nil
	}
	t, ok := r.Tenants[i.TenantID]
	if !ok {
		return out, domain.ErrNotFound
	}
	base := in.Now
	if t.PlanExpiresAt != nil && t.PlanExpiresAt.After(base) {
		base = *t.PlanExpiresAt
	}
	exp := base.AddDate(0, 0, int(i.PeriodDays))
	pid := i.PlanID
	t.BillingPlanID, t.MaxDevices, t.PlanExpiresAt = &pid, r.Plans[i.PlanID].MaxDevices, &exp
	paidAt, paid := in.PaidAt, in.PaidAmountIDR
	i.Status, i.PaidAt, i.PaidAmountIDR = domain.InvoicePaid, &paidAt, &paid
	r.Events[in.EventKey] = i.ID
	out.Applied, out.PlanExpiresAt = true, exp
	return out, nil
}

func (r *Repo) RecordWebhookEvent(_ context.Context, key string, invoiceID *uint64, _ string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.Events[key]; dup {
		return false, nil
	}
	var id uint64
	if invoiceID != nil {
		id = *invoiceID
	}
	r.Events[key] = id
	return true, nil
}

func (r *Repo) ListSubscriptionsBetween(_ context.Context, from, to time.Time) ([]domain.BillingTenantSubscription, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []domain.BillingTenantSubscription{}
	for _, t := range r.Tenants {
		if !t.IsActive || t.PlanExpiresAt == nil || t.PlanExpiresAt.Before(from) || t.PlanExpiresAt.After(to) {
			continue
		}
		s := domain.BillingTenantSubscription{TenantID: t.ID, TenantName: t.Name, PlanExpiresAt: *t.PlanExpiresAt}
		if t.BillingPlanID != nil {
			code := r.Plans[*t.BillingPlanID].Code
			s.PlanCode = &code
		}
		out = append(out, s)
	}
	sort.Slice(out, func(a, b int) bool { return out[a].TenantID < out[b].TenantID })
	return out, nil
}

func (r *Repo) ListTenantAdminContacts(_ context.Context, tenantID uint64, limit int) ([]domain.BillingContact, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c := r.Admins[tenantID]
	if len(c) > limit {
		c = c[:limit]
	}
	return append([]domain.BillingContact(nil), c...), nil
}

// ---- Gateway palsu ----

type Gateway struct {
	mu       sync.Mutex
	Invoices map[string]*domain.GatewayInvoice
	n        int

	CreateErr error
	GetErr    error
	// MutateCreated (opsional) mengubah respons pembuatan sebelum dikembalikan -- mensimulasikan respons dimanipulasi.
	MutateCreated func(*domain.GatewayInvoice)

	CreateCalls []domain.GatewayInvoiceInput
	GetCalls    []string
	// Now (opsional) = jam palsu untuk expiry_date respons; bawaan time.Now.
	Now func() time.Time
}

func NewGateway() *Gateway { return &Gateway{Invoices: map[string]*domain.GatewayInvoice{}} }

func (g *Gateway) CreateInvoice(_ context.Context, in domain.GatewayInvoiceInput) (*domain.GatewayInvoice, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.CreateCalls = append(g.CreateCalls, in)
	if g.CreateErr != nil {
		return nil, g.CreateErr
	}
	g.n++
	id := fmt.Sprintf("xnd_inv_%d", g.n)
	now := time.Now()
	if g.Now != nil {
		now = g.Now()
	}
	exp := now.Add(time.Duration(in.DurationSeconds) * time.Second)
	inv := &domain.GatewayInvoice{
		ID: id, ExternalID: in.ExternalID, Status: domain.GatewayStatusPending,
		InvoiceURL: "https://checkout.xendit.co/web/" + id, ExpiryDate: &exp, AmountIDR: in.AmountIDR,
	}
	g.Invoices[id] = inv
	out := *inv
	if g.MutateCreated != nil {
		g.MutateCreated(&out)
	}
	return &out, nil
}

func (g *Gateway) GetInvoice(_ context.Context, id string) (*domain.GatewayInvoice, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.GetCalls = append(g.GetCalls, id)
	if g.GetErr != nil {
		return nil, g.GetErr
	}
	inv, ok := g.Invoices[id]
	if !ok {
		return nil, fmt.Errorf("xendit: HTTP 404: tidak ditemukan")
	}
	c := *inv
	return &c, nil
}

// Set mengubah status invoice di "Xendit" (id gateway).
func (g *Gateway) Set(id, status string, paidAmount uint64, paidAt *time.Time) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if inv, ok := g.Invoices[id]; ok {
		inv.Status, inv.PaidAmountIDR, inv.PaidAt = status, paidAmount, paidAt
	}
}

// SetAmount mengubah nominal (mensimulasikan ketidakcocokan).
func (g *Gateway) SetAmount(id string, amount uint64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if inv, ok := g.Invoices[id]; ok {
		inv.AmountIDR = amount
	}
}

// GetCount = jumlah panggilan GetInvoice.
func (g *Gateway) GetCount() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.GetCalls)
}

// ---- Audit, notifikasi, email palsu ----

type Activity struct {
	mu   sync.Mutex
	Logs []domain.ActivityLog
}

func (a *Activity) Record(_ context.Context, l *domain.ActivityLog) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.Logs = append(a.Logs, *l)
	return nil
}
func (a *Activity) ListByEntity(context.Context, string, uint64, domain.Pagination) ([]domain.ActivityLog, int, error) {
	return nil, 0, nil
}
func (a *Activity) ListByTenant(context.Context, *uint64, domain.ActivityLogFilter, domain.Pagination) ([]domain.ActivityLog, int, error) {
	return nil, 0, nil
}

// Actions = daftar kode aksi yang tercatat.
func (a *Activity) Actions() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := []string{}
	for _, l := range a.Logs {
		out = append(out, l.Action)
	}
	return out
}

// Count menghitung aksi tertentu.
func (a *Activity) Count(action string) int {
	n := 0
	for _, x := range a.Actions() {
		if x == action {
			n++
		}
	}
	return n
}

type Notifier struct {
	mu     sync.Mutex
	Events []domain.NotificationEvent
	seen   map[string]bool
}

// Notify meniru dedupe: kunci (jenis, tenant, DedupeKey) yang sama tidak dicatat dua kali.
func (n *Notifier) Notify(_ context.Context, ev domain.NotificationEvent) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.seen == nil {
		n.seen = map[string]bool{}
	}
	if ev.DedupeKey != "" && ev.DedupeWindow > 0 {
		var tid uint64
		if ev.TenantID != nil {
			tid = *ev.TenantID
		}
		k := fmt.Sprintf("%s|%d|%s", ev.TypeCode, tid, ev.DedupeKey)
		if n.seen[k] {
			return nil
		}
		n.seen[k] = true
	}
	n.Events = append(n.Events, ev)
	return nil
}

func (n *Notifier) Types() []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	out := []string{}
	for _, e := range n.Events {
		out = append(out, e.TypeCode)
	}
	return out
}

type Mailer struct {
	mu   sync.Mutex
	Sent []mailer.Message
}

func (m *Mailer) Notify(msg mailer.Message) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Sent = append(m.Sent, msg)
}

func (m *Mailer) Count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.Sent)
}
