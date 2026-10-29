package domain

import (
	"reflect"
	"testing"
)

func TestValidParamPattern(t *testing.T) {
	ok := []string{
		"InternetGatewayDevice.LANDevice.1.WLANConfiguration.*.SSID",
		"InternetGatewayDevice.LANDevice.*.WLANConfiguration.*.SSID",
		"Device.WiFi.SSID.*.SSID",
		"InternetGatewayDevice.LANDevice.1.WLANConfiguration.1.SSID", // tanpa wildcard = ValidParamName biasa
		"wifi.5g.ssid", // logical key tanpa wildcard
	}
	for _, p := range ok {
		if !ValidParamPattern(p) {
			t.Errorf("harus sah: %q", p)
		}
	}
	bad := []string{
		"wifi.*.ssid", // wildcard pada logical key (bukan raw path)
		"InternetGatewayDevice.LANDevice.1.WLANConfiguration.*", // '*' di segmen terakhir
		"*.LANDevice.1.SSID", // '*' di segmen pertama
		"InternetGatewayDevice.LANDevice.*.WLANConfiguration.*.AssociatedDevice.*.MAC", // 3 wildcard
		"InternetGatewayDevice.LANDevice.1.WLANConfiguration.1*.SSID",                  // '*' bukan segmen utuh
		"InternetGatewayDevice.LANDevice.1.WLANConfiguration.**.SSID",                  // '**'
		"InternetGatewayDevice.LANDevice..WLANConfiguration.*.SSID",                    // segmen kosong
		"InternetGatewayDevice.LANDevice.1.WLANConfiguration.*.SS ID",                  // karakter ilegal
		"InternetGatewayDevice.LANDevice.1.WLANConfiguration.{*}.SSID",                 // karakter ilegal
	}
	for _, p := range bad {
		if ValidParamPattern(p) {
			t.Errorf("harus tak sah: %q", p)
		}
	}
}

func TestMatchParamPattern(t *testing.T) {
	pat := "InternetGatewayDevice.LANDevice.1.WLANConfiguration.*.SSID"
	cases := []struct {
		path string
		caps []string
		ok   bool
	}{
		{"InternetGatewayDevice.LANDevice.1.WLANConfiguration.1.SSID", []string{"1"}, true},
		{"InternetGatewayDevice.LANDevice.1.WLANConfiguration.12.SSID", []string{"12"}, true},
		{"InternetGatewayDevice.LANDevice.1.WLANConfiguration.1.SSIDAdvertisementEnabled", nil, false},
		{"InternetGatewayDevice.LANDevice.1.WLANConfiguration.a.SSID", nil, false},                // bukan instance numerik
		{"InternetGatewayDevice.LANDevice.1.WLANConfiguration.1.PreSharedKey.1.SSID", nil, false}, // '*' tak boleh melahap beberapa segmen
		{"InternetGatewayDevice.LANDevice.2.WLANConfiguration.1.SSID", nil, false},                // segmen literal berbeda
		{"InternetGatewayDevice.LANDevice.1.WLANConfiguration..SSID", nil, false},                 // kosong
		{"InternetGatewayDevice.LANDevice.1.WLANConfiguration.1234567890.SSID", nil, false},       // terlalu panjang
		{"InternetGatewayDevice.LANDevice.1.WLANConfiguration.1", nil, false},                     // lebih pendek
	}
	for _, c := range cases {
		caps, ok := MatchParamPattern(pat, c.path)
		if ok != c.ok || !reflect.DeepEqual(caps, c.caps) {
			t.Errorf("%q -> %v,%v; mau %v,%v", c.path, caps, ok, c.caps, c.ok)
		}
	}
	caps, ok := MatchParamPattern("Device.A.*.B.*.C", "Device.A.3.B.7.C")
	if !ok || !reflect.DeepEqual(caps, []string{"3", "7"}) {
		t.Errorf("dua wildcard: %v %v", caps, ok)
	}
}

func TestSubstituteParamWildcards(t *testing.T) {
	if got := SubstituteParamWildcards("Device.A.*.B.*.C", []string{"3", "7"}); got != "Device.A.3.B.7.C" {
		t.Errorf("got %q", got)
	}
	if got := SubstituteParamWildcards("Device.A.*.B.*.C", []string{"3"}); got != "Device.A.3.B.*.C" {
		t.Errorf("kurang caps: %q", got)
	}
	if got := SubstituteParamWildcards("Device.A.B", []string{"3"}); got != "Device.A.B" {
		t.Errorf("tanpa wildcard tak berubah: %q", got)
	}
	// hanya segmen persis '*' yang diganti
	if got := SubstituteParamWildcards("Device.A*.B.*", []string{"9"}); got != "Device.A*.B.9" {
		t.Errorf("'A*' bukan wildcard: %q", got)
	}
}

func TestFilterParamPattern_UrutNumerikUnikDanKetat(t *testing.T) {
	pat := "Device.WiFi.SSID.*.SSID"
	got := FilterParamPattern(pat, []string{
		"Device.WiFi.SSID.10.SSID", "Device.WiFi.SSID.2.SSID", "Device.WiFi.SSID.1.SSID", "Device.WiFi.SSID.2.SSID", // duplikat
		"Device.WiFi.SSID.1.SSID.Extra", "Device.WiFi.SSID.x.SSID", "Device.WiFi.SSIDx.1.SSID",
	})
	var paths []string
	for _, m := range got {
		paths = append(paths, m.Path)
	}
	want := []string{"Device.WiFi.SSID.1.SSID", "Device.WiFi.SSID.2.SSID", "Device.WiFi.SSID.10.SSID"}
	if !reflect.DeepEqual(paths, want) {
		t.Fatalf("got %v, mau %v (2 harus sebelum 10, tanpa duplikat, tanpa yang tak cocok)", paths, want)
	}
	if got[2].Caps[0] != "10" {
		t.Errorf("caps = %v", got[2].Caps)
	}
}

func TestParamWildcardCount(t *testing.T) {
	if ParamWildcardCount("A.*.B.*.C") != 2 || ParamWildcardCount("A.B*.C") != 0 || ParamWildcardCount("A.B") != 0 {
		t.Error("hitungan wildcard salah")
	}
	if !HasParamWildcard("A.B*.C") || HasParamWildcard("A.B.C") {
		t.Error("HasParamWildcard salah")
	}
}
