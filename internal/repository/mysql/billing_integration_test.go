package mysql

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"acs/internal/domain"
)

// Integrasi MariaDB NYATA (migrations/0032 harus sudah diterapkan): CRUD paket, siklus invoice, FK/unique, transisi
// status atomik, idempotensi event, perpanjangan tenant, dan balapan 20 goroutine (hanya satu pemenang).
// Dilewati tanpa ACS_TEST_DSN.

type billingFixture struct {
	db    *sqlx.DB
	repo  domain.BillingRepository
	sfx   string
	tid   uint64
	plan  *domain.BillingPlan
	clean []func()
}

func newBillingFixture(t *testing.T) *billingFixture {
	t.Helper()
	dsn := os.Getenv("ACS_TEST_DSN")
	if dsn == "" {
		t.Skip("ACS_TEST_DSN tidak diisi — test integrasi MariaDB dilewati")
	}
	db, err := Connect(dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	f := &billingFixture{db: db, repo: NewBillingRepository(db), sfx: uuid.NewString()[:8]}
	ctx := context.Background()
	res, err := db.ExecContext(ctx, `INSERT INTO tenants (tenant_uuid, code, name) VALUES (?, ?, ?)`, uuid.NewString(), "it-b-"+f.sfx, "IT Billing "+f.sfx)
	if err != nil {
		t.Fatalf("tenant: %v", err)
	}
	id, _ := res.LastInsertId()
	f.tid = uint64(id)
	maxDev := uint32(77)
	f.plan = &domain.BillingPlan{Code: "IT_" + f.sfx, Name: "IT Plan", Description: "uji integrasi", MaxDevices: &maxDev, PriceIDR: 123000, PeriodDays: 30, IsActive: true, SortOrder: 9000}
	if err := f.repo.CreatePlan(ctx, f.plan, nil); err != nil {
		t.Fatalf("plan: %v", err)
	}
	t.Cleanup(func() {
		for _, c := range f.clean {
			c()
		}
		_, _ = db.Exec(`DELETE e FROM billing_webhook_events e JOIN billing_invoices i ON i.id = e.invoice_id WHERE i.tenant_id = ?`, f.tid)
		_, _ = db.Exec(`DELETE FROM billing_invoices WHERE tenant_id = ?`, f.tid)
		_, _ = db.Exec(`UPDATE tenants SET billing_plan_id = NULL WHERE id = ?`, f.tid)
		_, _ = db.Exec(`DELETE FROM billing_plans WHERE plan_code LIKE ?`, "%"+f.sfx)
		_, _ = db.Exec(`DELETE FROM tenants WHERE id = ?`, f.tid)
		db.Close()
	})
	return f
}

func (f *billingFixture) newInvoice(t *testing.T, tag string) *domain.BillingInvoice {
	t.Helper()
	inv := &domain.BillingInvoice{
		InvoiceUUID: uuid.NewString(), InvoiceNumber: fmt.Sprintf("ACS-IT-%s-%s", f.sfx, tag),
		TenantID: f.tid, PlanID: f.plan.ID, AmountIDR: f.plan.PriceIDR, PeriodDays: f.plan.PeriodDays,
		ExpiresAt: time.Now().Add(24 * time.Hour).Truncate(time.Second),
	}
	if err := f.repo.CreateInvoice(context.Background(), inv); err != nil {
		t.Fatalf("CreateInvoice %s: %v", tag, err)
	}
	return inv
}

func evKey(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func (f *billingFixture) tenantExpiry(t *testing.T) (*time.Time, *uint32, *uint64) {
	t.Helper()
	var row struct {
		Exp  *time.Time `db:"plan_expires_at"`
		Max  *uint32    `db:"max_devices"`
		Plan *uint64    `db:"billing_plan_id"`
	}
	if err := f.db.Get(&row, `SELECT plan_expires_at, max_devices, billing_plan_id FROM tenants WHERE id = ?`, f.tid); err != nil {
		t.Fatal(err)
	}
	return row.Exp, row.Max, row.Plan
}

func TestBillingRepository_Plans_Integration(t *testing.T) {
	f := newBillingFixture(t)
	ctx := context.Background()

	dup := *f.plan
	dup.ID = 0
	if err := f.repo.CreatePlan(ctx, &dup, nil); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("kode ganda harus ErrConflict: %v", err)
	}
	got, err := f.repo.GetPlanByCode(ctx, f.plan.Code)
	if err != nil || got.PriceIDR != 123000 || got.MaxDevices == nil || *got.MaxDevices != 77 || got.PeriodDays != 30 {
		t.Fatalf("GetPlanByCode: %+v %v", got, err)
	}
	if _, err := f.repo.GetPlanByCode(ctx, "TIDAK_ADA_"+f.sfx); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("paket tak ada: %v", err)
	}
	got.MaxDevices = nil // tak dibatasi
	got.PriceIDR = 456000
	got.IsActive = false
	if err := f.repo.UpdatePlan(ctx, got, nil); err != nil {
		t.Fatal(err)
	}
	after, _ := f.repo.GetPlanByID(ctx, f.plan.ID)
	if after.MaxDevices != nil || after.PriceIDR != 456000 || after.IsActive {
		t.Fatalf("UpdatePlan: %+v", after)
	}
	// tanpa perubahan nilai tetap sukses (RowsAffected 0 bukan galat)
	if err := f.repo.UpdatePlan(ctx, after, nil); err != nil {
		t.Fatalf("update tanpa perubahan: %v", err)
	}
	missing := *after
	missing.ID = 999999999
	if err := f.repo.UpdatePlan(ctx, &missing, nil); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("update paket tak ada: %v", err)
	}
	active, _ := f.repo.ListPlans(ctx, true)
	all, _ := f.repo.ListPlans(ctx, false)
	hasPlan := func(list []domain.BillingPlan) bool {
		for _, p := range list {
			if p.ID == f.plan.ID {
				return true
			}
		}
		return false
	}
	if hasPlan(active) || !hasPlan(all) {
		t.Fatalf("activeOnly salah: aktif=%v semua=%v", hasPlan(active), hasPlan(all))
	}
	// seed contoh dari migrasi
	seed, err := f.repo.GetPlanByCode(ctx, "ENTERPRISE")
	if err != nil || seed.MaxDevices != nil || seed.PriceIDR != 2499000 {
		t.Fatalf("seed ENTERPRISE: %+v %v", seed, err)
	}
}

func TestBillingRepository_Invoices_Integration(t *testing.T) {
	f := newBillingFixture(t)
	ctx := context.Background()
	a := f.newInvoice(t, "a")

	// unique nomor & uuid
	d := *a
	d.InvoiceUUID = uuid.NewString()
	if err := f.repo.CreateInvoice(ctx, &d); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("nomor ganda harus ErrConflict: %v", err)
	}
	// FK tenant & paket
	bad := &domain.BillingInvoice{InvoiceUUID: uuid.NewString(), InvoiceNumber: "ACS-IT-" + f.sfx + "-fk1", TenantID: 999999999, PlanID: f.plan.ID, AmountIDR: 1, PeriodDays: 30, ExpiresAt: time.Now().Add(time.Hour)}
	if err := f.repo.CreateInvoice(ctx, bad); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("FK tenant: %v", err)
	}
	bad2 := &domain.BillingInvoice{InvoiceUUID: uuid.NewString(), InvoiceNumber: "ACS-IT-" + f.sfx + "-fk2", TenantID: f.tid, PlanID: 999999999, AmountIDR: 1, PeriodDays: 30, ExpiresAt: time.Now().Add(time.Hour)}
	if err := f.repo.CreateInvoice(ctx, bad2); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("FK paket: %v", err)
	}

	got, err := f.repo.GetInvoiceByUUID(ctx, a.InvoiceUUID)
	if err != nil || got.Status != domain.InvoicePending || got.PlanCode != f.plan.Code || got.AmountIDR != 123000 || got.XenditInvoiceID != nil || got.PaymentURL != nil {
		t.Fatalf("GetInvoiceByUUID: %+v %v", got, err)
	}
	if byNum, err := f.repo.GetInvoiceByNumber(ctx, a.InvoiceNumber); err != nil || byNum.ID != a.ID {
		t.Fatalf("GetInvoiceByNumber: %v", err)
	}
	if _, err := f.repo.GetInvoiceByNumber(ctx, "tidak-ada"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("nomor tak ada: %v", err)
	}

	xid := "xnd_it_" + f.sfx
	exp := time.Now().Add(48 * time.Hour).Truncate(time.Second)
	if err := f.repo.SetInvoiceGateway(ctx, a.ID, xid, "https://checkout.xendit.co/web/"+xid, exp); err != nil {
		t.Fatal(err)
	}
	byX, err := f.repo.GetInvoiceByXenditID(ctx, xid)
	if err != nil || byX.ID != a.ID || byX.PaymentURL == nil || !byX.ExpiresAt.Equal(exp) {
		t.Fatalf("GetInvoiceByXenditID: %+v %v", byX, err)
	}
	// id Xendit unik
	b := f.newInvoice(t, "b")
	if err := f.repo.SetInvoiceGateway(ctx, b.ID, xid, "https://checkout.xendit.co/web/x", exp); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("xendit_invoice_id ganda harus ErrConflict: %v", err)
	}

	pend, err := f.repo.ListPendingByTenant(ctx, f.tid, time.Now())
	if err != nil || len(pend) != 2 {
		t.Fatalf("ListPendingByTenant: %d %v", len(pend), err)
	}
	if pend[0].ID < pend[1].ID {
		t.Error("urutan harus terbaru dulu")
	}
	rows, total, err := f.repo.ListInvoices(ctx, &f.tid, domain.Pagination{Page: 1, PageSize: 1})
	if err != nil || total != 2 || len(rows) != 1 {
		t.Fatalf("ListInvoices: %d %d %v", total, len(rows), err)
	}
	if _, total, _ = f.repo.ListInvoices(ctx, nil, domain.Pagination{}); total < 2 {
		t.Fatalf("ListInvoices semua tenant: %d", total)
	}

	// transisi atomik dari PENDING
	if ok, err := f.repo.MarkInvoiceFailed(ctx, b.ID); err != nil || !ok {
		t.Fatalf("PENDING->FAILED: %v %v", ok, err)
	}
	if ok, _ := f.repo.MarkInvoiceFailed(ctx, b.ID); ok {
		t.Fatal("FAILED->FAILED harus false")
	}
	if ok, _ := f.repo.MarkInvoiceExpired(ctx, b.ID); ok {
		t.Fatal("FAILED->EXPIRED harus false")
	}
	if err := f.repo.SetInvoiceGateway(ctx, b.ID, "xnd_x_"+f.sfx, "https://checkout.xendit.co/x", exp); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("SetInvoiceGateway pada non-PENDING harus ditolak: %v", err)
	}
	// kedaluwarsa
	c := f.newInvoice(t, "c")
	if _, err := f.db.Exec(`UPDATE billing_invoices SET expires_at = ? WHERE id = ?`, time.Now().Add(-time.Hour), c.ID); err != nil {
		t.Fatal(err)
	}
	due, err := f.repo.ListPendingExpired(ctx, time.Now(), 100)
	found := false
	for _, i := range due {
		if i.ID == c.ID {
			found = true
		}
		if i.Status != domain.InvoicePending {
			t.Errorf("ListPendingExpired hanya PENDING: %s", i.Status)
		}
	}
	if err != nil || !found {
		t.Fatalf("ListPendingExpired: %v %v", found, err)
	}
	if ok, _ := f.repo.MarkInvoiceExpired(ctx, c.ID); !ok {
		t.Fatal("PENDING->EXPIRED")
	}
	if ok, _ := f.repo.MarkInvoiceExpired(ctx, c.ID); ok {
		t.Fatal("EXPIRED->EXPIRED harus false")
	}

	// idempotensi event non-pembayaran
	k := evKey("ev-" + f.sfx)
	if first, err := f.repo.RecordWebhookEvent(ctx, k, &c.ID, "EXPIRED"); err != nil || !first {
		t.Fatalf("event pertama: %v %v", first, err)
	}
	if first, err := f.repo.RecordWebhookEvent(ctx, k, &c.ID, "EXPIRED"); err != nil || first {
		t.Fatalf("event kedua harus false: %v %v", first, err)
	}
	// FK event -> invoice
	nonexist := uint64(999999999)
	if _, err := f.repo.RecordWebhookEvent(ctx, evKey("fk-"+f.sfx), &nonexist, "X"); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("FK event: %v", err)
	}
}

func TestBillingRepository_ApplyPayment_Integration(t *testing.T) {
	f := newBillingFixture(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	// Langganan lama masih 10 hari: perpanjangan menumpuk dari expiry lama.
	oldExp := now.Add(10 * 24 * time.Hour)
	if _, err := f.db.Exec(`UPDATE tenants SET plan_expires_at = ?, max_devices = 5 WHERE id = ?`, oldExp, f.tid); err != nil {
		t.Fatal(err)
	}
	a := f.newInvoice(t, "pay-a")
	in := domain.BillingPaymentInput{InvoiceID: a.ID, EventKey: evKey("a|PAID"), PaidAt: now, PaidAmountIDR: a.AmountIDR, Now: now}
	res, err := f.repo.ApplyPayment(ctx, in)
	if err != nil || !res.Applied || res.Duplicate {
		t.Fatalf("ApplyPayment: %+v %v", res, err)
	}
	wantExp := oldExp.AddDate(0, 0, 30)
	if d := res.PlanExpiresAt.Sub(wantExp); d < -time.Second || d > time.Second {
		t.Fatalf("expiry = %v, mau %v", res.PlanExpiresAt, wantExp)
	}
	exp, max, plan := f.tenantExpiry(t)
	if exp == nil || exp.Sub(wantExp).Abs() > time.Second || max == nil || *max != 77 || plan == nil || *plan != f.plan.ID {
		t.Fatalf("tenant: exp=%v max=%v plan=%v", exp, max, plan)
	}
	inv, _ := f.repo.GetInvoiceByUUID(ctx, a.InvoiceUUID)
	if inv.Status != domain.InvoicePaid || inv.PaidAt == nil || inv.PaidAmountIDR == nil || *inv.PaidAmountIDR != a.AmountIDR {
		t.Fatalf("invoice: %+v", inv)
	}

	// event sama -> Duplicate tanpa efek
	res, err = f.repo.ApplyPayment(ctx, in)
	if err != nil || res.Applied || !res.Duplicate {
		t.Fatalf("duplikat: %+v %v", res, err)
	}
	// event BERBEDA pada invoice yang sudah PAID -> tidak diterapkan, event tidak tercatat, expiry tetap
	in2 := in
	in2.EventKey = evKey("a|SETTLED")
	res, err = f.repo.ApplyPayment(ctx, in2)
	if err != nil || res.Applied || res.Duplicate {
		t.Fatalf("event lain pada invoice lunas: %+v %v", res, err)
	}
	var n int
	_ = f.db.Get(&n, `SELECT COUNT(*) FROM billing_webhook_events WHERE event_key = ?`, in2.EventKey)
	if n != 0 {
		t.Error("event yang tidak diterapkan tidak boleh tercatat (rollback)")
	}
	if exp2, _, _ := f.tenantExpiry(t); exp2.Sub(wantExp).Abs() > time.Second {
		t.Fatalf("expiry berubah: %v", exp2)
	}

	// EXPIRED -> PAID diizinkan (gateway berwenang)
	e := f.newInvoice(t, "pay-e")
	if ok, _ := f.repo.MarkInvoiceExpired(ctx, e.ID); !ok {
		t.Fatal("expire")
	}
	res, err = f.repo.ApplyPayment(ctx, domain.BillingPaymentInput{InvoiceID: e.ID, EventKey: evKey("e|PAID"), PaidAt: now, PaidAmountIDR: e.AmountIDR, Now: now})
	if err != nil || !res.Applied {
		t.Fatalf("EXPIRED->PAID: %+v %v", res, err)
	}
	// FAILED tidak boleh dilunasi
	fl := f.newInvoice(t, "pay-f")
	_, _ = f.repo.MarkInvoiceFailed(ctx, fl.ID)
	res, err = f.repo.ApplyPayment(ctx, domain.BillingPaymentInput{InvoiceID: fl.ID, EventKey: evKey("f|PAID"), PaidAt: now, PaidAmountIDR: fl.AmountIDR, Now: now})
	if err != nil || res.Applied {
		t.Fatalf("FAILED tidak boleh lunas: %+v %v", res, err)
	}

	// Langganan sudah lapse -> dasar = Now
	if _, err := f.db.Exec(`UPDATE tenants SET plan_expires_at = ? WHERE id = ?`, now.Add(-48*time.Hour), f.tid); err != nil {
		t.Fatal(err)
	}
	l := f.newInvoice(t, "pay-l")
	res, err = f.repo.ApplyPayment(ctx, domain.BillingPaymentInput{InvoiceID: l.ID, EventKey: evKey("l|PAID"), PaidAt: now, PaidAmountIDR: l.AmountIDR, Now: now})
	if err != nil || !res.Applied || res.PlanExpiresAt.Sub(now.AddDate(0, 0, 30)).Abs() > time.Second {
		t.Fatalf("lapse: %+v %v", res, err)
	}

	// Paket tak dibatasi (NULL) menimpa max_devices menjadi NULL
	f.plan.MaxDevices = nil
	if err := f.repo.UpdatePlan(ctx, f.plan, nil); err != nil {
		t.Fatal(err)
	}
	u := f.newInvoice(t, "pay-u")
	if res, err = f.repo.ApplyPayment(ctx, domain.BillingPaymentInput{InvoiceID: u.ID, EventKey: evKey("u|PAID"), PaidAt: now, PaidAmountIDR: u.AmountIDR, Now: now}); err != nil || !res.Applied {
		t.Fatalf("%+v %v", res, err)
	}
	if _, max, _ := f.tenantExpiry(t); max != nil {
		t.Fatalf("max_devices harus NULL (tak dibatasi): %d", *max)
	}

	// Pembacaan tenant lewat TenantRepository (SELECT *) memuat kolom baru.
	tn, err := NewTenantRepository(f.db).GetByID(ctx, f.tid)
	if err != nil || tn.BillingPlanID == nil || tn.PlanExpiresAt == nil {
		t.Fatalf("TenantRepository.GetByID: %+v %v", tn, err)
	}

	// Evaluator: rentang langganan + kontak ADMIN
	subs, err := f.repo.ListSubscriptionsBetween(ctx, now, now.AddDate(0, 0, 60))
	hit := false
	for _, s := range subs {
		if s.TenantID == f.tid && s.PlanCode != nil && *s.PlanCode == f.plan.Code {
			hit = true
		}
	}
	if err != nil || !hit {
		t.Fatalf("ListSubscriptionsBetween: %v %v", hit, err)
	}
	if _, err := f.db.Exec(`UPDATE tenants SET is_active = 0 WHERE id = ?`, f.tid); err != nil {
		t.Fatal(err)
	}
	subs, _ = f.repo.ListSubscriptionsBetween(ctx, now, now.AddDate(0, 0, 60))
	for _, s := range subs {
		if s.TenantID == f.tid {
			t.Error("tenant nonaktif tidak boleh dievaluasi")
		}
	}
}

func TestBillingRepository_ParallelSingleWinner_Integration(t *testing.T) {
	f := newBillingFixture(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	run := func(name string, keyFor func(i int) string) {
		inv := f.newInvoice(t, name)
		before, _, _ := f.tenantExpiry(t)
		var wg sync.WaitGroup
		var mu sync.Mutex
		applied, dup, none, failed := 0, 0, 0, 0
		start := make(chan struct{})
		for i := 0; i < 20; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-start
				res, err := f.repo.ApplyPayment(ctx, domain.BillingPaymentInput{InvoiceID: inv.ID, EventKey: keyFor(i), PaidAt: now, PaidAmountIDR: inv.AmountIDR, Now: now})
				mu.Lock()
				defer mu.Unlock()
				switch {
				case err != nil:
					failed++
					t.Errorf("%s: galat: %v", name, err)
				case res.Applied:
					applied++
				case res.Duplicate:
					dup++
				default:
					none++
				}
			}(i)
		}
		close(start)
		wg.Wait()
		if applied != 1 || failed != 0 {
			t.Fatalf("%s: harus tepat 1 pemenang: applied=%d dup=%d none=%d failed=%d", name, applied, dup, none, failed)
		}
		after, _, _ := f.tenantExpiry(t)
		base := now
		if before != nil && before.After(now) {
			base = *before
		}
		if want := base.AddDate(0, 0, 30); after == nil || after.Sub(want).Abs() > time.Second {
			t.Fatalf("%s: perpanjangan harus tepat satu periode: %v, mau %v", name, after, want)
		}
	}
	run("same-key", func(int) string { return evKey("same-" + f.sfx) })
	run("distinct-keys", func(i int) string { return evKey(fmt.Sprintf("distinct-%s-%d", f.sfx, i)) })

	// Transisi PENDING->EXPIRED paralel: satu pemenang.
	inv := f.newInvoice(t, "exp-race")
	var wg sync.WaitGroup
	var mu sync.Mutex
	wins := 0
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := f.repo.MarkInvoiceExpired(ctx, inv.ID)
			if err != nil {
				t.Errorf("galat: %v", err)
			}
			if ok {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if wins != 1 {
		t.Fatalf("PENDING->EXPIRED paralel harus 1 pemenang: %d", wins)
	}
}

// SoftDeletePlan: paket yang masih dipakai (invoice PENDING / tenant) ditolak 409, yang bebas terhapus, ID tak ada 404.
func TestBillingRepository_SoftDeletePlan_Integration(t *testing.T) {
	f := newBillingFixture(t)
	ctx := context.Background()

	// invoice PENDING pada paket fixture -> konflik
	f.newInvoice(t, "del")
	if err := f.repo.SoftDeletePlan(ctx, f.plan.ID, 1); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("paket dengan invoice PENDING harus ErrConflict: %v", err)
	}

	// tenant memakai paket -> konflik, walau invoice sudah tak pending
	if _, err := f.db.Exec(`UPDATE billing_invoices SET status_id = (SELECT id FROM ref_invoice_status WHERE code = 'EXPIRED') WHERE tenant_id = ?`, f.tid); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`UPDATE tenants SET billing_plan_id = ? WHERE id = ?`, f.plan.ID, f.tid); err != nil {
		t.Fatal(err)
	}
	if err := f.repo.SoftDeletePlan(ctx, f.plan.ID, 1); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("paket yang dipakai tenant harus ErrConflict: %v", err)
	}
	if _, err := f.db.Exec(`UPDATE tenants SET billing_plan_id = NULL WHERE id = ?`, f.tid); err != nil {
		t.Fatal(err)
	}

	// bebas dipakai (invoice kedaluwarsa tetap jadi riwayat) -> terhapus, hilang dari daftar & GetPlanByCode
	if err := f.repo.SoftDeletePlan(ctx, f.plan.ID, 1); err != nil {
		t.Fatalf("hapus paket bebas: %v", err)
	}
	if _, err := f.repo.GetPlanByCode(ctx, f.plan.Code); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("paket terhapus harus ErrNotFound: %v", err)
	}
	plans, _ := f.repo.ListPlans(ctx, false)
	for _, p := range plans {
		if p.Code == f.plan.Code {
			t.Fatal("paket terhapus masih di daftar")
		}
	}
	if err := f.repo.SoftDeletePlan(ctx, f.plan.ID, 1); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("hapus ulang harus ErrNotFound: %v", err)
	}
	// riwayat invoice tetap terbaca dengan nama paketnya
	invs, _, err := f.repo.ListInvoices(ctx, &f.tid, domain.Pagination{Page: 1, PageSize: 10})
	if err != nil || len(invs) != 1 || invs[0].PlanName != "IT Plan" {
		t.Fatalf("riwayat invoice harus utuh setelah paket dihapus: %+v err=%v", invs, err)
	}
}
