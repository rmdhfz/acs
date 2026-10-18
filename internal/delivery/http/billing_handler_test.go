package http

import (
	"encoding/json"
	"fmt"
	nethttp "net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v5"

	"acs/internal/domain"
	"acs/internal/usecase/billing"
	"acs/internal/usecase/billing/billingtest"
)

const hTestToken = "handler-token-0123456789-ABCDEFGH"

func hu64(v uint64) *uint64 { return &v }
func hu32(v uint32) *uint32 { return &v }

type billingHarness struct {
	e      *echo.Echo
	repo   *billingtest.Repo
	gw     *billingtest.Gateway
	actors map[string]domain.Actor
}

func newBillingHarness(t *testing.T, enabled bool) *billingHarness {
	t.Helper()
	repo := billingtest.NewRepo()
	gw := billingtest.NewGateway()
	repo.AddPlan("STARTER", hu32(50), 199000, 30, true)
	repo.AddPlan("OLD", hu32(5), 1000, 30, false)
	repo.AddTenant(1, "Satu", true)
	repo.AddTenant(2, "Dua", true)
	repo.AddUser(10, hu64(1), "admin1@satu.id")
	repo.AddUser(20, hu64(2), "admin2@dua.id")
	repo.AddUser(99, nil, "root@platform.id")
	svc := billing.NewService(repo, repo, billingtest.Users{R: repo}, gw, nil, billing.Config{
		Enabled: enabled, CallbackToken: hTestToken, Invoice: 24 * time.Hour, PublicURL: "https://acs.example.com",
	}, nil)
	r := &Router{Billing: svc}

	h := &billingHarness{e: echo.New(), repo: repo, gw: gw, actors: map[string]domain.Actor{
		"admin1":    {UserID: 10, TenantID: hu64(1), Roles: []string{domain.RoleAdmin}},
		"admin2":    {UserID: 20, TenantID: hu64(2), Roles: []string{domain.RoleAdmin}},
		"noc1":      {UserID: 11, TenantID: hu64(1), Roles: []string{domain.RoleNOC}},
		"viewer1":   {UserID: 12, TenantID: hu64(1), Roles: []string{domain.RoleViewer}},
		"enduser1":  {UserID: 13, TenantID: hu64(1), Roles: []string{domain.RoleEndUser}},
		"super":     {UserID: 99, Roles: []string{domain.RoleSuperadmin}},
		"impadmin1": {UserID: 10, TenantID: hu64(1), Roles: []string{domain.RoleAdmin}, ImpersonatorID: 99},
	}}
	inject := func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			who := c.Request().Header.Get("X-Test-Actor")
			a, ok := h.actors[who]
			if !ok {
				return echo.NewHTTPError(nethttp.StatusUnauthorized, "missing bearer token")
			}
			c.Set(actorContextKey, a)
			return next(c)
		}
	}
	// Meniru grup `authed` di Register: autentikasi + staffOnly. Rute billing dipasang oleh fungsi produksi yang sama.
	g := h.e.Group("/api/v1", inject, staffOnly)
	r.mountBillingRoutes(g)
	// Kontrol: rute dengan denyImpersonation HARUS menolak sesi impersonate (membuktikan harness benar-benar menguji itu).
	g.POST("/control/sensitive", func(c *echo.Context) error { return c.NoContent(nethttp.StatusNoContent) }, denyImpersonation)
	// Webhook publik persis seperti di Register (tanpa grup autentikasi).
	h.e.POST("/api/v1/billing/xendit/webhook", r.xenditWebhook, r.limiter("billing-webhook-test", 1000))
	return h
}

func (h *billingHarness) do(method, path, body, actor string, hdr map[string]string) (int, string) {
	var rd *strings.Reader
	if body != "" {
		rd = strings.NewReader(body)
	} else {
		rd = strings.NewReader("")
	}
	req := httptest.NewRequest(method, path, rd)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if actor != "" {
		req.Header.Set("X-Test-Actor", actor)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.e.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

func TestBillingHandler_ImpersonationNotBlocked(t *testing.T) {
	h := newBillingHarness(t, true)
	if code, _ := h.do("POST", "/api/v1/control/sensitive", "", "impadmin1", nil); code != 403 {
		t.Fatalf("kontrol: denyImpersonation harus 403, dapat %d", code)
	}
	code, body := h.do("POST", "/api/v1/billing/checkout", `{"plan_code":"STARTER"}`, "impadmin1", nil)
	if code != 201 {
		t.Fatalf("checkout saat impersonate ADMIN harus 201, dapat %d: %s", code, body)
	}
	var inv struct {
		Invoice map[string]any `json:"invoice"`
	}
	_ = json.Unmarshal([]byte(body), &inv)
	id, _ := inv.Invoice["id"].(string)
	if id == "" {
		t.Fatalf("invoice id kosong: %s", body)
	}
	if code, body := h.do("POST", "/api/v1/billing/invoices/"+id+"/sync", "", "impadmin1", nil); code != 200 {
		t.Fatalf("sync saat impersonate harus 200, dapat %d: %s", code, body)
	}
}

func TestBillingHandler_CheckoutShapeAndNoLeaks(t *testing.T) {
	h := newBillingHarness(t, true)
	code, body := h.do("POST", "/api/v1/billing/checkout", `{"plan_code":"starter"}`, "admin1", nil)
	if code != 201 {
		t.Fatalf("%d %s", code, body)
	}
	var out struct {
		Invoice    map[string]any `json:"invoice"`
		PaymentURL string         `json:"payment_url"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	keys := []string{}
	for k := range out.Invoice {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	want := "amount_idr,created_at,expires_at,id,number,paid_at,payment_url,period_days,plan_code,plan_name,status,tenant_id"
	if strings.Join(keys, ",") != want {
		t.Fatalf("bentuk invoice salah:\n got  %s\n want %s", strings.Join(keys, ","), want)
	}
	if out.Invoice["status"] != "PENDING" || out.Invoice["amount_idr"].(float64) != 199000 || out.PaymentURL == "" || out.Invoice["payment_url"] != out.PaymentURL {
		t.Fatalf("isi invoice salah: %s", body)
	}
	if len(out.Invoice["id"].(string)) != 36 {
		t.Errorf("id harus uuid: %v", out.Invoice["id"])
	}
	for _, leak := range []string{"xendit_invoice_id", "plan_id", `"ID"`, `"created_by"`, hTestToken} {
		if strings.Contains(body, leak) {
			t.Errorf("respons membocorkan %q: %s", leak, body)
		}
	}
	// Daftar invoice: bentuk + meta
	code, body = h.do("GET", "/api/v1/billing/invoices?page=1&page_size=10", "", "admin1", nil)
	if code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	var list struct {
		Data []map[string]any `json:"data"`
		Meta map[string]int   `json:"meta"`
	}
	_ = json.Unmarshal([]byte(body), &list)
	if len(list.Data) != 1 || list.Meta["total"] != 1 || list.Meta["page"] != 1 || list.Meta["limit"] != 10 {
		t.Fatalf("daftar invoice: %s", body)
	}
	if strings.Contains(body, "xendit_invoice_id") {
		t.Error("daftar invoice membocorkan id gateway")
	}
}

func TestBillingHandler_InputValidation(t *testing.T) {
	h := newBillingHarness(t, true)
	big := `{"plan_code":"` + strings.Repeat("A", 20000) + `"}`
	cases := []struct {
		name, method, path, body, actor string
		want                            int
	}{
		{"nominal dari klien (field tak dikenal)", "POST", "/api/v1/billing/checkout", `{"plan_code":"STARTER","amount":1}`, "admin1", 400},
		{"price_idr dari klien", "POST", "/api/v1/billing/checkout", `{"plan_code":"STARTER","price_idr":1}`, "admin1", 400},
		{"plan_code kosong", "POST", "/api/v1/billing/checkout", `{"plan_code":""}`, "admin1", 400},
		{"plan_code hilang", "POST", "/api/v1/billing/checkout", `{}`, "admin1", 400},
		{"plan_code angka", "POST", "/api/v1/billing/checkout", `{"plan_code":5}`, "admin1", 400},
		{"plan_code kontrol", "POST", "/api/v1/billing/checkout", "{\"plan_code\":\"STAR\\u0000TER\"}", "admin1", 400},
		{"plan_code SQL", "POST", "/api/v1/billing/checkout", `{"plan_code":"STARTER' OR '1'='1"}`, "admin1", 400},
		{"plan_code 65 karakter", "POST", "/api/v1/billing/checkout", `{"plan_code":"` + strings.Repeat("A", 65) + `"}`, "admin1", 400},
		{"plan tak ada", "POST", "/api/v1/billing/checkout", `{"plan_code":"NOPE"}`, "admin1", 400},
		{"plan nonaktif", "POST", "/api/v1/billing/checkout", `{"plan_code":"OLD"}`, "admin1", 400},
		{"tenant_id nol", "POST", "/api/v1/billing/checkout", `{"plan_code":"STARTER","tenant_id":0}`, "super", 400},
		{"tenant_id negatif", "POST", "/api/v1/billing/checkout", `{"plan_code":"STARTER","tenant_id":-1}`, "super", 400},
		{"tenant_id string", "POST", "/api/v1/billing/checkout", `{"plan_code":"STARTER","tenant_id":"1"}`, "super", 400},
		{"JSON rusak", "POST", "/api/v1/billing/checkout", `{"plan_code":`, "admin1", 400},
		{"data tambahan", "POST", "/api/v1/billing/checkout", `{"plan_code":"STARTER"}{"x":1}`, "admin1", 400},
		{"body terlalu besar", "POST", "/api/v1/billing/checkout", big, "admin1", 413},
		{"superadmin tanpa tenant", "POST", "/api/v1/billing/checkout", `{"plan_code":"STARTER"}`, "super", 400},
		{"tenant lain", "POST", "/api/v1/billing/checkout", `{"plan_code":"STARTER","tenant_id":2}`, "admin1", 403},
		{"NOC", "POST", "/api/v1/billing/checkout", `{"plan_code":"STARTER"}`, "noc1", 403},
		{"VIEWER", "POST", "/api/v1/billing/checkout", `{"plan_code":"STARTER"}`, "viewer1", 403},
		{"ENDUSER", "POST", "/api/v1/billing/checkout", `{"plan_code":"STARTER"}`, "enduser1", 403},
		{"tanpa autentikasi", "POST", "/api/v1/billing/checkout", `{"plan_code":"STARTER"}`, "", 401},

		{"query tenant_id huruf", "GET", "/api/v1/billing/subscription?tenant_id=abc", "", "admin1", 400},
		{"query tenant_id nol", "GET", "/api/v1/billing/invoices?tenant_id=0", "", "super", 400},
		{"query tenant_id negatif", "GET", "/api/v1/billing/invoices?tenant_id=-3", "", "super", 400},
		{"query tenant_id SQL", "GET", "/api/v1/billing/invoices?tenant_id=1%20OR%201=1", "", "super", 400},
		{"invoices oleh NOC", "GET", "/api/v1/billing/invoices", "", "noc1", 403},
		{"invoices oleh ENDUSER", "GET", "/api/v1/billing/invoices", "", "enduser1", 403},
		{"subscription oleh NOC", "GET", "/api/v1/billing/subscription", "", "noc1", 200},
		{"subscription oleh ENDUSER", "GET", "/api/v1/billing/subscription", "", "enduser1", 403},
		{"plans oleh VIEWER", "GET", "/api/v1/billing/plans", "", "viewer1", 200},
		{"plans oleh ENDUSER", "GET", "/api/v1/billing/plans", "", "enduser1", 403},
		{"plans tanpa auth", "GET", "/api/v1/billing/plans", "", "", 401},

		{"sync uuid sampah", "POST", "/api/v1/billing/invoices/not-a-uuid/sync", "", "admin1", 400},
		{"sync uuid panjang", "POST", "/api/v1/billing/invoices/" + strings.Repeat("a", 40) + "/sync", "", "admin1", 400},
		{"sync uuid tak ada", "POST", "/api/v1/billing/invoices/00000000-0000-4000-8000-000000000000/sync", "", "admin1", 404},
		{"sync oleh NOC", "POST", "/api/v1/billing/invoices/00000000-0000-4000-8000-000000000000/sync", "", "noc1", 403},

		{"plan dibuat ADMIN", "POST", "/api/v1/billing/plans", `{"code":"GOLD","name":"Gold","price_idr":1000}`, "admin1", 403},
		{"plan diubah ADMIN", "PATCH", "/api/v1/billing/plans/STARTER", `{"price_idr":1}`, "admin1", 403},
		{"plan harga pecahan", "POST", "/api/v1/billing/plans", `{"code":"GOLD","name":"Gold","price_idr":10.5}`, "super", 400},
		{"plan harga negatif", "POST", "/api/v1/billing/plans", `{"code":"GOLD","name":"Gold","price_idr":-1}`, "super", 400},
		{"plan harga hilang", "POST", "/api/v1/billing/plans", `{"code":"GOLD","name":"Gold"}`, "super", 400},
		{"plan kode jelek", "POST", "/api/v1/billing/plans", `{"code":"go ld","name":"Gold","price_idr":1}`, "super", 400},
		{"plan nama 65", "POST", "/api/v1/billing/plans", `{"code":"GOLD","name":"` + strings.Repeat("n", 65) + `","price_idr":1}`, "super", 400},
		{"plan deskripsi 256", "POST", "/api/v1/billing/plans", `{"code":"GOLD","name":"Gold","description":"` + strings.Repeat("d", 256) + `","price_idr":1}`, "super", 400},
		{"plan nama kontrol", "POST", "/api/v1/billing/plans", "{\"code\":\"GOLD\",\"name\":\"Go\\nld\",\"price_idr\":1}", "super", 400},
		{"plan field asing", "POST", "/api/v1/billing/plans", `{"code":"GOLD","name":"Gold","price_idr":1,"id":5}`, "super", 400},
		{"plan max_devices 0", "POST", "/api/v1/billing/plans", `{"code":"GOLD","name":"Gold","price_idr":1,"max_devices":0}`, "super", 400},
		{"plan periode 400", "POST", "/api/v1/billing/plans", `{"code":"GOLD","name":"Gold","price_idr":1,"period_days":400}`, "super", 400},
		{"plan harga 2 miliar", "POST", "/api/v1/billing/plans", `{"code":"GOLD","name":"Gold","price_idr":2000000000}`, "super", 400},
		{"patch max_devices teks", "PATCH", "/api/v1/billing/plans/STARTER", `{"max_devices":"abc"}`, "super", 400},
		{"patch kode jelek", "PATCH", "/api/v1/billing/plans/ST%20ART", `{"price_idr":1}`, "super", 400},
		{"patch paket tak ada", "PATCH", "/api/v1/billing/plans/NOPE", `{"price_idr":1}`, "super", 404},
	}
	for _, c := range cases {
		code, body := h.do(c.method, c.path, c.body, c.actor, nil)
		if code != c.want {
			t.Errorf("%s: status %d, mau %d (%s)", c.name, code, c.want, body)
		}
	}
}

func TestBillingHandler_PlanCRUD(t *testing.T) {
	h := newBillingHarness(t, true)
	code, body := h.do("POST", "/api/v1/billing/plans", `{"code":"gold","name":"Gold","description":"Emas","max_devices":100,"price_idr":500000,"period_days":30,"is_active":true,"sort_order":5}`, "super", nil)
	if code != 201 {
		t.Fatalf("%d %s", code, body)
	}
	var p map[string]any
	_ = json.Unmarshal([]byte(body), &p)
	if p["code"] != "GOLD" || p["max_devices"].(float64) != 100 || p["price_idr"].(float64) != 500000 || p["is_active"] != true {
		t.Fatalf("plan: %s", body)
	}
	if strings.Contains(body, `"ID"`) || strings.Contains(body, "created_at") {
		t.Errorf("PK/audit tidak boleh diekspos: %s", body)
	}
	if code, _ := h.do("POST", "/api/v1/billing/plans", `{"code":"GOLD","name":"Gold","price_idr":1}`, "super", nil); code != 409 {
		t.Errorf("kode ganda harus 409: %d", code)
	}
	// PATCH: max_devices null => tak dibatasi
	code, body = h.do("PATCH", "/api/v1/billing/plans/gold", `{"max_devices":null,"price_idr":600000}`, "super", nil)
	if code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	_ = json.Unmarshal([]byte(body), &p)
	if p["max_devices"] != nil || p["price_idr"].(float64) != 600000 {
		t.Fatalf("PATCH null: %s", body)
	}
	// PATCH tanpa max_devices => tak berubah
	h.do("PATCH", "/api/v1/billing/plans/GOLD", `{"max_devices":7}`, "super", nil)
	_, body = h.do("PATCH", "/api/v1/billing/plans/GOLD", `{"name":"Gold 2"}`, "super", nil)
	_ = json.Unmarshal([]byte(body), &p)
	if p["max_devices"].(float64) != 7 || p["name"] != "Gold 2" {
		t.Fatalf("PATCH parsial: %s", body)
	}
	// ADMIN hanya melihat paket aktif; superadmin semuanya
	count := func(actor string) int {
		_, b := h.do("GET", "/api/v1/billing/plans", "", actor, nil)
		var r struct {
			Data []map[string]any `json:"data"`
		}
		_ = json.Unmarshal([]byte(b), &r)
		return len(r.Data)
	}
	if count("admin1") != 2 || count("super") != 3 {
		t.Errorf("daftar paket per peran: admin=%d super=%d", count("admin1"), count("super"))
	}
}

func TestBillingHandler_Subscription(t *testing.T) {
	h := newBillingHarness(t, true)
	code, body := h.do("GET", "/api/v1/billing/subscription", "", "admin1", nil)
	if code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	var s map[string]any
	_ = json.Unmarshal([]byte(body), &s)
	for _, k := range []string{"enabled", "status", "plan", "expires_at", "days_left", "max_devices", "pending_invoice"} {
		if _, ok := s[k]; !ok {
			t.Errorf("kunci %q hilang: %s", k, body)
		}
	}
	if s["enabled"] != true || s["status"] != "NONE" || s["plan"] != nil || s["expires_at"] != nil || s["days_left"] != nil {
		t.Errorf("NONE: %s", body)
	}
	if code, _ := h.do("GET", "/api/v1/billing/subscription", "", "super", nil); code != 400 {
		t.Errorf("superadmin tanpa tenant_id: %d", code)
	}
	if code, _ := h.do("GET", "/api/v1/billing/subscription?tenant_id=1", "", "super", nil); code != 200 {
		t.Errorf("superadmin dengan tenant_id: %d", code)
	}
	if code, _ := h.do("GET", "/api/v1/billing/subscription?tenant_id=2", "", "admin1", nil); code != 403 {
		t.Errorf("tenant lain: %d", code)
	}
}

func TestBillingHandler_SyncIsolationAndRate(t *testing.T) {
	h := newBillingHarness(t, true)
	_, body := h.do("POST", "/api/v1/billing/checkout", `{"plan_code":"STARTER"}`, "admin1", nil)
	var out struct {
		Invoice map[string]any `json:"invoice"`
	}
	_ = json.Unmarshal([]byte(body), &out)
	id := out.Invoice["id"].(string)
	if code, _ := h.do("POST", "/api/v1/billing/invoices/"+id+"/sync", "", "admin2", nil); code != 404 {
		t.Errorf("ADMIN tenant lain harus 404: %d", code)
	}
	if code, _ := h.do("POST", "/api/v1/billing/invoices/"+id+"/sync", "", "admin1", nil); code != 200 {
		t.Errorf("sync pertama: %d", code)
	}
	if code, _ := h.do("POST", "/api/v1/billing/invoices/"+id+"/sync", "", "admin1", nil); code != 429 {
		t.Errorf("sync kedua harus 429: %d", code)
	}
}

func TestBillingHandler_Webhook(t *testing.T) {
	t.Run("billing mati -> 404", func(t *testing.T) {
		h := newBillingHarness(t, false)
		code, _ := h.do("POST", "/api/v1/billing/xendit/webhook", `{"id":"x"}`, "", map[string]string{"X-CALLBACK-TOKEN": hTestToken})
		if code != 404 {
			t.Fatalf("harus 404, dapat %d", code)
		}
		// rute mutasi lain: 409 jelas
		code, body := h.do("POST", "/api/v1/billing/checkout", `{"plan_code":"STARTER"}`, "admin1", nil)
		if code != 409 || !strings.Contains(body, "belum diaktifkan") {
			t.Fatalf("checkout saat mati harus 409 jelas: %d %s", code, body)
		}
	})

	h := newBillingHarness(t, true)
	// Publik: tidak butuh X-Test-Actor / JWT, tetapi token wajib.
	post := func(body string, hdr map[string]string) (int, string) {
		return h.do("POST", "/api/v1/billing/xendit/webhook", body, "", hdr)
	}
	if code, _ := post(`{"id":"x"}`, nil); code != 401 {
		t.Errorf("tanpa token harus 401: %d", code)
	}
	if code, body := post(`{"id":"x"}`, map[string]string{"X-CALLBACK-TOKEN": "salah"}); code != 401 || strings.Contains(body, hTestToken) {
		t.Errorf("token salah harus 401: %d %s", code, body)
	}
	if code, _ := post(`{"id":"x"}`, map[string]string{"X-CALLBACK-TOKEN": strings.Repeat("a", 600)}); code != 401 {
		t.Errorf("token kepanjangan harus 401: %d", code)
	}
	ok := map[string]string{"X-CALLBACK-TOKEN": hTestToken}
	if code, body := post(`{"id":"inv_asing","external_id":"ACS-1-1-zz","status":"PAID"}`, ok); code != 200 || !strings.Contains(body, `"ok"`) {
		t.Errorf("invoice tak dikenal harus 200 seragam: %d %s", code, body)
	}
	if code, _ := post(`bukan json`, ok); code != 400 {
		t.Errorf("JSON rusak dengan token valid: %d", code)
	}
	if code, _ := post(`{"id":"`+strings.Repeat("a", 70*1024)+`"}`, ok); code != 413 {
		t.Errorf("body >64KiB harus 413: %d", code)
	}
	if code, _ := h.do("GET", "/api/v1/billing/xendit/webhook", "", "", ok); code != 405 && code != 404 && code != 401 {
		t.Errorf("GET webhook harus ditolak: %d", code)
	}

	// Alur nyata: checkout -> Xendit melunasi -> callback -> tenant diperpanjang.
	_, b := h.do("POST", "/api/v1/billing/checkout", `{"plan_code":"STARTER"}`, "admin1", nil)
	var out struct {
		Invoice map[string]any `json:"invoice"`
	}
	_ = json.Unmarshal([]byte(b), &out)
	number := out.Invoice["number"].(string)
	inv := h.repo.InvoiceByNumber(number)
	// payload mengaku PAID, gateway masih PENDING => tidak lunas, tetap 200
	forged := fmt.Sprintf(`{"id":%q,"external_id":%q,"status":"PAID","amount":199000}`, *inv.XenditInvoiceID, number)
	if code, _ := post(forged, ok); code != 200 {
		t.Fatalf("callback harus 200: %d", code)
	}
	if h.repo.Tenant(1).PlanExpiresAt != nil {
		t.Fatal("payload palsu tidak boleh melunasi")
	}
	h.gw.Set(*inv.XenditInvoiceID, domain.GatewayStatusPaid, 199000, nil)
	if code, _ := post(forged, ok); code != 200 {
		t.Fatalf("callback harus 200: %d", code)
	}
	if h.repo.Tenant(1).PlanExpiresAt == nil || h.repo.InvoiceByNumber(number).Status != domain.InvoicePaid {
		t.Fatal("callback sah harus melunasi")
	}
	// Gateway tak terjangkau saat verifikasi -> 5xx supaya Xendit mengulang.
	h.gw.GetErr = fmt.Errorf("xendit: HTTP 503")
	_, b = h.do("POST", "/api/v1/billing/checkout", `{"plan_code":"STARTER"}`, "admin2", nil)
	var o2 struct {
		Invoice map[string]any `json:"invoice"`
	}
	_ = json.Unmarshal([]byte(b), &o2)
	n2 := o2.Invoice["number"].(string)
	inv2 := h.repo.InvoiceByNumber(n2)
	body2 := fmt.Sprintf(`{"id":%q,"external_id":%q,"status":"PAID"}`, *inv2.XenditInvoiceID, n2)
	if code, _ := post(body2, ok); code != 502 {
		t.Fatalf("gateway gagal harus 502 (agar diulang): %d", code)
	}
}
