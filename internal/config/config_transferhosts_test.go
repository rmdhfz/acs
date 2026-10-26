package config

import "testing"

func TestTransferAllowedHosts_DefaultTidakDibatasi(t *testing.T) {
	baseEnv(t)
	t.Setenv("ACS_TRANSFER_ALLOWED_HOSTS", "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TransferAllowedHosts != nil {
		t.Fatal("bawaan harus nil (perilaku lama: semua host diizinkan)")
	}
	if !cfg.TransferAllowedHosts.Allows("https://mana-saja.example.net/fw.bin") {
		t.Fatal("daftar nil harus mengizinkan semua host")
	}
}

func TestTransferAllowedHosts_DiparsedDanDipakai(t *testing.T) {
	baseEnv(t)
	t.Setenv("ACS_TRANSFER_ALLOWED_HOSTS", " files.example.com , *.cdn.example.org, 10.0.0.0/24 ")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	for url, want := range map[string]bool{
		"https://files.example.com/fw.bin": true,
		"https://a.cdn.example.org/fw.bin": true,
		"http://10.0.0.8/fw.bin":           true,
		"https://evil.example.net/fw.bin":  false,
	} {
		if got := cfg.TransferAllowedHosts.Allows(url); got != want {
			t.Errorf("Allows(%q) = %v, harapan %v", url, got, want)
		}
	}
}

func TestTransferAllowedHosts_MenolakNilaiTakValid(t *testing.T) {
	baseEnv(t)
	t.Setenv("ACS_TRANSFER_ALLOWED_HOSTS", "files.example.com,https://salah.example.com")
	if _, err := Load(); err == nil {
		t.Fatal("entri tak valid harus menggagalkan startup, bukan diabaikan diam-diam (bisa membuka atau menutup akses)")
	}
}
