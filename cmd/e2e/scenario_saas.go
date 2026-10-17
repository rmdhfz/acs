package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// S19 — operator SaaS: laporan platform & impersonate.
//  1. GET /reports/saas hanya SUPERADMIN (ADMIN tenant 403); memuat tenant pilot; CSV unduhan 200 text/csv.
//  2. SUPERADMIN meng-impersonate ADMIN tenant pilot: token pendek berperan ADMIN dengan jejak impersonator;
//     isolasi tenant tetap berlaku; endpoint platform & aksi sensitif (password, MFA, token API, impersonate lagi) 403.
//  3. ADMIN tenant tidak bisa meng-impersonate; tidak bisa ke diri sendiri.
//  4. Audit: IMPERSONATE_START/END tercatat dan aksi selama impersonate memuat impersonated_by.
func scenarioSaaSOperator(ctx context.Context, w *World) result {
	var doCall func(method, path string, body any, token string, retried bool) (int, []byte, string)
	doCall = func(method, path string, body any, token string, retried bool) (int, []byte, string) {
		var rd io.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			rd = bytes.NewReader(b)
		}
		req, err := http.NewRequestWithContext(ctx, method, w.super.base+path, rd)
		if err != nil {
			return 0, nil, ""
		}
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := w.hc.Do(req)
		if err != nil {
			return 0, nil, ""
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		if resp.StatusCode == http.StatusTooManyRequests && !retried {
			// rate limit global 30 req/dtk per IP: rentetan panggilan skenario ini bisa menabraknya; tunggu & ulangi sekali.
			time.Sleep(1200 * time.Millisecond)
			return doCall(method, path, body, token, true)
		}
		return resp.StatusCode, raw, resp.Header.Get("Content-Type")
	}
	call := func(method, path string, body any, token string) (int, []byte, string) {
		return doCall(method, path, body, token, false)
	}

	// ---- 1. laporan ----
	var rep struct {
		Platform struct {
			TenantsTotal int `json:"tenants_total"`
		} `json:"platform"`
		Tenants []struct {
			Code   string `json:"code"`
			Health string `json:"health"`
		} `json:"tenants"`
		Daily []any `json:"daily"`
	}
	if err := w.super.do(http.MethodGet, "/api/v1/reports/saas", nil, &rep); err != nil {
		return failr("GET reports/saas: %v", err)
	}
	if rep.Platform.TenantsTotal < 2 || len(rep.Tenants) != rep.Platform.TenantsTotal || len(rep.Daily) != 30 {
		return failr("isi laporan janggal: tenants_total=%d baris=%d daily=%d", rep.Platform.TenantsTotal, len(rep.Tenants), len(rep.Daily))
	}
	if st, _ := w.pilotAdmin.status(http.MethodGet, "/api/v1/reports/saas", nil); st != 403 {
		return failr("ADMIN tenant membaca laporan platform harus 403, dapat %d", st)
	}
	st, csvBody, ctype := call(http.MethodGet, "/api/v1/reports/saas/tenants.csv", nil, w.super.token)
	if st != 200 || !strings.HasPrefix(ctype, "text/csv") || !bytes.Contains(csvBody, []byte("kode,nama,status")) {
		return failr("CSV laporan: HTTP %d %q", st, ctype)
	}
	if st, _ := w.pilotAdmin.status(http.MethodGet, "/api/v1/reports/saas/tenants.csv", nil); st != 403 {
		return failr("ADMIN tenant mengunduh CSV platform harus 403, dapat %d", st)
	}

	// ---- 2. impersonate ----
	var users struct {
		Data []struct {
			ID    uint64   `json:"id"`
			Roles []string `json:"roles"`
		} `json:"data"`
	}
	if err := w.super.do(http.MethodGet, fmt.Sprintf("/api/v1/users?tenant_id=%d&page_size=100", w.pilotTenantID), nil, &users); err != nil {
		return failr("list user pilot: %v", err)
	}
	var adminID uint64
	for _, u := range users.Data {
		for _, r := range u.Roles {
			if r == "ADMIN" {
				adminID = u.ID
			}
		}
	}
	if adminID == 0 {
		return skipr("tidak ada ADMIN di tenant pilot")
	}
	var meSuper struct {
		ID uint64 `json:"id"`
	}
	if st, raw, _ := call(http.MethodGet, "/api/v1/auth/me", nil, w.super.token); st != 200 || json.Unmarshal(raw, &meSuper) != nil || meSuper.ID == 0 {
		return failr("auth/me superadmin: %d", st)
	}
	st, raw, _ := call(http.MethodPost, fmt.Sprintf("/api/v1/users/%d/impersonate", adminID), map[string]any{}, w.super.token)
	var imp struct {
		AccessToken    string `json:"access_token"`
		ImpersonatorID uint64 `json:"impersonator_id"`
	}
	if st != 200 || json.Unmarshal(raw, &imp) != nil || imp.AccessToken == "" || imp.ImpersonatorID != meSuper.ID {
		return failr("impersonate: HTTP %d %s", st, snippet(raw))
	}
	tok := imp.AccessToken

	var me struct {
		ID             uint64   `json:"id"`
		Roles          []string `json:"roles"`
		ImpersonatorID uint64   `json:"impersonator_id"`
	}
	if st, raw, _ := call(http.MethodGet, "/api/v1/auth/me", nil, tok); st != 200 || json.Unmarshal(raw, &me) != nil ||
		me.ID != adminID || me.ImpersonatorID != meSuper.ID || len(me.Roles) != 1 || me.Roles[0] != "ADMIN" {
		return failr("auth/me saat impersonate: %d %s", st, snippet(raw))
	}
	// berperan sebagai ADMIN tenant: melihat device pilot, TIDAK melihat tenant lain / fitur platform
	if st, _, _ := call(http.MethodGet, "/api/v1/devices?page_size=1", nil, tok); st != 200 {
		return failr("GET devices saat impersonate: %d", st)
	}
	forbidden := []struct{ m, p string }{
		{http.MethodGet, "/api/v1/reports/saas"},
		{http.MethodGet, "/api/v1/tenants"},
		{http.MethodPost, fmt.Sprintf("/api/v1/users/%d/impersonate", adminID)},
		{http.MethodPatch, "/api/v1/auth/password"},
		{http.MethodPost, "/api/v1/auth/mfa/setup"},
		{http.MethodPost, "/api/v1/auth/tokens"},
	}
	for _, f := range forbidden {
		if st, _, _ := call(f.m, f.p, map[string]any{"name": "x", "current_password": "x", "new_password": "x"}, tok); st != 403 {
			return failr("%s %s saat impersonate harus 403, dapat %d", f.m, f.p, st)
		}
	}
	var otherTenantDev struct {
		Data []struct {
			TenantID uint64 `json:"tenant_id"`
		} `json:"data"`
	}
	if _, raw, _ := call(http.MethodGet, "/api/v1/devices?page_size=100", nil, tok); json.Unmarshal(raw, &otherTenantDev) == nil {
		for _, d := range otherTenantDev.Data {
			if d.TenantID != w.pilotTenantID {
				return failr("impersonate ADMIN tenant pilot melihat device tenant lain (%d)", d.TenantID)
			}
		}
	}

	// ---- 3. yang tidak boleh meng-impersonate ----
	if st, _ := w.pilotAdmin.status(http.MethodPost, fmt.Sprintf("/api/v1/users/%d/impersonate", adminID), map[string]any{}); st != 403 {
		return failr("ADMIN tenant meng-impersonate harus 403, dapat %d", st)
	}
	if st, _, _ := call(http.MethodPost, fmt.Sprintf("/api/v1/users/%d/impersonate", meSuper.ID), map[string]any{}, w.super.token); st != 400 {
		return failr("impersonate diri sendiri harus 400, dapat %d", st)
	}

	// ---- 4. audit ----
	// aksi teraudit selama impersonate: reboot satu device pilot
	var devs struct {
		Data []struct {
			ID uint64 `json:"id"`
		} `json:"data"`
	}
	if _, raw, _ := call(http.MethodGet, "/api/v1/devices?page_size=1", nil, tok); json.Unmarshal(raw, &devs) != nil || len(devs.Data) == 0 {
		return failr("tidak ada device pilot untuk uji audit")
	}
	if st, _, _ := call(http.MethodPost, fmt.Sprintf("/api/v1/devices/%d/reboot", devs.Data[0].ID), map[string]any{}, tok); st >= 400 {
		return failr("reboot saat impersonate: %d", st)
	}
	if st, _, _ := call(http.MethodPost, "/api/v1/auth/impersonation/end", map[string]any{}, tok); st != 204 {
		return failr("akhiri impersonate: %d", st)
	}
	if st, _, _ := call(http.MethodPost, "/api/v1/auth/impersonation/end", map[string]any{}, w.super.token); st != 403 {
		return failr("end impersonation di luar sesi impersonate harus 403, dapat %d", st)
	}
	var act struct {
		Data []struct {
			Action      string  `json:"action"`
			Description *string `json:"description"`
		} `json:"data"`
	}
	if err := w.super.do(http.MethodGet, "/api/v1/activity?page_size=100", nil, &act); err != nil {
		return failr("GET activity: %v", err)
	}
	var started, ended, tagged bool
	for _, a := range act.Data {
		switch a.Action {
		case "IMPERSONATE_START":
			started = true
		case "IMPERSONATE_END":
			ended = true
		}
		if a.Description != nil && strings.Contains(*a.Description, fmt.Sprintf(`"impersonated_by":%d`, meSuper.ID)) {
			tagged = true
		}
	}
	if !started || !ended || !tagged {
		return failr("audit tidak lengkap: start=%v end=%v aksi-bertanda-impersonated_by=%v", started, ended, tagged)
	}
	return pass("laporan SaaS (superadmin saja, 30 hari, CSV); impersonate: token ADMIN pendek + jejak impersonator, isolasi tenant utuh, 6 aksi platform/sensitif 403, ADMIN/diri sendiri ditolak, audit START/END + impersonated_by")
}
