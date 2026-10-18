package simcpe

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// UDPConnReqListener meniru penerima UDP Connection Request pada CPE (TR-069 Annex G): datagram berformat pesan
// HTTP GET dengan query ts/id/un/cn/sig, sig = HMAC-SHA1(password, ts+id+un+cn) dalam heksadesimal kapital.
// Datagram yang sah (username cocok, tanda tangan benar, ts masih segar, id belum pernah dipakai) memicu sesi CWMP
// dengan event "6 CONNECTION REQUEST"; sisanya diabaikan DIAM-DIAM (UDP tidak punya balasan) dan dihitung Rejected.
type UDPConnReqListener struct {
	device   *Device
	opts     SessionOpts
	client   *http.Client
	conn     net.PacketConn
	user     string
	pass     string
	onInform func(SessionReport)

	fired, rejected int64
	mu              sync.Mutex
	seen            map[string]struct{}
}

// maxUDPClockSkew = toleransi selisih jam ACS↔CPE untuk ts.
const maxUDPClockSkew = 5 * time.Minute

func NewUDPConnReqListener(d *Device, o SessionOpts, hc *http.Client, username, password string, onInform func(SessionReport)) *UDPConnReqListener {
	return &UDPConnReqListener{device: d, opts: o, client: hc, user: username, pass: password, onInform: onInform, seen: map[string]struct{}{}}
}

// Start membuka soket UDP di port acak dan mengembalikan alamat "host:port" yang harus dilaporkan device sebagai
// ManagementServer.UDPConnectionRequestAddress.
func (l *UDPConnReqListener) Start(advertiseHost string) (string, error) {
	pc, err := net.ListenPacket("udp", ":0")
	if err != nil {
		return "", err
	}
	l.conn = pc
	go l.loop()
	port := pc.LocalAddr().(*net.UDPAddr).Port
	return net.JoinHostPort(advertiseHost, strconv.Itoa(port)), nil
}

func (l *UDPConnReqListener) loop() {
	buf := make([]byte, 4096)
	for {
		n, _, err := l.conn.ReadFrom(buf)
		if err != nil {
			return
		}
		if !l.valid(string(buf[:n])) {
			atomic.AddInt64(&l.rejected, 1)
			continue
		}
		atomic.AddInt64(&l.fired, 1)
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
}

// valid memeriksa satu datagram sesuai Annex G.
func (l *UDPConnReqListener) valid(pkt string) bool {
	line, _, _ := strings.Cut(pkt, "\r\n")
	parts := strings.Fields(line)
	if len(parts) != 3 || parts[0] != "GET" || !strings.HasPrefix(parts[2], "HTTP/") {
		return false
	}
	u, err := url.Parse(parts[1])
	if err != nil {
		return false
	}
	q := u.Query()
	ts, id, un, cn, sig := q.Get("ts"), q.Get("id"), q.Get("un"), q.Get("cn"), q.Get("sig")
	if ts == "" || id == "" || cn == "" || sig == "" || un != l.user {
		return false
	}
	sec, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return false
	}
	if d := time.Since(time.Unix(sec, 0)); d > maxUDPClockSkew || d < -maxUDPClockSkew {
		return false
	}
	mac := hmac.New(sha1.New, []byte(l.pass))
	mac.Write([]byte(ts + id + un + cn))
	want := strings.ToUpper(hex.EncodeToString(mac.Sum(nil)))
	if !hmac.Equal([]byte(want), []byte(strings.ToUpper(sig))) {
		return false
	}
	// replay: id yang sama tidak boleh dipakai dua kali
	key := fmt.Sprintf("%s|%s", id, cn)
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, dup := l.seen[key]; dup {
		return false
	}
	l.seen[key] = struct{}{}
	return true
}

// Fired = datagram sah yang memicu sesi; Rejected = datagram yang dibuang (tanda tangan/user/ts/replay salah).
func (l *UDPConnReqListener) Fired() int64    { return atomic.LoadInt64(&l.fired) }
func (l *UDPConnReqListener) Rejected() int64 { return atomic.LoadInt64(&l.rejected) }

func (l *UDPConnReqListener) Stop() {
	if l.conn != nil {
		_ = l.conn.Close()
	}
}
