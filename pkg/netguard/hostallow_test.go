package netguard

import "testing"

func TestHostAllowlist_NilAndEmptyAreDisabled(t *testing.T) {
	var nilList *HostAllowlist
	if !nilList.Allows("http://apa-saja.example.net/fw.bin") {
		t.Fatal("daftar nil harus mengizinkan semua host (fitur tidak aktif)")
	}
	for _, csv := range []string{"", "  ", " , ,"} {
		a, err := ParseHostAllowlist(csv)
		if err != nil || a != nil {
			t.Fatalf("daftar kosong %q harus (nil, nil): %v %v", csv, a, err)
		}
	}
}

func TestHostAllowlist_Matching(t *testing.T) {
	a, err := ParseHostAllowlist(" files.example.com, *.cdn.example.org , 10.0.0.5, 192.168.50.0/24, 2001:db8::/32, Mixed.Case.NET. ")
	if err != nil || a == nil {
		t.Fatalf("parse: %v %v", a, err)
	}
	for url, want := range map[string]bool{
		// nama host persis (tanpa memedulikan huruf, port, jalur, titik akhir)
		"https://files.example.com/fw/v1.bin": true,
		"http://FILES.example.com:8080/x":     true,
		"https://files.example.com./x":        true,
		"https://mixed.case.net/x":            true,
		"https://files.example.com.evil.io/x": false, // bukan sufiks yang cocok
		"https://evilfiles.example.com/x":     false,
		"https://example.com/x":               false,
		"https://other.example.net/x":         false,
		// wildcard subdomain: subdomain boleh, apex tidak
		"https://a.cdn.example.org/x":       true,
		"https://a.b.cdn.example.org/x":     true,
		"https://cdn.example.org/x":         false,
		"https://xcdn.example.org/x":        false,
		"https://cdn.example.org.evil.io/x": false,
		// IP literal & CIDR
		"http://10.0.0.5/fw.bin":           true,
		"http://10.0.0.6/fw.bin":           false,
		"http://192.168.50.77:9000/fw.bin": true,
		"http://192.168.51.1/fw.bin":       false,
		"http://[2001:db8::1]/fw.bin":      true,
		"http://[2001:db9::1]/fw.bin":      false,
		// bentuk IP alternatif tidak boleh menyelinap lewat pencocokan nama host
		"http://2130706433/fw.bin": false,
		"http://0x7f000001/fw.bin": false,
		// URL rusak / tanpa host
		"":             false,
		"/fw.bin":      false,
		"http://":      false,
		"http://%zz/x": false,
		// userinfo tidak boleh mengubah host yang dinilai
		"http://files.example.com@evil.io/x": false,
		"http://evil.io@files.example.com/x": true,
	} {
		if got := a.Allows(url); got != want {
			t.Errorf("Allows(%q) = %v, harapan %v", url, got, want)
		}
	}
}

func TestParseHostAllowlist_RejectsInvalidEntries(t *testing.T) {
	for _, csv := range []string{
		"https://files.example.com",     // skema
		"files.example.com:8080",        // port
		"files.example.com/path",        // jalur (bukan CIDR)
		"*.",                            // wildcard kosong
		"*",                             // wildcard telanjang
		"*.*.example.com",               // wildcard ganda
		"a*.example.com",                // wildcard bukan awalan *.
		"10.0.0.0/33",                   // CIDR rusak
		".example.com",                  // diawali titik
		"files example.com",             // spasi di dalam
		"ok.example.com, https://x.org", // satu entri buruk menggagalkan seluruh daftar
	} {
		if a, err := ParseHostAllowlist(csv); err == nil {
			t.Errorf("%q harus ditolak, malah diterima: %+v", csv, a)
		}
	}
}
