package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"acs/internal/simcpe"
)

// S27 — profil ONT NYATA FiberHome HG6145F (identitas dibaca dari halaman web ONT milik pengguna).
// Memastikan bahwa OUI nyata 58:AE:F1 dikenali katalog (seed IEEE, TANPA menyuntik OUI palsu), vendor ter-resolve ke
// FIBERHOME, versi firmware tersimpan, dan logical key standar bekerja lewat mapping vendor (bukan percabangan kode).
// BUKAN bukti perangkat fisik: yang nyata hanya identitasnya; perilaku TR-069 ONT (namespace, pohon parameter, path optik)
// belum pernah terlihat karena akun pengguna ONT tidak punya akses TR-069.
func scenarioRealOnt(ctx context.Context, w *World) result {
	vendors, err := w.super.refMap("ref_vendors")
	if err != nil {
		return failr("refMap ref_vendors: %v", err)
	}
	fhID := vendors["FIBERHOME"]
	if fhID == 0 {
		return skipr("vendor FIBERHOME belum ada di katalog")
	}
	var ouis []struct {
		OUI string `json:"oui"`
	}
	if err := w.super.do(http.MethodGet, fmt.Sprintf("/api/v1/vendors/%d/ouis?page_size=200", fhID), nil, &ouis); err != nil {
		return failr("daftar OUI FiberHome: %v", err)
	}
	seeded := false
	for _, o := range ouis {
		if strings.EqualFold(o.OUI, "58AEF1") {
			seeded = true
		}
	}
	if !seeded {
		return failr("OUI nyata 58AEF1 (FiberHome HG6145F) tidak ada di katalog vendor_ouis")
	}

	prof := simcpe.Profiles["hg6145f"]
	serial := "FHTT99" + strings.ToUpper(w.runID[:6])
	dev := simcpe.NewDevice(prof, serial)
	if rep := w.runCPE(ctx, dev, simcpe.SessionOpts{Events: []string{"0 BOOTSTRAP"}}); !rep.OK {
		return failr("sesi BOOTSTRAP: %s", rep.ErrMsg)
	}
	d, err := w.super.findDeviceBySerial(serial)
	if err != nil {
		return failr("%v", err)
	}
	if d.OUI == nil || !strings.EqualFold(*d.OUI, "58AEF1") {
		return failr("OUI tersimpan = %v, harusnya 58AEF1", d.OUI)
	}
	if d.VendorID == nil || *d.VendorID != fhID {
		return failr("vendor_id=%v, harusnya FIBERHOME (%d) dari OUI seed nyata", d.VendorID, fhID)
	}
	if d.SoftwareVersion == nil || *d.SoftwareVersion != "RP2951" {
		return failr("versi firmware tersimpan = %v, harusnya RP2951", d.SoftwareVersion)
	}

	// logical key standar -> path TR-098 lewat mapping vendor FiberHome; CPE menerima, tidak ada fault
	ssid := "S27-" + w.runID
	if err := w.pilotAdmin.do(http.MethodPost, fmt.Sprintf("/api/v1/devices/%d/tasks/set-parameter-values", d.ID), map[string]any{
		"values": map[string]string{"wifi.ssid": ssid},
	}, nil); err != nil {
		return failr("set logical key wifi.ssid: %v", err)
	}
	if err := w.pilotAdmin.do(http.MethodPost, fmt.Sprintf("/api/v1/devices/%d/tasks/get-parameter-values", d.ID), map[string]any{
		"names": []string{"wifi.ssid", "device.software_version"},
	}, nil); err != nil {
		return failr("get logical key: %v", err)
	}
	rep := w.runCPE(ctx, dev, simcpe.SessionOpts{Events: []string{"2 PERIODIC"}})
	if !rep.OK || rep.CountRPC("SetParameterValues") == 0 || rep.CountRPC("GetParameterValues") == 0 {
		return failr("sesi periodik: ok=%v rpc=%v err=%s", rep.OK, rep.RPCsReceived, rep.ErrMsg)
	}
	if len(rep.FaultsSent) != 0 {
		return failr("CPE membalas fault untuk path hasil mapping: %v", rep.FaultsSent)
	}
	if got, _ := dev.Param(prof.RootPrefix() + "LANDevice.1.WLANConfiguration.1.SSID"); got != ssid {
		return failr("SSID di CPE = %q, harusnya %q (path TR-098 hasil mapping FiberHome)", got, ssid)
	}
	// semua task selesai
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		tasks, err := w.pilotAdmin.listDeviceTasks(d.ID)
		if err != nil {
			return failr("list task: %v", err)
		}
		done := 0
		for _, t := range tasks {
			if t.TaskStatusID == w.taskStatus["COMPLETED"] {
				done++
			}
		}
		if done >= 2 {
			return pass("ONT nyata HG6145F (OUI 58AEF1 seed IEEE, firmware RP2951): vendor FIBERHOME ter-resolve tanpa OUI palsu; logical key wifi.ssid → path TR-098 lewat mapping, CPE menerima tanpa fault, %d task COMPLETED. Catatan: perilaku TR-069 ONT fisik belum teramati", done)
		}
		time.Sleep(500 * time.Millisecond)
	}
	return failr("task belum COMPLETED setelah sesi periodik")
}
