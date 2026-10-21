package billing

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"acs/internal/domain"
	"acs/internal/usecase/billing/billingtest"
	"acs/internal/usecase/notify"
)

const (
	testToken  = "tok-0123456789-abcdefghijkl-XYZ"
	secretLeak = "SUPER-SECRET-FROM-GATEWAY-ERR"
)

func u64(v uint64) *uint64 { return &v }
func u32(v uint32) *uint32 { return &v }

type env struct {
	svc    *Service
	repo   *billingtest.Repo
	gw     *billingtest.Gateway
	act    *billingtest.Activity
	notif  *billingtest.Notifier
	mail   *billingtest.Mailer
	logbuf *bytes.Buffer
	clock  *time.Time
	mu     *sync.Mutex

	admin1, admin2, noc1, viewer1, enduser1, super, impAdmin1 domain.Actor
}

var t0 = time.Date(2026, 10, 17, 10, 0, 0, 0, time.UTC)

func newEnv(t *testing.T, enabled bool) *env {
	t.Helper()
	repo := billingtest.NewRepo()
	gw := billingtest.NewGateway()
	act := &billingtest.Activity{}
	notif := &billingtest.Notifier{}
	ml := &billingtest.Mailer{}
	buf := &bytes.Buffer{}
	clock := t0
	mu := &sync.Mutex{}
	gw.Now = func() time.Time { mu.Lock(); defer mu.Unlock(); return clock }

	repo.AddPlan("STARTER", u32(50), 199000, 30, true)
	repo.AddPlan("BUSINESS", u32(500), 899000, 30, true)
	repo.AddPlan("ENTERPRISE", nil, 2499000, 30, true)
	repo.AddPlan("OLD", u32(5), 1000, 30, false)
	repo.AddPlan("FREE", u32(5), 0, 30, true)
	repo.AddTenant(1, "Tenant Satu", true)
	repo.AddTenant(2, "Tenant Dua", true)
	repo.AddTenant(3, "Tenant Mati", false)
	repo.AddUser(10, u64(1), "admin1@satu.id")
	repo.AddUser(20, u64(2), "admin2@dua.id")
	repo.AddUser(99, nil, "root@platform.id")
	repo.Admins[1] = []domain.BillingContact{{Email: "admin1@satu.id", FullName: "Admin Satu"}}

	svc := NewService(repo, repo, billingtest.Users{R: repo}, gw, act, Config{
		Enabled: enabled, CallbackToken: testToken, Invoice: 24 * time.Hour, PublicURL: "https://acs.example.com",
	}, slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	svc.now = func() time.Time { mu.Lock(); defer mu.Unlock(); return clock }
	svc.WithNotifier(notif).WithEmail(ml, notify.NewBuilder("ACS Test", "https://acs.example.com"))

	return &env{
		svc: svc, repo: repo, gw: gw, act: act, notif: notif, mail: ml, logbuf: buf, clock: &clock, mu: mu,
		admin1:    domain.Actor{UserID: 10, TenantID: u64(1), Roles: []string{domain.RoleAdmin}},
		admin2:    domain.Actor{UserID: 20, TenantID: u64(2), Roles: []string{domain.RoleAdmin}},
		noc1:      domain.Actor{UserID: 11, TenantID: u64(1), Roles: []string{domain.RoleNOC}},
		viewer1:   domain.Actor{UserID: 12, TenantID: u64(1), Roles: []string{domain.RoleViewer}},
		enduser1:  domain.Actor{UserID: 13, TenantID: u64(1), Roles: []string{domain.RoleEndUser}},
		super:     domain.Actor{UserID: 99, Roles: []string{domain.RoleSuperadmin}},
		impAdmin1: domain.Actor{UserID: 10, TenantID: u64(1), Roles: []string{domain.RoleAdmin}, ImpersonatorID: 99},
	}
}

func (e *env) advance(d time.Duration) { e.mu.Lock(); *e.clock = e.clock.Add(d); e.mu.Unlock() }

func (e *env) checkout(t *testing.T, a domain.Actor, plan string) *domain.BillingInvoice {
	t.Helper()
	inv, err := e.svc.Checkout(context.Background(), a, plan, nil)
	if err != nil {
		t.Fatalf("checkout %s: %v", plan, err)
	}
	return inv
}

// gwID = id invoice di gateway palsu untuk invoice kita.
func (e *env) gwID(t *testing.T, inv *domain.BillingInvoice) string {
	t.Helper()
	i := e.repo.InvoiceByNumber(inv.InvoiceNumber)
	if i == nil || i.XenditInvoiceID == nil {
		t.Fatal("invoice tanpa id gateway")
	}
	return *i.XenditInvoiceID
}

func (e *env) webhook(t *testing.T, inv *domain.BillingInvoice, payloadStatus string) (string, error) {
	t.Helper()
	body := fmt.Sprintf(`{"id":%q,"external_id":%q,"status":%q,"amount":1,"paid_amount":1,"unknown":{"x":[1,2]}}`, e.gwID(t, inv), inv.InvoiceNumber, payloadStatus)
	return e.svc.HandleWebhook(context.Background(), testToken, []byte(body))
}

func (e *env) pay(t *testing.T, inv *domain.BillingInvoice) {
	t.Helper()
	paidAt := e.clock.Add(-time.Minute)
	e.gw.Set(e.gwID(t, inv), domain.GatewayStatusPaid, inv.AmountIDR, &paidAt)
}

func isErr(err, target error) bool { return errors.Is(err, target) }

// ---- Checkout ----

func TestCheckout_AmountFromDB_AndRequestShape(t *testing.T) {
	e := newEnv(t, true)
	inv := e.checkout(t, e.admin1, "  starter ") // dinormalisasi
	if inv.AmountIDR != 199000 || inv.PlanCode != "STARTER" || inv.Status != domain.InvoicePending || inv.PeriodDays != 30 {
		t.Fatalf("invoice salah: %+v", inv)
	}
	if inv.PaymentURL == nil || !strings.HasPrefix(*inv.PaymentURL, "https://checkout.xendit.co/") {
		t.Fatalf("payment_url salah: %v", inv.PaymentURL)
	}
	if len(e.gw.CreateCalls) != 1 {
		t.Fatalf("gateway harus dipanggil sekali, %d", len(e.gw.CreateCalls))
	}
	c := e.gw.CreateCalls[0]
	if c.AmountIDR != 199000 {
		t.Errorf("nominal ke gateway harus dari DB: %d", c.AmountIDR)
	}
	if !strings.HasPrefix(c.ExternalID, "ACS-1-") || c.ExternalID != inv.InvoiceNumber || len(c.ExternalID) > 64 {
		t.Errorf("external_id salah: %q", c.ExternalID)
	}
	if c.PayerEmail != "admin1@satu.id" || c.DurationSeconds != 86400 ||
		c.SuccessRedirectURL != "https://acs.example.com/billing?status=success" || c.FailureRedirectURL != "https://acs.example.com/billing?status=failed" {
		t.Errorf("parameter gateway salah: %+v", c)
	}
	if e.act.Count("BILLING_CHECKOUT") != 1 {
		t.Errorf("audit BILLING_CHECKOUT harus tercatat: %v", e.act.Actions())
	}
}

func TestCheckout_RejectsBadPlans(t *testing.T) {
	e := newEnv(t, true)
	for _, code := range []string{"", "NOPE", "OLD", "FREE", "STARTER; DROP TABLE x", "star\x00ter", "S", strings.Repeat("A", 40), "1STARTER", "STAR TER"} {
		_, err := e.svc.Checkout(context.Background(), e.admin1, code, nil)
		if !isErr(err, domain.ErrInvalidInput) {
			t.Errorf("kode %q harus 400, dapat %v", code, err)
		}
	}
	if len(e.gw.CreateCalls) != 0 {
		t.Error("gateway tidak boleh dipanggil untuk paket tak valid")
	}
}

func TestCheckout_DisabledAndInactiveTenant(t *testing.T) {
	e := newEnv(t, false)
	if _, err := e.svc.Checkout(context.Background(), e.admin1, "STARTER", nil); !isErr(err, domain.ErrConflict) || !strings.Contains(err.Error(), "belum diaktifkan") {
		t.Fatalf("billing mati harus 409 jelas: %v", err)
	}
	e = newEnv(t, true)
	if _, err := e.svc.Checkout(context.Background(), e.super, "STARTER", u64(3)); !isErr(err, domain.ErrConflict) {
		t.Fatalf("tenant nonaktif harus 409: %v", err)
	}
	if _, err := e.svc.Checkout(context.Background(), e.super, "STARTER", u64(777)); !isErr(err, domain.ErrNotFound) {
		t.Fatalf("tenant tak ada harus 404: %v", err)
	}
}

func TestCheckout_Idempotent(t *testing.T) {
	e := newEnv(t, true)
	a := e.checkout(t, e.admin1, "STARTER")
	b := e.checkout(t, e.admin1, "STARTER")
	if a.InvoiceUUID != b.InvoiceUUID || len(e.gw.CreateCalls) != 1 || len(e.repo.AllInvoices()) != 1 {
		t.Fatalf("checkout kedua harus mengembalikan invoice yang sama: %s vs %s, gateway=%d", a.InvoiceUUID, b.InvoiceUUID, len(e.gw.CreateCalls))
	}
	c := e.checkout(t, e.admin1, "BUSINESS") // paket lain -> invoice baru
	if c.InvoiceUUID == a.InvoiceUUID || c.AmountIDR != 899000 {
		t.Fatalf("paket lain harus invoice baru: %+v", c)
	}
	// setelah kedaluwarsa, checkout membuat invoice baru
	e.advance(25 * time.Hour)
	d := e.checkout(t, e.admin1, "STARTER")
	if d.InvoiceUUID == a.InvoiceUUID {
		t.Error("invoice kedaluwarsa tidak boleh dipakai ulang")
	}
}

func TestCheckout_MaxFivePending(t *testing.T) {
	e := newEnv(t, true)
	for i := 1; i <= 5; i++ {
		code := fmt.Sprintf("PLAN%d", i)
		e.repo.AddPlan(code, u32(10), 1000, 30, true)
		e.checkout(t, e.admin1, code)
	}
	e.repo.AddPlan("PLAN6", u32(10), 1000, 30, true)
	if _, err := e.svc.Checkout(context.Background(), e.admin1, "PLAN6", nil); !isErr(err, domain.ErrQuotaExceeded) {
		t.Fatalf("invoice PENDING ke-6 harus ditolak (429): %v", err)
	}
	// tenant lain tidak terpengaruh
	if _, err := e.svc.Checkout(context.Background(), e.admin2, "PLAN6", nil); err != nil {
		t.Fatalf("tenant lain harus bebas: %v", err)
	}
}

func TestCheckout_GatewayFailure_MarksFailedAndHidesDetails(t *testing.T) {
	e := newEnv(t, true)
	e.gw.CreateErr = fmt.Errorf("xendit: HTTP 500: %s", secretLeak)
	_, err := e.svc.Checkout(context.Background(), e.admin1, "STARTER", nil)
	if !isErr(err, domain.ErrUpstreamUnavailable) {
		t.Fatalf("harus 502: %v", err)
	}
	if strings.Contains(err.Error(), secretLeak) || strings.Contains(err.Error(), "xendit") || strings.Contains(err.Error(), "500") {
		t.Fatalf("galat ke klien tidak boleh memuat detail gateway: %v", err)
	}
	all := e.repo.AllInvoices()
	if len(all) != 1 || all[0].Status != domain.InvoiceFailed || all[0].PaymentURL != nil || all[0].XenditInvoiceID != nil {
		t.Fatalf("invoice harus FAILED tanpa URL: %+v", all)
	}
	// FAILED tidak menghalangi percobaan berikutnya
	e.gw.CreateErr = nil
	if inv := e.checkout(t, e.admin1, "STARTER"); inv.Status != domain.InvoicePending {
		t.Fatalf("percobaan ulang harus sukses: %+v", inv)
	}
}

func TestCheckout_RejectsSuspiciousGatewayResponses(t *testing.T) {
	bad := map[string]func(*domain.GatewayInvoice){
		"http":             func(g *domain.GatewayInvoice) { g.InvoiceURL = "http://checkout.xendit.co/web/x" },
		"host asing":       func(g *domain.GatewayInvoice) { g.InvoiceURL = "https://evil.example.com/x" },
		"akhiran palsu":    func(g *domain.GatewayInvoice) { g.InvoiceURL = "https://xendit.co.evil.com/x" },
		"tanpa titik":      func(g *domain.GatewayInvoice) { g.InvoiceURL = "https://evilxendit.co/x" },
		"userinfo":         func(g *domain.GatewayInvoice) { g.InvoiceURL = "https://checkout.xendit.co@evil.com/x" },
		"javascript":       func(g *domain.GatewayInvoice) { g.InvoiceURL = "javascript:alert(1)" },
		"kosong":           func(g *domain.GatewayInvoice) { g.InvoiceURL = "" },
		"port asing":       func(g *domain.GatewayInvoice) { g.InvoiceURL = "https://checkout.xendit.co:8443/x" },
		"nominal beda":     func(g *domain.GatewayInvoice) { g.AmountIDR = 1 },
		"external_id lain": func(g *domain.GatewayInvoice) { g.ExternalID = "ACS-9-9-zzzz" },
		"tanpa id":         func(g *domain.GatewayInvoice) { g.ID = "" },
	}
	for name, mut := range bad {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t, true)
			e.gw.MutateCreated = mut
			_, err := e.svc.Checkout(context.Background(), e.admin1, "STARTER", nil)
			if !isErr(err, domain.ErrUpstreamUnavailable) {
				t.Fatalf("harus ditolak (502): %v", err)
			}
			all := e.repo.AllInvoices()
			if len(all) != 1 || all[0].Status != domain.InvoiceFailed || all[0].PaymentURL != nil {
				t.Fatalf("URL/respons mencurigakan tidak boleh disimpan: %+v", all)
			}
		})
	}
	// URL sah (termasuk subdomain) lolos
	for _, ok := range []string{"https://checkout.xendit.co/web/abc", "https://checkout-staging.xendit.co/web/abc", "https://XENDIT.co/x"} {
		e := newEnv(t, true)
		e.gw.MutateCreated = func(g *domain.GatewayInvoice) { g.InvoiceURL = ok }
		if _, err := e.svc.Checkout(context.Background(), e.admin1, "STARTER", nil); err != nil {
			t.Errorf("URL %q harus lolos: %v", ok, err)
		}
	}
}

func TestCheckout_ExtraAllowedHost(t *testing.T) {
	e := newEnv(t, true)
	e.svc.cfg.AllowedPaymentHosts = []string{"pay.test.local"}
	e.gw.MutateCreated = func(g *domain.GatewayInvoice) { g.InvoiceURL = "https://pay.test.local/inv" }
	if _, err := e.svc.Checkout(context.Background(), e.admin1, "STARTER", nil); err != nil {
		t.Fatalf("host tambahan terkonfigurasi harus lolos: %v", err)
	}
}

func TestCheckout_Authorization(t *testing.T) {
	e := newEnv(t, true)
	ctx := context.Background()
	for name, a := range map[string]domain.Actor{"NOC": e.noc1, "VIEWER": e.viewer1, "ENDUSER": e.enduser1, "tanpa peran": {UserID: 5, TenantID: u64(1)}} {
		if _, err := e.svc.Checkout(ctx, a, "STARTER", nil); !isErr(err, domain.ErrForbidden) {
			t.Errorf("%s harus 403: %v", name, err)
		}
	}
	// ADMIN tenant lain mencoba tenant 1
	if _, err := e.svc.Checkout(ctx, e.admin2, "STARTER", u64(1)); !isErr(err, domain.ErrForbidden) {
		t.Errorf("ADMIN tenant lain harus 403: %v", err)
	}
	// ADMIN tanpa tenant
	if _, err := e.svc.Checkout(ctx, domain.Actor{UserID: 77, Roles: []string{domain.RoleAdmin}}, "STARTER", nil); !isErr(err, domain.ErrForbidden) {
		t.Errorf("ADMIN tanpa tenant harus 403: %v", err)
	}
	// SUPERADMIN wajib tenant_id
	if _, err := e.svc.Checkout(ctx, e.super, "STARTER", nil); !isErr(err, domain.ErrInvalidInput) {
		t.Errorf("SUPERADMIN tanpa tenant_id harus 400: %v", err)
	}
	inv, err := e.svc.Checkout(ctx, e.super, "STARTER", u64(2))
	if err != nil || inv.TenantID != 2 {
		t.Fatalf("SUPERADMIN dengan tenant_id harus sukses: %v", err)
	}
	if got := e.gw.CreateCalls[len(e.gw.CreateCalls)-1].PayerEmail; got != "root@platform.id" {
		t.Errorf("payer_email = email aktor, dapat %q", got)
	}
	// sesi impersonate sebagai ADMIN tetap boleh dan memakai tenantnya
	inv, err = e.svc.Checkout(ctx, e.impAdmin1, "BUSINESS", nil)
	if err != nil || inv.TenantID != 1 {
		t.Fatalf("impersonate ADMIN harus boleh checkout: %v", err)
	}
}

// ---- Webhook ----

func TestWebhook_WrongTokenNoEffect(t *testing.T) {
	e := newEnv(t, true)
	inv := e.checkout(t, e.admin1, "STARTER")
	e.pay(t, inv)
	body := []byte(fmt.Sprintf(`{"id":%q,"external_id":%q,"status":"PAID"}`, e.gwID(t, inv), inv.InvoiceNumber))
	for _, tok := range []string{"", "salah", testToken + "x", testToken[:len(testToken)-1], strings.ToUpper(testToken)} {
		if _, err := e.svc.HandleWebhook(context.Background(), tok, body); !isErr(err, domain.ErrUnauthorized) {
			t.Errorf("token %q harus 401: %v", tok, err)
		}
	}
	if e.gw.GetCount() != 0 || e.repo.ApplyCalls != 0 {
		t.Error("token salah tidak boleh menyentuh gateway/DB")
	}
	if got := e.repo.Tenant(1); got.PlanExpiresAt != nil {
		t.Error("tenant tidak boleh berubah")
	}
}

func TestWebhook_Disabled(t *testing.T) {
	e := newEnv(t, false)
	if _, err := e.svc.HandleWebhook(context.Background(), testToken, []byte(`{"id":"x"}`)); !isErr(err, domain.ErrNotFound) {
		t.Fatalf("billing mati -> not found: %v", err)
	}
	// konstruksi dengan token kosong tidak boleh pernah mengaktifkan billing
	s := NewService(e.repo, e.repo, billingtest.Users{R: e.repo}, e.gw, e.act, Config{Enabled: true, CallbackToken: ""}, nil)
	if s.Enabled() {
		t.Fatal("tanpa token callback billing harus mati")
	}
}

func TestWebhook_UnknownInvoiceIgnored(t *testing.T) {
	e := newEnv(t, true)
	out, err := e.svc.HandleWebhook(context.Background(), testToken, []byte(`{"id":"inv_asing","external_id":"ACS-9-9-zz","status":"PAID"}`))
	if err != nil || out != OutcomeIgnored {
		t.Fatalf("invoice tak dikenal -> diabaikan: %q %v", out, err)
	}
	if e.gw.GetCount() != 0 || e.repo.ApplyCalls != 0 {
		t.Error("tidak boleh ada efek")
	}
}

func TestWebhook_ForgedPaidButGatewaySaysPending(t *testing.T) {
	e := newEnv(t, true)
	inv := e.checkout(t, e.admin1, "STARTER")
	out, err := e.webhook(t, inv, "PAID") // payload mengaku PAID; gateway masih PENDING
	if err != nil || out != OutcomePending {
		t.Fatalf("harus pending: %q %v", out, err)
	}
	if e.repo.ApplyCalls != 0 || e.repo.InvoiceByNumber(inv.InvoiceNumber).Status != domain.InvoicePending || e.repo.Tenant(1).PlanExpiresAt != nil {
		t.Fatal("payload palsu tidak boleh melunasi")
	}
	if e.gw.GetCount() != 1 {
		t.Error("harus memverifikasi ke gateway")
	}
}

func TestWebhook_AmountMismatchNotPaid(t *testing.T) {
	e := newEnv(t, true)
	inv := e.checkout(t, e.admin1, "STARTER")
	gid := e.gwID(t, inv)
	e.gw.SetAmount(gid, 1000)
	e.pay(t, inv)
	out, err := e.webhook(t, inv, "PAID")
	if err != nil || out != OutcomeMismatch || e.repo.ApplyCalls != 0 || e.repo.Tenant(1).PlanExpiresAt != nil {
		t.Fatalf("nominal beda tidak boleh melunasi: %q %v", out, err)
	}
	if e.act.Count("BILLING_MISMATCH") != 1 {
		t.Error("ketidakcocokan harus diaudit")
	}
}

func TestWebhook_ExternalIDMismatchAndUnderpaid(t *testing.T) {
	e := newEnv(t, true)
	inv := e.checkout(t, e.admin1, "STARTER")
	gid := e.gwID(t, inv)
	e.gw.Invoices[gid].ExternalID = "ACS-1-1-lain"
	e.pay(t, inv)
	if out, _ := e.webhook(t, inv, "PAID"); out != OutcomeMismatch {
		t.Fatalf("external_id beda -> mismatch: %q", out)
	}
	e.gw.Invoices[gid].ExternalID = inv.InvoiceNumber
	e.gw.Set(gid, domain.GatewayStatusPaid, 100000, nil) // terbayar kurang
	if out, _ := e.webhook(t, inv, "PAID"); out != OutcomeUnderpaid || e.repo.ApplyCalls != 0 {
		t.Fatalf("kurang bayar tidak boleh melunasi: %q", out)
	}
}

func TestWebhook_PaidAppliesPlanDevicesExpiry(t *testing.T) {
	e := newEnv(t, true)
	inv := e.checkout(t, e.admin1, "STARTER")
	e.pay(t, inv)
	out, err := e.webhook(t, inv, "PAID")
	if err != nil || out != OutcomePaid {
		t.Fatalf("harus lunas: %q %v", out, err)
	}
	ten := e.repo.Tenant(1)
	wantExp := t0.AddDate(0, 0, 30)
	if ten.PlanExpiresAt == nil || !ten.PlanExpiresAt.Equal(wantExp) {
		t.Fatalf("plan_expires_at = %v, mau %v", ten.PlanExpiresAt, wantExp)
	}
	if ten.MaxDevices == nil || *ten.MaxDevices != 50 || ten.BillingPlanID == nil {
		t.Fatalf("max_devices/paket salah: %+v", ten)
	}
	got := e.repo.InvoiceByNumber(inv.InvoiceNumber)
	if got.Status != domain.InvoicePaid || got.PaidAt == nil || got.PaidAmountIDR == nil || *got.PaidAmountIDR != 199000 {
		t.Fatalf("invoice harus PAID: %+v", got)
	}
	if e.act.Count("BILLING_PAID") != 1 {
		t.Errorf("audit BILLING_PAID: %v", e.act.Actions())
	}
	if len(e.notif.Events) != 1 || e.notif.Events[0].TypeCode != domain.NotifBillingPaid || !e.notif.Events[0].NoEmail {
		t.Fatalf("notifikasi BILLING_PAID (tanpa email ganda): %+v", e.notif.Events)
	}
	if e.mail.Count() != 1 {
		t.Fatalf("email tanda terima harus 1: %d", e.mail.Count())
	}
	m := e.mail.Sent[0]
	if m.To[0] != "admin1@satu.id" || !strings.Contains(m.Text, "Rp 199.000") || !strings.Contains(m.Text, inv.InvoiceNumber) {
		t.Errorf("isi email salah: %s", m.Text)
	}
	for _, bad := range []string{testToken, "xnd_", "Authorization"} {
		if strings.Contains(m.Text, bad) || strings.Contains(m.HTML, bad) {
			t.Errorf("email memuat %q", bad)
		}
	}
	// Setelah lunas payment_url tidak diekspos lagi.
	list, _, _ := e.svc.ListInvoices(context.Background(), e.admin1, nil, domain.Pagination{})
	if len(list) != 1 || list[0].PaymentURL != nil || list[0].Status != domain.InvoicePaid {
		t.Errorf("daftar invoice: %+v", list)
	}
}

func TestWebhook_SettledCountsAsPaid(t *testing.T) {
	e := newEnv(t, true)
	inv := e.checkout(t, e.admin1, "STARTER")
	e.gw.Set(e.gwID(t, inv), domain.GatewayStatusSettled, 199000, nil)
	if out, err := e.webhook(t, inv, "SETTLED"); err != nil || out != OutcomePaid {
		t.Fatalf("SETTLED = lunas: %q %v", out, err)
	}
}

func TestWebhook_RenewalStacksFromOldExpiry(t *testing.T) {
	e := newEnv(t, true)
	a := e.checkout(t, e.admin1, "STARTER")
	e.pay(t, a)
	e.webhook(t, a, "PAID")
	first := *e.repo.Tenant(1).PlanExpiresAt // t0+30d
	e.advance(10 * 24 * time.Hour)
	b := e.checkout(t, e.admin1, "BUSINESS")
	e.pay(t, b)
	if out, err := e.webhook(t, b, "PAID"); err != nil || out != OutcomePaid {
		t.Fatalf("%q %v", out, err)
	}
	ten := e.repo.Tenant(1)
	if want := first.AddDate(0, 0, 30); !ten.PlanExpiresAt.Equal(want) {
		t.Fatalf("perpanjangan harus menumpuk dari expiry lama: %v, mau %v", ten.PlanExpiresAt, want)
	}
	if *ten.MaxDevices != 500 {
		t.Errorf("max_devices harus ikut paket baru: %d", *ten.MaxDevices)
	}
	// langganan sudah lapse -> dasar = sekarang
	e.advance(100 * 24 * time.Hour)
	c := e.checkout(t, e.admin1, "ENTERPRISE")
	e.pay(t, c)
	e.webhook(t, c, "PAID")
	ten = e.repo.Tenant(1)
	if want := e.clock.AddDate(0, 0, 30); !ten.PlanExpiresAt.Equal(want) {
		t.Fatalf("setelah lapse dasar = sekarang: %v, mau %v", ten.PlanExpiresAt, want)
	}
	if ten.MaxDevices != nil {
		t.Errorf("ENTERPRISE = tak dibatasi (nil): %v", *ten.MaxDevices)
	}
}

func TestWebhook_DuplicateIsIdempotent(t *testing.T) {
	e := newEnv(t, true)
	inv := e.checkout(t, e.admin1, "STARTER")
	e.pay(t, inv)
	e.webhook(t, inv, "PAID")
	exp := *e.repo.Tenant(1).PlanExpiresAt
	for i := 0; i < 3; i++ {
		out, err := e.webhook(t, inv, "PAID")
		if err != nil || out != OutcomeDuplicate {
			t.Fatalf("ulang #%d harus duplicate: %q %v", i, out, err)
		}
	}
	// Event berbeda (SETTLED setelah PAID) juga tidak menambah periode.
	e.gw.Set(e.gwID(t, inv), domain.GatewayStatusSettled, 199000, nil)
	e.webhook(t, inv, "SETTLED")
	if !e.repo.Tenant(1).PlanExpiresAt.Equal(exp) {
		t.Fatal("callback ganda tidak boleh menambah periode")
	}
	if len(e.notif.Events) != 1 || e.mail.Count() != 1 || e.act.Count("BILLING_PAID") != 1 {
		t.Errorf("efek samping hanya sekali: notif=%d mail=%d audit=%d", len(e.notif.Events), e.mail.Count(), e.act.Count("BILLING_PAID"))
	}
}

func TestWebhook_ParallelSingleWinner(t *testing.T) {
	e := newEnv(t, true)
	inv := e.checkout(t, e.admin1, "STARTER")
	e.pay(t, inv)
	var wg sync.WaitGroup
	var mu sync.Mutex
	counts := map[string]int{}
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, err := e.webhook(t, inv, "PAID")
			if err != nil {
				out = "ERR:" + err.Error()
			}
			mu.Lock()
			counts[out]++
			mu.Unlock()
		}()
	}
	wg.Wait()
	if counts[OutcomePaid] != 1 {
		t.Fatalf("tepat satu pemenang, dapat %v", counts)
	}
	if want := t0.AddDate(0, 0, 30); !e.repo.Tenant(1).PlanExpiresAt.Equal(want) {
		t.Fatalf("diperpanjang sekali saja: %v", e.repo.Tenant(1).PlanExpiresAt)
	}
	if len(e.notif.Events) != 1 {
		t.Errorf("notifikasi sekali: %d", len(e.notif.Events))
	}
}

func TestWebhook_ExpiredThenLatePayment(t *testing.T) {
	e := newEnv(t, true)
	inv := e.checkout(t, e.admin1, "STARTER")
	e.gw.Set(e.gwID(t, inv), domain.GatewayStatusExpired, 0, nil)
	if out, err := e.webhook(t, inv, "EXPIRED"); err != nil || out != OutcomeExpired {
		t.Fatalf("%q %v", out, err)
	}
	if e.repo.InvoiceByNumber(inv.InvoiceNumber).Status != domain.InvoiceExpired {
		t.Fatal("harus EXPIRED")
	}
	if types := e.notif.Types(); len(types) != 1 || types[0] != domain.NotifBillingFailed {
		t.Fatalf("notifikasi BILLING_FAILED: %v", types)
	}
	// Pembayaran tiba mepet batas waktu: gateway berwenang -> tetap dilunasi.
	e.pay(t, inv)
	if out, err := e.webhook(t, inv, "PAID"); err != nil || out != OutcomePaid {
		t.Fatalf("pembayaran terlambat harus tetap diakui: %q %v", out, err)
	}
}

func TestWebhook_PayloadValidation(t *testing.T) {
	e := newEnv(t, true)
	bad := map[string]string{
		"bukan json":       `halo`,
		"array":            `[1,2]`,
		"kosong":           `{}`,
		"data tambahan":    `{"id":"a"}{"id":"b"}`,
		"id panjang":       fmt.Sprintf(`{"id":%q}`, strings.Repeat("a", 129)),
		"karakter kontrol": "{\"id\":\"a\\u0000b\"}",
		"external kontrol": "{\"external_id\":\"a\\nb\"}",
		"id angka":         `{"id":123}`,
	}
	for name, body := range bad {
		if _, err := e.svc.HandleWebhook(context.Background(), testToken, []byte(body)); !isErr(err, domain.ErrInvalidInput) {
			t.Errorf("%s harus 400: %v", name, err)
		}
	}
	if _, err := e.svc.HandleWebhook(context.Background(), testToken, make([]byte, maxWebhookBody+1)); !isErr(err, domain.ErrInvalidInput) {
		t.Error("body > 64 KiB harus ditolak")
	}
}

func TestWebhook_GatewayErrorIsRetryable(t *testing.T) {
	e := newEnv(t, true)
	inv := e.checkout(t, e.admin1, "STARTER")
	e.gw.GetErr = fmt.Errorf("xendit: HTTP 503: %s", secretLeak)
	_, err := e.webhook(t, inv, "PAID")
	if !isErr(err, domain.ErrUpstreamUnavailable) || strings.Contains(err.Error(), secretLeak) {
		t.Fatalf("galat gateway -> 502 umum agar Xendit mengulang: %v", err)
	}
}

func TestWebhook_TenantDeleted(t *testing.T) {
	e := newEnv(t, true)
	inv := e.checkout(t, e.admin1, "STARTER")
	e.pay(t, inv)
	delete(e.repo.Tenants, 1)
	out, err := e.webhook(t, inv, "PAID")
	if err != nil || out != OutcomeTenantMissed {
		t.Fatalf("tenant hilang: %q %v", out, err)
	}
	if e.act.Count("BILLING_TENANT_MISSING") != 1 {
		t.Error("harus diaudit untuk penanganan manual")
	}
}

// ---- Sync ----

func TestSync_AppliesGatewayStatus_AndRateLimits(t *testing.T) {
	e := newEnv(t, true)
	inv := e.checkout(t, e.admin1, "STARTER")
	got, err := e.svc.Sync(context.Background(), e.admin1, inv.InvoiceUUID)
	if err != nil || got.Status != domain.InvoicePending {
		t.Fatalf("masih pending: %+v %v", got, err)
	}
	if _, err := e.svc.Sync(context.Background(), e.admin1, inv.InvoiceUUID); !isErr(err, domain.ErrQuotaExceeded) {
		t.Fatalf("sync kedua <10 dtk harus 429: %v", err)
	}
	e.pay(t, inv)
	e.advance(11 * time.Second)
	got, err = e.svc.Sync(context.Background(), e.admin1, inv.InvoiceUUID)
	if err != nil || got.Status != domain.InvoicePaid || got.PaymentURL != nil {
		t.Fatalf("sync harus melunasi: %+v %v", got, err)
	}
	if e.repo.Tenant(1).PlanExpiresAt == nil {
		t.Fatal("tenant harus diperpanjang")
	}
	// sync invoice lunas tidak memanggil gateway lagi
	calls := e.gw.GetCount()
	if _, err := e.svc.Sync(context.Background(), e.admin1, inv.InvoiceUUID); err != nil || e.gw.GetCount() != calls {
		t.Errorf("invoice lunas tidak perlu ditanyakan: %v", err)
	}
}

func TestSync_AuthorizationAndValidation(t *testing.T) {
	e := newEnv(t, true)
	inv := e.checkout(t, e.admin1, "STARTER")
	ctx := context.Background()
	if _, err := e.svc.Sync(ctx, e.admin2, inv.InvoiceUUID); !isErr(err, domain.ErrNotFound) {
		t.Errorf("tenant lain harus 404: %v", err)
	}
	for name, a := range map[string]domain.Actor{"NOC": e.noc1, "VIEWER": e.viewer1, "ENDUSER": e.enduser1} {
		if _, err := e.svc.Sync(ctx, a, inv.InvoiceUUID); !isErr(err, domain.ErrForbidden) {
			t.Errorf("%s harus 403: %v", name, err)
		}
	}
	for _, id := range []string{"", "abc", "123", inv.InvoiceUUID + "x", " ' OR 1=1 --", strings.ToUpper(inv.InvoiceUUID)[:35], "{" + inv.InvoiceUUID + "}", "urn:uuid:" + inv.InvoiceUUID} {
		if _, err := e.svc.Sync(ctx, e.admin1, id); !isErr(err, domain.ErrInvalidInput) {
			t.Errorf("uuid %q harus 400: %v", id, err)
		}
	}
	if _, err := e.svc.Sync(ctx, e.admin1, "00000000-0000-4000-8000-000000000000"); !isErr(err, domain.ErrNotFound) {
		t.Errorf("uuid tak ada harus 404: %v", err)
	}
	if _, err := e.svc.Sync(ctx, e.super, inv.InvoiceUUID); err != nil {
		t.Errorf("SUPERADMIN boleh sync: %v", err)
	}
	e.advance(time.Minute)
	e.gw.GetErr = fmt.Errorf("xendit: %s", secretLeak)
	if _, err := e.svc.Sync(ctx, e.admin1, inv.InvoiceUUID); !isErr(err, domain.ErrUpstreamUnavailable) || strings.Contains(err.Error(), secretLeak) {
		t.Errorf("galat gateway -> 502 umum: %v", err)
	}
	off := newEnv(t, false)
	if _, err := off.svc.Sync(ctx, off.admin1, "00000000-0000-4000-8000-000000000000"); !isErr(err, domain.ErrConflict) {
		t.Errorf("billing mati -> 409: %v", err)
	}
}

// ---- Evaluate ----

func TestEvaluate_ExpiresPendingInvoicesOnce(t *testing.T) {
	e := newEnv(t, true)
	inv := e.checkout(t, e.admin1, "STARTER")
	e.advance(25 * time.Hour)
	n, err := e.svc.Evaluate(context.Background())
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if e.repo.InvoiceByNumber(inv.InvoiceNumber).Status != domain.InvoiceExpired {
		t.Fatal("harus EXPIRED")
	}
	if n, _ := e.svc.Evaluate(context.Background()); n != 0 {
		t.Errorf("putaran kedua tidak boleh bertindak lagi: %d", n)
	}
	if got := e.notif.Types(); len(got) != 1 || got[0] != domain.NotifBillingFailed {
		t.Errorf("notifikasi: %v", got)
	}
}

func TestEvaluate_SubscriptionNotificationsDeduped(t *testing.T) {
	e := newEnv(t, true)
	pid := uint64(1)
	set := func(id uint64, d time.Duration) {
		exp := t0.Add(d)
		e.repo.Tenants[id].BillingPlanID, e.repo.Tenants[id].PlanExpiresAt = &pid, &exp
	}
	set(1, 3*24*time.Hour)  // segera berakhir
	set(2, 20*24*time.Hour) // masih lama
	e.repo.AddTenant(4, "Sudah Lewat", true)
	set(4, -24*time.Hour) // sudah berakhir kemarin
	e.repo.AddTenant(5, "Lama Sekali", true)
	set(5, -40*24*time.Hour) // lewat 40 hari: di luar jendela
	for i := 0; i < 3; i++ {
		if _, err := e.svc.Evaluate(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	count := map[string]int{}
	for _, ev := range e.notif.Events {
		count[fmt.Sprintf("%s:%d", ev.TypeCode, *ev.TenantID)]++
	}
	want := map[string]int{"BILLING_EXPIRING:1": 1, "BILLING_EXPIRED:4": 1}
	if len(count) != len(want) || count["BILLING_EXPIRING:1"] != 1 || count["BILLING_EXPIRED:4"] != 1 {
		t.Fatalf("notifikasi %v, mau %v", count, want)
	}
	// perpanjangan -> periode baru -> kunci baru
	set(1, 33*24*time.Hour)
	e.advance(30 * 24 * time.Hour) // sisa 3 hari lagi
	e.svc.Evaluate(context.Background())
	if c := 0; true {
		for _, ev := range e.notif.Events {
			if ev.TypeCode == domain.NotifBillingExpiring && *ev.TenantID == 1 {
				c++
			}
		}
		if c != 2 {
			t.Errorf("periode baru harus memicu notifikasi baru: %d", c)
		}
	}
}

func TestEvaluate_DisabledDoesNothing(t *testing.T) {
	e := newEnv(t, false)
	if n, err := e.svc.Evaluate(context.Background()); n != 0 || err != nil {
		t.Fatalf("%d %v", n, err)
	}
}

// ---- Paket ----

func TestPlans_ListByRole(t *testing.T) {
	e := newEnv(t, true)
	ctx := context.Background()
	adm, err := e.svc.ListPlans(ctx, e.admin1)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range adm {
		if !p.IsActive {
			t.Errorf("non-superadmin hanya melihat paket aktif: %s", p.Code)
		}
	}
	all, _ := e.svc.ListPlans(ctx, e.super)
	if len(all) != len(adm)+1 {
		t.Errorf("superadmin melihat semua: %d vs %d", len(all), len(adm))
	}
	for _, a := range []domain.Actor{e.noc1, e.viewer1} {
		if _, err := e.svc.ListPlans(ctx, a); err != nil {
			t.Errorf("NOC/VIEWER boleh melihat paket: %v", err)
		}
	}
	if _, err := e.svc.ListPlans(ctx, e.enduser1); !isErr(err, domain.ErrForbidden) {
		t.Errorf("ENDUSER 403: %v", err)
	}
}

func i64(v int64) *int64 { return &v }

func TestCreatePlan_Validation(t *testing.T) {
	e := newEnv(t, true)
	ctx := context.Background()
	ok := CreatePlanInput{Code: "gold_1", Name: "Gold", Description: "Paket emas", MaxDevices: i64(100), PriceIDR: 500000, PeriodDays: 30, IsActive: true}
	cases := map[string]func(*CreatePlanInput){
		"kode kecil valid->normalisasi": nil,
		"kode diawali angka":            func(p *CreatePlanInput) { p.Code = "1GOLD" },
		"kode terlalu pendek":           func(p *CreatePlanInput) { p.Code = "G" },
		"kode terlalu panjang":          func(p *CreatePlanInput) { p.Code = strings.Repeat("A", 33) },
		"kode spasi":                    func(p *CreatePlanInput) { p.Code = "GO LD" },
		"nama pendek":                   func(p *CreatePlanInput) { p.Name = "x" },
		"nama panjang":                  func(p *CreatePlanInput) { p.Name = strings.Repeat("n", 65) },
		"nama kontrol":                  func(p *CreatePlanInput) { p.Name = "Go\x00ld" },
		"nama newline":                  func(p *CreatePlanInput) { p.Name = "Go\nld" },
		"deskripsi panjang":             func(p *CreatePlanInput) { p.Description = strings.Repeat("d", 256) },
		"deskripsi kontrol":             func(p *CreatePlanInput) { p.Description = "a\tb" },
		"harga negatif":                 func(p *CreatePlanInput) { p.PriceIDR = -1 },
		"harga terlalu besar":           func(p *CreatePlanInput) { p.PriceIDR = MaxPlanPriceIDR + 1 },
		"max_devices 0":                 func(p *CreatePlanInput) { p.MaxDevices = i64(0) },
		"max_devices negatif":           func(p *CreatePlanInput) { p.MaxDevices = i64(-5) },
		"max_devices besar":             func(p *CreatePlanInput) { p.MaxDevices = i64(MaxPlanDevices + 1) },
		"periode 0":                     func(p *CreatePlanInput) { p.PeriodDays = 0 },
		"periode 367":                   func(p *CreatePlanInput) { p.PeriodDays = 367 },
		"sort liar":                     func(p *CreatePlanInput) { p.SortOrder = 1 << 40 },
	}
	for name, mut := range cases {
		in := ok
		if mut != nil {
			mut(&in)
		}
		p, err := e.svc.CreatePlan(ctx, e.super, in)
		if mut == nil {
			if err != nil || p.Code != "GOLD_1" || p.PriceIDR != 500000 {
				t.Errorf("%s: %v %+v", name, err, p)
			}
			continue
		}
		if !isErr(err, domain.ErrInvalidInput) {
			t.Errorf("%s: harus 400, dapat %v", name, err)
		}
	}
	// batas valid
	edge := CreatePlanInput{Code: "EDGE", Name: "Ed", MaxDevices: nil, PriceIDR: 0, PeriodDays: 366}
	if _, err := e.svc.CreatePlan(ctx, e.super, edge); err != nil {
		t.Errorf("nilai batas valid: %v", err)
	}
	if _, err := e.svc.CreatePlan(ctx, e.super, ok); !isErr(err, domain.ErrConflict) {
		t.Errorf("kode ganda harus 409: %v", err)
	}
	if _, err := e.svc.CreatePlan(ctx, e.admin1, ok); !isErr(err, domain.ErrForbidden) {
		t.Errorf("ADMIN tidak boleh membuat paket: %v", err)
	}
}

func TestUpdatePlan_OnlyAffectsNewInvoices(t *testing.T) {
	e := newEnv(t, true)
	ctx := context.Background()
	old := e.checkout(t, e.admin1, "STARTER")
	newPrice := int64(250000)
	p, err := e.svc.UpdatePlan(ctx, e.super, "starter", UpdatePlanInput{PriceIDR: &newPrice, MaxDevices: OptionalInt64{Set: true, Value: nil}})
	if err != nil || p.PriceIDR != 250000 || p.MaxDevices != nil {
		t.Fatalf("%v %+v", err, p)
	}
	if got := e.repo.InvoiceByNumber(old.InvoiceNumber); got.AmountIDR != 199000 {
		t.Fatalf("nominal invoice lama tidak boleh berubah: %d", got.AmountIDR)
	}
	e.advance(25 * time.Hour)
	if inv := e.checkout(t, e.admin1, "STARTER"); inv.AmountIDR != 250000 {
		t.Fatalf("invoice baru memakai harga baru: %d", inv.AmountIDR)
	}
	// tidak mengirim max_devices = tidak berubah
	name := "Starter Plus"
	p, _ = e.svc.UpdatePlan(ctx, e.super, "STARTER", UpdatePlanInput{Name: &name})
	if p.Name != name || p.MaxDevices != nil || p.PriceIDR != 250000 {
		t.Errorf("PATCH parsial salah: %+v", p)
	}
	bad := int64(-1)
	zero := int64(0)
	for name, in := range map[string]UpdatePlanInput{
		"harga":   {PriceIDR: &bad},
		"periode": {PeriodDays: &zero},
		"max":     {MaxDevices: OptionalInt64{Set: true, Value: &zero}},
		"nama":    {Name: func() *string { s := "x"; return &s }()},
	} {
		if _, err := e.svc.UpdatePlan(ctx, e.super, "STARTER", in); !isErr(err, domain.ErrInvalidInput) {
			t.Errorf("%s harus 400: %v", name, err)
		}
	}
	if _, err := e.svc.UpdatePlan(ctx, e.super, "NOPE", UpdatePlanInput{}); !isErr(err, domain.ErrNotFound) {
		t.Errorf("paket tak ada 404: %v", err)
	}
	if _, err := e.svc.UpdatePlan(ctx, e.super, "bad code!", UpdatePlanInput{}); !isErr(err, domain.ErrInvalidInput) {
		t.Errorf("kode tak valid 400: %v", err)
	}
	if _, err := e.svc.UpdatePlan(ctx, e.admin1, "STARTER", UpdatePlanInput{}); !isErr(err, domain.ErrForbidden) {
		t.Errorf("ADMIN 403: %v", err)
	}
	inactive := false
	if _, err := e.svc.UpdatePlan(ctx, e.super, "STARTER", UpdatePlanInput{IsActive: &inactive}); err != nil {
		t.Fatal(err)
	}
	e.advance(25 * time.Hour)
	if _, err := e.svc.Checkout(ctx, e.admin1, "STARTER", nil); !isErr(err, domain.ErrInvalidInput) {
		t.Errorf("paket dinonaktifkan tidak bisa dibeli: %v", err)
	}
}

// ---- Langganan & daftar invoice ----

func TestSubscription_StatusesAndVisibility(t *testing.T) {
	e := newEnv(t, true)
	ctx := context.Background()
	v, err := e.svc.Subscription(ctx, e.admin1, nil)
	if err != nil || !v.Enabled || v.Status != domain.SubscriptionNone || v.Plan != nil || v.ExpiresAt != nil || v.DaysLeft != nil || v.PendingInvoice != nil {
		t.Fatalf("NONE: %+v %v", v, err)
	}
	inv := e.checkout(t, e.admin1, "STARTER")
	v, _ = e.svc.Subscription(ctx, e.admin1, nil)
	if v.PendingInvoice == nil || v.PendingInvoice.InvoiceUUID != inv.InvoiceUUID || v.PendingInvoice.PaymentURL == nil {
		t.Fatalf("ADMIN melihat pending_invoice: %+v", v.PendingInvoice)
	}
	if nv, _ := e.svc.Subscription(ctx, e.noc1, nil); nv.PendingInvoice != nil {
		t.Error("NOC tidak boleh melihat pending_invoice (memuat URL bayar)")
	}
	e.pay(t, inv)
	e.webhook(t, inv, "PAID")
	v, _ = e.svc.Subscription(ctx, e.admin1, nil)
	if v.Status != domain.SubscriptionActive || v.Plan == nil || v.Plan.Code != "STARTER" || v.DaysLeft == nil || *v.DaysLeft != 30 || v.MaxDevices == nil || *v.MaxDevices != 50 || v.PendingInvoice != nil {
		t.Fatalf("ACTIVE: %+v", v)
	}
	e.advance(31 * 24 * time.Hour)
	v, _ = e.svc.Subscription(ctx, e.admin1, nil)
	if v.Status != domain.SubscriptionExpired || *v.DaysLeft != 0 {
		t.Fatalf("EXPIRED: %+v", v)
	}
	if _, err := e.svc.Subscription(ctx, e.admin1, u64(2)); !isErr(err, domain.ErrForbidden) {
		t.Errorf("tenant lain 403: %v", err)
	}
	if _, err := e.svc.Subscription(ctx, e.super, nil); !isErr(err, domain.ErrInvalidInput) {
		t.Errorf("SUPERADMIN tanpa tenant_id 400: %v", err)
	}
	if _, err := e.svc.Subscription(ctx, e.super, u64(2)); err != nil {
		t.Errorf("SUPERADMIN dengan tenant_id: %v", err)
	}
	if _, err := e.svc.Subscription(ctx, e.enduser1, nil); !isErr(err, domain.ErrForbidden) {
		t.Errorf("ENDUSER 403: %v", err)
	}
	off := newEnv(t, false)
	if ov, err := off.svc.Subscription(ctx, off.admin1, nil); err != nil || ov.Enabled {
		t.Errorf("billing mati -> enabled=false: %+v %v", ov, err)
	}
}

func TestListInvoices_TenantIsolationAndRoles(t *testing.T) {
	e := newEnv(t, true)
	ctx := context.Background()
	e.checkout(t, e.admin1, "STARTER")
	e.checkout(t, e.admin2, "BUSINESS")
	rows, total, err := e.svc.ListInvoices(ctx, e.admin1, nil, domain.Pagination{})
	if err != nil || total != 1 || rows[0].TenantID != 1 {
		t.Fatalf("ADMIN hanya tenant sendiri: %v %d %+v", err, total, rows)
	}
	if _, _, err := e.svc.ListInvoices(ctx, e.admin1, u64(2), domain.Pagination{}); !isErr(err, domain.ErrForbidden) {
		t.Errorf("tenant_id lain 403: %v", err)
	}
	if _, total, _ := e.svc.ListInvoices(ctx, e.super, nil, domain.Pagination{}); total != 2 {
		t.Errorf("SUPERADMIN semua: %d", total)
	}
	if _, total, _ := e.svc.ListInvoices(ctx, e.super, u64(2), domain.Pagination{}); total != 1 {
		t.Errorf("SUPERADMIN filter tenant: %d", total)
	}
	for _, a := range []domain.Actor{e.noc1, e.viewer1, e.enduser1} {
		if _, _, err := e.svc.ListInvoices(ctx, a, nil, domain.Pagination{}); !isErr(err, domain.ErrForbidden) {
			t.Errorf("NOC/VIEWER/ENDUSER 403: %v", err)
		}
	}
	// PENDING yang sudah lewat expires_at tidak menampilkan URL bayar
	e.advance(30 * time.Hour)
	rows, _, _ = e.svc.ListInvoices(ctx, e.admin1, nil, domain.Pagination{})
	if rows[0].PaymentURL != nil {
		t.Error("invoice lewat batas tidak boleh menampilkan payment_url")
	}
}

// ---- Tidak ada rahasia di log ----

func TestNoSecretsInLogsOrErrors(t *testing.T) {
	e := newEnv(t, true)
	inv := e.checkout(t, e.admin1, "STARTER")
	_, _ = e.svc.HandleWebhook(context.Background(), "tebakan-salah", []byte(`{"id":"x"}`))
	e.gw.GetErr = fmt.Errorf("xendit: gagal")
	_, _ = e.webhook(t, inv, "PAID")
	e.gw.GetErr = nil
	e.gw.Set(e.gwID(t, inv), domain.GatewayStatusPaid, 1, nil) // mismatch/underpaid -> log peringatan
	_, _ = e.webhook(t, inv, "PAID")
	e.gw.CreateErr = fmt.Errorf("xendit: HTTP 500")
	e.advance(25 * time.Hour)
	_, _ = e.svc.Checkout(context.Background(), e.admin1, "STARTER", nil)
	logs := e.logbuf.String()
	if strings.Contains(logs, testToken) || strings.Contains(logs, "tebakan-salah") {
		t.Fatalf("log memuat token:\n%s", logs)
	}
	for _, l := range e.act.Logs {
		if s := fmt.Sprint(l.Details); strings.Contains(s, testToken) || strings.Contains(s, "checkout.xendit.co") {
			t.Errorf("audit memuat data sensitif: %s", s)
		}
	}
}

func TestDeletePlan(t *testing.T) {
	e := newEnv(t, true)
	ctx := context.Background()
	if _, err := e.svc.CreatePlan(ctx, e.super, CreatePlanInput{Code: "TEMP", Name: "Sementara", PriceIDR: 1000, PeriodDays: 30, IsActive: true}); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.DeletePlan(ctx, e.admin1, "TEMP"); !isErr(err, domain.ErrForbidden) {
		t.Errorf("ADMIN tidak boleh menghapus paket: %v", err)
	}
	if err := e.svc.DeletePlan(ctx, e.super, "bad code!"); !isErr(err, domain.ErrInvalidInput) {
		t.Errorf("kode jelek harus 400: %v", err)
	}
	if err := e.svc.DeletePlan(ctx, e.super, "TIDAKADA"); !isErr(err, domain.ErrNotFound) {
		t.Errorf("paket tak ada harus 404: %v", err)
	}
	// Paket dengan invoice PENDING tidak boleh dihapus (409), tetapi paket yang tak dipakai boleh.
	e.checkout(t, e.admin1, "STARTER")
	if err := e.svc.DeletePlan(ctx, e.super, "starter"); !isErr(err, domain.ErrConflict) {
		t.Errorf("paket dengan invoice PENDING harus 409: %v", err)
	}
	if err := e.svc.DeletePlan(ctx, e.super, "temp"); err != nil {
		t.Fatalf("hapus paket tak terpakai: %v", err)
	}
	plans, _ := e.svc.ListPlans(ctx, e.super)
	for _, p := range plans {
		if p.Code == "TEMP" {
			t.Error("paket terhapus masih muncul di daftar")
		}
	}
	if err := e.svc.DeletePlan(ctx, e.super, "TEMP"); !isErr(err, domain.ErrNotFound) {
		t.Errorf("hapus dua kali harus 404: %v", err)
	}
}
