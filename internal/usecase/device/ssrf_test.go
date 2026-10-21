package device

import (
	"net"
	"testing"
)

func TestSafeInformConnectionRequestURL(t *testing.T) {
	cases := []struct {
		name     string
		rawURL   string
		remoteIP string
		wantOK   bool
	}{
		{"host cocok RemoteIP", "http://203.0.113.7:7547/cr", "203.0.113.7", true},
		{"host cocok RemoteIP privat (LAN enterprise)", "http://192.168.1.20:7547/x", "192.168.1.20", true},
		{"https juga boleh", "https://203.0.113.7/cr", "203.0.113.7", true},
		{"host != RemoteIP (spoof / SSRF)", "http://10.0.0.5:7547/cr", "203.0.113.7", false},
		{"metadata cloud", "http://169.254.169.254/latest/meta-data/", "203.0.113.7", false},
		{"loopback service internal", "http://127.0.0.1:9000/", "203.0.113.7", false},
		{"nama DNS (rawan rebinding)", "http://cpe.attacker.test/cr", "203.0.113.7", false},
		{"skema non-http", "ftp://203.0.113.7/cr", "203.0.113.7", false},
		{"URL rusak", "://:::", "203.0.113.7", false},
		{"kosong", "", "203.0.113.7", false},
		{"RemoteIP kosong", "http://203.0.113.7/cr", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := safeInformConnectionRequestURL(c.rawURL, c.remoteIP, nil)
			if ok != c.wantOK {
				t.Fatalf("safeInformConnectionRequestURL(%q, %q) ok=%v, ingin %v", c.rawURL, c.remoteIP, ok, c.wantOK)
			}
			if ok && got != c.rawURL {
				t.Fatalf("URL yang dikembalikan = %q, ingin %q", got, c.rawURL)
			}
		})
	}
}

// Pengecualian terbatas (ACS_CR_TRUSTED_CIDRS): ACS di balik NAT/Docker sehingga RemoteIP bukan alamat CPE.
func TestSafeInformConnectionRequestURL_RentangTepercaya(t *testing.T) {
	mgmt := []*net.IPNet{mustCIDR(t, "10.0.0.0/8")}
	open := []*net.IPNet{mustCIDR(t, "0.0.0.0/0")} // salah konfigurasi ekstrem: semua dianggap tepercaya
	const docker = "172.21.0.1"                    // gateway NAT Docker: IP sumber yang terlihat ACS

	cases := []struct {
		name    string
		rawURL  string
		trusted []*net.IPNet
		wantOK  bool
	}{
		{"host di rentang tepercaya walau beda dari IP sumber", "http://10.0.123.11:58000/x", mgmt, true},
		{"host di luar rentang tepercaya tetap ditolak", "http://203.0.113.7:58000/x", mgmt, false},
		{"tanpa rentang (default) tetap ketat", "http://10.0.123.11:58000/x", nil, false},
		{"nama DNS ditolak walau rentang ada", "http://cpe.internal:58000/x", mgmt, false},
		{"metadata cloud ditolak walau rentang membuka semuanya", "http://169.254.169.254/latest/meta-data", open, false},
		{"loopback ditolak walau rentang membuka semuanya", "http://127.0.0.1:58000/x", open, false},
		{"0.0.0.0 (WAN belum ber-IP) ditolak", "http://0.0.0.0:58000/x", open, false},
		{"skema non-http ditolak", "ftp://10.0.123.11/x", mgmt, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, ok := safeInformConnectionRequestURL(c.rawURL, docker, c.trusted)
			if ok != c.wantOK {
				t.Fatalf("safeInformConnectionRequestURL(%q, trusted=%v) ok=%v, ingin %v", c.rawURL, c.trusted, ok, c.wantOK)
			}
		})
	}
}

func mustCIDR(t *testing.T, s string) *net.IPNet {
	t.Helper()
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		t.Fatal(err)
	}
	return n
}
