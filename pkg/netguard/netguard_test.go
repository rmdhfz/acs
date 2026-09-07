package netguard

import "testing"

func TestCheckURL(t *testing.T) {
	blocked := []string{
		"http://127.0.0.1/x",
		"http://localhost:8080/api",
		"http://169.254.169.254/latest/meta-data/",
		"http://[::1]:9000/",
		"http://0.0.0.0/",
		"http://224.0.0.1/",
		"ftp://example.com/x",
		"http://",
		"not a url at all ::::",
	}
	for _, u := range blocked {
		if err := CheckURL(u); err == nil {
			t.Errorf("CheckURL(%q) = nil, ingin ditolak", u)
		}
	}

	allowed := []string{
		"http://10.20.30.40:7547/ctrl", // privat — deployment enterprise sah
		"https://192.168.1.1/cwmp",     // privat
		"http://8.8.8.8/x",             // publik literal
		"https://[2001:4860:4860::8888]/x",
	}
	for _, u := range allowed {
		if err := CheckURL(u); err != nil {
			t.Errorf("CheckURL(%q) = %v, ingin lolos", u, err)
		}
	}
}

func TestCheckHostPort(t *testing.T) {
	if err := CheckHostPort("127.0.0.1:3478"); err == nil {
		t.Error("loopback host:port harus ditolak")
	}
	if err := CheckHostPort("203.0.113.5:3478"); err != nil {
		t.Errorf("host:port publik harus lolos: %v", err)
	}
	if err := CheckHostPort(""); err == nil {
		t.Error("host kosong harus ditolak")
	}
}
