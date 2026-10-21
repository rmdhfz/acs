package domain

import "testing"

func TestOpticalLevelOf(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	cases := []struct {
		name string
		rx   *float64
		want string
	}{
		{"nil", nil, OpticalUnknown},
		{"normal", f(-20), OpticalOK},
		{"batas-warn-masih-ok", f(-25), OpticalOK},
		{"batas-overload-masih-ok", f(-8), OpticalOK},
		{"waspada", f(-25.01), OpticalWarn},
		{"batas-kritis-masih-waspada", f(-27), OpticalWarn},
		{"kritis-lemah", f(-27.01), OpticalBad},
		{"kritis-overload", f(-7.99), OpticalBad},
	}
	for _, c := range cases {
		if got := OpticalLevelOf(c.rx); got != c.want {
			t.Errorf("%s: dapat %q, mau %q", c.name, got, c.want)
		}
	}
}

func TestValidOpticalLevel(t *testing.T) {
	for _, v := range []string{"ok", "warn", "bad", "unknown"} {
		if !ValidOpticalLevel(v) {
			t.Errorf("%q harus valid", v)
		}
	}
	for _, v := range []string{"", "OK", "critical", "ok "} {
		if ValidOpticalLevel(v) {
			t.Errorf("%q harus ditolak", v)
		}
	}
}

func TestNormalizeRXDBM(t *testing.T) {
	cases := []struct {
		raw  float64
		want float64
		ok   bool
	}{
		{-23.5, -23.5, true},      // dBm langsung
		{-2350, -23.5, true},      // per 0,01 dBm
		{-23500, -23.5, true},     // per 0,001 dBm (TR-181 OpticalSignalLevel)
		{-27.01, -27.01, true},    // batas kritis tetap dBm
		{0, 0, true},              // 0 dBm masuk akal
		{-5000000, 0, false},      // tak ada skala yang masuk akal
		{2740000, 0, false},       // µW/ngawur: jangan dinilai
	}
	for _, c := range cases {
		got, ok := NormalizeRXDBM(c.raw)
		if ok != c.ok || (ok && (got-c.want > 1e-9 || c.want-got > 1e-9)) {
			t.Errorf("NormalizeRXDBM(%v) = (%v, %v), mau (%v, %v)", c.raw, got, ok, c.want, c.ok)
		}
	}
}
