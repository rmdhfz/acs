package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"acs/internal/simcpe"
)

// S30 — cakupan test case UAT yang dapat dibuktikan tanpa ONT fisik (lihat docs/pengujian/MATRIKS_TC_UAT.md):
//
//	RD-06  snapshot konfigurasi: dua snapshot sebelum/sesudah perubahan menunjukkan selisih yang benar
//	WR-02  ubah SSID lalu KEMBALIKAN ke nilai asli (CPE dan snapshot ketiga sama dengan snapshot pertama)
//	WR-03  ubah interval Inform (PeriodicInformInterval) dan nilainya tercatat di ACS
//	INF-10 tiga Inform periodik berturut-turut, ketiganya tercatat sebagai event "2 PERIODIC"
//	OBS-05 setelah sesi selesai tidak ada sesi CWMP yang tertinggal terbuka
//	OBS-03 audit trail perangkat memuat SET_PARAMETER_VALUES dan CREATE_CONFIG_SNAPSHOT
//	SEC-01 tenant lain tidak dapat membuat/melihat snapshot perangkat ini
//	PRE-12 OUI nyata ZTE F660/F609, Optictimes, dan FiberHome ter-resolve ke vendor & model yang benar di katalog
func scenarioUATCoverage(ctx context.Context, w *World) result {
	key := "fiberhome"
	dev, serial := w.devices[key], w.serials[key]
	if dev == nil {
		return skipr("device %s dari S1 tidak ada", key)
	}
	d, err := w.super.findDeviceBySerial(serial)
	if err != nil {
		return failr("%v", err)
	}
	base := fmt.Sprintf("/api/v1/devices/%d", d.ID)
	prof := simcpe.Profiles[key]
	ssidPath := prof.RootPrefix() + "LANDevice.1.WLANConfiguration.1.SSID"
	informPath := prof.RootPrefix() + "ManagementServer.PeriodicInformInterval"
	if prof.DataModel == simcpe.TR181 {
		ssidPath = prof.RootPrefix() + "WiFi.SSID.1.SSID"
	}
	orig, ok := dev.Param(ssidPath)
	if !ok || orig == "" {
		return failr("SSID awal simulator tidak ada di %s", ssidPath)
	}
	origInterval, _ := dev.Param(informPath)

	setParams := func(values map[string]string) error {
		if c, b := w.pilotAdmin.status(http.MethodPost, base+"/tasks/set-parameter-values", map[string]any{"values": values}); c >= 400 {
			return fmt.Errorf("POST set-parameter-values: HTTP %d %s", c, b)
		}
		for i := 0; i < 2; i++ {
			if rep := w.runCPE(ctx, dev, simcpe.SessionOpts{Events: []string{"2 PERIODIC"}}); !rep.OK {
				return fmt.Errorf("sesi %d gagal: %s", i+1, rep.ErrMsg)
			}
		}
		time.Sleep(300 * time.Millisecond)
		return nil
	}
	snapshot := func() (map[string]string, error) {
		before := snapCount(w, base)
		if c, b := w.pilotAdmin.status(http.MethodPost, base+"/config-snapshots", nil); c >= 400 {
			return nil, fmt.Errorf("POST config-snapshots: HTTP %d %s", c, b)
		}
		var list struct {
			Data []struct {
				ID           uint64          `json:"id"`
				SnapshotData json.RawMessage `json:"snapshot_data"`
			} `json:"data"`
			Total int `json:"total"`
		}
		if err := w.pilotAdmin.do(http.MethodGet, base+"/config-snapshots?page=1&page_size=50", nil, &list); err != nil {
			return nil, err
		}
		if list.Total != before+1 || len(list.Data) == 0 {
			return nil, fmt.Errorf("jumlah snapshot %d -> %d, harusnya +1", before, list.Total)
		}
		newest := list.Data[0]
		for _, s := range list.Data {
			if s.ID > newest.ID {
				newest = s
			}
		}
		m := map[string]string{}
		if err := json.Unmarshal(newest.SnapshotData, &m); err != nil {
			return nil, fmt.Errorf("isi snapshot bukan peta string: %v", err)
		}
		return m, nil
	}

	// Snapshot hanya memuat parameter yang SUDAH tersinkron ke ACS (Inform tidak membawa SSID): baca dulu, seperti operator "Refresh".
	if c, b := w.pilotAdmin.status(http.MethodPost, base+"/tasks/get-parameter-values", map[string]any{"names": []string{ssidPath, informPath}}); c >= 400 {
		return failr("POST get-parameter-values: HTTP %d %s", c, b)
	}
	for i := 0; i < 2; i++ {
		if rep := w.runCPE(ctx, dev, simcpe.SessionOpts{Events: []string{"2 PERIODIC"}}); !rep.OK {
			return failr("sesi baca parameter %d gagal: %s", i+1, rep.ErrMsg)
		}
	}
	time.Sleep(300 * time.Millisecond)

	// --- RD-06 + WR-02: snapshot1 -> ubah -> snapshot2 -> kembalikan -> snapshot3 ---
	snap1, err := snapshot()
	if err != nil {
		return failr("snapshot 1: %v", err)
	}
	if snap1[ssidPath] != orig {
		return failr("snapshot 1 memuat SSID %q, harusnya %q (kosong = Inform belum menyetor parameter)", snap1[ssidPath], orig)
	}
	changed := "UAT-" + w.runID
	if err := setParams(map[string]string{ssidPath: changed}); err != nil {
		return failr("ubah SSID: %v", err)
	}
	if got, _ := dev.Param(ssidPath); got != changed {
		return failr("CPE tidak menerima SSID baru (%q)", got)
	}
	snap2, err := snapshot()
	if err != nil {
		return failr("snapshot 2: %v", err)
	}
	if snap2[ssidPath] != changed {
		return failr("snapshot 2 memuat SSID %q, harusnya %q", snap2[ssidPath], changed)
	}
	diff := 0
	for k, v := range snap2 {
		if snap1[k] != v {
			diff++
			if k != ssidPath && !strings.HasSuffix(k, ".SSID") && !strings.Contains(strings.ToLower(k), "time") {
				return failr("selisih tak terduga antara snapshot 1 dan 2: %s (%q -> %q)", k, snap1[k], v)
			}
		}
	}
	if diff == 0 {
		return failr("snapshot 1 dan 2 identik padahal SSID berubah")
	}
	if err := setParams(map[string]string{ssidPath: orig}); err != nil {
		return failr("kembalikan SSID: %v", err)
	}
	if got, _ := dev.Param(ssidPath); got != orig {
		return failr("SSID tidak kembali ke nilai asli (%q != %q)", got, orig)
	}
	snap3, err := snapshot()
	if err != nil {
		return failr("snapshot 3: %v", err)
	}
	if snap3[ssidPath] != snap1[ssidPath] {
		return failr("snapshot 3 SSID %q != snapshot 1 %q setelah dikembalikan", snap3[ssidPath], snap1[ssidPath])
	}

	// --- WR-03: interval Inform ---
	if err := setParams(map[string]string{informPath: "600"}); err != nil {
		return failr("ubah interval Inform: %v", err)
	}
	if got, _ := dev.Param(informPath); got != "600" {
		return failr("CPE tidak menerima interval baru (%q)", got)
	}
	var params []struct {
		Name  string  `json:"parameter_name"`
		Value *string `json:"parameter_value"`
	}
	if err := w.pilotAdmin.do(http.MethodGet, base+"/parameters", nil, &params); err != nil {
		return failr("GET parameters: %v", err)
	}
	seen := ""
	for _, p := range params {
		if p.Name == informPath && p.Value != nil {
			seen = *p.Value
		}
	}
	if seen != "600" {
		return failr("ACS mencatat %s = %q, harusnya 600", informPath, seen)
	}
	if err := setParams(map[string]string{informPath: firstNonEmpty(origInterval, "300")}); err != nil {
		return failr("kembalikan interval Inform: %v", err)
	}

	// --- INF-10 + OBS-05: tiga Inform periodik, tidak ada sesi tertinggal ---
	evBefore, _ := w.super.listDeviceEvents(d.ID)
	nBefore := countEvent(w, evBefore, "2 PERIODIC")
	for i := 0; i < 3; i++ {
		if rep := w.runCPE(ctx, dev, simcpe.SessionOpts{Events: []string{"2 PERIODIC"}}); !rep.OK {
			return failr("Inform periodik %d gagal: %s", i+1, rep.ErrMsg)
		}
	}
	time.Sleep(300 * time.Millisecond)
	evAfter, _ := w.super.listDeviceEvents(d.ID)
	if got := countEvent(w, evAfter, "2 PERIODIC") - nBefore; got < 3 {
		return failr("hanya %d dari 3 event '2 PERIODIC' bertambah", got)
	}
	var open struct {
		Count int `json:"count"`
	}
	if err := w.pilotAdmin.do(http.MethodGet, "/api/v1/cwmp/sessions/count", nil, &open); err != nil {
		return failr("GET sessions/count: %v", err)
	}
	if open.Count != 0 {
		return failr("%d sesi CWMP masih terbuka setelah semua sesi selesai", open.Count)
	}

	// --- OBS-03: audit trail ---
	var act struct {
		Data []struct {
			Action string `json:"action"`
		} `json:"data"`
	}
	if err := w.pilotAdmin.do(http.MethodGet, base+"/activity?page_size=100", nil, &act); err != nil {
		return failr("GET activity: %v", err)
	}
	acts := map[string]int{}
	for _, a := range act.Data {
		acts[a.Action]++
	}
	if acts["CREATE_CONFIG_SNAPSHOT"] < 3 || acts["SET_PARAMETER_VALUES"] < 3 {
		return failr("audit trail tidak lengkap: %v (butuh >=3 CREATE_CONFIG_SNAPSHOT dan >=3 SET_PARAMETER_VALUES)", acts)
	}

	// --- SEC-01: tenant lain ---
	if c, _ := w.pilot2Admin.status(http.MethodPost, base+"/config-snapshots", nil); c != http.StatusForbidden && c != http.StatusNotFound {
		return failr("tenant lain membuat snapshot harus 403/404, dapat %d", c)
	}
	if c, _ := w.pilot2Admin.status(http.MethodGet, base+"/config-snapshots", nil); c != http.StatusForbidden && c != http.StatusNotFound {
		return failr("tenant lain membaca snapshot harus 403/404, dapat %d", c)
	}

	// --- PRE-12: OUI nyata -> vendor & model ---
	var vendors struct {
		Data []struct {
			ID   uint64 `json:"id"`
			Code string `json:"code"`
		} `json:"data"`
	}
	if err := w.super.do(http.MethodGet, "/api/v1/vendors?page_size=200", nil, &vendors); err != nil {
		return failr("GET vendors: %v", err)
	}
	codeOf := map[uint64]string{}
	for _, v := range vendors.Data {
		codeOf[v.ID] = v.Code
	}
	real := []struct {
		label, base, oui, class, manuf, vendor string
		wantModel                              bool
	}{
		{"ZTE F660", "zte", "38D82F", "F660", "ZTE Corporation", "ZTE", true},
		{"ZTE F609", "zte", "E4CA12", "F609", "ZTE Corporation", "ZTE", true},
		{"Optictimes RXP4-AC1200", "zte", "9028F6", "RXP4-AC1200", "Optictimes", "OPTICTIMES", true},
		{"FiberHome HG6145F", "fiberhome", "58AEF1", "HG6145F", "FiberHome", "FIBERHOME", false},
	}
	var resolved []string
	for _, r := range real {
		p := simcpe.Profiles[r.base]
		p.OUI, p.ProductClass, p.ModelName, p.Manufacturer = r.oui, r.class, r.class, r.manuf
		sn := fmt.Sprintf("S30-%s-%s", r.oui, w.runID)
		sim := simcpe.NewDevice(p, sn)
		if rep := w.runCPE(ctx, sim, simcpe.SessionOpts{Events: []string{"0 BOOTSTRAP", "1 BOOT"}}); !rep.OK {
			return failr("%s: Inform gagal: %s", r.label, rep.ErrMsg)
		}
		var got struct {
			Data []struct {
				SerialNumber  string  `json:"serial_number"`
				VendorID      *uint64 `json:"vendor_id"`
				DeviceModelID *uint64 `json:"device_model_id"`
			} `json:"data"`
		}
		if err := w.pilotAdmin.do(http.MethodGet, "/api/v1/devices?search="+sn, nil, &got); err != nil || len(got.Data) == 0 {
			return failr("%s: perangkat tak ditemukan (err=%v)", r.label, err)
		}
		g := got.Data[0]
		if g.VendorID == nil || codeOf[*g.VendorID] != r.vendor {
			return failr("%s: vendor ter-resolve %v, harusnya %s", r.label, vendorName(codeOf, g.VendorID), r.vendor)
		}
		if r.wantModel && g.DeviceModelID == nil {
			return failr("%s: model perangkat tidak ter-resolve (vendor %s)", r.label, r.vendor)
		}
		resolved = append(resolved, fmt.Sprintf("%s→%s", r.label, r.vendor))
	}

	return pass("snapshot sebelum/sesudah (selisih benar) & SSID dikembalikan (snapshot 3 = snapshot 1); interval Inform 600 tercatat lalu dipulihkan; 3 event 2 PERIODIC, 0 sesi tertinggal; audit trail lengkap; tenant lain 403/404; OUI nyata ter-resolve: %s", strings.Join(resolved, ", "))
}

func snapCount(w *World, base string) int {
	var l struct {
		Total int `json:"total"`
	}
	_ = w.pilotAdmin.do(http.MethodGet, base+"/config-snapshots?page=1&page_size=1", nil, &l)
	return l.Total
}

func countEvent(w *World, evs []eventJSON, code string) int {
	n := 0
	for _, e := range evs {
		c := firstNonEmpty(e.EventCode, e.Code)
		if c == "" {
			c = w.eventCodeRev[e.EventCodeID]
		}
		if c == code {
			n++
		}
	}
	return n
}

func vendorName(m map[uint64]string, id *uint64) string {
	if id == nil {
		return "(kosong)"
	}
	return m[*id]
}
