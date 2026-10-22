package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"sort"

	"acs/internal/simcpe"
)

// S32 — query perangkat bebas pada GET /devices (padanan query/projection/sort GenieACS):
//
//	DQ-01  kondisi param=<nama>~<op>~<nilai> memfilter menurut nilai parameter yang dilaporkan CPE (eq/prefix/exists/missing)
//	DQ-02  params= menyertakan nilai parameter pada tiap baris; fields= hanya mengirim kunci yang diminta (+ id)
//	DQ-03  sort=<kolom> / -<kolom> membalik urutan; kolom tak dikenal 400
//	DQ-04  masukan tak sah (op, nama, kunci fields, batas jumlah, tanggal) 400
//	DQ-05  scope tenant tetap berlaku: tenant lain tidak melihat perangkat tenant ini lewat query apa pun
func scenarioDeviceQuery(ctx context.Context, w *World) result {
	key := "zte"
	serial := w.serials[key]
	if w.devices[key] == nil {
		return skipr("device %s dari S1 tidak ada", key)
	}
	prof := simcpe.Profiles[key]
	verPath := prof.RootPrefix() + "DeviceInfo.SoftwareVersion"
	// Versi AKTUAL perangkat (S8 mengubahnya lewat rollout firmware), bukan versi bawaan profil simulator.
	cur, err := w.pilotAdmin.findDeviceBySerial(serial)
	if err != nil || cur.SoftwareVersion == nil || len(*cur.SoftwareVersion) < 2 {
		return failr("perangkat %s tanpa software_version: %v", serial, err)
	}
	ver := *cur.SoftwareVersion

	get := func(c *Client, q url.Values) ([]map[string]json.RawMessage, int, error) {
		var env listEnvelope[map[string]json.RawMessage]
		err := c.do(http.MethodGet, "/api/v1/devices?"+q.Encode(), nil, &env)
		return env.Data, env.Total, err
	}
	serialsOf := func(rows []map[string]json.RawMessage) []string {
		var out []string
		for _, r := range rows {
			var s string
			_ = json.Unmarshal(r["serial_number"], &s)
			out = append(out, s)
		}
		return out
	}
	has := func(list []string, s string) bool {
		for _, x := range list {
			if x == s {
				return true
			}
		}
		return false
	}
	q := func(kv ...string) url.Values {
		v := url.Values{}
		v.Set("page_size", "100")
		for i := 0; i+1 < len(kv); i += 2 {
			v.Add(kv[i], kv[i+1])
		}
		return v
	}

	// DQ-01
	rows, _, err := get(w.pilotAdmin, q("param", verPath+"~eq~"+ver))
	if err != nil {
		return failr("DQ-01 eq: %v", err)
	}
	if !has(serialsOf(rows), serial) {
		return failr("DQ-01: eq %s=%q tidak menemukan perangkat %s (dapat %v)", verPath, ver, serial, serialsOf(rows))
	}
	rows, _, err = get(w.pilotAdmin, q("param", verPath+"~prefix~"+ver[:2]))
	if err != nil || !has(serialsOf(rows), serial) {
		return failr("DQ-01: prefix tidak menemukan perangkat (%v, %v)", serialsOf(rows), err)
	}
	if rows, _, err = get(w.pilotAdmin, q("param", verPath+"~eq~tidak-ada-nilai-ini-"+w.runID)); err != nil || len(rows) != 0 {
		return failr("DQ-01: nilai yang tak ada harus kosong (%d baris, %v)", len(rows), err)
	}
	rows, _, err = get(w.pilotAdmin, q("param", verPath+"~exists"))
	if err != nil || !has(serialsOf(rows), serial) {
		return failr("DQ-01: exists tidak menemukan perangkat (%v)", err)
	}
	if rows, _, err = get(w.pilotAdmin, q("param", prof.RootPrefix()+"X_E2E_TAK_ADA_"+w.runID+"~exists")); err != nil || len(rows) != 0 {
		return failr("DQ-01: exists pada parameter yang tak pernah dilaporkan harus kosong (%d baris, %v)", len(rows), err)
	}
	if rows, _, err = get(w.pilotAdmin, q("param", prof.RootPrefix()+"X_E2E_TAK_ADA_"+w.runID+"~missing")); err != nil || !has(serialsOf(rows), serial) {
		return failr("DQ-01: missing pada parameter yang tak pernah dilaporkan harus memuat perangkat (%v)", err)
	}
	// AND: kondisi kedua yang tak terpenuhi mengosongkan hasil
	if rows, _, err = get(w.pilotAdmin, q("param", verPath+"~exists", "param", verPath+"~eq~bukan-ini")); err != nil || len(rows) != 0 {
		return failr("DQ-01: dua kondisi harus AND (%d baris, %v)", len(rows), err)
	}

	// DQ-02
	rows, _, err = get(w.pilotAdmin, q("param", verPath+"~eq~"+ver, "params", verPath, "fields", "serial_number,parameters"))
	if err != nil || len(rows) == 0 {
		return failr("DQ-02: %v (%d baris)", err, len(rows))
	}
	for _, r := range rows {
		for k := range r {
			if k != "id" && k != "serial_number" && k != "parameters" {
				return failr("DQ-02: fields= hanya boleh id/serial_number/parameters, ada kunci %q", k)
			}
		}
		var ps map[string]string
		if err := json.Unmarshal(r["parameters"], &ps); err != nil || ps[verPath] != ver {
			return failr("DQ-02: parameters[%s] = %v (err %v), harusnya %q", verPath, ps, err, ver)
		}
	}

	// DQ-03
	asc, _, err := get(w.pilotAdmin, q("sort", "serial_number"))
	if err != nil || len(asc) < 2 {
		return failr("DQ-03: sort asc: %v (%d baris)", err, len(asc))
	}
	desc, _, err := get(w.pilotAdmin, q("sort", "-serial_number"))
	if err != nil {
		return failr("DQ-03: sort desc: %v", err)
	}
	a, d := serialsOf(asc), serialsOf(desc)
	if !sort.StringsAreSorted(a) {
		return failr("DQ-03: sort=serial_number tidak terurut naik: %v", a)
	}
	if len(a) != len(d) || a[0] != d[len(d)-1] || a[len(a)-1] != d[0] {
		return failr("DQ-03: -serial_number bukan kebalikan: %v vs %v", a, d)
	}

	// DQ-04
	bad := []struct {
		name string
		q    url.Values
	}{
		{"op tak dikenal", q("param", verPath+"~regex~.*")},
		{"nama berbahaya", q("param", "X;DROP TABLE devices~eq~1")},
		{"tanpa op", q("param", verPath)},
		{"numerik non-angka", q("param", verPath+"~gt~abc")},
		{"kunci fields tak dikenal", q("fields", "inform_password_enc")},
		{"sort tak dikenal", q("sort", "password")},
		{"tanggal bukan RFC3339", q("last_inform_before", "kemarin")},
		{"params nama berbahaya", q("params", "A B")},
		{"terlalu banyak param", q("param", "A~exists", "param", "B~exists", "param", "C~exists", "param", "D~exists", "param", "E~exists", "param", "F~exists")},
	}
	for _, b := range bad {
		if st, _ := w.pilotAdmin.status(http.MethodGet, "/api/v1/devices?"+b.q.Encode(), nil); st != http.StatusBadRequest {
			return failr("DQ-04: %s -> HTTP %d, harusnya 400", b.name, st)
		}
	}
	if rows, _, err = get(w.pilotAdmin, q("last_inform_before", "2000-01-01T00:00:00Z")); err != nil || len(rows) != 0 {
		return failr("DQ-04: last_inform_before 2000 harus kosong (%d baris, %v)", len(rows), err)
	}
	if rows, _, err = get(w.pilotAdmin, q("last_inform_after", "2000-01-01T00:00:00Z")); err != nil || !has(serialsOf(rows), serial) {
		return failr("DQ-04: last_inform_after 2000 harus memuat perangkat (%v)", err)
	}

	// DQ-05
	other, total, err := get(w.pilot2Admin, q("param", verPath+"~exists"))
	if err != nil {
		return failr("DQ-05: %v", err)
	}
	if has(serialsOf(other), serial) {
		return failr("DQ-05: tenant lain melihat perangkat %s lewat query param (total %d)", serial, total)
	}
	if other, _, err = get(w.pilot2Admin, q("param", verPath+"~eq~"+ver, "params", verPath)); err != nil || has(serialsOf(other), serial) {
		return failr("DQ-05: tenant lain melihat perangkat lewat eq+params (%v)", err)
	}

	return pass("param eq/prefix/exists/missing & AND benar; params= + fields= hanya kunci diminta; sort naik/turun saling kebalikan; 9 masukan tak sah 400; last_inform_before/after; tenant lain tidak melihat perangkat lewat query apa pun")
}
