package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

type notifItem struct {
	ID       string  `json:"id"`
	Type     string  `json:"type"`
	Severity string  `json:"severity"`
	Title    string  `json:"title"`
	Body     string  `json:"body"`
	Link     *string `json:"link"`
	ReadAt   *string `json:"read_at"`
}

type notifList struct {
	Data []notifItem `json:"data"`
	Meta struct {
		Total  int `json:"total"`
		Unread int `json:"unread"`
		Page   int `json:"page"`
		Limit  int `json:"limit"`
	} `json:"meta"`
}

func notifFind(items []notifItem, title string) *notifItem {
	for i := range items {
		if items[i].Title == title {
			return &items[i]
		}
	}
	return nil
}

func unreadCount(c *Client) (int, error) {
	var out struct {
		Count int `json:"count"`
	}
	err := c.do(http.MethodGet, "/api/v1/notifications/unread-count", nil, &out)
	return out.Count, err
}

// S22 — notifikasi persisten end-to-end: SUPERADMIN mengumumkan ke tenant pilot -> hanya staf tenant pilot yang
// melihatnya (tenant pilot2 TIDAK), ENDUSER ditolak, ADMIN tidak boleh mengumumkan, link berbahaya ditolak,
// tandai-baca hanya berlaku untuk milik sendiri, dan preferensi (jenis wajib tak bisa dimatikan).
func scenarioNotify(ctx context.Context, w *World) result {
	run := w.runID
	title := "S22 pengumuman " + run
	body := "Uji notifikasi <b>S22</b> " + run // HTML harus diperlakukan sebagai teks biasa

	baseP, err := unreadCount(w.pilotAdmin)
	if err != nil {
		return failr("unread-count ADMIN pilot: %v", err)
	}
	baseP2, err := unreadCount(w.pilot2Admin)
	if err != nil {
		return failr("unread-count ADMIN pilot2: %v", err)
	}

	// 1. penolakan: ADMIN tidak boleh mengumumkan; input tak sah 400 (tidak menghabiskan kuota pengumuman)
	ann := "/api/v1/notifications/announce"
	good := map[string]any{"title": title, "body": body, "link": "/dashboard", "tenant_id": w.pilotTenantID, "email": false}
	if st, _ := w.pilotAdmin.status(http.MethodPost, ann, good); st != 403 {
		return failr("ADMIN mengumumkan harus 403, dapat %d", st)
	}
	for name, p := range map[string]map[string]any{
		"link protokol-relatif": {"title": "x", "body": "y", "link": "//evil.test/x"},
		"link absolut":          {"title": "x", "body": "y", "link": "https://evil.test"},
		"link skema":            {"title": "x", "body": "y", "link": "javascript:alert(1)"},
		"judul 161 karakter":    {"title": strings.Repeat("a", 161), "body": "y"},
		"judul kosong":          {"title": "", "body": "y"},
		"tenant tak ada":        {"title": "x", "body": "y", "tenant_id": 999999999},
	} {
		if st, b := w.super.status(http.MethodPost, ann, p); st != 400 {
			return failr("%s harus 400, dapat %d %s", name, st, b)
		}
	}

	// 2. ENDUSER: tidak punya akses ke notifikasi staf sama sekali
	user, pw := "s22-"+run, "S22-Pass-"+run+"-Qz7!"
	var created idResp
	if err := w.super.do(http.MethodPost, "/api/v1/users", map[string]any{
		"tenant_id": w.pilotTenantID, "username": user, "email": user + "@example.test", "password": pw,
		"full_name": "S22 Pelanggan", "role_codes": []string{"ENDUSER"},
	}, &created); err != nil {
		return failr("buat akun ENDUSER: %v", err)
	}
	defer func() { _ = w.super.do(http.MethodDelete, fmt.Sprintf("/api/v1/users/%d", created.ID), nil, nil) }()
	cust := NewClient(w.super.base, false)
	if err := cust.Login(user, pw); err != nil {
		return failr("login ENDUSER: %v", err)
	}
	for _, c := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/notifications"},
		{http.MethodGet, "/api/v1/notifications/unread-count"},
		{http.MethodGet, "/api/v1/notifications/preferences"},
	} {
		if st, _ := cust.status(c.method, c.path, nil); st != 403 {
			return failr("ENDUSER %s %s harus 403, dapat %d", c.method, c.path, st)
		}
	}
	if st, _ := cust.status(http.MethodPost, ann, good); st != 403 {
		return failr("ENDUSER mengumumkan harus 403, dapat %d", st)
	}

	// 3. SUPERADMIN mengumumkan ke tenant pilot
	var res struct {
		Recipients int `json:"recipients"`
	}
	if err := w.super.do(http.MethodPost, ann, good, &res); err != nil {
		return failr("announce: %v", err)
	}
	if res.Recipients < 1 {
		return failr("pengumuman tenant pilot harus punya >= 1 penerima, dapat %d", res.Recipients)
	}

	// 4. ADMIN pilot melihatnya (judul cocok, belum dibaca, unread naik); isi tetap teks, id = UUID
	var lp notifList
	if err := w.pilotAdmin.do(http.MethodGet, "/api/v1/notifications?page_size=100", nil, &lp); err != nil {
		return failr("list ADMIN pilot: %v", err)
	}
	n := notifFind(lp.Data, title)
	if n == nil {
		return failr("ADMIN pilot tidak menerima pengumuman %q (total %d)", title, lp.Meta.Total)
	}
	if n.Type != "ANNOUNCEMENT" || n.Severity != "INFO" || n.ReadAt != nil || n.Link == nil || *n.Link != "/dashboard" || n.Body != body {
		return failr("isi notifikasi tidak sesuai: %+v", *n)
	}
	if len(n.ID) != 36 || strings.Count(n.ID, "-") != 4 {
		return failr("id notifikasi harus UUID, dapat %q", n.ID)
	}
	if lp.Meta.Page != 1 || lp.Meta.Limit != 100 || lp.Meta.Unread < 1 {
		return failr("meta salah: %+v", lp.Meta)
	}
	gotP, err := unreadCount(w.pilotAdmin)
	if err != nil || gotP != baseP+1 {
		return failr("unread ADMIN pilot harus %d, dapat %d (%v)", baseP+1, gotP, err)
	}
	var unreadOnly notifList
	if err := w.pilotAdmin.do(http.MethodGet, "/api/v1/notifications?unread=true&page_size=100", nil, &unreadOnly); err != nil || notifFind(unreadOnly.Data, title) == nil {
		return failr("filter unread=true harus memuat pengumuman: %v", err)
	}

	// 5. isolasi tenant: ADMIN pilot2 TIDAK melihat apa pun dari pengumuman ini
	var lp2 notifList
	if err := w.pilot2Admin.do(http.MethodGet, "/api/v1/notifications?page_size=100", nil, &lp2); err != nil {
		return failr("list ADMIN pilot2: %v", err)
	}
	if notifFind(lp2.Data, title) != nil {
		return failr("ADMIN pilot2 TIDAK boleh menerima pengumuman tenant pilot")
	}
	if gotP2, err := unreadCount(w.pilot2Admin); err != nil || gotP2 != baseP2 {
		return failr("unread ADMIN pilot2 tidak boleh berubah (%d -> %d, %v)", baseP2, gotP2, err)
	}

	// 6. tandai-baca: milik orang lain diabaikan; id rusak 400; milik sendiri menurunkan hitungan
	read := "/api/v1/notifications/read"
	if st, b := w.pilot2Admin.status(http.MethodPost, read, map[string]any{"ids": []string{n.ID}}); st != 200 {
		return failr("mark-read id milik orang lain harus diabaikan diam-diam (204), dapat %d %s", st, b)
	}
	if got, _ := unreadCount(w.pilotAdmin); got != baseP+1 {
		return failr("pengguna lain tidak boleh bisa menandai baca notifikasi orang (unread %d, mau %d)", got, baseP+1)
	}
	if st, _ := w.pilotAdmin.status(http.MethodPost, read, map[string]any{"ids": []string{"bukan-uuid"}}); st != 400 {
		return failr("id bukan UUID harus 400, dapat %d", st)
	}
	if st, _ := w.pilotAdmin.status(http.MethodPost, read, map[string]any{}); st != 400 {
		return failr("tanpa ids/all harus 400, dapat %d", st)
	}
	if st, b := w.pilotAdmin.status(http.MethodPost, read, map[string]any{"ids": []string{n.ID}}); st >= 400 {
		return failr("mark-read milik sendiri: HTTP %d %s", st, b)
	}
	if got, _ := unreadCount(w.pilotAdmin); got != baseP {
		return failr("setelah mark-read unread harus kembali %d, dapat %d", baseP, got)
	}
	if st, _ := w.pilotAdmin.status(http.MethodPost, read, map[string]any{"all": true}); st >= 400 {
		return failr("mark-read all: HTTP %d", st)
	}
	if got, _ := unreadCount(w.pilotAdmin); got != 0 {
		return failr("setelah all=true unread harus 0, dapat %d", got)
	}

	// 7. preferensi: jenis wajib tak bisa dimatikan (400, atomik); jenis lain bisa
	var prefs struct {
		Data []struct {
			Type      string `json:"type"`
			InApp     bool   `json:"in_app"`
			Email     bool   `json:"email"`
			Mandatory bool   `json:"mandatory"`
			Severity  string `json:"severity"`
		} `json:"data"`
	}
	pp := "/api/v1/notifications/preferences"
	if err := w.pilotAdmin.do(http.MethodGet, pp, nil, &prefs); err != nil || len(prefs.Data) < 7 {
		return failr("daftar preferensi: %v (%d jenis)", err, len(prefs.Data))
	}
	var secOK, taskOK bool
	for _, p := range prefs.Data {
		switch p.Type {
		case "SECURITY_ALERT":
			secOK = p.Mandatory && p.InApp && p.Email
		case "TASK_FAILURES":
			taskOK = !p.Mandatory
		}
	}
	if !secOK || !taskOK {
		return failr("SECURITY_ALERT harus mandatory (in-app+email) dan TASK_FAILURES tidak: %+v", prefs.Data)
	}
	if st, b := w.pilotAdmin.status(http.MethodPut, pp, map[string]any{"items": []map[string]any{
		{"type": "TASK_FAILURES", "in_app": false, "email": false},
		{"type": "SECURITY_ALERT", "in_app": true, "email": false},
	}}); st != 400 {
		return failr("mematikan SECURITY_ALERT harus 400, dapat %d %s", st, b)
	}
	if st, _ := w.pilotAdmin.status(http.MethodPut, pp, map[string]any{"items": []map[string]any{{"type": "NGAWUR", "in_app": true, "email": true}}}); st != 400 {
		return failr("jenis tak dikenal harus 400, dapat %d", st)
	}
	if err := w.pilotAdmin.do(http.MethodGet, pp, nil, &prefs); err != nil {
		return failr("%v", err)
	}
	for _, p := range prefs.Data {
		if p.Type == "TASK_FAILURES" && !p.InApp {
			return failr("permintaan ditolak harus atomik: TASK_FAILURES tidak boleh ikut tersimpan")
		}
	}
	if st, b := w.pilotAdmin.status(http.MethodPut, pp, map[string]any{"items": []map[string]any{{"type": "TASK_FAILURES", "in_app": false, "email": true}}}); st >= 400 {
		return failr("mematikan in-app TASK_FAILURES: HTTP %d %s", st, b)
	}
	if err := w.pilotAdmin.do(http.MethodGet, pp, nil, &prefs); err != nil {
		return failr("%v", err)
	}
	for _, p := range prefs.Data {
		if p.Type == "TASK_FAILURES" && (p.InApp || !p.Email) {
			return failr("preferensi TASK_FAILURES tidak tersimpan: %+v", p)
		}
	}
	// pulihkan bawaan (akun uji dipakai skenario lain)
	_ = w.pilotAdmin.do(http.MethodPut, pp, map[string]any{"items": []map[string]any{{"type": "TASK_FAILURES", "in_app": true, "email": false}}}, nil)

	return pass("pengumuman SUPERADMIN → ADMIN pilot melihatnya (unread +1, id UUID, isi teks), pilot2 tidak; ENDUSER 403; ADMIN announce 403; link/judul tak sah & tenant tak ada 400; mark-read milik sendiri saja; jenis wajib tak bisa dimatikan (400 atomik)")
}
