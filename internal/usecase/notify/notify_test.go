package notify

import (
	"regexp"
	"strings"
	"testing"
	"time"
)

var leakRe = regexp.MustCompile(`(?i)(password|kata sandi)[^
:=]{0,20}[:=][ 	]*S`)

func TestMessagesRenderSafely(t *testing.T) {
	b := NewBuilder("ACS Console", "https://acs.example.test/")
	evil := `<script>alert(1)</script>`
	link := b.ResetLink("TOKEN123")
	if link != "https://acs.example.test/reset-password#token=TOKEN123" {
		t.Fatalf("link reset salah / harus pakai fragment: %s", link)
	}

	msgs := map[string]struct {
		subject, html, text string
	}{}
	add := func(name, subj, html, text string) { msgs[name] = struct{ subject, html, text string }{subj, html, text} }
	m := b.PasswordReset("a@example.test", evil, link, 30*time.Minute)
	add("reset", m.Subject, m.HTML, m.Text)
	m = b.PasswordChanged("a@example.test", evil)
	add("changed", m.Subject, m.HTML, m.Text)
	m = b.PasswordResetByAdmin("a@example.test", evil)
	add("admin", m.Subject, m.HTML, m.Text)
	m = b.AccountLocked("a@example.test", evil, 15)
	add("locked", m.Subject, m.HTML, m.Text)
	m = b.Welcome("a@example.test", evil, "a@example.test", "ADMIN")
	add("welcome", m.Subject, m.HTML, m.Text)

	for name, c := range msgs {
		if strings.Contains(c.html, "<script>") {
			t.Errorf("%s: nama pengguna harus di-escape di HTML", name)
		}
		if !strings.Contains(c.html, "linkedin.com/in/hfzrmd") || !strings.Contains(c.text, "Hafiz Ramadhan") {
			t.Errorf("%s: kredit pembuat harus ada", name)
		}
		if c.subject == "" || strings.ContainsAny(c.subject, "\r\n") {
			t.Errorf("%s: subject tidak valid: %q", name, c.subject)
		}
		// Tidak boleh ada pola "password: <nilai>" di baris yang sama (label
		// tombol diikuti baris baru + URL, jadi tidak cocok).
		if leakRe.MatchString(c.text) {
			t.Errorf("%s: email tidak boleh memuat nilai password", name)
		}
	}
	if !strings.Contains(msgs["reset"].text, "30 menit") || !strings.Contains(msgs["reset"].html, "#token=TOKEN123") {
		t.Error("email reset harus memuat masa berlaku & tautan")
	}
}
