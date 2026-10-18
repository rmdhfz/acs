package main

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"strings"
)

// e2ePNG membuat PNG kecil yang sah untuk uji unggah tangkapan layar.
func e2ePNG(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < w; i++ {
		img.Set(i, i%h, color.RGBA{R: 220, A: 255})
	}
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}

// S26 — fitur Console (migrations/0033) + pembuktian TANPA IDOR lintas tenant:
//   - layout menu: milik pengguna sendiri; pengguna/tenant lain tidak melihatnya; bentuk jahat ditolak 400;
//   - laporan bug: pelapor hanya melihat miliknya, laporan tenant lain 404, hanya SUPERADMIN yang mengubah status;
//   - riwayat impor/ekspor: staf hanya melihat tenant-nya (parameter tenant_id dari klien diabaikan), baris tenant lain
//     404, SUPERADMIN melihat semua; angka tak masuk akal ditolak 400.
func scenarioConsole(ctx context.Context, w *World) result {
	run := w.runID
	a, b, su := w.pilotAdmin, w.pilot2Admin, w.super

	// ---- 1. layout menu ----
	layout := map[string]any{
		"version": 1,
		"nodes": []any{
			map[string]any{"type": "item", "key": "devices"},
			map[string]any{"type": "group", "id": "favorit", "label": "Favorit " + run[:4], "children": []string{"tasks", "tags"}},
		},
		"hidden": []string{"files"},
	}
	var got struct {
		Layout map[string]any `json:"layout"`
	}
	if err := a.do(http.MethodPut, "/api/v1/auth/menu-layout", layout, &got); err != nil || got.Layout == nil {
		return failr("PUT menu-layout (admin pilot): %v", err)
	}
	for who, c := range map[string]*Client{"admin tenant lain": b, "superadmin": su} {
		var other struct {
			Layout map[string]any `json:"layout"`
		}
		if err := c.do(http.MethodGet, "/api/v1/auth/menu-layout", nil, &other); err != nil {
			return failr("GET menu-layout (%s): %v", who, err)
		}
		if other.Layout != nil {
			return failr("IDOR: %s melihat layout menu milik admin pilot: %v", who, other.Layout)
		}
	}
	if err := a.do(http.MethodGet, "/api/v1/auth/menu-layout", nil, &got); err != nil || got.Layout == nil {
		return failr("pemilik harus melihat layout-nya sendiri: %v", err)
	}
	for name, body := range map[string]any{
		"tipe asing":         map[string]any{"version": 1, "nodes": []any{map[string]any{"type": "script", "key": "x"}}, "hidden": []string{}},
		"key jahat":          map[string]any{"version": 1, "nodes": []any{map[string]any{"type": "item", "key": "javascript:alert(1)"}}, "hidden": []string{}},
		"key ganda":          map[string]any{"version": 1, "nodes": []any{map[string]any{"type": "item", "key": "a"}, map[string]any{"type": "item", "key": "a"}}, "hidden": []string{}},
		"label kontrol":      map[string]any{"version": 1, "nodes": []any{map[string]any{"type": "group", "id": "g", "label": "a\u0000b", "children": []string{}}}, "hidden": []string{}},
		"versi salah":        map[string]any{"version": 9, "nodes": []any{}, "hidden": []string{}},
		"bidang tak dikenal": map[string]any{"version": 1, "nodes": []any{}, "hidden": []string{}, "x": 1},
	} {
		if st, bd := a.status(http.MethodPut, "/api/v1/auth/menu-layout", body); st != 400 {
			return failr("layout %s harus 400, dapat %d %s", name, st, bd)
		}
	}
	if st, _ := a.status(http.MethodDelete, "/api/v1/auth/menu-layout", nil); st >= 300 {
		return failr("DELETE menu-layout: HTTP %d", st)
	}
	if err := a.do(http.MethodGet, "/api/v1/auth/menu-layout", nil, &got); err != nil || got.Layout != nil {
		return failr("setelah reset layout harus null: %v %v", err, got.Layout)
	}

	// ---- 2. laporan bug ----
	bugBody := func(title string) map[string]any {
		return map[string]any{"title": title, "description": "Apa yang terjadi: uji e2e " + run + " (cukup panjang).", "expected": "Apa yang diharapkan: tidak ada galat.", "severity": "MAJOR", "page_path": "/tags?x=1", "user_agent": "e2e", "app_version": "1.0.0"}
	}
	var bugA, bugB struct {
		ID       string  `json:"id"`
		TenantID *uint64 `json:"tenant_id"`
		Path     string  `json:"page_path"`
	}
	if err := a.do(http.MethodPost, "/api/v1/bug-reports", bugBody("Bug A "+run), &bugA); err != nil || bugA.ID == "" {
		return failr("POST bug-reports (admin pilot): %v", err)
	}
	if bugA.TenantID == nil || *bugA.TenantID != w.pilotTenantID || bugA.Path != "/tags" {
		return failr("tenant/konteks laporan harus dari token & dibersihkan: %+v", bugA)
	}
	if err := b.do(http.MethodPost, "/api/v1/bug-reports", bugBody("Bug B "+run), &bugB); err != nil || bugB.ID == "" {
		return failr("POST bug-reports (admin pilot2): %v", err)
	}
	// field asing (mis. mencoba menyetel tenant/pelapor) ditolak
	evil := bugBody("Bug evil " + run)
	evil["tenant_id"] = w.pilot2TenantID
	if st, bd := a.status(http.MethodPost, "/api/v1/bug-reports", evil); st != 400 {
		return failr("body dengan tenant_id harus 400 (tidak ada mass assignment), dapat %d %s", st, bd)
	}
	for name, mut := range map[string]func(map[string]any){
		"judul pendek":     func(m map[string]any) { m["title"] = "abc" },
		"deskripsi kosong": func(m map[string]any) { m["description"] = "          " },
		"dampak asing":     func(m map[string]any) { m["severity"] = "CRITICAL" },
		"judul panjang":    func(m map[string]any) { m["title"] = strings.Repeat("x", 121) },
		"harapan kosong":   func(m map[string]any) { m["expected"] = "" },
		"harapan pendek":   func(m map[string]any) { m["expected"] = "abc" },
	} {
		body := bugBody("Bug valid " + run)
		mut(body)
		if st, bd := a.status(http.MethodPost, "/api/v1/bug-reports", body); st != 400 {
			return failr("laporan %s harus 400, dapat %d %s", name, st, bd)
		}
	}
	// dua kolom terpisah + tangkapan layar (multipart)
	fields := map[string]string{"title": "Bug dengan gambar " + run, "description": "Terjadi: tombol tidak merespons " + run, "expected": "Diharapkan: tombol menyimpan data", "severity": "MAJOR", "page_path": "/tags", "user_agent": "e2e", "app_version": "1.0.0"}
	var bugShot struct {
		ID            string `json:"id"`
		Description   string `json:"description"`
		Expected      string `json:"expected"`
		HasScreenshot bool   `json:"has_screenshot"`
	}
	if err := a.uploadMultipart("/api/v1/bug-reports", fields, "screenshot", "layar.png", e2ePNG(320, 200), &bugShot); err != nil || bugShot.ID == "" {
		return failr("POST bug-reports multipart (dengan PNG): %v", err)
	}
	if !bugShot.HasScreenshot || bugShot.Description == bugShot.Expected || !strings.Contains(bugShot.Expected, "Diharapkan") || !strings.Contains(bugShot.Description, "Terjadi") {
		return failr("dua kolom harus terpisah & has_screenshot true: %+v", bugShot)
	}
	st, ctype, body, err := a.getRaw("/api/v1/bug-reports/" + bugShot.ID + "/screenshot")
	if err != nil || st != 200 || ctype != "image/png" || !bytes.HasPrefix(body, []byte("\x89PNG")) {
		return failr("pelapor mengambil gambar: status=%d ctype=%q err=%v", st, ctype, err)
	}
	if st, _, _, _ := su.getRaw("/api/v1/bug-reports/" + bugShot.ID + "/screenshot"); st != 200 {
		return failr("superadmin harus bisa mengambil gambar laporan mana pun, dapat %d", st)
	}
	if st, _, _, _ := b.getRaw("/api/v1/bug-reports/" + bugShot.ID + "/screenshot"); st != 404 {
		return failr("IDOR: admin tenant lain mengambil gambar harus 404, dapat %d", st)
	}
	if st, _, _, _ := a.getRaw("/api/v1/bug-reports/" + bugA.ID + "/screenshot"); st != 404 {
		return failr("laporan tanpa gambar harus 404, dapat %d", st)
	}
	for name, data := range map[string][]byte{"teks": []byte("bukan gambar"), "svg": []byte("<svg xmlns='http://www.w3.org/2000/svg'><script>alert(1)</script></svg>"), "png rusak": []byte("\x89PNG\r\n\x1a\nrusak")} {
		if err := a.uploadMultipart("/api/v1/bug-reports", fields, "screenshot", "x.png", data, nil); err == nil {
			return failr("unggah %s sebagai tangkapan layar harus ditolak", name)
		} else if ae, ok := err.(*apiError); !ok || ae.Status != 400 {
			return failr("unggah %s harus 400, dapat %v", name, err)
		}
	}
	if err := a.uploadMultipart("/api/v1/bug-reports", fields, "lampiran", "x.png", e2ePNG(64, 64), nil); err == nil {
		return failr("berkas berkunci tak dikenal harus ditolak")
	}
	evilFields := map[string]string{}
	for k, v := range fields {
		evilFields[k] = v
	}
	evilFields["tenant_id"] = fmt.Sprint(w.pilot2TenantID)
	if err := a.uploadMultipart("/api/v1/bug-reports", evilFields, "screenshot", "x.png", e2ePNG(64, 64), nil); err == nil {
		return failr("field multipart tak dikenal (tenant_id) harus ditolak")
	}
	// pelapor diberi tahu saat bug ditangani (notifikasi in-app BUG_REPORT_UPDATE)
	if st, bd := su.status(http.MethodPatch, "/api/v1/bug-reports/"+bugShot.ID, map[string]any{"status": "RESOLVED", "note": "sudah diperbaiki"}); st != 200 {
		return failr("superadmin menandai selesai: HTTP %d %s", st, bd)
	}
	var notifs notifList
	if err := a.do(http.MethodGet, "/api/v1/notifications?page_size=50", nil, &notifs); err != nil {
		return failr("notifikasi pelapor: %v", err)
	}
	gotUpdate := false
	for _, n := range notifs.Data {
		if n.Type == "BUG_REPORT_UPDATE" && strings.Contains(n.Title, "sudah ditangani") {
			gotUpdate = true
		}
	}
	if !gotUpdate {
		return failr("pelapor harus menerima notifikasi 'sudah ditangani' (BUG_REPORT_UPDATE)")
	}
	var notifsB notifList
	if err := b.do(http.MethodGet, "/api/v1/notifications?page_size=50", nil, &notifsB); err == nil {
		for _, n := range notifsB.Data {
			if n.Type == "BUG_REPORT_UPDATE" && strings.Contains(n.Body, run) {
				return failr("IDOR: notifikasi status laporan bocor ke admin tenant lain")
			}
		}
	}

	// IDOR: laporan tenant lain
	if st, _ := b.status(http.MethodGet, "/api/v1/bug-reports/"+bugA.ID, nil); st != 404 {
		return failr("IDOR: admin pilot2 membaca laporan pilot harus 404, dapat %d", st)
	}
	if st, _ := a.status(http.MethodGet, "/api/v1/bug-reports/"+bugB.ID, nil); st != 404 {
		return failr("IDOR: admin pilot membaca laporan pilot2 harus 404, dapat %d", st)
	}
	var listB struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := b.do(http.MethodGet, fmt.Sprintf("/api/v1/bug-reports?tenant_id=%d&page_size=200", w.pilotTenantID), nil, &listB); err != nil {
		return failr("list bug-reports (pilot2): %v", err)
	}
	for _, r := range listB.Data {
		if r.ID == bugA.ID {
			return failr("IDOR: daftar laporan pilot2 memuat laporan tenant pilot (filter tenant_id harus diabaikan)")
		}
	}
	// hanya SUPERADMIN yang mengubah status
	for who, c := range map[string]*Client{"admin pelapor": a, "admin tenant lain": b} {
		if st, _ := c.status(http.MethodPatch, "/api/v1/bug-reports/"+bugA.ID, map[string]any{"status": "RESOLVED"}); st != 403 {
			return failr("%s mengubah status laporan harus 403, dapat %d", who, st)
		}
	}
	if st, bd := su.status(http.MethodPatch, "/api/v1/bug-reports/"+bugA.ID, map[string]any{"status": "IN_PROGRESS", "note": "sedang dicek e2e"}); st != 200 {
		return failr("superadmin mengubah status: HTTP %d %s", st, bd)
	}
	if st, _ := su.status(http.MethodPatch, "/api/v1/bug-reports/"+bugA.ID, map[string]any{"status": "HAPUS"}); st != 400 {
		return failr("status asing harus 400, dapat %d", st)
	}
	var seen map[string]bool = map[string]bool{}
	var listSu struct {
		Data []struct {
			ID            string `json:"id"`
			ReporterEmail string `json:"reporter_email"`
		} `json:"data"`
	}
	if err := su.do(http.MethodGet, "/api/v1/bug-reports?page_size=200", nil, &listSu); err != nil {
		return failr("list bug-reports (superadmin): %v", err)
	}
	for _, r := range listSu.Data {
		seen[r.ID] = r.ReporterEmail != ""
	}
	if !seen[bugA.ID] || !seen[bugB.ID] {
		return failr("superadmin harus melihat laporan semua tenant lengkap dengan email pelapor (A=%v B=%v)", seen[bugA.ID], seen[bugB.ID])
	}

	// ---- 3. riwayat impor/ekspor ----
	tr := func(kind, status, module string, total, ok, bad int) map[string]any {
		return map[string]any{"kind": kind, "status": status, "module": module, "format": "csv", "file_name": `C:\fakepath\data-` + run + ".csv", "total_rows": total, "success_rows": ok, "failed_rows": bad, "detail": "uji e2e " + run}
	}
	var trA, trB struct {
		ID       string  `json:"id"`
		TenantID *uint64 `json:"tenant_id"`
		File     *string `json:"file_name"`
		Source   string  `json:"source"`
	}
	if err := a.do(http.MethodPost, "/api/v1/data-transfers", tr("IMPORT", "PARTIAL", "devices", 10, 7, 3), &trA); err != nil || trA.ID == "" {
		return failr("POST data-transfers (pilot): %v", err)
	}
	if trA.TenantID == nil || *trA.TenantID != w.pilotTenantID || trA.Source != "CLIENT" || trA.File == nil || strings.ContainsAny(*trA.File, `\/`) {
		return failr("tenant/sumber/nama berkas harus dari server & tersanitasi: %+v", trA)
	}
	if err := b.do(http.MethodPost, "/api/v1/data-transfers", tr("EXPORT", "COMPLETED", "tasks", 5, 5, 0), &trB); err != nil || trB.ID == "" {
		return failr("POST data-transfers (pilot2): %v", err)
	}
	for name, body := range map[string]map[string]any{
		"kind asing":      tr("DELETE", "COMPLETED", "devices", 1, 1, 0),
		"status asing":    tr("IMPORT", "OK", "devices", 1, 1, 0),
		"modul jahat":     tr("IMPORT", "COMPLETED", "devices; DROP TABLE x", 1, 1, 0),
		"jumlah negatif":  tr("IMPORT", "COMPLETED", "devices", -1, 0, 0),
		"sukses>total":    tr("IMPORT", "PARTIAL", "devices", 5, 5, 3),
		"COMPLETED+gagal": tr("IMPORT", "COMPLETED", "devices", 5, 3, 2),
	} {
		if st, bd := a.status(http.MethodPost, "/api/v1/data-transfers", body); st != 400 {
			return failr("transfer %s harus 400, dapat %d %s", name, st, bd)
		}
	}
	evilTr := tr("IMPORT", "COMPLETED", "devices", 1, 1, 0)
	evilTr["tenant_id"] = w.pilot2TenantID
	if st, _ := a.status(http.MethodPost, "/api/v1/data-transfers", evilTr); st != 400 {
		return failr("body transfer dengan tenant_id harus 400, dapat %d", st)
	}
	// IDOR
	if st, _ := b.status(http.MethodGet, "/api/v1/data-transfers/"+trA.ID, nil); st != 404 {
		return failr("IDOR: pilot2 membaca riwayat pilot harus 404, dapat %d", st)
	}
	if st, _ := a.status(http.MethodGet, "/api/v1/data-transfers/"+trB.ID, nil); st != 404 {
		return failr("IDOR: pilot membaca riwayat pilot2 harus 404, dapat %d", st)
	}
	type trList struct {
		Data []struct {
			ID       string  `json:"id"`
			TenantID *uint64 `json:"tenant_id"`
		} `json:"data"`
		Meta struct {
			Total int `json:"total"`
		} `json:"meta"`
	}
	var la trList
	// admin pilot mencoba mengintip tenant pilot2 lewat parameter -> harus diabaikan
	if err := a.do(http.MethodGet, fmt.Sprintf("/api/v1/data-transfers?tenant_id=%d&page_size=200", w.pilot2TenantID), nil, &la); err != nil {
		return failr("list data-transfers (pilot): %v", err)
	}
	foundA := false
	for _, r := range la.Data {
		if r.TenantID == nil || *r.TenantID != w.pilotTenantID {
			return failr("IDOR: riwayat tenant lain bocor di daftar pilot: %+v", r)
		}
		if r.ID == trA.ID {
			foundA = true
		}
		if r.ID == trB.ID {
			return failr("IDOR: riwayat pilot2 muncul di daftar pilot")
		}
	}
	if !foundA {
		return failr("riwayat milik sendiri tidak muncul di daftar")
	}
	var ls trList
	if err := su.do(http.MethodGet, "/api/v1/data-transfers?page_size=200", nil, &ls); err != nil {
		return failr("list data-transfers (superadmin): %v", err)
	}
	var sawA, sawB bool
	for _, r := range ls.Data {
		sawA = sawA || r.ID == trA.ID
		sawB = sawB || r.ID == trB.ID
	}
	if !sawA || !sawB {
		return failr("superadmin harus melihat riwayat semua tenant (A=%v B=%v)", sawA, sawB)
	}
	var lf trList
	if err := su.do(http.MethodGet, fmt.Sprintf("/api/v1/data-transfers?tenant_id=%d&page_size=200", w.pilot2TenantID), nil, &lf); err != nil {
		return failr("list data-transfers (superadmin, filter): %v", err)
	}
	for _, r := range lf.Data {
		if r.TenantID == nil || *r.TenantID != w.pilot2TenantID {
			return failr("filter tenant superadmin salah: %+v", r)
		}
	}
	if st, _ := su.status(http.MethodGet, "/api/v1/data-transfers/"+trB.ID, nil); st != 200 {
		return failr("superadmin membaca riwayat tenant mana pun harus 200, dapat %d", st)
	}
	// query rusak
	for _, q := range []string{"kind=HAPUS", "module=x%3Bdrop", "page=0", "tenant_id=abc"} {
		if st, _ := a.status(http.MethodGet, "/api/v1/data-transfers?"+q, nil); st != 400 {
			return failr("GET data-transfers?%s harus 400, dapat %d", q, st)
		}
	}
	return pass("menu: layout per pengguna (tenant lain/superadmin tak melihat; bentuk jahat 400); bug: tenant/pelapor dari token, lintas tenant 404, hanya SUPERADMIN ubah status; transfer: daftar dikunci ke tenant (tenant_id klien diabaikan), lintas tenant 404, SUPERADMIN lihat semua, input tak wajar 400")
}
