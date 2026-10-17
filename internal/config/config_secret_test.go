package config

import "testing"

func TestWeakSecret(t *testing.T) {
	weak := []string{
		"change-me-to-a-random-secret-of-at-least-32-chars", // placeholder .env.example
		"CHANGEME_CHANGEME_CHANGEME_CHANGEME_1234",
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"abababababababababababababababababababab",
		"my-password-is-very-long-but-guessable-123",
	}
	for _, s := range weak {
		if !weakSecret([]byte(s)) {
			t.Errorf("%q harus dianggap lemah", s)
		}
	}
	strong := []string{
		"3f9a1c7e5b2d48a0963ce1f7a4d85b20c6e91f3a7b4d2058",
		"Zk8#vQ2!mX9$pL4@wN7&rT5^yB3*hJ6(",
	}
	for _, s := range strong {
		if weakSecret([]byte(s)) {
			t.Errorf("%q tidak boleh dianggap lemah", s)
		}
	}
}
