package console

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"

	"acs/internal/domain"
)

func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		img.Set(x, x%h, color.RGBA{R: 200, A: 255})
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func jpegBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := jpeg.Encode(&b, image.NewRGBA(image.Rect(0, 0, w, h)), nil); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// "Apa yang terjadi" dan "apa yang diharapkan" adalah dua kolom terpisah dan keduanya wajib.
func TestBugReport_DuaKolomTerpisah(t *testing.T) {
	s, _, mail, _ := newBugSvc()
	ctx := context.Background()
	in := goodBug()
	in.Expected = ""
	if _, err := s.Create(ctx, adminT1, in); !isInvalid(err) {
		t.Fatalf("harapan kosong harus ditolak: %v", err)
	}
	in.Expected = "abc"
	if _, err := s.Create(ctx, adminT1, in); !isInvalid(err) {
		t.Fatalf("harapan terlalu pendek harus ditolak: %v", err)
	}
	in.Expected = strings.Repeat("x", bugExpectedMax+1)
	if _, err := s.Create(ctx, adminT1, in); !isInvalid(err) {
		t.Fatalf("harapan terlalu panjang harus ditolak: %v", err)
	}
	in = goodBug()
	rep, err := s.Create(ctx, adminT1, in)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Description == rep.Expected || rep.Expected != in.Expected || rep.Description != in.Description {
		t.Fatalf("dua kolom harus tersimpan terpisah: %+v", rep)
	}
	text := mail.msgs[len(mail.msgs)-1].Text
	if !strings.Contains(text, "APA YANG TERJADI") || !strings.Contains(text, "APA YANG DIHARAPKAN") {
		t.Fatalf("email harus memuat dua bagian terpisah:\n%s", text)
	}
}

func TestBugReport_TangkapanLayar_ValidasiDanLampiranEmail(t *testing.T) {
	s, repo, mail, _ := newBugSvc()
	ctx := context.Background()

	ok := goodBug()
	ok.Screenshot = pngBytes(t, 400, 300)
	rep, err := s.Create(ctx, adminT1, ok)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.HasScreenshot || repo.shots[rep.ID] == nil || repo.shots[rep.ID].ContentType != "image/png" || repo.shots[rep.ID].SHA256 == "" {
		t.Fatalf("tangkapan layar harus tersimpan dengan jenis hasil validasi: %+v", repo.shots[rep.ID])
	}
	m := mail.msgs[len(mail.msgs)-1]
	if len(m.Attachments) != 1 || m.Attachments[0].ContentType != "image/png" || !strings.HasSuffix(m.Attachments[0].Filename, ".png") {
		t.Fatalf("email operator harus melampirkan gambar: %+v", m.Attachments)
	}
	jp := goodBug()
	jp.Screenshot = jpegBytes(t, 300, 200)
	if r, err := s.Create(ctx, adminT1, jp); err != nil || repo.shots[r.ID].ContentType != "image/jpeg" {
		t.Fatalf("JPEG harus diterima: %v", err)
	}

	bad := map[string][]byte{
		"teks berekstensi gambar": []byte("ini bukan gambar sama sekali, hanya teks biasa"),
		"svg (skrip)":             []byte("<svg xmlns='http://www.w3.org/2000/svg'><script>alert(1)</script></svg>"),
		"html":                    []byte("<html><script>alert(1)</script></html>"),
		"png terpotong":           append([]byte("\x89PNG\r\n\x1a\n"), 1, 2, 3),
		"jpeg terpotong":          {0xff, 0xd8, 0xff, 0xe0, 0, 1},
		"terlalu besar":           append(pngBytes(t, 400, 300), bytes.Repeat([]byte{0}, maxScreenshotBytes)...),
		"dimensi terlalu kecil":   pngBytes(t, 4, 4),
		"gif":                     []byte("GIF89a\x01\x00\x01\x00\x00\x00\x00;"),
	}
	for name, data := range bad {
		in := goodBug()
		in.Screenshot = data
		if _, err := s.Create(ctx, adminT1, in); !isInvalid(err) {
			t.Errorf("%s: harus ErrInvalidInput, dapat %v", name, err)
		}
	}
}

// IDOR: gambar hanya untuk pelapor & SUPERADMIN; laporan tanpa gambar atau milik orang lain = NotFound.
func TestBugReport_TangkapanLayar_TanpaIDOR(t *testing.T) {
	s, _, _, _ := newBugSvc()
	ctx := context.Background()
	in := goodBug()
	in.Screenshot = pngBytes(t, 200, 100)
	a, _ := s.Create(ctx, adminT1, in)
	noShot, _ := s.Create(ctx, adminT1, goodBug())

	if sc, err := s.GetScreenshot(ctx, adminT1, a.UUID); err != nil || sc.ContentType != "image/png" {
		t.Fatalf("pelapor harus bisa membaca gambarnya: %v", err)
	}
	if _, err := s.GetScreenshot(ctx, superA, a.UUID); err != nil {
		t.Fatalf("superadmin harus bisa: %v", err)
	}
	for name, actor := range map[string]domain.Actor{"admin tenant lain": adminT2, "noc sesama tenant": nocT1, "enduser": endT1} {
		if _, err := s.GetScreenshot(ctx, actor, a.UUID); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("%s: harus NotFound, dapat %v", name, err)
		}
	}
	if _, err := s.GetScreenshot(ctx, adminT1, noShot.UUID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("laporan tanpa gambar harus NotFound: %v", err)
	}
	if _, err := s.GetScreenshot(ctx, adminT1, "../etc/passwd"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("id ngawur harus NotFound: %v", err)
	}
}

// Pelapor diberi tahu (in-app + email) saat statusnya berubah; tidak saat tidak berubah atau laporan dibuat via impersonate.
func TestBugReport_PelaporDiberiTahuSaatDitangani(t *testing.T) {
	s, _, mail, notif := newBugSvc()
	ctx := context.Background()
	rep, _ := s.Create(ctx, adminT1, goodBug())
	baseMail, baseNotif := len(mail.msgs), len(notif.events)
	note := "sudah diperbaiki di rilis 1.0.1"

	if _, err := s.UpdateStatus(ctx, superA, rep.UUID, domain.BugStatusResolved, &note); err != nil {
		t.Fatal(err)
	}
	if len(mail.msgs) != baseMail+1 {
		t.Fatalf("email ke pelapor harus terkirim (email=%d, dasar=%d)", len(mail.msgs), baseMail)
	}
	m := mail.msgs[len(mail.msgs)-1]
	if m.To[0] != "adm1@x.id" || !strings.Contains(m.Subject, "Sudah ditangani") || !strings.Contains(m.Text, note) {
		t.Fatalf("email pelapor salah: to=%v subject=%q", m.To, m.Subject)
	}
	if len(notif.events) != baseNotif+1 {
		t.Fatalf("notifikasi in-app ke pelapor harus dibuat")
	}
	ev := notif.events[len(notif.events)-1]
	if ev.TypeCode != domain.NotifBugReportUpdate || ev.Audience.Kind != domain.AudienceSelf || ev.Audience.UserID != adminT1.UserID || !ev.NoEmail {
		t.Fatalf("notifikasi harus AudienceSelf ke pelapor: %+v", ev)
	}
	if !strings.Contains(ev.Title, "sudah ditangani") {
		t.Fatalf("judul notifikasi harus menyebut sudah ditangani: %q", ev.Title)
	}
	// status sama lagi -> tidak ada pemberitahuan baru
	if _, err := s.UpdateStatus(ctx, superA, rep.UUID, domain.BugStatusResolved, &note); err != nil {
		t.Fatal(err)
	}
	if len(mail.msgs) != baseMail+1 || len(notif.events) != baseNotif+1 {
		t.Fatal("status tak berubah tidak boleh memberi tahu lagi")
	}
	// laporan dari sesi impersonate: pelapor sebenarnya SUPERADMIN -> jangan mengirim ke pengguna tenant
	imp, _ := s.Create(ctx, impT1, goodBug())
	before := len(mail.msgs)
	if _, err := s.UpdateStatus(ctx, superA, imp.UUID, domain.BugStatusInProgress, nil); err != nil {
		t.Fatal(err)
	}
	if len(mail.msgs) != before {
		t.Fatal("laporan hasil impersonate tidak boleh mengirim email ke pengguna tenant")
	}
}
