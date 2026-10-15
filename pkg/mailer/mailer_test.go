package mailer

import (
	"bufio"
	"context"
	"errors"
	"net"
	"net/mail"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestBuildMessage(t *testing.T) {
	s := NewSMTP(Config{Host: "smtp.example.test", From: "noreply@example.test", FromName: "ACS Console"})
	to, _ := mail.ParseAddress("Budi <budi@example.test>")
	raw, err := s.buildMessage(Message{Subject: "Reset password — ACS", Text: "halo teks", HTML: "<b>halo</b>"}, []*mail.Address{to})
	if err != nil {
		t.Fatal(err)
	}
	msg := string(raw)
	for _, want := range []string{
		"From: \"ACS Console\" <noreply@example.test>", "To: \"Budi\" <budi@example.test>",
		"Subject: =?utf-8?q?Reset_password_=E2=80=94_ACS?=", "MIME-Version: 1.0",
		"multipart/alternative", "Content-Type: text/plain; charset=UTF-8", "Content-Type: text/html; charset=UTF-8",
		"Message-ID: <", "Auto-Submitted: auto-generated",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("pesan tidak memuat %q\n%s", want, msg)
		}
	}
	// Body base64: isi asli tidak muncul polos.
	if strings.Contains(msg, "halo teks") {
		t.Error("body harus ter-encode base64")
	}
	if _, err := mail.ReadMessage(strings.NewReader(msg)); err != nil {
		t.Errorf("pesan harus parseable sebagai RFC 5322: %v", err)
	}
}

func TestHeaderInjectionRejected(t *testing.T) {
	s := NewSMTP(Config{Host: "smtp.example.test", From: "noreply@example.test"})
	for name, m := range map[string]Message{
		"subject":   {To: []string{"a@example.test"}, Subject: "x\r\nBcc: evil@example.test", Text: "t"},
		"recipient": {To: []string{"a@example.test\r\nBcc: evil@example.test"}, Subject: "x", Text: "t"},
	} {
		if err := s.Send(context.Background(), m); err == nil {
			t.Errorf("%s: injeksi header harus ditolak", name)
		}
	}
	if err := s.Send(context.Background(), Message{To: []string{"bukan-email"}, Subject: "x", Text: "t"}); err == nil {
		t.Error("alamat tidak valid harus ditolak")
	}
}

func TestMaskEmail(t *testing.T) {
	if got := MaskEmail("hfzrmd@gmail.com"); got != "h***@gmail.com" {
		t.Errorf("MaskEmail = %q", got)
	}
	if MaskEmail("tanpa-at") != "***" {
		t.Error("tanpa @ harus ***")
	}
}

// fakeSMTP — server SMTP minimal (tanpa TLS) untuk menguji alur Send & Queue.
type fakeSMTP struct {
	ln       net.Listener
	mu       sync.Mutex
	data     []string
	authSeen bool
}

func newFakeSMTP(t *testing.T) *fakeSMTP {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeSMTP{ln: ln}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go f.serve(c)
		}
	}()
	return f
}

func (f *fakeSMTP) serve(c net.Conn) {
	defer c.Close()
	r := bufio.NewReader(c)
	w := func(s string) { c.Write([]byte(s + "\r\n")) }
	w("220 fake ESMTP")
	var body strings.Builder
	inData := false
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		if inData {
			if line == "." {
				inData = false
				f.mu.Lock()
				f.data = append(f.data, body.String())
				f.mu.Unlock()
				w("250 OK queued")
				continue
			}
			body.WriteString(line + "\n")
			continue
		}
		up := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(up, "EHLO"), strings.HasPrefix(up, "HELO"):
			c.Write([]byte("250-fake\r\n250 AUTH PLAIN\r\n"))
		case strings.HasPrefix(up, "AUTH"):
			f.mu.Lock()
			f.authSeen = true
			f.mu.Unlock()
			w("235 ok")
		case strings.HasPrefix(up, "MAIL"), strings.HasPrefix(up, "RCPT"):
			w("250 OK")
		case up == "DATA":
			inData = true
			w("354 go ahead")
		case up == "QUIT":
			w("221 bye")
			return
		default:
			w("250 OK")
		}
	}
}

func (f *fakeSMTP) hostPort() (string, int) {
	a := f.ln.Addr().(*net.TCPAddr)
	return "127.0.0.1", a.Port
}

func TestSendRequiresSTARTTLSUnlessInsecure(t *testing.T) {
	f := newFakeSMTP(t)
	host, port := f.hostPort()
	msg := Message{To: []string{"a@example.test"}, Subject: "x", Text: "t"}

	strict := NewSMTP(Config{Host: host, Port: port, From: "n@example.test", Username: "u", Password: "p"})
	if err := strict.Send(context.Background(), msg); err == nil || !strings.Contains(err.Error(), "STARTTLS") {
		t.Fatalf("server tanpa STARTTLS harus ditolak, dapat %v", err)
	}
	f.mu.Lock()
	seen := f.authSeen
	f.mu.Unlock()
	if seen {
		t.Fatal("kredensial TIDAK boleh dikirim sebelum TLS")
	}

	loose := NewSMTP(Config{Host: host, Port: port, From: "n@example.test", Username: "u", Password: "p", AllowInsecure: true})
	if err := loose.Send(context.Background(), msg); err != nil {
		t.Fatalf("AllowInsecure harus berhasil: %v", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.data) != 1 || !strings.Contains(f.data[0], "Subject: x") {
		t.Fatalf("pesan tidak diterima server: %v", f.data)
	}
}

type flakySender struct {
	mu    sync.Mutex
	calls int
	failN int
}

func (f *flakySender) Send(context.Context, Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.calls <= f.failN {
		return errors.New("sementara gagal")
	}
	return nil
}

func TestQueueRetriesAndDoesNotBlock(t *testing.T) {
	fs := &flakySender{failN: 1}
	q := NewQueue(fs, nil, 1, 4)
	q.backoff = []time.Duration{10 * time.Millisecond, 10 * time.Millisecond}
	start := time.Now()
	q.Notify(Message{To: []string{"a@perusahaan.co.id"}, Subject: "x"})
	if time.Since(start) > 100*time.Millisecond {
		t.Fatal("Notify tidak boleh memblok")
	}
	q.Close()
	fs.mu.Lock()
	defer fs.mu.Unlock()
	if fs.calls != 2 {
		t.Fatalf("harus retry sekali setelah gagal: calls=%d", fs.calls)
	}
}

func TestQueueDropsWhenFull(t *testing.T) {
	block := make(chan struct{})
	q := &Queue{sender: blockingSender{block}, ch: make(chan Message, 1)}
	q.Notify(Message{Subject: "1"})
	q.Notify(Message{Subject: "2"}) // penuh → dibuang, tidak memblok
	close(block)
}

type blockingSender struct{ c chan struct{} }

func (b blockingSender) Send(context.Context, Message) error { <-b.c; return nil }

func TestIsUndeliverable(t *testing.T) {
	for _, a := range []string{"a@e2e.local", "a@example.test", "A@Example.COM", "a@foo.invalid", "a@localhost", "tanpa-at"} {
		if !IsUndeliverable(a) {
			t.Errorf("%q harus dianggap tidak bisa terkirim", a)
		}
	}
	for _, a := range []string{"hfzrmd@gmail.com", "hfzrmd+x@gmail.com", "a@perusahaan.co.id", "a@mail.example.co"} {
		if IsUndeliverable(a) {
			t.Errorf("%q harus dianggap valid", a)
		}
	}
}

func TestQueueSkipsReservedDomains(t *testing.T) {
	fs := &flakySender{}
	q := NewQueue(fs, nil, 1, 4)
	q.Notify(Message{To: []string{"x@e2e.local", "y@example.test"}, Subject: "x"})
	q.Notify(Message{To: []string{"x@e2e.local", "ok@gmail.com"}, Subject: "y"})
	q.Close()
	fs.mu.Lock()
	defer fs.mu.Unlock()
	if fs.calls != 1 {
		t.Fatalf("hanya pesan dengan penerima valid yang dikirim: calls=%d", fs.calls)
	}
}
