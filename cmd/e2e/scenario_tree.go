package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"acs/internal/simcpe"
)

// S17 — penelusuran pohon parameter + atribut notifikasi:
//  1. CPE punya parameter yang TIDAK pernah dilaporkan di Inform (X_E2E_Tree...). Sebelum refresh ACS tak
//     mengenalnya; setelah POST /devices/:id/refresh ACS menelusuri pohon (GetParameterNames berantai,
//     kedalaman 4) lalu membaca nilai daun (GetParameterValues) sehingga keduanya muncul lengkap dengan nilai.
//  2. SetParameterAttributes (notifikasi aktif) pada daun itu diterima CPE, task COMPLETED; ADMIN tenant
//     lain ditolak (403).
//  3. Sesi berevent "4 VALUE CHANGE" tercatat di riwayat event.
func scenarioTreeAndAttributes(ctx context.Context, w *World) result {
	key := "huawei"
	dev, serial := w.devices[key], w.serials[key]
	if dev == nil {
		return skipr("device %s dari S1 tidak ada", key)
	}
	d, err := w.super.findDeviceBySerial(serial)
	if err != nil {
		return failr("%v", err)
	}
	root := simcpe.Profiles[key].RootPrefix()
	leaf1 := root + "X_E2E_Tree.Branch.Leaf"
	leaf2 := root + "X_E2E_Tree.Branch.Deep.Leaf2"
	v1, v2 := "v1-"+w.runID, "v2-"+w.runID
	dev.Set(map[string]string{leaf1: v1, leaf2: v2})

	before, _ := w.super.listDeviceParameters(d.ID)
	if paramsContain(before, leaf1) || paramsContain(before, leaf2) {
		return failr("parameter uji sudah dikenal ACS sebelum refresh (data sisa?)")
	}

	// ---- 1. refresh pohon ----
	if st, body := w.pilotAdmin.status(http.MethodPost, fmt.Sprintf("/api/v1/devices/%d/refresh", d.ID), map[string]any{"depth": 4}); st >= 400 {
		return failr("POST refresh: HTTP %d %s", st, body)
	}
	if st, _ := w.pilot2Admin.status(http.MethodPost, fmt.Sprintf("/api/v1/devices/%d/refresh", d.ID), map[string]any{"depth": 4}); st != 403 && st != 404 {
		return failr("ADMIN tenant lain me-refresh device harus 403/404, dapat %d", st)
	}
	names := 0
	var got1, got2 *string
	for i := 0; i < 10; i++ {
		rep := w.runCPE(ctx, dev, simcpe.SessionOpts{Events: []string{"2 PERIODIC"}})
		if !rep.OK {
			return failr("sesi %d gagal: %s", i+1, rep.ErrMsg)
		}
		names += rep.CountRPC("GetParameterNames")
		ps, _ := w.super.listDeviceParameters(d.ID)
		got1, got2 = nil, nil
		for _, p := range ps {
			switch p.ParameterName {
			case leaf1:
				got1 = p.ParameterValue
			case leaf2:
				got2 = p.ParameterValue
			}
		}
		if got1 != nil && got2 != nil {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if names == 0 {
		return failr("CPE tidak menerima GetParameterNames (task: %s)", w.taskDump(d.ID))
	}
	if got1 == nil || got2 == nil || *got1 != v1 || *got2 != v2 {
		return failr("pohon tidak lengkap setelah refresh: leaf1=%v leaf2=%v (task: %s)", got1, got2, w.taskDump(d.ID))
	}

	// ---- 2. atribut notifikasi ----
	attr := map[string]any{"attributes": []map[string]any{{"name": leaf1, "notification_change": true, "notification": 2}}}
	if st, _ := w.pilot2Admin.status(http.MethodPost, fmt.Sprintf("/api/v1/devices/%d/tasks/set-parameter-attributes", d.ID), attr); st != 403 && st != 404 {
		return failr("ADMIN tenant lain mengatur atribut harus 403/404, dapat %d", st)
	}
	if st, body := w.pilotAdmin.status(http.MethodPost, fmt.Sprintf("/api/v1/devices/%d/tasks/set-parameter-attributes", d.ID), attr); st >= 400 {
		return failr("POST set-parameter-attributes: HTTP %d %s", st, body)
	}
	rep := w.runCPE(ctx, dev, simcpe.SessionOpts{Events: []string{"2 PERIODIC"}})
	if !rep.OK {
		return failr("sesi atribut: %s", rep.ErrMsg)
	}
	if rep.CountRPC("SetParameterAttributes") == 0 {
		return failr("CPE tidak menerima SetParameterAttributes (rpc: %v)", rep.RPCsReceived)
	}
	if !w.deviceHasTask(d.ID, "SET_PARAMETER_ATTRIBUTES", "COMPLETED") {
		return failr("task SET_PARAMETER_ATTRIBUTES tidak COMPLETED (%s)", w.taskDump(d.ID))
	}

	// ---- 3. event 4 VALUE CHANGE ----
	if rep := w.runCPE(ctx, dev, simcpe.SessionOpts{Events: []string{"4 VALUE CHANGE"}}); !rep.OK {
		return failr("sesi event 4: %s", rep.ErrMsg)
	}
	evs, _ := w.super.listDeviceEvents(d.ID)
	if !w.eventsContain(evs, "4 VALUE CHANGE") {
		return failr("event 4 VALUE CHANGE tidak tercatat (event: %s)", w.eventCodes(evs))
	}
	return pass("refresh menelusuri pohon (%d GetParameterNames) hingga daun bernilai (kedalaman 4); atribut notifikasi diterima CPE (tenant lain ditolak); event 4 VALUE CHANGE tercatat", names)
}
