package domain

import (
	"context"
	"time"
)

// ---- Pembayaran langganan SaaS (Xendit Invoice API), migrations/0032 ----

// Kode ref_invoice_status.
const (
	InvoicePending = "PENDING"
	InvoicePaid    = "PAID"
	InvoiceExpired = "EXPIRED"
	InvoiceFailed  = "FAILED"
)

// Status invoice di sisi Xendit (Invoice API v2). PAID dan SETTLED sama-sama berarti lunas.
const (
	GatewayStatusPending = "PENDING"
	GatewayStatusPaid    = "PAID"
	GatewayStatusSettled = "SETTLED"
	GatewayStatusExpired = "EXPIRED"
)

// Status langganan tenant yang diturunkan dari tenants.plan_expires_at.
const (
	SubscriptionNone    = "NONE"
	SubscriptionActive  = "ACTIVE"
	SubscriptionExpired = "EXPIRED"
)

// BillingPlan = baris billing_plans. Harga bilangan bulat rupiah. ID numerik tidak pernah diekspos (kode paket
// adalah identifier publik).
type BillingPlan struct {
	ID          uint64  `db:"id" json:"-"`
	Code        string  `db:"plan_code" json:"code"`
	Name        string  `db:"name" json:"name"`
	Description string  `db:"description" json:"description"`
	MaxDevices  *uint32 `db:"max_devices" json:"max_devices"` // nil = tak dibatasi
	PriceIDR    uint64  `db:"price_idr" json:"price_idr"`
	PeriodDays  uint16  `db:"period_days" json:"period_days"`
	IsActive    bool    `db:"is_active" json:"is_active"`
	SortOrder   int     `db:"sort_order" json:"sort_order"`
	Audit       `json:"-"`
}

// BillingInvoice = baris billing_invoices (gabungan dengan kode paket dan kode status). Bentuk JSON-nya adalah
// kontrak API: id = invoice_uuid; PK numerik dan xendit_invoice_id TIDAK pernah diekspos.
type BillingInvoice struct {
	ID              uint64     `db:"id" json:"-"`
	InvoiceUUID     string     `db:"invoice_uuid" json:"id"`
	InvoiceNumber   string     `db:"invoice_number" json:"number"`
	TenantID        uint64     `db:"tenant_id" json:"tenant_id"`
	PlanID          uint64     `db:"plan_id" json:"-"`
	PlanCode        string     `db:"plan_code" json:"plan_code"`
	PlanName        string     `db:"plan_name" json:"plan_name"`
	AmountIDR       uint64     `db:"amount_idr" json:"amount_idr"`
	Status          string     `db:"status" json:"status"` // kode ref_invoice_status
	XenditInvoiceID *string    `db:"xendit_invoice_id" json:"-"`
	PaymentURL      *string    `db:"payment_url" json:"payment_url"` // hanya diisi untuk PENDING (dibersihkan usecase)
	PeriodDays      uint16     `db:"period_days" json:"period_days"`
	CreatedAt       time.Time  `db:"created_at" json:"created_at"`
	ExpiresAt       time.Time  `db:"expires_at" json:"expires_at"`
	PaidAt          *time.Time `db:"paid_at" json:"paid_at"`
	PaidAmountIDR   *uint64    `db:"paid_amount_idr" json:"-"`
	CreatedBy       *uint64    `db:"created_by" json:"-"`
}

// BillingPaymentInput = data untuk menandai invoice lunas secara atomik.
type BillingPaymentInput struct {
	InvoiceID     uint64
	EventKey      string // kunci idempotensi callback (SHA-256 "<xendit_invoice_id>|<status>")
	PaidAt        time.Time
	PaidAmountIDR uint64
	Now           time.Time // dasar perhitungan perpanjangan: max(Now, plan_expires_at saat ini) + period_days
}

// BillingPaymentResult = hasil ApplyPayment.
type BillingPaymentResult struct {
	// Applied = pemanggilan ini yang memenangkan transisi (hanya SATU pemanggil paralel yang true).
	Applied bool
	// Duplicate = event_key yang sama sudah pernah diproses.
	Duplicate bool
	// PlanExpiresAt = masa berlaku baru tenant (hanya bermakna saat Applied).
	PlanExpiresAt time.Time
}

// BillingTenantSubscription = tenant dengan masa berlaku langganan (bahan evaluator kedaluwarsa).
type BillingTenantSubscription struct {
	TenantID      uint64    `db:"tenant_id"`
	TenantName    string    `db:"tenant_name"`
	PlanCode      *string   `db:"plan_code"`
	PlanExpiresAt time.Time `db:"plan_expires_at"`
}

// BillingContact = penerima email tanda terima (ADMIN aktif tenant).
type BillingContact struct {
	Email    string `db:"email"`
	FullName string `db:"full_name"`
}

// BillingRepository -- hanya query dan transisi atomik, tanpa logika keputusan (semua keputusan di usecase/billing).
type BillingRepository interface {
	// ListPlans: activeOnly=true -> hanya is_active. Urut sort_order, id.
	ListPlans(ctx context.Context, activeOnly bool) ([]BillingPlan, error)
	GetPlanByCode(ctx context.Context, code string) (*BillingPlan, error)
	GetPlanByID(ctx context.Context, id uint64) (*BillingPlan, error)
	// CreatePlan: ErrConflict bila kode sudah ada. Mengisi ID.
	CreatePlan(ctx context.Context, p *BillingPlan, createdBy *uint64) error
	// UpdatePlan menimpa kolom yang dapat diubah (nama, deskripsi, batas, harga, periode, aktif, urutan); kode tetap.
	UpdatePlan(ctx context.Context, p *BillingPlan, updatedBy *uint64) error
	// SoftDeletePlan menghapus paket (is_deleted). ErrNotFound bila tak ada; ErrConflict bila masih dipakai tenant atau punya
	// invoice PENDING (pengecekan & penghapusan atomik). Invoice lama tetap menyimpan paketnya sebagai riwayat.
	SoftDeletePlan(ctx context.Context, id uint64, deletedBy uint64) error

	// CreateInvoice menyisipkan invoice berstatus PENDING. Mengisi ID/CreatedAt. ErrConflict bila nomor/uuid ganda.
	CreateInvoice(ctx context.Context, inv *BillingInvoice) error
	GetInvoiceByUUID(ctx context.Context, uuid string) (*BillingInvoice, error)
	GetInvoiceByNumber(ctx context.Context, number string) (*BillingInvoice, error)
	GetInvoiceByXenditID(ctx context.Context, xenditID string) (*BillingInvoice, error)
	// SetInvoiceGateway menyimpan id Xendit + URL bayar + waktu kedaluwarsa dari gateway (hanya selagi PENDING).
	SetInvoiceGateway(ctx context.Context, id uint64, xenditID, paymentURL string, expiresAt time.Time) error
	// ListInvoices: tenantID nil = semua tenant (hanya untuk SUPERADMIN; dijamin pemanggil). Terbaru dulu.
	ListInvoices(ctx context.Context, tenantID *uint64, p Pagination) ([]BillingInvoice, int, error)
	// ListPendingByTenant: invoice PENDING milik tenant yang belum lewat expires_at (terbaru dulu).
	ListPendingByTenant(ctx context.Context, tenantID uint64, now time.Time) ([]BillingInvoice, error)
	// ListPendingExpired: invoice PENDING yang expires_at <= now (maks limit).
	ListPendingExpired(ctx context.Context, now time.Time, limit int) ([]BillingInvoice, error)

	// MarkInvoiceFailed / MarkInvoiceExpired = transisi atomik dari PENDING; true bila pemanggil ini yang mengubahnya.
	MarkInvoiceFailed(ctx context.Context, id uint64) (bool, error)
	MarkInvoiceExpired(ctx context.Context, id uint64) (bool, error)
	// ApplyPayment dalam SATU transaksi: mencatat event (idempotensi), mengubah invoice PENDING/EXPIRED -> PAID,
	// lalu memperpanjang tenant: plan_expires_at = max(Now, plan_expires_at) + period_days, billing_plan_id dan
	// max_devices dari paket invoice. Invoice yang sudah PAID/FAILED atau event ganda -> Applied=false tanpa efek.
	ApplyPayment(ctx context.Context, in BillingPaymentInput) (BillingPaymentResult, error)
	// RecordWebhookEvent mencatat event non-pembayaran (idempoten lewat kunci unik); true bila baru, false bila kunci sudah ada.
	RecordWebhookEvent(ctx context.Context, eventKey string, invoiceID *uint64, outcome string) (bool, error)

	// ListSubscriptionsBetween: tenant aktif dengan plan_expires_at di rentang [from, to].
	ListSubscriptionsBetween(ctx context.Context, from, to time.Time) ([]BillingTenantSubscription, error)
	// ListTenantAdminContacts: ADMIN aktif tenant (untuk email tanda terima), maks limit.
	ListTenantAdminContacts(ctx context.Context, tenantID uint64, limit int) ([]BillingContact, error)
}

// GatewayInvoiceItem = baris rincian invoice (opsional di Xendit).
type GatewayInvoiceItem struct {
	Name     string
	Quantity int
	PriceIDR uint64
}

// GatewayInvoiceInput = permintaan pembuatan invoice ke gateway pembayaran.
type GatewayInvoiceInput struct {
	ExternalID         string
	AmountIDR          uint64
	PayerEmail         string
	Description        string
	DurationSeconds    int
	SuccessRedirectURL string
	FailureRedirectURL string
	Items              []GatewayInvoiceItem
}

// GatewayInvoice = invoice menurut gateway (sumber kebenaran status pembayaran).
type GatewayInvoice struct {
	ID            string
	ExternalID    string
	Status        string // PENDING | PAID | SETTLED | EXPIRED (upper-case)
	InvoiceURL    string
	ExpiryDate    *time.Time
	AmountIDR     uint64
	PaidAmountIDR uint64
	PaidAt        *time.Time
}

// PaymentGateway = antarmuka sempit ke penyedia pembayaran (implementasi: internal/payment/xendit). Galat yang
// dikembalikan sudah disanitasi (tanpa kunci/header) sehingga aman dicatat di log.
type PaymentGateway interface {
	CreateInvoice(ctx context.Context, in GatewayInvoiceInput) (*GatewayInvoice, error)
	GetInvoice(ctx context.Context, id string) (*GatewayInvoice, error)
}
