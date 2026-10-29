package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"acs/internal/simcpe"
)

// S38 — peta perangkat, diagnosis offline, dan penemuan interval Inform (irisan A: tabel/peta/status/troubleshoot):
//
//	MAP-01  GET /devices/map: hanya perangkat ber-koordinat yang digambar; total/located jujur; status dan device_uuid ikut; tanpa PK rahasia
//	MAP-02  bbox menyaring (termasuk bbox melintasi bujur 180); masukan buruk 400
//	MAP-04  GET /devices?has_location=true|false menyaring menurut ada/tidaknya koordinat; nilai lain 400
//	MAP-03  isolasi tenant, tanpa token 401, ENDUSER 403
//	DIAG-01 GET /devices/{id}/diagnosis konsisten dengan ambang offline perangkat; perangkat pra-registrasi = NEVER_CONNECTED
//	DIAG-02 akses: tenant lain tak melihat, id tak valid 400, tak ada 404, tanpa token 401, ENDUSER 403
//	IVL-01  perangkat yang TIDAK melaporkan PeriodicInformInterval di Inform (profil nokia, seperti ONT nyata) tetap ditemukan intervalnya lewat tugas baca
func scenarioMapDiagnosis(ctx context.Context, w *World) result {
	zKey, nKey := "zte", "nokia"
	zSerial, nSerial := w.serials[zKey], w.serials[nKey]
	if w.devices[zKey] == nil || w.devices[nKey] == nil {
		return skipr("device %s/%s dari S1 tidak ada", zKey, nKey)
	}
	zDev, err := w.pilotAdmin.findDeviceBySerial(zSerial)
	if err != nil {
		return failr("%v", err)
	}
	nDev, err := w.pilotAdmin.findDeviceBySerial(nSerial)
	if err != nil {
		return failr("%v", err)
	}

	type point struct {
		ID           uint64  `json:"id"`
		DeviceUUID   string  `json:"device_uuid"`
		SerialNumber string  `json:"serial_number"`
		Latitude     float64 `json:"latitude"`
		Longitude    float64 `json:"longitude"`
		Status       string  `json:"status"`
		LastInformAt *string `json:"last_inform_at"`
	}
	type mapResp struct {
		Data      []point `json:"data"`
		Total     int     `json:"total"`
		Located   int     `json:"located"`
		Truncated bool    `json:"truncated"`
	}
	getMap := func(c *Client, kv ...string) (int, mapResp, string) {
		v := url.Values{}
		for i := 0; i+1 < len(kv); i += 2 {
			v.Set(kv[i], kv[i+1])
		}
		var out mapResp
		err := c.do(http.MethodGet, "/api/v1/devices/map?"+v.Encode(), nil, &out)
		if err != nil {
			if ae, ok := err.(*apiError); ok {
				return ae.Status, out, ae.Body
			}
			return 0, out, err.Error()
		}
		return 200, out, ""
	}

	// MAP-01: koordinat Jakarta pada perangkat zte; nokia tetap tanpa koordinat.
	if err := w.pilotAdmin.do(http.MethodPatch, fmt.Sprintf("/api/v1/devices/%d", zDev.ID), map[string]any{"latitude": -6.2088, "longitude": 106.8456}, nil); err != nil {
		return failr("MAP-01 set koordinat: %v", err)
	}
	st, m, body := getMap(w.pilotAdmin, "serial_number", zSerial)
	if st != 200 || len(m.Data) != 1 || m.Total != 1 || m.Located != 1 || m.Truncated {
		return failr("MAP-01: HTTP %d total=%d located=%d titik=%d truncated=%v (%s)", st, m.Total, m.Located, len(m.Data), m.Truncated, body)
	}
	p := m.Data[0]
	if p.DeviceUUID == "" || p.Status == "" || p.SerialNumber != zSerial || p.Latitude != -6.2088 || p.Longitude != 106.8456 || p.LastInformAt == nil {
		return failr("MAP-01: titik tak lengkap/salah: %+v", p)
	}
	st, m, body = getMap(w.pilotAdmin, "serial_number", nSerial)
	if st != 200 || len(m.Data) != 0 || m.Total != 1 || m.Located != 0 {
		return failr("MAP-01: perangkat tanpa koordinat harus total=1 located=0 tanpa titik: HTTP %d %+v (%s)", st, m, body)
	}
	var raw struct {
		Data []any `json:"data"`
	}
	if err := w.pilotAdmin.do(http.MethodGet, "/api/v1/devices/map?serial_number="+url.QueryEscape(nSerial), nil, &raw); err != nil || raw.Data == nil {
		return failr("MAP-01: data kosong harus [] bukan null (%v)", err)
	}
	for _, leaked := range []string{"tenant_id", "connection_request", "password", "inform_username"} {
		var all []byte
		_, _, all, _ = w.pilotAdmin.getRaw("/api/v1/devices/map?serial_number=" + url.QueryEscape(zSerial))
		if strings.Contains(string(all), leaked) {
			return failr("MAP-01: respons membocorkan %q: %s", leaked, all)
		}
	}

	// MAP-02
	jakarta := "106,-7,108,-6"
	if st, m, _ = getMap(w.pilotAdmin, "serial_number", zSerial, "bbox", jakarta); st != 200 || len(m.Data) != 1 {
		return failr("MAP-02: bbox Jakarta harus memuat perangkat (HTTP %d, %d titik)", st, len(m.Data))
	}
	st, m, _ = getMap(w.pilotAdmin, "serial_number", zSerial, "bbox", "0,40,10,50") // Eropa
	if st != 200 || len(m.Data) != 0 || m.Total != 1 || m.Located != 1 {
		return failr("MAP-02: bbox Eropa harus kosong tetapi total/located tetap menggambarkan filter: HTTP %d %+v", st, m)
	}
	if st, m, _ = getMap(w.pilotAdmin, "serial_number", zSerial, "bbox", "170,-20,-170,-10"); st != 200 || len(m.Data) != 0 {
		return failr("MAP-02: bbox melintasi bujur 180 tidak boleh memuat Jakarta (HTTP %d, %d titik)", st, len(m.Data))
	}
	for name, kv := range map[string][]string{
		"bbox bukan angka":     {"bbox", "a,b,c,d"},
		"bbox kurang elemen":   {"bbox", "1,2,3"},
		"bbox selatan>utara":   {"bbox", "0,10,1,-10"},
		"bbox lintang di luar": {"bbox", "0,-91,1,0"},
		"optical_level buruk":  {"optical_level", "ungu"},
		"param op tak dikenal": {"param", "X~regex~."},
		"serial kepanjangan":   {"serial_number", strings.Repeat("x", 200)},
	} {
		if st, _, b := getMap(w.pilotAdmin, kv...); st != http.StatusBadRequest {
			return failr("MAP-02: %s -> HTTP %d, harusnya 400 (%s)", name, st, b)
		}
	}

	// MAP-04: daftar perangkat dapat disaring menurut ada/tidaknya koordinat (dasar panel "tetapkan lokasi" di peta). zte sudah diberi
	// koordinat di MAP-01, nokia belum.
	hasLoc := func(serial, val string) (int, int) {
		q := url.Values{}
		q.Set("search", serial)
		q.Set("has_location", val)
		devs, err := w.pilotAdmin.listDevices(q)
		if err != nil {
			if ae, ok := err.(*apiError); ok {
				return ae.Status, 0
			}
			return 0, 0
		}
		n := 0
		for _, d := range devs {
			if d.SerialNumber == serial {
				n++
			}
		}
		return 200, n
	}
	for _, c := range []struct {
		serial, val string
		want        int
	}{{zSerial, "true", 1}, {zSerial, "false", 0}, {nSerial, "false", 1}, {nSerial, "true", 0}} {
		if st, n := hasLoc(c.serial, c.val); st != 200 || n != c.want {
			return failr("MAP-04: GET /devices?search=%s&has_location=%s -> HTTP %d, %d cocok (harusnya %d)", c.serial, c.val, st, n, c.want)
		}
	}
	if st, _ := hasLoc(zSerial, "mungkin"); st != http.StatusBadRequest {
		return failr("MAP-04: has_location tidak valid harus 400, dapat %d", st)
	}

	// MAP-03
	if st, m, _ = getMap(w.pilot2Admin, "serial_number", zSerial); st != 200 || len(m.Data) != 0 || m.Total != 0 {
		return failr("MAP-03: tenant lain melihat perangkat di peta (HTTP %d %+v)", st, m)
	}
	if st, m, _ = getMap(w.pilot2Admin, "serial_number", zSerial, "tenant_id", strconv.FormatUint(w.pilotTenantID, 10)); st != 200 || len(m.Data) != 0 {
		return failr("MAP-03: tenant lain menyuntik tenant_id dan melihat perangkat (HTTP %d, %d titik)", st, len(m.Data))
	}
	if st, m, _ = getMap(w.super, "serial_number", zSerial, "tenant_id", strconv.FormatUint(w.pilotTenantID, 10)); st != 200 || len(m.Data) != 1 {
		return failr("MAP-03: superadmin harus bisa memilih tenant (HTTP %d, %d titik)", st, len(m.Data))
	}
	anon := NewClient(w.super.base, false)
	if st, _, _ = getMap(anon, "serial_number", zSerial); st != http.StatusUnauthorized {
		return failr("MAP-03: tanpa token harus 401, dapat %d", st)
	}
	user, pw := "s38-"+w.runID, "S38-Pass-"+w.runID+"-Qz7!"
	var created idResp
	if err := w.super.do(http.MethodPost, "/api/v1/users", map[string]any{
		"tenant_id": w.pilotTenantID, "username": user, "email": user + "@example.test", "password": pw,
		"full_name": "S38 Pelanggan", "role_codes": []string{"ENDUSER"},
	}, &created); err != nil {
		return failr("MAP-03: buat akun ENDUSER: %v", err)
	}
	defer func() { _ = w.super.do(http.MethodDelete, fmt.Sprintf("/api/v1/users/%d", created.ID), nil, nil) }()
	cust := NewClient(w.super.base, false)
	if err := cust.Login(user, pw); err != nil {
		return failr("MAP-03: login ENDUSER: %v", err)
	}
	if st, _, _ = getMap(cust, "serial_number", zSerial); st != http.StatusForbidden {
		return failr("MAP-03: ENDUSER harus 403, dapat %d", st)
	}

	// DIAG-01
	type finding struct {
		Code, Severity, Title, Evidence, Hint string
	}
	type diagResp struct {
		DeviceID                uint64    `json:"device_id"`
		Status                  string    `json:"status"`
		Verdict                 string    `json:"verdict"`
		Summary                 string    `json:"summary"`
		LastInformAt            *string   `json:"last_inform_at"`
		SilentSeconds           *int64    `json:"silent_seconds"`
		InformIntervalSeconds   *uint32   `json:"inform_interval_seconds"`
		InformIntervalParameter string    `json:"inform_interval_parameter"`
		OfflineThresholdSeconds int64     `json:"offline_threshold_seconds"`
		Findings                []finding `json:"findings"`
		Actions                 []string  `json:"actions"`
		Limits                  string    `json:"limits"`
	}
	getDiag := func(c *Client, id string) (int, diagResp, string) {
		var out diagResp
		err := c.do(http.MethodGet, "/api/v1/devices/"+id+"/diagnosis", nil, &out)
		if err != nil {
			if ae, ok := err.(*apiError); ok {
				return ae.Status, out, ae.Body
			}
			return 0, out, err.Error()
		}
		return 200, out, ""
	}
	st, d, body := getDiag(w.pilotAdmin, strconv.FormatUint(zDev.ID, 10))
	if st != 200 || d.DeviceID != zDev.ID || d.Limits == "" || d.Findings == nil || d.Actions == nil || d.LastInformAt == nil || d.SilentSeconds == nil {
		return failr("DIAG-01: HTTP %d, respons tak lengkap: %+v (%s)", st, d, body)
	}
	if d.OfflineThresholdSeconds < 15*60 {
		return failr("DIAG-01: ambang offline %d dtk di bawah minimum 15 menit", d.OfflineThresholdSeconds)
	}
	// Konsisten dengan aturan penyapu (offline hanya bila diam LEBIH dari ambang) — tidak bergantung pada lamanya suite berjalan.
	wantVerdict := "REACHABLE"
	if *d.SilentSeconds > d.OfflineThresholdSeconds {
		wantVerdict = "OFFLINE"
	}
	if d.Verdict != wantVerdict {
		return failr("DIAG-01: verdict %s padahal diam %d dtk dan ambang %d dtk (mau %s)", d.Verdict, *d.SilentSeconds, d.OfflineThresholdSeconds, wantVerdict)
	}
	if d.Summary == "" {
		return failr("DIAG-01: summary kosong")
	}

	var pre struct {
		ID uint64 `json:"id"`
	}
	preSerial := "S38P" + w.runID
	if err := w.pilotAdmin.do(http.MethodPost, "/api/v1/devices", map[string]any{"oui": "ABCDEF", "serial_number": preSerial}, &pre); err != nil {
		return failr("DIAG-01: pra-registrasi: %v", err)
	}
	defer func() { _ = w.pilotAdmin.do(http.MethodDelete, fmt.Sprintf("/api/v1/devices/%d", pre.ID), nil, nil) }()
	st, d, body = getDiag(w.pilotAdmin, strconv.FormatUint(pre.ID, 10))
	if st != 200 || d.Verdict != "NEVER_CONNECTED" || d.SilentSeconds != nil || d.LastInformAt != nil {
		return failr("DIAG-01: pra-registrasi harus NEVER_CONNECTED tanpa waktu diam: HTTP %d %+v (%s)", st, d, body)
	}
	found := false
	for _, f := range d.Findings {
		found = found || f.Code == "NEVER_INFORMED"
	}
	if !found {
		return failr("DIAG-01: temuan NEVER_INFORMED tidak ada: %+v", d.Findings)
	}

	// DIAG-02
	zid := strconv.FormatUint(zDev.ID, 10)
	if st, _, _ = getDiag(w.pilot2Admin, zid); st != http.StatusNotFound && st != http.StatusForbidden {
		return failr("DIAG-02: tenant lain harus 404/403, dapat %d", st)
	}
	if st, _, _ = getDiag(w.pilotAdmin, "abc"); st != http.StatusBadRequest {
		return failr("DIAG-02: id bukan angka harus 400, dapat %d", st)
	}
	if st, _, _ = getDiag(w.pilotAdmin, "999999999"); st != http.StatusNotFound {
		return failr("DIAG-02: id tak ada harus 404, dapat %d", st)
	}
	if st, _, _ = getDiag(anon, zid); st != http.StatusUnauthorized {
		return failr("DIAG-02: tanpa token harus 401, dapat %d", st)
	}
	if st, _, _ = getDiag(cust, zid); st != http.StatusForbidden {
		return failr("DIAG-02: ENDUSER harus 403, dapat %d", st)
	}

	// IVL-01: nokia tidak melaporkan interval di Inform (OmitPeriodicInformInterval); ACS harus menemukannya lewat tugas baca. Penemuan
	// hanya mengantre bila perangkat tidak punya task terbuka pada Inform itu, jadi beberapa Inform periodik dipicu (tiap sesi
	// menghabiskan tugas yang antre, termasuk poll optik/PPPoE) sampai interval terisi.
	var nokia struct {
		InformIntervalSeconds *uint32 `json:"inform_interval_seconds"`
	}
	for attempt := 0; attempt < 4; attempt++ {
		if err := w.pilotAdmin.do(http.MethodGet, fmt.Sprintf("/api/v1/devices/%d", nDev.ID), nil, &nokia); err != nil {
			return failr("IVL-01: %v", err)
		}
		if nokia.InformIntervalSeconds != nil {
			break
		}
		if rep := w.runCPE(ctx, w.devices[nKey], simcpe.SessionOpts{Events: []string{"2 PERIODIC"}}); !rep.OK {
			return failr("IVL-01: sesi Inform periodik nokia gagal: %s", rep.ErrMsg)
		}
	}
	if err := w.pilotAdmin.do(http.MethodGet, fmt.Sprintf("/api/v1/devices/%d", nDev.ID), nil, &nokia); err != nil {
		return failr("IVL-01: %v", err)
	}
	if nokia.InformIntervalSeconds == nil || *nokia.InformIntervalSeconds != 300 {
		return failr("IVL-01: interval nokia = %v setelah 4 Inform, harusnya 300 (dibaca lewat tugas GetParameterValues; TR-181 root Device.)", nokia.InformIntervalSeconds)
	}
	st, d, body = getDiag(w.pilotAdmin, strconv.FormatUint(nDev.ID, 10))
	if d.InformIntervalParameter != "Device.ManagementServer.PeriodicInformInterval" {
		return failr("IVL-01: nama parameter interval nokia (TR-181) = %q", d.InformIntervalParameter)
	}
	if st != 200 || d.InformIntervalSeconds == nil || *d.InformIntervalSeconds != 300 || d.OfflineThresholdSeconds != 15*60 {
		return failr("IVL-01: diagnosis nokia harus membawa interval 300 dan ambang 900: HTTP %d %+v (%s)", st, d, body)
	}
	// Perangkat zte melaporkan interval di Inform: harus sama-sama terisi (jalur penangkapan Inform).
	var zte struct {
		InformIntervalSeconds *uint32 `json:"inform_interval_seconds"`
	}
	if err := w.pilotAdmin.do(http.MethodGet, fmt.Sprintf("/api/v1/devices/%d", zDev.ID), nil, &zte); err != nil || zte.InformIntervalSeconds == nil || *zte.InformIntervalSeconds != 300 {
		return failr("IVL-01: interval zte (dilaporkan di Inform) = %v, harusnya 300 (%v)", zte.InformIntervalSeconds, err)
	}

	if _, zd, _ := getDiag(w.pilotAdmin, zid); zd.InformIntervalParameter != "InternetGatewayDevice.ManagementServer.PeriodicInformInterval" {
		return failr("IVL-01: nama parameter interval zte (TR-098) = %q", zd.InformIntervalParameter)
	}

	return pass("peta: hanya yang berkoordinat digambar, total/located jujur, bbox (termasuk lintas bujur 180) benar, 7 masukan buruk 400, saringan has_location pada daftar perangkat, tenant lain kosong/401/ENDUSER 403; diagnosis konsisten dengan ambang offline, pra-registrasi NEVER_CONNECTED, akses 404/400/401/403; interval Inform ditemukan baik dari Inform (zte) maupun lewat tugas baca (nokia, TR-181)")
}
