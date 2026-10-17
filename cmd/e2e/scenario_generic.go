package main

import (
	"context"
	"fmt"
	"net/http"

	"acs/internal/simcpe"
)

// S21 — dukungan "semua ONT" (migrations/0029): perangkat di luar 5 vendor baseline tetap dikelola lewat logical key.
//  1. OUI nyata vendor tambahan (TP-Link, dari seed IEEE) -> vendor_id ter-resolve otomatis; logical key wifi.ssid dan
//     wan.pppoe.username jatuh ke mapping standar (vendor GENERIC) -> CPE (TR-098) menerima path standar yang benar.
//  2. OUI yang tidak dikenal sama sekali (vendor NULL) pada CPE TR-181 -> data model terdeteksi dari parameter Inform,
//     logical key tetap ter-resolve ke path standar TR-181 dan nilainya berubah di CPE.
func scenarioGenericVendor(ctx context.Context, w *World) result {
	vendors, err := w.super.refMap("ref_vendors")
	if err != nil {
		return failr("refMap ref_vendors: %v", err)
	}
	tpID, generic := vendors["TPLINK"], vendors["GENERIC"]
	if tpID == 0 || generic == 0 {
		return skipr("vendor TPLINK/GENERIC belum ada (migrasi 0029 belum diterapkan)")
	}
	var ouis []struct {
		OUI string `json:"oui"`
	}
	if err := w.super.do(http.MethodGet, fmt.Sprintf("/api/v1/vendors/%d/ouis", tpID), nil, &ouis); err != nil || len(ouis) == 0 {
		return failr("OUI TP-Link tidak ada di katalog: %v", err)
	}

	// ---- 1. vendor tambahan, TR-098 ----
	tpProf := simcpe.Profiles["zte"]
	tpProf.Key, tpProf.Manufacturer, tpProf.OUI = "tplink-e2e", "TP-Link", ouis[0].OUI
	tpProf.ProductClass, tpProf.ModelName = "XX230v", "XX230v"
	tpSerial := fmt.Sprintf("E2E-TPLINK-%s-01", w.runID)
	tp := simcpe.NewDevice(tpProf, tpSerial)
	if rep := w.runCPE(ctx, tp, simcpe.SessionOpts{Events: []string{"0 BOOTSTRAP"}}); !rep.OK {
		return failr("TP-Link: sesi gagal: %s", rep.ErrMsg)
	}
	d, err := w.super.findDeviceBySerial(tpSerial)
	if err != nil {
		return failr("TP-Link: %v", err)
	}
	if d.VendorID == nil || *d.VendorID != tpID {
		return failr("TP-Link: vendor_id=%v, harusnya %d (resolve dari OUI seed %s)", d.VendorID, tpID, tpProf.OUI)
	}
	ssid := "S21-TP-" + w.runID
	if err := w.pilotAdmin.do(http.MethodPost, fmt.Sprintf("/api/v1/devices/%d/tasks/set-parameter-values", d.ID), map[string]any{
		"values": map[string]string{"wifi.ssid": ssid, "wan.pppoe.username": "s21-" + w.runID},
	}, nil); err != nil {
		return failr("TP-Link: set logical key: %v", err)
	}
	if rep := w.runCPE(ctx, tp, simcpe.SessionOpts{Events: []string{"2 PERIODIC"}}); !rep.OK || rep.CountRPC("SetParameterValues") == 0 {
		return failr("TP-Link: CPE tidak menerima SetParameterValues (%s)", rep.ErrMsg)
	}
	ssidPath := tpProf.RootPrefix() + "LANDevice.1.WLANConfiguration.1.SSID"
	if got, _ := tp.Param(ssidPath); got != ssid {
		return failr("TP-Link: SSID di CPE = %q, harusnya %q (path standar TR-098)", got, ssid)
	}
	if got, _ := tp.Param(tpProf.RootPrefix() + "WANDevice.1.WANConnectionDevice.1.WANPPPConnection.1.Username"); got != "s21-"+w.runID {
		return failr("TP-Link: PPPoE username di CPE = %q", got)
	}

	// ---- 2. OUI tak dikenal, TR-181 ----
	unkProf := simcpe.Profiles["nokia"]
	unkProf.Key, unkProf.Manufacturer, unkProf.OUI = "unknown-e2e", "NoName ONT", "UNK9Z9"
	unkProf.ProductClass, unkProf.ModelName = "NN-1", "NN-1"
	unkSerial := fmt.Sprintf("E2E-UNKNOWN-%s-01", w.runID)
	unk := simcpe.NewDevice(unkProf, unkSerial)
	if rep := w.runCPE(ctx, unk, simcpe.SessionOpts{Events: []string{"0 BOOTSTRAP"}}); !rep.OK {
		return failr("tak dikenal: sesi gagal: %s", rep.ErrMsg)
	}
	u, err := w.super.findDeviceBySerial(unkSerial)
	if err != nil {
		return failr("tak dikenal: %v", err)
	}
	if u.VendorID != nil {
		return failr("tak dikenal: vendor_id seharusnya NULL, dapat %d", *u.VendorID)
	}
	ssid2 := "S21-UNK-" + w.runID
	if err := w.pilotAdmin.do(http.MethodPost, fmt.Sprintf("/api/v1/devices/%d/tasks/set-parameter-values", u.ID), map[string]any{
		"values": map[string]string{"wifi.ssid": ssid2},
	}, nil); err != nil {
		return failr("tak dikenal: logical key ditolak (vendor NULL harus jatuh ke mapping standar): %v", err)
	}
	if rep := w.runCPE(ctx, unk, simcpe.SessionOpts{Events: []string{"2 PERIODIC"}}); !rep.OK || rep.CountRPC("SetParameterValues") == 0 {
		return failr("tak dikenal: CPE tidak menerima SetParameterValues (%s)", rep.ErrMsg)
	}
	if got, _ := unk.Param("Device.WiFi.SSID.1.SSID"); got != ssid2 {
		return failr("tak dikenal: SSID di CPE = %q, harusnya %q (path standar TR-181)", got, ssid2)
	}
	if !w.deviceHasTask(u.ID, "SET_PARAMETER_VALUES", "COMPLETED") {
		return failr("tak dikenal: task SET_PARAMETER_VALUES tidak COMPLETED (%s)", w.taskDump(u.ID))
	}
	return pass("vendor tambahan (TP-Link, OUI seed IEEE) & OUI tak dikenal (vendor NULL, TR-181): logical key wifi.ssid/wan.pppoe.* ter-resolve ke mapping standar GENERIC, nilai berubah di CPE")
}
