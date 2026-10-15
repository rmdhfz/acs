package digestauth

import (
	"strings"
	"testing"
)

// Vektor dari RFC 2617 §3.5 (contoh resmi): hasil harus persis sama.
func TestAuthorize_RFC2617Vector(t *testing.T) {
	c := &Challenge{
		Realm:  "testrealm@host.com",
		Nonce:  "dcd98b7102dd2f0e8b11d0f600bfb0c093",
		Opaque: "5ccc069c403ebaf9f0171e9517f40e41",
		QOP:    "auth,auth-int",
	}
	got, err := c.Authorize("GET", "/dir/index.html", "Mufasa", "Circle Of Life", "0a4f113b", 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`response="6629fae49393a05397450978507c4ef1"`,
		`username="Mufasa"`, `uri="/dir/index.html"`, `qop=auth`, `nc=00000001`, `cnonce="0a4f113b"`,
		`opaque="5ccc069c403ebaf9f0171e9517f40e41"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("header tidak memuat %s\n%s", want, got)
		}
	}
}

func TestParseChallenge(t *testing.T) {
	c, err := ParseChallenge([]string{
		`Basic realm="x"`,
		`Digest realm="IGD, home", nonce="abc\"def", qop="auth", algorithm=SHA-256, opaque="o", stale=true`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if c.Realm != "IGD, home" || c.Nonce != `abc"def` || c.QOP != "auth" || c.Algorithm != "SHA-256" || c.Opaque != "o" || !c.Stale {
		t.Errorf("parse salah: %+v", c)
	}

	// Skema Basic + Digest digabung dalam satu header.
	c, err = ParseChallenge([]string{`Basic realm="a", Digest realm="b", nonce="n"`})
	if err != nil || c.Realm != "b" {
		t.Errorf("tantangan gabungan: %+v %v", c, err)
	}

	if _, err := ParseChallenge([]string{`Basic realm="x"`}); err != ErrNoDigestChallenge {
		t.Errorf("harus ErrNoDigestChallenge, dapat %v", err)
	}
	if _, err := ParseChallenge([]string{`Digest realm="x"`}); err == nil {
		t.Error("Digest tanpa nonce harus ditolak")
	}
	if !HasScheme([]string{`BASIC realm="x"`}, "basic") {
		t.Error("HasScheme harus case-insensitive")
	}
}

// Round-trip semua algoritma × (qop auth | tanpa qop) lewat Verify, plus kasus salah.
func TestAuthorizeVerifyRoundTrip(t *testing.T) {
	for _, algo := range []string{"", "MD5", "MD5-sess", "SHA-256", "SHA-256-sess"} {
		for _, qop := range []string{"auth", ""} {
			c := &Challenge{Realm: "r", Nonce: "n0nce", Algorithm: algo, QOP: qop}
			hdr, err := c.Authorize("GET", "/cr?x=1", "acs-user", "p@ss\"w\\d", "", 1)
			if err != nil {
				t.Fatalf("algo=%q qop=%q: %v", algo, qop, err)
			}
			if !Verify("GET", hdr, "acs-user", "p@ss\"w\\d", "r", "n0nce") {
				t.Errorf("algo=%q qop=%q: verify gagal untuk kredensial benar\n%s", algo, qop, hdr)
			}
			if Verify("GET", hdr, "acs-user", "salah", "r", "n0nce") {
				t.Errorf("algo=%q qop=%q: password salah harus ditolak", algo, qop)
			}
			if Verify("POST", hdr, "acs-user", "p@ss\"w\\d", "r", "n0nce") {
				t.Errorf("algo=%q qop=%q: method berbeda harus ditolak", algo, qop)
			}
			if Verify("GET", hdr, "acs-user", "p@ss\"w\\d", "r", "nonce-lain") {
				t.Errorf("algo=%q qop=%q: nonce berbeda harus ditolak", algo, qop)
			}
		}
	}
}

func TestAuthorize_Unsupported(t *testing.T) {
	if _, err := (&Challenge{Nonce: "n", Algorithm: "SHA-512-256"}).Authorize("GET", "/", "u", "p", "", 1); err == nil {
		t.Error("algoritma tak didukung harus error")
	}
	if _, err := (&Challenge{Nonce: "n", QOP: "auth-int"}).Authorize("GET", "/", "u", "p", "", 1); err == nil {
		t.Error("hanya auth-int harus error")
	}
}
