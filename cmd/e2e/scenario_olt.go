package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// S35 — OLT Controller irisan 1 (migrations/0044, docs/arsitektur/OLT_CONTROLLER_DESIGN.md), lewat driver simulator
// (ACS_OLT_SIM_ENABLED=true di stack e2e). TIDAK menguji OLT nyata: tidak ada driver vendor di irisan ini.
//
//	OLT-01  driver "sim" terdaftar; OLT dibuat, kredensial tersimpan tetapi TIDAK PERNAH muncul di respons (hanya penanda)
//	OLT-02  validasi: host loopback/berskema/berport, driver tak dikenal, port di luar 1-65535, field asing (strict JSON) -> 400
//	OLT-03  RBAC: NOC/VIEWER tidak boleh membuat/menguji; VIEWER hanya baca; polling ADMIN/NOC; abaikan ONU ADMIN/NOC
//	OLT-04  isolasi tenant: tenant lain 403 pada detail/ubah/hapus/uji/poll/antrean, dan daftar OLT/ONU-nya tidak memuatnya
//	OLT-05  uji koneksi mencatat status OK + identitas yang dilaporkan OLT
//	OLT-06  polling merekonsiliasi antrean: baru -> PENDING, hilang -> RESOLVED, IGNORED bertahan, muncul lagi -> PENDING
//	OLT-07  OLT tak terjangkau / autentikasi ditolak -> 502, status tercatat, antrean lama tidak berubah
//	OLT-08  ubah (parsial, kredensial dikosongkan), nonaktif -> poll 409; hapus -> 404 dan antreannya ikut hilang
type oltJSON struct {
	ID               string  `json:"id"`
	TenantID         uint64  `json:"tenant_id"`
	Name             string  `json:"name"`
	DriverCode       string  `json:"driver_code"`
	Host             string  `json:"host"`
	SNMPPort         int     `json:"snmp_port"`
	SSHPort          int     `json:"ssh_port"`
	SSHUsername      *string `json:"ssh_username"`
	IsActive         bool    `json:"is_active"`
	PollStatus       string  `json:"poll_status"`
	LastPollAt       *string `json:"last_poll_at"`
	LastPollError    *string `json:"last_poll_error"`
	ReportedName     *string `json:"reported_name"`
	HasSNMPCommunity bool    `json:"has_snmp_community"`
	HasSSHPassword   bool    `json:"has_ssh_password"`
}

type oltONUJSON struct {
	ID           string `json:"id"`
	OLTID        string `json:"olt_id"`
	SerialNumber string `json:"serial_number"`
	PonPort      string `json:"pon_port"`
	Status       string `json:"status"`
}

type oltPollJSON struct {
	Seen        int `json:"seen"`
	New         int `json:"new"`
	Reappeared  int `json:"reappeared"`
	Resolved    int `json:"resolved"`
	Skipped     int `json:"skipped"`
	QueueLength int `json:"queue_length"`
}

func scenarioOLT(ctx context.Context, w *World) result {
	_ = ctx
	run := w.runID

	// Port SNMP unik per proses e2e: hitungan polling simulator per (host, snmp_port) di memori acsd, sehingga e2e yang diulang pada
	// stack yang sama tetap memulai urutan dari awal.
	h := fnv.New32a()
	_, _ = h.Write([]byte(run))
	snmpPort := 20000 + int(h.Sum32()%20000)

	// ---- fixture: NOC dan VIEWER di tenant pilot ----
	mkUser := func(role string) (*Client, func(), error) {
		user, pw := "s35-"+strings.ToLower(role)+"-"+run, "S35-Pass-"+run+"-Qz7!"
		var created idResp
		if err := w.super.do(http.MethodPost, "/api/v1/users", map[string]any{
			"tenant_id": w.pilotTenantID, "username": user, "email": user + "@example.test", "password": pw,
			"full_name": "S35 " + role, "role_codes": []string{role},
		}, &created); err != nil {
			return nil, nil, fmt.Errorf("buat akun %s: %w", role, err)
		}
		cleanup := func() { _ = w.super.do(http.MethodDelete, fmt.Sprintf("/api/v1/users/%d", created.ID), nil, nil) }
		c := NewClient(w.super.base, false)
		if err := c.Login(user, pw); err != nil {
			cleanup()
			return nil, nil, fmt.Errorf("login %s: %w", role, err)
		}
		return c, cleanup, nil
	}
	noc, cleanNoc, err := mkUser("NOC")
	if err != nil {
		return failr("%v", err)
	}
	defer cleanNoc()
	viewer, cleanViewer, err := mkUser("VIEWER")
	if err != nil {
		return failr("%v", err)
	}
	defer cleanViewer()

	var created []string // OLT yang dibuat, dihapus di akhir (soft delete) walau skenario gagal di tengah
	defer func() {
		for _, id := range created {
			_ = w.pilotAdmin.do(http.MethodDelete, "/api/v1/olts/"+id, nil, nil)
		}
	}()

	create := func(c *Client, host string, port int, extra map[string]any) (oltJSON, int, string) {
		body := map[string]any{"name": "e2e-olt-" + run + "-" + host, "driver_code": "sim", "host": host, "snmp_port": port}
		for k, v := range extra {
			body[k] = v
		}
		var o oltJSON
		raw, code := doRaw(c, http.MethodPost, "/api/v1/olts", body)
		if code == http.StatusCreated {
			_ = json.Unmarshal([]byte(raw), &o)
			created = append(created, o.ID)
		}
		return o, code, raw
	}
	listONUs := func(c *Client, q url.Values) ([]oltONUJSON, int, error) {
		var env listEnvelope[oltONUJSON]
		err := c.do(http.MethodGet, "/api/v1/olt-onus?"+q.Encode(), nil, &env)
		return env.Data, env.Total, err
	}
	poll := func(c *Client, id string) (oltPollJSON, int, string) {
		var s oltPollJSON
		raw, code := doRaw(c, http.MethodPost, "/api/v1/olts/"+id+"/poll", nil)
		if code == http.StatusOK {
			_ = json.Unmarshal([]byte(raw), &s)
		}
		return s, code, raw
	}
	getOLT := func(c *Client, id string) (oltJSON, int) {
		var o oltJSON
		raw, code := doRaw(c, http.MethodGet, "/api/v1/olts/"+id, nil)
		if code == http.StatusOK {
			_ = json.Unmarshal([]byte(raw), &o)
		}
		return o, code
	}
	byStatus := func(id string) (map[string]string, error) {
		rows, _, err := listONUs(w.pilotAdmin, url.Values{"olt": {id}, "page_size": {"100"}})
		m := map[string]string{}
		for _, n := range rows {
			m[n.SerialNumber] = n.Status
		}
		return m, err
	}
	onuIDBySerial := func(id, serial string) (string, error) {
		rows, _, err := listONUs(w.pilotAdmin, url.Values{"olt": {id}, "page_size": {"100"}})
		for _, n := range rows {
			if n.SerialNumber == serial {
				return n.ID, err
			}
		}
		return "", fmt.Errorf("ONU %s tidak ada di antrean", serial)
	}
	const A, B, C, D = "SIMONU0001", "SIMONU0002", "SIMONU0003", "SIMONU0004"
	const sim = "10.255.0."
	rotating := sim + "4"

	// ---- OLT-01 ----
	var drivers struct {
		Data []string `json:"data"`
	}
	if err := w.pilotAdmin.do(http.MethodGet, "/api/v1/olt-drivers", nil, &drivers); err != nil || !contains(drivers.Data, "sim") {
		return failr("OLT-01: GET /olt-drivers harus memuat sim (ACS_OLT_SIM_ENABLED di stack e2e): %v %v", drivers.Data, err)
	}
	const secretCommunity, secretPassword = "komunitas-e2e-rahasia", "sandi-e2e-rahasia"
	main, code, raw := create(w.pilotAdmin, rotating, snmpPort, map[string]any{
		"snmp_community": secretCommunity, "ssh_username": "operator", "ssh_password": secretPassword, "notes": "e2e",
	})
	if code != http.StatusCreated {
		return failr("OLT-01: POST /olts HTTP %d %s", code, raw)
	}
	if !main.HasSNMPCommunity || !main.HasSSHPassword || main.TenantID != w.pilotTenantID || main.PollStatus != "UNKNOWN" || main.SSHPort != 22 || !main.IsActive {
		return failr("OLT-01: isi OLT baru salah: %+v", main)
	}
	if strings.Contains(raw, secretCommunity) || strings.Contains(raw, secretPassword) || strings.Contains(raw, "_enc") {
		return failr("OLT-01: respons POST memuat kredensial/ciphertext: %s", raw)
	}
	if raw, code := doRaw(w.pilotAdmin, http.MethodGet, "/api/v1/olts/"+main.ID, nil); code != http.StatusOK || strings.Contains(raw, secretCommunity) || strings.Contains(raw, secretPassword) {
		return failr("OLT-01: GET detail HTTP %d atau memuat kredensial: %s", code, raw)
	}
	var lst listEnvelope[oltJSON]
	rawList, _ := doRaw(w.pilotAdmin, http.MethodGet, "/api/v1/olts?search="+url.QueryEscape(run), nil)
	if err := json.Unmarshal([]byte(rawList), &lst); err != nil || lst.Total != 1 || strings.Contains(rawList, secretCommunity) || strings.Contains(rawList, secretPassword) {
		return failr("OLT-01: GET /olts total=%d err=%v atau memuat kredensial", lst.Total, err)
	}

	// ---- OLT-02 ----
	bad := map[string]map[string]any{
		"host loopback":      {"host": "127.0.0.1"},
		"host metadata":      {"host": "169.254.169.254"},
		"host berskema":      {"host": "http://10.255.0.9"},
		"host berport":       {"host": "10.255.0.9:22"},
		"driver tak dikenal": {"driver_code": "zte-bukan-driver"},
		"snmp_port 0":        {"snmp_port": 0},
		"ssh_port 70000":     {"ssh_port": 70000},
		"nama kosong":        {"name": "  "},
		"field asing":        {"bukan_field": "x"},
	}
	for name, extra := range bad {
		body := map[string]any{"name": "e2e-bad-" + run, "driver_code": "sim", "host": sim + "9"}
		for k, v := range extra {
			body[k] = v
		}
		if raw, code := doRaw(w.pilotAdmin, http.MethodPost, "/api/v1/olts", body); code != http.StatusBadRequest {
			if code == http.StatusCreated {
				var o oltJSON
				_ = json.Unmarshal([]byte(raw), &o)
				created = append(created, o.ID)
			}
			return failr("OLT-02: %s: HTTP %d, harusnya 400 (%s)", name, code, snippet([]byte(raw)))
		}
	}

	// ---- OLT-03 ----
	for role, c := range map[string]*Client{"NOC": noc, "VIEWER": viewer} {
		if _, code, _ := create(c, sim+"8", 0, nil); code != http.StatusForbidden {
			return failr("OLT-03: %s membuat OLT HTTP %d, harusnya 403", role, code)
		}
	}
	if code, _ := noc.status(http.MethodPost, "/api/v1/olts/"+main.ID+"/test", nil); code != http.StatusForbidden {
		return failr("OLT-03: NOC menguji koneksi HTTP %d, harusnya 403", code)
	}
	if code, _ := noc.status(http.MethodPatch, "/api/v1/olts/"+main.ID, map[string]any{"name": "x"}); code != http.StatusForbidden {
		return failr("OLT-03: NOC mengubah OLT HTTP %d, harusnya 403", code)
	}
	if code, _ := noc.status(http.MethodDelete, "/api/v1/olts/"+main.ID, nil); code != http.StatusForbidden {
		return failr("OLT-03: NOC menghapus OLT HTTP %d, harusnya 403", code)
	}
	if v, code := getOLT(viewer, main.ID); code != http.StatusOK {
		return failr("OLT-03: VIEWER membaca OLT HTTP %d, harusnya 200", code)
	} else if v.SSHUsername != nil || !v.HasSSHPassword {
		return failr("OLT-03: VIEWER tidak boleh melihat username SSH (hanya penanda password): %+v", v)
	}
	if a, _ := getOLT(w.pilotAdmin, main.ID); a.SSHUsername == nil || *a.SSHUsername != "operator" {
		return failr("OLT-03: ADMIN harus melihat username SSH: %+v", a)
	}
	if _, code, _ := poll(viewer, main.ID); code != http.StatusForbidden {
		return failr("OLT-03: VIEWER polling HTTP %d, harusnya 403", code)
	}

	// ---- OLT-04 ----
	if _, code := getOLT(w.pilot2Admin, main.ID); code != http.StatusForbidden {
		return failr("OLT-04: GET OLT lintas tenant HTTP %d, harusnya 403", code)
	}
	for name, call := range map[string]func() int{
		"PATCH": func() int {
			c, _ := w.pilot2Admin.status(http.MethodPatch, "/api/v1/olts/"+main.ID, map[string]any{"name": "x"})
			return c
		},
		"DELETE": func() int { c, _ := w.pilot2Admin.status(http.MethodDelete, "/api/v1/olts/"+main.ID, nil); return c },
		"test": func() int {
			c, _ := w.pilot2Admin.status(http.MethodPost, "/api/v1/olts/"+main.ID+"/test", nil)
			return c
		},
		"poll": func() int { _, c, _ := poll(w.pilot2Admin, main.ID); return c },
		"onus": func() int {
			_, _, err := listONUs(w.pilot2Admin, url.Values{"olt": {main.ID}})
			return errStatus(err)
		},
	} {
		if code := call(); code != http.StatusForbidden {
			return failr("OLT-04: %s lintas tenant HTTP %d, harusnya 403", name, code)
		}
	}
	var other listEnvelope[oltJSON]
	if err := w.pilot2Admin.do(http.MethodGet, "/api/v1/olts?search="+url.QueryEscape(run), nil, &other); err != nil || other.Total != 0 {
		return failr("OLT-04: tenant lain melihat %d OLT (err=%v)", other.Total, err)
	}
	if _, total, err := listONUs(w.pilot2Admin, url.Values{}); err != nil || total != 0 {
		// tenant lain bisa saja punya OLT sendiri dari proses lain, tetapi tidak boleh memuat milik kita
		rows, _, _ := listONUs(w.pilot2Admin, url.Values{"page_size": {"100"}})
		for _, n := range rows {
			if n.OLTID == main.ID {
				return failr("OLT-04: antrean ONU tenant lain memuat entri OLT kita")
			}
		}
	}

	// ---- OLT-05 ----
	var tested oltJSON
	if err := w.pilotAdmin.do(http.MethodPost, "/api/v1/olts/"+main.ID+"/test", nil, &tested); err != nil {
		return failr("OLT-05: POST test: %v", err)
	}
	if tested.PollStatus != "OK" || tested.ReportedName == nil || *tested.ReportedName != "SIM-OLT-"+rotating || tested.LastPollAt == nil || tested.LastPollError != nil {
		return failr("OLT-05: hasil uji koneksi salah: %+v", tested)
	}

	// ---- OLT-06 ----
	s, code, raw := poll(noc, main.ID)
	if code != http.StatusOK || s.Seen != 3 || s.New != 3 || s.QueueLength != 3 {
		return failr("OLT-06: polling 1 HTTP %d %+v %s", code, s, snippet([]byte(raw)))
	}
	if q, err := byStatus(main.ID); err != nil || len(q) != 3 || q[A] != "PENDING" || q[B] != "PENDING" || q[C] != "PENDING" {
		return failr("OLT-06: antrean setelah polling 1: %v %v", q, err)
	}
	bID, err := onuIDBySerial(main.ID, B)
	if err != nil {
		return failr("OLT-06: %v", err)
	}
	if code, _ := viewer.status(http.MethodPatch, "/api/v1/olt-onus/"+bID, map[string]any{"ignored": true}); code != http.StatusForbidden {
		return failr("OLT-06: VIEWER mengabaikan ONU HTTP %d, harusnya 403", code)
	}
	if code, _ := w.pilot2Admin.status(http.MethodPatch, "/api/v1/olt-onus/"+bID, map[string]any{"ignored": true}); code != http.StatusForbidden {
		return failr("OLT-06: tenant lain mengabaikan ONU HTTP %d, harusnya 403", code)
	}
	if code, _ := noc.status(http.MethodPatch, "/api/v1/olt-onus/"+bID, map[string]any{}); code != http.StatusBadRequest {
		return failr("OLT-06: PATCH tanpa 'ignored' HTTP %d, harusnya 400", code)
	}
	var ign oltONUJSON
	if err := noc.do(http.MethodPatch, "/api/v1/olt-onus/"+bID, map[string]any{"ignored": true}, &ign); err != nil || ign.Status != "IGNORED" {
		return failr("OLT-06: NOC mengabaikan ONU: %+v %v", ign, err)
	}
	s, code, raw = poll(w.pilotAdmin, main.ID) // B,C,D
	if code != http.StatusOK || s.New != 1 || s.Resolved != 1 || s.QueueLength != 2 {
		return failr("OLT-06: polling 2 HTTP %d %+v %s", code, s, snippet([]byte(raw)))
	}
	if q, _ := byStatus(main.ID); len(q) != 4 || q[A] != "RESOLVED" || q[B] != "IGNORED" || q[C] != "PENDING" || q[D] != "PENDING" {
		return failr("OLT-06: antrean setelah polling 2: %v", q)
	}
	for st, want := range map[string]int{"PENDING": 2, "IGNORED": 1, "RESOLVED": 1} {
		if _, total, err := listONUs(w.pilotAdmin, url.Values{"olt": {main.ID}, "status": {st}}); err != nil || total != want {
			return failr("OLT-06: filter status=%s total=%d (err=%v), harusnya %d", st, total, err, want)
		}
	}
	if code := errStatus(func() error { _, _, err := listONUs(w.pilotAdmin, url.Values{"status": {"BUKAN"}}); return err }()); code != http.StatusBadRequest {
		return failr("OLT-06: status tak dikenal HTTP %d, harusnya 400", code)
	}
	dID, _ := onuIDBySerial(main.ID, D)
	// mengulang aksi yang sudah berlaku idempoten; entri RESOLVED tidak dapat diubah
	if err := noc.do(http.MethodPatch, "/api/v1/olt-onus/"+bID, map[string]any{"ignored": true}, nil); err != nil {
		return failr("OLT-06: mengabaikan ulang harus idempoten: %v", err)
	}
	aID, _ := onuIDBySerial(main.ID, A)
	if code, _ := noc.status(http.MethodPatch, "/api/v1/olt-onus/"+aID, map[string]any{"ignored": true}); code != http.StatusConflict {
		return failr("OLT-06: mengabaikan entri RESOLVED HTTP %d, harusnya 409", code)
	}
	if err := noc.do(http.MethodPatch, "/api/v1/olt-onus/"+dID, map[string]any{"ignored": true}, nil); err != nil {
		return failr("OLT-06: abaikan D: %v", err)
	}
	if err := noc.do(http.MethodPatch, "/api/v1/olt-onus/"+dID, map[string]any{"ignored": false}, &ign); err != nil || ign.Status != "PENDING" {
		return failr("OLT-06: pulihkan D: %+v %v", ign, err)
	}
	s, code, _ = poll(w.pilotAdmin, main.ID) // kosong
	if code != http.StatusOK || s.Seen != 0 || s.Resolved != 3 || s.QueueLength != 0 {
		return failr("OLT-06: polling 3 (kosong) HTTP %d %+v", code, s)
	}
	if q, _ := byStatus(main.ID); len(q) != 4 || q[A] != "RESOLVED" || q[B] != "RESOLVED" || q[C] != "RESOLVED" || q[D] != "RESOLVED" {
		return failr("OLT-06: antrean setelah polling 3: %v", q)
	}
	s, code, _ = poll(w.pilotAdmin, main.ID) // A,B,C muncul lagi
	if code != http.StatusOK || s.Reappeared != 3 || s.New != 0 || s.QueueLength != 3 {
		return failr("OLT-06: polling 4 HTTP %d %+v", code, s)
	}
	if q, _ := byStatus(main.ID); q[A] != "PENDING" || q[B] != "PENDING" || q[C] != "PENDING" || q[D] != "RESOLVED" {
		return failr("OLT-06: antrean setelah polling 4: %v", q)
	}

	// ---- OLT-07 ----
	for host, want := range map[string]string{sim + "2": "UNREACHABLE", sim + "3": "AUTH_FAILED"} {
		o, code, raw := create(w.pilotAdmin, host, snmpPort, nil)
		if code != http.StatusCreated {
			return failr("OLT-07: buat OLT %s HTTP %d %s", host, code, raw)
		}
		if code, body := w.pilotAdmin.status(http.MethodPost, "/api/v1/olts/"+o.ID+"/test", nil); code != http.StatusBadGateway {
			return failr("OLT-07: %s: uji koneksi HTTP %d (%s), harusnya 502", host, code, snippet([]byte(body)))
		}
		cur, _ := getOLT(w.pilotAdmin, o.ID)
		if cur.PollStatus != want || cur.LastPollError == nil || cur.LastPollAt == nil {
			return failr("OLT-07: %s: status tercatat %+v, harusnya %s dengan galat", host, cur, want)
		}
		if code, body := w.pilotAdmin.status(http.MethodPost, "/api/v1/olts/"+o.ID+"/poll", nil); code != http.StatusBadGateway || strings.Contains(strings.ToLower(body), "panic") {
			return failr("OLT-07: %s: polling HTTP %d (%s), harusnya 502", host, code, snippet([]byte(body)))
		}
	}

	// ---- OLT-08 ----
	var upd oltJSON
	if err := w.pilotAdmin.do(http.MethodPatch, "/api/v1/olts/"+main.ID, map[string]any{"name": "e2e-baru-" + run, "snmp_community": ""}, &upd); err != nil {
		return failr("OLT-08: PATCH: %v", err)
	}
	if upd.Name != "e2e-baru-"+run || upd.HasSNMPCommunity || !upd.HasSSHPassword || upd.Host != rotating {
		return failr("OLT-08: PATCH parsial salah (nama berubah, community kosong, password & host tetap): %+v", upd)
	}
	if code, _ := w.pilotAdmin.status(http.MethodPatch, "/api/v1/olts/"+main.ID, map[string]any{"host": "127.0.0.1"}); code != http.StatusBadRequest {
		return failr("OLT-08: PATCH host loopback HTTP %d, harusnya 400", code)
	}
	if err := w.pilotAdmin.do(http.MethodPatch, "/api/v1/olts/"+main.ID, map[string]any{"is_active": false}, nil); err != nil {
		return failr("OLT-08: nonaktifkan: %v", err)
	}
	if _, code, _ := poll(w.pilotAdmin, main.ID); code != http.StatusConflict {
		return failr("OLT-08: polling OLT nonaktif HTTP %d, harusnya 409", code)
	}
	if code, _ := w.pilotAdmin.status(http.MethodDelete, "/api/v1/olts/"+main.ID, nil); code != http.StatusNoContent && code != 200 {
		return failr("OLT-08: DELETE HTTP %d, harusnya 204", code)
	}
	if _, code := getOLT(w.pilotAdmin, main.ID); code != http.StatusNotFound {
		return failr("OLT-08: GET setelah hapus HTTP %d, harusnya 404", code)
	}
	if code := errStatus(func() error { _, _, err := listONUs(w.pilotAdmin, url.Values{"olt": {main.ID}}); return err }()); code != http.StatusNotFound {
		return failr("OLT-08: antrean OLT terhapus HTTP %d, harusnya 404", code)
	}
	if code, _ := w.pilotAdmin.status(http.MethodDelete, "/api/v1/olts/"+main.ID, nil); code != http.StatusNotFound {
		return failr("OLT-08: hapus ulang HTTP %d, harusnya 404", code)
	}
	return pass("OLT dibuat tanpa kredensial bocor (respons/daftar/detail), 9 masukan buruk 400 (termasuk field asing), RBAC NOC/VIEWER, tenant lain 403 di 5 jalur + daftar kosong, " +
		"uji koneksi OK, rekonsiliasi 4 polling (baru/hilang/IGNORED bertahan/muncul lagi), tak terjangkau & auth ditolak 502 dengan status tercatat, PATCH parsial, nonaktif 409, hapus 404")
}

// doRaw mengirim permintaan JSON dan mengembalikan isi respons beserta KODE STATUS ASLI (Client.do hanya membedakan sukses/galat).
// 429 dicoba ulang seperti Client.do. Galat jaringan menghasilkan kode 0.
func doRaw(c *Client, method, path string, body any) (string, int) {
	var payload []byte
	if body != nil {
		payload, _ = json.Marshal(body)
	}
	for attempt := 0; attempt < 5; attempt++ {
		req, err := http.NewRequest(method, c.base+path, bytes.NewReader(payload))
		if err != nil {
			return err.Error(), 0
		}
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if c.token != "" {
			req.Header.Set("Authorization", "Bearer "+c.token)
		}
		resp, err := c.http.Do(req)
		if err != nil {
			return err.Error(), 0
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests {
			time.Sleep(time.Duration(attempt+1) * 900 * time.Millisecond)
			continue
		}
		return string(raw), resp.StatusCode
	}
	return "429 berulang", http.StatusTooManyRequests
}

func errStatus(err error) int {
	if err == nil {
		return http.StatusOK
	}
	if ae, ok := err.(*apiError); ok {
		return ae.Status
	}
	return 0
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}
