package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"acs/internal/simcpe"
)

// S29 — capture sesi CWMP per perangkat: saklar (nyala/mati/validasi/RBAC/isolasi tenant), pesan SOAP dua arah terekam,
// nilai rahasia DISAMARKAN sebelum tersimpan (daftar, detail, dan ekspor NDJSON), ACS mengirim SetParameterValues dengan
// atribut xsi:type literal (regresi T-07: bukan _XMLSchema-instance), dan setelah dimatikan tidak ada pesan baru.
func scenarioCapture(ctx context.Context, w *World) result {
	key := "huawei"
	dev, serial := w.devices[key], w.serials[key]
	if dev == nil {
		return skipr("device %s dari S1 tidak ada", key)
	}
	d, err := w.super.findDeviceBySerial(serial)
	if err != nil {
		return failr("%v", err)
	}
	base := fmt.Sprintf("/api/v1/devices/%d", d.ID)

	type status struct {
		Active bool       `json:"active"`
		Until  *time.Time `json:"until"`
	}
	var st status
	if err := w.pilotAdmin.do(http.MethodGet, base+"/capture", nil, &st); err != nil || st.Active {
		return failr("status awal harus tidak aktif (err=%v st=%+v)", err, st)
	}

	// validasi, RBAC, dan isolasi tenant
	for name, body := range map[string]map[string]any{
		"minutes 0":    {"minutes": 0},
		"minutes 61":   {"minutes": 61},
		"minutes -5":   {"minutes": -5},
		"field asing":  {"minutes": 5, "id": 7},
		"tanpa minute": {},
	} {
		if c, b := w.pilotAdmin.status(http.MethodPost, base+"/capture", body); c != http.StatusBadRequest {
			return failr("%s harus 400, dapat %d %s", name, c, b)
		}
	}
	if c, _ := w.pilot2Admin.status(http.MethodPost, base+"/capture", map[string]any{"minutes": 5}); c != http.StatusForbidden && c != http.StatusNotFound {
		return failr("ADMIN tenant lain menyalakan capture harus 403/404, dapat %d", c)
	}
	if c, _ := w.pilot2Admin.status(http.MethodGet, base+"/captures", nil); c != http.StatusForbidden && c != http.StatusNotFound {
		return failr("ADMIN tenant lain membaca daftar capture harus 403/404, dapat %d", c)
	}
	var none struct {
		Total int `json:"total"`
	}
	if err := w.pilotAdmin.do(http.MethodGet, base+"/captures", nil, &none); err != nil || none.Total != 0 {
		return failr("sebelum dinyalakan tidak boleh ada pesan (err=%v total=%d)", err, none.Total)
	}

	// nyalakan, lalu kirim SetParameterValues yang memuat nilai RAHASIA dan jalankan sesi CPE
	if err := w.pilotAdmin.do(http.MethodPost, base+"/capture", map[string]any{"minutes": 5}, &st); err != nil || !st.Active || st.Until == nil {
		return failr("menyalakan capture: err=%v st=%+v", err, st)
	}
	secret := "e2e-rahasia-" + w.runID
	pskPath := simcpe.Profiles[key].RootPrefix() + "LANDevice.1.WLANConfiguration.1.KeyPassphrase"
	if c, b := w.pilotAdmin.status(http.MethodPost, base+"/tasks/set-parameter-values", map[string]any{"values": map[string]string{pskPath: secret}}); c >= 400 {
		return failr("POST set-parameter-values: HTTP %d %s", c, b)
	}
	for i := 0; i < 2; i++ {
		if rep := w.runCPE(ctx, dev, simcpe.SessionOpts{Events: []string{"2 PERIODIC"}}); !rep.OK {
			return failr("sesi CPE %d gagal: %s", i+1, rep.ErrMsg)
		}
	}
	time.Sleep(300 * time.Millisecond)

	type msg struct {
		ID         string `json:"id"`
		FromCPE    bool   `json:"from_cpe"`
		RPCName    string `json:"rpc_name"`
		HTTPStatus *int   `json:"http_status"`
		Body       string `json:"body"`
	}
	var list struct {
		Data  []msg `json:"data"`
		Total int   `json:"total"`
	}
	if err := w.pilotAdmin.do(http.MethodGet, base+"/captures?page_size=100", nil, &list); err != nil {
		return failr("GET captures: %v", err)
	}
	var sawInform, sawInformResp, sawSPV bool
	redacted := false
	for _, m := range list.Data {
		if m.Body != "" {
			return failr("daftar tidak boleh membawa body (id %s)", m.ID)
		}
		var full msg
		if err := w.pilotAdmin.do(http.MethodGet, base+"/captures/"+m.ID, nil, &full); err != nil {
			return failr("GET capture %s: %v", m.ID, err)
		}
		if strings.Contains(full.Body, secret) {
			return failr("RAHASIA BOCOR di pesan %s (%s)", m.ID, m.RPCName)
		}
		redacted = redacted || strings.Contains(full.Body, "***REDACTED***")
		switch {
		case m.FromCPE && m.RPCName == "Inform":
			sawInform = true
		case !m.FromCPE && m.RPCName == "InformResponse":
			sawInformResp = true
		case !m.FromCPE && m.RPCName == "SetParameterValues":
			sawSPV = true
			// regresi T-07: ACS harus menulis xsi:type literal, tidak pernah _XMLSchema-instance
			if !strings.Contains(full.Body, `xsi:type="xsd:string"`) || strings.Contains(full.Body, "_XMLSchema-instance") {
				return failr("SetParameterValues keluar dengan atribut tipe yang salah: %s", full.Body)
			}
		}
	}
	if !sawInform || !sawInformResp || !sawSPV {
		return failr("pesan kunci tidak terekam (Inform=%v InformResponse=%v SetParameterValues=%v dari %d pesan)", sawInform, sawInformResp, sawSPV, len(list.Data))
	}
	if !redacted {
		return failr("tidak ada nilai yang disamarkan padahal SetParameterValues memuat passphrase")
	}

	// ekspor NDJSON: hanya pemilik; isi tersamarkan
	code, ctype, raw, err := w.pilotAdmin.getRaw(base + "/captures/export")
	if err != nil || code != http.StatusOK || !strings.Contains(ctype, "ndjson") {
		return failr("ekspor: HTTP %d %s err=%v", code, ctype, err)
	}
	if bytes.Contains(raw, []byte(secret)) {
		return failr("RAHASIA BOCOR di ekspor NDJSON")
	}
	lines := 0
	sc := bufio.NewScanner(bytes.NewReader(raw))
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		if strings.TrimSpace(sc.Text()) != "" {
			lines++
		}
	}
	if lines < len(list.Data) {
		return failr("ekspor kurang pesan: %d baris < %d pesan", lines, len(list.Data))
	}
	if c, _, _, _ := w.pilot2Admin.getRaw(base + "/captures/export"); c != http.StatusForbidden && c != http.StatusNotFound {
		return failr("ekspor oleh tenant lain harus 403/404, dapat %d", c)
	}
	if c, _ := w.pilot2Admin.status(http.MethodGet, base+"/captures/"+list.Data[0].ID, nil); c != http.StatusForbidden && c != http.StatusNotFound {
		return failr("detail pesan oleh tenant lain harus 403/404, dapat %d", c)
	}
	// UUID milik perangkat lain / ngawur: 404, bukan bocor lintas perangkat
	if c, _ := w.pilotAdmin.status(http.MethodGet, base+"/captures/00000000-0000-0000-0000-000000000000", nil); c != http.StatusNotFound {
		return failr("UUID tak dikenal harus 404, dapat %d", c)
	}

	// dimatikan: tidak ada pesan baru
	before := list.Total
	if c, b := w.pilotAdmin.status(http.MethodDelete, base+"/capture", nil); c >= 400 {
		return failr("mematikan capture: HTTP %d %s", c, b)
	}
	if rep := w.runCPE(ctx, dev, simcpe.SessionOpts{Events: []string{"2 PERIODIC"}}); !rep.OK {
		return failr("sesi CPE setelah dimatikan gagal: %s", rep.ErrMsg)
	}
	time.Sleep(300 * time.Millisecond)
	var after struct {
		Total int `json:"total"`
	}
	if err := w.pilotAdmin.do(http.MethodGet, base+"/captures", nil, &after); err != nil || after.Total != before {
		return failr("setelah dimatikan tidak boleh ada pesan baru: %d -> %d (err=%v)", before, after.Total, err)
	}
	if err := w.pilotAdmin.do(http.MethodGet, base+"/capture", nil, &st); err != nil || st.Active {
		return failr("status setelah dimatikan harus tidak aktif: err=%v st=%+v", err, st)
	}
	return pass("capture: validasi 400 & isolasi tenant; %d pesan dua arah terekam (Inform, InformResponse, SetParameterValues dengan xsi:type literal); passphrase tersamarkan di daftar/detail/ekspor NDJSON (%d baris); dimatikan = berhenti merekam", before, lines)
}
