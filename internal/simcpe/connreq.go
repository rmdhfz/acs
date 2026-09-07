package simcpe

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync/atomic"
	"time"
)

// ConnReqListener meniru endpoint Connection Request pada CPE (TR-069
// ManagementServer.ConnectionRequestURL). Saat ACS melakukan GET/POST ke URL
// ini (TriggerConnectionRequest di internal/usecase/device), CPE membalas 200
// lalu SEGERA membuka sesi CWMP baru dengan event "6 CONNECTION REQUEST".
//
// Auth: TR-069 mensyaratkan HTTP Digest di sini. Simulator menerima request
// apa pun (dengan atau tanpa Authorization) — fokus uji ada di "apakah ACS
// benar men-trigger & CPE merespons dengan Inform", bukan kekuatan Digest.
type ConnReqListener struct {
	device   *Device
	opts     SessionOpts
	client   *http.Client
	srv      *http.Server
	fired    int64
	onInform func(SessionReport)
}

func NewConnReqListener(addr string, d *Device, o SessionOpts, hc *http.Client, onInform func(SessionReport)) *ConnReqListener {
	l := &ConnReqListener{device: d, opts: o, client: hc, onInform: onInform}
	mux := http.NewServeMux()
	mux.HandleFunc("/", l.handle)
	l.srv = &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	return l
}

func (l *ConnReqListener) handle(w http.ResponseWriter, _ *http.Request) {
	atomic.AddInt64(&l.fired, 1)
	w.WriteHeader(http.StatusOK)
	// Buka sesi CWMP baru asinkron — persis perilaku CPE: balas 200 dulu,
	// lalu Inform.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		o := l.opts
		o.Events = []string{"6 CONNECTION REQUEST"}
		rep := RunSession(ctx, l.client, l.device, o)
		if l.onInform != nil {
			l.onInform(rep)
		}
	}()
}

// Start membuka listener pada port yang diminta (0 = acak) dan mengembalikan
// URL yang harus dilaporkan device sebagai ConnectionRequestURL.
func (l *ConnReqListener) Start(advertiseHost string) (string, error) {
	ln, err := net.Listen("tcp", l.srv.Addr)
	if err != nil {
		return "", err
	}
	go func() { _ = l.srv.Serve(ln) }()
	port := ln.Addr().(*net.TCPAddr).Port
	return fmt.Sprintf("http://%s:%d/cr", advertiseHost, port), nil
}

func (l *ConnReqListener) Fired() int64 { return atomic.LoadInt64(&l.fired) }

func (l *ConnReqListener) Stop() { _ = l.srv.Close() }
