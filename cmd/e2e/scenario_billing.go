package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type billingPlanJSON struct {
	Code       string  `json:"code"`
	Name       string  `json:"name"`
	MaxDevices *uint32 `json:"max_devices"`
	PriceIDR   uint64  `json:"price_idr"`
	PeriodDays uint16  `json:"period_days"`
	IsActive   bool    `json:"is_active"`
}

// S23 — langganan SaaS (Xendit) pada kondisi billing MATI (bawaan stack dev; tanpa kunci Xendit):
//  1. GET /billing/plans: 3 paket contoh (STARTER/BUSINESS/ENTERPRISE) terbaca ADMIN; ENDUSER 403;
//  2. subscription enabled=false; SUPERADMIN wajib tenant_id; ADMIN tak boleh melihat tenant lain;
//  3. checkout -> 409 jelas (ADMIN & SUPERADMIN), NOC/ENDUSER 403, field nominal dari klien ditolak (400);
//     invoices: ADMIN 200 (kosong), NOC 403; sync -> 409;
//  4. webhook publik -> 404 (dengan maupun tanpa token);
//  5. kelola paket: ADMIN 403, SUPERADMIN membuat paket (nonaktif, agar tidak mencemari katalog), validasi ketat
//     (harga negatif/pecahan, kode jelek, field asing), duplikat 409, PATCH (termasuk max_devices:null), paket tak ada 404.
//
// Jalur pembayaran nyata (invoice, callback, perpanjangan) TIDAK diuji di sini: butuh Xendit/sandbox atau server palsu
// dan dicakup tes unit/integrasi Go.
func scenarioBilling(ctx context.Context, w *World) result {
	run := w.runID

	// ---- fixture: ENDUSER dan NOC di tenant pilot ----
	mkUser := func(role string) (*Client, func(), error) {
		user, pw := "s23-"+strings.ToLower(role)+"-"+run, "S23-Pass-"+run+"-Qz7!"
		var created idResp
		if err := w.super.do(http.MethodPost, "/api/v1/users", map[string]any{
			"tenant_id": w.pilotTenantID, "username": user, "email": user + "@example.test", "password": pw,
			"full_name": "S23 " + role, "role_codes": []string{role},
		}, &created); err != nil {
			return nil, nil, fmt.Errorf("buat akun %s: %w", role, err)
		}
		cleanup := func() { _ = w.super.do(http.MethodDelete, fmt.Sprintf("/api/v1/users/%d", created.ID), nil, nil) }
		c := NewClient(w.super.base, false)
		if err := c.Login(user, pw); err != nil {
			cleanup()
			return nil, nil, fmt.Errorf("login %s: %w", role, err)
		}
		return c, cleanup, nil
	}
	cust, cleanCust, err := mkUser("ENDUSER")
	if err != nil {
		return failr("%v", err)
	}
	defer cleanCust()
	noc, cleanNoc, err := mkUser("NOC")
	if err != nil {
		return failr("%v", err)
	}
	defer cleanNoc()

	// ---- 1. paket ----
	var plans struct {
		Data []billingPlanJSON `json:"data"`
	}
	if err := w.pilotAdmin.do(http.MethodGet, "/api/v1/billing/plans", nil, &plans); err != nil {
		return failr("GET billing/plans (ADMIN): %v", err)
	}
	have := map[string]billingPlanJSON{}
	for _, p := range plans.Data {
		have[p.Code] = p
		if !p.IsActive {
			return failr("ADMIN hanya boleh melihat paket aktif, tetapi %s nonaktif", p.Code)
		}
	}
	for _, code := range []string{"STARTER", "BUSINESS", "ENTERPRISE"} {
		p, ok := have[code]
		if !ok {
			return failr("paket contoh %s tidak ada di GET /billing/plans (%d paket)", code, len(plans.Data))
		}
		if p.PriceIDR == 0 || p.PeriodDays == 0 || p.Name == "" {
			return failr("paket %s janggal: %+v", code, p)
		}
	}
	if have["ENTERPRISE"].MaxDevices != nil {
		return failr("ENTERPRISE harus max_devices=null (tak dibatasi), dapat %d", *have["ENTERPRISE"].MaxDevices)
	}
	if st, _ := noc.status(http.MethodGet, "/api/v1/billing/plans", nil); st != 200 {
		return failr("NOC membaca paket harus 200, dapat %d", st)
	}
	if st, _ := cust.status(http.MethodGet, "/api/v1/billing/plans", nil); st != 403 {
		return failr("ENDUSER membaca paket harus 403, dapat %d", st)
	}

	// ---- 2. subscription ----
	var sub struct {
		Enabled bool    `json:"enabled"`
		Status  string  `json:"status"`
		Plan    *any    `json:"plan"`
		Expires *string `json:"expires_at"`
	}
	if err := w.pilotAdmin.do(http.MethodGet, "/api/v1/billing/subscription", nil, &sub); err != nil {
		return failr("GET billing/subscription: %v", err)
	}
	if sub.Enabled {
		return failr("billing harus MATI secara bawaan (enabled=false); stack ini mengaktifkannya — jalankan S23 pada stack bawaan")
	}
	if sub.Status != "NONE" && sub.Status != "ACTIVE" && sub.Status != "EXPIRED" {
		return failr("status langganan tidak dikenal: %q", sub.Status)
	}
	if st, _ := noc.status(http.MethodGet, "/api/v1/billing/subscription", nil); st != 200 {
		return failr("NOC membaca subscription harus 200, dapat %d", st)
	}
	if st, _ := cust.status(http.MethodGet, "/api/v1/billing/subscription", nil); st != 403 {
		return failr("ENDUSER membaca subscription harus 403, dapat %d", st)
	}
	if st, _ := w.super.status(http.MethodGet, "/api/v1/billing/subscription", nil); st != 400 {
		return failr("SUPERADMIN tanpa tenant_id harus 400, dapat %d", st)
	}
	if st, _ := w.super.status(http.MethodGet, fmt.Sprintf("/api/v1/billing/subscription?tenant_id=%d", w.pilotTenantID), nil); st != 200 {
		return failr("SUPERADMIN dengan tenant_id harus 200, dapat %d", st)
	}
	if st, _ := w.pilotAdmin.status(http.MethodGet, fmt.Sprintf("/api/v1/billing/subscription?tenant_id=%d", w.pilot2TenantID), nil); st != 403 {
		return failr("ADMIN melihat langganan tenant lain harus 403, dapat %d", st)
	}
	if st, _ := w.pilotAdmin.status(http.MethodGet, "/api/v1/billing/subscription?tenant_id=abc", nil); st != 400 {
		return failr("tenant_id=abc harus 400, dapat %d", st)
	}

	// ---- 3. checkout / invoices / sync ----
	co := "/api/v1/billing/checkout"
	st, body := w.pilotAdmin.status(http.MethodPost, co, map[string]any{"plan_code": "STARTER"})
	if st != 409 || !strings.Contains(body, "belum diaktifkan") {
		return failr("checkout saat billing mati harus 409 jelas, dapat %d %s", st, body)
	}
	if st, _ := w.super.status(http.MethodPost, co, map[string]any{"plan_code": "STARTER", "tenant_id": w.pilotTenantID}); st != 409 {
		return failr("checkout SUPERADMIN saat billing mati harus 409, dapat %d", st)
	}
	if st, _ := noc.status(http.MethodPost, co, map[string]any{"plan_code": "STARTER"}); st != 403 {
		return failr("checkout NOC harus 403, dapat %d", st)
	}
	if st, _ := cust.status(http.MethodPost, co, map[string]any{"plan_code": "STARTER"}); st != 403 {
		return failr("checkout ENDUSER harus 403, dapat %d", st)
	}
	if st, _ := w.pilotAdmin.status(http.MethodPost, co, map[string]any{"plan_code": "STARTER", "amount": 1}); st != 400 {
		return failr("nominal dari klien (field amount) harus ditolak 400, dapat %d", st)
	}
	if st, _ := w.pilotAdmin.status(http.MethodPost, co, map[string]any{"plan_code": strings.Repeat("A", 65)}); st != 400 {
		return failr("plan_code 65 karakter harus 400, dapat %d", st)
	}
	var inv struct {
		Data []any `json:"data"`
		Meta struct {
			Total int `json:"total"`
			Page  int `json:"page"`
			Limit int `json:"limit"`
		} `json:"meta"`
	}
	if err := w.pilotAdmin.do(http.MethodGet, "/api/v1/billing/invoices?page=1&page_size=10", nil, &inv); err != nil {
		return failr("GET billing/invoices (ADMIN): %v", err)
	}
	if inv.Meta.Page != 1 || inv.Meta.Limit != 10 || inv.Data == nil {
		return failr("bentuk daftar invoice janggal: %+v", inv)
	}
	if st, _ := noc.status(http.MethodGet, "/api/v1/billing/invoices", nil); st != 403 {
		return failr("invoices untuk NOC harus 403, dapat %d", st)
	}
	if st, _ := cust.status(http.MethodGet, "/api/v1/billing/invoices", nil); st != 403 {
		return failr("invoices untuk ENDUSER harus 403, dapat %d", st)
	}
	if st, _ := w.pilotAdmin.status(http.MethodGet, "/api/v1/billing/invoices?tenant_id=0", nil); st != 400 {
		return failr("tenant_id=0 harus 400, dapat %d", st)
	}
	if st, _ := w.pilotAdmin.status(http.MethodPost, "/api/v1/billing/invoices/00000000-0000-4000-8000-000000000000/sync", nil); st != 409 {
		return failr("sync saat billing mati harus 409, dapat %d", st)
	}

	// ---- 4. webhook publik: billing mati -> 404 ----
	for _, tok := range []string{"", "token-apa-saja"} {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.super.base+"/api/v1/billing/xendit/webhook", bytes.NewReader([]byte(`{"id":"x","external_id":"y","status":"PAID"}`)))
		if err != nil {
			return failr("buat request webhook: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")
		if tok != "" {
			req.Header.Set("X-CALLBACK-TOKEN", tok)
		}
		resp, err := w.hc.Do(req)
		if err != nil {
			return failr("webhook: %v", err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			return failr("webhook saat billing mati harus 404 (token=%q), dapat %d", tok, resp.StatusCode)
		}
	}

	// ---- 5. kelola paket ----
	code := "S23" + strings.ToUpper(run)
	newPlan := map[string]any{"code": code, "name": "S23 uji", "description": "paket uji e2e (nonaktif)", "max_devices": 5, "price_idr": 12345, "period_days": 30, "is_active": false, "sort_order": 9990}
	if st, _ := w.pilotAdmin.status(http.MethodPost, "/api/v1/billing/plans", newPlan); st != 403 {
		return failr("ADMIN membuat paket harus 403, dapat %d", st)
	}
	if st, _ := cust.status(http.MethodPost, "/api/v1/billing/plans", newPlan); st != 403 {
		return failr("ENDUSER membuat paket harus 403, dapat %d", st)
	}
	if st, _ := w.pilotAdmin.status(http.MethodPatch, "/api/v1/billing/plans/STARTER", map[string]any{"price_idr": 1}); st != 403 {
		return failr("ADMIN mengubah paket harus 403, dapat %d", st)
	}
	for name, p := range map[string]map[string]any{
		"harga negatif":  {"code": code + "X", "name": "x1", "price_idr": -1},
		"harga pecahan":  {"code": code + "X", "name": "x1", "price_idr": 10.5},
		"harga hilang":   {"code": code + "X", "name": "x1"},
		"kode jelek":     {"code": "kode jelek!", "name": "x1", "price_idr": 1},
		"nama 1 huruf":   {"code": code + "X", "name": "x", "price_idr": 1},
		"field asing":    {"code": code + "X", "name": "x1", "price_idr": 1, "id": 7},
		"periode 0":      {"code": code + "X", "name": "x1", "price_idr": 1, "period_days": 0},
		"max_devices 0":  {"code": code + "X", "name": "x1", "price_idr": 1, "max_devices": 0},
		"harga 2 miliar": {"code": code + "X", "name": "x1", "price_idr": 2000000000},
	} {
		if st, b := w.super.status(http.MethodPost, "/api/v1/billing/plans", p); st != 400 {
			return failr("%s harus 400, dapat %d %s", name, st, b)
		}
	}
	var created billingPlanJSON
	if err := w.super.do(http.MethodPost, "/api/v1/billing/plans", newPlan, &created); err != nil {
		return failr("SUPERADMIN membuat paket: %v", err)
	}
	if created.Code != code || created.PriceIDR != 12345 || created.MaxDevices == nil || *created.MaxDevices != 5 || created.IsActive {
		return failr("paket terbuat janggal: %+v", created)
	}
	if st, _ := w.super.status(http.MethodPost, "/api/v1/billing/plans", newPlan); st != 409 {
		return failr("kode paket ganda harus 409, dapat %d", st)
	}
	var upd map[string]any
	if err := w.super.do(http.MethodPatch, "/api/v1/billing/plans/"+strings.ToLower(code), map[string]any{"price_idr": 54321, "max_devices": nil}, &upd); err != nil {
		return failr("PATCH paket: %v", err)
	}
	if upd["price_idr"] != float64(54321) || upd["max_devices"] != nil {
		return failr("PATCH paket tidak berlaku: %v", upd)
	}
	if st, _ := w.super.status(http.MethodPatch, "/api/v1/billing/plans/"+code, map[string]any{"period_days": 0}); st != 400 {
		return failr("PATCH periode 0 harus 400, dapat %d", st)
	}
	if st, _ := w.super.status(http.MethodPatch, "/api/v1/billing/plans/"+code, map[string]any{"max_devices": "abc"}); st != 400 {
		return failr("PATCH max_devices teks harus 400, dapat %d", st)
	}
	if st, _ := w.super.status(http.MethodPatch, "/api/v1/billing/plans/TIDAK_ADA_"+strings.ToUpper(run), map[string]any{"price_idr": 1}); st != 404 {
		return failr("PATCH paket tak ada harus 404, dapat %d", st)
	}
	// SUPERADMIN melihat paket nonaktif buatannya; ADMIN tidak.
	var all struct {
		Data []billingPlanJSON `json:"data"`
	}
	if err := w.super.do(http.MethodGet, "/api/v1/billing/plans", nil, &all); err != nil {
		return failr("GET billing/plans (SUPERADMIN): %v", err)
	}
	seenSuper := false
	for _, p := range all.Data {
		if p.Code == code {
			seenSuper = true
		}
	}
	if err := w.pilotAdmin.do(http.MethodGet, "/api/v1/billing/plans", nil, &plans); err != nil {
		return failr("GET billing/plans (ADMIN) ulang: %v", err)
	}
	for _, p := range plans.Data {
		if p.Code == code {
			return failr("ADMIN tidak boleh melihat paket nonaktif %s", code)
		}
	}
	if !seenSuper {
		return failr("SUPERADMIN harus melihat paket nonaktif %s", code)
	}

	// Hapus paket uji: hanya SUPERADMIN, kode jelek 400, tak ada 404, lalu hilang dari daftar; hapus ulang 404.
	// (Tanpa ini setiap run e2e meninggalkan satu paket "S23 uji" di katalog.)
	del := "/api/v1/billing/plans/" + strings.ToLower(code)
	if st, _ := w.pilotAdmin.status(http.MethodDelete, del, nil); st != 403 {
		return failr("ADMIN menghapus paket harus 403, dapat %d", st)
	}
	if st, _ := w.super.status(http.MethodDelete, "/api/v1/billing/plans/kode jelek!", nil); st != 400 {
		return failr("hapus paket kode jelek harus 400, dapat %d", st)
	}
	if st, _ := w.super.status(http.MethodDelete, "/api/v1/billing/plans/TIDAK_ADA_"+strings.ToUpper(run), nil); st != 404 {
		return failr("hapus paket tak ada harus 404, dapat %d", st)
	}
	if err := w.super.do(http.MethodDelete, del, nil, nil); err != nil {
		return failr("SUPERADMIN menghapus paket uji: %v", err)
	}
	if st, _ := w.super.status(http.MethodDelete, del, nil); st != 404 {
		return failr("hapus paket dua kali harus 404, dapat %d", st)
	}
	if err := w.super.do(http.MethodGet, "/api/v1/billing/plans", nil, &all); err != nil {
		return failr("GET billing/plans setelah hapus: %v", err)
	}
	for _, p := range all.Data {
		if p.Code == code {
			return failr("paket %s masih muncul setelah dihapus", code)
		}
	}

	return pass("billing mati: 3 paket contoh, subscription enabled=false, checkout/sync 409, webhook 404, RBAC (ENDUSER/NOC/ADMIN) benar, CRUD paket tervalidasi, paket uji dihapus (hanya SUPERADMIN)")
}
