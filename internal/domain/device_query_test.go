package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestParseParamCond(t *testing.T) {
	ok := []struct {
		in   string
		want ParamCond
	}{
		{"InternetGatewayDevice.DeviceInfo.SoftwareVersion~prefix~V5", ParamCond{"InternetGatewayDevice.DeviceInfo.SoftwareVersion", "prefix", "V5"}},
		{"Device.WiFi.SSID.1.SSID~eq~a~b~c", ParamCond{"Device.WiFi.SSID.1.SSID", "eq", "a~b~c"}}, // nilai boleh memuat '~'
		{"X.Rx~lt~-25.5", ParamCond{"X.Rx", "lt", "-25.5"}},
		{"X.Y~exists", ParamCond{"X.Y", "exists", ""}},
		{"X.Y~missing", ParamCond{"X.Y", "missing", ""}},
	}
	for _, c := range ok {
		got, err := ParseParamCond(c.in)
		if err != nil || got != c.want {
			t.Errorf("%q -> %+v, %v; want %+v", c.in, got, err, c.want)
		}
	}
	bad := []string{
		"",                                 // kosong
		"X.Y",                              // tanpa op
		"X.Y~eq",                           // eq tanpa nilai
		"X.Y~exists~1",                     // exists tak memakai nilai
		"X.Y~gt~abc",                       // numerik butuh angka
		"X.Y~regex~.*",                     // op tak dikenal
		"X Y~eq~1",                         // spasi pada nama
		"X.Y;DROP TABLE~eq~1",              // karakter berbahaya pada nama
		"~eq~1",                            // nama kosong
		strings.Repeat("a", 257) + "~eq~1", // nama kepanjangan
		"X.Y~eq~" + strings.Repeat("v", MaxParamCondValue+1),
	}
	for _, in := range bad {
		if _, err := ParseParamCond(in); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("%q harus ErrInvalidInput, got %v", in, err)
		}
	}
}

func TestParseSort(t *testing.T) {
	if f, d, err := ParseSort(""); f != "" || d || err != nil {
		t.Errorf("kosong = default: %q %v %v", f, d, err)
	}
	if f, d, err := ParseSort("last_inform_at"); f != "last_inform_at" || d || err != nil {
		t.Errorf("asc: %q %v %v", f, d, err)
	}
	if f, d, err := ParseSort("-serial_number"); f != "serial_number" || !d || err != nil {
		t.Errorf("desc: %q %v %v", f, d, err)
	}
	for _, in := range []string{"inform_password_enc", "-", "id; DROP", "ID", "--id"} {
		if _, _, err := ParseSort(in); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("%q harus ditolak, got %v", in, err)
		}
	}
}
