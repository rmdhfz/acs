package automation

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"acs/internal/domain"
)

const (
	wlanPrefix = "InternetGatewayDevice.LANDevice.1.WLANConfiguration."
	wlanSSID   = wlanPrefix + "*.SSID"
)

// lister = fakeParams + kemampuan DeviceParameterPathLister: meniru LIKE (tiap '*' = apa saja, termasuk titik) sehingga hasilnya SUPERSET
// seperti repository nyata, dan penyaringan ketat domain.FilterParamPattern ikut teruji.
type lister struct {
	*fakeParams
	calls int
}

func (l *lister) CandidatePaths(_ context.Context, deviceID uint64, pattern string, limit int) ([]string, error) {
	l.calls++
	parts := strings.Split(pattern, "*")
	var out []string
	for name := range l.vals[deviceID] {
		rest, ok := name, true
		for i, p := range parts {
			idx := strings.Index(rest, p)
			if idx < 0 || (i == 0 && idx != 0) {
				ok = false
				break
			}
			rest = rest[idx+len(p):]
		}
		if ok && (len(parts) == 1 || rest == "" || strings.HasSuffix(name, parts[len(parts)-1])) {
			out = append(out, name)
		}
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

// wlanRig: perangkat 1 melaporkan SSID instance 1..n (nilai awal "Lama<i>"), ditambah parameter yang TIDAK boleh ikut cocok.
func wlanRig(n int) (*rig, *lister) {
	g := newRig()
	l := &lister{fakeParams: g.params}
	g.svc.params = l
	delete(g.params.vals[1], ssidPath) // newRig menanam SSID instance 1; rig ini mengatur sendiri instance yang ada
	for i := 1; i <= n; i++ {
		g.params.vals[1][fmt.Sprintf("%s%d.SSID", wlanPrefix, i)] = fmt.Sprintf("Lama%d", i)
		g.params.vals[1][fmt.Sprintf("%s%d.BSSID", wlanPrefix, i)] = fmt.Sprintf("AA:BB:CC:00:00:0%d", i)
	}
	g.params.vals[1][wlanPrefix+"Extra.SSID"] = "bukan-instance"                         // segmen bukan angka
	g.params.vals[1][wlanPrefix+"1.PreSharedKey.1.SSID"] = "terlalu-dalam"               // '*' tak boleh melahap beberapa segmen
	g.params.vals[1][wlanPrefix+"1.SSIDAdvertisementEnabled"] = "1"                      // akhiran berbeda
	g.params.vals[1]["InternetGatewayDevice.LANDevice.2.WLANConfiguration.1.SSID"] = "x" // LANDevice lain
	return g, l
}

func setAction(name, value string) []domain.AutomationAction {
	return []domain.AutomationAction{{Type: "set_parameter", Name: name, Value: value}}
}

func TestWildcard_Validasi(t *testing.T) {
	g := newRig()
	ctx := context.Background()
	base := Input{TriggerEvent: domain.ZtpTriggerEventEveryInform, CooldownSeconds: 600, MaxConsecutiveFires: 2}
	try := func(name string, mod func(*Input)) error {
		in := base
		in.Name = name
		mod(&in)
		_, err := g.svc.Create(ctx, adminA, in)
		return err
	}
	if err := try("ok-literal", func(i *Input) { i.Actions = setAction(wlanSSID, "'ACS'") }); err != nil {
		t.Fatalf("wildcard + nilai literal harus sah: %v", err)
	}
	if err := try("ok-ref", func(i *Input) { i.Actions = setAction(wlanSSID, "concat('ACS-', {"+wlanPrefix+"*.BSSID})") }); err != nil {
		t.Fatalf("rujukan berwildcard sepadan harus sah: %v", err)
	}
	bad := map[string]struct {
		mod  func(*Input)
		want string
	}{
		"logical key berwildcard":  {func(i *Input) { i.Actions = setAction("wifi.*.ssid", "'x'") }, "pola name tidak valid"},
		"wildcard di segmen akhir": {func(i *Input) { i.Actions = setAction(wlanPrefix+"*", "'x'") }, "pola name tidak valid"},
		"tiga wildcard": {func(i *Input) {
			i.Actions = setAction("InternetGatewayDevice.A.*.B.*.C.*.D", "'x'")
		}, "pola name tidak valid"},
		"bukan segmen utuh": {func(i *Input) { i.Actions = setAction(wlanPrefix+"1*.SSID", "'x'") }, "pola name tidak valid"},
		"rujukan * tapi target biasa": {func(i *Input) {
			i.Actions = setAction(wlanPrefix+"1.SSID", "{"+wlanPrefix+"*.BSSID}")
		}, "tidak berwildcard"},
		"jumlah * berbeda": {func(i *Input) {
			i.Actions = setAction("InternetGatewayDevice.LANDevice.*.WLANConfiguration.*.SSID", "{"+wlanPrefix+"*.BSSID}")
		}, "tepat 2 segmen"},
		"rujukan * tak sah": {func(i *Input) { i.Actions = setAction(wlanSSID, "{"+wlanPrefix+"1*.BSSID}") }, "tepat 1 segmen"},
		"kondisi berwildcard": {func(i *Input) {
			i.Condition = "{" + wlanPrefix + "*.SSID} == 'x'"
			i.Actions = []domain.AutomationAction{{Type: "reboot"}}
		}, "condition tidak mendukung wildcard"},
		"ManagementServer berwildcard": {func(i *Input) {
			i.Actions = setAction("InternetGatewayDevice.ManagementServer.X.*.Y", "'x'")
		}, "ManagementServer"},
		"rujukan rahasia lewat wildcard": {func(i *Input) {
			i.Actions = setAction(wlanSSID, "{"+wlanPrefix+"*.PreSharedKey}")
		}, "rahasia"},
	}
	for name, c := range bad {
		err := try("bad-"+name, c.mod)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err=%v, mau memuat %q", name, err, c.want)
		}
	}
	// '*' di dalam string berkutip pada nilai BUKAN rujukan: tidak ikut diperiksa/diganti
	if err := try("ok-kutip", func(i *Input) { i.Actions = setAction(wlanSSID, "'{"+wlanPrefix+"*.BSSID}'") }); err != nil {
		t.Errorf("teks berkutip yang menyerupai rujukan tak boleh diperlakukan sebagai rujukan: %v", err)
	}
}

func TestWildcard_AnggaranMemakaiBobotInstanceMaksimum(t *testing.T) {
	g := newRig()
	ctx := context.Background()
	mk := func(name string, acts []domain.AutomationAction) error {
		_, err := g.svc.Create(ctx, adminA, Input{Name: name, TriggerEvent: domain.ZtpTriggerEventEveryInform, CooldownSeconds: 600, MaxConsecutiveFires: 2, Actions: acts})
		return err
	}
	// 3 aksi berwildcard x 8 = 24; menambah 7 aksi biasa = 31 > 30 -> ditolak; 6 aksi biasa = 30 -> sah.
	for i := 0; i < 3; i++ {
		if err := mk(fmt.Sprintf("w%d", i), setAction(wlanSSID, "'x'")); err != nil {
			t.Fatalf("w%d: %v", i, err)
		}
	}
	plain := func(n int) []domain.AutomationAction {
		var a []domain.AutomationAction
		for i := 0; i < n; i++ {
			a = append(a, domain.AutomationAction{Type: "set_parameter", Name: fmt.Sprintf("InternetGatewayDevice.X.P%d", i), Value: "'v'"})
		}
		return a
	}
	if err := mk("plain7", plain(7)); err == nil || !strings.Contains(err.Error(), "dibatasi 30") {
		t.Fatalf("31 harus ditolak: %v", err)
	}
	if err := mk("plain6", plain(6)); err != nil {
		t.Fatalf("30 harus sah: %v", err)
	}
	if err := mk("w3", setAction(wlanSSID, "'x'")); err == nil {
		t.Fatal("aksi berwildcard ke-4 harus melewati anggaran")
	}
}

func evalOnce(t *testing.T, g *rig) {
	t.Helper()
	if err := g.svc.Evaluate(context.Background(), dev1(), nil); err != nil {
		t.Fatalf("evaluate: %v", err)
	}
}

func TestWildcard_EvaluateMenerapkanKeSetiapInstanceSaja(t *testing.T) {
	g, _ := wlanRig(3)
	g.mk(t, func(i *Input) { i.Name = "wlan"; i.Condition = ""; i.Actions = setAction(wlanSSID, "'ACS'") })
	evalOnce(t, g)
	if len(g.tasks.sets) != 1 {
		t.Fatalf("harus tepat satu task SetParameterValues: %+v", g.tasks.sets)
	}
	want := map[string]string{wlanPrefix + "1.SSID": "ACS", wlanPrefix + "2.SSID": "ACS", wlanPrefix + "3.SSID": "ACS"}
	got := g.tasks.sets[0]
	if len(got) != len(want) {
		t.Fatalf("hanya instance 1..3 yang boleh diatur (bukan Extra/PreSharedKey/LANDevice.2): %+v", got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, mau %q", k, got[k], v)
		}
	}
	if n := len(g.runs.rows); n != 1 || !g.runs.rows[0].Fired || !strings.Contains(g.runs.rows[0].Summary, "WLANConfiguration.2.SSID=ACS") {
		t.Errorf("riwayat harus mencatat path konkret: %+v", g.runs.rows)
	}
}

func TestWildcard_InstanceYangSudahSesuaiTidakDikerjakanUlang(t *testing.T) {
	g, _ := wlanRig(3)
	g.params.vals[1][wlanPrefix+"2.SSID"] = "ACS"
	g.mk(t, func(i *Input) { i.Name = "wlan"; i.Condition = ""; i.Actions = setAction(wlanSSID, "'ACS'") })
	evalOnce(t, g)
	got := g.tasks.sets[0]
	if len(got) != 2 || got[wlanPrefix+"1.SSID"] != "ACS" || got[wlanPrefix+"3.SSID"] != "ACS" {
		t.Fatalf("instance 2 sudah sesuai, hanya 1 dan 3 yang diatur: %+v", got)
	}
	// semua sesuai -> tidak memicu apa pun
	for _, i := range []int{1, 3} {
		g.params.vals[1][fmt.Sprintf("%s%d.SSID", wlanPrefix, i)] = "ACS"
	}
	g.clock = g.clock.Add(2 * 3600e9)
	before := len(g.tasks.sets)
	evalOnce(t, g)
	if len(g.tasks.sets) != before {
		t.Errorf("semua instance sudah sesuai tidak boleh memicu: %+v", g.tasks.sets)
	}
}

func TestWildcard_NilaiTerikatKeInstanceYangSama(t *testing.T) {
	g, _ := wlanRig(3)
	delete(g.params.vals[1], wlanPrefix+"2.BSSID") // instance 2 belum melaporkan BSSID
	g.mk(t, func(i *Input) {
		i.Name = "wlan-bssid"
		i.Condition = ""
		i.Actions = setAction(wlanSSID, "concat('AP-', {"+wlanPrefix+"*.BSSID})")
	})
	evalOnce(t, g)
	got := g.tasks.sets[0]
	if got[wlanPrefix+"1.SSID"] != "AP-AA:BB:CC:00:00:01" || got[wlanPrefix+"3.SSID"] != "AP-AA:BB:CC:00:00:03" {
		t.Errorf("tiap instance harus memakai BSSID miliknya sendiri: %+v", got)
	}
	if _, ok := got[wlanPrefix+"2.SSID"]; ok {
		t.Errorf("instance 2 tanpa BSSID harus dilewati (bukan menulis nilai parsial 'AP-'): %+v", got)
	}
}

func TestWildcard_TanpaInstanceAtauTerlaluBanyakTidakMenerapkanApaPun(t *testing.T) {
	// tidak ada instance
	g, _ := wlanRig(0)
	g.mk(t, func(i *Input) { i.Name = "wlan"; i.Condition = ""; i.Actions = setAction(wlanSSID, "'ACS'") })
	evalOnce(t, g)
	if len(g.tasks.sets) != 0 || len(g.runs.rows) != 0 {
		t.Errorf("tanpa instance tidak boleh memicu: sets=%d runs=%d", len(g.tasks.sets), len(g.runs.rows))
	}
	// 9 instance (> batas 8): tidak diterapkan SEBAGIAN
	g2, _ := wlanRig(9)
	g2.mk(t, func(i *Input) { i.Name = "wlan"; i.Condition = ""; i.Actions = setAction(wlanSSID, "'ACS'") })
	evalOnce(t, g2)
	if len(g2.tasks.sets) != 0 {
		t.Errorf("9 instance melebihi batas: aksi harus dilewati seluruhnya, bukan sebagian: %+v", g2.tasks.sets)
	}
	// tepat 8 instance masih sah
	g3, _ := wlanRig(8)
	g3.mk(t, func(i *Input) { i.Name = "wlan"; i.Condition = ""; i.Actions = setAction(wlanSSID, "'ACS'") })
	evalOnce(t, g3)
	if len(g3.tasks.sets) != 1 || len(g3.tasks.sets[0]) != 8 {
		t.Errorf("8 instance = batas, harus diterapkan semua: %+v", g3.tasks.sets)
	}
}

func TestWildcard_AksiBiasaTidakMenyentuhPencarianInstance(t *testing.T) {
	g, l := wlanRig(3)
	g.mk(t, func(i *Input) {
		i.Name = "biasa"
		i.Condition = ""
		i.Actions = setAction(wlanPrefix+"1.SSID", "'ACS'")
	})
	valsBefore := g.params.valsCall
	evalOnce(t, g)
	if l.calls != 0 {
		t.Errorf("aturan tanpa wildcard tidak boleh mencari instance: %d panggilan", l.calls)
	}
	if g.params.valsCall-valsBefore != 1 {
		t.Errorf("aturan tanpa wildcard tetap satu ValuesFor (jalur panas tak berubah): %d", g.params.valsCall-valsBefore)
	}
}

func TestWildcard_PenyimpananTanpaKemampuanDilewatiBukanPanik(t *testing.T) {
	g := newRig() // fakeParams polos: tidak mengimplementasikan DeviceParameterPathLister
	g.mk(t, func(i *Input) { i.Name = "wlan"; i.Condition = ""; i.Actions = setAction(wlanSSID, "'ACS'") })
	evalOnce(t, g)
	if len(g.tasks.sets) != 0 {
		t.Errorf("tanpa kemampuan pencarian instance, aksi dilewati: %+v", g.tasks.sets)
	}
}

func TestWildcard_DryRunMenampilkanInstanceDanAlasanLewati(t *testing.T) {
	g, _ := wlanRig(2)
	res, err := g.svc.Test(context.Background(), adminA, TestInput{Actions: setAction(wlanSSID, "'ACS'")}, 1)
	if err != nil || !res.OK || res.Error != "" {
		t.Fatalf("test: %v %+v", err, res)
	}
	if len(res.Steps) != 2 || !res.WouldFire {
		t.Fatalf("dua langkah konkret + akan memicu: %+v", res.Steps)
	}
	for i, st := range res.Steps {
		if st.Path != fmt.Sprintf("%s%d.SSID", wlanPrefix, i+1) || !st.Change || st.Value == nil || *st.Value != "ACS" {
			t.Errorf("langkah %d = %+v", i, st)
		}
	}

	g9, _ := wlanRig(9)
	res, err = g9.svc.Test(context.Background(), adminA, TestInput{Actions: setAction(wlanSSID, "'ACS'")}, 1)
	if err != nil || len(res.Steps) != 1 || res.WouldFire || !strings.Contains(res.Steps[0].Skip, "melebihi batas") {
		t.Fatalf("terlalu banyak: alasan lewati harus tampil, tidak memicu: %+v err=%v", res, err)
	}
	// dry-run tidak mengantre apa pun
	if len(g9.tasks.sets) != 0 {
		t.Error("dry-run tidak boleh mengantre task")
	}
}

func TestRewriteRefs_HanyaDiLuarKutip(t *testing.T) {
	got := bindWildcards(`concat('{A.*.B}', {A.*.B}, "x{C.*}", {@serial_number})`, []string{"4"})
	want := `concat('{A.*.B}', {A.4.B}, "x{C.*}", {@serial_number})`
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
	if got := bindWildcards(`{A.*.B.*.C}`, []string{"1", "2"}); got != `{A.1.B.2.C}` {
		t.Errorf("dua wildcard: %s", got)
	}
	if got := bindWildcards(`{A.*.B`, []string{"1"}); got != `{A.*.B` {
		t.Errorf("kurung tak tertutup dibiarkan untuk pengurai: %s", got)
	}
	if got := bindWildcards(`'a\'{A.*.B}'`, []string{"1"}); got != `'a\'{A.*.B}'` {
		t.Errorf("escape kutip: %s", got)
	}
	if refs := wildcardRefs(`{A.*.B} + {@serial_number} + {C.D} + '{E.*.F}'`); len(refs) != 1 || refs[0] != "A.*.B" {
		t.Errorf("wildcardRefs = %v", refs)
	}
}
