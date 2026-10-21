package mysql

import (
	"context"
	"errors"
	"time"

	driver "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"

	"acs/internal/domain"
)

// billingRepository -- penyimpanan langganan SaaS (migrations/0032). Hanya query dan transisi atomik; semua
// keputusan bisnis (siapa boleh, nominal, validasi gateway) ada di usecase/billing. Semua nilai dari luar dikirim
// sebagai parameter terikat -- tidak ada SQL yang dirangkai dari input.
type billingRepository struct{ db *sqlx.DB }

func NewBillingRepository(db *sqlx.DB) domain.BillingRepository { return &billingRepository{db: db} }

// ---- paket ----

const planCols = `id, plan_code, name, description, max_devices, price_idr, period_days, is_active, sort_order,
	created_at, created_by, updated_at, updated_by, deleted_at, deleted_by, is_deleted`

func (r *billingRepository) ListPlans(ctx context.Context, activeOnly bool) ([]domain.BillingPlan, error) {
	q := `SELECT ` + planCols + ` FROM billing_plans WHERE is_deleted = 0`
	if activeOnly {
		q += ` AND is_active = 1`
	}
	q += ` ORDER BY sort_order, id`
	out := []domain.BillingPlan{}
	if err := r.db.SelectContext(ctx, &out, q); err != nil {
		return nil, translateErr(err)
	}
	return out, nil
}

func (r *billingRepository) GetPlanByCode(ctx context.Context, code string) (*domain.BillingPlan, error) {
	var p domain.BillingPlan
	if err := r.db.GetContext(ctx, &p, `SELECT `+planCols+` FROM billing_plans WHERE plan_code = ? AND is_deleted = 0`, code); err != nil {
		return nil, translateErr(err)
	}
	return &p, nil
}

func (r *billingRepository) GetPlanByID(ctx context.Context, id uint64) (*domain.BillingPlan, error) {
	var p domain.BillingPlan
	if err := r.db.GetContext(ctx, &p, `SELECT `+planCols+` FROM billing_plans WHERE id = ? AND is_deleted = 0`, id); err != nil {
		return nil, translateErr(err)
	}
	return &p, nil
}

func (r *billingRepository) CreatePlan(ctx context.Context, p *domain.BillingPlan, createdBy *uint64) error {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO billing_plans (plan_code, name, description, max_devices, price_idr, period_days, is_active, sort_order, created_by)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.Code, p.Name, p.Description, p.MaxDevices, p.PriceIDR, p.PeriodDays, p.IsActive, p.SortOrder, createdBy)
	if err != nil {
		return translateErr(err)
	}
	id, _ := res.LastInsertId()
	p.ID = uint64(id)
	return nil
}

func (r *billingRepository) UpdatePlan(ctx context.Context, p *domain.BillingPlan, updatedBy *uint64) error {
	// Keberadaan dicek terpisah: RowsAffected tidak menghitung baris yang nilainya tidak berubah.
	var exists bool
	if err := r.db.GetContext(ctx, &exists, `SELECT EXISTS(SELECT 1 FROM billing_plans WHERE id = ? AND is_deleted = 0)`, p.ID); err != nil {
		return translateErr(err)
	}
	if !exists {
		return domain.ErrNotFound
	}
	_, err := r.db.ExecContext(ctx,
		`UPDATE billing_plans SET name = ?, description = ?, max_devices = ?, price_idr = ?, period_days = ?,
		        is_active = ?, sort_order = ?, updated_by = ?
		 WHERE id = ? AND is_deleted = 0`,
		p.Name, p.Description, p.MaxDevices, p.PriceIDR, p.PeriodDays, p.IsActive, p.SortOrder, updatedBy, p.ID)
	return translateErr(err)
}

func (r *billingRepository) SoftDeletePlan(ctx context.Context, id, deletedBy uint64) error {
	// Satu UPDATE bersyarat: paket yang masih dipakai tenant / punya invoice PENDING tidak boleh hilang (race-safe).
	res, err := r.db.ExecContext(ctx,
		`UPDATE billing_plans p SET p.is_deleted = 1, p.deleted_at = NOW(), p.deleted_by = ?, p.updated_by = ?
		 WHERE p.id = ? AND p.is_deleted = 0
		   AND NOT EXISTS (SELECT 1 FROM tenants t WHERE t.billing_plan_id = p.id AND t.is_deleted = 0)
		   AND NOT EXISTS (SELECT 1 FROM billing_invoices i JOIN ref_invoice_status s ON s.id = i.status_id
		                    WHERE i.plan_id = p.id AND s.code = 'PENDING')`,
		deletedBy, deletedBy, id)
	if err != nil {
		return translateErr(err)
	}
	if n, _ := res.RowsAffected(); n == 1 {
		return nil
	}
	var exists bool
	if err := r.db.GetContext(ctx, &exists, `SELECT EXISTS(SELECT 1 FROM billing_plans WHERE id = ? AND is_deleted = 0)`, id); err != nil {
		return translateErr(err)
	}
	if !exists {
		return domain.ErrNotFound
	}
	return domain.ErrConflict
}

// ---- invoice ----

const invoiceSelect = `SELECT i.id, i.invoice_uuid, i.invoice_number, i.tenant_id, i.plan_id, p.plan_code, p.name AS plan_name,
	i.amount_idr, s.code AS status, i.xendit_invoice_id, i.payment_url, i.period_days, i.created_at, i.expires_at,
	i.paid_at, i.paid_amount_idr, i.created_by
	FROM billing_invoices i
	JOIN billing_plans p ON p.id = i.plan_id
	JOIN ref_invoice_status s ON s.id = i.status_id`

func (r *billingRepository) CreateInvoice(ctx context.Context, inv *domain.BillingInvoice) error {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO billing_invoices
		   (invoice_uuid, invoice_number, tenant_id, plan_id, amount_idr, status_id, period_days, expires_at, created_by)
		 VALUES (?, ?, ?, ?, ?, (SELECT id FROM ref_invoice_status WHERE code = ?), ?, ?, ?)`,
		inv.InvoiceUUID, inv.InvoiceNumber, inv.TenantID, inv.PlanID, inv.AmountIDR, domain.InvoicePending,
		inv.PeriodDays, inv.ExpiresAt, inv.CreatedBy)
	if err != nil {
		return translateErr(err)
	}
	id, _ := res.LastInsertId()
	inv.ID = uint64(id)
	inv.Status = domain.InvoicePending
	if inv.CreatedAt.IsZero() {
		inv.CreatedAt = time.Now()
	}
	return nil
}

func (r *billingRepository) getInvoice(ctx context.Context, where string, arg any) (*domain.BillingInvoice, error) {
	var inv domain.BillingInvoice
	if err := r.db.GetContext(ctx, &inv, invoiceSelect+` WHERE i.is_deleted = 0 AND `+where, arg); err != nil {
		return nil, translateErr(err)
	}
	return &inv, nil
}

func (r *billingRepository) GetInvoiceByUUID(ctx context.Context, uuid string) (*domain.BillingInvoice, error) {
	return r.getInvoice(ctx, `i.invoice_uuid = ?`, uuid)
}

func (r *billingRepository) GetInvoiceByNumber(ctx context.Context, number string) (*domain.BillingInvoice, error) {
	return r.getInvoice(ctx, `i.invoice_number = ?`, number)
}

func (r *billingRepository) GetInvoiceByXenditID(ctx context.Context, xenditID string) (*domain.BillingInvoice, error) {
	return r.getInvoice(ctx, `i.xendit_invoice_id = ?`, xenditID)
}

func (r *billingRepository) SetInvoiceGateway(ctx context.Context, id uint64, xenditID, paymentURL string, expiresAt time.Time) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE billing_invoices SET xendit_invoice_id = ?, payment_url = ?, expires_at = ?
		 WHERE id = ? AND is_deleted = 0 AND status_id = (SELECT id FROM ref_invoice_status WHERE code = ?)`,
		xenditID, paymentURL, expiresAt, id, domain.InvoicePending)
	if err != nil {
		return translateErr(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *billingRepository) ListInvoices(ctx context.Context, tenantID *uint64, p domain.Pagination) ([]domain.BillingInvoice, int, error) {
	where, args := ` WHERE i.is_deleted = 0`, []any{}
	if tenantID != nil {
		where += ` AND i.tenant_id = ?`
		args = append(args, *tenantID)
	}
	var total int
	if err := r.db.GetContext(ctx, &total, `SELECT COUNT(*) FROM billing_invoices i`+where, args...); err != nil {
		return nil, 0, translateErr(err)
	}
	out := []domain.BillingInvoice{}
	args = append(args, p.Limit(), p.Offset())
	if err := r.db.SelectContext(ctx, &out, invoiceSelect+where+` ORDER BY i.id DESC LIMIT ? OFFSET ?`, args...); err != nil {
		return nil, 0, translateErr(err)
	}
	return out, total, nil
}

func (r *billingRepository) ListPendingByTenant(ctx context.Context, tenantID uint64, now time.Time) ([]domain.BillingInvoice, error) {
	out := []domain.BillingInvoice{}
	if err := r.db.SelectContext(ctx, &out,
		invoiceSelect+` WHERE i.is_deleted = 0 AND i.tenant_id = ? AND s.code = ? AND i.expires_at > ? ORDER BY i.id DESC`,
		tenantID, domain.InvoicePending, now); err != nil {
		return nil, translateErr(err)
	}
	return out, nil
}

func (r *billingRepository) ListPendingExpired(ctx context.Context, now time.Time, limit int) ([]domain.BillingInvoice, error) {
	out := []domain.BillingInvoice{}
	if err := r.db.SelectContext(ctx, &out,
		invoiceSelect+` WHERE i.is_deleted = 0 AND s.code = ? AND i.expires_at <= ? ORDER BY i.id LIMIT ?`,
		domain.InvoicePending, now, limit); err != nil {
		return nil, translateErr(err)
	}
	return out, nil
}

// transition: UPDATE atomik dari PENDING; true bila baris ini yang berubah.
func (r *billingRepository) transition(ctx context.Context, id uint64, to string) (bool, error) {
	res, err := r.db.ExecContext(ctx,
		`UPDATE billing_invoices SET status_id = (SELECT id FROM ref_invoice_status WHERE code = ?)
		 WHERE id = ? AND is_deleted = 0 AND status_id = (SELECT id FROM ref_invoice_status WHERE code = ?)`,
		to, id, domain.InvoicePending)
	if err != nil {
		return false, translateErr(err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

func (r *billingRepository) MarkInvoiceFailed(ctx context.Context, id uint64) (bool, error) {
	return r.transition(ctx, id, domain.InvoiceFailed)
}

func (r *billingRepository) MarkInvoiceExpired(ctx context.Context, id uint64) (bool, error) {
	return r.transition(ctx, id, domain.InvoiceExpired)
}

// isDeadlock: galat 1213 (deadlock) / 1205 (lock wait timeout) -- aman diulang karena transaksi sudah di-rollback.
func isDeadlock(err error) bool {
	var me *driver.MySQLError
	return errors.As(err, &me) && (me.Number == 1213 || me.Number == 1205)
}

// ApplyPayment menjalankan applyPaymentOnce dan mengulangnya (maks 3 kali) bila terkena deadlock/lock timeout.
func (r *billingRepository) ApplyPayment(ctx context.Context, in domain.BillingPaymentInput) (domain.BillingPaymentResult, error) {
	var (
		res domain.BillingPaymentResult
		err error
	)
	for attempt := 0; attempt < 3; attempt++ {
		res, err = r.applyPaymentOnce(ctx, in)
		if err == nil || !isDeadlock(err) {
			return res, err
		}
	}
	return res, err
}

// applyPaymentOnce = satu transaksi: (0) kunci baris invoice (SELECT ... FOR UPDATE) -- menserialkan semua pemanggil
// pada invoice yang sama SEBELUM menyentuh tabel lain (tanpa ini, cek FK event menahan lock-bersama pada baris invoice
// lalu UPDATE meminta lock-eksklusif -> deadlock); (1) catat event -- kunci unik membuat callback ganda hanya satu yang
// lanjut (yang lain mendapat duplikat); (2) ubah invoice PENDING/EXPIRED -> PAID (EXPIRED ikut diterima karena gateway
// yang berwenang: pembayaran bisa masuk tepat di batas waktu); (3) perpanjang tenant. Bila langkah 2 tidak mengubah baris
// (sudah PAID/FAILED), seluruh transaksi di-rollback tanpa efek.
func (r *billingRepository) applyPaymentOnce(ctx context.Context, in domain.BillingPaymentInput) (domain.BillingPaymentResult, error) {
	var out domain.BillingPaymentResult
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return out, translateErr(err)
	}
	defer func() { _ = tx.Rollback() }() // no-op setelah Commit

	var locked uint64
	if err := tx.GetContext(ctx, &locked, `SELECT id FROM billing_invoices WHERE id = ? AND is_deleted = 0 FOR UPDATE`, in.InvoiceID); err != nil {
		return out, translateErr(err)
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO billing_webhook_events (event_key, invoice_id, outcome, received_at) VALUES (?, ?, ?, ?)`,
		in.EventKey, in.InvoiceID, domain.InvoicePaid, in.Now); err != nil {
		if te := translateErr(err); te == domain.ErrConflict {
			out.Duplicate = true
			return out, nil
		}
		return out, translateErr(err)
	}

	res, err := tx.ExecContext(ctx,
		`UPDATE billing_invoices SET status_id = (SELECT id FROM ref_invoice_status WHERE code = ?), paid_at = ?, paid_amount_idr = ?
		 WHERE id = ? AND is_deleted = 0
		   AND status_id IN (SELECT id FROM ref_invoice_status WHERE code IN (?, ?))`,
		domain.InvoicePaid, in.PaidAt, in.PaidAmountIDR, in.InvoiceID, domain.InvoicePending, domain.InvoiceExpired)
	if err != nil {
		return out, translateErr(err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return out, nil // sudah lunas / gagal: tidak ada efek (transaksi di-rollback, event tidak tercatat)
	}

	var inv struct {
		TenantID   uint64 `db:"tenant_id"`
		PlanID     uint64 `db:"plan_id"`
		PeriodDays uint16 `db:"period_days"`
	}
	if err := tx.GetContext(ctx, &inv, `SELECT tenant_id, plan_id, period_days FROM billing_invoices WHERE id = ?`, in.InvoiceID); err != nil {
		return out, translateErr(err)
	}
	// Perpanjangan bertumpuk: dasar = yang lebih lama antara sekarang dan masa berlaku saat ini.
	res, err = tx.ExecContext(ctx,
		`UPDATE tenants SET billing_plan_id = ?,
		        max_devices = (SELECT max_devices FROM billing_plans WHERE id = ?),
		        plan_expires_at = DATE_ADD(GREATEST(COALESCE(plan_expires_at, ?), ?), INTERVAL ? DAY)
		 WHERE id = ? AND is_deleted = 0`,
		inv.PlanID, inv.PlanID, in.Now, in.Now, int(inv.PeriodDays), inv.TenantID)
	if err != nil {
		return out, translateErr(err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return out, domain.ErrNotFound // tenant sudah dihapus: rollback, pembayaran perlu penanganan manual
	}
	if err := tx.GetContext(ctx, &out.PlanExpiresAt, `SELECT plan_expires_at FROM tenants WHERE id = ?`, inv.TenantID); err != nil {
		return out, translateErr(err)
	}
	if err := tx.Commit(); err != nil {
		return domain.BillingPaymentResult{}, translateErr(err)
	}
	out.Applied = true
	return out, nil
}

func (r *billingRepository) RecordWebhookEvent(ctx context.Context, eventKey string, invoiceID *uint64, outcome string) (bool, error) {
	// INSERT biasa (bukan IGNORE) supaya pelanggaran FK tetap menjadi galat; kunci ganda -> false.
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO billing_webhook_events (event_key, invoice_id, outcome) VALUES (?, ?, ?)`,
		eventKey, invoiceID, outcome)
	if err != nil {
		if te := translateErr(err); te == domain.ErrConflict {
			return false, nil
		}
		return false, translateErr(err)
	}
	return true, nil
}

// ---- langganan tenant ----

func (r *billingRepository) ListSubscriptionsBetween(ctx context.Context, from, to time.Time) ([]domain.BillingTenantSubscription, error) {
	out := []domain.BillingTenantSubscription{}
	if err := r.db.SelectContext(ctx, &out,
		`SELECT t.id AS tenant_id, t.name AS tenant_name, p.plan_code, t.plan_expires_at
		 FROM tenants t LEFT JOIN billing_plans p ON p.id = t.billing_plan_id
		 WHERE t.is_deleted = 0 AND t.is_active = 1 AND t.plan_expires_at IS NOT NULL
		   AND t.plan_expires_at BETWEEN ? AND ?
		 ORDER BY t.id`, from, to); err != nil {
		return nil, translateErr(err)
	}
	return out, nil
}

func (r *billingRepository) ListTenantAdminContacts(ctx context.Context, tenantID uint64, limit int) ([]domain.BillingContact, error) {
	out := []domain.BillingContact{}
	if err := r.db.SelectContext(ctx, &out,
		`SELECT DISTINCT u.email, u.full_name FROM users u
		 JOIN user_roles ur ON ur.user_id = u.id
		 JOIN ref_roles rr ON rr.id = ur.role_id AND rr.is_deleted = 0 AND rr.code = ?
		 WHERE u.tenant_id = ? AND u.is_deleted = 0 AND u.is_active = 1 AND u.email <> ''
		 ORDER BY u.email LIMIT ?`, domain.RoleAdmin, tenantID, limit); err != nil {
		return nil, translateErr(err)
	}
	return out, nil
}
