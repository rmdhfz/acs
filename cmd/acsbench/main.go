// Command acsbench membandingkan ACS kita dengan GenieACS memakai beban IDENTIK dari simulator CPE (internal/simcpe):
// fase registrasi (Inform 0 BOOTSTRAP), fase periodik (Inform 2 PERIODIC berulang), dan fase tugas (antre
// GetParameterValues lewat API ACS lalu CPE menjawabnya dalam sesi). Mengukur throughput, latensi p50/p95/p99, galat, dan
// pemakaian CPU/memori kontainer (docker stats).
//
// Batas klaim: simulator bukan firmware nyata dan bebannya satu mesin; angka berlaku untuk perangkat keras & konfigurasi
// bawaan yang dipakai saat pengukuran. Lihat docs/pengujian/BENCHMARK_GENIEACS.md.
//
//	acsbench -kind ours     -cwmp http://127.0.0.1:28080... -api http://127.0.0.1:28080/api/v1 -devices 500 ...
//	acsbench -kind genieacs -cwmp http://127.0.0.1:37547    -api http://127.0.0.1:37557        -devices 500 ...
package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"acs/internal/simcpe"
)

type phaseResult struct {
	Name       string  `json:"name"`
	Sessions   int     `json:"sessions"`
	OK         int     `json:"ok"`
	Failed     int     `json:"failed"`
	ElapsedSec float64 `json:"elapsed_sec"`
	PerSec     float64 `json:"sessions_per_sec"`
	P50ms      float64 `json:"p50_ms"`
	P95ms      float64 `json:"p95_ms"`
	P99ms      float64 `json:"p99_ms"`
	MaxMS      float64 `json:"max_ms"`
	FirstError string  `json:"first_error,omitempty"`
}

type resource struct {
	Container string  `json:"container"`
	CPUMaxPct float64 `json:"cpu_max_pct"`
	CPUAvgPct float64 `json:"cpu_avg_pct"`
	MemMaxMiB float64 `json:"mem_max_mib"`
	Samples   int     `json:"samples"`
}

type report struct {
	Label     string        `json:"label"`
	Kind      string        `json:"kind"`
	Devices   int           `json:"devices"`
	Conc      int           `json:"concurrency"`
	Vendor    string        `json:"vendor"`
	Phases    []phaseResult `json:"phases"`
	Resources []resource    `json:"resources"`
	At        string        `json:"at"`
	// ProvisionApplied = "diterapkan/total": berapa perangkat yang nilai provisioning-nya benar-benar terlihat di sisi CPE (hanya -provision).
	ProvisionApplied string `json:"provision_applied,omitempty"`
}

// timed = laporan sesi simulator + durasi yang diukur sendiri (presisi sub-milidetik).
type timed struct {
	simcpe.SessionReport
	Dur time.Duration
}

type outcome struct {
	d   time.Duration
	ok  bool
	err string
}

func pct(sorted []time.Duration, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	i := int(float64(len(sorted)-1) * p)
	return float64(sorted[i].Microseconds()) / 1000
}

func summarize(name string, outs []outcome, elapsed time.Duration) phaseResult {
	r := phaseResult{Name: name, Sessions: len(outs), ElapsedSec: elapsed.Seconds()}
	var lat []time.Duration
	for _, o := range outs {
		if o.ok {
			r.OK++
			lat = append(lat, o.d)
		} else {
			r.Failed++
			if r.FirstError == "" {
				r.FirstError = o.err
			}
		}
	}
	sort.Slice(lat, func(i, j int) bool { return lat[i] < lat[j] })
	r.P50ms, r.P95ms, r.P99ms = pct(lat, 0.50), pct(lat, 0.95), pct(lat, 0.99)
	if len(lat) > 0 {
		r.MaxMS = float64(lat[len(lat)-1].Microseconds()) / 1000
	}
	if elapsed > 0 {
		r.PerSec = float64(r.OK) / elapsed.Seconds()
	}
	return r
}

// parallel menjalankan fn untuk tiap indeks 0..n-1 dengan paling banyak conc worker (w = nomor worker, 0..conc-1).
func parallel(n, conc int, fn func(w, i int) outcome) ([]outcome, time.Duration) {
	outs := make([]outcome, n)
	jobs := make(chan int)
	var wg sync.WaitGroup
	start := time.Now()
	for w := 0; w < conc; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := range jobs {
				outs[i] = fn(w, i)
			}
		}(w)
	}
	for i := 0; i < n; i++ {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	return outs, time.Since(start)
}

// ---- API per target ----

type apiClient struct {
	kind  string
	base  string
	token string
	hc    *http.Client
}

func (a *apiClient) do(method, path string, body any, out any) (int, error) {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, a.base+path, rd)
	if err != nil {
		return 0, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if a.token != "" {
		req.Header.Set("Authorization", "Bearer "+a.token)
	}
	resp, err := a.hc.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return resp.StatusCode, fmt.Errorf("HTTP %d %s", resp.StatusCode, strings.TrimSpace(string(raw[:min(len(raw), 200)])))
	}
	if out != nil && len(raw) > 0 {
		return resp.StatusCode, json.Unmarshal(raw, out)
	}
	return resp.StatusCode, nil
}

// setupOurs: login SUPERADMIN, buat tenant benchmark dengan kredensial CWMP; mengembalikan kredensial Inform.
func setupOurs(a *apiClient, user, pass string, run string) (cwmpUser, cwmpPass string, tenantID uint64, err error) {
	var lr struct {
		AccessToken string `json:"access_token"`
	}
	if _, err = a.do(http.MethodPost, "/auth/login", map[string]any{"username": user, "password": pass}, &lr); err != nil || lr.AccessToken == "" {
		return "", "", 0, fmt.Errorf("login: %v", err)
	}
	a.token = lr.AccessToken
	cwmpUser, cwmpPass = "bench-"+run, "BenchPass-"+run+"-x"
	var tr struct {
		ID uint64 `json:"id"`
	}
	_, err = a.do(http.MethodPost, "/tenants", map[string]any{
		"code": "bench-" + run, "name": "Bench " + run, "cwmp_inform_username": cwmpUser, "cwmp_inform_password": cwmpPass,
	}, &tr)
	return cwmpUser, cwmpPass, tr.ID, err
}

// intervalParam = parameter yang dipaksa oleh pekerjaan provisioning benchmark (ikut dilaporkan pada Inform).
func intervalParam(prof simcpe.Profile) string {
	return prof.RootPrefix() + "ManagementServer.PeriodicInformInterval"
}

const provisionInterval = "600"

// provisionOurs memasang pekerjaan setara di ACS kita: satu aturan otomasi yang memaksa interval Inform pada SETIAP Inform
// (konvergen = diam setelah diterapkan) + n pengecoh yang kondisinya dievaluasi tiap Inform tetapi tidak pernah benar.
func provisionOurs(a *apiClient, tenantID uint64, prof simcpe.Profile, decoys int) error {
	root := prof.RootPrefix()
	rules := []map[string]any{{
		"tenant_id": tenantID, "name": "bench-enforce-interval", "trigger_event": "EVERY_INFORM", "condition": "",
		"actions":          []map[string]any{{"type": "set_parameter", "name": intervalParam(prof), "value": "'" + provisionInterval + "'"}},
		"cooldown_seconds": 300,
	}}
	for i := 0; i < decoys; i++ {
		rules = append(rules, map[string]any{
			"tenant_id": tenantID, "name": fmt.Sprintf("bench-decoy-%d", i), "trigger_event": "EVERY_INFORM",
			"condition": fmt.Sprintf("{%sDeviceInfo.SoftwareVersion} == 'tidak-ada-%d' and {@oui} == 'x'", root, i),
			"actions":   []map[string]any{{"type": "reboot"}}, "cooldown_seconds": 300,
		})
	}
	for _, r := range rules {
		if _, err := a.do(http.MethodPost, "/automation-rules", r, nil); err != nil {
			return fmt.Errorf("aturan %v: %w", r["name"], err)
		}
	}
	return nil
}

// putText mengirim badan teks mentah (skrip provision GenieACS).
func (a *apiClient) putText(path, body string) error {
	req, err := http.NewRequest(http.MethodPut, a.base+path, strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "text/plain")
	resp, err := a.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d %s", resp.StatusCode, strings.TrimSpace(string(raw[:min(len(raw), 200)])))
	}
	return nil
}

// provisionGenie memasang pekerjaan setara di GenieACS 1.2 lewat NBI: provision (JavaScript) yang memaksa interval Inform di setiap
// sesi + n preset pengecoh yang preconditionnya tak pernah cocok (dievaluasi tiap sesi).
func provisionGenie(a *apiClient, prof simcpe.Profile, decoys int) error {
	script := fmt.Sprintf("declare(%q, null, {value: %q});\n", intervalParam(prof), provisionInterval)
	if err := a.putText("/provisions/bench-enforce", script); err != nil {
		return fmt.Errorf("provision: %w", err)
	}
	if err := a.putText("/provisions/bench-noop", "// pengecoh: tak pernah dijalankan\n"); err != nil {
		return fmt.Errorf("provision noop: %w", err)
	}
	put := func(name string, preset map[string]any) error {
		b, _ := json.Marshal(preset)
		req, err := http.NewRequest(http.MethodPut, a.base+"/presets/"+name, bytes.NewReader(b))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := a.hc.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			raw, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("HTTP %d %s", resp.StatusCode, strings.TrimSpace(string(raw[:min(len(raw), 200)])))
		}
		return nil
	}
	if err := put("bench-enforce", map[string]any{"weight": 0, "precondition": "", "configurations": []map[string]any{{"type": "provision", "name": "bench-enforce"}}}); err != nil {
		return fmt.Errorf("preset: %w", err)
	}
	for i := 0; i < decoys; i++ {
		if err := put(fmt.Sprintf("bench-decoy-%d", i), map[string]any{
			"weight": 0, "precondition": fmt.Sprintf("DeviceID.ProductClass = \"NEVER-%d\"", i),
			"configurations": []map[string]any{{"type": "provision", "name": "bench-noop"}},
		}); err != nil {
			return fmt.Errorf("preset pengecoh: %w", err)
		}
	}
	return nil
}

// enqueueGPV mengantre GetParameterValues untuk perangkat; mengembalikan error bila gagal.
func (a *apiClient) enqueueGPV(d *simcpe.Device, path string) error {
	switch a.kind {
	case "genieacs":
		id := url.PathEscape(d.Profile.OUI + "-" + d.Profile.ProductClass + "-" + d.SerialNumber)
		_, err := a.do(http.MethodPost, "/devices/"+id+"/tasks", map[string]any{"name": "getParameterValues", "parameterNames": []string{path}}, nil)
		return err
	default:
		var list struct {
			Data []struct {
				ID uint64 `json:"id"`
			} `json:"data"`
		}
		if _, err := a.do(http.MethodGet, "/devices?serial_number="+url.QueryEscape(d.SerialNumber)+"&page_size=1", nil, &list); err != nil {
			return err
		}
		if len(list.Data) == 0 {
			return fmt.Errorf("perangkat %s belum terdaftar", d.SerialNumber)
		}
		_, err := a.do(http.MethodPost, fmt.Sprintf("/devices/%d/tasks/get-parameter-values", list.Data[0].ID), map[string]any{"names": []string{path}}, nil)
		return err
	}
}

// ---- sampling docker stats ----

type sampler struct {
	names []string
	mu    sync.Mutex
	cpu   map[string][]float64
	mem   map[string]float64
	stop  chan struct{}
	done  chan struct{}
}

func startSampler(names []string) *sampler {
	s := &sampler{names: names, cpu: map[string][]float64{}, mem: map[string]float64{}, stop: make(chan struct{}), done: make(chan struct{})}
	go func() {
		defer close(s.done)
		for {
			s.sample()
			select {
			case <-s.stop:
				return
			case <-time.After(2 * time.Second):
			}
		}
	}()
	return s
}

func parseMiB(v string) float64 {
	v = strings.TrimSpace(strings.SplitN(v, "/", 2)[0])
	mult := 1.0
	switch {
	case strings.HasSuffix(v, "GiB"):
		mult, v = 1024, strings.TrimSuffix(v, "GiB")
	case strings.HasSuffix(v, "MiB"):
		v = strings.TrimSuffix(v, "MiB")
	case strings.HasSuffix(v, "KiB"):
		mult, v = 1.0/1024, strings.TrimSuffix(v, "KiB")
	case strings.HasSuffix(v, "B"):
		mult, v = 1.0/1048576, strings.TrimSuffix(v, "B")
	}
	f, _ := strconv.ParseFloat(strings.TrimSpace(v), 64)
	return f * mult
}

func (s *sampler) sample() {
	if len(s.names) == 0 {
		return
	}
	args := append([]string{"stats", "--no-stream", "--format", "{{.Name}}|{{.CPUPerc}}|{{.MemUsage}}"}, s.names...)
	out, err := exec.Command("docker", args...).Output()
	if err != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		p := strings.Split(strings.TrimSpace(line), "|")
		if len(p) != 3 {
			continue
		}
		c, _ := strconv.ParseFloat(strings.TrimSuffix(p[1], "%"), 64)
		s.cpu[p[0]] = append(s.cpu[p[0]], c)
		if m := parseMiB(p[2]); m > s.mem[p[0]] {
			s.mem[p[0]] = m
		}
	}
}

func (s *sampler) finish() []resource {
	close(s.stop)
	<-s.done
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []resource
	for _, n := range s.names {
		xs := s.cpu[n]
		r := resource{Container: n, Samples: len(xs), MemMaxMiB: s.mem[n]}
		var sum float64
		for _, x := range xs {
			sum += x
			if x > r.CPUMaxPct {
				r.CPUMaxPct = x
			}
		}
		if len(xs) > 0 {
			r.CPUAvgPct = sum / float64(len(xs))
		}
		out = append(out, r)
	}
	return out
}

func main() {
	var (
		kind       = flag.String("kind", "ours", "ours|genieacs")
		cwmp       = flag.String("cwmp", "", "URL endpoint CWMP")
		api        = flag.String("api", "", "base URL API (ours: .../api/v1, genieacs: NBI http://host:7557)")
		superUser  = flag.String("super-user", "e2e-super", "[ours] SUPERADMIN")
		superPass  = flag.String("super-pass", "E2eSuperPass!2345", "[ours] password SUPERADMIN")
		user       = flag.String("user", "", "[genieacs] username CWMP (kosong = tanpa auth)")
		pass       = flag.String("pass", "", "[genieacs] password CWMP")
		devices    = flag.Int("devices", 300, "jumlah perangkat simulasi")
		conc       = flag.Int("concurrency", 16, "sesi paralel")
		rounds     = flag.Int("rounds", 2, "putaran Inform periodik per perangkat")
		tasks      = flag.Int("tasks", 100, "jumlah perangkat pada fase tugas (antre GetParameterValues)")
		vendor     = flag.String("vendor", "zte", "profil simulator")
		containers = flag.String("containers", "", "kontainer yang disampel docker stats, pisahkan koma")
		style      = flag.String("style", "prefixed", "gaya SOAP CPE: prefixed (seperti CPE nyata; wajib untuk GenieACS) | go (apa adanya dari simulator)")
		label      = flag.String("label", "", "label hasil")
		out        = flag.String("out", "", "tulis laporan JSON ke berkas ini")
		provision  = flag.Bool("provision", false, "pasang pekerjaan provisioning setara di kedua sistem: paksa PeriodicInformInterval=600 di tiap Inform + pengecoh")
		decoys     = flag.Int("decoys", 3, "jumlah aturan/preset pengecoh yang dievaluasi tiap Inform bila -provision")
	)
	flag.Parse()
	if *cwmp == "" || *api == "" {
		fmt.Fprintln(os.Stderr, "-cwmp dan -api wajib")
		os.Exit(2)
	}
	prof, ok := simcpe.Profiles[*vendor]
	if !ok {
		fmt.Fprintln(os.Stderr, "profil tak dikenal:", *vendor)
		os.Exit(2)
	}
	apiHC := &http.Client{Timeout: 60 * time.Second, Transport: &http.Transport{
		MaxIdleConns: 512, MaxIdleConnsPerHost: 512, TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // benchmark lokal
	}}
	run := strconv.FormatInt(time.Now().UnixNano()%1_000_000_000, 36)
	a := &apiClient{kind: *kind, base: strings.TrimRight(*api, "/"), hc: apiHC}
	cu, cp := *user, *pass
	if *kind == "ours" {
		var err error
		var tenantID uint64
		if cu, cp, tenantID, err = setupOurs(a, *superUser, *superPass, run); err != nil {
			fmt.Fprintln(os.Stderr, "setup ACS kita gagal:", err)
			os.Exit(1)
		}
		if *provision {
			if err := provisionOurs(a, tenantID, prof, *decoys); err != nil {
				fmt.Fprintln(os.Stderr, "pasang aturan otomasi gagal:", err)
				os.Exit(1)
			}
		}
	} else if *provision {
		if err := provisionGenie(a, prof, *decoys); err != nil {
			fmt.Fprintln(os.Stderr, "pasang provision GenieACS gagal:", err)
			os.Exit(1)
		}
	}

	devs := make([]*simcpe.Device, *devices)
	for i := range devs {
		devs[i] = simcpe.NewDevice(prof, fmt.Sprintf("BENCH%s%05d", strings.ToUpper(run), i))
	}
	// Satu koneksi TCP khusus per worker (MaxConnsPerHost=1): satu sesi CWMP selalu berjalan di SATU koneksi, seperti CPE nyata.
	// Penting untuk GenieACS: sesinya disimpan di memori proses worker dan terikat pada koneksi, jadi request satu sesi yang
	// tersebar ke koneksi berbeda ditolak "Invalid session". Koneksi dipakai ulang antar sesi oleh worker yang sama (keep-alive)
	// supaya klien Windows tidak kehabisan port ephemeral (TIME_WAIT) pada ribuan sesi — artefak klien yang menimpa ACS yang
	// diuji belakangan. Kedua ACS diperlakukan identik.
	clients := make([]*http.Client, *conc)
	for w := range clients {
		tr := &http.Transport{MaxConnsPerHost: 1, MaxIdleConns: 1, MaxIdleConnsPerHost: 1, IdleConnTimeout: 2 * time.Minute, TLSClientConfig: &tls.Config{InsecureSkipVerify: true}} //nolint:gosec // benchmark lokal
		var rt http.RoundTripper = tr
		if *style == "prefixed" {
			rt = prefixedTransport{rt: tr}
		}
		clients[w] = &http.Client{Timeout: 60 * time.Second, Transport: rt}
	}
	session := func(w int, d *simcpe.Device, events ...string) timed {
		start := time.Now()
		r := simcpe.RunSession(context.Background(), clients[w], d, simcpe.SessionOpts{ACSURL: *cwmp, Username: cu, Password: cp, Events: events})
		return timed{SessionReport: r, Dur: time.Since(start)}
	}
	toOutcome := func(t timed) outcome { return outcome{d: t.Dur, ok: t.OK, err: t.ErrMsg} }

	var names []string
	for _, n := range strings.Split(*containers, ",") {
		if n = strings.TrimSpace(n); n != "" {
			names = append(names, n)
		}
	}
	smp := startSampler(names)
	rep := report{Label: *label, Kind: *kind, Devices: *devices, Conc: *conc, Vendor: *vendor, At: time.Now().Format(time.RFC3339)}

	// Fase 1: registrasi
	outs, el := parallel(*devices, *conc, func(w, i int) outcome { return toOutcome(session(w, devs[i], "0 BOOTSTRAP", "1 BOOT")) })
	rep.Phases = append(rep.Phases, summarize("registrasi (0 BOOTSTRAP)", outs, el))
	if *provision {
		// Bukti pekerjaan setara benar-benar terjadi: nilai di sisi CPE (bukan sekadar klaim sistem).
		applied := 0
		for _, d := range devs {
			if v, _ := d.Param(intervalParam(prof)); v == provisionInterval {
				applied++
			}
		}
		rep.ProvisionApplied = fmt.Sprintf("%d/%d", applied, len(devs))
	}

	// Fase 2: Inform periodik
	for r := 1; r <= *rounds; r++ {
		outs, el = parallel(*devices, *conc, func(w, i int) outcome { return toOutcome(session(w, devs[i], "2 PERIODIC")) })
		rep.Phases = append(rep.Phases, summarize(fmt.Sprintf("periodik putaran %d (2 PERIODIC)", r), outs, el))
	}

	// Fase 3: tugas — antre GetParameterValues lewat API, lalu CPE menjawabnya dalam sesi
	k := min(*tasks, *devices)
	// Parameter yang TIDAK ikut dilaporkan pada Inform (SSID): GenieACS menganggap nilai dari Inform sudah segar dan tidak perlu
	// GetParameterValues; ACS kita selalu mengirim RPC. Dengan SSID keduanya benar-benar harus bertanya ke CPE.
	gpvPath := prof.RootPrefix() + "LANDevice.1.WLANConfiguration.1.SSID"
	if prof.DataModel == simcpe.TR181 {
		gpvPath = prof.RootPrefix() + "WiFi.SSID.1.SSID"
	}
	outs, el = parallel(k, *conc, func(w, i int) outcome {
		start := time.Now()
		if err := a.enqueueGPV(devs[i], gpvPath); err != nil {
			return outcome{err: "antre tugas: " + err.Error()}
		}
		r := session(w, devs[i], "2 PERIODIC")
		if !r.OK {
			return outcome{err: r.ErrMsg}
		}
		if r.CountRPC("GetParameterValues") == 0 {
			return outcome{err: fmt.Sprintf("CPE tidak menerima GetParameterValues (rpc: %v)", r.RPCsReceived)}
		}
		return outcome{d: time.Since(start), ok: true}
	})
	rep.Phases = append(rep.Phases, summarize("tugas (antre via API + sesi + jawaban RPC)", outs, el))

	rep.Resources = smp.finish()

	fmt.Printf("\n=== %s (%s) — %d perangkat, concurrency %d ===\n", *label, *kind, *devices, *conc)
	if rep.ProvisionApplied != "" {
		fmt.Printf("provisioning diterapkan di CPE: %s\n", rep.ProvisionApplied)
	}
	fmt.Printf("%-44s %6s %5s %8s %8s %8s %8s\n", "fase", "ok/n", "gagal", "sesi/dtk", "p50 ms", "p95 ms", "p99 ms")
	for _, p := range rep.Phases {
		fmt.Printf("%-44s %3d/%-3d %5d %8.1f %8.1f %8.1f %8.1f\n", p.Name, p.OK, p.Sessions, p.Failed, p.PerSec, p.P50ms, p.P95ms, p.P99ms)
		if p.FirstError != "" {
			fmt.Printf("    galat pertama: %s\n", p.FirstError)
		}
	}
	for _, r := range rep.Resources {
		fmt.Printf("  %-34s CPU maks %6.1f%%  rata-rata %6.1f%%  memori maks %7.1f MiB (%d sampel)\n", r.Container, r.CPUMaxPct, r.CPUAvgPct, r.MemMaxMiB, r.Samples)
	}
	if *out != "" {
		b, _ := json.MarshalIndent(rep, "", "  ")
		if err := os.WriteFile(*out, b, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "gagal menulis laporan:", err)
			os.Exit(1)
		}
	}
}
