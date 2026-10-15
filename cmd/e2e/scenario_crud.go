package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"acs/internal/simcpe"
)

// idResp — respons create yang hanya perlu id.
type idResp struct {
	ID uint64 `json:"id"`
}

// S11 — kelengkapan CRUD + guard hapus + revive, terhadap stack hidup (SQL nyata).
// Yang dibuktikan di sini TIDAK bisa dibuktikan unit test berfake: query
// "masih dipakai" (usage_repository), revive device/OUI saat Inform/POST ulang,
// pembatalan task saat device dihapus, dan RBAC/scope tenant di jalur HTTP.
func scenarioCRUDLengkap(ctx context.Context, w *World) result {
	run := strings.ToUpper(w.runID)
	super, admin, admin2 := w.super, w.pilotAdmin, w.pilot2Admin
	expect := func(label string, got int, want ...int) string {
		for _, x := range want {
			if got == x {
				return ""
			}
		}
		return fmt.Sprintf("%s: HTTP %d, harusnya %v", label, got, want)
	}
	check := func(label string, st int, body string, want ...int) string {
		if m := expect(label, st, want...); m != "" {
			return m + " (" + body + ")"
		}
		return ""
	}

	// ---------------- A. Katalog vendor ----------------
	var v idResp
	if err := super.do(http.MethodPost, "/api/v1/vendors", map[string]any{"code": "CRUD" + run, "name": "CRUD Vendor " + run}, &v); err != nil || v.ID == 0 {
		return failr("A: buat vendor: %v", err)
	}
	vp := fmt.Sprintf("/api/v1/vendors/%d", v.ID)

	if st, b := super.status(http.MethodGet, vp, nil); st != 200 {
		return failr("A: GET vendor: HTTP %d %s", st, b)
	}
	if m := check("A: PATCH vendor oleh ADMIN (RBAC)", func() int { s, _ := admin.status(http.MethodPatch, vp, map[string]any{"name": "x"}); return s }(), "", 403); m != "" {
		return failr("%s", m)
	}
	if st, b := super.status(http.MethodPatch, vp, map[string]any{"name": "CRUD Vendor Renamed " + run, "description": "uji CRUD"}); st != 200 {
		return failr("A: PATCH vendor: HTTP %d %s", st, b)
	}
	var vGot struct {
		Name string `json:"name"`
		Code string `json:"code"`
	}
	if err := super.do(http.MethodGet, vp, nil, &vGot); err != nil || !strings.Contains(vGot.Name, "Renamed") || vGot.Code != "CRUD"+run {
		return failr("A: setelah PATCH, name/code salah: %+v err=%v", vGot, err)
	}
	if st, _ := super.status(http.MethodPatch, vp, map[string]any{}); st != 400 {
		return failr("A: PATCH vendor tanpa field harusnya 400, dapat %d", st)
	}

	// OUI: tambah -> hapus -> tambah LAGI (jebakan lama: unique key memblokir).
	oui := ""
	var o idResp
	for i := 0; i < 8 && oui == ""; i++ {
		cand := strings.ToUpper(randHex(3))
		if err := super.do(http.MethodPost, vp+"/ouis", map[string]any{"oui": cand, "notes": "uji CRUD"}, &o); err == nil {
			oui = cand
		} else if !isConflict(err) {
			return failr("A: tambah OUI: %v", err)
		}
	}
	if oui == "" || o.ID == 0 {
		return failr("A: tidak berhasil menambah OUI uji")
	}
	if st, b := super.status(http.MethodDelete, fmt.Sprintf("%s/ouis/%d", vp, o.ID), nil); st != 200 && st != 204 {
		return failr("A: hapus OUI: HTTP %d %s", st, b)
	}
	if st, _ := super.status(http.MethodDelete, fmt.Sprintf("%s/ouis/%d", vp, o.ID), nil); st != 404 {
		return failr("A: hapus OUI yang sudah terhapus harusnya 404, dapat %d", st)
	}
	var o2 idResp
	if err := super.do(http.MethodPost, vp+"/ouis", map[string]any{"oui": oui, "notes": "didaftarkan ulang"}, &o2); err != nil {
		return failr("A: daftar ulang OUI %s yang pernah dihapus ditolak (revive gagal): %v", oui, err)
	}
	var ouis []struct {
		OUI string `json:"oui"`
	}
	if err := super.do(http.MethodGet, vp+"/ouis", nil, &ouis); err != nil || len(ouis) != 1 || !strings.EqualFold(ouis[0].OUI, oui) {
		return failr("A: setelah revive, list OUI harusnya tepat 1 (%s): %+v err=%v", oui, ouis, err)
	}

	// Device model + mapping.
	types, err := super.refMap("ref_device_types")
	if err != nil || len(types) == 0 {
		return failr("A: refMap ref_device_types: %v", err)
	}
	dmvs, err := super.refMap("ref_data_model_versions")
	if err != nil || len(dmvs) == 0 {
		return failr("A: refMap ref_data_model_versions: %v", err)
	}
	var typeID, dmvID uint64
	for _, id := range types {
		typeID = id
		break
	}
	for _, id := range dmvs {
		dmvID = id
		break
	}
	var m idResp
	if err := super.do(http.MethodPost, "/api/v1/device-models", map[string]any{
		"vendor_id": v.ID, "device_type_id": typeID, "data_model_version_id": dmvID,
		"model_name": "CRUD-M1", "product_class": "CRUDPC-" + run,
	}, &m); err != nil || m.ID == 0 {
		return failr("A: buat device model: %v", err)
	}
	mp := fmt.Sprintf("/api/v1/device-models/%d", m.ID)
	if st, b := super.status(http.MethodPatch, mp, map[string]any{"description": "model uji", "product_class": ""}); st != 200 {
		return failr("A: PATCH device model: HTTP %d %s", st, b)
	}
	var mGot struct {
		VendorID     uint64  `json:"vendor_id"`
		ProductClass *string `json:"product_class"`
		Description  *string `json:"description"`
	}
	if err := super.do(http.MethodGet, mp, nil, &mGot); err != nil || mGot.VendorID != v.ID || mGot.ProductClass != nil || mGot.Description == nil {
		return failr("A: model setelah PATCH salah (vendor_id harus tetap, product_class '' = kosongkan): %+v err=%v", mGot, err)
	}

	var mapping idResp
	if err := super.do(http.MethodPost, "/api/v1/vendor-parameter-mappings", map[string]any{
		"vendor_id": v.ID, "data_model_version_id": dmvID,
		"logical_key": "crud.test." + run, "tr069_path": "InternetGatewayDevice.X_CRUD.Test",
	}, &mapping); err != nil || mapping.ID == 0 {
		return failr("A: buat mapping: %v (id=%d)", err, mapping.ID)
	}
	mapP := fmt.Sprintf("/api/v1/vendor-parameter-mappings/%d", mapping.ID)
	if st, b := super.status(http.MethodGet, mapP, nil); st != 200 {
		return failr("A: GET mapping: HTTP %d %s", st, b)
	}

	// Guard: vendor masih punya model + mapping -> 409 yang menyebut penyebabnya.
	st, body := super.status(http.MethodDelete, vp, nil)
	if st != 409 || !strings.Contains(body, "device_model") || !strings.Contains(body, "parameter_mapping") {
		return failr("A: DELETE vendor yang masih dipakai harusnya 409 menyebut device_model & parameter_mapping, dapat HTTP %d: %s", st, body)
	}
	// Guard pada vendor NYATA yang dipakai device hasil S1.
	if zid := w.vendorIDByKey["zte"]; zid != 0 {
		st, body := super.status(http.MethodDelete, fmt.Sprintf("/api/v1/vendors/%d", zid), nil)
		if st != 409 || !strings.Contains(body, "device") {
			return failr("A: DELETE vendor ZTE yang dipakai device harusnya 409, dapat HTTP %d: %s", st, body)
		}
		if st, _ := super.status(http.MethodGet, fmt.Sprintf("/api/v1/vendors/%d", zid), nil); st != 200 {
			return failr("A: vendor ZTE hilang setelah DELETE ditolak (HTTP %d)", st)
		}
	}

	// Bersihkan berurutan: mapping, model, lalu vendor (+ OUI ikut).
	if st, b := super.status(http.MethodDelete, mapP, nil); st != 200 && st != 204 {
		return failr("A: DELETE mapping: HTTP %d %s", st, b)
	}
	if st, _ := super.status(http.MethodGet, mapP, nil); st != 404 {
		return failr("A: GET mapping terhapus harusnya 404, dapat %d", st)
	}
	if st, b := super.status(http.MethodDelete, mp, nil); st != 200 && st != 204 {
		return failr("A: DELETE device model: HTTP %d %s", st, b)
	}
	if st, _ := super.status(http.MethodGet, mp, nil); st != 404 {
		return failr("A: GET model terhapus harusnya 404, dapat %d", st)
	}
	if st, b := super.status(http.MethodDelete, vp, nil); st != 200 && st != 204 {
		return failr("A: DELETE vendor bebas: HTTP %d %s", st, b)
	}
	if st, _ := super.status(http.MethodGet, vp, nil); st != 404 {
		return failr("A: GET vendor terhapus harusnya 404, dapat %d", st)
	}

	// ---------------- B. Device delete + batalkan task + revive saat Inform ----------------
	prof := simcpe.Profiles["zte"]
	serial := fmt.Sprintf("E2E-DEL-%s-01", run)
	dev := simcpe.NewDevice(prof, serial)
	if rep := w.runCPE(ctx, dev, simcpe.SessionOpts{Events: []string{"0 BOOTSTRAP"}}); !rep.OK {
		return failr("B: onboarding device uji: %s", rep.ErrMsg)
	}
	d, err := super.findDeviceBySerial(serial)
	if err != nil {
		return failr("B: %v", err)
	}
	dp := fmt.Sprintf("/api/v1/devices/%d", d.ID)
	if st, b := admin.status(http.MethodPost, dp+"/reboot", map[string]any{}); st >= 400 {
		return failr("B: antre reboot: HTTP %d %s", st, b)
	}
	if m := check("B: DELETE device tenant lain (scope)", func() int { s, _ := admin2.status(http.MethodDelete, dp, nil); return s }(), "", 403, 404); m != "" {
		return failr("%s", m)
	}
	if st, b := super.status(http.MethodGet, dp, nil); st != 200 {
		return failr("B: device hilang setelah DELETE ditolak: HTTP %d %s", st, b)
	}
	if st, b := admin.status(http.MethodDelete, dp, nil); st != 200 && st != 204 {
		return failr("B: DELETE device: HTTP %d %s", st, b)
	}
	if st, _ := super.status(http.MethodGet, dp, nil); st != 404 {
		return failr("B: GET device terhapus harusnya 404, dapat %d", st)
	}
	if !w.deviceHasTask(d.ID, "REBOOT", "CANCELLED") {
		return failr("B: task REBOOT yang antre harusnya CANCELLED setelah device dihapus (dapat: %s)", w.taskDump(d.ID))
	}

	// Tenant LAIN (punya shared secret CWMP-nya sendiri) tidak boleh mengambil alih
	// device yang dihapus tenant pilot lewat Inform dengan OUI+serial yang sama.
	hijack := simcpe.SessionOpts{Events: []string{"0 BOOTSTRAP"}, ACSURL: w.cwmpURL, Username: w.cwmpUser2, Password: w.cwmpPass2}
	hrep := simcpe.RunSession(ctx, w.hc, dev, hijack)
	if hrep.OK || hrep.InformStatus != http.StatusConflict {
		return failr("B: Inform tenant LAIN untuk device yang dihapus tenant pilot harusnya ditolak 409, dapat OK=%v HTTP %d (%s) — revive lintas tenant terbuka", hrep.OK, hrep.InformStatus, hrep.ErrMsg)
	}
	if st, _ := super.status(http.MethodGet, dp, nil); st != 404 {
		return failr("B: device harus tetap terhapus setelah percobaan tenant lain, GET -> %d", st)
	}
	if _, err := admin2.findDeviceBySerial(serial); err == nil {
		return failr("B: tenant pilot2 melihat device yang dihapus tenant pilot (pengambilalihan berhasil)")
	}

	// CPE yang masih menyala Inform lagi -> HARUS diterima (revive), bukan error.
	rep := w.runCPE(ctx, dev, simcpe.SessionOpts{Events: []string{"2 PERIODIC"}})
	if !rep.OK || rep.InformStatus != 200 {
		return failr("B: Inform dari device yang pernah dihapus ditolak (HTTP %d, %s) — revive gagal, CPE akan terputus dari ACS", rep.InformStatus, rep.ErrMsg)
	}
	d2, err := super.findDeviceBySerial(serial)
	if err != nil {
		return failr("B: device tidak muncul lagi setelah Inform: %v", err)
	}
	if d2.ID != d.ID {
		return failr("B: revive harus memakai baris yang sama (id %d), dapat id %d", d.ID, d2.ID)
	}
	if d2.VendorID == nil || *d2.VendorID != w.vendorIDByKey["zte"] {
		return failr("B: vendor harus ter-resolve ulang saat revive, dapat %v", d2.VendorID)
	}
	if w.deviceHasTask(d.ID, "REBOOT", "COMPLETED") {
		return failr("B: task REBOOT lama dieksekusi setelah revive — harusnya sudah dibatalkan")
	}

	// ---------------- C. Tenant & user ----------------
	var t idResp
	if err := super.do(http.MethodPost, "/api/v1/tenants", map[string]any{"code": "DEL" + run, "name": "Tenant Hapus " + run}, &t); err != nil || t.ID == 0 {
		return failr("C: buat tenant uji: %v", err)
	}
	tp := fmt.Sprintf("/api/v1/tenants/%d", t.ID)
	if st, b := super.status(http.MethodGet, tp, nil); st != 200 {
		return failr("C: GET tenant: HTTP %d %s", st, b)
	}
	if m := check("C: DELETE tenant oleh ADMIN (RBAC)", func() int { s, _ := admin.status(http.MethodDelete, tp, nil); return s }(), "", 403); m != "" {
		return failr("%s", m)
	}
	if st, b := super.status(http.MethodDelete, fmt.Sprintf("/api/v1/tenants/%d", w.pilotTenantID), nil); st != 409 || !strings.Contains(b, "device") {
		return failr("C: DELETE tenant pilot (punya user & device) harusnya 409 menyebut device, dapat HTTP %d: %s", st, b)
	}
	if st, b := super.status(http.MethodDelete, tp, nil); st != 200 && st != 204 {
		return failr("C: DELETE tenant kosong: HTTP %d %s", st, b)
	}
	if st, _ := super.status(http.MethodGet, tp, nil); st != 404 {
		return failr("C: GET tenant terhapus harusnya 404, dapat %d", st)
	}
	if st, b := admin.status(http.MethodGet, fmt.Sprintf("/api/v1/tenants/%d", w.pilotTenantID), nil); st != 200 {
		return failr("C: ADMIN GET tenant sendiri: HTTP %d %s", st, b)
	}
	if st, _ := admin.status(http.MethodGet, fmt.Sprintf("/api/v1/tenants/%d", w.pilot2TenantID), nil); st != 403 {
		return failr("C: ADMIN GET tenant lain harusnya 403, dapat %d", st)
	}
	var own, other struct {
		Data []idResp `json:"data"`
	}
	_ = super.do(http.MethodGet, fmt.Sprintf("/api/v1/users?tenant_id=%d", w.pilotTenantID), nil, &own)
	_ = super.do(http.MethodGet, fmt.Sprintf("/api/v1/users?tenant_id=%d", w.pilot2TenantID), nil, &other)
	if len(own.Data) == 0 || len(other.Data) == 0 {
		return failr("C: tidak menemukan user pilot/pilot2 untuk uji GET /users/:id")
	}
	if st, b := admin.status(http.MethodGet, fmt.Sprintf("/api/v1/users/%d", own.Data[0].ID), nil); st != 200 {
		return failr("C: ADMIN GET user tenant sendiri: HTTP %d %s", st, b)
	}
	if st, _ := admin.status(http.MethodGet, fmt.Sprintf("/api/v1/users/%d", other.Data[0].ID), nil); st != 403 {
		return failr("C: ADMIN GET user tenant lain harusnya 403, dapat %d", st)
	}

	// ---------------- D. Firmware ----------------
	var fw idResp
	if err := super.uploadMultipart("/api/v1/firmware", map[string]string{
		"vendor_id": fmt.Sprint(w.vendorIDByKey["zte"]), "version": "CRUD-FW-" + run, "release_notes": "awal",
	}, "file", "crud-"+w.runID+".bin", []byte("CRUD-FIRMWARE-"+w.runID+strings.Repeat("y", 256)), &fw); err != nil || fw.ID == 0 {
		return failr("D: upload firmware: %v", err)
	}
	fp := fmt.Sprintf("/api/v1/firmware/%d", fw.ID)
	if st, b := super.status(http.MethodGet, fp, nil); st != 200 {
		return failr("D: GET firmware: HTTP %d %s", st, b)
	}
	if m := check("D: PATCH firmware oleh ADMIN (RBAC)", func() int { s, _ := admin.status(http.MethodPatch, fp, map[string]any{"is_active": false}); return s }(), "", 403); m != "" {
		return failr("%s", m)
	}
	if st, b := super.status(http.MethodPatch, fp, map[string]any{"is_active": false, "release_notes": "direvisi"}); st != 200 {
		return failr("D: PATCH firmware: HTTP %d %s", st, b)
	}
	var fGot struct {
		IsActive     bool    `json:"is_active"`
		ReleaseNotes *string `json:"release_notes"`
		StorageKey   string  `json:"storage_key"`
	}
	if err := super.do(http.MethodGet, fp, nil, &fGot); err != nil || fGot.IsActive || fGot.ReleaseNotes == nil || *fGot.ReleaseNotes != "direvisi" || fGot.StorageKey == "" {
		return failr("D: firmware setelah PATCH salah: %+v err=%v", fGot, err)
	}
	if st, b := super.status(http.MethodDelete, fp, nil); st != 200 && st != 204 {
		return failr("D: DELETE firmware bebas: HTTP %d %s", st, b)
	}
	if st, _ := super.status(http.MethodGet, fp, nil); st != 404 {
		return failr("D: GET firmware terhapus harusnya 404, dapat %d", st)
	}

	// ---------------- E. Tag, preset, ZTP ----------------
	var tag idResp
	if err := admin.do(http.MethodPost, "/api/v1/tags", map[string]any{"name": "crud-" + run, "color": "#112233"}, &tag); err != nil || tag.ID == 0 {
		return failr("E: buat tag: %v", err)
	}
	tgp := fmt.Sprintf("/api/v1/tags/%d", tag.ID)
	var tGot struct {
		Name  string  `json:"name"`
		Color *string `json:"color"`
	}
	if err := admin.do(http.MethodPatch, tgp, map[string]any{"name": "crud-renamed-" + run}, &tGot); err != nil || tGot.Name != "crud-renamed-"+run || tGot.Color == nil || *tGot.Color != "#112233" {
		return failr("E: PATCH tag (name berubah, color tetap): %+v err=%v", tGot, err)
	}
	if st, _ := admin.status(http.MethodPatch, tgp, map[string]any{}); st != 400 {
		return failr("E: PATCH tag tanpa field harusnya 400, dapat %d", st)
	}
	// Tag GLOBAL (dibuat superadmin tanpa tenant) tidak boleh dimutasi ADMIN tenant.
	var gtag idResp
	if err := super.do(http.MethodPost, "/api/v1/tags", map[string]any{"name": "crud-global-" + run}, &gtag); err != nil || gtag.ID == 0 {
		return failr("E: buat tag global: %v", err)
	}
	gp := fmt.Sprintf("/api/v1/tags/%d", gtag.ID)
	if st, _ := admin.status(http.MethodPatch, gp, map[string]any{"name": "hijack"}); st != 403 {
		return failr("E: ADMIN PATCH tag global harusnya 403, dapat %d", st)
	}
	if st, _ := admin.status(http.MethodDelete, gp, nil); st != 403 {
		return failr("E: ADMIN DELETE tag global harusnya 403, dapat %d", st)
	}
	if st, b := super.status(http.MethodDelete, gp, nil); st != 200 && st != 204 {
		return failr("E: superadmin DELETE tag global: HTTP %d %s", st, b)
	}
	if st, b := admin.status(http.MethodDelete, tgp, nil); st != 200 && st != 204 {
		return failr("E: DELETE tag sendiri: HTTP %d %s", st, b)
	}

	var pr idResp
	if err := admin.do(http.MethodPost, "/api/v1/presets", map[string]any{
		"name": "crud-preset-" + run, "weight": 1, "precondition": "{}", "configurations": "[]", "enforce": false, "is_active": false,
	}, &pr); err != nil || pr.ID == 0 {
		return failr("E: buat preset: %v", err)
	}
	if st, b := admin.status(http.MethodGet, fmt.Sprintf("/api/v1/presets/%d", pr.ID), nil); st != 200 {
		return failr("E: GET preset: HTTP %d %s", st, b)
	}
	if st, _ := admin2.status(http.MethodGet, fmt.Sprintf("/api/v1/presets/%d", pr.ID), nil); st != 403 {
		return failr("E: ADMIN tenant lain GET preset harusnya 403, dapat %d", st)
	}
	_ = admin.do(http.MethodDelete, fmt.Sprintf("/api/v1/presets/%d", pr.ID), nil, nil)

	if st, _ := admin.status(http.MethodGet, "/api/v1/zero-touch-rules/99999999", nil); st != 404 {
		return failr("E: GET zero-touch-rule tak ada harusnya 404, dapat %d", st)
	}

	return pass("CRUD lengkap: katalog (vendor/OUI/model/mapping, revive OUI, guard 409), device delete+revive+task dibatalkan, tenant/user get+guard, firmware get/patch/delete, tag/preset/ZTP — RBAC & scope tenant terjaga")
}
