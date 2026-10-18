package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// S24 — validasi masukan end-to-end ("jangan percaya input pengguna"): query-string rusak ditolak 400 (bukan
// diam-diam jadi tanpa filter), user/tenant dengan nilai cacat ditolak di POST maupun PATCH, parameter task
// melewati batas ditolak, endpoint publik menolak payload raksasa, dan Inform dengan identitas cacat ditolak 400
// (bukan 500 yang membuat CPE tak bisa Inform).
func scenarioValidation(ctx context.Context, w *World) result {
	// 1. query-string
	for _, q := range []string{
		"page=0", "page=abc", "page=-3", "page=100001", "page_size=0", "page_size=201", "page_size=x",
		"tenant_id=abc", "device_status_id=-1", "vendor_id=0",
	} {
		if st, b := w.super.status(http.MethodGet, "/api/v1/devices?"+q, nil); st != 400 {
			return failr("GET /devices?%s harus 400, dapat %d %s", q, st, b)
		}
	}
	if st, b := w.super.status(http.MethodGet, "/api/v1/devices?page=1&page_size=200", nil); st != 200 {
		return failr("page_size=200 (batas) harus 200, dapat %d %s", st, b)
	}
	if st, b := w.super.status(http.MethodGet, "/api/v1/devices?search="+strings.Repeat("a", 300), nil); st != 400 {
		return failr("search 300 karakter harus 400, dapat %d %s", st, b)
	}

	// 2. user & tenant: nilai cacat ditolak
	run := w.runID
	badUsers := map[string]map[string]any{
		"username berspasi":       {"username": "a b c", "email": "s24a-" + run + "@example.test", "password": "S24-Pass-" + run + "-Qz7!", "full_name": "S24 Uji"},
		"username terlalu pendek": {"username": "ab", "email": "s24b-" + run + "@example.test", "password": "S24-Pass-" + run + "-Qz7!", "full_name": "S24 Uji"},
		"username berisi newline": {"username": "s24c\nx" + run, "email": "s24c-" + run + "@example.test", "password": "S24-Pass-" + run + "-Qz7!", "full_name": "S24 Uji"},
		"nama kosong":             {"username": "s24d-" + run, "email": "s24d-" + run + "@example.test", "password": "S24-Pass-" + run + "-Qz7!", "full_name": "   "},
		"password = username":     {"username": "s24e-pass-" + run, "email": "s24e-" + run + "@example.test", "password": "s24e-pass-" + run, "full_name": "S24 Uji"},
		"password 129 karakter":   {"username": "s24f-" + run, "email": "s24f-" + run + "@example.test", "password": strings.Repeat("p", 129), "full_name": "S24 Uji"},
		"role_codes 6 item":       {"username": "s24g-" + run, "email": "s24g-" + run + "@example.test", "password": "S24-Pass-" + run + "-Qz7!", "full_name": "S24 Uji", "role_codes": []string{"VIEWER", "VIEWER", "VIEWER", "VIEWER", "VIEWER", "VIEWER"}},
	}
	for name, p := range badUsers {
		p["tenant_id"] = w.pilotTenantID
		if st, b := w.super.status(http.MethodPost, "/api/v1/users", p); st != 400 {
			return failr("buat user (%s) harus 400, dapat %d %s", name, st, b)
		}
	}
	for name, p := range map[string]map[string]any{
		"code berspasi":      {"code": "S24 " + run, "name": "S24 Tenant"},
		"code newline":       {"code": "S24\n" + run, "name": "S24 Tenant"},
		"name spasi saja":    {"code": "S24-" + run, "name": "   "},
		"inform user ':'":    {"code": "S24I-" + run, "name": "S24 Tenant", "cwmp_inform_username": "a:b-" + run, "cwmp_inform_password": "Rahasia-Inform-" + run + "-123"},
		"inform pass pendek": {"code": "S24J-" + run, "name": "S24 Tenant", "cwmp_inform_username": "s24j-" + run, "cwmp_inform_password": "pendek"},
	} {
		if st, b := w.super.status(http.MethodPost, "/api/v1/tenants", p); st != 400 {
			return failr("buat tenant (%s) harus 400, dapat %d %s", name, st, b)
		}
	}
	// PATCH tidak boleh meloloskan nilai yang ditolak POST (aturan Create = Update)
	tp := fmt.Sprintf("/api/v1/tenants/%d", w.pilotTenantID)
	for name, p := range map[string]map[string]any{
		"PATCH code berspasi": {"code": "a b"},
		"PATCH name kosong":   {"name": "  "},
	} {
		if st, b := w.super.status(http.MethodPatch, tp, p); st != 400 {
			return failr("%s harus 400, dapat %d %s", name, st, b)
		}
	}

	// 3. parameter task melewati batas
	devs, err := w.pilotAdmin.listDevices(nil)
	if err != nil {
		return failr("list perangkat pilot: %v", err)
	}
	taskNote := "parameter task >batas 400"
	if len(devs) == 0 {
		taskNote = "bagian task DILEWATI (tak ada perangkat; jalankan bersama S1)"
	}
	dev := uint64(0)
	if len(devs) > 0 {
		dev = devs[0].ID
	}
	if dev != 0 {
		names := make([]string, 201)
		for i := range names {
			names[i] = fmt.Sprintf("InternetGatewayDevice.DeviceInfo.X_%d", i)
		}
		if st, b := w.pilotAdmin.status(http.MethodPost, fmt.Sprintf("/api/v1/devices/%d/tasks/get-parameter-values", dev), map[string]any{"names": names}); st != 400 {
			return failr("get-parameter-values 201 nama harus 400, dapat %d %s", st, b)
		}
		if st, b := w.pilotAdmin.status(http.MethodPost, "/api/v1/tasks", map[string]any{
			"device_id": dev, "task_type": "GET_PARAMETER_VALUES", "parameters": map[string]any{"names": names},
		}); st != 400 {
			return failr("POST /tasks 201 nama harus 400, dapat %d %s", st, b)
		}
		if st, b := w.pilotAdmin.status(http.MethodPost, "/api/v1/tasks", map[string]any{
			"device_id": dev, "task_type": "SET_PARAMETER_VALUES",
			"parameters": map[string]any{"values": map[string]string{"InternetGatewayDevice.X": strings.Repeat("v", 8193)}},
		}); st != 400 {
			return failr("nilai parameter 8193 karakter harus 400, dapat %d %s", st, b)
		}

	}

	// 4. endpoint publik menolak payload raksasa SEBELUM menyentuh DB / hashing
	anon := NewClient(w.super.base, false)
	if st, b := anon.status(http.MethodPost, "/api/v1/auth/login", map[string]any{"email": strings.Repeat("a", 300) + "@x.test", "password": "x"}); st != 400 {
		return failr("login email 300+ karakter harus 400, dapat %d %s", st, b)
	}
	if st, b := anon.status(http.MethodPost, "/api/v1/auth/reset-password", map[string]any{"token": strings.Repeat("t", 600), "new_password": "Rahasia-Baru-123"}); st != 400 {
		return failr("reset token 600 karakter harus 400, dapat %d %s", st, b)
	}

	// 5. Inform dengan identitas cacat -> 400 (bukan 500)
	for name, id := range map[string][2]string{
		"OUI 11 karakter":     {"TOOLONGOUI1", "S24" + run},
		"OUI kosong":          {"", "S24" + run},
		"serial kosong":       {"00D0D0", ""},
		"serial 129 karakter": {"00D0D0", strings.Repeat("s", 129)},
	} {
		body := informEnvelope(id[0], id[1])
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, w.cwmpURL, strings.NewReader(body))
		req.Header.Set("Content-Type", "text/xml; charset=utf-8")
		req.SetBasicAuth(w.cwmpUser, w.cwmpPass)
		resp, err := w.hc.Do(req)
		if err != nil {
			return failr("kirim Inform (%s): %v", name, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			return failr("Inform %s harus 400, dapat %d", name, resp.StatusCode)
		}
	}

	return result{ok: true, detail: "query rusak (page/page_size/*_id/search) 400; user/tenant cacat 400 di POST & PATCH; " + taskNote + " (endpoint khusus & POST /tasks); login/reset payload raksasa 400; Inform identitas cacat 400"}
}

func informEnvelope(oui, serial string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>` +
		`<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/" xmlns:cwmp="urn:dslforum-org:cwmp-1-2">` +
		`<soap:Header><cwmp:ID soap:mustUnderstand="1">s24</cwmp:ID></soap:Header><soap:Body><cwmp:Inform>` +
		`<DeviceId><Manufacturer>S24</Manufacturer><OUI>` + oui + `</OUI><ProductClass>S24</ProductClass><SerialNumber>` + serial + `</SerialNumber></DeviceId>` +
		`<Event><EventStruct><EventCode>0 BOOTSTRAP</EventCode><CommandKey></CommandKey></EventStruct></Event>` +
		`<MaxEnvelopes>1</MaxEnvelopes><CurrentTime>2026-01-01T00:00:00Z</CurrentTime><RetryCount>0</RetryCount>` +
		`<ParameterList></ParameterList></cwmp:Inform></soap:Body></soap:Envelope>`
}
