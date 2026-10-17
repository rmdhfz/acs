package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"acs/internal/simcpe"
)

// S20 — paket SaaS & pendaftaran mandiri:
//  1. Batas perangkat per tenant (tenants.max_devices): Inform perangkat BARU ditolak 503 saat batas tercapai (tidak ada
//     baris dibuat), pra-registrasi ditolak 429, perangkat yang sudah ada tetap boleh Inform, menaikkan batas memulihkan;
//     hanya SUPERADMIN yang boleh mengubah batas.
//  2. Self-signup: jalur publik mengikuti konfigurasi (mati -> 403; menyala -> honeypot diam, token ngawur 400).
func scenarioPlanAndSignup(ctx context.Context, w *World) result {
	run := w.runID
	user, pw := "s20c"+run, "S20-Cwmp-Secret-"+run+"-Xq"
	var tenant idResp
	if err := w.super.do(http.MethodPost, "/api/v1/tenants", map[string]any{
		"code": "s20-" + run, "name": "S20 Paket " + run, "cwmp_inform_username": user, "cwmp_inform_password": pw,
	}, &tenant); err != nil {
		return failr("buat tenant uji: %v", err)
	}
	var created []uint64
	defer func() {
		for _, id := range created {
			_ = w.super.do(http.MethodDelete, fmt.Sprintf("/api/v1/devices/%d", id), nil, nil)
		}
		_ = w.super.do(http.MethodDelete, fmt.Sprintf("/api/v1/tenants/%d", tenant.ID), nil, nil)
	}()

	limitURL := fmt.Sprintf("/api/v1/tenants/%d/device-limit", tenant.ID)
	setLimit := func(v any) (int, string) {
		return w.super.status(http.MethodPatch, limitURL, map[string]any{"max_devices": v})
	}
	inform := func(dev *simcpe.Device) simcpe.SessionReport {
		time.Sleep(400 * time.Millisecond)
		rep := simcpe.RunSession(ctx, w.hc, dev, simcpe.SessionOpts{ACSURL: w.cwmpURL, Username: user, Password: pw, Events: []string{"0 BOOTSTRAP"}})
		if !rep.OK && strings.Contains(rep.ErrMsg, "429") {
			time.Sleep(2 * time.Second)
			rep = simcpe.RunSession(ctx, w.hc, dev, simcpe.SessionOpts{ACSURL: w.cwmpURL, Username: user, Password: pw, Events: []string{"0 BOOTSTRAP"}})
		}
		return rep
	}
	count := func() (int, error) {
		var l struct {
			Total int `json:"total"`
			Data  []struct {
				ID uint64 `json:"id"`
			} `json:"data"`
		}
		if err := w.super.do(http.MethodGet, fmt.Sprintf("/api/v1/devices?tenant_id=%d&page_size=50", tenant.ID), nil, &l); err != nil {
			return 0, err
		}
		created = created[:0]
		for _, d := range l.Data {
			created = append(created, d.ID)
		}
		return len(l.Data), nil
	}

	// ---- 1. batas perangkat ----
	if st, body := setLimit(1); st >= 400 {
		return failr("set batas 1: HTTP %d %s", st, body)
	}
	if st, _ := w.pilotAdmin.status(http.MethodPatch, limitURL, map[string]any{"max_devices": 100}); st != 403 {
		return failr("ADMIN tenant mengubah batas perangkat harus 403, dapat %d", st)
	}
	prof := simcpe.Profiles["zte"]
	d1 := simcpe.NewDevice(prof, "S20A-"+run)
	d2 := simcpe.NewDevice(prof, "S20B-"+run)
	if rep := inform(d1); !rep.OK {
		return failr("perangkat pertama (di bawah batas) harus diterima: %s", rep.ErrMsg)
	}
	if n, err := count(); err != nil || n != 1 {
		return failr("setelah Inform pertama harus 1 perangkat: n=%d err=%v", n, err)
	}
	if rep := inform(d2); rep.OK || !strings.Contains(rep.ErrMsg, "503") {
		return failr("perangkat kedua (melewati batas) harus ditolak 503: ok=%v err=%q", rep.OK, rep.ErrMsg)
	}
	if n, _ := count(); n != 1 {
		return failr("perangkat ditolak tidak boleh membuat baris: n=%d", n)
	}
	if st, _ := w.super.status(http.MethodPost, "/api/v1/devices", map[string]any{
		"tenant_id": tenant.ID, "oui": "00D0D0", "serial_number": "S20PRE-" + run,
	}); st != 429 {
		return failr("pra-registrasi melewati batas harus 429, dapat %d", st)
	}
	// perangkat yang sudah ada tetap boleh Inform walau batas terlampaui
	if rep := inform(d1); !rep.OK {
		return failr("perangkat existing harus tetap diterima di batas penuh: %s", rep.ErrMsg)
	}
	// menaikkan batas memulihkan; null = tak dibatasi
	if st, body := setLimit(2); st >= 400 {
		return failr("naikkan batas: %d %s", st, body)
	}
	if rep := inform(d2); !rep.OK {
		return failr("setelah batas dinaikkan perangkat kedua harus diterima: %s", rep.ErrMsg)
	}
	if st, _ := setLimit(nil); st >= 400 {
		return failr("hapus batas (null): %d", st)
	}
	if n, _ := count(); n != 2 {
		return failr("akhirnya harus 2 perangkat: %d", n)
	}

	// ---- 2. self-signup mengikuti konfigurasi ----
	var cfg struct {
		SelfSignup bool `json:"self_signup_enabled"`
	}
	if err := w.super.do(http.MethodGet, "/api/v1/auth/config", nil, &cfg); err != nil {
		return failr("GET auth/config: %v", err)
	}
	signupNote := "self-signup mati (bawaan): signup & verify -> 403"
	if !cfg.SelfSignup {
		if st, _ := w.super.status(http.MethodPost, "/api/v1/auth/signup", map[string]any{"company_name": "X Corp", "email": "x@example.test"}); st != 403 {
			return failr("signup saat mati harus 403, dapat %d", st)
		}
		if st, _ := w.super.status(http.MethodPost, "/api/v1/auth/signup/verify", map[string]any{"token": strings.Repeat("a", 43)}); st != 403 {
			return failr("verify saat mati harus 403, dapat %d", st)
		}
	} else {
		time.Sleep(1200 * time.Millisecond)
		if st, _ := w.super.status(http.MethodPost, "/api/v1/auth/signup/verify", map[string]any{"token": strings.Repeat("z", 43)}); st != 400 {
			return failr("verify token ngawur harus 400, dapat %d", st)
		}
		signupNote = "self-signup menyala: verify token ngawur -> 400"
	}
	return pass("batas perangkat: Inform baru 503 tanpa membuat baris, pra-registrasi 429, perangkat existing tetap diterima, naikkan batas memulihkan, ADMIN tenant 403; %s", signupNote)
}
