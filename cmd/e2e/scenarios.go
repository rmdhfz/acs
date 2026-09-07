package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"acs/internal/simcpe"
)

// S1 — onboarding lintas 5 vendor: BOOTSTRAP -> device tercatat, tenant_id
// ter-assign dari shared secret, vendor_id ter-resolve dari OUI, event
// "0 BOOTSTRAP" tersimpan.
func scenarioOnboarding(ctx context.Context, w *World) result {
	var okVendors []string
	for _, key := range simcpe.ProfileKeys {
		prof := simcpe.Profiles[key]
		serial := fmt.Sprintf("E2E-%s-%s-01", strings.ToUpper(key), strings.ToUpper(w.runID))
		dev := simcpe.NewDevice(prof, serial)

		rep := w.runCPE(ctx, dev, simcpe.SessionOpts{Events: []string{"0 BOOTSTRAP"}})
		if !rep.OK {
			return failr("%s: sesi CWMP gagal: %s", key, rep.ErrMsg)
		}
		if rep.InformStatus != 200 {
			return failr("%s: Inform HTTP %d", key, rep.InformStatus)
		}

		d, err := w.super.findDeviceBySerial(serial)
		if err != nil {
			return failr("%s: %v", key, err)
		}
		if d.TenantID == nil || *d.TenantID != w.pilotTenantID {
			return failr("%s: tenant_id device = %v, harusnya %d (assign dari shared secret CWMP)", key, d.TenantID, w.pilotTenantID)
		}
		if d.VendorID == nil || *d.VendorID != w.vendorIDByKey[key] {
			return failr("%s: vendor_id = %v, harusnya %d (resolve dari OUI %s)", key, d.VendorID, w.vendorIDByKey[key], prof.OUI)
		}
		evs, err := w.super.listDeviceEvents(d.ID)
		if err != nil {
			return failr("%s: GET events: %v", key, err)
		}
		if !w.eventsContain(evs, "0 BOOTSTRAP") {
			return failr("%s: event '0 BOOTSTRAP' tidak tercatat (dapat: %s)", key, w.eventCodes(evs))
		}

		w.devices[key] = dev
		w.serials[key] = serial
		okVendors = append(okVendors, key)
	}
	return pass("5 vendor onboarding lengkap (%s) — tenant & vendor ter-resolve, event tercatat", strings.Join(okVendors, ", "))
}

// S2 — provisioning: buat profile (raw path SSID), apply ke device, sesi
// berikutnya CPE menerima SetParameterValues, data model berubah, task COMPLETED.
func scenarioProvisioning(ctx context.Context, w *World) result {
	key := "zte"
	dev, serial := w.devices[key], w.serials[key]
	if dev == nil {
		return skipr("device %s dari S1 tidak ada", key)
	}
	d, err := w.super.findDeviceBySerial(serial)
	if err != nil {
		return failr("%v", err)
	}
	path := simcpe.Profiles[key].RootPrefix() + "LANDevice.1.WLANConfiguration.1.SSID"
	target := "ACS-Prov-" + w.runID

	var prof struct {
		ID uint64 `json:"id"`
	}
	err = w.pilotAdmin.do(http.MethodPost, "/api/v1/provisioning-profiles", map[string]any{
		"tenant_id": w.pilotTenantID,
		"name":      "e2e-ssid-" + w.runID,
		"parameters": []map[string]any{
			{"parameter_name": path, "parameter_value": target, "apply_order": 1},
		},
	}, &prof)
	if err != nil {
		return failr("buat profile: %v", err)
	}
	if st, body := w.pilotAdmin.status(http.MethodPost, fmt.Sprintf("/api/v1/devices/%d/apply-profile/%d", d.ID, prof.ID), nil); st >= 400 {
		return failr("apply-profile: HTTP %d %s", st, body)
	}

	rep := w.runCPE(ctx, dev, simcpe.SessionOpts{Events: []string{"2 PERIODIC"}})
	if !rep.OK {
		return failr("sesi CWMP: %s", rep.ErrMsg)
	}
	if rep.CountRPC("SetParameterValues") == 0 {
		return failr("CPE tidak menerima SetParameterValues (rpc: %v)", rep.RPCsReceived)
	}
	if got, _ := dev.Param(path); got != target {
		return failr("nilai di CPE = %q, harusnya %q", got, target)
	}
	if !w.deviceHasTask(d.ID, "SET_PARAMETER_VALUES", "COMPLETED") {
		return failr("task SET_PARAMETER_VALUES tidak COMPLETED (%s)", w.taskDump(d.ID))
	}
	return pass("profile diterapkan: %s = %q di CPE, task COMPLETED", shortPath(path), target)
}

// S3 — reboot manual: POST /devices/:id/reboot -> CPE menerima Reboot RPC,
// task REBOOT COMPLETED.
func scenarioReboot(ctx context.Context, w *World) result {
	key := "huawei"
	dev, serial := w.devices[key], w.serials[key]
	if dev == nil {
		return skipr("device %s dari S1 tidak ada", key)
	}
	d, err := w.super.findDeviceBySerial(serial)
	if err != nil {
		return failr("%v", err)
	}
	if st, body := w.pilotAdmin.status(http.MethodPost, fmt.Sprintf("/api/v1/devices/%d/reboot", d.ID), map[string]any{}); st >= 400 {
		return failr("POST reboot: HTTP %d %s", st, body)
	}
	rep := w.runCPE(ctx, dev, simcpe.SessionOpts{Events: []string{"2 PERIODIC"}})
	if !rep.OK {
		return failr("sesi CWMP: %s", rep.ErrMsg)
	}
	if rep.CountRPC("Reboot") == 0 {
		return failr("CPE tidak menerima Reboot (rpc: %v)", rep.RPCsReceived)
	}
	if !w.deviceHasTask(d.ID, "REBOOT", "COMPLETED") {
		return failr("task REBOOT tidak COMPLETED (%s)", w.taskDump(d.ID))
	}
	return pass("Reboot RPC diterima CPE, task COMPLETED")
}

// S4 — GetParameterValues ad-hoc: NOC minta baca parameter, CPE menjawab,
// nilainya masuk device_parameters.
func scenarioGetParams(ctx context.Context, w *World) result {
	key := "fiberhome"
	dev, serial := w.devices[key], w.serials[key]
	if dev == nil {
		return skipr("device %s dari S1 tidak ada", key)
	}
	d, err := w.super.findDeviceBySerial(serial)
	if err != nil {
		return failr("%v", err)
	}
	root := simcpe.Profiles[key].RootPrefix()
	names := []string{root + "DeviceInfo.SoftwareVersion", root + "DeviceInfo.UpTime"}
	if st, body := w.pilotAdmin.status(http.MethodPost,
		fmt.Sprintf("/api/v1/devices/%d/tasks/get-parameter-values", d.ID),
		map[string]any{"names": names}); st >= 400 {
		return failr("POST get-parameter-values: HTTP %d %s", st, body)
	}
	rep := w.runCPE(ctx, dev, simcpe.SessionOpts{Events: []string{"2 PERIODIC"}})
	if !rep.OK {
		return failr("sesi CWMP: %s", rep.ErrMsg)
	}
	if rep.CountRPC("GetParameterValues") == 0 {
		return failr("CPE tidak menerima GetParameterValues (rpc: %v)", rep.RPCsReceived)
	}
	if !w.deviceHasTask(d.ID, "GET_PARAMETER_VALUES", "COMPLETED") {
		return failr("task GET_PARAMETER_VALUES tidak COMPLETED (%s)", w.taskDump(d.ID))
	}
	params, _ := w.super.listDeviceParameters(d.ID)
	if !paramsContain(params, root+"DeviceInfo.SoftwareVersion") {
		return failr("SoftwareVersion tidak tersimpan di device_parameters")
	}
	return pass("GetParameterValues dijawab CPE, nilai tersimpan (%d parameter)", len(params))
}

// S5 — fault 9005 Invalid Parameter Name: task harus FAILED PERMANEN dalam
// satu percobaan (retry_count tetap 0), bukan di-retry sampai max_retries.
func scenarioFaultNoRetry(ctx context.Context, w *World) result {
	key := "cdata"
	dev, serial := w.devices[key], w.serials[key]
	if dev == nil {
		return skipr("device %s dari S1 tidak ada", key)
	}
	d, err := w.super.findDeviceBySerial(serial)
	if err != nil {
		return failr("%v", err)
	}
	badPath := simcpe.Profiles[key].RootPrefix() + "X_E2E_BADPARAM_" + w.runID + ".Trigger"
	if st, body := w.pilotAdmin.status(http.MethodPost,
		fmt.Sprintf("/api/v1/devices/%d/tasks/set-parameter-values", d.ID),
		map[string]any{"values": map[string]string{badPath: "x"}}); st >= 400 {
		return failr("POST set-parameter-values: HTTP %d %s", st, body)
	}
	rep := w.runCPE(ctx, dev, simcpe.SessionOpts{
		Events: []string{"2 PERIODIC"}, FaultParamSubstr: "X_E2E_BADPARAM",
	})
	if !rep.OK {
		return failr("sesi CWMP: %s", rep.ErrMsg)
	}
	if len(rep.FaultsSent) == 0 {
		return failr("CPE tidak membalas Fault (rpc: %v)", rep.RPCsReceived)
	}
	// Beri ACS sesaat memproses fault.
	time.Sleep(500 * time.Millisecond)
	tasks, _ := w.super.listDeviceTasks(d.ID)
	var setTask *taskJSON
	for i := range tasks {
		if w.taskTypeCode(tasks[i]) == "SET_PARAMETER_VALUES" {
			t := tasks[i]
			setTask = &t
		}
	}
	if setTask == nil {
		return failr("task SET_PARAMETER_VALUES tidak ditemukan")
	}
	status := w.taskStatusCode(setTask.TaskStatusID)
	if status != "FAILED" {
		return failr("task status = %s, harusnya FAILED (fault 9005 = permanen)", status)
	}
	// FailPermanently mencatat tepat 1 percobaan (by design) lalu langsung
	// FAILED — retry_count TIDAK boleh naik mendekati max_retries.
	if setTask.RetryCount > 1 {
		return failr("retry_count = %d — task di-retry (9005 seharusnya gagal permanen dalam 1 percobaan)", setTask.RetryCount)
	}
	// Sesi kedua: pastikan task TIDAK dikirim ulang.
	rep2 := w.runCPE(ctx, dev, simcpe.SessionOpts{Events: []string{"2 PERIODIC"}, FaultParamSubstr: "X_E2E_BADPARAM"})
	if rep2.OK && rep2.CountRPC("SetParameterValues") > 0 {
		return failr("task 9005 dikirim ulang di sesi berikutnya (harusnya permanen FAILED)")
	}
	return pass("fault 9005 -> task FAILED permanen (1 percobaan, retry_count=%d), tidak dikirim ulang", setTask.RetryCount)
}

// S6 — isolasi tenant: admin tenant B tidak boleh melihat device tenant A.
func scenarioTenantIsolation(ctx context.Context, w *World) result {
	_ = ctx
	if len(w.serials) == 0 {
		return skipr("tidak ada device dari S1")
	}
	bDevices, err := w.pilot2Admin.listDevices(nil)
	if err != nil {
		return failr("pilot2 GET /devices: %v", err)
	}
	leaked := map[string]bool{}
	for _, s := range w.serials {
		leaked[s] = false
	}
	for _, d := range bDevices {
		if _, ok := leaked[d.SerialNumber]; ok {
			return failr("BOCOR: admin tenant B melihat device %s milik tenant A", d.SerialNumber)
		}
	}
	// GET by id device tenant A sebagai admin B -> 403/404.
	da, err := w.super.findDeviceBySerial(w.serials["zte"])
	if err != nil {
		return failr("%v", err)
	}
	st, _ := w.pilot2Admin.status(http.MethodGet, fmt.Sprintf("/api/v1/devices/%d", da.ID), nil)
	if st != http.StatusForbidden && st != http.StatusNotFound {
		return failr("GET /devices/%d sebagai admin tenant B = HTTP %d, harusnya 403/404", da.ID, st)
	}
	// Sanity: admin tenant A tetap bisa lihat device-nya.
	aDevices, err := w.pilotAdmin.listDevices(nil)
	if err != nil || len(aDevices) == 0 {
		return failr("admin tenant A tidak bisa melihat device-nya sendiri (err=%v, n=%d)", err, len(aDevices))
	}
	return pass("admin tenant B: 0 device tenant A terlihat, GET by id -> %d; admin tenant A lihat %d device", st, len(aDevices))
}

// S7 — Connection Request: ACS memaksa CPE Inform segera lewat
// ConnectionRequestURL, CPE membuka sesi baru dengan event "6 CONNECTION REQUEST".
func scenarioConnectionRequest(ctx context.Context, w *World) result {
	// Vendor "cdata" sengaja: BUKAN "zte" (dipakai rollout firmware S8) —
	// device CR ekstra ini tak boleh ikut tersapu wave rollout S8.
	prof := simcpe.Profiles["cdata"]
	serial := fmt.Sprintf("E2E-CR-%s-01", strings.ToUpper(w.runID))
	dev := simcpe.NewDevice(prof, serial)

	// got dilindungi mu: callback listener dipanggil dari goroutine sesi CR.
	var (
		mu  sync.Mutex
		got []simcpe.SessionReport
	)
	crl := simcpe.NewConnReqListener(":0", dev, simcpe.SessionOpts{
		ACSURL: w.cwmpURL, Username: w.cwmpUser, Password: w.cwmpPass,
	}, w.hc, func(r simcpe.SessionReport) {
		mu.Lock()
		got = append(got, r)
		mu.Unlock()
	})
	crURL, err := crl.Start(w.crHost)
	if err != nil {
		return failr("start listener: %v", err)
	}
	defer crl.Stop()
	dev.ConnReqURL = crURL

	// Onboard dulu supaya ACS meng-capture connection_request_url dari Inform.
	if rep := w.runCPE(ctx, dev, simcpe.SessionOpts{Events: []string{"0 BOOTSTRAP"}}); !rep.OK {
		return failr("onboard awal: %s", rep.ErrMsg)
	}
	d, err := w.super.findDeviceBySerial(serial)
	if err != nil {
		return failr("%v", err)
	}

	st, body := w.pilotAdmin.status(http.MethodPost, fmt.Sprintf("/api/v1/devices/%d/connection-request", d.ID), map[string]any{})
	if st >= 400 {
		return failr("POST connection-request: HTTP %d %s", st, body)
	}

	gotOK := func() (int, bool) {
		mu.Lock()
		defer mu.Unlock()
		if len(got) == 0 {
			return 0, false
		}
		return len(got), got[0].OK
	}
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if n, _ := gotOK(); crl.Fired() > 0 && n > 0 {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if crl.Fired() == 0 {
		return failr("ACS tidak pernah GET ConnectionRequestURL (HTTP %d dari POST, url=%s)", st, crURL)
	}
	if n, ok := gotOK(); n == 0 || !ok {
		return failr("sesi Inform balik gagal setelah Connection Request")
	}
	evs, _ := w.super.listDeviceEvents(d.ID)
	if !w.eventsContain(evs, "6 CONNECTION REQUEST") {
		return failr("event '6 CONNECTION REQUEST' tidak tercatat (dapat: %s)", w.eventCodes(evs))
	}
	return pass("ACS trigger CR (%dx), CPE Inform balik dgn '6 CONNECTION REQUEST', event tercatat", crl.Fired())
}

// S8 — firmware canary rollout: upload file, buat batch wave 100%, tiap CPE
// menerima Download + mengirim TransferComplete, batch tuntas tanpa kegagalan.
func scenarioFirmwareRollout(ctx context.Context, w *World) result {
	key := "zte"
	dev, serial := w.devices[key], w.serials[key]
	if dev == nil {
		return skipr("device %s dari S1 tidak ada", key)
	}
	d, err := w.super.findDeviceBySerial(serial)
	if err != nil {
		return failr("%v", err)
	}
	vendorID := w.vendorIDByKey[key]

	fwData := []byte("E2E-FIRMWARE-" + w.runID + "-" + strings.Repeat("x", 512))
	var fw struct {
		ID uint64 `json:"id"`
	}
	err = w.super.uploadMultipart("/api/v1/firmware", map[string]string{
		"vendor_id": fmt.Sprint(vendorID),
		"version":   "E2E-FW-" + w.runID,
	}, "file", "fw-e2e-"+w.runID+".bin", fwData, &fw)
	if err != nil {
		return failr("upload firmware: %v", err)
	}

	var batch struct {
		ID uint64 `json:"id"`
	}
	err = w.pilotAdmin.do(http.MethodPost, "/api/v1/firmware/rollout-batches", map[string]any{
		"tenant_id":                w.pilotTenantID,
		"firmware_file_id":         fw.ID,
		"vendor_id":                vendorID,
		"wave_percentage":          100,
		"max_failure_rate_percent": 80,
	}, &batch)
	if err != nil {
		return failr("buat rollout batch: %v", err)
	}

	// Jalankan sesi untuk device sampai menerima Download + kirim TransferComplete.
	var sawDownload, sawXfer bool
	for attempt := 0; attempt < 4; attempt++ {
		rep := w.runCPE(ctx, dev, simcpe.SessionOpts{Events: []string{"2 PERIODIC"}})
		if !rep.OK {
			return failr("sesi CWMP: %s", rep.ErrMsg)
		}
		if rep.CountRPC("Download") > 0 {
			sawDownload = true
		}
		if rep.Transfers > 0 {
			sawXfer = true
		}
		if sawDownload && sawXfer {
			break
		}
		time.Sleep(1 * time.Second)
	}
	if !sawDownload {
		return failr("CPE tidak pernah menerima Download RPC dari rollout")
	}
	if !sawXfer {
		return failr("CPE tidak mengirim TransferComplete")
	}

	// Poll status batch. Advance hanya DIDORONG bila status tak berubah 3
	// iterasi berturut-turut (macet) — supaya bila engine seharusnya
	// auto-advance setelah TransferComplete, skenario ini menangkap kalau
	// auto-advance rusak, bukan menutupinya dengan mendorong sendiri.
	var status, prev string
	stuck := 0
	for i := 0; i < 20; i++ {
		var b struct {
			StatusID    uint64 `json:"status_id"`
			CurrentWave uint32 `json:"current_wave"`
		}
		if err := w.pilotAdmin.do(http.MethodGet, fmt.Sprintf("/api/v1/firmware/rollout-batches/%d", batch.ID), nil, &b); err == nil {
			status = fmt.Sprintf("%s/w%d", w.rolloutRev[b.StatusID], b.CurrentWave)
			if w.rolloutRev[b.StatusID] == "COMPLETED" {
				break
			}
			if w.rolloutRev[b.StatusID] == "PAUSED_FAILURE_THRESHOLD" {
				return failr("batch PAUSED_FAILURE_THRESHOLD — Download/TransferComplete tidak dihitung sukses")
			}
		}
		if status == prev {
			stuck++
		} else {
			stuck = 0
		}
		prev = status
		if stuck >= 3 {
			_ = w.pilotAdmin.do(http.MethodPost, fmt.Sprintf("/api/v1/firmware/rollout-batches/%d/advance", batch.ID), map[string]any{}, nil)
			stuck = 0
		}
		time.Sleep(1500 * time.Millisecond)
	}
	status = strings.SplitN(status, "/", 2)[0]
	if status != "COMPLETED" {
		return failr("batch tidak COMPLETED setelah beberapa advance (status=%q)", status)
	}
	// Verifikasi firmware CPE benar-benar "berubah" di simulator.
	if got := dev.SoftwareVersion(); !strings.HasPrefix(got, "SIM-") {
		return failr("SoftwareVersion CPE = %q, harusnya berubah setelah Download", got)
	}
	jobsRaw := json.RawMessage{}
	_ = w.super.do(http.MethodGet, fmt.Sprintf("/api/v1/devices/%d/firmware-jobs", d.ID), nil, &jobsRaw)
	return pass("firmware canary rollout tuntas: Download+TransferComplete, batch COMPLETED, CPE fw=%s", dev.SoftwareVersion())
}

// S9 — engine preset (drift-heal gaya GenieACS): preset enforce=1, device
// menyimpang -> ACS otomatis mengantre SetParameterValues, CPE konvergen.
func scenarioPresetDriftHeal(ctx context.Context, w *World) result {
	prof := simcpe.Profiles["nokia"]
	serial := fmt.Sprintf("E2E-PRESET-%s-01", strings.ToUpper(w.runID))
	dev := simcpe.NewDevice(prof, serial)
	root := prof.RootPrefix()
	target := "ACS-Enforced-" + w.runID
	// SSID, bukan PeriodicInformInterval: ACS sengaja men-jitter interval
	// (jitterPeriodicInformInterval, anti thundering-herd) sehingga nilai
	// akhir di CPE bukan nilai literal preset — bukan bug, tapi bikin
	// assertion "sama persis" keliru.
	presetPath := root + "WiFi.SSID.1.SSID"

	precond, _ := json.Marshal(map[string]any{"oui": strings.ToUpper(prof.OUI)})
	configs, _ := json.Marshal([]map[string]any{
		{"op": "set_parameter", "key": presetPath, "value": target},
	})
	err := w.pilotAdmin.do(http.MethodPost, "/api/v1/presets", map[string]any{
		"name":           "e2e-preset-" + w.runID,
		"weight":         10,
		"precondition":   string(precond),
		"configurations": string(configs),
		"enforce":        true,
		"is_active":      true,
	}, nil)
	if err != nil {
		return failr("buat preset: %v", err)
	}

	// Onboarding + hingga 3 sesi: EvaluatePresets berjalan tiap Inform.
	var converged bool
	for i := 0; i < 3; i++ {
		ev := []string{"2 PERIODIC"}
		if i == 0 {
			ev = []string{"0 BOOTSTRAP"}
		}
		rep := w.runCPE(ctx, dev, simcpe.SessionOpts{Events: ev})
		if !rep.OK {
			return failr("sesi %d: %s", i, rep.ErrMsg)
		}
		if got, _ := dev.Param(presetPath); got == target {
			converged = true
			break
		}
		time.Sleep(1 * time.Second)
	}
	if !converged {
		got, _ := dev.Param(presetPath)
		return failr("CPE tidak konvergen ke preset: %s = %q, target %q", shortPath(presetPath), got, target)
	}
	d, err := w.super.findDeviceBySerial(serial)
	if err != nil {
		return failr("%v", err)
	}
	if !w.deviceHasTask(d.ID, "SET_PARAMETER_VALUES", "COMPLETED") {
		return failr("tidak ada task SET_PARAMETER_VALUES COMPLETED dari drift-heal (%s)", w.taskDump(d.ID))
	}
	return pass("preset enforce: CPE drift -> ACS auto-SetParameterValues -> konvergen (%s=%s)", shortPath(presetPath), target)
}

// S10 — regresi keamanan: (a) role ENDUSER dikurung ke /self-service — token
// portal pelanggan tidak boleh memanggil API staf; (b) SSRF guard menolak
// Connection Request ke alamat internal (169.254.169.254 metadata cloud).
func scenarioSecurityGuards(ctx context.Context, w *World) result {
	_ = ctx
	// (a) ENDUSER confinement.
	enduPass := "EnduPass-" + randHex(8)
	err := w.super.do(http.MethodPost, "/api/v1/users", map[string]any{
		"tenant_id": w.pilotTenantID, "username": "endu-" + w.runID,
		"email": "endu-" + w.runID + "@e2e.local", "password": enduPass,
		"full_name": "E2E EndUser", "role_codes": []string{"ENDUSER"},
	}, nil)
	if err != nil {
		return failr("buat ENDUSER: %v", err)
	}
	endu := NewClient(w.super.base, true)
	if err := endu.Login("endu-"+w.runID, enduPass); err != nil {
		return failr("login ENDUSER: %v", err)
	}
	for _, p := range []string{"/api/v1/devices", "/api/v1/tasks", "/api/v1/provisioning-profiles", "/api/v1/refs/ref_vendors", "/api/v1/presets"} {
		if st, body := endu.status(http.MethodGet, p, nil); st != http.StatusForbidden {
			return failr("ENDUSER GET %s = HTTP %d (%s), harusnya 403", p, st, body)
		}
	}
	if st, _ := endu.status(http.MethodGet, "/api/v1/self-service/devices", nil); st == http.StatusForbidden {
		return failr("ENDUSER GET /self-service/devices = 403, harusnya diizinkan")
	}
	if st, _ := endu.status(http.MethodPatch, "/api/v1/auth/password", map[string]any{"current_password": "x", "new_password": "yyyyyyyy"}); st == http.StatusForbidden {
		return failr("ENDUSER PATCH /auth/password = 403, harusnya bisa (current_password salah -> 401)")
	}

	// (b) SSRF guard pada Connection Request.
	serial := w.serials["zte"]
	if serial == "" {
		return failr("device zte dari S1 tidak ada untuk uji SSRF")
	}
	d, err := w.super.findDeviceBySerial(serial)
	if err != nil {
		return failr("%v", err)
	}
	// Admin set CR URL ke alamat metadata cloud (skenario: admin ceroboh /
	// akun terkompromi, atau nilai dari Inform di versi lama tanpa guard).
	metaURL := "http://169.254.169.254/latest/meta-data/"
	if st, body := w.pilotAdmin.status(http.MethodPatch, fmt.Sprintf("/api/v1/devices/%d", d.ID),
		map[string]any{"connection_request_url": metaURL}); st >= 400 {
		return failr("PATCH device connection_request_url: HTTP %d %s", st, body)
	}
	st, body := w.pilotAdmin.status(http.MethodPost, fmt.Sprintf("/api/v1/devices/%d/connection-request", d.ID), map[string]any{})
	if st < 400 || st >= 500 {
		return failr("Connection Request ke 169.254.169.254 -> HTTP %d (%s), harusnya 4xx (ditolak SSRF guard, BUKAN 5xx/2xx)", st, body)
	}
	// Kembalikan ke nilai kosong supaya tidak mengganggu skenario lain.
	_ = w.pilotAdmin.do(http.MethodPatch, fmt.Sprintf("/api/v1/devices/%d", d.ID), map[string]any{"connection_request_url": ""}, nil)

	return pass("ENDUSER 403 di 5 API staf & lolos /self-service; Connection Request ke metadata-cloud ditolak (HTTP %d)", st)
}

// --- util assertion ---

func (w *World) taskTypeCode(t taskJSON) string {
	if t.TaskTypeCode != "" {
		return t.TaskTypeCode
	}
	if c, ok := w.taskTypeRev[t.TaskTypeID]; ok {
		return c
	}
	return fmt.Sprintf("type=%d", t.TaskTypeID)
}

func (w *World) deviceHasTask(deviceID uint64, typeCode, statusCode string) bool {
	tasks, err := w.super.listDeviceTasks(deviceID)
	if err != nil {
		return false
	}
	for _, t := range tasks {
		if w.taskTypeCode(t) == typeCode && w.taskStatusCode(t.TaskStatusID) == statusCode {
			return true
		}
	}
	return false
}

func (w *World) taskDump(deviceID uint64) string {
	tasks, _ := w.super.listDeviceTasks(deviceID)
	var parts []string
	for _, t := range tasks {
		parts = append(parts, fmt.Sprintf("%s=%s", w.taskTypeCode(t), w.taskStatusCode(t.TaskStatusID)))
	}
	if len(parts) == 0 {
		return "tidak ada task"
	}
	return strings.Join(parts, " ")
}

func (w *World) eventsContain(evs []eventJSON, code string) bool {
	for _, e := range evs {
		if e.EventCode == code || e.Code == code || w.eventCodeRev[e.EventCodeID] == code {
			return true
		}
	}
	return false
}

func (w *World) eventCodes(evs []eventJSON) string {
	var s []string
	for _, e := range evs {
		c := firstNonEmpty(e.EventCode, e.Code)
		if c == "" {
			c = w.eventCodeRev[e.EventCodeID]
		}
		s = append(s, c)
	}
	return strings.Join(s, ",")
}

func paramsContain(ps []paramJSON, name string) bool {
	for _, p := range ps {
		if p.ParameterName == name {
			return true
		}
	}
	return false
}

func shortPath(p string) string {
	i := strings.IndexByte(p, '.')
	if i < 0 {
		return p
	}
	return "..." + p[i:]
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
