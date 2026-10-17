package mysql

import (
	"encoding/json"
	"strings"
	"testing"

	"acs/pkg/cryptoutil"
)

func testSealer(t *testing.T) ValueSealer {
	t.Helper()
	enc, err := cryptoutil.NewEncryptor([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	return enc
}

func TestSealOpenValue(t *testing.T) {
	s := testSealer(t)
	// nama sensitif -> tersegel & kembali utuh
	sealed, err := sealValue(s, "InternetGatewayDevice.WANDevice.1.WANConnectionDevice.1.WANPPPConnection.1.Password", "rahasia-ppp")
	if err != nil || !strings.HasPrefix(sealed, sealedPrefix) || strings.Contains(sealed, "rahasia-ppp") {
		t.Fatalf("seal: %q err=%v", sealed, err)
	}
	if got, err := openValue(s, sealed); err != nil || got != "rahasia-ppp" {
		t.Fatalf("open: %q err=%v", got, err)
	}
	// nama tidak sensitif / nilai kosong / tanpa sealer -> apa adanya
	for _, c := range []struct {
		name, val string
		sealer    ValueSealer
	}{
		{"InternetGatewayDevice.DeviceInfo.Manufacturer", "ZTE", s},
		{"x.Password", "", s},
		{"x.Password", "plain", nil},
	} {
		if got, err := sealValue(c.sealer, c.name, c.val); err != nil || got != c.val {
			t.Fatalf("%+v harus tak berubah: %q %v", c, got, err)
		}
	}
	// data lama (plaintext tanpa awalan) tetap terbaca
	if got, err := openValue(s, "lama-plaintext"); err != nil || got != "lama-plaintext" {
		t.Fatalf("legacy: %q %v", got, err)
	}
	// nilai plaintext yang kebetulan berawalan enc:v1: tetap bolak-balik benar
	odd := sealedPrefix + "bukan-ciphertext"
	sealedOdd, _ := sealValue(s, "x.Password", odd)
	if got, err := openValue(s, sealedOdd); err != nil || got != odd {
		t.Fatalf("nilai berawalan enc:v1: harus bolak-balik: %q %v", got, err)
	}
	// fail closed: ciphertext rusak / kunci salah / tanpa sealer
	if _, err := openValue(s, sealedPrefix+"!!bukan-base64"); err == nil {
		t.Fatal("base64 rusak harus error")
	}
	other, _ := cryptoutil.NewEncryptor([]byte("ffffffffffffffffffffffffffffffff"))
	if _, err := openValue(other, sealed); err == nil {
		t.Fatal("kunci berbeda harus error, bukan mengembalikan sampah")
	}
	if _, err := openValue(nil, sealed); err == nil {
		t.Fatal("tanpa sealer harus error: ciphertext tak boleh lolos ke CPE/API")
	}
}

func TestPresetConfigs_SealOpen(t *testing.T) {
	s := testSealer(t)
	in := `[{"op":"set_parameter","key":"wifi.password","value":"kunci-wifi"},{"op":"set_parameter","key":"wifi.ssid","value":"ISP-Home"},{"op":"reboot"}]`
	sealed, err := sealPresetConfigs(s, in)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(sealed, "kunci-wifi") || !strings.Contains(sealed, sealedPrefix) {
		t.Fatalf("nilai rahasia harus tersegel: %s", sealed)
	}
	if !strings.Contains(sealed, "ISP-Home") {
		t.Fatalf("nilai tak sensitif harus tetap terbaca: %s", sealed)
	}
	opened, err := openPresetConfigs(s, sealed)
	if err != nil {
		t.Fatal(err)
	}
	var a, b []map[string]any
	_ = json.Unmarshal([]byte(in), &a)
	_ = json.Unmarshal([]byte(opened), &b)
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	if string(ja) != string(jb) {
		t.Fatalf("round-trip beda:\n%s\n%s", ja, jb)
	}
	// konfigurasi kosong / bukan array tidak menyebabkan error
	for _, c := range []string{"", "null", "[]", "bukan-json", `{"a":1}`} {
		if out, err := sealPresetConfigs(s, c); err != nil || out != c {
			t.Fatalf("%q harus lolos apa adanya: %q %v", c, out, err)
		}
	}
}
