package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestCleanText(t *testing.T) {
	if v, err := CleanText("f", "  Budi Santoso ", 2, 128, false); err != nil || v != "Budi Santoso" {
		t.Fatalf("harus dipangkas & lolos: %q %v", v, err)
	}
	if v, err := CleanText("f", "baris1\nbaris2", 1, 50, true); err != nil || v != "baris1\nbaris2" {
		t.Fatalf("newline diizinkan: %q %v", v, err)
	}
	for _, c := range []struct {
		in       string
		min, max int
		nl       bool
	}{
		{"   ", 1, 10, false}, {"a", 2, 10, false}, {strings.Repeat("é", 11), 1, 10, false},
		{"a\nb", 1, 10, false}, {"a\x00b", 1, 10, true}, {"\xff\xfe", 1, 10, false},
	} {
		if _, err := CleanText("f", c.in, c.min, c.max, c.nl); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("%q harus ditolak dengan ErrInvalidInput, dapat %v", c.in, err)
		}
	}
	// batas dihitung KARAKTER, bukan byte
	if _, err := CleanText("f", strings.Repeat("é", 10), 1, 10, false); err != nil {
		t.Errorf("10 karakter multibyte harus lolos: %v", err)
	}
}

func TestValidateUsernameTenantCodePassword(t *testing.T) {
	for _, u := range []string{"budi", "e2e-super@acs.local", "a.b_c-d+1"} {
		if _, err := ValidateUsername(u); err != nil {
			t.Errorf("username %q harus lolos: %v", u, err)
		}
	}
	for _, u := range []string{"", "ab", "  ", "a b c", "bud\ni", "-budi", strings.Repeat("a", 65), "budі" /* kirilik */} {
		if _, err := ValidateUsername(u); err == nil {
			t.Errorf("username %q harus ditolak", u)
		}
	}
	for _, c := range []string{"DEL123", "nusa-net-a1b2", "t.1_x"} {
		if _, err := ValidateTenantCode(c); err != nil {
			t.Errorf("code %q harus lolos: %v", c, err)
		}
	}
	for _, c := range []string{"", "a", "a b", "a\n", ".x", strings.Repeat("a", 33)} {
		if _, err := ValidateTenantCode(c); err == nil {
			t.Errorf("code %q harus ditolak", c)
		}
	}
	if err := ValidatePassword("Rahasia123"); err != nil {
		t.Errorf("password sah ditolak: %v", err)
	}
	for _, p := range []string{"pendek", strings.Repeat("x", MaxUserPasswordLen+1)} {
		if err := ValidatePassword(p); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("password %d byte harus ditolak: %v", len(p), err)
		}
	}
	if err := ValidatePassword("Budi.Santoso", "budi.santoso"); err == nil {
		t.Error("password sama dengan username harus ditolak")
	}
}
