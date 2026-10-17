package mysql

import "testing"

func TestLikeEscape(t *testing.T) {
	cases := map[string]string{
		"abc":     "abc",
		"50%":     `50\%`,
		"a_b":     `a\_b`,
		`C:\path`: `C:\\path`,
		`%_\`:     `\%\_\\`,
	}
	for in, want := range cases {
		if got := likeEscape(in); got != want {
			t.Errorf("likeEscape(%q) = %q, mau %q", in, got, want)
		}
	}
}
