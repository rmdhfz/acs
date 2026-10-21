package config

import "testing"

func TestCRTrustedCIDRs_DefaultKosongKetat(t *testing.T) {
	baseEnv(t)
	t.Setenv("ACS_CR_TRUSTED_CIDRS", "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.CRTrustedCIDRs) != 0 {
		t.Fatalf("bawaan harus kosong (anti-SSRF ketat): %v", cfg.CRTrustedCIDRs)
	}
}

func TestCRTrustedCIDRs_ParseCIDRDanIPTunggal(t *testing.T) {
	baseEnv(t)
	t.Setenv("ACS_CR_TRUSTED_CIDRS", " 10.0.0.0/8 , 192.168.50.7 ,, 2001:db8::/32")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, n := range cfg.CRTrustedCIDRs {
		got = append(got, n.String())
	}
	want := []string{"10.0.0.0/8", "192.168.50.7/32", "2001:db8::/32"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestCRTrustedCIDRs_MenolakNilaiTakValid(t *testing.T) {
	baseEnv(t)
	t.Setenv("ACS_CR_TRUSTED_CIDRS", "10.0.0.0/8,bukan-cidr")
	if _, err := Load(); err == nil {
		t.Fatal("CIDR rusak harus ditolak saat startup, bukan diabaikan diam-diam")
	}
}
