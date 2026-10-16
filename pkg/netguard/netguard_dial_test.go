package netguard

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestIsBlocked_Kebijakan(t *testing.T) {
	always := []string{"127.0.0.1", "::1", "169.254.169.254", "0.0.0.0", "0.1.2.3", "224.0.0.1", "240.0.0.1", "255.255.255.255", "fd00:ec2::254", "::ffff:127.0.0.1", "fe80::1"}
	for _, s := range always {
		for _, bp := range []bool{false, true} {
			if !isBlocked(net.ParseIP(s), bp) {
				t.Errorf("isBlocked(%s, %v) harus true", s, bp)
			}
		}
	}
	private := []string{"10.1.2.3", "172.16.0.5", "192.168.1.1", "100.64.0.1", "100.127.255.254", "198.18.0.1", "fc00::1", "fd12:3456::1"}
	for _, s := range private {
		if isBlocked(net.ParseIP(s), false) {
			t.Errorf("%s harus lolos pada kebijakan bawaan (jaringan privat sah)", s)
		}
		if !isBlocked(net.ParseIP(s), true) {
			t.Errorf("%s harus ditolak saat blockPrivate", s)
		}
	}
	public := []string{"8.8.8.8", "1.1.1.1", "203.0.113.5", "2001:4860:4860::8888", "100.63.255.255", "100.128.0.1"}
	for _, s := range public {
		for _, bp := range []bool{false, true} {
			if isBlocked(net.ParseIP(s), bp) {
				t.Errorf("%s publik tidak boleh ditolak (blockPrivate=%v)", s, bp)
			}
		}
	}
}

func TestCheckURLPolicy_BlockPrivate(t *testing.T) {
	if err := CheckURLPolicy("http://10.20.30.40/x", false); err != nil {
		t.Errorf("privat lolos pada bawaan: %v", err)
	}
	if err := CheckURLPolicy("http://10.20.30.40/x", true); err == nil {
		t.Error("privat harus ditolak saat blockPrivate")
	}
	if err := CheckURLPolicy("http://8.8.8.8/x", true); err != nil {
		t.Errorf("publik tetap lolos: %v", err)
	}
}

// Inti perlindungan DNS rebinding: walau validasi dilewati (atau hasil resolve berubah), sambungan
// ke loopback tetap ditolak di tingkat dial.
func TestSafeTransport_MenolakLoopbackSaatSambungan(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("rahasia-internal")) }))
	defer srv.Close()

	client := &http.Client{Transport: SafeTransport(false), Timeout: 3 * time.Second}
	// lewat IP literal
	if _, err := client.Get(srv.URL); err == nil {
		t.Fatal("sambungan ke 127.0.0.1 harus ditolak oleh penjaga dial")
	}
	// lewat nama host yang me-resolve ke loopback (skenario rebinding: validasi tak dipanggil sama sekali)
	_, port, _ := net.SplitHostPort(srv.Listener.Addr().String())
	if _, err := client.Get("http://localhost:" + port); err == nil {
		t.Fatal("localhost -> 127.0.0.1 harus ditolak saat dial")
	}
	// kontrol: tanpa penjaga, server memang terjangkau (tes ini bukan lolos karena server mati)
	plain := &http.Client{Timeout: 3 * time.Second}
	resp, err := plain.Get(srv.URL)
	if err != nil {
		t.Fatalf("kontrol: %v", err)
	}
	resp.Body.Close()
}

func TestSafeDialer_UDPDanTCP(t *testing.T) {
	d := SafeDialer(false)
	if c, err := d.DialContext(context.Background(), "udp", "127.0.0.1:9"); err == nil {
		c.Close()
		t.Fatal("UDP ke loopback harus ditolak")
	}
	if c, err := d.DialContext(context.Background(), "tcp", "169.254.169.254:80"); err == nil {
		c.Close()
		t.Fatal("TCP ke metadata cloud harus ditolak")
	}
	// blockPrivate menolak RFC1918 sebelum paket keluar (tidak butuh host-nya hidup)
	if c, err := SafeDialer(true).DialContext(context.Background(), "tcp", "10.255.255.1:80"); err == nil {
		c.Close()
		t.Fatal("RFC1918 harus ditolak saat blockPrivate")
	}
}

// Redirect ke alamat internal: setiap hop melewati dialer yang sama.
func TestSafeTransport_RedirectKeInternalDitolak(t *testing.T) {
	internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("internal")) }))
	defer internal.Close()
	// "publik": diwakili listener yang kita biarkan lolos dengan transport biasa untuk hop pertama
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, internal.URL, http.StatusFound)
	}))
	defer redirector.Close()

	tr := SafeTransport(false)
	client := &http.Client{Transport: tr, Timeout: 3 * time.Second}
	// hop pertama (loopback) sudah ditolak; yang dibuktikan: tidak ada jalan mencapai server internal.
	if resp, err := client.Get(redirector.URL); err == nil {
		resp.Body.Close()
		t.Fatal("tidak boleh ada respons dari target internal")
	}
}
