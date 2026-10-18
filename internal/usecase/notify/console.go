package notify

import (
	"fmt"
	"strings"
	"time"

	"acs/pkg/mailer"
)

// Email fitur Console (migrations/0033). Semua nilai dari pengguna dilewatkan sebagai TEKS ke html/template (di-escape)
// dan dipotong; subjek dibersihkan dari baris baru. Tidak pernah memuat password/token.

func clip(s string, n int) string {
	r := []rune(oneLine(s))
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return string(r)
}

// BugReport — email ke operator platform saat pengguna melaporkan bug.
func (b *Builder) BugReport(to, severityLabel, title, description, expected, reporter, reporterEmail, tenant, role, page, version, userAgent, reportID string, impersonated, hasScreenshot bool) mailer.Message {
	paras := []string{
		"Laporan bug baru dari Console.",
		"Judul: " + clip(title, 120),
		"Dampak: " + severityLabel,
		fmt.Sprintf("Pelapor: %s (%s), peran %s", clip(reporter, 64), clip(reporterEmail, 191), clip(role, 32)),
		"Tenant: " + clip(tenant, 128),
		"Halaman: " + clip(page, 255),
		"Versi Console: " + clip(version, 32),
		"Browser: " + clip(userAgent, 255),
	}
	if impersonated {
		paras = append(paras, "Catatan: dikirim dalam sesi impersonate (SUPERADMIN bertindak sebagai pengguna ini).")
	}
	// Dua bagian TERPISAH: apa yang terjadi vs apa yang diharapkan (tiap baris di-escape).
	section := func(head, body string) {
		paras = append(paras, head)
		for _, line := range strings.Split(strings.TrimSpace(body), "\n") {
			if l := strings.TrimSpace(line); l != "" {
				paras = append(paras, clip(l, 1000))
			}
		}
	}
	section("APA YANG TERJADI:", description)
	section("APA YANG DIHARAPKAN:", expected)
	if hasScreenshot {
		paras = append(paras, "Tangkapan layar terlampir pada email ini dan tersimpan di laporan.")
	}
	subject := fmt.Sprintf("[Bug %s] %s", severityLabel, clip(title, 80))
	return b.render(to, subject, layout{
		Greeting:   "Halo operator,",
		Paragraphs: paras,
		Button:     "Buka laporan",
		ButtonURL:  b.PublicURL + "/bug-reports",
		Footnote:   "ID laporan: " + clip(reportID, 36) + ". Laporan tersimpan di database dan dapat ditangani dari menu Laporan Bug.",
	})
}

// DataTransfer — email ke pelaku setiap proses impor/ekspor selesai. Hanya hitungan & ringkasan; tanpa isi berkas.
func (b *Builder) DataTransfer(to, name, kindLabel, moduleLabel, statusLabel string, total, success, failed uint32, fileName, detail string, at time.Time) mailer.Message {
	paras := []string{
		fmt.Sprintf("Proses %s data %s telah selesai dengan status: %s.", strings.ToLower(kindLabel), clip(moduleLabel, 64), statusLabel),
		fmt.Sprintf("Baris: %d total, %d berhasil, %d gagal.", total, success, failed),
		"Waktu: " + at.Format("02-01-2006 15:04 MST"),
	}
	if fileName != "" {
		paras = append(paras, "Berkas: "+clip(fileName, 255))
	}
	if detail != "" {
		paras = append(paras, "Catatan: "+clip(detail, 500))
	}
	return b.render(to, fmt.Sprintf("%s data %s — %s", kindLabel, clip(moduleLabel, 48), statusLabel), layout{
		Greeting:   greeting(name),
		Paragraphs: paras,
		Button:     "Lihat riwayat",
		ButtonURL:  b.PublicURL + "/data-transfers",
		Footnote:   "Jika Anda tidak merasa menjalankan proses ini, segera ganti password Anda dan hubungi administrator.",
	})
}

// BugReportUpdated — email ke PELAPOR saat status laporan bug berubah (mis. sudah ditangani). Hanya judul, status, dan
// catatan penanganan; tanpa data pelapor lain.
func (b *Builder) BugReportUpdated(to, name, title, statusLabel, note, reportID string) mailer.Message {
	paras := []string{
		fmt.Sprintf("Laporan bug Anda \"%s\" kini berstatus: %s.", clip(title, 120), statusLabel),
	}
	if note != "" {
		paras = append(paras, "Catatan dari tim: "+clip(note, 500))
	}
	paras = append(paras, "Terima kasih sudah membantu menyempurnakan aplikasi.")
	return b.render(to, "Laporan bug Anda: "+statusLabel, layout{
		Greeting:   greeting(name),
		Paragraphs: paras,
		Button:     "Lihat laporan",
		ButtonURL:  b.PublicURL + "/bug-reports",
		Footnote:   "ID laporan: " + clip(reportID, 36),
	})
}
