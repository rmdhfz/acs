package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"acs/internal/simcpe"
)

// S33 — virtual parameter (migrations/0039, pkg/vexpr): nilai turunan dari parameter perangkat lewat ekspresi deklaratif.
//
//	VP-01  buat definisi (ADMIN tenant) -> GET /devices/:id/virtual-parameters menghitung dari parameter yang dilaporkan CPE
//	VP-02  proyeksi ?vparams= pada daftar perangkat membawa nilai yang sama
//	VP-03  endpoint uji ekspresi: nilai + rujukan; salah sintaks & rujukan rahasia = hasil (bukan 500)
//	VP-04  validasi: sintaks salah, fungsi tak dikenal, rujukan parameter rahasia, nama jelek -> 400; nama ganda -> 409
//	VP-05  isolasi tenant: tenant lain 403 pada baca/ubah/hapus & pada nilai perangkat; global hanya SUPERADMIN yang mengubah
//	VP-06  ubah ekspresi / nonaktifkan / hapus -> nilai berubah, hilang, 404
type vparamJSON struct {
	ID         string  `json:"id"`
	TenantID   *uint64 `json:"tenant_id"`
	Name       string  `json:"name"`
	Label      string  `json:"label"`
	Expression string  `json:"expression"`
	IsActive   bool    `json:"is_active"`
}

type vvalueJSON struct {
	Name  string  `json:"name"`
	Label string  `json:"label"`
	Unit  *string `json:"unit"`
	Value *string `json:"value"`
}

func scenarioVirtualParameter(ctx context.Context, w *World) result {
	key := "zte"
	serial := w.serials[key]
	if w.devices[key] == nil {
		return skipr("device %s dari S1 tidak ada", key)
	}
	prof := simcpe.Profiles[key]
	d, err := w.pilotAdmin.findDeviceBySerial(serial)
	if err != nil || d.SoftwareVersion == nil {
		return failr("perangkat %s tanpa software_version: %v", serial, err)
	}
	ver := *d.SoftwareVersion
	verPath := prof.RootPrefix() + "DeviceInfo.SoftwareVersion"
	name := "e2e_" + strings.ToLower(strings.ReplaceAll(w.runID, "-", "_"))
	if len(name) > 60 {
		name = name[:60]
	}

	var created []string // uuid untuk dibersihkan
	defer func() {
		for _, id := range created {
			_ = w.pilotAdmin.do(http.MethodDelete, "/api/v1/virtual-parameters/"+id, nil, nil)
			_ = w.super.do(http.MethodDelete, "/api/v1/virtual-parameters/"+id, nil, nil)
		}
	}()
	values := func(c *Client) ([]vvalueJSON, error) {
		var out []vvalueJSON
		err := c.do(http.MethodGet, fmt.Sprintf("/api/v1/devices/%d/virtual-parameters", d.ID), nil, &out)
		return out, err
	}
	find := func(vs []vvalueJSON, n string) *vvalueJSON {
		for i := range vs {
			if vs[i].Name == n {
				return &vs[i]
			}
		}
		return nil
	}

	// VP-01
	var vp vparamJSON
	body := map[string]any{"name": name, "label": "Versi (huruf besar)", "unit": "txt", "expression": "upper(first({" + verPath + "}, 'n/a'))"}
	if err := w.pilotAdmin.do(http.MethodPost, "/api/v1/virtual-parameters", body, &vp); err != nil {
		return failr("VP-01 POST: %v", err)
	}
	created = append(created, vp.ID)
	if vp.ID == "" || vp.TenantID == nil || *vp.TenantID != w.pilotTenantID || !vp.IsActive {
		return failr("VP-01: definisi salah: %+v", vp)
	}
	vs, err := values(w.pilotAdmin)
	if err != nil {
		return failr("VP-01 nilai: %v", err)
	}
	if v := find(vs, name); v == nil || v.Value == nil || *v.Value != strings.ToUpper(ver) || v.Unit == nil || *v.Unit != "txt" {
		return failr("VP-01: nilai %s = %+v, harusnya %q", name, v, strings.ToUpper(ver))
	}

	// VP-02
	q := url.Values{}
	q.Set("search", serial)
	q.Set("vparams", name)
	q.Set("fields", "serial_number,virtual")
	var env listEnvelope[map[string]json.RawMessage]
	if err := w.pilotAdmin.do(http.MethodGet, "/api/v1/devices?"+q.Encode(), nil, &env); err != nil || len(env.Data) == 0 {
		return failr("VP-02: %v (%d baris)", err, len(env.Data))
	}
	var virt map[string]string
	if err := json.Unmarshal(env.Data[0]["virtual"], &virt); err != nil || virt[name] != strings.ToUpper(ver) {
		return failr("VP-02: virtual = %v (err %v), harusnya %s=%q", virt, err, name, strings.ToUpper(ver))
	}
	if st, _ := w.pilotAdmin.status(http.MethodGet, "/api/v1/devices?vparams=Bukan Nama Valid", nil); st != http.StatusBadRequest {
		return failr("VP-02: vparams tak valid HTTP %d, harusnya 400", st)
	}
	// 10 vparams x page_size 200 = 2000 evaluasi > batas 1000 per permintaan (audit keamanan 22 Okt 2026)
	if st, _ := w.pilotAdmin.status(http.MethodGet, "/api/v1/devices?page_size=200&vparams=a,b,c,d,e,f,g,h,i,j", nil); st != http.StatusBadRequest {
		return failr("VP-02: vparams x page_size berlebih HTTP %d, harusnya 400", st)
	}

	// VP-03
	type testRes struct {
		OK    bool    `json:"ok"`
		Value *string `json:"value"`
		Refs  []struct {
			Name  string  `json:"name"`
			Value *string `json:"value"`
		} `json:"refs"`
		Error *struct {
			Pos     int    `json:"pos"`
			Message string `json:"message"`
		} `json:"error"`
	}
	runTest := func(expr string) (testRes, error) {
		var tr testRes
		err := w.pilotAdmin.do(http.MethodPost, "/api/v1/virtual-parameters/test", map[string]any{"expression": expr, "device_id": d.ID}, &tr)
		return tr, err
	}
	tr, err := runTest("len({" + verPath + "}) * 2")
	if err != nil || !tr.OK || tr.Value == nil || *tr.Value != fmt.Sprint(len(ver)*2) || len(tr.Refs) != 1 || tr.Refs[0].Value == nil {
		return failr("VP-03 nilai: %+v (%v), harusnya %d", tr, err, len(ver)*2)
	}
	if tr, err = runTest("1 + * 2"); err != nil || tr.OK || tr.Error == nil || tr.Error.Message == "" {
		return failr("VP-03 sintaks salah harus hasil OK=false: %+v (%v)", tr, err)
	}
	if tr, err = runTest("{" + prof.RootPrefix() + "LANDevice.1.WLANConfiguration.1.KeyPassphrase}"); err != nil || tr.OK || tr.Error == nil {
		return failr("VP-03 rujukan rahasia harus ditolak sebagai hasil: %+v (%v)", tr, err)
	}
	if st, _ := w.pilot2Admin.status(http.MethodPost, "/api/v1/virtual-parameters/test", map[string]any{"expression": "1", "device_id": d.ID}); st != http.StatusForbidden {
		return failr("VP-03 uji pada perangkat tenant lain HTTP %d, harusnya 403", st)
	}

	// VP-04
	bad := []struct {
		why  string
		body map[string]any
		want int
	}{
		{"sintaks salah", map[string]any{"name": "bad1", "label": "x", "expression": "1 +"}, 400},
		{"fungsi tak dikenal", map[string]any{"name": "bad2", "label": "x", "expression": "exec('x')"}, 400},
		{"rujukan rahasia", map[string]any{"name": "bad3", "label": "x", "expression": "{" + prof.RootPrefix() + "LANDevice.1.WLANConfiguration.1.KeyPassphrase}"}, 400},
		{"nama huruf besar", map[string]any{"name": "BadName", "label": "x", "expression": "1"}, 400},
		{"nama ganda", map[string]any{"name": name, "label": "x", "expression": "1"}, 409},
		{"label kosong", map[string]any{"name": "bad4", "label": "", "expression": "1"}, 400},
	}
	for _, b := range bad {
		if st, _ := w.pilotAdmin.status(http.MethodPost, "/api/v1/virtual-parameters", b.body); st != b.want {
			return failr("VP-04: %s -> HTTP %d, harusnya %d", b.why, st, b.want)
		}
	}
	if st, _ := w.pilotAdmin.status(http.MethodPost, "/api/v1/virtual-parameters", map[string]any{"name": "x", "label": "x", "expression": "1", "ekstra": 1}); st != http.StatusBadRequest {
		return failr("VP-04: field tak dikenal HTTP %d, harusnya 400", st)
	}

	// VP-05
	if st, _ := w.pilot2Admin.status(http.MethodGet, "/api/v1/virtual-parameters/"+vp.ID, nil); st != http.StatusForbidden {
		return failr("VP-05: GET lintas tenant HTTP %d, harusnya 403", st)
	}
	if st, _ := w.pilot2Admin.status(http.MethodPatch, "/api/v1/virtual-parameters/"+vp.ID, map[string]any{"label": "hack"}); st != http.StatusForbidden {
		return failr("VP-05: PATCH lintas tenant HTTP %d, harusnya 403", st)
	}
	if st, _ := w.pilot2Admin.status(http.MethodDelete, "/api/v1/virtual-parameters/"+vp.ID, nil); st != http.StatusForbidden {
		return failr("VP-05: DELETE lintas tenant HTTP %d, harusnya 403", st)
	}
	if st, _ := w.pilot2Admin.status(http.MethodGet, fmt.Sprintf("/api/v1/devices/%d/virtual-parameters", d.ID), nil); st != http.StatusForbidden {
		return failr("VP-05: nilai perangkat tenant lain HTTP %d, harusnya 403", st)
	}
	var list listEnvelope[vparamJSON]
	if err := w.pilot2Admin.do(http.MethodGet, "/api/v1/virtual-parameters?page_size=200", nil, &list); err != nil {
		return failr("VP-05 list tenant lain: %v", err)
	}
	for _, x := range list.Data {
		if x.ID == vp.ID {
			return failr("VP-05: definisi tenant lain muncul di daftar tenant ini")
		}
	}
	// global: dibuat SUPERADMIN, dibaca tenant mana pun, tak bisa diubah ADMIN
	var glob vparamJSON
	gname := name + "_g"
	if len(gname) > 64 {
		gname = gname[:64]
	}
	if err := w.super.do(http.MethodPost, "/api/v1/virtual-parameters", map[string]any{"name": gname, "label": "Global", "expression": "1"}, &glob); err != nil {
		return failr("VP-05 buat global: %v", err)
	}
	created = append(created, glob.ID)
	if glob.TenantID != nil {
		return failr("VP-05: superadmin tanpa tenant_id harus membuat definisi global, dapat tenant %d", *glob.TenantID)
	}
	if st, _ := w.pilot2Admin.status(http.MethodGet, "/api/v1/virtual-parameters/"+glob.ID, nil); st >= 400 {
		return failr("VP-05: tenant lain harus bisa membaca definisi global, HTTP %d", st)
	}
	if st, _ := w.pilotAdmin.status(http.MethodPatch, "/api/v1/virtual-parameters/"+glob.ID, map[string]any{"label": "hack"}); st != http.StatusForbidden {
		return failr("VP-05: ADMIN mengubah global HTTP %d, harusnya 403", st)
	}
	if st, _ := w.pilotAdmin.status(http.MethodDelete, "/api/v1/virtual-parameters/"+glob.ID, nil); st != http.StatusForbidden {
		return failr("VP-05: ADMIN menghapus global HTTP %d, harusnya 403", st)
	}
	if vs, err = values(w.pilotAdmin); err != nil || find(vs, gname) == nil {
		return failr("VP-05: definisi global harus berlaku untuk perangkat tenant (%v)", err)
	}

	// VP-06
	if err := w.pilotAdmin.do(http.MethodPatch, "/api/v1/virtual-parameters/"+vp.ID, map[string]any{"expression": "lower({" + verPath + "})"}, &vp); err != nil {
		return failr("VP-06 PATCH: %v", err)
	}
	if vs, _ = values(w.pilotAdmin); find(vs, name) == nil || find(vs, name).Value == nil || *find(vs, name).Value != strings.ToLower(ver) {
		return failr("VP-06: nilai setelah ubah ekspresi = %+v, harusnya %q", find(vs, name), strings.ToLower(ver))
	}
	if st, _ := w.pilotAdmin.status(http.MethodPatch, "/api/v1/virtual-parameters/"+vp.ID, map[string]any{"expression": "1 +"}); st != http.StatusBadRequest {
		return failr("VP-06: PATCH ekspresi salah HTTP %d, harusnya 400", st)
	}
	if err := w.pilotAdmin.do(http.MethodPatch, "/api/v1/virtual-parameters/"+vp.ID, map[string]any{"is_active": false}, &vp); err != nil {
		return failr("VP-06 nonaktifkan: %v", err)
	}
	if vs, _ = values(w.pilotAdmin); find(vs, name) != nil {
		return failr("VP-06: definisi nonaktif tidak boleh dihitung")
	}
	if st, _ := w.pilotAdmin.status(http.MethodDelete, "/api/v1/virtual-parameters/"+vp.ID, nil); st >= 400 {
		return failr("VP-06: DELETE HTTP %d", st)
	}
	if st, _ := w.pilotAdmin.status(http.MethodGet, "/api/v1/virtual-parameters/"+vp.ID, nil); st != http.StatusNotFound {
		return failr("VP-06: GET setelah hapus HTTP %d, harusnya 404", st)
	}

	return pass("definisi tenant dihitung dari parameter CPE (device + proyeksi ?vparams=), uji ekspresi (nilai/sintaks/rahasia sebagai hasil), 6 validasi 400/409, tenant lain 403 (baca/ubah/hapus/nilai), global hanya SUPERADMIN yang mengubah tetapi terbaca semua tenant, ubah/nonaktif/hapus berlaku")
}
