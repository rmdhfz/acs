package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"acs/pkg/totp"
)

// S16 — MFA TOTP ujung-ke-ujung terhadap server nyata: enrol (setup → enable), login dua tahap,
// token tahap-2 BUKAN sesi, kode salah/replay ditolak, kode pemulihan sekali pakai, ADMIN tenant
// lain tidak bisa mereset, ADMIN tenant yang sama bisa, lalu login kembali normal.
func scenarioMFA(ctx context.Context, w *World) result {
	run := w.runID
	email := "s16-" + run + "@example.test"
	pw := "S16-Pass-" + run + "-Xy9!"

	call := func(method, path string, body any, token string) (int, map[string]any) {
		var rd io.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			rd = bytes.NewReader(b)
		}
		req, err := http.NewRequestWithContext(ctx, method, w.super.base+path, rd)
		if err != nil {
			return 0, nil
		}
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := w.hc.Do(req)
		if err != nil {
			return 0, nil
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		var out map[string]any
		_ = json.Unmarshal(raw, &out)
		return resp.StatusCode, out
	}
	str := func(m map[string]any, k string) string { s, _ := m[k].(string); return s }
	// limiter login/verify 2 req/s per IP: beri jeda.
	login := func() (int, map[string]any) {
		time.Sleep(700 * time.Millisecond)
		return call(http.MethodPost, "/api/v1/auth/login", map[string]any{"email": email, "password": pw}, "")
	}
	verify := func(mfaToken, code string) (int, map[string]any) {
		time.Sleep(700 * time.Millisecond)
		return call(http.MethodPost, "/api/v1/auth/mfa/verify", map[string]any{"mfa_token": mfaToken, "code": code}, "")
	}

	var created idResp
	if err := w.super.do(http.MethodPost, "/api/v1/users", map[string]any{
		"tenant_id": w.pilotTenantID, "username": "s16-" + run, "email": email, "password": pw,
		"full_name": "S16 MFA Test", "role_codes": []string{"NOC"},
	}, &created); err != nil {
		return failr("buat user uji: %v", err)
	}
	defer func() { _ = w.super.do(http.MethodDelete, fmt.Sprintf("/api/v1/users/%d", created.ID), nil, nil) }()

	// 1. login biasa (belum ber-MFA) → sesi langsung
	st, body := login()
	sess := str(body, "access_token")
	if st != 200 || sess == "" {
		return failr("login awal: %d %v", st, body)
	}
	if st, body = call(http.MethodGet, "/api/v1/auth/mfa", nil, sess); st != 200 || body["available"] != true || body["enabled"] != false {
		return failr("status MFA awal: %d %v", st, body)
	}

	// 2. setup + enable (kode salah ditolak 400, benar → 10 kode pemulihan)
	st, body = call(http.MethodPost, "/api/v1/auth/mfa/setup", nil, sess)
	secret := str(body, "secret")
	if st != 200 || secret == "" || str(body, "otpauth_uri") == "" {
		return failr("setup MFA: %d %v", st, body)
	}
	if st, _ = call(http.MethodPost, "/api/v1/auth/mfa/enable", map[string]any{"code": "000000"}, sess); st != 400 {
		return failr("enable dengan kode salah harus 400, dapat %d", st)
	}
	code, _ := totp.Code(secret, time.Now())
	st, body = call(http.MethodPost, "/api/v1/auth/mfa/enable", map[string]any{"code": code}, sess)
	rawCodes, _ := body["recovery_codes"].([]any)
	if st != 200 || len(rawCodes) != 10 {
		return failr("enable MFA: %d %v", st, body)
	}
	recovery, _ := rawCodes[0].(string)

	// 3. login sekarang dua tahap; token tahap-2 bukan sesi
	st, body = login()
	mt := str(body, "mfa_token")
	if st != 200 || body["mfa_required"] != true || mt == "" || str(body, "access_token") != "" {
		return failr("login akun ber-MFA harus mfa_required tanpa access_token: %d %v", st, body)
	}
	if st, _ = call(http.MethodGet, "/api/v1/devices?page_size=1", nil, mt); st != 401 {
		return failr("mfa_token dipakai sebagai sesi harus 401, dapat %d", st)
	}
	// 4. kode salah → 401; kode benar (langkah berikut, karena langkah kini sudah dipakai enable) → sesi
	if st, _ = verify(mt, "000000"); st != 401 {
		return failr("kode salah harus 401, dapat %d", st)
	}
	next, _ := totp.Code(secret, time.Now().Add(30*time.Second))
	st, body = verify(mt, next)
	sess2 := str(body, "access_token")
	if st != 200 || sess2 == "" {
		return failr("verifikasi kode benar: %d %v", st, body)
	}
	if st, _ = call(http.MethodGet, "/api/v1/devices?page_size=1", nil, sess2); st != 200 {
		return failr("sesi hasil MFA harus valid, dapat %d", st)
	}
	// 5. replay kode yang sama ditolak
	if st, _ = verify(mt, next); st != 401 {
		return failr("replay kode TOTP harus 401, dapat %d", st)
	}
	// 6. kode pemulihan: sekali pakai
	_, body = login()
	mt2 := str(body, "mfa_token")
	if st, body = verify(mt2, recovery); st != 200 || str(body, "access_token") == "" {
		return failr("login dengan kode pemulihan: %d %v", st, body)
	}
	if st, _ = verify(mt2, recovery); st != 401 {
		return failr("kode pemulihan dipakai ulang harus 401, dapat %d", st)
	}
	if st, body = call(http.MethodGet, "/api/v1/auth/mfa", nil, sess2); st != 200 || body["enabled"] != true || body["recovery_codes_left"] != float64(9) {
		return failr("status MFA setelah dipakai: %d %v", st, body)
	}

	// 7. reset admin: tenant lain 403; tenant sama 204 → login normal lagi
	if st, _ = w.pilot2Admin.status(http.MethodDelete, fmt.Sprintf("/api/v1/users/%d/mfa", created.ID), nil); st != 403 {
		return failr("ADMIN tenant lain mereset MFA harus 403, dapat %d", st)
	}
	if st, _ = w.pilotAdmin.status(http.MethodDelete, fmt.Sprintf("/api/v1/users/%d/mfa", created.ID), nil); st != 200 { // helper status() memetakan sukses (204) ke 200
		return failr("ADMIN tenant sama reset MFA harus sukses, dapat %d", st)
	}
	// sesi lama dicabut oleh reset (password_changed_at dinaikkan)
	time.Sleep(1100 * time.Millisecond)
	if st, _ = call(http.MethodGet, "/api/v1/devices?page_size=1", nil, sess2); st != 401 {
		return failr("sesi lama setelah reset MFA harus dicabut (401), dapat %d", st)
	}
	st, body = login()
	if st != 200 || str(body, "access_token") == "" || body["mfa_required"] == true {
		return failr("setelah reset, login harus kembali satu tahap: %d %v", st, body)
	}
	return pass("MFA TOTP: enrol, login 2 tahap, token tahap-2 bukan sesi, replay & kode salah ditolak, kode pemulihan sekali pakai, reset admin lintas-tenant 403/204")
}
