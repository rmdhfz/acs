// Command e2e menjalankan skenario end-to-end terhadap stack ACS yang benar-
// benar hidup (docker-compose): REST API + endpoint CWMP + MariaDB + MinIO.
//
// Simulator CPE (internal/simcpe) dipakai in-process sebagai "device fisik":
// tiap skenario membuka sesi CWMP sungguhnya, ACS memprosesnya, lalu skenario
// meng-assert hasilnya lewat REST API.
//
// Prasyarat: `docker compose up -d` sudah jalan, dan sudah ada satu user
// SUPERADMIN (buat lewat `docker compose exec acsd /app/seed-admin ...`).
//
//	e2e -rest http://localhost:18080 -cwmp http://localhost:17547/cwmp \
//	    -super-user <u> -super-pass <p>
package main

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"acs/internal/simcpe"
)

type World struct {
	super       *Client
	pilotAdmin  *Client
	pilot2Admin *Client
	cwmpURL     string
	crHost      string
	hc          *http.Client

	runID          string
	pilotTenantID  uint64
	pilot2TenantID uint64
	cwmpUser       string
	cwmpPass       string

	vendorIDByKey map[string]uint64 // simcpe profile key -> ref_vendors.id
	taskStatus    map[string]uint64 // code -> id
	taskStatusRev map[uint64]string
	taskTypeRev   map[uint64]string // ref_task_types.id -> code
	eventCodeRev  map[uint64]string // ref_event_codes.id -> code
	rolloutRev    map[uint64]string // ref_firmware_rollout_status.id -> code

	devices map[string]*simcpe.Device // profile key -> device dari S1
	serials map[string]string         // profile key -> serial
}

type result struct {
	name   string
	ok     bool
	detail string
	skip   bool
}

func main() {
	var (
		restURL   = flag.String("rest", "http://localhost:18080", "base URL REST API ACS")
		cwmpURL   = flag.String("cwmp", "http://localhost:17547/cwmp", "URL endpoint CWMP ACS")
		superUser = flag.String("super-user", "", "username SUPERADMIN yang sudah di-seed (wajib)")
		superPass = flag.String("super-pass", "", "password SUPERADMIN (wajib)")
		crHost    = flag.String("cr-host", "", "host Connection Request reachable dari container acsd (default: hostname container ini, mis. saat dijalankan di jaringan compose)")
		only      = flag.String("run", "", "jalankan hanya skenario yang namanya mengandung substring ini")
		jsonOut   = flag.String("json", "", "tulis ringkasan hasil sebagai JSON ke file ini")
	)
	flag.Parse()
	if *superUser == "" || *superPass == "" {
		die("wajib isi -super-user dan -super-pass (SUPERADMIN yang sudah di-seed)")
	}
	if *crHost == "" {
		// ACS mem-validasi host ConnectionRequestURL == RemoteIP Inform
		// (anti-SSRF), jadi laporkan IP kontainer ini, bukan hostname.
		*crHost = primaryIP()
	}

	w := &World{
		cwmpURL: *cwmpURL,
		crHost:  *crHost,
		hc: &http.Client{
			Timeout:   30 * time.Second,
			Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}, //nolint:gosec
		},
		runID:         randHex(4),
		vendorIDByKey: map[string]uint64{},
		devices:       map[string]*simcpe.Device{},
		serials:       map[string]string{},
	}
	w.super = NewClient(*restURL, true)

	ctx := context.Background()
	fmt.Printf("=== ACS end-to-end test  (run %s) ===\n", w.runID)

	if err := setup(ctx, w, *superUser, *superPass); err != nil {
		die("setup gagal: " + err.Error())
	}

	scenarios := []struct {
		name string
		fn   func(context.Context, *World) result
	}{
		{"S1-onboarding-multivendor", scenarioOnboarding},
		{"S2-provisioning-push", scenarioProvisioning},
		{"S3-reboot-rpc", scenarioReboot},
		{"S4-get-parameter-values", scenarioGetParams},
		{"S5-fault-9005-no-retry", scenarioFaultNoRetry},
		{"S6-tenant-isolation", scenarioTenantIsolation},
		{"S7-connection-request", scenarioConnectionRequest},
		{"S8-firmware-rollout", scenarioFirmwareRollout},
		{"S9-preset-drift-heal", scenarioPresetDriftHeal},
	}

	var results []result
	for _, s := range scenarios {
		if *only != "" && !strings.Contains(s.name, *only) {
			continue
		}
		fmt.Printf("\n--- %s ---\n", s.name)
		r := s.fn(ctx, w)
		r.name = s.name
		results = append(results, r)
		switch {
		case r.skip:
			fmt.Printf("  SKIP: %s\n", r.detail)
		case r.ok:
			fmt.Printf("  PASS: %s\n", r.detail)
		default:
			fmt.Printf("  FAIL: %s\n", r.detail)
		}
	}

	pass, fail, skip := 0, 0, 0
	fmt.Printf("\n=================== RINGKASAN ===================\n")
	for _, r := range results {
		tag := "PASS"
		switch {
		case r.skip:
			tag = "SKIP"
			skip++
		case r.ok:
			pass++
		default:
			tag = "FAIL"
			fail++
		}
		fmt.Printf("  [%s] %-28s %s\n", tag, r.name, r.detail)
	}
	fmt.Printf("------------------------------------------------\n")
	fmt.Printf("  %d PASS  /  %d FAIL  /  %d SKIP  (dari %d skenario)\n", pass, fail, skip, len(results))

	if *jsonOut != "" {
		writeJSON(*jsonOut, map[string]any{
			"run_id": w.runID, "pass": pass, "fail": fail, "skip": skip,
			"results": results, "at": time.Now().Format(time.RFC3339),
		})
	}

	if fail > 0 {
		os.Exit(1)
	}
}

// --- setup ---

func setup(ctx context.Context, w *World, superUser, superPass string) error {
	_ = ctx
	if err := w.super.Login(superUser, superPass); err != nil {
		return fmt.Errorf("login superadmin: %w", err)
	}
	fmt.Println("  login superadmin: OK")

	// Ref maps.
	ts, err := w.super.refMap("ref_task_status")
	if err != nil {
		return fmt.Errorf("refMap ref_task_status: %w", err)
	}
	w.taskStatus = ts
	w.taskStatusRev = map[uint64]string{}
	for k, v := range ts {
		w.taskStatusRev[v] = k
	}
	ec, err := w.super.refMap("ref_event_codes")
	if err != nil {
		return fmt.Errorf("refMap ref_event_codes: %w", err)
	}
	w.eventCodeRev = map[uint64]string{}
	for code, id := range ec {
		w.eventCodeRev[id] = code
	}
	tt, err := w.super.refMap("ref_task_types")
	if err != nil {
		return fmt.Errorf("refMap ref_task_types: %w", err)
	}
	w.taskTypeRev = map[uint64]string{}
	for code, id := range tt {
		w.taskTypeRev[id] = code
	}
	rs, err := w.super.refMap("ref_firmware_rollout_status")
	if err != nil {
		return fmt.Errorf("refMap ref_firmware_rollout_status: %w", err)
	}
	w.rolloutRev = map[uint64]string{}
	for code, id := range rs {
		w.rolloutRev[id] = code
	}

	// Vendors -> id per profil.
	if err := resolveVendors(w); err != nil {
		return err
	}

	// Daftarkan OUI simulator ke tiap vendor (idempoten — abaikan konflik).
	for key, vid := range w.vendorIDByKey {
		oui := strings.ToUpper(simcpe.Profiles[key].OUI)
		err := w.super.do(http.MethodPost, fmt.Sprintf("/api/v1/vendors/%d/ouis", vid),
			map[string]any{"oui": oui, "notes": "e2e simulator " + w.runID}, nil)
		if err != nil && !isConflict(err) {
			fmt.Printf("  WARN: daftar OUI %s ke vendor %d: %v\n", oui, vid, err)
		}
	}
	fmt.Printf("  vendor OUI simulator terdaftar: %d vendor\n", len(w.vendorIDByKey))

	// Tenant pilot + kredensial CWMP.
	w.cwmpUser = "pilot-" + w.runID
	w.cwmpPass = "cwmp-secret-" + randHex(12) // > 16 char
	tid, err := createTenant(w.super, "PIL"+strings.ToUpper(w.runID), "Pilot E2E "+w.runID, w.cwmpUser, w.cwmpPass)
	if err != nil {
		return fmt.Errorf("buat tenant pilot: %w", err)
	}
	w.pilotTenantID = tid

	// Tenant kedua utk uji isolasi.
	t2id, err := createTenant(w.super, "PI2"+strings.ToUpper(w.runID), "Pilot2 E2E "+w.runID,
		"pilot2-"+w.runID, "cwmp-secret-"+randHex(12))
	if err != nil {
		return fmt.Errorf("buat tenant pilot2: %w", err)
	}
	w.pilot2TenantID = t2id

	// Admin user per tenant.
	w.pilotAdmin, err = createAdmin(w.super, NewClient(w.super.base, true), tid, "adm-a-"+w.runID)
	if err != nil {
		return fmt.Errorf("buat admin pilot: %w", err)
	}
	w.pilot2Admin, err = createAdmin(w.super, NewClient(w.super.base, true), t2id, "adm-b-"+w.runID)
	if err != nil {
		return fmt.Errorf("buat admin pilot2: %w", err)
	}
	fmt.Printf("  tenant pilot=%d pilot2=%d, admin per tenant: OK\n", tid, t2id)
	return nil
}

func resolveVendors(w *World) error {
	raw := json.RawMessage{}
	if err := w.super.do(http.MethodGet, "/api/v1/vendors", nil, &raw); err != nil {
		return fmt.Errorf("GET /vendors: %w", err)
	}
	type vendorRow struct {
		ID   uint64 `json:"id"`
		Code string `json:"code"`
		Name string `json:"name"`
	}
	var rows []vendorRow
	var env listEnvelope[vendorRow]
	if json.Unmarshal(raw, &env) == nil && env.Data != nil {
		rows = env.Data
	} else {
		_ = json.Unmarshal(raw, &rows)
	}
	if len(rows) == 0 {
		return fmt.Errorf("GET /vendors kosong")
	}
	match := func(needle string) uint64 {
		needle = strings.ToLower(needle)
		for _, r := range rows {
			if strings.Contains(strings.ToLower(r.Code), needle) || strings.Contains(strings.ToLower(r.Name), needle) {
				return r.ID
			}
		}
		return 0
	}
	for _, key := range simcpe.ProfileKeys {
		needle := key
		if key == "cdata" {
			needle = "data" // "C-Data" / "Cdata"
		}
		id := match(needle)
		if id == 0 {
			return fmt.Errorf("vendor untuk profil %q tidak ada di katalog (GET /vendors) — jalankan migrasi 0004", key)
		}
		w.vendorIDByKey[key] = id
	}
	return nil
}

func createTenant(c *Client, code, name, cwmpUser, cwmpPass string) (uint64, error) {
	var resp struct {
		ID uint64 `json:"id"`
	}
	err := c.do(http.MethodPost, "/api/v1/tenants", map[string]any{
		"code": code, "name": name,
		"cwmp_inform_username": cwmpUser, "cwmp_inform_password": cwmpPass,
	}, &resp)
	if err != nil {
		return 0, err
	}
	return resp.ID, nil
}

func createAdmin(super, target *Client, tenantID uint64, username string) (*Client, error) {
	pass := "AdmPass-" + randHex(10)
	err := super.do(http.MethodPost, "/api/v1/users", map[string]any{
		"tenant_id": tenantID, "username": username,
		"email": username + "@e2e.local", "password": pass,
		"full_name": "E2E Admin " + username, "role_codes": []string{"ADMIN"},
	}, nil)
	if err != nil {
		return nil, err
	}
	if err := target.Login(username, pass); err != nil {
		return nil, fmt.Errorf("login admin baru %s: %w", username, err)
	}
	return target, nil
}

// --- helper skenario ---

// runCPE menjalankan satu sesi CWMP, dengan retry sekali bila kena 429 (rate
// limiter /cwmp 5 req/s per IP — di produksi tiap CPE punya IP sendiri).
func (w *World) runCPE(ctx context.Context, d *simcpe.Device, o simcpe.SessionOpts) simcpe.SessionReport {
	o.ACSURL, o.Username, o.Password = w.cwmpURL, w.cwmpUser, w.cwmpPass
	time.Sleep(400 * time.Millisecond)
	rep := simcpe.RunSession(ctx, w.hc, d, o)
	if !rep.OK && strings.Contains(rep.ErrMsg, "429") {
		time.Sleep(2 * time.Second)
		rep = simcpe.RunSession(ctx, w.hc, d, o)
	}
	return rep
}

func (w *World) taskStatusCode(id uint64) string {
	if c, ok := w.taskStatusRev[id]; ok {
		return c
	}
	return fmt.Sprintf("id=%d", id)
}

func pass(format string, a ...any) result { return result{ok: true, detail: fmt.Sprintf(format, a...)} }
func failr(format string, a ...any) result {
	return result{ok: false, detail: fmt.Sprintf(format, a...)}
}
func skipr(format string, a ...any) result {
	return result{skip: true, detail: fmt.Sprintf(format, a...)}
}

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func isConflict(err error) bool {
	ae, ok := err.(*apiError)
	return ok && (ae.Status == 409 || strings.Contains(strings.ToLower(ae.Body), "sudah") || strings.Contains(strings.ToLower(ae.Body), "duplicate") || strings.Contains(strings.ToLower(ae.Body), "exist"))
}

func die(msg string) {
	fmt.Fprintln(os.Stderr, "e2e: "+msg)
	os.Exit(2)
}

func writeJSON(path string, v any) {
	b, _ := json.MarshalIndent(v, "", "  ")
	_ = os.WriteFile(path, b, 0o644)
}

// primaryIP mengembalikan IPv4 non-loopback pertama kontainer ini — dipakai
// sebagai host ConnectionRequestURL supaya lolos validasi host==RemoteIP di ACS.
func primaryIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "host.docker.internal"
	}
	for _, a := range addrs {
		if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if v4 := ipnet.IP.To4(); v4 != nil {
				return v4.String()
			}
		}
	}
	return "host.docker.internal"
}
