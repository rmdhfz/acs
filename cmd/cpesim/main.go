// Command cpesim adalah CLI simulator CPE TR-069/CWMP multi-vendor (logika
// inti di internal/simcpe, dipakai bersama cmd/e2e).
//
// Berbeda dari cmd/loadtest (Inform minimal untuk ukur latency), cpesim
// menjalankan siklus sesi CWMP penuh seperti device fisik: Inform dengan
// event code apa pun, menerima & MEMBALAS RPC yang dikirim ACS, data model
// in-memory yang benar-benar berubah, opsi membalas cwmp:Fault, dan listener
// Connection Request.
//
// Contoh:
//
//	cpesim -acs http://localhost:17547/cwmp -user <secret_user> -pass <secret_pass> \
//	       -vendor zte -serial ZTE-PILOT-001 -events "0 BOOTSTRAP" -v
//
//	cpesim -acs ... -user ... -pass ... -fleet -devices 20 -cycles 3 -interval 5s
package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"

	"acs/internal/simcpe"
)

func main() {
	var (
		acs        = flag.String("acs", "http://localhost:17547/cwmp", "URL endpoint CWMP ACS")
		user       = flag.String("user", "", "shared secret Inform CWMP tenant (wajib)")
		pass       = flag.String("pass", "", "shared secret Inform CWMP tenant (wajib)")
		vendor     = flag.String("vendor", "zte", "profil: zte|huawei|fiberhome|nokia|cdata")
		serial     = flag.String("serial", "", "serial number (default: acak per profil)")
		events     = flag.String("events", "2 PERIODIC", "event code Inform, pisahkan koma")
		faultParam = flag.String("fault-param", "", "balas Fault 9005 bila SetParameterValues memuat substring nama parameter ini")
		fleet      = flag.Bool("fleet", false, "mode fleet: banyak device, beberapa siklus")
		devices    = flag.Int("devices", 5, "[fleet] jumlah device (dibagi rata ke-5 profil)")
		cycles     = flag.Int("cycles", 1, "[fleet] siklus Inform per device")
		interval   = flag.Duration("interval", 3*time.Second, "[fleet] jeda antar siklus")
		conc       = flag.Int("concurrency", 4, "[fleet] worker paralel (<=4 utk hormati rate limiter /cwmp 5 req/s)")
		connReq    = flag.Bool("conn-req", false, "aktifkan listener Connection Request (single device)")
		crHost     = flag.String("cr-host", "host.docker.internal", "host Connection Request yang dilaporkan ke ACS")
		crPort     = flag.Int("cr-port", 0, "port listener Connection Request (0 = acak)")
		timeout    = flag.Duration("timeout", 30*time.Second, "timeout per request HTTP")
		insecure   = flag.Bool("insecure", true, "abaikan verifikasi TLS ACS (dev)")
		jsonOut    = flag.Bool("json", false, "keluarkan report JSON")
		verbose    = flag.Bool("v", false, "log tiap langkah sesi")
	)
	flag.Parse()

	if *user == "" || *pass == "" {
		fmt.Fprintln(os.Stderr, "cpesim: wajib isi -user dan -pass (shared secret Inform CWMP tenant pilot)")
		os.Exit(2)
	}

	hc := &http.Client{
		Timeout: *timeout,
		Transport: &http.Transport{
			MaxIdleConns: 100, MaxIdleConnsPerHost: 20,
			TLSClientConfig: &tls.Config{InsecureSkipVerify: *insecure}, //nolint:gosec // dev/e2e
		},
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	base := simcpe.SessionOpts{
		ACSURL: *acs, Username: *user, Password: *pass,
		Events: splitCSV(*events), FaultParamSubstr: *faultParam, Verbose: *verbose,
	}

	if *fleet {
		os.Exit(runFleet(ctx, hc, base, *devices, *cycles, *conc, *interval, *jsonOut))
	}

	prof, err := simcpe.ProfileByKey(*vendor)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cpesim:", err)
		os.Exit(2)
	}
	sn := *serial
	if sn == "" {
		sn = fmt.Sprintf("%s-SIM-%06d", strings.ToUpper(prof.Key), rand.Intn(1_000_000))
	}
	dev := simcpe.NewDevice(prof, sn)

	var crl *simcpe.ConnReqListener
	if *connReq {
		crl = simcpe.NewConnReqListener(fmt.Sprintf(":%d", *crPort), dev, base, hc, func(r simcpe.SessionReport) {
			fmt.Println("  [conn-req -> Inform]", r.String())
		})
		url, err := crl.Start(*crHost)
		if err != nil {
			fmt.Fprintln(os.Stderr, "cpesim: listener Connection Request:", err)
			os.Exit(2)
		}
		dev.ConnReqURL = url
		fmt.Printf("listener Connection Request di %s\n", url)
		defer crl.Stop()
	}

	rep := simcpe.RunSession(ctx, hc, dev, base)
	emit(*jsonOut, []simcpe.SessionReport{rep})

	if *connReq {
		fmt.Println("menunggu Connection Request dari ACS (Ctrl+C untuk berhenti)...")
		<-ctx.Done()
		fmt.Printf("Connection Request diterima: %d kali\n", crl.Fired())
	}
	if !rep.OK {
		os.Exit(1)
	}
}

func runFleet(ctx context.Context, hc *http.Client, base simcpe.SessionOpts, n, cycles, conc int, interval time.Duration, jsonOut bool) int {
	devs := make([]*simcpe.Device, 0, n)
	for i := 0; i < n; i++ {
		p := simcpe.Profiles[simcpe.ProfileKeys[i%len(simcpe.ProfileKeys)]]
		devs = append(devs, simcpe.NewDevice(p, fmt.Sprintf("%s-FLEET-%04d", strings.ToUpper(p.Key), i)))
	}
	if conc < 1 {
		conc = 1
	}

	var (
		mu      sync.Mutex
		reports []simcpe.SessionReport
		okN     int
	)
	for c := 0; c < cycles; c++ {
		ev := []string{"2 PERIODIC"}
		if c == 0 {
			ev = []string{"0 BOOTSTRAP"}
		}
		if !jsonOut {
			fmt.Printf("=== siklus %d/%d (event %v) ===\n", c+1, cycles, ev)
		}
		var wg sync.WaitGroup
		sem := make(chan struct{}, conc)
		for _, d := range devs {
			wg.Add(1)
			sem <- struct{}{}
			go func(d *simcpe.Device) {
				defer wg.Done()
				defer func() { <-sem }()
				o := base
				o.Events = ev
				r := simcpe.RunSession(ctx, hc, d, o)
				mu.Lock()
				reports = append(reports, r)
				if r.OK {
					okN++
				}
				mu.Unlock()
				if !jsonOut {
					fmt.Println("  " + r.String())
				}
			}(d)
		}
		wg.Wait()
		if c < cycles-1 {
			select {
			case <-ctx.Done():
			case <-time.After(interval):
			}
		}
	}
	emit(jsonOut, reports)
	fmt.Printf("\nfleet selesai: %d/%d sesi OK\n", okN, len(reports))
	if okN != len(reports) {
		return 1
	}
	return 0
}

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func emit(jsonOut bool, reports []simcpe.SessionReport) {
	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(reports)
		return
	}
	for _, r := range reports {
		fmt.Println(r.String())
	}
}
