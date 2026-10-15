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

// S12 — login dengan EMAIL + alur lupa/reset password (sisi API publik).
// Dibuktikan terhadap stack hidup: login email (tak peka huruf besar, spasi
// dipangkas), kompatibilitas username, pesan error seragam, anti-enumerasi di
// forgot-password (respons identik untuk email tak terdaftar), rate limit
// per-IP, penolakan token ngawur, dan reset oleh admin yang mengganti
// password (hash Argon2id) tanpa memutus login baru.
//
// Sengaja TIDAK memicu email ke alamat nyata: semua alamat uji berdomain
// @e2e.local (mailer membuang domain reserved), dan forgot-password hanya
// dipanggil untuk email yang tidak terdaftar. Alur token end-to-end (email ->
// tautan -> password baru) dibuktikan di test unit/integrasi + uji manual.
func scenarioAuthEmailReset(ctx context.Context, w *World) result {
	run := strings.ToLower(w.runID)
	email := "s12-" + run + "@e2e.local"
	pass1, pass2 := "S12-Pass-"+randHex(8), "S12-NewPass-"+randHex(8)

	raw := func(method, path string, body any) (int, string) {
		var rd io.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			rd = bytes.NewReader(b)
		}
		req, err := http.NewRequestWithContext(ctx, method, w.super.base+path, rd)
		if err != nil {
			return 0, err.Error()
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := w.hc.Do(req)
		if err != nil {
			return 0, err.Error()
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}
	// login limiter 2 req/s per IP: beri jeda antar panggilan.
	login := func(body map[string]any) (int, string) {
		time.Sleep(700 * time.Millisecond)
		return raw(http.MethodPost, "/api/v1/auth/login", body)
	}

	var created idResp
	if err := w.super.do(http.MethodPost, "/api/v1/users", map[string]any{
		"tenant_id": w.pilotTenantID, "username": "s12-" + run, "email": email, "password": pass1,
		"full_name": "S12 Auth Test", "role_codes": []string{"VIEWER"},
	}, &created); err != nil {
		return failr("buat user uji: %v", err)
	}
	defer func() { _ = w.super.do(http.MethodDelete, fmt.Sprintf("/api/v1/users/%d", created.ID), nil, nil) }()

	// statusWithToken memanggil endpoint terproteksi memakai JWT tertentu.
	statusWithToken := func(token string) int {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, w.super.base+"/api/v1/devices?page_size=1", nil)
		if err != nil {
			return 0
		}
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := w.hc.Do(req)
		if err != nil {
			return 0
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}

	// JWT sesi pertama — dipakai membuktikan pencabutan sesi setelah password diganti.
	_, bFirst := login(map[string]any{"email": email, "password": pass1})
	var first struct {
		AccessToken string `json:"access_token"`
	}
	_ = json.Unmarshal([]byte(bFirst), &first)
	if first.AccessToken == "" {
		return failr("login pertama tidak mengembalikan access_token: %s", bFirst)
	}
	if st := statusWithToken(first.AccessToken); st != http.StatusOK {
		return failr("JWT baru harus bisa mengakses API, dapat HTTP %d", st)
	}

	// 1. Login email: persis, huruf besar + spasi, dan username (kompatibilitas).
	for label, body := range map[string]map[string]any{
		"email persis":       {"email": email, "password": pass1},
		"email HURUF BESAR":  {"email": "  " + strings.ToUpper(email) + " ", "password": pass1},
		"username (kompat.)": {"username": "s12-" + run, "password": pass1},
	} {
		st, b := login(body)
		if st != http.StatusOK || !strings.Contains(b, "access_token") {
			return failr("login %s: HTTP %d %s", label, st, b)
		}
	}

	// 2. Salah password & email tak dikenal → 401 dengan pesan SERAGAM.
	st1, b1 := login(map[string]any{"email": email, "password": "salah-total"})
	st2, b2 := login(map[string]any{"email": "tidak-ada-" + run + "@e2e.local", "password": "salah-total"})
	if st1 != http.StatusUnauthorized || st2 != http.StatusUnauthorized || b1 != b2 || !strings.Contains(b1, "email atau password salah") {
		return failr("pesan 401 harus seragam: [%d %s] vs [%d %s]", st1, b1, st2, b2)
	}
	if st, b := login(map[string]any{"password": "x"}); st != http.StatusBadRequest {
		return failr("login tanpa email/username harus 400, dapat %d %s", st, b)
	}

	// 3. Forgot-password: payload kosong 400; email TAK terdaftar → 202 generik;
	//    panggilan beruntun → 429 (rate limit per-IP).
	if st, b := raw(http.MethodPost, "/api/v1/auth/forgot-password", map[string]any{"email": ""}); st != http.StatusBadRequest {
		return failr("forgot tanpa email harus 400, dapat %d %s", st, b)
	}
	time.Sleep(6 * time.Second) // isi ulang bucket (0,2 req/s)
	stF, bF := raw(http.MethodPost, "/api/v1/auth/forgot-password", map[string]any{"email": "tidak-ada-" + run + "@e2e.local"})
	if stF == http.StatusServiceUnavailable {
		return skipr("SMTP tidak dikonfigurasi di stack ini — forgot-password menjawab 503 (wajar)")
	}
	if stF != http.StatusAccepted || !strings.Contains(bF, "Jika email terdaftar") {
		return failr("forgot (email tak terdaftar) harus 202 generik, dapat %d %s", stF, bF)
	}
	if st, _ := raw(http.MethodPost, "/api/v1/auth/forgot-password", map[string]any{"email": "lagi-" + run + "@e2e.local"}); st != http.StatusTooManyRequests {
		return failr("forgot beruntun harus kena rate limit 429, dapat %d", st)
	}
	time.Sleep(6 * time.Second) // jangan menyisakan bucket kosong untuk run berikutnya

	// 4. Reset-password: token ngawur/kosong → 400 pesan generik.
	for label, body := range map[string]map[string]any{
		"token ngawur":          {"token": "ngawur-" + run, "new_password": pass2},
		"token ngawur + pendek": {"token": "ngawur-" + run, "new_password": "x"},
		"token kosong":          {"token": "", "new_password": pass2},
	} {
		time.Sleep(2200 * time.Millisecond) // limiter reset-password 0,5 req/s
		if st, b := raw(http.MethodPost, "/api/v1/auth/reset-password", body); st != http.StatusBadRequest {
			return failr("reset %s harus 400, dapat %d %s", label, st, b)
		}
	}

	// 5. Admin mereset password → password baru berlaku, lama ditolak.
	if err := w.super.do(http.MethodPatch, fmt.Sprintf("/api/v1/users/%d/password", created.ID), map[string]any{"new_password": pass2}, nil); err != nil {
		return failr("admin reset password: %v", err)
	}
	stNew, bNew := login(map[string]any{"email": email, "password": pass2})
	if stNew != http.StatusOK {
		return failr("login dengan password hasil reset admin: HTTP %d %s", stNew, bNew)
	}
	var second struct {
		AccessToken string `json:"access_token"`
	}
	_ = json.Unmarshal([]byte(bNew), &second)
	// Sesi lama DICABUT (password_changed_at), sesi baru sah.
	if st := statusWithToken(first.AccessToken); st != http.StatusUnauthorized {
		return failr("JWT lama harus 401 setelah password direset, dapat HTTP %d", st)
	}
	if st := statusWithToken(second.AccessToken); st != http.StatusOK {
		return failr("JWT baru (setelah reset) harus 200, dapat HTTP %d", st)
	}
	if st, _ := login(map[string]any{"email": email, "password": pass1}); st != http.StatusUnauthorized {
		return failr("password lama harus ditolak setelah reset, dapat HTTP %d", st)
	}
	return pass("login email (huruf besar/spasi/kompat. username), 401 seragam, forgot 202 generik + 429, reset token ngawur 400, admin-reset bekerja + JWT lama dicabut")
}
