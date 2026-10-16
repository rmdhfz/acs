package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"acs/internal/simcpe"
)

// S14 — diagnostik Ping & Traceroute end-to-end: pemicu (SetParameterValues dengan logical key
// diag.* yang di-resolve per vendor/data model), CPE menjalankan diagnostik, event
// "8 DIAGNOSTICS COMPLETE", ACS mengambil hasil (GetParameterValues) dan menyimpan ringkasan.
// Juga membuktikan: deteksi data model dari parameter perangkat (model belum terdaftar),
// validasi masukan (400), dan tipe yang belum didukung ditolak.
func scenarioDiagnostics(ctx context.Context, w *World) result {
	type diagJSON struct {
		ID     uint64          `json:"id"`
		Type   string          `json:"diagnostic_type"`
		Status string          `json:"status"`
		Result json.RawMessage `json:"result"`
	}
	var details []string
	for _, c := range []struct{ key, typ string }{{"zte", "PING"}, {"nokia", "TRACEROUTE"}} {
		dev, serial := w.devices[c.key], w.serials[c.key]
		if dev == nil {
			return skipr("device %s dari S1 tidak ada", c.key)
		}
		d, err := w.super.findDeviceBySerial(serial)
		if err != nil {
			return failr("%v", err)
		}
		base := fmt.Sprintf("/api/v1/devices/%d/diagnostics", d.ID)

		// validasi
		if st, body := w.pilotAdmin.status(http.MethodPost, base, map[string]any{"diagnostic_type": c.typ, "parameters": map[string]string{"host": "bad host!"}}); st != http.StatusBadRequest {
			return failr("%s: host tak valid harus 400, dapat %d %s", c.key, st, body)
		}
		if st, body := w.pilotAdmin.status(http.MethodPost, base, map[string]any{"diagnostic_type": "WIFI_SCAN", "parameters": map[string]string{"host": "1.1.1.1"}}); st != http.StatusBadRequest {
			return failr("%s: tipe belum didukung harus 400, dapat %d %s", c.key, st, body)
		}

		var created diagJSON
		if err := w.pilotAdmin.do(http.MethodPost, base, map[string]any{
			"diagnostic_type": c.typ, "parameters": map[string]string{"host": "203.0.113.50"},
		}, &created); err != nil {
			return failr("%s: POST diagnostics: %v", c.key, err)
		}
		if created.Status != "PENDING" {
			return failr("%s: status awal harus PENDING, dapat %s", c.key, created.Status)
		}

		// sesi 1: CPE menerima SetParameterValues pemicu & menjalankan diagnostik
		rep := w.runCPE(ctx, dev, simcpe.SessionOpts{Events: []string{"2 PERIODIC"}})
		if !rep.OK || rep.CountRPC("SetParameterValues") == 0 {
			return failr("%s: sesi 1 gagal / tanpa SetParameterValues (rpc %v, err %s)", c.key, rep.RPCsReceived, rep.ErrMsg)
		}
		// sesi 2: Inform membawa event 8 -> ACS mengambil hasil
		rep = w.runCPE(ctx, dev, simcpe.SessionOpts{Events: []string{"2 PERIODIC"}})
		if !rep.OK || rep.CountRPC("GetParameterValues") == 0 {
			return failr("%s: sesi 2 tidak mengambil hasil (events %v rpc %v err %s)", c.key, rep.Events, rep.RPCsReceived, rep.ErrMsg)
		}

		var list struct {
			Data []diagJSON `json:"data"`
		}
		deadline := time.Now().Add(5 * time.Second)
		var got *diagJSON
		for time.Now().Before(deadline) {
			if err := w.pilotAdmin.do(http.MethodGet, base, nil, &list); err != nil {
				return failr("%s: GET diagnostics: %v", c.key, err)
			}
			for i := range list.Data {
				if list.Data[i].ID == created.ID {
					got = &list.Data[i]
				}
			}
			if got != nil && got.Status == "COMPLETED" {
				break
			}
			time.Sleep(300 * time.Millisecond)
		}
		if got == nil || got.Status != "COMPLETED" {
			return failr("%s: diagnostik tidak COMPLETED (%+v)", c.key, got)
		}
		var res struct {
			State   string         `json:"state"`
			Summary map[string]any `json:"summary"`
			Hops    []any          `json:"hops"`
		}
		if err := json.Unmarshal(got.Result, &res); err != nil || res.State != "Complete" {
			return failr("%s: hasil tidak valid: %s", c.key, string(got.Result))
		}
		if c.typ == "PING" && fmt.Sprint(res.Summary["success_count"]) != "4" {
			return failr("%s: PING success_count harus 4: %s", c.key, string(got.Result))
		}
		if c.typ == "TRACEROUTE" && len(res.Hops) != 2 {
			return failr("%s: TRACEROUTE harus 2 hop: %s", c.key, string(got.Result))
		}
		details = append(details, fmt.Sprintf("%s/%s", c.key, c.typ))
	}
	return pass("diagnostik end-to-end (%v): pemicu → event 8 → hasil terstruktur; validasi host/tipe 400", details)
}
