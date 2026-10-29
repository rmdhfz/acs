package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"acs/internal/simcpe"
)

// S18 — portal pelanggan "WiFi Saya" end-to-end, termasuk PENAUTAN akun↔perangkat oleh admin (sebelumnya
// tidak ada API-nya sama sekali): akun ENDUSER tanpa tautan tidak melihat apa pun; ADMIN tenant lain tidak
// bisa menautkan; ADMIN tenant sama menautkan; pelanggan melihat tepat 1 perangkat, mengganti SSID (divalidasi),
// perubahan benar-benar sampai ke CPE; setelah tautan dilepas akses hilang lagi (404, bukan 403).
func scenarioPortal(ctx context.Context, w *World) result {
	key := "zte"
	dev, serial := w.devices[key], w.serials[key]
	if dev == nil {
		return skipr("device %s dari S1 tidak ada", key)
	}
	d, err := w.super.findDeviceBySerial(serial)
	if err != nil {
		return failr("%v", err)
	}
	run := w.runID
	user, pw := "s18-"+run, "S18-Pass-"+run+"-Qz7!"

	var created idResp
	if err := w.super.do(http.MethodPost, "/api/v1/users", map[string]any{
		"tenant_id": w.pilotTenantID, "username": user, "email": user + "@example.test", "password": pw,
		"full_name": "S18 Pelanggan", "role_codes": []string{"ENDUSER"},
	}, &created); err != nil {
		return failr("buat akun ENDUSER: %v", err)
	}
	defer func() { _ = w.super.do(http.MethodDelete, fmt.Sprintf("/api/v1/users/%d", created.ID), nil, nil) }()
	link := fmt.Sprintf("/api/v1/users/%d/devices/%d", created.ID, d.ID)

	cust := NewClient(w.super.base, false)
	if err := cust.Login(user, pw); err != nil {
		return failr("login ENDUSER: %v", err)
	}

	// 1. belum ditautkan: daftar kosong, akses perangkat 404 (tidak membocorkan keberadaan)
	var list struct {
		Data  []map[string]any `json:"data"`
		Total int              `json:"total"`
	}
	if err := cust.do(http.MethodGet, "/api/v1/self-service/devices", nil, &list); err != nil || len(list.Data) != 0 {
		return failr("sebelum ditautkan daftar harus kosong: %v %v", err, list.Data)
	}
	if st, _ := cust.status(http.MethodGet, fmt.Sprintf("/api/v1/self-service/devices/%d", d.ID), nil); st != 404 {
		return failr("perangkat belum ditautkan harus 404, dapat %d", st)
	}
	diagURL := fmt.Sprintf("/api/v1/self-service/devices/%d/diagnosis", d.ID)
	if st, _ := cust.status(http.MethodGet, diagURL, nil); st != 404 {
		return failr("PORTAL-DIAG: diagnosis perangkat yang belum ditautkan harus 404, dapat %d", st)
	}

	// 2. kontrol akses penautan
	if st, _ := w.pilot2Admin.status(http.MethodPut, link, nil); st != 403 && st != 404 {
		return failr("ADMIN tenant lain menautkan harus ditolak, dapat %d", st)
	}
	if st, _ := cust.status(http.MethodPut, link, nil); st != 403 {
		return failr("ENDUSER menautkan sendiri harus 403, dapat %d", st)
	}
	if st, body := w.pilotAdmin.status(http.MethodPut, link, nil); st >= 400 {
		return failr("ADMIN tenant sama menautkan: HTTP %d %s", st, body)
	}
	var linked struct {
		Data []struct {
			SerialNumber string `json:"serial_number"`
		} `json:"data"`
	}
	if err := w.pilotAdmin.do(http.MethodGet, fmt.Sprintf("/api/v1/users/%d/devices", created.ID), nil, &linked); err != nil || len(linked.Data) != 1 || linked.Data[0].SerialNumber != serial {
		return failr("daftar tautan di sisi admin salah: %v %+v", err, linked.Data)
	}

	// 3. pelanggan melihat tepat satu perangkat
	if err := cust.do(http.MethodGet, "/api/v1/self-service/devices", nil, &list); err != nil || len(list.Data) != 1 {
		return failr("setelah ditautkan harus 1 perangkat: %v %v", err, list.Data)
	}
	if st, _ := cust.status(http.MethodGet, "/api/v1/devices", nil); st != 403 {
		return failr("ENDUSER ke API staf harus 403, dapat %d", st)
	}

	// 3b. "Internet saya bermasalah?": hasil berbahasa awam dari daftar putih, tanpa satu pun nilai internal; akses staf ke rute ini ditolak
	_, _, rawDiag, derr := cust.getRaw(diagURL)
	if derr != nil {
		return failr("PORTAL-DIAG: GET diagnosis: %v", derr)
	}
	var diag map[string]any
	if err := json.Unmarshal(rawDiag, &diag); err != nil || len(diag) != 7 {
		return failr("PORTAL-DIAG: bentuk respons harus tepat 7 field: %v %s", err, rawDiag)
	}
	if c, _ := diag["cause"].(string); c == "" || diag["headline"] == "" || diag["disclaimer"] == "" {
		return failr("PORTAL-DIAG: isi tak lengkap: %s", rawDiag)
	}
	for _, leak := range []string{d.SerialNumber, "tenant", "olt", "OLT", "dBm", "rx_power", "inform", "ManagementServer", "findings", "evidence"} {
		if leak != "" && strings.Contains(string(rawDiag), leak) {
			return failr("PORTAL-DIAG: respons pelanggan membocorkan %q: %s", leak, rawDiag)
		}
	}
	if st, _ := w.pilotAdmin.status(http.MethodGet, diagURL, nil); st != 403 {
		return failr("PORTAL-DIAG: staf di rute pelanggan harus 403, dapat %d", st)
	}
	if st, _ := cust.status(http.MethodGet, "/api/v1/self-service/devices/abc/diagnosis", nil); st != 400 {
		return failr("PORTAL-DIAG: id tak valid harus 400, dapat %d", st)
	}

	// 4. ganti WiFi: validasi lalu sampai ke CPE
	wifi := fmt.Sprintf("/api/v1/self-service/devices/%d/wifi", d.ID)
	if st, _ := cust.status(http.MethodPatch, wifi, map[string]any{"ssid": "x", "passphrase": "pendek"}); st != 400 {
		return failr("passphrase < 8 karakter harus 400, dapat %d", st)
	}
	if st, _ := cust.status(http.MethodPatch, wifi, map[string]any{}); st != 400 {
		return failr("payload kosong harus 400, dapat %d", st)
	}
	newSSID := "Rumah-" + run
	if st, body := cust.status(http.MethodPatch, wifi, map[string]any{"ssid": newSSID, "passphrase": "Rahasia-" + run + "12"}); st >= 400 {
		return failr("PATCH wifi: HTTP %d %s", st, body)
	}
	rep := w.runCPE(ctx, dev, simcpe.SessionOpts{Events: []string{"2 PERIODIC"}})
	if !rep.OK {
		return failr("sesi CWMP: %s", rep.ErrMsg)
	}
	applied := false
	for name, val := range dev.Snapshot() {
		if strings.HasSuffix(name, "SSID") && val == newSSID {
			applied = true
		}
	}
	if !applied {
		return failr("SSID baru tidak sampai ke CPE (rpc: %v; task: %s)", rep.RPCsReceived, w.taskDump(d.ID))
	}

	// 5. lepas tautan: akses hilang lagi
	if st, _ := w.pilotAdmin.status(http.MethodDelete, link, nil); st >= 400 {
		return failr("melepas tautan: HTTP %d", st)
	}
	if st, _ := cust.status(http.MethodGet, fmt.Sprintf("/api/v1/self-service/devices/%d", d.ID), nil); st != 404 {
		return failr("setelah dilepas harus 404, dapat %d", st)
	}
	if st, _ := cust.status(http.MethodGet, diagURL, nil); st != 404 {
		return failr("PORTAL-DIAG: setelah dilepas diagnosis harus 404, dapat %d", st)
	}
	return pass("portal pelanggan: diagnosis \"Internet saya bermasalah?\" 7 field tanpa data internal (404 sebelum ditautkan/sesudah dilepas, staf 403);  tanpa tautan tak melihat apa pun (404); tenant lain 403; ADMIN menautkan; pelanggan melihat 1 perangkat, validasi WiFi 400, SSID sampai ke CPE; lepas tautan → 404")
}
