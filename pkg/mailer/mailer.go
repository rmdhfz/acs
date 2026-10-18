// Package mailer mengirim email lewat SMTP (STARTTLS atau TLS implisit) dengan
// antrean asinkron + retry. Dipakai untuk reset password dan notifikasi
// keamanan; pengirim TIDAK pernah menulis isi email / token / kredensial SMTP
// ke log.
package mailer

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Message adalah satu email transaksional (teks + HTML alternatif).
type Message struct {
	To      []string
	Subject string
	Text    string
	HTML    string
	// Attachments (opsional): lampiran gambar kecil, mis. tangkapan layar laporan bug. Hanya tipe di allowedAttachmentTypes.
	Attachments []Attachment
}

// Attachment = satu lampiran email. Nama berkas dibersihkan (hanya [A-Za-z0-9._-]) dan tipe dibatasi supaya isi dari
// pengguna tidak bisa menyisipkan header MIME atau tipe berbahaya.
type Attachment struct {
	Filename    string
	ContentType string
	Data        []byte
}

var allowedAttachmentTypes = map[string]bool{"image/png": true, "image/jpeg": true}

func safeFilename(n string) string {
	var b strings.Builder
	for _, r := range n {
		if r < 128 && (r == '.' || r == '-' || r == '_' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')) {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	if b.Len() == 0 {
		return "lampiran"
	}
	out := b.String()
	if len(out) > 64 {
		out = out[:64]
	}
	return out
}

// Sender mengirim satu pesan secara sinkron.
type Sender interface {
	Send(ctx context.Context, m Message) error
}

// Notifier dipakai usecase: antrekan pesan, jangan blok request, jangan
// mengembalikan error ke pemanggil (kegagalan email tidak boleh menggagalkan
// operasi bisnis — hanya dicatat).
type Notifier interface {
	Notify(m Message)
}

// Config konfigurasi SMTP. Host kosong = email nonaktif (NewNotifier mengembalikan Noop).
type Config struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string // alamat pengirim, mis. noreply@domain atau akun Gmail
	FromName string // nama tampilan pengirim
	Timeout  time.Duration
	// AllowInsecure mengizinkan SMTP tanpa TLS — HANYA untuk test lokal.
	AllowInsecure bool
}

// Enabled melaporkan apakah konfigurasi cukup untuk mengirim email.
func (c Config) Enabled() bool { return c.Host != "" && c.From != "" }

// SMTP mengirim lewat server SMTP.
type SMTP struct {
	cfg Config
}

// NewSMTP membuat pengirim SMTP. cfg.Timeout default 20 detik.
func NewSMTP(cfg Config) *SMTP {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 20 * time.Second
	}
	if cfg.Port == 0 {
		cfg.Port = 587
	}
	return &SMTP{cfg: cfg}
}

var errHeaderInjection = errors.New("mailer: karakter kontrol (CR/LF) pada header tidak diizinkan")

func cleanHeader(s string) (string, error) {
	if strings.ContainsAny(s, "\r\n\x00") {
		return "", errHeaderInjection
	}
	return s, nil
}

func parseAddr(s string) (*mail.Address, error) {
	if _, err := cleanHeader(s); err != nil {
		return nil, err
	}
	a, err := mail.ParseAddress(s)
	if err != nil {
		return nil, fmt.Errorf("mailer: alamat email tidak valid: %w", err)
	}
	return a, nil
}

func randomID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// buildMessage menyusun MIME multipart/alternative (UTF-8) lengkap dengan
// header Date & Message-ID. Dipisah dari Send agar bisa diuji tanpa jaringan.
func (s *SMTP) buildMessage(m Message, to []*mail.Address) ([]byte, error) {
	from := &mail.Address{Name: s.cfg.FromName, Address: s.cfg.From}
	subject, err := cleanHeader(m.Subject)
	if err != nil {
		return nil, err
	}
	toStrs := make([]string, len(to))
	for i, a := range to {
		toStrs[i] = a.String()
	}
	domain := "localhost"
	if i := strings.LastIndex(s.cfg.From, "@"); i >= 0 {
		domain = s.cfg.From[i+1:]
	}
	boundary := "acs-" + randomID()
	for _, a := range m.Attachments {
		if !allowedAttachmentTypes[a.ContentType] {
			return nil, fmt.Errorf("mailer: tipe lampiran %q tidak diizinkan", a.ContentType)
		}
	}
	mixed := ""
	if len(m.Attachments) > 0 {
		mixed = "acs-m-" + randomID()
	}

	var b strings.Builder
	h := func(k, v string) { b.WriteString(k + ": " + v + "\r\n") }
	h("From", from.String())
	h("To", strings.Join(toStrs, ", "))
	h("Subject", mime.QEncoding.Encode("utf-8", subject))
	h("Date", time.Now().Format(time.RFC1123Z))
	h("Message-ID", "<"+randomID()+"@"+domain+">")
	h("MIME-Version", "1.0")
	h("Auto-Submitted", "auto-generated")
	if mixed != "" {
		h("Content-Type", `multipart/mixed; boundary="`+mixed+`"`)
		b.WriteString("\r\n--" + mixed + "\r\n")
	}
	h("Content-Type", `multipart/alternative; boundary="`+boundary+`"`)
	b.WriteString("\r\n")
	part := func(ctype, body string) {
		b.WriteString("--" + boundary + "\r\n")
		b.WriteString("Content-Type: " + ctype + "; charset=UTF-8\r\n")
		b.WriteString("Content-Transfer-Encoding: base64\r\n\r\n")
		b.WriteString(wrapBase64([]byte(body)))
	}
	part("text/plain", m.Text)
	if m.HTML != "" {
		part("text/html", m.HTML)
	}
	b.WriteString("--" + boundary + "--\r\n")
	if mixed != "" {
		for _, a := range m.Attachments {
			name := safeFilename(a.Filename)
			b.WriteString("\r\n--" + mixed + "\r\n")
			b.WriteString("Content-Type: " + a.ContentType + "; name=\"" + name + "\"\r\n")
			b.WriteString("Content-Disposition: attachment; filename=\"" + name + "\"\r\n")
			b.WriteString("Content-Transfer-Encoding: base64\r\n\r\n")
			b.WriteString(wrapBase64(a.Data))
		}
		b.WriteString("--" + mixed + "--\r\n")
	}
	return []byte(b.String()), nil
}

// Send mengirim email. STARTTLS (port 587) atau TLS implisit (port 465) WAJIB
// kecuali AllowInsecure — kredensial tidak pernah dikirim lewat koneksi polos.
func (s *SMTP) Send(ctx context.Context, m Message) error {
	if !s.cfg.Enabled() {
		return errors.New("mailer: SMTP belum dikonfigurasi")
	}
	if len(m.To) == 0 {
		return errors.New("mailer: penerima kosong")
	}
	to := make([]*mail.Address, 0, len(m.To))
	for _, r := range m.To {
		a, err := parseAddr(r)
		if err != nil {
			return err
		}
		to = append(to, a)
	}
	if _, err := parseAddr(s.cfg.From); err != nil {
		return fmt.Errorf("mailer: alamat pengirim tidak valid: %w", err)
	}
	if _, err := cleanHeader(s.cfg.FromName); err != nil {
		return err
	}
	payload, err := s.buildMessage(m, to)
	if err != nil {
		return err
	}

	addr := net.JoinHostPort(s.cfg.Host, strconv.Itoa(s.cfg.Port))
	d := &net.Dialer{Timeout: s.cfg.Timeout}
	tlsCfg := &tls.Config{ServerName: s.cfg.Host, MinVersion: tls.VersionTLS12}

	var conn net.Conn
	if s.cfg.Port == 465 {
		conn, err = tls.DialWithDialer(d, "tcp", addr, tlsCfg)
	} else {
		conn, err = d.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("mailer: gagal terhubung ke SMTP: %w", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(s.cfg.Timeout))

	c, err := smtp.NewClient(conn, s.cfg.Host)
	if err != nil {
		return fmt.Errorf("mailer: handshake SMTP gagal: %w", err)
	}
	defer c.Close()

	if s.cfg.Port != 465 {
		if ok, _ := c.Extension("STARTTLS"); ok {
			if err := c.StartTLS(tlsCfg); err != nil {
				return fmt.Errorf("mailer: STARTTLS gagal: %w", err)
			}
		} else if !s.cfg.AllowInsecure {
			return errors.New("mailer: server SMTP tidak mendukung STARTTLS (ditolak demi keamanan)")
		}
	}
	if s.cfg.Username != "" {
		auth := smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)
		if s.cfg.AllowInsecure {
			auth = insecurePlainAuth{username: s.cfg.Username, password: s.cfg.Password}
		}
		if err := c.Auth(auth); err != nil {
			return fmt.Errorf("mailer: autentikasi SMTP gagal: %w", err)
		}
	}
	if err := c.Mail(s.cfg.From); err != nil {
		return fmt.Errorf("mailer: MAIL FROM ditolak: %w", err)
	}
	for _, a := range to {
		if err := c.Rcpt(a.Address); err != nil {
			return fmt.Errorf("mailer: RCPT TO ditolak: %w", err)
		}
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("mailer: DATA gagal: %w", err)
	}
	if _, err := w.Write(payload); err != nil {
		return fmt.Errorf("mailer: menulis pesan gagal: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("mailer: pesan ditolak server: %w", err)
	}
	return c.Quit()
}

// insecurePlainAuth — PLAIN tanpa pemeriksaan TLS, khusus pengujian lokal.
type insecurePlainAuth struct{ username, password string }

func (a insecurePlainAuth) Start(*smtp.ServerInfo) (string, []byte, error) {
	return "PLAIN", []byte("\x00" + a.username + "\x00" + a.password), nil
}
func (a insecurePlainAuth) Next([]byte, bool) ([]byte, error) { return nil, nil }

func wrapBase64(data []byte) string {
	const lineLen = 76
	enc := base64.StdEncoding.EncodeToString(data)
	var b strings.Builder
	for i := 0; i < len(enc); i += lineLen {
		end := i + lineLen
		if end > len(enc) {
			end = len(enc)
		}
		b.WriteString(enc[i:end])
		b.WriteString("\r\n")
	}
	return b.String()
}

// MaskEmail menyamarkan alamat untuk log (a***@gmail.com).
func MaskEmail(e string) string {
	at := strings.LastIndex(e, "@")
	if at <= 0 {
		return "***"
	}
	return e[:1] + "***" + e[at:]
}

// Noop dipakai bila SMTP belum dikonfigurasi: pesan dibuang (dicatat sekali di
// level WARN tanpa isi/penerima lengkap).
type Noop struct {
	log  *slog.Logger
	once sync.Once
}

func NewNoop(log *slog.Logger) *Noop { return &Noop{log: log} }

func (n *Noop) Notify(Message) {
	n.once.Do(func() {
		if n.log != nil {
			n.log.Warn("mailer: SMTP belum dikonfigurasi (ACS_SMTP_HOST/ACS_SMTP_FROM) — email notifikasi tidak dikirim")
		}
	})
}

// Queue mengirim pesan di goroutine worker dengan retry terbatas. Notify tidak
// pernah memblok: bila antrean penuh, pesan dibuang dan dicatat.
type Queue struct {
	sender  Sender
	log     *slog.Logger
	ch      chan Message
	wg      sync.WaitGroup
	backoff []time.Duration
	// mu melindungi closed + penutupan channel: Notify yang datang setelah
	// Close() dibuang (bukan panic "send on closed channel").
	mu     sync.RWMutex
	closed bool
}

// NewQueue memulai `workers` goroutine; buf = kapasitas antrean.
func NewQueue(s Sender, log *slog.Logger, workers, buf int) *Queue {
	if workers < 1 {
		workers = 2
	}
	if buf < 1 {
		buf = 256
	}
	q := &Queue{sender: s, log: log, ch: make(chan Message, buf), backoff: []time.Duration{2 * time.Second, 8 * time.Second}}
	for i := 0; i < workers; i++ {
		q.wg.Add(1)
		go q.worker()
	}
	return q
}

func (q *Queue) worker() {
	defer q.wg.Done()
	for m := range q.ch {
		q.deliver(m)
	}
}

func (q *Queue) deliver(m Message) {
	defer func() {
		if r := recover(); r != nil && q.log != nil {
			q.log.Error("mailer: panic saat mengirim email", "panic", fmt.Sprint(r))
		}
	}()
	var err error
	for attempt := 0; attempt <= len(q.backoff); attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		err = q.sender.Send(ctx, m)
		cancel()
		if err == nil {
			return
		}
		// Kesalahan permanen (alamat/header tidak valid, auth ditolak) tidak
		// perlu diulang.
		if errors.Is(err, errHeaderInjection) || strings.Contains(err.Error(), "alamat email tidak valid") || strings.Contains(err.Error(), "autentikasi SMTP gagal") {
			break
		}
		if attempt < len(q.backoff) {
			time.Sleep(q.backoff[attempt])
		}
	}
	if q.log != nil {
		rcpt := ""
		if len(m.To) > 0 {
			rcpt = MaskEmail(m.To[0])
		}
		q.log.Error("mailer: email gagal terkirim", "to", rcpt, "subject", m.Subject, "error", err)
	}
}

// reservedTLDs / reservedDomains — domain yang MUSTAHIL punya kotak surat nyata
// (RFC 2606 / 6761 / 6762). Mengirim ke sana hanya menghasilkan bounce ke
// pengirim, jadi pesan seperti itu dibuang sebelum antrean (mis. akun uji e2e
// `*@e2e.local`, `*@example.test`).
var reservedTLDs = map[string]bool{"test": true, "example": true, "invalid": true, "localhost": true, "local": true}
var reservedDomains = map[string]bool{"example.com": true, "example.org": true, "example.net": true}

// IsUndeliverable melaporkan apakah alamat memakai domain khusus-uji/reserved.
func IsUndeliverable(addr string) bool {
	at := strings.LastIndex(addr, "@")
	if at < 0 {
		return true
	}
	d := strings.ToLower(strings.TrimSpace(strings.Trim(addr[at+1:], ">")))
	if reservedDomains[d] {
		return true
	}
	tld := d[strings.LastIndex(d, ".")+1:]
	return reservedTLDs[tld]
}

// Notify mengantrekan pesan tanpa memblok. Penerima berdomain reserved dibuang;
// bila tidak ada penerima tersisa, pesan tidak diantrekan sama sekali.
func (q *Queue) Notify(m Message) {
	kept := make([]string, 0, len(m.To))
	for _, to := range m.To {
		if IsUndeliverable(to) {
			if q.log != nil {
				q.log.Debug("mailer: penerima domain reserved dilewati", "to", MaskEmail(to))
			}
			continue
		}
		kept = append(kept, to)
	}
	if len(kept) == 0 {
		return
	}
	m.To = kept
	q.mu.RLock()
	defer q.mu.RUnlock()
	if q.closed {
		return
	}
	select {
	case q.ch <- m:
	default:
		if q.log != nil {
			q.log.Error("mailer: antrean email penuh, pesan dibuang", "subject", m.Subject)
		}
	}
}

// Close menghentikan menerima pesan baru dan menunggu antrean habis (maks. 30 dtk).
func (q *Queue) Close() {
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return
	}
	q.closed = true
	close(q.ch)
	q.mu.Unlock()
	done := make(chan struct{})
	go func() { q.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
	}
}

// NewNotifier memilih implementasi: Queue+SMTP bila dikonfigurasi, selain itu Noop.
func NewNotifier(cfg Config, log *slog.Logger) (Notifier, func()) {
	if !cfg.Enabled() {
		return NewNoop(log), func() {}
	}
	q := NewQueue(NewSMTP(cfg), log, 4, 256)
	return q, q.Close
}
