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
	add := func(name, subj, html, text string) {
		msgs[name] = struct{ subject, html, text string }{subj, html, text}
	}
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

func TestNotification_RenderAman(t *testing.T) {
	b := NewBuilder("ACS Console", "https://acs.example.test/")
	evil := `<script>alert(1)</script>`

	m := b.Notification("a@example.test", evil, evil+"\r\nBcc: x@evil.test", []string{evil, "Baris kedua"}, "/devices/12", "Lihat perangkat", "CRITICAL")
	if strings.Contains(m.HTML, "<script>") {
		t.Error("nama, judul, dan paragraf harus di-escape di HTML")
	}
	if strings.ContainsAny(m.Subject, "\r\n") {
		t.Errorf("subject tidak boleh memuat CR/LF (header injection): %q", m.Subject)
	}
	if !strings.HasPrefix(m.Subject, "[ACS Console] ") {
		t.Errorf("subject harus berawalan merek: %q", m.Subject)
	}
	const wantLink = "https://acs.example.test/devices/12"
	if !strings.Contains(m.HTML, `href="`+wantLink+`"`) || !strings.Contains(m.Text, wantLink) {
		t.Error("tautan absolut harus ada di HTML (href) dan teks")
	}
	if !strings.Contains(m.HTML, "Lihat perangkat") || !strings.Contains(m.HTML, "Kritis") || !strings.Contains(m.Text, "Baris kedua") {
		t.Error("label tombol, chip keparahan, dan semua paragraf harus tampil")
	}
	if strings.Contains(m.HTML, "ZgotmplZ") {
		t.Error("template menolak nilai CSS/URL yang seharusnya sah")
	}
	if !strings.Contains(m.HTML, "border-radius:16px") || !strings.Contains(m.HTML, "#1a73e8") || !strings.Contains(m.HTML, "border-radius:20px") {
		t.Error("gaya Google (kartu 16px, biru #1a73e8, tombol pill 20px) harus dipakai")
	}
	if !strings.Contains(m.HTML, "linkedin.com/in/hfzrmd") || !strings.Contains(m.Text, "Hafiz Ramadhan") {
		t.Error("kredit pembuat harus ada")
	}
	if leakRe.MatchString(m.Text) {
		t.Error("email notifikasi tidak boleh memuat nilai password")
	}
}

func TestNotification_LinkBerbahayaDibuang(t *testing.T) {
	b := NewBuilder("ACS Console", "https://acs.example.test")
	for _, link := range []string{"https://evil.test/x", "//evil.test/x", "javascript:alert(1)", `/\evil.test`, ""} {
		m := b.Notification("a@example.test", "A", "Judul", []string{"isi"}, link, "", "INFO")
		if strings.Contains(m.HTML, "<a href=\"https://evil") || strings.Contains(m.HTML, "javascript:") || strings.Contains(m.Text, "evil.test") {
			t.Errorf("link berbahaya %q tidak boleh jadi tautan", link)
		}
		if strings.Contains(m.HTML, "Jika tombol tidak berfungsi") {
			t.Errorf("tanpa link valid (%q) tombol tidak boleh dirender", link)
		}
	}
	// keparahan tak dikenal diperlakukan INFO, tanpa memasukkan input ke style
	m := b.Notification("a@example.test", "A", "J", []string{"x"}, "", "", `red;background:url(x)`)
	if strings.Contains(m.HTML, "url(x)") || !strings.Contains(m.HTML, "Informasi") {
		t.Error("keparahan tak dikenal harus jatuh ke INFO")
	}
}
