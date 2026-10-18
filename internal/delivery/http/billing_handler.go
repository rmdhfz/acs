package http

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v5"

	"acs/internal/domain"
	"acs/internal/usecase/billing"
)

// ---- Langganan SaaS (Xendit), migrations/0032 ----
// Handler tipis: parsing + validasi bentuk masukan (panjang, karakter kontrol, UUID, rentang angka, field tak dikenal
// ditolak, body dibatasi); seluruh keputusan (peran, tenant, nominal, status) ada di usecase/billing. SENGAJA tanpa
// denyImpersonation: membuat/memeriksa invoice bukan aksi sensitif (tidak mengubah kredensial/peran); audit tetap memuat
// impersonated_by lewat konteks permintaan.

// decodeStrictJSON membaca satu objek JSON dari body (maks maxBillingBody) tanpa field tak dikenal dan tanpa data
// tambahan. Galat diberi pesan umum (tidak memantulkan isi body).
func decodeStrictJSON(c *echo.Context, dst any) error {
	req := c.Request()
	req.Body = http.MaxBytesReader(c.Response(), req.Body, maxBillingBody)
	dec := json.NewDecoder(req.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			return echo.NewHTTPError(http.StatusRequestEntityTooLarge, "payload terlalu besar")
		}
		return echo.NewHTTPError(http.StatusBadRequest, "payload tidak valid")
	}
	if _, err := dec.Token(); err != io.EOF {
		return echo.NewHTTPError(http.StatusBadRequest, "payload tidak valid")
	}
	return nil
}

// queryTenantID membaca ?tenant_id= secara KETAT: kosong = nil; bukan angka desimal positif -> 400.
func queryTenantID(c *echo.Context) (*uint64, error) {
	v := c.QueryParam("tenant_id")
	if v == "" {
		return nil, nil
	}
	n, err := strconv.ParseUint(v, 10, 64)
	if err != nil || n == 0 || len(v) > 20 {
		return nil, echo.NewHTTPError(http.StatusBadRequest, "tenant_id tidak valid")
	}
	return &n, nil
}

func (r *Router) billingEnabled() bool { return r.Billing != nil && r.Billing.Enabled() }

// mountBillingRoutes memasang rute billing pada grup staf terautentikasi (g) -- tanpa denyImpersonation.
// Webhook Xendit publik dipasang terpisah di Register (di luar grup autentikasi).
func (r *Router) mountBillingRoutes(authed *echo.Group) {
	if r.Billing == nil {
		return
	}
	admin := []string{domain.RoleAdmin, domain.RoleSuperadmin}
	superadminOnly := []string{domain.RoleSuperadmin}
	// Paket & langganan: semua staf (NOC/VIEWER juga). Invoice: ADMIN/SUPERADMIN. ENDUSER ditolak staffOnly.
	authed.GET("/billing/plans", r.billingListPlans)
	authed.POST("/billing/plans", r.billingCreatePlan, RequireRoles(superadminOnly...))
	authed.PATCH("/billing/plans/:code", r.billingUpdatePlan, RequireRoles(superadminOnly...))
	authed.GET("/billing/subscription", r.billingSubscription)
	authed.POST("/billing/checkout", r.billingCheckout, RequireRoles(admin...))
	authed.GET("/billing/invoices", r.billingListInvoices, RequireRoles(admin...))
	authed.POST("/billing/invoices/:uuid/sync", r.billingSyncInvoice, RequireRoles(admin...))
}

// GET /billing/plans
func (r *Router) billingListPlans(c *echo.Context) error {
	plans, err := r.Billing.ListPlans(c.Request().Context(), ActorFrom(c))
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{"data": plans})
}

type createPlanRequest struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description"`
	MaxDevices  *int64 `json:"max_devices"`
	PriceIDR    *int64 `json:"price_idr"`
	PeriodDays  *int64 `json:"period_days"`
	IsActive    *bool  `json:"is_active"`
	SortOrder   *int64 `json:"sort_order"`
}

// POST /billing/plans (SUPERADMIN)
func (r *Router) billingCreatePlan(c *echo.Context) error {
	var req createPlanRequest
	if err := decodeStrictJSON(c, &req); err != nil {
		return err
	}
	if err := checkMaxLen(
		lenRule{"code", req.Code, maxBillingPlanCode},
		lenRule{"name", req.Name, maxBillingPlanName},
		lenRule{"description", req.Description, maxDescription},
	); err != nil {
		return err
	}
	for _, f := range [][2]string{{"code", req.Code}, {"name", req.Name}, {"description", req.Description}} {
		if err := rejectControl(f[0], f[1]); err != nil {
			return err
		}
	}
	if req.PriceIDR == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "price_idr wajib diisi")
	}
	in := billing.CreatePlanInput{
		Code: req.Code, Name: req.Name, Description: req.Description, MaxDevices: req.MaxDevices,
		PriceIDR: *req.PriceIDR, PeriodDays: 30, IsActive: true,
	}
	if req.PeriodDays != nil {
		in.PeriodDays = *req.PeriodDays
	}
	if req.IsActive != nil {
		in.IsActive = *req.IsActive
	}
	if req.SortOrder != nil {
		in.SortOrder = *req.SortOrder
	}
	p, err := r.Billing.CreatePlan(c.Request().Context(), ActorFrom(c), in)
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusCreated, p)
}

type updatePlanRequest struct {
	Name        *string         `json:"name"`
	Description *string         `json:"description"`
	MaxDevices  json.RawMessage `json:"max_devices"` // tidak dikirim = tak berubah; null = tak dibatasi
	PriceIDR    *int64          `json:"price_idr"`
	PeriodDays  *int64          `json:"period_days"`
	IsActive    *bool           `json:"is_active"`
	SortOrder   *int64          `json:"sort_order"`
}

// PATCH /billing/plans/:code (SUPERADMIN)
func (r *Router) billingUpdatePlan(c *echo.Context) error {
	code := c.Param("code")
	if err := checkMaxLen(lenRule{"code", code, maxBillingPlanCode}); err != nil {
		return err
	}
	if err := rejectControl("code", code); err != nil {
		return err
	}
	var req updatePlanRequest
	if err := decodeStrictJSON(c, &req); err != nil {
		return err
	}
	in := billing.UpdatePlanInput{
		Name: req.Name, Description: req.Description, PriceIDR: req.PriceIDR,
		PeriodDays: req.PeriodDays, IsActive: req.IsActive, SortOrder: req.SortOrder,
	}
	if err := checkMaxLen(lenRule{"name", optStr(req.Name), maxBillingPlanName}, lenRule{"description", optStr(req.Description), maxDescription}); err != nil {
		return err
	}
	for _, f := range [][2]string{{"name", optStr(req.Name)}, {"description", optStr(req.Description)}} {
		if err := rejectControl(f[0], f[1]); err != nil {
			return err
		}
	}
	if len(req.MaxDevices) > 0 {
		in.MaxDevices.Set = true
		if string(req.MaxDevices) != "null" {
			var n int64
			if err := json.Unmarshal(req.MaxDevices, &n); err != nil {
				return echo.NewHTTPError(http.StatusBadRequest, "max_devices harus bilangan bulat atau null")
			}
			in.MaxDevices.Value = &n
		}
	}
	p, err := r.Billing.UpdatePlan(c.Request().Context(), ActorFrom(c), code, in)
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, p)
}

// GET /billing/subscription?tenant_id=
func (r *Router) billingSubscription(c *echo.Context) error {
	tid, err := queryTenantID(c)
	if err != nil {
		return err
	}
	v, err := r.Billing.Subscription(c.Request().Context(), ActorFrom(c), tid)
	if err != nil {
		return handleErr(c, err)
	}
	c.Response().Header().Set("Cache-Control", "no-store")
	return c.JSON(http.StatusOK, v)
}

type checkoutRequest struct {
	PlanCode string  `json:"plan_code"`
	TenantID *uint64 `json:"tenant_id"`
}

// POST /billing/checkout
func (r *Router) billingCheckout(c *echo.Context) error {
	var req checkoutRequest
	if err := decodeStrictJSON(c, &req); err != nil {
		return err
	}
	if req.PlanCode == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "plan_code wajib diisi")
	}
	if err := checkMaxLen(lenRule{"plan_code", req.PlanCode, maxBillingPlanCode}); err != nil {
		return err
	}
	if err := rejectControl("plan_code", req.PlanCode); err != nil {
		return err
	}
	if req.TenantID != nil && *req.TenantID == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "tenant_id tidak valid")
	}
	inv, err := r.Billing.Checkout(c.Request().Context(), ActorFrom(c), req.PlanCode, req.TenantID)
	if err != nil {
		return handleErr(c, err)
	}
	c.Response().Header().Set("Cache-Control", "no-store")
	return c.JSON(http.StatusCreated, map[string]any{"invoice": inv, "payment_url": inv.PaymentURL})
}

// GET /billing/invoices?tenant_id=&page=&page_size=
func (r *Router) billingListInvoices(c *echo.Context) error {
	tid, err := queryTenantID(c)
	if err != nil {
		return err
	}
	p := paginationFromQuery(c)
	rows, total, err := r.Billing.ListInvoices(c.Request().Context(), ActorFrom(c), tid, p)
	if err != nil {
		return handleErr(c, err)
	}
	page := p.Page
	if page < 1 {
		page = 1
	}
	c.Response().Header().Set("Cache-Control", "no-store")
	return c.JSON(http.StatusOK, map[string]any{
		"data": rows,
		"meta": map[string]int{"total": total, "page": page, "limit": p.Limit()},
	})
}

// POST /billing/invoices/:uuid/sync
func (r *Router) billingSyncInvoice(c *echo.Context) error {
	id := c.Param("uuid")
	if err := checkMaxLen(lenRule{"uuid", id, maxBillingInvoiceID}); err != nil {
		return err
	}
	inv, err := r.Billing.Sync(c.Request().Context(), ActorFrom(c), id)
	if err != nil {
		return handleErr(c, err)
	}
	c.Response().Header().Set("Cache-Control", "no-store")
	return c.JSON(http.StatusOK, inv)
}

// POST /billing/xendit/webhook -- PUBLIK (tanpa JWT): keamanan = header X-CALLBACK-TOKEN (waktu-konstan) + verifikasi ulang
// status/nominal ke gateway di usecase + pembatas laju + batas body 64 KiB. Billing mati -> 404 (rute seolah tidak ada).
// Respons sukses SELALU seragam (tidak membocorkan invoice dikenal/tidak); 401 token salah; 5xx agar Xendit mengulang.
func (r *Router) xenditWebhook(c *echo.Context) error {
	if !r.billingEnabled() {
		return echo.NewHTTPError(http.StatusNotFound, "not found")
	}
	req := c.Request()
	token := req.Header.Get("X-CALLBACK-TOKEN")
	if len(token) > maxCallbackToken {
		return echo.NewHTTPError(http.StatusUnauthorized, "token callback tidak valid")
	}
	body, err := io.ReadAll(http.MaxBytesReader(c.Response(), req.Body, maxBillingWebhook))
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			return echo.NewHTTPError(http.StatusRequestEntityTooLarge, "payload terlalu besar")
		}
		return echo.NewHTTPError(http.StatusBadRequest, "payload tidak dapat dibaca")
	}
	if _, err := r.Billing.HandleWebhook(req.Context(), token, body); err != nil {
		return handleErr(c, err)
	}
	c.Response().Header().Set("Cache-Control", "no-store")
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}
