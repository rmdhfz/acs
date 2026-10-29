package main

import (
	"context"
	"fmt"
	"hash/fnv"
	"net/http"
	"net/url"
	"strings"
)

// S39 — OLT Controller irisan B: ONU TERDAFTAR di OLT (migrations/0047), lewat driver simulator (ACS_OLT_SIM_ENABLED di stack e2e).
// TIDAK menguji OLT nyata. Polling berkala (ACS_OLT_POLL_MINUTES) dan siaran WebSocket diuji di unit test, bukan di sini.
//
//	INV-01  poll mengisi inventaris: ringkasan, daftar + filter (port/keadaan/cari), masukan buruk 400
//	INV-02  ringkasan per port PON (jumlah per keadaan)
//	INV-03  tautan ONU di OLT <-> perangkat ACS lewat serial (tak peka huruf besar/kecil) dan tenant yang sama
//	INV-04  diagnosis memakai bukti OLT: LOS/dying gasp = kesimpulan fisik, ONLINE di OLT tetapi belum Inform = lapisan optik sehat, serial tak ada = OLT_NOT_FOUND
//	INV-05  isolasi tenant (OLT, daftar, port, tautan perangkat) dan akses (tanpa token 401, VIEWER/NOC boleh baca)
//	INV-06  mode berputar: ONU baru, keadaan berubah (ONLINE -> LOS), ONU hilang dihapus-lunak
//	INV-07  OLT dihapus: inventaris ikut tak terbaca dan tautan perangkat lepas
func scenarioOLTInventory(ctx context.Context, w *World) result {
	_ = ctx
	run := w.runID
	h := fnv.New32a()
	_, _ = h.Write([]byte("inv" + run))
	snmpPort := 20000 + int(h.Sum32()%20000)

	type regONU struct {
		ID           string  `json:"id"`
		OLTID        string  `json:"olt_id"`
		OLTName      string  `json:"olt_name"`
		PonPort      string  `json:"pon_port"`
		ONUIndex     *string `json:"onu_index"`
		SerialNumber string  `json:"serial_number"`
		State        string  `json:"state"`
		DeviceID     *uint64 `json:"device_id"`
		DeviceStatus *string `json:"device_status"`
	}
	type portJSON struct {
		PonPort   string `json:"pon_port"`
		Total     int    `json:"total"`
		Online    int    `json:"online"`
		Offline   int    `json:"offline"`
		LOS       int    `json:"los"`
		DyingGasp int    `json:"dying_gasp"`
		Linked    int    `json:"linked"`
	}
	type regSummary struct {
		Seen         int            `json:"seen"`
		New          int            `json:"new"`
		Removed      int            `json:"removed"`
		StateChanged int            `json:"state_changed"`
		ByState      map[string]int `json:"by_state"`
	}
	type pollResp struct {
		Seen       int         `json:"seen"`
		Registered *regSummary `json:"registered"`
	}
	type devOLT struct {
		Linked       bool    `json:"linked"`
		TenantHasOLT bool    `json:"tenant_has_olt"`
		ONU          *regONU `json:"onu"`
	}
	type diagFinding struct {
		Code     string `json:"code"`
		Severity string `json:"severity"`
		Evidence string `json:"evidence"`
	}
	type diagResp struct {
		Verdict  string        `json:"verdict"`
		Summary  string        `json:"summary"`
		Limits   string        `json:"limits"`
		Findings []diagFinding `json:"findings"`
		OLT      *struct {
			OLTName string `json:"olt_name"`
			PonPort string `json:"pon_port"`
			State   string `json:"state"`
		} `json:"olt"`
	}

	var cleanups []func()
	defer func() {
		for i := len(cleanups) - 1; i >= 0; i-- {
			cleanups[i]()
		}
	}()

	mkOLT := func(c *Client, name, host string, port int) (oltJSON, error) {
		var o oltJSON
		err := c.do(http.MethodPost, "/api/v1/olts", map[string]any{"name": name, "driver_code": "sim", "host": host, "snmp_port": port}, &o)
		if err == nil {
			id := o.ID
			cleanups = append(cleanups, func() { _ = c.do(http.MethodDelete, "/api/v1/olts/"+id, nil, nil) })
		}
		return o, err
	}
	poll := func(c *Client, id string) (pollResp, error) {
		var p pollResp
		err := c.do(http.MethodPost, "/api/v1/olts/"+id+"/poll", nil, &p)
		return p, err
	}
	listReg := func(c *Client, id string, q url.Values) ([]regONU, int, error) {
		var env listEnvelope[regONU]
		err := c.do(http.MethodGet, "/api/v1/olts/"+id+"/registered-onus?"+q.Encode(), nil, &env)
		return env.Data, env.Total, err
	}
	mkDeviceOUI := func(c *Client, oui, serial string) (uint64, error) {
		var d struct {
			ID uint64 `json:"id"`
		}
		if err := c.do(http.MethodPost, "/api/v1/devices", map[string]any{"oui": oui, "serial_number": serial}, &d); err != nil {
			return 0, err
		}
		id := d.ID
		cleanups = append(cleanups, func() { _ = c.do(http.MethodDelete, fmt.Sprintf("/api/v1/devices/%d", id), nil, nil) })
		return id, nil
	}
	mkDevice := func(c *Client, serial string) (uint64, error) { return mkDeviceOUI(c, "ABCDEF", serial) }
	serialsOf := func(rows []regONU) []string {
		out := make([]string, len(rows))
		for i, r := range rows {
			out[i] = r.SerialNumber
		}
		return out
	}
	states := func(rows []regONU) map[string]string {
		m := map[string]string{}
		for _, r := range rows {
			m[r.SerialNumber] = r.State
		}
		return m
	}

	// ---- INV-01 ----
	main, err := mkOLT(w.pilotAdmin, "e2e-inv-"+run, "10.1.2.3", snmpPort)
	if err != nil {
		return failr("INV-01: buat OLT: %v", err)
	}
	p, err := poll(w.pilotAdmin, main.ID)
	if err != nil || p.Registered == nil {
		return failr("INV-01: poll: %v (%+v)", err, p)
	}
	if p.Registered.Seen != 5 || p.Registered.New != 5 || p.Registered.Removed != 0 || p.Registered.StateChanged != 0 ||
		p.Registered.ByState["ONLINE"] != 2 || p.Registered.ByState["LOS"] != 1 || p.Registered.ByState["DYING_GASP"] != 1 || p.Registered.ByState["OFFLINE"] != 1 {
		return failr("INV-01: ringkasan poll = %+v", p.Registered)
	}
	rows, total, err := listReg(w.pilotAdmin, main.ID, url.Values{})
	want := map[string]string{"SIMREG0001": "ONLINE", "SIMREG0002": "LOS", "SIMREG0003": "DYING_GASP", "SIMREG0004": "ONLINE", "SIMREG0005": "OFFLINE"}
	got := states(rows)
	if err != nil || total != 5 {
		return failr("INV-01: daftar total=%d err=%v", total, err)
	}
	for k, v := range want {
		if got[k] != v {
			return failr("INV-01: keadaan %s = %q, mau %q (semua: %v)", k, got[k], v, got)
		}
	}
	if rows[0].OLTName != "e2e-inv-"+run || rows[0].OLTID != main.ID || rows[0].PonPort == "" || rows[0].ONUIndex == nil {
		return failr("INV-01: baris tak lengkap: %+v", rows[0])
	}
	for name, c := range map[string]struct {
		q    url.Values
		want int
	}{
		"per port":                    {url.Values{"pon_port": {"sim/1/1"}}, 3},
		"per keadaan":                 {url.Values{"state": {"LOS"}}, 1},
		"cari serial":                 {url.Values{"search": {"REG0004"}}, 1},
		"bermasalah":                  {url.Values{"problem": {"true"}}, 3},                          // LOS + dying gasp + offline
		"bermasalah di port 1/2":      {url.Values{"problem": {"true"}, "pon_port": {"sim/1/2"}}, 1}, // hanya SIMREG0005 (offline)
		"problem=false tak menyaring": {url.Values{"problem": {"false"}}, 5},
	} {
		r, tot, err := listReg(w.pilotAdmin, main.ID, c.q)
		if err != nil || len(r) != c.want || tot != c.want {
			return failr("INV-01: filter %s: %d baris (total %d) err=%v, mau %d", name, len(r), tot, err, c.want)
		}
	}
	for name, q := range map[string]url.Values{
		"keadaan tak dikenal": {"state": {"MELAYANG"}},
		"port kepanjangan":    {"pon_port": {strings.Repeat("x", 40)}},
		"cari kepanjangan":    {"search": {strings.Repeat("x", 300)}},
		"problem tak valid":   {"problem": {"mungkin"}},
		"sort tak dikenal":    {"sort": {"acak"}},
	} {
		if st, _ := w.pilotAdmin.status(http.MethodGet, "/api/v1/olts/"+main.ID+"/registered-onus?"+q.Encode(), nil); st != http.StatusBadRequest {
			return failr("INV-01: %s -> HTTP %d, harusnya 400", name, st)
		}
	}

	// urut keparahan: LOS, dying gasp, offline dulu; tanpa sort urutan port lama tetap
	if r, _, err := listReg(w.pilotAdmin, main.ID, url.Values{"sort": {"severity"}}); err != nil || len(r) != 5 || r[0].SerialNumber != "SIMREG0002" || r[1].SerialNumber != "SIMREG0003" || r[2].SerialNumber != "SIMREG0005" {
		return failr("INV-01: sort=severity harus LOS, dying gasp, offline lebih dulu: %v err=%v", serialsOf(r), err)
	}
	if r, _, err := listReg(w.pilotAdmin, main.ID, url.Values{}); err != nil || len(r) != 5 || r[0].SerialNumber != "SIMREG0001" {
		return failr("INV-01: tanpa sort urutan lama (port, nomor ONU) harus tetap: %v err=%v", serialsOf(r), err)
	}

	// ---- INV-02 ----
	var portsEnv listEnvelope[portJSON]
	if err := w.pilotAdmin.do(http.MethodGet, "/api/v1/olts/"+main.ID+"/ports", nil, &portsEnv); err != nil || len(portsEnv.Data) != 2 {
		return failr("INV-02: ports: %v (%d port)", err, len(portsEnv.Data))
	}
	p1, p2 := portsEnv.Data[0], portsEnv.Data[1]
	if p1.PonPort != "sim/1/1" || p1.Total != 3 || p1.Online != 1 || p1.LOS != 1 || p1.DyingGasp != 1 || p2.PonPort != "sim/1/2" || p2.Total != 2 || p2.Online != 1 || p2.Offline != 1 {
		return failr("INV-02: ringkasan port salah: %+v / %+v", p1, p2)
	}

	// ---- INV-03: tautan lewat serial (huruf kecil di ACS, huruf besar di OLT) ----
	devLos, err := mkDevice(w.pilotAdmin, "simreg0002")
	if err != nil {
		return failr("INV-03: buat perangkat: %v", err)
	}
	devOnline, err := mkDevice(w.pilotAdmin, "SIMREG0001")
	if err != nil {
		return failr("INV-03: buat perangkat: %v", err)
	}
	devGasp, err := mkDevice(w.pilotAdmin, "SIMREG0003")
	if err != nil {
		return failr("INV-03: buat perangkat: %v", err)
	}
	devNone, err := mkDevice(w.pilotAdmin, "S39NOONU"+strings.ToUpper(run))
	if err != nil {
		return failr("INV-03: buat perangkat: %v", err)
	}
	rows, _, _ = listReg(w.pilotAdmin, main.ID, url.Values{})
	byS := map[string]regONU{}
	for _, r := range rows {
		byS[r.SerialNumber] = r
	}
	for serial, id := range map[string]uint64{"SIMREG0002": devLos, "SIMREG0001": devOnline, "SIMREG0003": devGasp} {
		r := byS[serial]
		if r.DeviceID == nil || *r.DeviceID != id || r.DeviceStatus == nil || *r.DeviceStatus != "UNREGISTERED" {
			return failr("INV-03: tautan %s = %+v, mau perangkat %d berstatus UNREGISTERED", serial, r, id)
		}
	}
	if byS["SIMREG0004"].DeviceID != nil {
		return failr("INV-03: ONU tanpa perangkat tidak boleh tertaut: %+v", byS["SIMREG0004"].DeviceID)
	}
	var v devOLT
	if err := w.pilotAdmin.do(http.MethodGet, fmt.Sprintf("/api/v1/devices/%d/olt", devLos), nil, &v); err != nil || !v.Linked || v.ONU == nil || v.ONU.State != "LOS" || v.ONU.PonPort != "sim/1/1" || !v.TenantHasOLT {
		return failr("INV-03: /devices/{id}/olt = %+v err=%v", v, err)
	}
	if err := w.pilotAdmin.do(http.MethodGet, fmt.Sprintf("/api/v1/devices/%d/olt", devNone), nil, &v); err != nil || v.Linked || v.ONU != nil || !v.TenantHasOLT {
		return failr("INV-03: perangkat tanpa ONU harus linked=false, tenant_has_olt=true: %+v err=%v", v, err)
	}

	// ---- INV-04: diagnosis memakai bukti OLT ----
	diag := func(id uint64) (diagResp, error) {
		var d diagResp
		err := w.pilotAdmin.do(http.MethodGet, fmt.Sprintf("/api/v1/devices/%d/diagnosis", id), nil, &d)
		return d, err
	}
	finding := func(d diagResp, code string) *diagFinding {
		for i := range d.Findings {
			if d.Findings[i].Code == code {
				return &d.Findings[i]
			}
		}
		return nil
	}
	d, err := diag(devLos)
	if err != nil {
		return failr("INV-04: diagnosis LOS: %v", err)
	}
	f := finding(d, "OLT_LOS")
	if f == nil || f.Severity != "critical" || d.Verdict != "NEVER_CONNECTED" || d.OLT == nil || d.OLT.PonPort != "sim/1/1" || d.OLT.State != "LOS" ||
		!strings.HasPrefix(d.Summary, "Dugaan utama: OLT melaporkan ONU kehilangan sinyal optik") || !strings.Contains(d.Limits, "data OLT") {
		return failr("INV-04: diagnosis LOS = %+v finding=%+v", d, f)
	}
	// ONU baru ditemukan oleh SATU polling: tidak boleh dikatakan baru berubah (waktu mulainya tidak diketahui)
	if !strings.Contains(f.Evidence, "waktu mulainya sebelum itu tidak diketahui") || strings.Contains(f.Evidence, "terpantau berubah") {
		return failr("INV-04: bukti LOS harus jujur soal waktu mulai untuk ONU yang baru ditemukan: %q", f.Evidence)
	}
	if !strings.Contains(f.Evidence, "OLT e2e-inv-"+run) || !strings.Contains(f.Evidence, "port sim/1/1") {
		return failr("INV-04: evidence harus menyebut OLT dan port: %q", f.Evidence)
	}
	if d, err = diag(devGasp); err != nil || finding(d, "OLT_DYING_GASP") == nil {
		return failr("INV-04: dying gasp: %v %+v", err, d.Findings)
	}
	d, err = diag(devOnline)
	if err != nil {
		return failr("INV-04: diagnosis online: %v", err)
	}
	if f := finding(d, "OLT_ONLINE_ACS_SILENT"); f == nil || !strings.Contains(f.Evidence, "belum pernah menerima Inform") {
		return failr("INV-04: ONLINE di OLT tetapi belum Inform harus OLT_ONLINE_ACS_SILENT: %+v", d.Findings)
	}
	if d, err = diag(devNone); err != nil || finding(d, "OLT_NOT_FOUND") == nil || d.OLT != nil || strings.Contains(d.Limits, "data OLT") {
		return failr("INV-04: serial tak ada di OLT harus OLT_NOT_FOUND tanpa bukti OLT: %v %+v", err, d)
	}

	// ---- INV-05: isolasi & akses ----
	for name, path := range map[string]string{
		"daftar": "/api/v1/olts/" + main.ID + "/registered-onus", "port": "/api/v1/olts/" + main.ID + "/ports",
	} {
		if st, _ := w.pilot2Admin.status(http.MethodGet, path, nil); st != http.StatusForbidden && st != http.StatusNotFound {
			return failr("INV-05: tenant lain membaca %s OLT: HTTP %d", name, st)
		}
	}
	if st, _ := w.pilot2Admin.status(http.MethodGet, fmt.Sprintf("/api/v1/devices/%d/olt", devLos), nil); st != http.StatusForbidden && st != http.StatusNotFound {
		return failr("INV-05: tenant lain membaca posisi OLT perangkat tenant ini: HTTP %d", st)
	}
	// Serial yang sama di tenant lain: kunci unik perangkat adalah (OUI, serial) lintas tenant, jadi OUI-nya sengaja berbeda.
	foreign, err := mkDeviceOUI(w.pilot2Admin, "FEDCBA", "SIMREG0002")
	if err != nil {
		return failr("INV-05: buat perangkat tenant lain: %v", err)
	}
	var fv devOLT
	if err := w.pilot2Admin.do(http.MethodGet, fmt.Sprintf("/api/v1/devices/%d/olt", foreign), nil, &fv); err != nil || fv.Linked || fv.TenantHasOLT {
		return failr("INV-05: perangkat tenant lain dengan serial sama TIDAK boleh tertaut ke OLT tenant ini: %+v err=%v", fv, err)
	}
	if fd, err := func() (diagResp, error) {
		var x diagResp
		return x, w.pilot2Admin.do(http.MethodGet, fmt.Sprintf("/api/v1/devices/%d/diagnosis", foreign), nil, &x)
	}(); err != nil || fd.OLT != nil || finding(fd, "OLT_LOS") != nil || finding(fd, "OLT_NOT_FOUND") != nil {
		return failr("INV-05: diagnosis tenant lain tidak boleh memakai bukti OLT tenant ini: %v %+v", err, fd)
	}
	if st, _ := NewClient(w.super.base, false).status(http.MethodGet, "/api/v1/olts/"+main.ID+"/registered-onus", nil); st != http.StatusUnauthorized {
		return failr("INV-05: tanpa token harus 401, dapat %d", st)
	}
	noc := NewClient(w.super.base, false)
	user, pw := "s39-noc-"+run, "S39-Pass-"+run+"-Qz7!"
	var created idResp
	if err := w.super.do(http.MethodPost, "/api/v1/users", map[string]any{
		"tenant_id": w.pilotTenantID, "username": user, "email": user + "@example.test", "password": pw, "full_name": "S39 NOC", "role_codes": []string{"VIEWER"},
	}, &created); err != nil {
		return failr("INV-05: buat akun VIEWER: %v", err)
	}
	cleanups = append(cleanups, func() { _ = w.super.do(http.MethodDelete, fmt.Sprintf("/api/v1/users/%d", created.ID), nil, nil) })
	if err := noc.Login(user, pw); err != nil {
		return failr("INV-05: login VIEWER: %v", err)
	}
	if r, tot, err := listReg(noc, main.ID, url.Values{}); err != nil || tot != 5 || len(r) != 5 {
		return failr("INV-05: VIEWER harus bisa membaca daftar: %d err=%v", tot, err)
	}

	// ---- INV-06: mode berputar ----
	rot, err := mkOLT(w.pilotAdmin, "e2e-inv-rot-"+run, "10.255.0.4", snmpPort+1)
	if err != nil {
		return failr("INV-06: buat OLT berputar: %v", err)
	}
	// INV-08 (disiapkan di sini): langganan webhook ONU_STATE_CHANGED harus sudah ada SEBELUM perubahan keadaan terjadi.
	var sub struct {
		ID uint64 `json:"id"`
	}
	if err := w.pilotAdmin.do(http.MethodPost, "/api/v1/webhooks", map[string]any{
		"event_type": "ONU_STATE_CHANGED", "name": "e2e-onu-" + run, "target_url": "https://203.0.113.9/e2e-onu-hook",
	}, &sub); err != nil || sub.ID == 0 {
		return failr("INV-08: buat langganan ONU_STATE_CHANGED: %v", err)
	}
	cleanups = append(cleanups, func() { _ = w.pilotAdmin.do(http.MethodDelete, fmt.Sprintf("/api/v1/webhooks/%d", sub.ID), nil, nil) })
	type delivery struct {
		Payload struct {
			Event string `json:"event"`
			Data  struct {
				OLTID     string `json:"olt_id"`
				OLTName   string `json:"olt_name"`
				Changed   int    `json:"changed"`
				Truncated bool   `json:"truncated"`
				Changes   []struct {
					Serial string `json:"serial_number"`
					From   string `json:"from"`
					To     string `json:"to"`
				} `json:"changes"`
			} `json:"data"`
		} `json:"payload"`
	}
	deliveries := func() ([]delivery, error) {
		var env listEnvelope[delivery]
		err := w.pilotAdmin.do(http.MethodGet, fmt.Sprintf("/api/v1/webhooks/%d/deliveries", sub.ID), nil, &env)
		return env.Data, err
	}
	if p, err = poll(w.pilotAdmin, rot.ID); err != nil || p.Registered == nil || p.Registered.New != 5 || p.Registered.StateChanged != 0 {
		return failr("INV-06: poll 1 = %+v err=%v", p.Registered, err)
	}
	if ds, err := deliveries(); err != nil || len(ds) != 0 {
		return failr("INV-08: polling pertama (semua ONU baru) tak boleh memicu webhook: %d err=%v", len(ds), err)
	}
	if p, err = poll(w.pilotAdmin, rot.ID); err != nil || p.Registered == nil {
		return failr("INV-06: poll 2: %v", err)
	}
	if s := p.Registered; s.StateChanged != 1 || s.New != 1 || s.Removed != 1 || s.Seen != 5 {
		return failr("INV-06: poll 2 harus 1 berubah/1 baru/1 hilang: %+v", s)
	}
	rows, _, _ = listReg(w.pilotAdmin, rot.ID, url.Values{})
	got = states(rows)
	if got["SIMREG0002"] != "LOS" || got["SIMREG0006"] != "ONLINE" || got["SIMREG0005"] != "" || len(got) != 5 {
		return failr("INV-06: inventaris setelah poll 2 = %v", got)
	}

	// ---- INV-09: riwayat perubahan keadaan ONU (migrations/0049) ----
	// OLT berputar: poll 1 mencatat 5 ONU baru, poll 2 mengubah SIMREG0002 ONLINE -> LOS. Riwayat SIMREG0002 = [ONLINE>LOS, baru>ONLINE] (terbaru dulu).
	var losONU regONU
	for _, r := range rows {
		if r.SerialNumber == "SIMREG0002" {
			losONU = r
		}
	}
	if losONU.ID == "" {
		return failr("INV-09: SIMREG0002 tidak ada di daftar OLT berputar")
	}
	var hist struct {
		Data []struct {
			PonPort   string   `json:"pon_port"`
			FromState *string  `json:"from_state"`
			ToState   *string  `json:"to_state"`
			RXPower   *float64 `json:"rx_power_dbm"`
			Occurred  string   `json:"occurred_at"`
		} `json:"data"`
		Total      int  `json:"total"`
		Changes24h int  `json:"changes_24h"`
		Flapping   bool `json:"flapping"`
	}
	histURL := "/api/v1/olts/" + rot.ID + "/registered-onus/" + losONU.ID + "/history"
	if err := w.pilotAdmin.do(http.MethodGet, histURL, nil, &hist); err != nil {
		return failr("INV-09: GET riwayat: %v", err)
	}
	if hist.Total != 2 || len(hist.Data) != 2 || hist.Changes24h != 1 || hist.Flapping {
		return failr("INV-09: total=%d changes_24h=%d flapping=%v, mau 2/1/false: %+v", hist.Total, hist.Changes24h, hist.Flapping, hist.Data)
	}
	a, b := hist.Data[0], hist.Data[1]
	if a.FromState == nil || *a.FromState != "ONLINE" || a.ToState == nil || *a.ToState != "LOS" || b.FromState != nil || b.ToState == nil || *b.ToState != "ONLINE" || a.Occurred == "" || a.PonPort == "" {
		return failr("INV-09: urutan/isi riwayat salah (terbaru dulu: ONLINE>LOS, lalu baru>ONLINE): %+v", hist.Data)
	}
	// ONU yang tidak berubah hanya punya peristiwa "baru"
	var stableONU regONU
	for _, r := range rows {
		if r.SerialNumber == "SIMREG0001" {
			stableONU = r
		}
	}
	var h1 struct {
		Total      int `json:"total"`
		Changes24h int `json:"changes_24h"`
	}
	if err := w.pilotAdmin.do(http.MethodGet, "/api/v1/olts/"+rot.ID+"/registered-onus/"+stableONU.ID+"/history", nil, &h1); err != nil || h1.Total != 1 || h1.Changes24h != 0 {
		return failr("INV-09: ONU stabil harus 1 peristiwa (baru) dan 0 perubahan: %+v err=%v", h1, err)
	}
	// akses: ONU tak ada 404, identifier kepanjangan 400, tenant lain ditolak, tanpa token 401, ONU dari OLT lain 404
	if st, _ := w.pilotAdmin.status(http.MethodGet, "/api/v1/olts/"+rot.ID+"/registered-onus/00000000-0000-0000-0000-000000000000/history", nil); st != http.StatusNotFound {
		return failr("INV-09: ONU tak ada harus 404, dapat %d", st)
	}
	if st, _ := w.pilotAdmin.status(http.MethodGet, "/api/v1/olts/"+rot.ID+"/registered-onus/"+strings.Repeat("x", 40)+"/history", nil); st != http.StatusBadRequest {
		return failr("INV-09: identifier kepanjangan harus 400, dapat %d", st)
	}
	if st, _ := w.pilot2Admin.status(http.MethodGet, histURL, nil); st != http.StatusForbidden && st != http.StatusNotFound {
		return failr("INV-09: tenant lain harus ditolak, dapat %d", st)
	}
	if st, _ := NewClient(w.super.base, false).status(http.MethodGet, histURL, nil); st != http.StatusUnauthorized {
		return failr("INV-09: tanpa token harus 401, dapat %d", st)
	}
	if st, _ := w.pilotAdmin.status(http.MethodGet, "/api/v1/olts/"+main.ID+"/registered-onus/"+losONU.ID+"/history", nil); st != http.StatusNotFound {
		return failr("INV-09: ONU dari OLT lain harus 404, dapat %d", st)
	}

	// ---- INV-08: pemberitahuan perubahan keadaan ONU ----
	// Poll 2 mengubah SIMREG0002 ONLINE -> LOS: tepat SATU event webhook per polling (daftar perubahan, tanpa PK) dan satu notifikasi
	// ONU_STATE_ALERT (WARNING: hanya 1 LOS) untuk ADMIN tenant; tenant lain tidak menerima apa pun.
	ds, err := deliveries()
	if err != nil || len(ds) != 1 {
		return failr("INV-08: poll 2 harus membuat tepat 1 pengiriman webhook, dapat %d err=%v", len(ds), err)
	}
	if d := ds[0].Payload; d.Event != "ONU_STATE_CHANGED" || d.Data.OLTID != rot.ID || d.Data.OLTName != "e2e-inv-rot-"+run || d.Data.Changed != 1 || d.Data.Truncated ||
		len(d.Data.Changes) != 1 || d.Data.Changes[0].Serial != "SIMREG0002" || d.Data.Changes[0].From != "ONLINE" || d.Data.Changes[0].To != "LOS" {
		return failr("INV-08: payload webhook salah: %+v", d)
	}
	alertTitle := "1 ONU LOS/dying gasp di e2e-inv-rot-" + run
	countAlert := func(c *Client) (int, string, error) {
		var l struct {
			Data []struct {
				Type     string `json:"type"`
				Severity string `json:"severity"`
				Title    string `json:"title"`
				Body     string `json:"body"`
			} `json:"data"`
		}
		if err := c.do(http.MethodGet, "/api/v1/notifications?page_size=100", nil, &l); err != nil {
			return 0, "", err
		}
		n, sev := 0, ""
		for _, it := range l.Data {
			if it.Type == "ONU_STATE_ALERT" && it.Title == alertTitle {
				n++
				sev = it.Severity
				if !strings.Contains(it.Body, "SIMREG0002") {
					return 0, "", fmt.Errorf("isi notifikasi tak menyebut ONU: %q", it.Body)
				}
			}
		}
		return n, sev, nil
	}
	if n, sev, err := countAlert(w.pilotAdmin); err != nil || n != 1 || sev != "WARNING" {
		return failr("INV-08: ADMIN tenant harus menerima 1 notifikasi ONU_STATE_ALERT WARNING, dapat n=%d sev=%q err=%v", n, sev, err)
	}
	if n, _, err := countAlert(w.pilot2Admin); err != nil || n != 0 {
		return failr("INV-08: notifikasi bocor ke tenant lain (n=%d err=%v)", n, err)
	}
	var prefs struct {
		Data []struct {
			Type  string `json:"type"`
			InApp bool   `json:"in_app"`
			Email bool   `json:"email"`
		} `json:"data"`
	}
	if err := w.pilotAdmin.do(http.MethodGet, "/api/v1/notifications/preferences", nil, &prefs); err != nil {
		return failr("INV-08: GET /notifications/preferences: %v", err)
	}
	seen := map[string]bool{}
	for _, ty := range prefs.Data {
		if ty.Type == "DEVICE_OFFLINE" || ty.Type == "ONU_STATE_ALERT" {
			seen[ty.Type] = true
			if !ty.InApp || ty.Email {
				return failr("INV-08: %s harus in-app aktif & email mati secara bawaan: %+v", ty.Type, ty)
			}
		}
	}
	if len(seen) != 2 {
		return failr("INV-08: jenis notifikasi DEVICE_OFFLINE/ONU_STATE_ALERT tidak terdaftar: %v", seen)
	}
	var sub2 struct {
		ID uint64 `json:"id"`
	}
	if err := w.pilotAdmin.do(http.MethodPost, "/api/v1/webhooks", map[string]any{
		"event_type": "DEVICE_OFFLINE", "name": "e2e-off-" + run, "target_url": "https://203.0.113.9/e2e-offline-hook",
	}, &sub2); err != nil || sub2.ID == 0 {
		return failr("INV-08: langganan DEVICE_OFFLINE harus diterima: %v", err)
	}
	_ = w.pilotAdmin.do(http.MethodDelete, fmt.Sprintf("/api/v1/webhooks/%d", sub2.ID), nil, nil)

	// ---- INV-07: OLT dihapus ----
	if err := w.pilotAdmin.do(http.MethodDelete, "/api/v1/olts/"+main.ID, nil, nil); err != nil {
		return failr("INV-07: hapus OLT: %v", err)
	}
	if st, _ := w.pilotAdmin.status(http.MethodGet, "/api/v1/olts/"+main.ID+"/registered-onus", nil); st != http.StatusNotFound {
		return failr("INV-07: daftar OLT terhapus harus 404, dapat %d", st)
	}
	// OLT berputar masih ada di tenant yang sama dan melaporkan SIMREG0002 (LOS); tautan perangkat berpindah ke sana, bukan ke OLT terhapus.
	if err := w.pilotAdmin.do(http.MethodGet, fmt.Sprintf("/api/v1/devices/%d/olt", devLos), nil, &v); err != nil || !v.Linked || v.ONU == nil || v.ONU.OLTID != rot.ID {
		return failr("INV-07: setelah OLT utama dihapus, serial harus tertaut ke OLT yang masih ada: %+v err=%v", v, err)
	}
	if err := w.pilotAdmin.do(http.MethodDelete, "/api/v1/olts/"+rot.ID, nil, nil); err != nil {
		return failr("INV-07: hapus OLT berputar: %v", err)
	}
	if err := w.pilotAdmin.do(http.MethodGet, fmt.Sprintf("/api/v1/devices/%d/olt", devLos), nil, &v); err != nil || v.Linked || v.ONU != nil || v.TenantHasOLT {
		return failr("INV-07: tanpa OLT tersisa tautan harus lepas dan tenant_has_olt=false: %+v err=%v", v, err)
	}

	return pass("poll mengisi inventaris (5 ONU, keadaan netral) + filter port/keadaan/cari & 3 masukan buruk 400; ringkasan port benar; tautan serial↔perangkat (huruf kecil) tanpa lintas-tenant; diagnosis memakai bukti OLT (LOS/dying gasp kritis, ONLINE-tapi-senyap, tak ditemukan) dan limits berubah; tenant lain 403/404 & tak tertaut, tanpa token 401, VIEWER boleh baca; mode berputar (1 baru/1 berubah/1 hilang); perubahan ONLINE→LOS memicu tepat 1 webhook ONU_STATE_CHANGED (tanpa PK) + 1 notifikasi ONU_STATE_ALERT hanya untuk tenant pemilik, jenis DEVICE_OFFLINE/ONU_STATE_ALERT terdaftar (in-app saja); OLT dihapus melepas tautan")
}
