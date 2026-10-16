package device

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"net"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestBuildUDPConnectionRequest_FormatDanSignature(t *testing.T) {
	now := time.Unix(1760000000, 0)
	pkt := string(BuildUDPConnectionRequest("203.0.113.9:4000", "acs", "rahasia", now, "ID1", "CN1"))
	if !strings.HasPrefix(pkt, "GET http://203.0.113.9:4000?") || !strings.HasSuffix(pkt, " HTTP/1.1\r\nHost: 203.0.113.9:4000\r\n\r\n") {
		t.Fatalf("format pesan salah: %q", pkt)
	}
	rawQ := strings.TrimSuffix(strings.TrimPrefix(strings.SplitN(pkt, " ", 3)[1], "http://203.0.113.9:4000?"), "")
	q, err := url.ParseQuery(rawQ)
	if err != nil {
		t.Fatal(err)
	}
	mac := hmac.New(sha1.New, []byte("rahasia"))
	mac.Write([]byte(q.Get("ts") + q.Get("id") + q.Get("un") + q.Get("cn")))
	if want := strings.ToUpper(hex.EncodeToString(mac.Sum(nil))); q.Get("sig") != want {
		t.Fatalf("sig = %s, want %s", q.Get("sig"), want)
	}
	if q.Get("ts") != "1760000000" || q.Get("un") != "acs" || q.Get("id") != "ID1" || q.Get("cn") != "CN1" {
		t.Fatalf("query salah: %v", q)
	}
}

// Datagram benar-benar sampai ke penerima UDP.
func TestWakeUpViaUDP_TerkirimKePenerima(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Skip("UDP loopback tidak tersedia")
	}
	defer pc.Close()
	got := make(chan string, 1)
	go func() {
		buf := make([]byte, 2048)
		n, _, _ := pc.ReadFrom(buf)
		got <- string(buf[:n])
	}()
	if err := WakeUpViaUDP(pc.LocalAddr().String(), "acs", "pw"); err != nil {
		t.Fatal(err)
	}
	select {
	case m := <-got:
		if !strings.Contains(m, "&sig=") || !strings.Contains(m, "un=acs") {
			t.Fatalf("pesan tak lengkap: %q", m)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("tidak ada datagram diterima")
	}
}

func TestExtractUDPAddress_KeduaRoot(t *testing.T) {
	for _, k := range []string{"InternetGatewayDevice.ManagementServer.UDPConnectionRequestAddress", "Device.ManagementServer.UDPConnectionRequestAddress"} {
		if got := ExtractUDPAddress(map[string]string{k: "1.2.3.4:5"}); got != "1.2.3.4:5" {
			t.Errorf("%s: %q", k, got)
		}
	}
}
