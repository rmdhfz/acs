package main

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	"acs/internal/simcpe"
)

// S34 — aturan otomasi kondisi-aksi (migrations/0040, pkg/vexpr): "jika <ekspresi> maka <aksi>" dievaluasi saat Inform.
//
//	AU-01  dry-run (/automation-rules/test): kondisi benar + rencana set_parameter dihitung dari ekspresi; TIDAK ada yang diantre
//	AU-02  rule nyata: satu sesi Inform -> ACS mengantre SetParameterValues, CPE menerimanya, riwayat mencatat 1 pemicuan
//	AU-03  sesi berikutnya: tidak memicu lagi (cooldown + sudah konvergen); riwayat tetap 1
//	AU-04  validasi: sintaks, rujukan/target rahasia, variabel konteks, aksi, cooldown, trigger -> 400; nama ganda 409
//	AU-05  isolasi tenant: tenant lain 403 (baca/ubah/hapus/reset/uji) dan tidak melihat aturan/riwayat
//	AU-06  nonaktifkan / hapus: tidak memicu lagi, 404 setelah hapus
type automationJSON struct {
	ID                  string `json:"id"`
	TenantID            uint64 `json:"tenant_id"`
	Name                string `json:"name"`
	TriggerEvent        string `json:"trigger_event"`
	Condition           string `json:"condition"`
	CooldownSeconds     uint32 `json:"cooldown_seconds"`
	MaxConsecutiveFires uint8  `json:"max_consecutive_fires"`
	IsActive            bool   `json:"is_active"`
}

type automationRunJSON struct {
	ID           string  `json:"id"`
	RuleID       string  `json:"rule_id"`
	RuleName     string  `json:"rule_name"`
	DeviceSerial string  `json:"device_serial"`
	Fired        bool    `json:"fired"`
	Suspended    bool    `json:"suspended"`
	Summary      string  `json:"summary"`
	Error        *string `json:"error"`
}

func scenarioAutomation(ctx context.Context, w *World) result {
	key := "huawei"
	dev, serial := w.devices[key], w.serials[key]
	if dev == nil {
		return skipr("device %s dari S1 tidak ada", key)
	}
	d, err := w.pilotAdmin.findDeviceBySerial(serial)
	if err != nil {
		return failr("%v", err)
	}
	prof := simcpe.Profiles[key]
	ssidPath := prof.RootPrefix() + "LANDevice.1.WLANConfiguration.1.SSID"
	if prof.RootPrefix() == "Device." {
		ssidPath = "Device.WiFi.SSID.1.SSID"
	}
	want := "AU-" + serial[:12]
	name := "e2e-au-" + w.runID

	var created []string
	defer func() {
		for _, id := range created {
			_ = w.pilotAdmin.do(http.MethodDelete, "/api/v1/automation-rules/"+id, nil, nil)
		}
	}()
	actions := []map[string]any{{"type": "set_parameter", "name": ssidPath, "value": "concat('AU-', substr({@serial_number}, 0, 12))"}}
	condition := "{@serial_number} == '" + serial + "' and {" + prof.RootPrefix() + "DeviceInfo.SoftwareVersion} != ''"
	def := map[string]any{
		"name": name, "trigger_event": "EVERY_INFORM", "condition": condition, "actions": actions,
		"cooldown_seconds": 300, "max_consecutive_fires": 2,
	}
	runs := func(c *Client, extra ...string) ([]automationRunJSON, error) {
		q := url.Values{}
		q.Set("page_size", "100")
		for i := 0; i+1 < len(extra); i += 2 {
			q.Set(extra[i], extra[i+1])
		}
		var env listEnvelope[automationRunJSON]
		err := c.do(http.MethodGet, "/api/v1/automation-runs?"+q.Encode(), nil, &env)
		return env.Data, err
	}
	setRPCs := func(rep simcpe.SessionReport) int { return rep.CountRPC("SetParameterValues") }

	// pastikan awalnya belum sama (Param = nilai di sisi CPE)
	if cur, _ := dev.Param(ssidPath); cur == want {
		return failr("prasyarat: SSID CPE sudah %q sebelum aturan dibuat", want)
	}
	tasksBefore, _ := w.super.listDeviceTasks(d.ID)

	// AU-01 dry-run
	var tr struct {
		OK        bool `json:"ok"`
		Matched   bool `json:"matched"`
		WouldFire bool `json:"would_fire"`
		Steps     []struct {
			Type   string  `json:"type"`
			Path   string  `json:"path"`
			Value  *string `json:"value"`
			Change bool    `json:"change"`
		} `json:"steps"`
		Error string `json:"error"`
	}
	if err := w.pilotAdmin.do(http.MethodPost, "/api/v1/automation-rules/test",
		map[string]any{"condition": condition, "actions": actions, "device_id": d.ID}, &tr); err != nil {
		return failr("AU-01 test: %v", err)
	}
	if !tr.OK || !tr.Matched || !tr.WouldFire || len(tr.Steps) != 1 || tr.Steps[0].Path != ssidPath || tr.Steps[0].Value == nil || *tr.Steps[0].Value != want || !tr.Steps[0].Change {
		return failr("AU-01: hasil dry-run salah: %+v", tr)
	}
	if after, _ := w.super.listDeviceTasks(d.ID); len(after) != len(tasksBefore) {
		return failr("AU-01: dry-run tidak boleh mengantre task (%d -> %d)", len(tasksBefore), len(after))
	}
	var bad struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := w.pilotAdmin.do(http.MethodPost, "/api/v1/automation-rules/test", map[string]any{"condition": "1 +", "actions": actions, "device_id": d.ID}, &bad); err != nil || bad.OK || bad.Error == "" {
		return failr("AU-01: kondisi salah harus hasil OK=false: %+v (%v)", bad, err)
	}

	// AU-02 aturan nyata
	var rule automationJSON
	if err := w.pilotAdmin.do(http.MethodPost, "/api/v1/automation-rules", def, &rule); err != nil {
		return failr("AU-02 POST: %v", err)
	}
	created = append(created, rule.ID)
	if rule.ID == "" || rule.TenantID != w.pilotTenantID || rule.TriggerEvent != "EVERY_INFORM" || !rule.IsActive || rule.CooldownSeconds != 300 {
		return failr("AU-02: definisi salah: %+v", rule)
	}
	// cache aturan di jalur Inform berumur <=5 dtk pada instance lain; satu instance di e2e langsung diinvalidasi, tetap beri jeda
	time.Sleep(200 * time.Millisecond)
	rep := w.runCPE(ctx, dev, simcpe.SessionOpts{Events: []string{"2 PERIODIC"}})
	if !rep.OK {
		return failr("AU-02 sesi CWMP: %s", rep.ErrMsg)
	}
	if setRPCs(rep) == 0 {
		return failr("AU-02: ACS tidak mengirim SetParameterValues pada sesi Inform (rpc: %v)", rep.RPCsReceived)
	}
	if got, _ := dev.Param(ssidPath); got != want {
		return failr("AU-02: SSID di CPE = %q, harusnya %q", got, want)
	}
	time.Sleep(300 * time.Millisecond)
	rs, err := runs(w.pilotAdmin, "rule_uuid", rule.ID)
	if err != nil || len(rs) != 1 || !rs[0].Fired || rs[0].Suspended || rs[0].RuleName != name || rs[0].DeviceSerial != serial || !strings.Contains(rs[0].Summary, "set_parameter") {
		return failr("AU-02: riwayat salah: %+v (%v)", rs, err)
	}
	if strings.Contains(rs[0].Summary, "psk") || rs[0].Error != nil {
		return failr("AU-02: catatan tidak bersih: %+v", rs[0])
	}

	// AU-03
	rep2 := w.runCPE(ctx, dev, simcpe.SessionOpts{Events: []string{"2 PERIODIC"}})
	if !rep2.OK || setRPCs(rep2) != 0 {
		return failr("AU-03: sesi kedua tidak boleh memicu lagi (ok=%v set=%d err=%s)", rep2.OK, setRPCs(rep2), rep2.ErrMsg)
	}
	if rs, _ = runs(w.pilotAdmin, "rule_uuid", rule.ID); len(rs) != 1 {
		return failr("AU-03: riwayat harus tetap 1, dapat %d", len(rs))
	}

	// AU-04 validasi
	post := func(mod func(m map[string]any)) int {
		m := map[string]any{}
		for k, v := range def {
			m[k] = v
		}
		m["name"] = "bad-" + w.runID
		mod(m)
		st, _ := w.pilotAdmin.status(http.MethodPost, "/api/v1/automation-rules", m)
		return st
	}
	bads := []struct {
		why  string
		mod  func(m map[string]any)
		want int
	}{
		{"sintaks salah", func(m map[string]any) { m["condition"] = "1 +" }, 400},
		{"kondisi merujuk rahasia", func(m map[string]any) {
			m["condition"] = "{" + prof.RootPrefix() + "LANDevice.1.WLANConfiguration.1.KeyPassphrase} == 'x'"
		}, 400},
		{"konteks tak dikenal", func(m map[string]any) { m["condition"] = "{@rahasia} == 1" }, 400},
		{"target aksi rahasia", func(m map[string]any) {
			m["actions"] = []map[string]any{{"type": "set_parameter", "name": prof.RootPrefix() + "LANDevice.1.WLANConfiguration.1.KeyPassphrase", "value": "'x'"}}
		}, 400},
		{"aksi tak dikenal", func(m map[string]any) { m["actions"] = []map[string]any{{"type": "exec"}} }, 400},
		{"tanpa aksi", func(m map[string]any) { m["actions"] = []map[string]any{} }, 400},
		{"tag tak ada", func(m map[string]any) { m["actions"] = []map[string]any{{"type": "add_tag", "tag_id": 99999999}} }, 400},
		{"trigger tak dikenal", func(m map[string]any) { m["trigger_event"] = "SELALU" }, 400},
		{"cooldown terlalu pendek", func(m map[string]any) { m["cooldown_seconds"] = 5 }, 400},
		{"field tak dikenal", func(m map[string]any) { m["ekstra"] = 1 }, 400},
		{"nama ganda", func(m map[string]any) { m["name"] = name }, 409},
	}
	for _, b := range bads {
		if st := post(b.mod); st != b.want {
			return failr("AU-04: %s -> HTTP %d, harusnya %d", b.why, st, b.want)
		}
	}

	// AU-05
	paths := []struct{ method, path string }{
		{http.MethodGet, "/api/v1/automation-rules/" + rule.ID},
		{http.MethodDelete, "/api/v1/automation-rules/" + rule.ID},
		{http.MethodPost, "/api/v1/automation-rules/" + rule.ID + "/reset"},
	}
	for _, p := range paths {
		if st, _ := w.pilot2Admin.status(p.method, p.path, nil); st != http.StatusForbidden {
			return failr("AU-05: %s %s lintas tenant HTTP %d, harusnya 403", p.method, p.path, st)
		}
	}
	if st, _ := w.pilot2Admin.status(http.MethodPatch, "/api/v1/automation-rules/"+rule.ID, map[string]any{"name": "hack"}); st != http.StatusForbidden {
		return failr("AU-05: PATCH lintas tenant HTTP %d, harusnya 403", st)
	}
	if st, _ := w.pilot2Admin.status(http.MethodPost, "/api/v1/automation-rules/test", map[string]any{"condition": "1", "actions": actions, "device_id": d.ID}); st != http.StatusForbidden {
		return failr("AU-05: uji pada perangkat tenant lain HTTP %d, harusnya 403", st)
	}
	var list listEnvelope[automationJSON]
	if err := w.pilot2Admin.do(http.MethodGet, "/api/v1/automation-rules?page_size=100", nil, &list); err != nil {
		return failr("AU-05 list: %v", err)
	}
	for _, x := range list.Data {
		if x.ID == rule.ID {
			return failr("AU-05: aturan tenant lain muncul di daftar")
		}
	}
	if other, err := runs(w.pilot2Admin); err != nil {
		return failr("AU-05 runs: %v", err)
	} else {
		for _, r := range other {
			if r.RuleID == rule.ID {
				return failr("AU-05: riwayat tenant lain bocor")
			}
		}
	}

	// AU-06
	var patched automationJSON
	if err := w.pilotAdmin.do(http.MethodPatch, "/api/v1/automation-rules/"+rule.ID, map[string]any{"is_active": false, "priority": 5}, &patched); err != nil || patched.IsActive {
		return failr("AU-06 nonaktifkan: %+v (%v)", patched, err)
	}
	// ubah SSID di CPE lalu sesi: aturan nonaktif tidak boleh memulihkan
	dev.Set(map[string]string{ssidPath: "diubah-manual"})
	if rep3 := w.runCPE(ctx, dev, simcpe.SessionOpts{Events: []string{"2 PERIODIC"}}); !rep3.OK || setRPCs(rep3) != 0 {
		return failr("AU-06: aturan nonaktif tidak boleh memicu (set=%d)", setRPCs(rep3))
	}
	if st, _ := w.pilotAdmin.status(http.MethodDelete, "/api/v1/automation-rules/"+rule.ID, nil); st >= 400 {
		return failr("AU-06: DELETE HTTP %d", st)
	}
	if st, _ := w.pilotAdmin.status(http.MethodGet, "/api/v1/automation-rules/"+rule.ID, nil); st != http.StatusNotFound {
		return failr("AU-06: GET setelah hapus HTTP %d, harusnya 404", st)
	}

	return pass("dry-run menghitung %q tanpa mengantre apa pun; sesi Inform memicu SetParameterValues (CPE menerima, riwayat 1 pemicuan, tanpa rahasia); sesi berikutnya tidak memicu lagi; 11 masukan tak sah 400/409; tenant lain 403 pada baca/ubah/hapus/reset/uji dan tidak melihat aturan/riwayat; nonaktif tidak memicu; hapus -> 404", want)
}
