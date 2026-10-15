package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"acs/internal/simcpe"
)

// S13 — pra-registrasi & impor massal perangkat (POST /devices, POST
// /devices/import dengan dry-run), edit (termasuk lokasi yang dulu tidak pernah
// tersimpan), serta klaim lewat Inform pertama dan isolasi tenant.
// Dibuktikan terhadap stack hidup (SQL nyata):
//   - pra-registrasi membuat baris UNREGISTERED milik tenant admin; duplikat 409;
//     tenant lain tidak bisa mendaftarkan/mengubah/mengambil alih;
//   - PATCH menyimpan notes/MAC/lokasi (regresi: repository dulu tidak menulis lat/long);
//   - dry-run mengklasifikasi (create/update/unchanged/error) TANPA menulis; eksekusi
//     sama dengan pratinjau; mode skip melewati yang sudah ada;
//   - CPE yang Inform pertama kali dengan OUI+serial pra-registrasi memakai baris
//     yang sama (id sama, catatan terjaga, status berubah, vendor ter-resolve),
//     sementara tenant LAIN ditolak.
func scenarioDeviceProvision(ctx context.Context, w *World) result {
	run := strings.ToUpper(w.runID)
	prof := simcpe.Profiles["fiberhome"]
	serial1, serial2, serialImp := "PV-"+run+"-01", "PV-"+run+"-02", "PV-"+run+"-IMP"
	admin, admin2, super := w.pilotAdmin, w.pilot2Admin, w.super
	dev1 := simcpe.NewDevice(prof, serial1)
	dev2 := simcpe.NewDevice(prof, serial2)

	type devFull struct {
		ID             uint64   `json:"id"`
		TenantID       *uint64  `json:"tenant_id"`
		VendorID       *uint64  `json:"vendor_id"`
		DeviceStatusID uint64   `json:"device_status_id"`
		Notes          *string  `json:"notes"`
		MAC            *string  `json:"mac_address"`
		ProductClass   *string  `json:"product_class"`
		Latitude       *float64 `json:"latitude"`
		Longitude      *float64 `json:"longitude"`
	}
	get := func(c *Client, id uint64) (*devFull, error) {
		var d devFull
		err := c.do(http.MethodGet, fmt.Sprintf("/api/v1/devices/%d", id), nil, &d)
		return &d, err
	}

	var statuses []struct {
		ID   uint64 `json:"id"`
		Code string `json:"code"`
	}
	if err := super.do(http.MethodGet, "/api/v1/refs/ref_device_status", nil, &statuses); err != nil {
		return failr("ref_device_status: %v", err)
	}
	var unregID uint64
	for _, s := range statuses {
		if s.Code == "UNREGISTERED" {
			unregID = s.ID
		}
	}

	var ids []uint64
	defer func() {
		for _, id := range ids {
			_ = super.do(http.MethodDelete, fmt.Sprintf("/api/v1/devices/%d", id), nil, nil)
		}
	}()

	// ---- A. pra-registrasi ----
	var d1 devFull
	if err := admin.do(http.MethodPost, "/api/v1/devices", map[string]any{
		"oui": prof.OUI, "serial_number": serial1, "notes": "pra-registrasi e2e",
		"mac_address": "aa-bb-cc-00-11-22", "latitude": -6.2, "longitude": 106.8,
	}, &d1); err != nil {
		return failr("A: POST /devices oleh ADMIN: %v", err)
	}
	ids = append(ids, d1.ID)
	if d1.TenantID == nil || *d1.TenantID != w.pilotTenantID || d1.DeviceStatusID != unregID {
		return failr("A: perangkat harus milik tenant pilot & UNREGISTERED: %+v", d1)
	}
	if d1.MAC == nil || *d1.MAC != "AA:BB:CC:00:11:22" {
		return failr("A: MAC harus dinormalkan ke AA:BB:CC:00:11:22, dapat %v", d1.MAC)
	}
	if got, err := get(admin, d1.ID); err != nil || got.Latitude == nil || *got.Latitude != -6.2 || got.Longitude == nil || *got.Longitude != 106.8 {
		return failr("A: lokasi harus tersimpan saat pra-registrasi: %+v err=%v", got, err)
	}

	same := map[string]any{"oui": prof.OUI, "serial_number": serial1}
	for label, tc := range map[string]struct {
		c    *Client
		body map[string]any
		want int
	}{
		"duplikat oleh pemilik":      {admin, same, http.StatusConflict},
		"duplikat oleh tenant LAIN":  {admin2, same, http.StatusConflict},
		"ADMIN menarget tenant lain": {admin, map[string]any{"oui": prof.OUI, "serial_number": serial1 + "X", "tenant_id": w.pilot2TenantID}, http.StatusForbidden},
		"OUI tidak valid":            {admin, map[string]any{"oui": "12", "serial_number": "X"}, http.StatusBadRequest},
		"serial kosong":              {admin, map[string]any{"oui": prof.OUI, "serial_number": " "}, http.StatusBadRequest},
		"MAC salah":                  {admin, map[string]any{"oui": prof.OUI, "serial_number": serial1 + "M", "mac_address": "xyz"}, http.StatusBadRequest},
		"lat tanpa lon":              {admin, map[string]any{"oui": prof.OUI, "serial_number": serial1 + "L", "latitude": 1.5}, http.StatusBadRequest},
		"vendor tidak ada":           {admin, map[string]any{"oui": prof.OUI, "serial_number": serial1 + "V", "vendor_id": 999999}, http.StatusBadRequest},
		"superadmin tanpa tenant":    {super, map[string]any{"oui": prof.OUI, "serial_number": serial1 + "S"}, http.StatusBadRequest},
		"superadmin tenant tak ada":  {super, map[string]any{"oui": prof.OUI, "serial_number": serial1 + "T", "tenant_id": 999999}, http.StatusBadRequest},
	} {
		if st, b := tc.c.status(http.MethodPost, "/api/v1/devices", tc.body); st != tc.want {
			return failr("A: %s: HTTP %d (mau %d) %s", label, st, tc.want, b)
		}
	}

	// ---- B. edit: notes/MAC/lokasi/vendor benar-benar tersimpan ----
	var patched devFull
	if err := admin.do(http.MethodPatch, fmt.Sprintf("/api/v1/devices/%d", d1.ID), map[string]any{
		"notes": "diedit e2e", "mac_address": "ddeeff001122", "product_class": "AN5506",
		"latitude": 3.5952, "longitude": 98.6722, "vendor_id": w.vendorIDByKey["fiberhome"],
	}, &patched); err != nil {
		return failr("B: PATCH device: %v", err)
	}
	got, err := get(admin, d1.ID)
	if err != nil || got.Notes == nil || *got.Notes != "diedit e2e" || got.MAC == nil || *got.MAC != "DD:EE:FF:00:11:22" ||
		got.ProductClass == nil || *got.ProductClass != "AN5506" || got.Latitude == nil || *got.Latitude != 3.5952 ||
		got.Longitude == nil || *got.Longitude != 98.6722 || got.VendorID == nil || *got.VendorID != w.vendorIDByKey["fiberhome"] {
		return failr("B: hasil PATCH tidak tersimpan utuh: %+v err=%v", got, err)
	}
	if st, _ := admin2.status(http.MethodPatch, fmt.Sprintf("/api/v1/devices/%d", d1.ID), map[string]any{"notes": "bobol"}); st != http.StatusForbidden {
		return failr("B: tenant lain PATCH device harus 403, dapat %d", st)
	}
	if st, _ := admin.status(http.MethodPatch, fmt.Sprintf("/api/v1/devices/%d", d1.ID), map[string]any{"latitude": 1.0}); st != http.StatusBadRequest {
		return failr("B: latitude tanpa longitude harus 400, dapat %d", st)
	}

	// ---- C. impor massal: dry-run vs eksekusi ----
	type rowRes struct {
		Index  int      `json:"index"`
		Action string   `json:"action"`
		Error  string   `json:"error"`
		Chg    []string `json:"changes"`
	}
	type impRes struct {
		DryRun    bool     `json:"dry_run"`
		Created   int      `json:"created"`
		Updated   int      `json:"updated"`
		Unchanged int      `json:"unchanged"`
		Skipped   int      `json:"skipped"`
		Failed    int      `json:"failed"`
		Results   []rowRes `json:"results"`
	}
	rows := []map[string]any{
		{"oui": prof.OUI, "serial_number": serialImp, "notes": "baru via impor"},        // 0 create
		{"oui": prof.OUI, "serial_number": serial1, "notes": "diubah via impor"},        // 1 update
		{"oui": prof.OUI, "serial_number": serial1, "mac_address": "dd:ee:ff:00:11:22"}, // 2 duplikat dalam permintaan
		{"oui": "ZZ", "serial_number": "X"},                                             // 3 error
	}
	var dry impRes
	if err := admin.do(http.MethodPost, "/api/v1/devices/import?dry_run=true", map[string]any{"mode": "update", "rows": rows}, &dry); err != nil {
		return failr("C: dry-run: %v", err)
	}
	if !dry.DryRun || dry.Created != 1 || dry.Updated != 1 || dry.Failed != 2 || len(dry.Results) != 4 ||
		dry.Results[0].Action != "create" || dry.Results[1].Action != "update" || dry.Results[2].Action != "error" || dry.Results[3].Action != "error" {
		return failr("C: klasifikasi dry-run salah: %+v", dry)
	}
	if len(dry.Results[1].Chg) != 1 || dry.Results[1].Chg[0] != "notes" {
		return failr("C: perubahan baris update harus [notes], dapat %v", dry.Results[1].Chg)
	}
	if _, err := admin.findDeviceBySerial(serialImp); err == nil {
		return failr("C: dry-run MENULIS data (perangkat %s sudah ada)", serialImp)
	}
	if g, _ := get(admin, d1.ID); g == nil || g.Notes == nil || *g.Notes != "diedit e2e" {
		return failr("C: dry-run mengubah catatan perangkat yang ada")
	}

	var real impRes
	if err := admin.do(http.MethodPost, "/api/v1/devices/import", map[string]any{"mode": "update", "rows": rows}, &real); err != nil {
		return failr("C: eksekusi impor: %v", err)
	}
	if real.DryRun || real.Created != dry.Created || real.Updated != dry.Updated || real.Failed != dry.Failed {
		return failr("C: eksekusi harus sama dengan pratinjau: dry=%+v real=%+v", dry, real)
	}
	impDev, err := admin.findDeviceBySerial(serialImp)
	if err != nil {
		return failr("C: perangkat hasil impor tidak ada: %v", err)
	}
	ids = append(ids, impDev.ID)
	if g, _ := get(admin, d1.ID); g == nil || g.Notes == nil || *g.Notes != "diubah via impor" {
		return failr("C: baris update harus mengubah catatan")
	}
	// impor ulang idempoten; mode skip melewati yang ada
	var again, skip impRes
	if err := admin.do(http.MethodPost, "/api/v1/devices/import", map[string]any{"mode": "update", "rows": rows[:2]}, &again); err != nil {
		return failr("C: impor ulang: %v", err)
	}
	if again.Created != 0 || again.Updated != 0 || again.Unchanged != 2 {
		return failr("C: impor ulang harus idempoten (2 unchanged): %+v", again)
	}
	if err := admin.do(http.MethodPost, "/api/v1/devices/import", map[string]any{"mode": "skip", "rows": rows[:2]}, &skip); err != nil || skip.Skipped != 2 || skip.Created != 0 {
		return failr("C: mode skip harus melewati 2 baris yang ada: %+v err=%v", skip, err)
	}
	for label, tc := range map[string]struct {
		body map[string]any
		want int
	}{
		"mode salah":  {map[string]any{"mode": "ngawur", "rows": rows[:1]}, http.StatusBadRequest},
		"rows kosong": {map[string]any{"mode": "skip", "rows": []any{}}, http.StatusBadRequest},
	} {
		if st, b := admin.status(http.MethodPost, "/api/v1/devices/import", tc.body); st != tc.want {
			return failr("C: %s: HTTP %d (mau %d) %s", label, st, tc.want, b)
		}
	}

	// ---- D. isolasi tenant pada impor ----
	var cross impRes
	if err := admin2.do(http.MethodPost, "/api/v1/devices/import", map[string]any{"mode": "update", "rows": []map[string]any{
		{"oui": prof.OUI, "serial_number": serial1, "notes": "BOBOL oleh tenant lain"},
	}}, &cross); err != nil {
		return failr("D: impor lintas tenant: %v", err)
	}
	if cross.Failed != 1 || cross.Updated != 0 || cross.Results[0].Action != "error" {
		return failr("D: impor tenant LAIN atas perangkat milik pilot harus error: %+v", cross)
	}
	if strings.Contains(strings.ToLower(cross.Results[0].Error), "tenant") && strings.Contains(cross.Results[0].Error, fmt.Sprint(w.pilotTenantID)) {
		return failr("D: pesan error membocorkan tenant pemilik: %s", cross.Results[0].Error)
	}
	if g, _ := get(admin, d1.ID); g == nil || g.Notes == nil || *g.Notes != "diubah via impor" {
		return failr("D: catatan perangkat berubah oleh tenant lain")
	}
	var superRows impRes
	if err := super.do(http.MethodPost, "/api/v1/devices/import?dry_run=true", map[string]any{"mode": "skip", "rows": []map[string]any{
		{"oui": prof.OUI, "serial_number": "NOTENANT-" + run},
	}}, &superRows); err != nil || superRows.Failed != 1 {
		return failr("D: superadmin tanpa tenant per baris harus error: %+v err=%v", superRows, err)
	}

	// ---- E. klaim lewat Inform pertama ----
	rep := w.runCPE(ctx, dev1, simcpe.SessionOpts{Events: []string{"0 BOOTSTRAP"}})
	if !rep.OK {
		return failr("E: Inform dari CPE pra-registrasi gagal: HTTP %d %s", rep.InformStatus, rep.ErrMsg)
	}
	claimed, err := admin.findDeviceBySerial(serial1)
	if err != nil {
		return failr("E: %v", err)
	}
	if claimed.ID != d1.ID {
		return failr("E: Inform harus memakai baris pra-registrasi (id %d), dapat id %d", d1.ID, claimed.ID)
	}
	full, _ := get(admin, d1.ID)
	if full == nil || full.DeviceStatusID == unregID || full.Notes == nil || *full.Notes != "diubah via impor" ||
		full.TenantID == nil || *full.TenantID != w.pilotTenantID || full.VendorID == nil {
		return failr("E: setelah Inform status harus berubah, catatan/tenant terjaga, vendor ter-resolve: %+v", full)
	}

	// tenant LAIN tidak bisa mengklaim perangkat pra-registrasi milik pilot
	var d2 devFull
	if err := admin.do(http.MethodPost, "/api/v1/devices", map[string]any{"oui": prof.OUI, "serial_number": serial2}, &d2); err != nil {
		return failr("E: pra-registrasi perangkat kedua: %v", err)
	}
	ids = append(ids, d2.ID)
	hijack := simcpe.SessionOpts{Events: []string{"0 BOOTSTRAP"}, ACSURL: w.cwmpURL, Username: w.cwmpUser2, Password: w.cwmpPass2}
	if hrep := simcpe.RunSession(ctx, w.hc, dev2, hijack); hrep.OK {
		return failr("E: Inform tenant LAIN untuk perangkat pra-registrasi pilot harusnya DITOLAK (klaim lintas tenant terbuka)")
	}
	if g, _ := get(admin, d2.ID); g == nil || g.DeviceStatusID != unregID || g.TenantID == nil || *g.TenantID != w.pilotTenantID {
		return failr("E: perangkat harus tetap UNREGISTERED milik pilot setelah percobaan tenant lain: %+v", g)
	}
	if _, err := admin2.findDeviceBySerial(serial2); err == nil {
		return failr("E: tenant pilot2 melihat perangkat pra-registrasi milik pilot")
	}

	return pass("pra-registrasi (409/403/400 benar), edit menyimpan lokasi, impor dry-run = eksekusi (create/update/error), idempoten, skip, tenant lain ditolak, Inform pertama memakai baris yang sama")
}
