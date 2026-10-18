package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"acs/internal/simcpe"
)

// S25 — Connection Request via UDP (TR-069 Annex G) + RPC yang DIINISIASI CPE.
//
// Dua bentuk CPE diuji: (a) hanya mengiklankan alamat UDP tanpa ConnectionRequestURL (umum di balik CGNAT; profil
// "nokia") dan (b) mengiklankan URL TCP yang tidak terjangkau + alamat UDP (ACS jatuh ke fallback UDP; profil
// "cdata"). Penerima UDP simulator memverifikasi tanda tangan HMAC-SHA1 persis seperti CPE nyata: datagram sah memicu
// Inform "6 CONNECTION REQUEST"; kata sandi salah dibuang diam-diam tanpa Inform.
//
// RPC CPE: CPE mengirim GetRPCMethods (ACS wajib menjawab daftar method), Kicked & RequestDownload (opsional, belum
// didukung → Fault 9000), lalu sesi berlanjut normal — task yang sedang menunggu tetap selesai dengan respons aslinya.
func scenarioCPEWake(ctx context.Context, w *World) result {
	var lastDev *simcpe.Device
	var lastID uint64
	for _, c := range []struct {
		key, label string
		closedURL  bool
	}{
		{"nokia", "hanya alamat UDP (tanpa URL TCP)", false},
		{"cdata", "URL TCP tertutup → fallback UDP", true},
	} {
		dev, id, msg := udpWakeCase(ctx, w, c.key, c.closedURL)
		if msg != "" {
			return failr("[%s] %s", c.label, msg)
		}
		lastDev, lastID = dev, id
	}

	// RPC yang diinisiasi CPE; task menunggu harus tetap selesai dengan respons aslinya.
	devPath := fmt.Sprintf("/api/v1/devices/%d", lastID)
	if st, b := w.pilotAdmin.status(http.MethodPost, devPath+"/tasks/get-parameter-values", map[string]any{"names": []string{"wifi.ssid"}}); st >= 400 {
		return failr("antre GetParameterValues: HTTP %d %s", st, b)
	}
	rep := w.runCPE(ctx, lastDev, simcpe.SessionOpts{Events: []string{"2 PERIODIC"}, CPEProbes: true})
	if !rep.OK {
		return failr("sesi dengan probe RPC CPE gagal: %s", rep.ErrMsg)
	}
	for _, m := range []string{"GetRPCMethods", "Inform", "TransferComplete"} {
		if !slices.Contains(rep.ACSMethods, m) {
			return failr("GetRPCMethodsResponse tidak memuat %s (dapat %v)", m, rep.ACSMethods)
		}
	}
	for _, m := range []string{"Kicked", "RequestDownload"} {
		if rep.UnsupportedCode[m] != "9000" {
			return failr("%s dari CPE harus dijawab Fault 9000, dapat %q", m, rep.UnsupportedCode[m])
		}
	}
	if rep.CountRPC("GetParameterValues") < 1 {
		return failr("task GetParameterValues tidak dikirim ke CPE setelah probe (rpc=%v)", rep.RPCsReceived)
	}
	tasks, err := w.pilotAdmin.listDeviceTasks(lastID)
	if err != nil {
		return failr("list task: %v", err)
	}
	done := w.taskStatus["COMPLETED"]
	completed := 0
	for _, t := range tasks {
		if t.TaskStatusID == done {
			completed++
		}
	}
	if completed < 1 {
		return failr("task GetParameterValues tidak COMPLETED setelah probe RPC CPE (task: %+v)", tasks)
	}
	return pass("UDP CR: (a) hanya alamat UDP dan (b) URL TCP tertutup → fallback UDP — HMAC benar memicu Inform '6 CONNECTION REQUEST', HMAC salah dibuang tanpa Inform; RPC CPE: GetRPCMethods dijawab %v, Kicked/RequestDownload → Fault 9000, task menunggu tetap COMPLETED", rep.ACSMethods)
}

// udpWakeCase menjalankan satu bentuk CPE; mengembalikan pesan galat ("" = lolos).
func udpWakeCase(ctx context.Context, w *World, profKey string, closedURL bool) (*simcpe.Device, uint64, string) {
	serial := fmt.Sprintf("E2E-UDP-%s-%s", strings.ToUpper(w.runID), strings.ToUpper(profKey))
	dev := simcpe.NewDevice(simcpe.Profiles[profKey], serial)

	if closedURL {
		// Port TCP tertutup: dibuka lalu ditutup supaya pasti "connection refused" di sisi ACS.
		ln, err := net.Listen("tcp", ":0")
		if err != nil {
			return nil, 0, fmt.Sprintf("alokasi port tertutup: %v", err)
		}
		port := ln.Addr().(*net.TCPAddr).Port
		_ = ln.Close()
		dev.ConnReqURL = fmt.Sprintf("http://%s:%d/cr", w.crHost, port)
	}

	var (
		mu  sync.Mutex
		got []simcpe.SessionReport
	)
	const crUser, crPass = "acs-udp-e2e", "udp-hmac-secret"
	udp := simcpe.NewUDPConnReqListener(dev, simcpe.SessionOpts{
		ACSURL: w.cwmpURL, Username: w.cwmpUser, Password: w.cwmpPass,
	}, w.hc, crUser, crPass, func(r simcpe.SessionReport) {
		mu.Lock()
		got = append(got, r)
		mu.Unlock()
	})
	addr, err := udp.Start(w.crHost)
	if err != nil {
		return nil, 0, fmt.Sprintf("start listener UDP: %v", err)
	}
	defer udp.Stop()
	dev.UDPAddr = addr
	informs := func() int { mu.Lock(); defer mu.Unlock(); return len(got) }

	// Onboard: ACS menangkap CR URL (bila ada) & alamat UDP dari Inform.
	if rep := w.runCPE(ctx, dev, simcpe.SessionOpts{Events: []string{"0 BOOTSTRAP"}}); !rep.OK {
		return nil, 0, "onboard awal: " + rep.ErrMsg
	}
	d, err := w.super.findDeviceBySerial(serial)
	if err != nil {
		return nil, 0, err.Error()
	}
	devPath := fmt.Sprintf("/api/v1/devices/%d", d.ID)
	if st, b := w.pilotAdmin.status(http.MethodPatch, devPath, map[string]any{
		"connection_request_username": crUser, "connection_request_password": crPass,
	}); st >= 400 {
		return nil, 0, fmt.Sprintf("PATCH kredensial CR: HTTP %d %s", st, b)
	}

	// 1. tanda tangan benar → sesi Inform balik
	if st, b := w.pilotAdmin.status(http.MethodPost, devPath+"/connection-request", map[string]any{}); st >= 400 {
		return nil, 0, fmt.Sprintf("connection-request: HTTP %d %s", st, b)
	}
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) && !(udp.Fired() > 0 && informs() > 0) {
		time.Sleep(400 * time.Millisecond)
	}
	if udp.Fired() != 1 {
		return nil, 0, fmt.Sprintf("datagram UDP sah harus diterima tepat 1x, dapat fired=%d rejected=%d", udp.Fired(), udp.Rejected())
	}
	mu.Lock()
	first := got[0]
	mu.Unlock()
	if !first.OK {
		return nil, 0, "sesi Inform balik setelah UDP CR gagal: " + first.ErrMsg
	}
	evs, _ := w.super.listDeviceEvents(d.ID)
	if !w.eventsContain(evs, "6 CONNECTION REQUEST") {
		return nil, 0, "event '6 CONNECTION REQUEST' tidak tercatat setelah UDP CR (dapat: " + w.eventCodes(evs) + ")"
	}

	// 2. kata sandi salah → tanda tangan tidak cocok → datagram dibuang, tanpa Inform baru
	if st, b := w.pilotAdmin.status(http.MethodPatch, devPath, map[string]any{"connection_request_password": "salah-total-xyz"}); st >= 400 {
		return nil, 0, fmt.Sprintf("PATCH password salah: HTTP %d %s", st, b)
	}
	before := informs()
	_, _ = w.pilotAdmin.status(http.MethodPost, devPath+"/connection-request", map[string]any{})
	time.Sleep(2 * time.Second)
	if udp.Rejected() == 0 {
		return nil, 0, "penerima UDP tidak menolak datagram dengan HMAC salah"
	}
	if udp.Fired() != 1 || informs() != before {
		return nil, 0, fmt.Sprintf("HMAC salah tetap memicu Inform (fired=%d)", udp.Fired())
	}
	_, _ = w.pilotAdmin.status(http.MethodPatch, devPath, map[string]any{"connection_request_password": crPass})
	return dev, d.ID, ""
}
