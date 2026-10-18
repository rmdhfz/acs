package mailer

import (
	"bytes"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"strings"
	"testing"
)

func TestBuildMessage_DenganLampiranGambar(t *testing.T) {
	s := NewSMTP(Config{Host: "smtp.example.test", From: "noreply@example.test", FromName: "ACS"})
	to, _ := mail.ParseAddress("ops@example.test")
	png := append([]byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}, bytes.Repeat([]byte{7}, 500)...)
	raw, err := s.buildMessage(Message{
		To: []string{"ops@example.test"}, Subject: "uji", Text: "teks", HTML: "<p>html</p>",
		Attachments: []Attachment{{Filename: "../../etc/pass wd\r\nBcc: x@y.z.png", ContentType: "image/png", Data: png}},
	}, []*mail.Address{to})
	if err != nil {
		t.Fatal(err)
	}
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	mt, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if err != nil || mt != "multipart/mixed" {
		t.Fatalf("Content-Type = %q, mau multipart/mixed (%v)", mt, err)
	}
	mr := multipart.NewReader(msg.Body, params["boundary"])
	var sawAlt, sawImg bool
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		ct := p.Header.Get("Content-Type")
		switch {
		case strings.HasPrefix(ct, "multipart/alternative"):
			sawAlt = true
		case strings.HasPrefix(ct, "image/png"):
			sawImg = true
			cd := p.Header.Get("Content-Disposition")
			if strings.ContainsAny(cd, "/\\\r\n") {
				t.Fatalf("nama lampiran harus dibersihkan: %q", cd)
			}
			if !strings.Contains(cd, "attachment") {
				t.Fatalf("lampiran harus attachment: %q", cd)
			}
		}
	}
	if !sawAlt || !sawImg {
		t.Fatalf("struktur MIME salah: alt=%v img=%v", sawAlt, sawImg)
	}
	if msg.Header.Get("Bcc") != "" {
		t.Fatal("injeksi header Bcc lewat nama lampiran")
	}
}

func TestBuildMessage_TipeLampiranTerlarangDitolak(t *testing.T) {
	s := NewSMTP(Config{Host: "smtp.example.test", From: "noreply@example.test"})
	to, _ := mail.ParseAddress("ops@example.test")
	for _, ct := range []string{"text/html", "application/x-msdownload", "image/svg+xml", "image/png\r\nX: y"} {
		if _, err := s.buildMessage(Message{Subject: "x", Text: "t", Attachments: []Attachment{{Filename: "a", ContentType: ct, Data: []byte{1}}}}, []*mail.Address{to}); err == nil {
			t.Errorf("tipe %q harus ditolak", ct)
		}
	}
}

func TestBuildMessage_TanpaLampiranTetapAlternative(t *testing.T) {
	s := NewSMTP(Config{Host: "smtp.example.test", From: "noreply@example.test"})
	to, _ := mail.ParseAddress("ops@example.test")
	raw, err := s.buildMessage(Message{Subject: "x", Text: "t", HTML: "<p>h</p>"}, []*mail.Address{to})
	if err != nil {
		t.Fatal(err)
	}
	msg, _ := mail.ReadMessage(bytes.NewReader(raw))
	if mt, _, _ := mime.ParseMediaType(msg.Header.Get("Content-Type")); mt != "multipart/alternative" {
		t.Fatalf("tanpa lampiran harus multipart/alternative, dapat %q", mt)
	}
}
