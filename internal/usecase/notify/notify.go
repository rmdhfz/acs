// Package notify menyusun email transaksional (reset password, peringatan
// keamanan, selamat datang). Tidak mengirim apa pun — hanya membangun
// mailer.Message; pengiriman dilakukan mailer.Notifier oleh pemanggil.
//
// Aturan isi: email TIDAK PERNAH memuat password (termasuk password sementara)
// — hanya tautan sekali-pakai atau instruksi (CLAUDE.md: kredensial tidak
// boleh muncul di jalur yang bisa tersimpan/ter-log).
package notify

import (
	"bytes"
	"fmt"
	"html/template"
	"strings"
	"time"

	"acs/pkg/mailer"
)

const (
	// CreditName/CreditURL — atribusi pembuat, ikut di footer email.
	CreditName = "Hafiz Ramadhan"
	CreditURL  = "https://linkedin.com/in/hfzrmd"
)

// Builder membangun pesan dengan brand & URL publik aplikasi.
type Builder struct {
	Brand     string // mis. "ACS Console"
	PublicURL string // origin frontend, mis. https://acs.example.com (tanpa slash akhir)
}

// NewBuilder — brand default "ACS Console".
func NewBuilder(brand, publicURL string) *Builder {
	if brand == "" {
		brand = "ACS Console"
	}
	return &Builder{Brand: brand, PublicURL: strings.TrimRight(publicURL, "/")}
}

// ResetLink menyusun tautan reset. Token ditaruh di FRAGMENT (#token=…), bukan
// query: fragment tidak dikirim ke server, tidak masuk access log proxy, dan
// tidak bocor lewat header Referer.
func (b *Builder) ResetLink(token string) string {
	return b.PublicURL + "/reset-password#token=" + token
}

// VerifySignupLink — tautan verifikasi pendaftaran tenant; token di FRAGMENT (tidak masuk log proxy/Referer).
func (b *Builder) VerifySignupLink(token string) string {
	return b.PublicURL + "/verify-signup#token=" + token
}

// LoginURL — halaman masuk.
func (b *Builder) LoginURL() string { return b.PublicURL + "/login" }

type layout struct {
	Brand      string
	Title      string
	Greeting   string
	Paragraphs []string
	Button     string
	ButtonURL  string
	Footnote   string
	CreditName string
	CreditURL  string
	Year       int
	// Chip keparahan (opsional; hanya email notifikasi). Warna bukan input pengguna: dipilih dari tabel tetap.
	SevLabel string
	SevFG    template.CSS
	SevBG    template.CSS
}

// htmlTpl — gaya Google/Material: kartu putih bersudut 16px di atas latar abu terang, header ringan (bukan blok
// gelap), aksen biru #1a73e8, teks #202124/#5f6368, tombol pill biru. Dibangun dengan tabel + style inline agar
// kompatibel dengan klien email (Gmail, Outlook, Apple Mail); font Google Sans/Roboto dengan fallback Arial.
var htmlTpl = template.Must(template.New("mail").Parse(`<!doctype html>
<html lang="id"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta name="color-scheme" content="light"><title>{{.Title}}</title></head>
<body style="margin:0;padding:0;background:#f1f3f4;font-family:'Google Sans',Roboto,'Segoe UI',Helvetica,Arial,sans-serif;color:#202124">
<table role="presentation" width="100%" cellspacing="0" cellpadding="0" border="0" style="background:#f1f3f4;padding:24px 12px"><tr><td align="center">
<table role="presentation" width="100%" cellspacing="0" cellpadding="0" border="0" style="max-width:560px;background:#ffffff;border-radius:16px;border:1px solid #dadce0">
<tr><td style="padding:24px 32px 0 32px"><table role="presentation" cellspacing="0" cellpadding="0" border="0"><tr>
<td style="width:28px;height:28px;background:#1a73e8;border-radius:14px;text-align:center;color:#ffffff;font-size:14px;font-weight:700;line-height:28px">&#9679;</td>
<td style="padding-left:10px;font-size:15px;font-weight:500;color:#5f6368;font-family:'Google Sans',Roboto,Arial,sans-serif">{{.Brand}}</td></tr></table></td></tr>
<tr><td style="padding:20px 32px 8px 32px">
{{if .SevLabel}}<p style="margin:0 0 12px"><span style="display:inline-block;background:{{.SevBG}};color:{{.SevFG}};border-radius:12px;padding:3px 12px;font-size:12px;font-weight:500;line-height:18px">{{.SevLabel}}</span></p>{{end}}
<h1 style="margin:0 0 16px;font-size:22px;line-height:1.35;font-weight:400;color:#202124;font-family:'Google Sans',Roboto,Arial,sans-serif">{{.Title}}</h1>
<p style="margin:0 0 14px;font-size:14px;line-height:1.6;color:#202124">{{.Greeting}}</p>
{{range .Paragraphs}}<p style="margin:0 0 14px;font-size:14px;line-height:1.6;color:#202124">{{.}}</p>{{end}}
{{if .ButtonURL}}<p style="margin:24px 0"><a href="{{.ButtonURL}}" style="display:inline-block;background:#1a73e8;color:#ffffff;text-decoration:none;font-weight:500;font-size:14px;line-height:20px;padding:10px 24px;border-radius:20px;font-family:'Google Sans',Roboto,Arial,sans-serif">{{.Button}}</a></p>
<p style="margin:0 0 14px;font-size:12px;line-height:1.6;color:#5f6368">Jika tombol tidak berfungsi, salin tautan ini ke browser:<br><span style="word-break:break-all;color:#1a73e8">{{.ButtonURL}}</span></p>{{end}}
{{if .Footnote}}<p style="margin:20px 0 0;padding-top:16px;border-top:1px solid #e8eaed;font-size:12px;line-height:1.6;color:#5f6368">{{.Footnote}}</p>{{end}}
</td></tr>
<tr><td style="padding:16px 32px 24px 32px;font-size:12px;line-height:1.6;color:#5f6368;text-align:center">&copy; {{.Year}} {{.Brand}} &middot; Dibuat oleh <a href="{{.CreditURL}}" style="color:#5f6368;text-decoration:underline">{{.CreditName}}</a><br>Email otomatis &mdash; mohon tidak membalas.</td></tr>
</table></td></tr></table></body></html>`))

func (b *Builder) render(to, subject string, l layout) mailer.Message {
	l.Brand, l.CreditName, l.CreditURL, l.Year = b.Brand, CreditName, CreditURL, time.Now().Year()
	if l.Title == "" {
		l.Title = subject
	}
	var buf bytes.Buffer
	_ = htmlTpl.Execute(&buf, l)

	var t strings.Builder
	t.WriteString(l.Greeting + "\n\n")
	for _, p := range l.Paragraphs {
		t.WriteString(p + "\n\n")
	}
	if l.ButtonURL != "" {
		t.WriteString(l.Button + ":\n" + l.ButtonURL + "\n\n")
	}
	if l.Footnote != "" {
		t.WriteString(l.Footnote + "\n\n")
	}
	t.WriteString(fmt.Sprintf("-- \n%s · Dibuat oleh %s (%s)\n", b.Brand, CreditName, CreditURL))
	return mailer.Message{To: []string{to}, Subject: subject, Text: t.String(), HTML: buf.String()}
}

func greeting(name string) string {
	if strings.TrimSpace(name) == "" {
		return "Halo,"
	}
	return "Halo " + name + ","
}

// SignupVerification — verifikasi email pendaftaran tenant mandiri.
func (b *Builder) SignupVerification(to, name, company, link string, ttl time.Duration) mailer.Message {
	return b.render(to, "Verifikasi pendaftaran "+b.Brand, layout{
		Greeting: greeting(name),
		Paragraphs: []string{
			fmt.Sprintf("Terima kasih telah mendaftarkan %q di %s. Klik tombol di bawah untuk memverifikasi email dan mengaktifkan akun administrator Anda.", company, b.Brand),
		},
		Button:    "Verifikasi & aktifkan akun",
		ButtonURL: link,
		Footnote:  fmt.Sprintf("Tautan ini berlaku %d jam dan hanya bisa dipakai sekali. Jika Anda tidak mendaftar, abaikan email ini — tidak ada akun yang dibuat.", int(ttl.Hours())),
	})
}

// PasswordReset — tautan reset password sekali-pakai.
func (b *Builder) PasswordReset(to, name, link string, ttl time.Duration) mailer.Message {
	return b.render(to, "Reset password akun "+b.Brand, layout{
		Greeting:   greeting(name),
		Paragraphs: []string{"Kami menerima permintaan untuk mereset password akun Anda. Klik tombol di bawah untuk membuat password baru."},
		Button:     "Buat password baru",
		ButtonURL:  link,
		Footnote:   fmt.Sprintf("Tautan ini berlaku %d menit dan hanya bisa dipakai sekali. Jika Anda tidak meminta reset, abaikan email ini — password Anda tidak berubah.", int(ttl.Minutes())),
	})
}

// PasswordChanged — pemberitahuan password berhasil diganti (alarm bila bukan pemilik).
func (b *Builder) PasswordChanged(to, name string) mailer.Message {
	return b.render(to, "Password akun Anda telah diganti", layout{
		Greeting:   greeting(name),
		Paragraphs: []string{"Password akun " + b.Brand + " Anda baru saja diganti."},
		Footnote:   "Jika ini bukan Anda, segera hubungi administrator dan minta reset password. Semua sesi aktif sebaiknya dianggap tidak aman.",
	})
}

// PasswordResetByAdmin — admin mereset password pengguna lain.
func (b *Builder) PasswordResetByAdmin(to, name string) mailer.Message {
	return b.render(to, "Password Anda direset oleh administrator", layout{
		Greeting:   greeting(name),
		Paragraphs: []string{"Administrator telah mereset password akun " + b.Brand + " Anda. Administrator akan memberikan password sementara lewat kanal internal — kami tidak pernah mengirim password lewat email."},
		Button:     "Masuk",
		ButtonURL:  b.LoginURL(),
		Footnote:   "Segera ganti password sementara itu lewat menu Profil → Keamanan setelah masuk.",
	})
}

// AccountLocked — akun dikunci otomatis karena banyak percobaan salah.
func (b *Builder) AccountLocked(to, name string, minutes int) mailer.Message {
	return b.render(to, "Akun Anda dikunci sementara", layout{
		Greeting:   greeting(name),
		Paragraphs: []string{fmt.Sprintf("Akun %s Anda dikunci selama %d menit karena terlalu banyak percobaan masuk dengan password salah.", b.Brand, minutes)},
		Button:     "Reset password",
		ButtonURL:  b.PublicURL + "/forgot-password",
		Footnote:   "Jika itu Anda, tunggu hingga kunci terbuka atau reset password sekarang. Jika bukan Anda, ada pihak lain yang mencoba masuk — gunakan reset password untuk mengamankan akun.",
	})
}

// severityStyle — chip keparahan untuk email notifikasi (teks, warna latar, warna teks). Tabel tetap: nilai
// keparahan tak dikenal diperlakukan INFO, sehingga input tak pernah masuk ke atribut style.
func severityStyle(sev string) (label string, fg, bg template.CSS) {
	switch strings.ToUpper(sev) {
	case "CRITICAL":
		return "Kritis", "#a50e0e", "#fce8e6"
	case "WARNING":
		return "Peringatan", "#b06000", "#fef7e0"
	default:
		return "Informasi", "#1967d2", "#e8f0fe"
	}
}

// oneLine meratakan spasi/baris baru jadi satu spasi (aman dipakai di header Subject).
func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// Notification — email untuk notifikasi in-app (lihat usecase/notification). `link` adalah path internal Console
// (diawali "/") dan dijadikan tautan absolut dengan PublicURL; link yang bukan path internal dibuang. Semua teks
// di-escape oleh html/template; pemanggil bertanggung jawab tidak memasukkan rahasia ke title/paragraphs.
func (b *Builder) Notification(to, name, title string, paragraphs []string, link, linkLabel string, severity string) mailer.Message {
	title = oneLine(title)
	l := layout{
		Title:      title, // judul di badan email tanpa awalan merek; Subject memakai awalan
		Greeting:   greeting(name),
		Paragraphs: paragraphs,
		Footnote:   "Anda menerima email ini karena pengaturan notifikasi akun " + b.Brand + " Anda. Kanal notifikasi dapat diatur di Console.",
	}
	l.SevLabel, l.SevFG, l.SevBG = severityStyle(severity)
	if strings.HasPrefix(link, "/") && !strings.HasPrefix(link, "//") && !strings.Contains(link, "\\") {
		l.ButtonURL = b.PublicURL + link
		l.Button = strings.TrimSpace(linkLabel)
		if l.Button == "" {
			l.Button = "Buka di Console"
		}
	}
	return b.render(to, "["+b.Brand+"] "+title, l)
}

// Welcome — akun baru dibuat oleh administrator.
func (b *Builder) Welcome(to, name, email, role string) mailer.Message {
	paras := []string{
		fmt.Sprintf("Administrator telah membuat akun %s untuk Anda.", b.Brand),
		"Email login: " + email,
	}
	if role != "" {
		paras = append(paras, "Peran: "+role)
	}
	paras = append(paras, "Password awal diberikan administrator lewat kanal internal (tidak dikirim lewat email). Lupa atau belum menerimanya? Gunakan tombol di bawah untuk membuat password sendiri.")
	return b.render(to, "Selamat datang di "+b.Brand, layout{
		Greeting:   greeting(name),
		Paragraphs: paras,
		Button:     "Buat password / masuk",
		ButtonURL:  b.PublicURL + "/forgot-password",
		Footnote:   "Jika Anda tidak merasa perlu akun ini, abaikan email ini atau hubungi administrator.",
	})
}
