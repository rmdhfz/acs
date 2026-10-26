package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

// S36 — penerima webhook Alertmanager (migrations/0045): alert Prometheus menjadi notifikasi PLATFORM_ALERT untuk SUPERADMIN.
// Memanggil endpoint langsung dengan payload versi 4 (rantai Prometheus -> Alertmanager -> acsd dibuktikan terpisah dengan kontainer
// nyata, bukan di suite ini). Token e2e hanya berlaku di stack e2e terpisah (docker-compose.e2e.yml).
//
//	AM-01  tanpa token / token salah / skema salah -> 401 dan tidak ada notifikasi
//	AM-02  alert firing -> notifikasi PLATFORM_ALERT (judul, keparahan dari label severity) untuk SUPERADMIN
//	AM-03  alert yang sama dikirim ulang (repeat_interval Alertmanager) tidak menggandakan notifikasi (dedupe)
//	AM-04  alert pulih -> notifikasi [PULIH] berkeparahan INFO, terpisah dari yang firing
//	AM-05  label berbeda = alert berbeda (vendor lain tidak tertahan dedupe); severity critical -> CRITICAL
//	AM-06  admin tenant TIDAK menerima alert platform (metrik lintas tenant, hanya operator)
//	AM-07  payload tak valid -> 400 (versi lain, alerts kosong, bukan JSON); field asing tetap diterima
const e2eAlertToken = "e2e-alertmanager-token-0123456789abcdef"

func scenarioAlertmanager(ctx context.Context, w *World) result {
	_ = ctx
	run := w.runID
	url := w.super.base + "/api/v1/alertmanager-webhook"

	// send mengirim satu permintaan; 429 (pembatas laju khusus webhook, 5/detik per IP) dicoba ulang seperti Client.do, karena skenario ini
	// sengaja mengirim banyak permintaan beruntun (termasuk 5 percobaan token salah). Pembatasnya sendiri bekerja sebagaimana mestinya.
	send := func(auth string, body any) (int, string) {
		var payloadBytes []byte
		switch b := body.(type) {
		case string:
			payloadBytes = []byte(b)
		default:
			payloadBytes, _ = json.Marshal(b)
		}
		for attempt := 0; attempt < 6; attempt++ {
			req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payloadBytes))
			if err != nil {
				return 0, err.Error()
			}
			req.Header.Set("Content-Type", "application/json")
			if auth != "" {
				req.Header.Set("Authorization", auth)
			}
			resp, err := w.super.http.Do(req)
			if err != nil {
				return 0, err.Error()
			}
			raw, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode == http.StatusTooManyRequests {
				time.Sleep(time.Duration(attempt+1) * 700 * time.Millisecond)
				continue
			}
			return resp.StatusCode, string(raw)
		}
		return http.StatusTooManyRequests, "429 berulang"
	}
	payload := func(status, name, severity, vendor, summary string) map[string]any {
		labels := map[string]any{"alertname": name, "severity": severity, "job": "acsd"}
		if vendor != "" {
			labels["vendor"] = vendor
		}
		return map[string]any{
			"version": "4", "status": status, "receiver": "acsd", "groupKey": "{}:{alertname=\"" + name + "\"}",
			"externalURL": "http://alertmanager:9093", "fieldBaruDariVersiMendatang": map[string]any{"x": 1},
			"alerts": []any{map[string]any{
				"status": status, "labels": labels, "annotations": map[string]any{"summary": summary, "description": "Uji e2e " + run},
				"startsAt": "2026-10-26T08:00:00Z", "endsAt": "0001-01-01T00:00:00Z", "generatorURL": "http://prometheus/graph", "fingerprint": "e2e",
			}},
		}
	}
	list := func(c *Client) ([]notifItem, error) {
		var l notifList
		err := c.do(http.MethodGet, "/api/v1/notifications?page_size=100", nil, &l)
		return l.Data, err
	}
	count := func(items []notifItem, title string) int {
		n := 0
		for _, it := range items {
			if it.Title == title {
				n++
			}
		}
		return n
	}
	bearer := "Bearer " + e2eAlertToken
	name := "E2EAlert" + run
	summary := "Backlog e2e " + run + " menumpuk"
	titleFiring, titleResolved := "[ALERT] "+summary, "[PULIH] "+summary

	// ---- AM-01 ----
	before, err := list(w.super)
	if err != nil {
		return failr("GET /notifications: %v", err)
	}
	for label, auth := range map[string]string{
		"tanpa token": "", "token salah": "Bearer token-salah-abcdefghijklmnopqrstuv", "skema salah": "Basic " + e2eAlertToken,
		"tanpa skema": e2eAlertToken, "token lebih panjang": bearer + "x",
	} {
		if code, _ := send(auth, payload("firing", name, "warning", "", summary)); code != http.StatusUnauthorized {
			return failr("AM-01: %s: HTTP %d, harusnya 401", label, code)
		}
	}
	if after, _ := list(w.super); len(after) != len(before) {
		return failr("AM-01: notifikasi bertambah (%d -> %d) padahal semua permintaan ditolak", len(before), len(after))
	}

	// ---- AM-02 ----
	code, body := send(bearer, payload("firing", name, "warning", "ZTE", summary))
	if code != http.StatusOK || !strings.Contains(body, `"accepted":1`) {
		return failr("AM-02: webhook sah HTTP %d %s", code, body)
	}
	items, err := list(w.super)
	if err != nil {
		return failr("AM-02: %v", err)
	}
	f := notifFind(items, titleFiring)
	if f == nil {
		return failr("AM-02: notifikasi %q tidak muncul untuk SUPERADMIN", titleFiring)
	}
	if f.Type != "PLATFORM_ALERT" || f.Severity != "WARNING" || !strings.Contains(f.Body, "vendor=ZTE") || !strings.Contains(f.Body, "Uji e2e "+run) {
		return failr("AM-02: isi notifikasi salah: %+v", f)
	}

	// ---- AM-03 ----
	for i := 0; i < 2; i++ {
		if code, body := send(bearer, payload("firing", name, "warning", "ZTE", summary)); code != http.StatusOK {
			return failr("AM-03: pengiriman ulang #%d HTTP %d %s", i+1, code, body)
		}
	}
	if items, _ = list(w.super); count(items, titleFiring) != 1 {
		return failr("AM-03: alert yang sama dikirim ulang menghasilkan %d notifikasi, harusnya tetap 1 (dedupe)", count(items, titleFiring))
	}

	// ---- AM-04 ----
	if code, body := send(bearer, payload("resolved", name, "warning", "ZTE", summary)); code != http.StatusOK {
		return failr("AM-04: resolved HTTP %d %s", code, body)
	}
	items, _ = list(w.super)
	r := notifFind(items, titleResolved)
	if r == nil || r.Severity != "INFO" || count(items, titleFiring) != 1 {
		return failr("AM-04: notifikasi pulih salah: %+v (firing=%d)", r, count(items, titleFiring))
	}

	// ---- AM-05 ----
	if code, body := send(bearer, payload("firing", name, "warning", "HUAWEI", summary)); code != http.StatusOK {
		return failr("AM-05: vendor lain HTTP %d %s", code, body)
	}
	if items, _ = list(w.super); count(items, titleFiring) != 2 {
		return failr("AM-05: vendor lain adalah alert berbeda, harusnya 2 notifikasi firing, dapat %d", count(items, titleFiring))
	}
	critName, critSummary := "E2ECrit"+run, "Kritis e2e "+run
	if code, body := send(bearer, payload("firing", critName, "critical", "", critSummary)); code != http.StatusOK {
		return failr("AM-05: critical HTTP %d %s", code, body)
	}
	items, _ = list(w.super)
	if c := notifFind(items, "[ALERT] "+critSummary); c == nil || c.Severity != "CRITICAL" {
		return failr("AM-05: severity critical harus CRITICAL: %+v", c)
	}

	// ---- AM-06 ----
	adminItems, err := list(w.pilotAdmin)
	if err != nil {
		return failr("AM-06: %v", err)
	}
	for _, it := range adminItems {
		if it.Type == "PLATFORM_ALERT" {
			return failr("AM-06: admin tenant menerima alert platform: %+v", it)
		}
	}

	// ---- AM-07 ----
	for label, c := range map[string]struct {
		body any
		want int
	}{
		"versi lain":    {`{"version":"3","alerts":[{"status":"firing","labels":{"alertname":"A"}}]}`, http.StatusBadRequest},
		"alerts kosong": {`{"version":"4","alerts":[]}`, http.StatusBadRequest},
		"bukan JSON":    {"{bukan json", http.StatusBadRequest},
	} {
		if code, body := send(bearer, c.body); code != c.want {
			return failr("AM-07: %s: HTTP %d (%s), harusnya %d", label, code, snippet([]byte(body)), c.want)
		}
	}
	return pass("5 jenis autentikasi salah 401 tanpa notifikasi, firing -> PLATFORM_ALERT WARNING untuk SUPERADMIN, kirim ulang tidak menggandakan, pulih -> [PULIH] INFO, " +
		"vendor lain = alert berbeda, critical -> CRITICAL, admin tenant tidak menerima, 3 payload buruk 400 (field asing tetap diterima)")
}
