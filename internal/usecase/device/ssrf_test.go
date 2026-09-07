package device

import "testing"

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
			got, ok := safeInformConnectionRequestURL(c.rawURL, c.remoteIP)
			if ok != c.wantOK {
				t.Fatalf("safeInformConnectionRequestURL(%q, %q) ok=%v, ingin %v", c.rawURL, c.remoteIP, ok, c.wantOK)
			}
			if ok && got != c.rawURL {
				t.Fatalf("URL yang dikembalikan = %q, ingin %q", got, c.rawURL)
			}
		})
	}
}
