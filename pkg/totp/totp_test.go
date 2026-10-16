package totp

import (
	"strings"
	"testing"
	"time"
)

// Test vector RFC 6238 Appendix B (SHA-1, rahasia ASCII "12345678901234567890"; 8 digit di RFC,
// 6 digit = 6 digit terakhirnya).
func TestRFC6238Vectors(t *testing.T) {
	secret := b32.EncodeToString([]byte("12345678901234567890"))
	cases := []struct {
		unix int64
		want string // 6 digit terakhir dari nilai 8-digit RFC
	}{
		{59, "287082"}, {1111111109, "081804"}, {1111111111, "050471"},
		{1234567890, "005924"}, {2000000000, "279037"}, {20000000000, "353130"},
	}
	for _, c := range cases {
		got, err := Code(secret, time.Unix(c.unix, 0))
		if err != nil || got != c.want {
			t.Errorf("t=%d: got %q err=%v, want %q", c.unix, got, err, c.want)
		}
	}
}

func TestVerifyWindowDanReplay(t *testing.T) {
	secret, err := GenerateSecret()
	if err != nil || len(secret) != 32 {
		t.Fatalf("secret: %q %v", secret, err)
	}
	now := time.Unix(1_700_000_000, 0)
	code, _ := Code(secret, now)
	step, ok := Verify(secret, code, now, 1)
	if !ok || step != uint64(now.Unix()/Period) {
		t.Fatalf("kode saat ini harus valid: %d %v", step, ok)
	}
	// ±1 langkah diterima, ±2 ditolak
	if _, ok := Verify(secret, code, now.Add(30*time.Second), 1); !ok {
		t.Error("selisih +30 dtk harus diterima")
	}
	if _, ok := Verify(secret, code, now.Add(90*time.Second), 1); ok {
		t.Error("selisih +90 dtk harus ditolak")
	}
	for _, bad := range []string{"", "12345", "1234567", "abcdef", "000000x"} {
		if _, ok := Verify(secret, bad, now, 1); ok {
			t.Errorf("format salah %q harus ditolak", bad)
		}
	}
	if _, ok := Verify("bukan-base32!!", code, now, 1); ok {
		t.Error("secret rusak harus ditolak")
	}
}

func TestProvisioningURI(t *testing.T) {
	u := ProvisioningURI("ACS Console", "budi@x.id", "JBSWY3DPEHPK3PXP")
	if !strings.HasPrefix(u, "otpauth://totp/ACS%20Console:budi@x.id?") || !strings.Contains(u, "secret=JBSWY3DPEHPK3PXP") || !strings.Contains(u, "period=30") {
		t.Fatal(u)
	}
}
