package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"acs/internal/simcpe"
)

// S37 — NBI kompatibel-sebagian GenieACS (GET /api/v1/nbi/devices): query/projection/sort/skip/limit bergaya GenieACS
// diterjemahkan ke query perangkat yang sama dengan GET /devices.
//
//	NBI-01  query DeviceID.SerialNumber -> satu dokumen (_id = device_uuid, DeviceID, _lastInform), header X-Total-Count
//	NBI-02  query parameter ($eq/$exists) + projection -> nilai bersarang {"_value": ...}; yang tak diminta tidak ikut
//	NBI-03  sort satu kunci naik/turun saling kebalikan; skip/limit membagi halaman tanpa baris terlewat/terulang
//	NBI-04  masukan di luar subset (regex, _id, JSON rusak, skip bukan kelipatan limit, $gt pada _lastInform, projection di luar root) 400
//	NBI-05  scope tenant & akses: tenant lain tidak melihat perangkat; tanpa token 401; ENDUSER 403
//	NBI-06  tahap 2 — POST /nbi/devices/{uuid}/tasks, DELETE /nbi/tasks/{uuid}, POST /nbi/tasks/{uuid}/retry: dokumen tugas, rahasia tak bocor, 400/404/401/403
func scenarioNBI(ctx context.Context, w *World) result {
	key := "zte"
	serial := w.serials[key]
	if w.devices[key] == nil {
		return skipr("device %s dari S1 tidak ada", key)
	}
	prof := simcpe.Profiles[key]
	verPath := prof.RootPrefix() + "DeviceInfo.SoftwareVersion"
	cur, err := w.pilotAdmin.findDeviceBySerial(serial)
	if err != nil || cur.SoftwareVersion == nil || *cur.SoftwareVersion == "" {
		return failr("perangkat %s tanpa software_version: %v", serial, err)
	}
	ver := *cur.SoftwareVersion

	// uuid publik perangkat dari GET /devices (field device_uuid), bukan dari PK.
	var list listEnvelope[struct {
		DeviceUUID string `json:"device_uuid"`
	}]
	if err := w.pilotAdmin.do(http.MethodGet, "/api/v1/devices?serial_number="+url.QueryEscape(serial), nil, &list); err != nil || len(list.Data) != 1 {
		return failr("ambil device_uuid: %v (%d baris)", err, len(list.Data))
	}
	wantUUID := list.Data[0].DeviceUUID

	get := func(c *Client, q url.Values) (int, string, []map[string]any, string) {
		req, _ := http.NewRequest(http.MethodGet, c.base+"/api/v1/nbi/devices?"+q.Encode(), nil)
		if c.token != "" {
			req.Header.Set("Authorization", "Bearer "+c.token)
		}
		resp, err := c.http.Do(req)
		if err != nil {
			return 0, "", nil, err.Error()
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		var docs []map[string]any
		if resp.StatusCode == http.StatusOK {
			_ = json.Unmarshal(raw, &docs)
		}
		return resp.StatusCode, resp.Header.Get("X-Total-Count"), docs, string(raw)
	}
	q := func(kv ...string) url.Values {
		v := url.Values{}
		for i := 0; i+1 < len(kv); i += 2 {
			v.Set(kv[i], kv[i+1])
		}
		return v
	}
	serialOf := func(d map[string]any) string {
		did, _ := d["DeviceID"].(map[string]any)
		sn, _ := did["SerialNumber"].(map[string]any)
		s, _ := sn["_value"].(string)
		return s
	}
	jsonStr := func(s string) string { b, _ := json.Marshal(s); return string(b) }

	// NBI-01
	st, total, docs, body := get(w.pilotAdmin, q("query", `{"DeviceID.SerialNumber":`+jsonStr(serial)+`}`))
	if st != 200 || len(docs) != 1 || total != "1" {
		return failr("NBI-01: HTTP %d, %d dokumen, X-Total-Count=%q (%s)", st, len(docs), total, body)
	}
	if docs[0]["_id"] != wantUUID || serialOf(docs[0]) != serial {
		return failr("NBI-01: _id=%v (mau device_uuid %q), serial=%q", docs[0]["_id"], wantUUID, serialOf(docs[0]))
	}
	if _, ok := docs[0]["_lastInform"].(string); !ok {
		return failr("NBI-01: _lastInform tidak ada pada perangkat yang sudah Inform: %s", body)
	}
	for _, leaked := range []string{"tenant_id", "connection_request", "password", "\"id\":"} {
		if strings.Contains(body, leaked) {
			return failr("NBI-01: dokumen membocorkan %q: %s", leaked, body)
		}
	}

	// NBI-02
	st, _, docs, body = get(w.pilotAdmin, q(
		"query", `{`+jsonStr(verPath)+`:{"$eq":`+jsonStr(ver)+`},"DeviceID.SerialNumber":`+jsonStr(serial)+`}`,
		"projection", verPath))
	if st != 200 || len(docs) != 1 {
		return failr("NBI-02: HTTP %d, %d dokumen (%s)", st, len(docs), body)
	}
	var parent map[string]any = docs[0]
	parts := strings.Split(verPath, ".")
	for _, part := range parts[:len(parts)-1] {
		parent, _ = parent[part].(map[string]any)
	}
	leaf, _ := parent[parts[len(parts)-1]].(map[string]any)
	if leaf["_value"] != ver {
		return failr("NBI-02: %s._value = %v, harusnya %q (%s)", verPath, leaf["_value"], ver, body)
	}
	if len(parent) != 1 {
		return failr("NBI-02: parameter yang tidak diminta ikut terkirim (saudara dari %s: %v)", parts[len(parts)-1], parent)
	}
	if st, _, docs, _ = get(w.pilotAdmin, q("query", `{`+jsonStr(verPath)+`:"tidak-ada-`+w.runID+`"}`)); st != 200 || len(docs) != 0 {
		return failr("NBI-02: nilai yang tak ada harus kosong (HTTP %d, %d dokumen)", st, len(docs))
	}
	if st, _, docs, _ = get(w.pilotAdmin, q("query", `{`+jsonStr(prof.RootPrefix()+"X_E2E_TAK_ADA_"+w.runID)+`:{"$exists":false}}`)); st != 200 || len(docs) == 0 {
		return failr("NBI-02: $exists:false pada parameter yang tak pernah dilaporkan harus memuat perangkat (HTTP %d)", st)
	}

	// NBI-03
	_, _, asc, _ := get(w.pilotAdmin, q("sort", `{"DeviceID.SerialNumber":1}`, "limit", "200"))
	_, _, desc, _ := get(w.pilotAdmin, q("sort", `{"DeviceID.SerialNumber":-1}`, "limit", "200"))
	var a, d []string
	for _, x := range asc {
		a = append(a, serialOf(x))
	}
	for _, x := range desc {
		d = append(d, serialOf(x))
	}
	if len(a) < 3 || !sort.StringsAreSorted(a) || len(a) != len(d) || a[0] != d[len(d)-1] || a[len(a)-1] != d[0] {
		return failr("NBI-03: sort naik/turun tidak saling kebalikan (%d vs %d baris): %v / %v", len(a), len(d), a, d)
	}
	var paged []string
	for skip := 0; skip < len(a); skip += 2 {
		_, _, page, _ := get(w.pilotAdmin, q("sort", `{"DeviceID.SerialNumber":1}`, "skip", fmt.Sprint(skip), "limit", "2"))
		for _, x := range page {
			paged = append(paged, serialOf(x))
		}
	}
	if strings.Join(paged, "|") != strings.Join(a, "|") {
		return failr("NBI-03: gabungan halaman skip/limit != daftar utuh (baris terlewat/terulang): %v vs %v", paged, a)
	}

	// NBI-04
	bad := map[string]url.Values{
		"regex":                      q("query", `{`+jsonStr(verPath)+`:{"$regex":"."}}`),
		"kunci _id":                  q("query", `{"_id":"x"}`),
		"JSON rusak":                 q("query", `{"DeviceID.SerialNumber":`),
		"operator logika":            q("query", `{"$or":[]}`),
		"skip bukan kelipatan limit": q("skip", "3", "limit", "2"),
		"limit di atas batas":        q("limit", "201"),
		"_lastInform $gt":            q("query", `{"_lastInform":{"$gt":"2000-01-01T00:00:00Z"}}`),
		"projection di luar root":    q("projection", "VirtualParameters.x"),
		"sort kunci tak dikenal":     q("sort", `{"password":1}`),
		"nama parameter berbahaya":   q("query", `{"Device.X;DROP TABLE devices":"1"}`),
	}
	for name, vals := range bad {
		if st, _, _, body := get(w.pilotAdmin, vals); st != http.StatusBadRequest {
			return failr("NBI-04: %s -> HTTP %d, harusnya 400 (%s)", name, st, body)
		}
	}
	if st, _, docs, _ = get(w.pilotAdmin, q("query", `{"_lastInform":{"$lt":"2000-01-01T00:00:00Z"}}`)); st != 200 || len(docs) != 0 {
		return failr("NBI-04: _lastInform $lt 2000 harus kosong (HTTP %d, %d dokumen)", st, len(docs))
	}
	if st, _, docs, _ = get(w.pilotAdmin, q("query", `{"_lastInform":{"$gte":"2000-01-01T00:00:00Z"},"DeviceID.SerialNumber":`+jsonStr(serial)+`}`)); st != 200 || len(docs) != 1 {
		return failr("NBI-04: _lastInform $gte 2000 harus memuat perangkat (HTTP %d, %d dokumen)", st, len(docs))
	}

	// NBI-05
	st, total, docs, body = get(w.pilot2Admin, q("query", `{"DeviceID.SerialNumber":`+jsonStr(serial)+`}`))
	if st != 200 || len(docs) != 0 || total != "0" {
		return failr("NBI-05: tenant lain melihat perangkat %s (HTTP %d, %d dokumen, total %q)", serial, st, len(docs), total)
	}
	if st, _, docs, _ = get(w.pilot2Admin, q("query", `{`+jsonStr(verPath)+`:{"$eq":`+jsonStr(ver)+`}}`)); st != 200 || len(docs) != 0 {
		return failr("NBI-05: tenant lain melihat perangkat lewat query parameter (HTTP %d, %d dokumen)", st, len(docs))
	}
	if st, _, _, _ = get(NewClient(w.super.base, false), q()); st != http.StatusUnauthorized {
		return failr("NBI-05: tanpa token harus 401, dapat %d", st)
	}
	user, pw := "s37-"+w.runID, "S37-Pass-"+w.runID+"-Qz7!"
	var created idResp
	if err := w.super.do(http.MethodPost, "/api/v1/users", map[string]any{
		"tenant_id": w.pilotTenantID, "username": user, "email": user + "@example.test", "password": pw,
		"full_name": "S37 Pelanggan", "role_codes": []string{"ENDUSER"},
	}, &created); err != nil {
		return failr("NBI-05: buat akun ENDUSER: %v", err)
	}
	defer func() { _ = w.super.do(http.MethodDelete, fmt.Sprintf("/api/v1/users/%d", created.ID), nil, nil) }()
	cust := NewClient(w.super.base, false)
	if err := cust.Login(user, pw); err != nil {
		return failr("NBI-05: login ENDUSER: %v", err)
	}
	if st, _, _, _ = get(cust, q()); st != http.StatusForbidden {
		return failr("NBI-05: ENDUSER harus 403, dapat %d", st)
	}

	// NBI-06: tugas (tahap 2). Semua tugas yang dibuat di sini dibatalkan di akhir supaya tidak mengganggu skenario lain.
	var madeTasks []string
	defer func() {
		for _, id := range madeTasks {
			_ = w.pilotAdmin.do(http.MethodDelete, "/api/v1/nbi/tasks/"+id, nil, nil)
		}
	}()
	tasksURL := "/api/v1/nbi/devices/" + wantUUID + "/tasks"
	type taskDoc struct {
		ID              string     `json:"_id"`
		Device          string     `json:"device"`
		Name            string     `json:"name"`
		Status          string     `json:"status"`
		ParameterNames  []string   `json:"parameterNames"`
		ParameterValues [][]string `json:"parameterValues"`
	}
	var td taskDoc
	if err := w.pilotAdmin.do(http.MethodPost, tasksURL+"?connection_request&timeout=3000", map[string]any{"name": "getParameterValues", "parameterNames": []string{verPath}}, &td); err != nil {
		return failr("NBI-06: buat tugas getParameterValues: %v", err)
	}
	madeTasks = append(madeTasks, td.ID)
	if td.ID == "" || td.Device != wantUUID || td.Name != "getParameterValues" || td.Status != "PENDING" || len(td.ParameterNames) != 1 || td.ParameterNames[0] != verPath {
		return failr("NBI-06: dokumen tugas salah: %+v", td)
	}
	// tugas yang sama terlihat di REST /tasks (satu antrean, bukan salinan terpisah)
	d0, err := w.pilotAdmin.findDeviceBySerial(serial)
	if err != nil {
		return failr("NBI-06: %v", err)
	}
	if ts, err := w.pilotAdmin.listDeviceTasks(d0.ID); err != nil {
		return failr("NBI-06: daftar tugas: %v", err)
	} else {
		found := false
		for _, x := range ts {
			if x.UUID == td.ID {
				found = true
			}
		}
		if !found {
			return failr("NBI-06: tugas NBI %s tidak ada di antrean tugas perangkat", td.ID)
		}
	}
	// hapus -> 204; hapus lagi (sudah CANCELLED) -> 409; ulang -> 200 PENDING lagi; hapus -> 204
	if st, b := w.pilotAdmin.status(http.MethodDelete, "/api/v1/nbi/tasks/"+td.ID, nil); st != http.StatusNoContent && st != 200 {
		return failr("NBI-06: DELETE tugas HTTP %d %s", st, b)
	}
	if st, _ := w.pilotAdmin.status(http.MethodDelete, "/api/v1/nbi/tasks/"+td.ID, nil); st != http.StatusConflict {
		return failr("NBI-06: DELETE tugas yang sudah dibatalkan harus 409, dapat %d", st)
	}
	var rt taskDoc
	if err := w.pilotAdmin.do(http.MethodPost, "/api/v1/nbi/tasks/"+td.ID+"/retry", nil, &rt); err != nil || rt.ID != td.ID || rt.Status != "PENDING" || rt.Device != wantUUID {
		return failr("NBI-06: retry harus membangkitkan tugas yang SAMA berstatus PENDING: %+v (%v)", rt, err)
	}
	if st, _ := w.pilotAdmin.status(http.MethodPost, "/api/v1/nbi/tasks/"+td.ID+"/retry", nil); st != 200 {
		return failr("NBI-06: retry tugas PENDING idempoten (200), dapat %d", st)
	}

	// setParameterValues: nilai rahasia tidak pernah kembali dalam dokumen
	root := verPath[:strings.Index(verPath, ".")+1]
	var sd taskDoc
	pskPath := root + "LANDevice.1.WLANConfiguration.1.PreSharedKey.1.PreSharedKey"
	if err := w.pilotAdmin.do(http.MethodPost, tasksURL, map[string]any{"name": "setParameterValues", "parameterValues": [][]any{
		{root + "LANDevice.1.WLANConfiguration.1.SSID", "NBI-" + w.runID, "xsd:string"}, {pskPath, "RahasiaNBI-" + w.runID + "12"},
	}}, &sd); err != nil {
		return failr("NBI-06: buat tugas setParameterValues: %v", err)
	}
	madeTasks = append(madeTasks, sd.ID)
	rawSet, _ := json.Marshal(sd)
	if strings.Contains(string(rawSet), "RahasiaNBI") || len(sd.ParameterValues) != 2 {
		return failr("NBI-06: dokumen setParameterValues membocorkan rahasia / jumlah salah: %s", rawSet)
	}

	// masukan di luar subset / tak sah -> 400
	for name, body := range map[string]any{
		"factoryReset (belum didukung)": map[string]any{"name": "factoryReset"},
		"tanpa name":                    map[string]any{},
		"nilai null":                    map[string]any{"name": "setParameterValues", "parameterValues": [][]any{{root + "X", nil}}},
		"tipe xsd ngawur":               map[string]any{"name": "setParameterValues", "parameterValues": [][]any{{root + "X", "v", "xsd:ngawur"}}},
		"get tanpa parameterNames":      map[string]any{"name": "getParameterValues"},
	} {
		if st, b := w.pilotAdmin.status(http.MethodPost, tasksURL, body); st != http.StatusBadRequest {
			return failr("NBI-06: %s -> HTTP %d, harusnya 400 (%s)", name, st, b)
		}
	}
	if st, _ := w.pilotAdmin.status(http.MethodPost, tasksURL+"?query=x", map[string]any{"name": "reboot"}); st != http.StatusBadRequest {
		return failr("NBI-06: parameter query tak didukung harus 400, dapat %d", st)
	}
	if st, _ := w.pilotAdmin.status(http.MethodPost, "/api/v1/nbi/devices/00000000-0000-0000-0000-000000000000/tasks", map[string]any{"name": "reboot"}); st != http.StatusNotFound {
		return failr("NBI-06: perangkat tak ada harus 404, dapat %d", st)
	}

	// isolasi & akses: tenant lain tak bisa membedakan UUID milik tenant ini dari UUID yang tak ada
	if st, _ := w.pilot2Admin.status(http.MethodPost, tasksURL, map[string]any{"name": "getParameterValues", "parameterNames": []string{verPath}}); st != http.StatusNotFound {
		return failr("NBI-06: tenant lain membuat tugas pada perangkat ini: HTTP %d, harusnya 404", st)
	}
	for _, c := range []struct{ m, p string }{{http.MethodDelete, "/api/v1/nbi/tasks/" + sd.ID}, {http.MethodPost, "/api/v1/nbi/tasks/" + sd.ID + "/retry"}} {
		if st, _ := w.pilot2Admin.status(c.m, c.p, nil); st != http.StatusNotFound {
			return failr("NBI-06: tenant lain %s %s: HTTP %d, harusnya 404", c.m, c.p, st)
		}
	}
	if st, _ := NewClient(w.super.base, false).status(http.MethodPost, tasksURL, map[string]any{"name": "reboot"}); st != http.StatusUnauthorized {
		return failr("NBI-06: tanpa token harus 401, dapat %d", st)
	}
	if st, _ := cust.status(http.MethodPost, tasksURL, map[string]any{"name": "reboot"}); st != http.StatusForbidden {
		return failr("NBI-06: ENDUSER harus 403, dapat %d", st)
	}

	return pass("query serial/parameter/_lastInform & projection bersarang benar; _id = device_uuid tanpa PK/rahasia; sort naik/turun saling kebalikan dan skip/limit tanpa baris terlewat; 10 masukan di luar subset 400; tenant lain kosong, tanpa token 401, ENDUSER 403; tugas NBI: buat (get/set, rahasia tak bocor) → hapus 204/409 → retry tugas yang sama, masukan di luar subset 400, tenant lain 404, tanpa token 401, ENDUSER 403")
}
