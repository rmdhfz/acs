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
