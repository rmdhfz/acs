package simcpe

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"acs/pkg/digestauth"
)

// ConnReqListener meniru endpoint Connection Request pada CPE (TR-069
// ManagementServer.ConnectionRequestURL). Saat ACS melakukan GET/POST ke URL
// ini (TriggerConnectionRequest di internal/usecase/device), CPE membalas 200
// lalu SEGERA membuka sesi CWMP baru dengan event "6 CONNECTION REQUEST".
//
// Auth: TR-069 mensyaratkan HTTP Digest di sini. Secara default simulator
// menerima request apa pun (tanpa kredensial = tanpa pemeriksaan). Setelah
// RequireDigest(user, pass) dipanggil, listener berperilaku seperti CPE nyata:
// request tanpa/dengan Authorization salah dijawab 401 + tantangan Digest dan
// TIDAK memicu Inform; hanya jawaban Digest yang benar yang dilayani.
type ConnReqListener struct {
	device   *Device
	opts     SessionOpts
	client   *http.Client
	srv      *http.Server
	fired    int64
	rejected int64
	onInform func(SessionReport)

	authUser, authPass string
}

const connReqRealm = "cpesim"

func NewConnReqListener(addr string, d *Device, o SessionOpts, hc *http.Client, onInform func(SessionReport)) *ConnReqListener {
	l := &ConnReqListener{device: d, opts: o, client: hc, onInform: onInform}
	mux := http.NewServeMux()
	mux.HandleFunc("/", l.handle)
	l.srv = &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	return l
}

// RequireDigest mewajibkan HTTP Digest dengan kredensial ini. Panggil sebelum Start.
func (l *ConnReqListener) RequireDigest(username, password string) {
	l.authUser, l.authPass = username, password
}

func (l *ConnReqListener) handle(w http.ResponseWriter, r *http.Request) {
	if l.authUser != "" {
		nonce := l.nonceFor(r)
		if !digestauth.Verify(r.Method, r.Header.Get("Authorization"), l.authUser, l.authPass, connReqRealm, nonce) {
			atomic.AddInt64(&l.rejected, 1)
			w.Header().Set("WWW-Authenticate", digestauth.NewChallengeHeader(connReqRealm, nonce))
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
	}
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

// nonceFor menurunkan nonce dari serial device + alamat klien: stateless dan
// cukup untuk simulator (bukan untuk produksi).
func (l *ConnReqListener) nonceFor(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	sum := sha256.Sum256([]byte(l.device.SerialNumber + "|" + host + "|" + l.authPass))
	return hex.EncodeToString(sum[:16])
}

// Fired = jumlah Connection Request yang DITERIMA (lolos autentikasi bila diwajibkan).
func (l *ConnReqListener) Fired() int64 { return atomic.LoadInt64(&l.fired) }

// Rejected = jumlah request yang dijawab 401 karena autentikasi belum/salah.
func (l *ConnReqListener) Rejected() int64 { return atomic.LoadInt64(&l.rejected) }

func (l *ConnReqListener) Stop() { _ = l.srv.Close() }
