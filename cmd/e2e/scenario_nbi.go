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

	return pass("query serial/parameter/_lastInform & projection bersarang benar; _id = device_uuid tanpa PK/rahasia; sort naik/turun saling kebalikan dan skip/limit tanpa baris terlewat; 10 masukan di luar subset 400; tenant lain kosong, tanpa token 401, ENDUSER 403")
}
