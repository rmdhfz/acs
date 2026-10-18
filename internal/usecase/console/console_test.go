package console

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"acs/internal/domain"
	"acs/internal/usecase/notify"
	"acs/pkg/mailer"
)

// ---- fakes ----

type fakeActivity struct{}

func (fakeActivity) Record(context.Context, *domain.ActivityLog) error { return nil }
func (fakeActivity) ListByEntity(context.Context, string, uint64, domain.Pagination) ([]domain.ActivityLog, int, error) {
	return nil, 0, nil
}
func (fakeActivity) ListByTenant(context.Context, *uint64, domain.ActivityLogFilter, domain.Pagination) ([]domain.ActivityLog, int, error) {
	return nil, 0, nil
}

type fakeUsers map[uint64]*domain.User

func (f fakeUsers) GetByID(_ context.Context, id uint64) (*domain.User, error) {
	if u, ok := f[id]; ok {
		return u, nil
	}
	return nil, domain.ErrNotFound
}

type fakeTenants map[uint64]string

func (f fakeTenants) GetByID(_ context.Context, id uint64) (*domain.Tenant, error) {
	if n, ok := f[id]; ok {
		return &domain.Tenant{ID: id, Name: n}, nil
	}
	return nil, domain.ErrNotFound
}

type fakeMail struct {
	mu   sync.Mutex
	msgs []mailer.Message
}

func (m *fakeMail) Notify(msg mailer.Message) {
	m.mu.Lock()
	m.msgs = append(m.msgs, msg)
	m.mu.Unlock()
}

type fakeNotifier struct{ events []domain.NotificationEvent }

func (n *fakeNotifier) Notify(_ context.Context, ev domain.NotificationEvent) error {
	n.events = append(n.events, ev)
	return nil
}

type menuRepo struct{ m map[uint64]string }

func (r *menuRepo) Get(_ context.Context, id uint64) (string, error) {
	if v, ok := r.m[id]; ok {
		return v, nil
	}
	return "", domain.ErrNotFound
}
func (r *menuRepo) Upsert(_ context.Context, id uint64, j string) error { r.m[id] = j; return nil }
func (r *menuRepo) Reset(_ context.Context, id uint64) error            { delete(r.m, id); return nil }

type bugRepo struct {
	rows    []domain.BugReport
	emailed []uint64
	next    uint64
	shots   map[uint64]*domain.BugScreenshot
}

func (r *bugRepo) GetScreenshot(_ context.Context, id uint64) (*domain.BugScreenshot, error) {
	if sc, ok := r.shots[id]; ok {
		return sc, nil
	}
	return nil, domain.ErrNotFound
}

func (r *bugRepo) Create(_ context.Context, in domain.BugReportCreate) (*domain.BugReport, error) {
	r.next++
	b := domain.BugReport{
		ID: r.next, UUID: in.UUID, TenantID: in.TenantID, ReporterID: in.ReporterID, ReporterName: "pelapor", ReporterEmail: "p@x.id",
		Impersonated: in.ImpersonatorID != nil, Title: in.Title, Description: in.Description, Expected: in.Expected, HasScreenshot: in.Screenshot != nil, PagePath: in.PagePath,
		UserAgent: in.UserAgent, AppVersion: in.AppVersion, Severity: in.SeverityCode, Status: domain.BugStatusOpen, CreatedAt: time.Now(),
	}
	if in.TenantID != nil {
		n := "Tenant"
		b.TenantName = &n
	}
	r.rows = append(r.rows, b)
	if in.Screenshot != nil {
		if r.shots == nil {
			r.shots = map[uint64]*domain.BugScreenshot{}
		}
		r.shots[b.ID] = in.Screenshot
	}
	return &b, nil
}
func (r *bugRepo) GetByUUID(_ context.Context, id string) (*domain.BugReport, error) {
	for i := range r.rows {
		if r.rows[i].UUID == id {
			c := r.rows[i]
			return &c, nil
		}
	}
	return nil, domain.ErrNotFound
}
func (r *bugRepo) List(_ context.Context, f domain.BugReportFilter, _ domain.Pagination) ([]domain.BugReport, int, error) {
	out := []domain.BugReport{}
	for _, b := range r.rows {
		if f.ReporterID != nil && b.ReporterID != *f.ReporterID {
			continue
		}
		if f.TenantID != nil && (b.TenantID == nil || *b.TenantID != *f.TenantID) {
			continue
		}
		if f.Status != "" && b.Status != f.Status {
			continue
		}
		out = append(out, b)
	}
	return out, len(out), nil
}
func (r *bugRepo) UpdateStatus(_ context.Context, id, status string, note *string, _ uint64) error {
	for i := range r.rows {
		if r.rows[i].UUID == id {
			r.rows[i].Status, r.rows[i].AdminNote = status, note
			return nil
		}
	}
	return domain.ErrNotFound
}
func (r *bugRepo) MarkEmailed(_ context.Context, id uint64) error {
	r.emailed = append(r.emailed, id)
	return nil
}
func (r *bugRepo) CountByReporterSince(_ context.Context, id uint64, _ time.Time) (int, error) {
	n := 0
	for _, b := range r.rows {
		if b.ReporterID == id {
			n++
		}
	}
	return n, nil
}

type trRepo struct {
	rows    []domain.DataTransferLog
	emailed map[uint64]int
	next    uint64
}

func (r *trRepo) Create(_ context.Context, in domain.DataTransferCreate) (*domain.DataTransferLog, error) {
	r.next++
	d := domain.DataTransferLog{
		ID: r.next, UUID: in.UUID, TenantID: in.TenantID, UserID: in.UserID, UserName: "u", Impersonated: in.ImpersonatorID != nil,
		Kind: in.KindCode, Status: in.StatusCode, Module: in.Module, Format: in.Format, FileName: in.FileName,
		TotalRows: in.TotalRows, SuccessRows: in.SuccessRows, FailedRows: in.FailedRows, Detail: in.Detail, Source: in.Source, CreatedAt: time.Now(),
	}
	r.rows = append(r.rows, d)
	return &d, nil
}
func (r *trRepo) GetByUUID(_ context.Context, id string) (*domain.DataTransferLog, error) {
	for i := range r.rows {
		if r.rows[i].UUID == id {
			c := r.rows[i]
			return &c, nil
		}
	}
	return nil, domain.ErrNotFound
}
func (r *trRepo) List(_ context.Context, f domain.DataTransferFilter, _ domain.Pagination) ([]domain.DataTransferLog, int, error) {
	out := []domain.DataTransferLog{}
	for _, d := range r.rows {
		if f.TenantID != nil && (d.TenantID == nil || *d.TenantID != *f.TenantID) {
			continue
		}
		if f.Kind != "" && d.Kind != f.Kind {
			continue
		}
		if f.Module != "" && d.Module != f.Module {
			continue
		}
		out = append(out, d)
	}
	return out, len(out), nil
}
func (r *trRepo) MarkEmailed(_ context.Context, id uint64) error {
	for i := range r.rows {
		if r.rows[i].ID == id {
			r.emailed[r.rows[i].UserID]++
		}
	}
	return nil
}
func (r *trRepo) CountEmailedByUserSince(_ context.Context, id uint64, _ time.Time) (int, error) {
	return r.emailed[id], nil
}

func u64(v uint64) *uint64 { return &v }

var (
	superA  = domain.Actor{UserID: 1, Roles: []string{domain.RoleSuperadmin}}
	adminT1 = domain.Actor{UserID: 10, TenantID: u64(1), Roles: []string{domain.RoleAdmin}}
	nocT1   = domain.Actor{UserID: 11, TenantID: u64(1), Roles: []string{domain.RoleNOC}}
	adminT2 = domain.Actor{UserID: 20, TenantID: u64(2), Roles: []string{domain.RoleAdmin}}
	endT1   = domain.Actor{UserID: 12, TenantID: u64(1), Roles: []string{domain.RoleEndUser}}
	impT1   = domain.Actor{UserID: 10, TenantID: u64(1), Roles: []string{domain.RoleAdmin}, ImpersonatorID: 1}
)

func isInvalid(err error) bool { return errors.Is(err, domain.ErrInvalidInput) }

// ---- layout menu ----

func TestValidateMenuLayout(t *testing.T) {
	ok := `{"version":1,"nodes":[{"type":"item","key":"dashboard"},{"type":"group","id":"cfg","label":"Konfigurasi","collapsed":true,"children":["presets","tags"]}],"hidden":["files"]}`
	if _, err := ValidateMenuLayout([]byte(ok)); err != nil {
		t.Fatalf("layout sah ditolak: %v", err)
	}
	bad := map[string]string{
		"kosong":            ``,
		"bukan json":        `{`,
		"versi salah":       `{"version":2,"nodes":[],"hidden":[]}`,
		"field asing":       `{"version":1,"nodes":[],"hidden":[],"x":1}`,
		"tipe asing":        `{"version":1,"nodes":[{"type":"script","key":"a"}],"hidden":[]}`,
		"key tidak sah":     `{"version":1,"nodes":[{"type":"item","key":"A B"}],"hidden":[]}`,
		"key javascript":    `{"version":1,"nodes":[{"type":"item","key":"javascript:alert(1)"}],"hidden":[]}`,
		"key ganda":         `{"version":1,"nodes":[{"type":"item","key":"a"},{"type":"item","key":"a"}],"hidden":[]}`,
		"ganda lintas grup": `{"version":1,"nodes":[{"type":"item","key":"a"},{"type":"group","id":"g","label":"G","children":["a"]}],"hidden":[]}`,
		"grup tanpa label":  `{"version":1,"nodes":[{"type":"group","id":"g","label":"  ","children":[]}],"hidden":[]}`,
		"label kontrol":     `{"version":1,"nodes":[{"type":"group","id":"g","label":"a\u0000b","children":[]}],"hidden":[]}`,
		"label panjang":     `{"version":1,"nodes":[{"type":"group","id":"g","label":"` + strings.Repeat("x", 33) + `","children":[]}],"hidden":[]}`,
		"id grup ganda":     `{"version":1,"nodes":[{"type":"group","id":"g","label":"A","children":[]},{"type":"group","id":"g","label":"B","children":[]}],"hidden":[]}`,
		"item bawa label":   `{"version":1,"nodes":[{"type":"item","key":"a","label":"x"}],"hidden":[]}`,
		"hidden ganda":      `{"version":1,"nodes":[],"hidden":["a","a"]}`,
		"data tambahan":     ok + `{}`,
	}
	for name, body := range bad {
		if _, err := ValidateMenuLayout([]byte(body)); !isInvalid(err) {
			t.Errorf("%s: harus ditolak ErrInvalidInput, dapat %v", name, err)
		}
	}
	// terlalu banyak simpul / terlalu besar
	var nodes []string
	for i := 0; i < maxMenuNodes+1; i++ {
		nodes = append(nodes, `{"type":"item","key":"k`+strings.Repeat("a", i%5)+string(rune('a'+i%20))+strings.Repeat("b", i/20)+`"}`)
	}
	if _, err := ValidateMenuLayout([]byte(`{"version":1,"nodes":[` + strings.Join(nodes, ",") + `],"hidden":[]}`)); !isInvalid(err) {
		t.Errorf("simpul > batas harus ditolak: %v", err)
	}
	if _, err := ValidateMenuLayout([]byte(`{"version":1,"nodes":[],"hidden":[],"pad":"` + strings.Repeat("x", maxMenuJSONBytes) + `"}`)); !isInvalid(err) {
		t.Errorf("JSON > 8 KB harus ditolak: %v", err)
	}
}

func TestMenuLayout_PerPenggunaDanImpersonate(t *testing.T) {
	repo := &menuRepo{m: map[uint64]string{}}
	s := NewMenuService(repo)
	ctx := context.Background()
	body := []byte(`{"version":1,"nodes":[{"type":"item","key":"dashboard"}],"hidden":[]}`)
	if _, err := s.Put(ctx, adminT1, body); err != nil {
		t.Fatal(err)
	}
	// pengguna lain tidak melihat layout orang lain (tidak ada parameter pemilik sama sekali)
	if raw, err := s.Get(ctx, adminT2); err != nil || raw != nil {
		t.Fatalf("admin tenant lain harus kosong, dapat %s %v", raw, err)
	}
	if raw, _ := s.Get(ctx, adminT1); raw == nil {
		t.Fatal("pemilik harus melihat layout-nya")
	}
	// SUPERADMIN pun tidak punya jalur membaca layout orang lain: Get hanya milik actor sendiri
	if raw, _ := s.Get(ctx, superA); raw != nil {
		t.Fatal("superadmin tidak boleh melihat layout orang lain")
	}
	// sesi impersonate tidak boleh mengubah/mereset preferensi pengguna yang ditiru
	if _, err := s.Put(ctx, impT1, body); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("PUT dalam impersonate harus Forbidden: %v", err)
	}
	if err := s.Reset(ctx, impT1); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("DELETE dalam impersonate harus Forbidden: %v", err)
	}
	if err := s.Reset(ctx, adminT1); err != nil {
		t.Fatal(err)
	}
	if raw, _ := s.Get(ctx, adminT1); raw != nil {
		t.Fatal("setelah reset harus kosong")
	}
	var back MenuLayout
	norm, _ := s.Put(ctx, adminT1, body)
	if err := json.Unmarshal(norm, &back); err != nil || back.Version != 1 {
		t.Fatalf("hasil normalisasi rusak: %s", norm)
	}
}

// ---- laporan bug ----

func newBugSvc() (*BugService, *bugRepo, *fakeMail, *fakeNotifier) {
	repo := &bugRepo{}
	mail, notif := &fakeMail{}, &fakeNotifier{}
	s := NewBugService(repo, fakeActivity{}, fakeUsers{10: {ID: 10, Email: "adm1@x.id", FullName: "Adm Satu"}, 12: {ID: 12, Email: "end1@x.id"}}, fakeTenants{1: "T1"}, "ops@example.test", nil).
		WithNotifier(notif).WithEmail(mail, notify.NewBuilder("ACS", "https://acs.test"), true)
	return s, repo, mail, notif
}

func goodBug() CreateBugInput {
	return CreateBugInput{Title: "Tombol simpan tidak bekerja", Description: "Saat klik simpan di halaman tag tidak ada reaksi sama sekali.", Expected: "Seharusnya tag tersimpan dan muncul di daftar.", Severity: "MAJOR", PagePath: "/tags?x=1#y", UserAgent: "Mozilla/5.0", AppVersion: "1.0.0"}
}

func TestBugReport_DisimpanDanDiemailkan(t *testing.T) {
	s, repo, mail, notif := newBugSvc()
	rep, err := s.Create(context.Background(), adminT1, goodBug())
	if err != nil {
		t.Fatal(err)
	}
	if rep.PagePath != "/tags" || rep.AppVersion != "1.0.0" || rep.Severity != "MAJOR" {
		t.Fatalf("konteks salah: %+v", rep)
	}
	if rep.TenantID == nil || *rep.TenantID != 1 || rep.ReporterID != adminT1.UserID {
		t.Fatalf("tenant/pelapor harus dari token: %+v", rep)
	}
	if rep.ReporterEmail != "" {
		t.Fatal("email pelapor tidak boleh kembali ke pelapor")
	}
	if len(mail.msgs) != 1 || mail.msgs[0].To[0] != "ops@example.test" || len(repo.emailed) != 1 {
		t.Fatalf("email ke operator tidak terkirim/ditandai: %+v %v", mail.msgs, repo.emailed)
	}
	if strings.ContainsAny(mail.msgs[0].Subject, "\r\n") || !strings.Contains(mail.msgs[0].Subject, "SEDANG") {
		t.Fatalf("subjek email salah: %q", mail.msgs[0].Subject)
	}
	if len(notif.events) != 1 || notif.events[0].Audience.Kind != domain.AudienceSuperadmins || !notif.events[0].NoEmail {
		t.Fatalf("notifikasi in-app ke superadmin tidak sesuai: %+v", notif.events)
	}
}

func TestBugReport_EmailTidakMemuatHTMLMentah(t *testing.T) {
	s, _, mail, _ := newBugSvc()
	in := goodBug()
	in.Title = "<script>alert(1)</script> judul"
	in.Description = "<img src=x onerror=alert(1)> deskripsi panjang cukup"
	if _, err := s.Create(context.Background(), adminT1, in); err != nil {
		t.Fatal(err)
	}
	html := mail.msgs[0].HTML
	if strings.Contains(html, "<script>") || strings.Contains(html, "<img src=x") {
		t.Fatal("input pengguna harus di-escape di email HTML")
	}
}

func TestBugReport_Validasi(t *testing.T) {
	s, _, _, _ := newBugSvc()
	ctx := context.Background()
	cases := map[string]func(*CreateBugInput){
		"judul pendek":      func(i *CreateBugInput) { i.Title = "abc" },
		"judul spasi":       func(i *CreateBugInput) { i.Title = "          " },
		"judul kontrol":     func(i *CreateBugInput) { i.Title = "judul\x00bug" },
		"judul newline":     func(i *CreateBugInput) { i.Title = "judul\nbug panjang" },
		"judul panjang":     func(i *CreateBugInput) { i.Title = strings.Repeat("a", bugTitleMax+1) },
		"deskripsi pendek":  func(i *CreateBugInput) { i.Description = "pendek" },
		"deskripsi panjang": func(i *CreateBugInput) { i.Description = strings.Repeat("a", bugDescMax+1) },
		"deskripsi kontrol": func(i *CreateBugInput) { i.Description = "deskripsi\x07 dengan karakter bel" },
		"dampak asing":      func(i *CreateBugInput) { i.Severity = "CRITICAL" },
	}
	for name, mut := range cases {
		in := goodBug()
		mut(&in)
		if _, err := s.Create(ctx, adminT1, in); !isInvalid(err) {
			t.Errorf("%s: harus ErrInvalidInput, dapat %v", name, err)
		}
	}
	// konteks tidak sah dikosongkan, bukan menolak laporan
	in := goodBug()
	in.PagePath, in.AppVersion, in.UserAgent = "http://evil.test/x", "v1 <b>", "UA\x00\x01bad"
	rep, err := s.Create(ctx, adminT1, in)
	if err != nil {
		t.Fatal(err)
	}
	if rep.PagePath != "" || rep.AppVersion != "" || strings.ContainsAny(rep.UserAgent, "\x00\x01") {
		t.Fatalf("konteks tak sah harus dibersihkan: %+v", rep)
	}
}

func TestBugReport_BatasLaju(t *testing.T) {
	s, _, _, _ := newBugSvc()
	for i := 0; i < maxBugReportsPerHour; i++ {
		if _, err := s.Create(context.Background(), adminT1, goodBug()); err != nil {
			t.Fatalf("laporan ke-%d: %v", i+1, err)
		}
	}
	if _, err := s.Create(context.Background(), adminT1, goodBug()); !errors.Is(err, domain.ErrQuotaExceeded) {
		t.Fatalf("laporan ke-%d harus kena batas laju: %v", maxBugReportsPerHour+1, err)
	}
	// pelapor lain tidak terpengaruh
	if _, err := s.Create(context.Background(), adminT2, goodBug()); err != nil {
		t.Fatalf("pelapor lain tidak boleh ikut dibatasi: %v", err)
	}
}

// IDOR: pelapor hanya melihat miliknya; laporan orang lain = NotFound (bukan Forbidden); hanya SUPERADMIN yang menangani.
func TestBugReport_TanpaIDOR(t *testing.T) {
	s, _, _, _ := newBugSvc()
	ctx := context.Background()
	a, _ := s.Create(ctx, adminT1, goodBug())
	b, _ := s.Create(ctx, adminT2, goodBug())
	e, _ := s.Create(ctx, endT1, goodBug())

	// baca by uuid: orang lain -> NotFound, bahkan sesama tenant / sesama ADMIN
	for name, c := range map[string]struct {
		actor domain.Actor
		id    string
	}{
		"admin T2 baca laporan T1":      {adminT2, a.UUID},
		"admin T1 baca laporan T2":      {adminT1, b.UUID},
		"noc T1 baca laporan admin":     {nocT1, a.UUID},
		"admin T1 baca laporan enduser": {adminT1, e.UUID},
		"enduser baca laporan admin":    {endT1, a.UUID},
		"uuid ngawur":                   {adminT1, "1' OR '1'='1"},
	} {
		if _, err := s.Get(ctx, c.actor, c.id); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("%s: harus NotFound, dapat %v", name, err)
		}
	}
	if got, err := s.Get(ctx, adminT1, a.UUID); err != nil || got.UUID != a.UUID {
		t.Fatalf("pelapor harus bisa membaca miliknya: %v", err)
	}
	if got, err := s.Get(ctx, superA, b.UUID); err != nil || got.ReporterEmail == "" {
		t.Fatalf("superadmin membaca semua termasuk email pelapor: %v %+v", err, got)
	}

	// daftar: filter tenant dari klien diabaikan untuk non-superadmin
	rows, total, _ := s.List(ctx, adminT1, u64(2), "", domain.Pagination{})
	if total != 1 || rows[0].UUID != a.UUID {
		t.Fatalf("admin T1 harus hanya melihat miliknya (filter tenant=2 diabaikan): %+v", rows)
	}
	for _, r := range rows {
		if r.ReporterEmail != "" {
			t.Fatal("email pelapor tidak boleh ada di daftar non-superadmin")
		}
	}
	if _, total, _ := s.List(ctx, superA, nil, "", domain.Pagination{}); total != 3 {
		t.Fatalf("superadmin melihat semua (3), dapat %d", total)
	}
	if _, total, _ := s.List(ctx, superA, u64(2), "", domain.Pagination{}); total != 1 {
		t.Fatalf("superadmin dengan filter tenant=2 harus 1, dapat %d", total)
	}
	// ubah status: hanya SUPERADMIN (admin & sesi impersonate ditolak)
	note := "sedang dicek"
	for name, actor := range map[string]domain.Actor{"admin": adminT1, "noc": nocT1, "enduser": endT1, "impersonate": impT1} {
		if _, err := s.UpdateStatus(ctx, actor, a.UUID, domain.BugStatusResolved, &note); !errors.Is(err, domain.ErrForbidden) {
			t.Errorf("%s mengubah status harus Forbidden: %v", name, err)
		}
	}
	got, err := s.UpdateStatus(ctx, superA, a.UUID, domain.BugStatusInProgress, &note)
	if err != nil || got.Status != domain.BugStatusInProgress {
		t.Fatalf("superadmin harus bisa: %v %+v", err, got)
	}
	if _, err := s.UpdateStatus(ctx, superA, a.UUID, "HAPUS", nil); !isInvalid(err) {
		t.Fatalf("status asing harus ditolak: %v", err)
	}
	if _, _, err := s.List(ctx, adminT1, nil, "BUKAN_STATUS", domain.Pagination{}); !isInvalid(err) {
		t.Fatalf("filter status asing harus ditolak: %v", err)
	}
}

// ---- riwayat transfer ----

func newTransferSvc(mailOn bool) (*TransferService, *trRepo, *fakeMail) {
	repo := &trRepo{emailed: map[uint64]int{}}
	mail := &fakeMail{}
	users := fakeUsers{
		1: {ID: 1, Email: "root@x.id", FullName: "Root"}, 10: {ID: 10, Email: "adm1@x.id", FullName: "Adm 1"},
		11: {ID: 11, Email: "noc1@x.id"}, 20: {ID: 20, Email: "adm2@x.id"},
	}
	s := NewTransferService(repo, fakeActivity{}, users, nil).WithEmail(mail, notify.NewBuilder("ACS", "https://acs.test"), mailOn)
	return s, repo, mail
}

func goodTransfer() RecordInput {
	return RecordInput{Kind: "IMPORT", Status: "PARTIAL", Module: "devices", Format: "csv", FileName: `C:\data\perangkat.csv`, TotalRows: 10, SuccessRows: 7, FailedRows: 3, Detail: "3 baris duplikat"}
}

func TestTransfer_CatatDanEmailPelaku(t *testing.T) {
	s, repo, mail := newTransferSvc(true)
	rec, err := s.RecordClient(context.Background(), adminT1, goodTransfer())
	if err != nil {
		t.Fatal(err)
	}
	if rec.TenantID == nil || *rec.TenantID != 1 || rec.UserID != 10 || rec.Source != domain.TransferSourceClient {
		t.Fatalf("tenant/pelaku/sumber harus dari token & server: %+v", rec)
	}
	if rec.FileName == nil || *rec.FileName != "perangkat.csv" {
		t.Fatalf("nama berkas harus basename tersanitasi: %v", rec.FileName)
	}
	if len(mail.msgs) != 1 || mail.msgs[0].To[0] != "adm1@x.id" || repo.emailed[10] != 1 {
		t.Fatalf("email harus ke pelaku sendiri: %+v", mail.msgs)
	}
	// server-side mencatat sumber SERVER
	rs, err := s.RecordServer(context.Background(), adminT1, goodTransfer())
	if err != nil || rs.Source != domain.TransferSourceServer {
		t.Fatalf("RecordServer: %v %+v", err, rs)
	}
}

func TestTransfer_TanpaEmailBilaImpersonateAtauSMTPMatiAtauBatas(t *testing.T) {
	s, _, mail := newTransferSvc(true)
	if _, err := s.RecordClient(context.Background(), impT1, goodTransfer()); err != nil {
		t.Fatal(err)
	}
	if len(mail.msgs) != 0 {
		t.Fatal("sesi impersonate tidak boleh mengirim email ke tenant")
	}
	off, _, mailOff := newTransferSvc(false)
	if _, err := off.RecordClient(context.Background(), adminT1, goodTransfer()); err != nil {
		t.Fatal(err)
	}
	if len(mailOff.msgs) != 0 {
		t.Fatal("SMTP mati: tidak ada email, riwayat tetap tercatat")
	}
	// batas email per jam: proses ke-21 tetap tercatat tapi tanpa email
	lim, repo, mailLim := newTransferSvc(true)
	for i := 0; i < maxTransferEmailsHour+3; i++ {
		if _, err := lim.RecordClient(context.Background(), adminT1, goodTransfer()); err != nil {
			t.Fatal(err)
		}
	}
	if len(mailLim.msgs) != maxTransferEmailsHour || len(repo.rows) != maxTransferEmailsHour+3 {
		t.Fatalf("email dibatasi %d tapi semua tercatat: email=%d rows=%d", maxTransferEmailsHour, len(mailLim.msgs), len(repo.rows))
	}
}

func TestTransfer_Validasi(t *testing.T) {
	s, _, _ := newTransferSvc(false)
	ctx := context.Background()
	mut := map[string]func(*RecordInput){
		"kind asing":          func(i *RecordInput) { i.Kind = "DELETE" },
		"status asing":        func(i *RecordInput) { i.Status = "OK" },
		"modul berspasi":      func(i *RecordInput) { i.Module = "devices; DROP" },
		"modul huruf besar":   func(i *RecordInput) { i.Module = "Devices" },
		"format asing":        func(i *RecordInput) { i.Format = "exe" },
		"jumlah negatif":      func(i *RecordInput) { i.TotalRows = -1 },
		"jumlah raksasa":      func(i *RecordInput) { i.TotalRows = maxTransferRows + 1 },
		"sukses+gagal>total":  func(i *RecordInput) { i.SuccessRows, i.FailedRows, i.TotalRows = 8, 8, 10 },
		"COMPLETED ada gagal": func(i *RecordInput) { i.Status = "COMPLETED" },
		"PARTIAL tanpa gagal": func(i *RecordInput) { i.FailedRows, i.SuccessRows, i.TotalRows = 0, 10, 10 },
		"FAILED ada sukses":   func(i *RecordInput) { i.Status = "FAILED" },
		"nama berkas kontrol": func(i *RecordInput) { i.FileName = "a\x00b.csv" },
		"detail panjang":      func(i *RecordInput) { i.Detail = strings.Repeat("x", 501) },
		"detail newline":      func(i *RecordInput) { i.Detail = "baris1\nbaris2" },
	}
	for name, m := range mut {
		in := goodTransfer()
		m(&in)
		if _, err := s.RecordClient(ctx, adminT1, in); !isInvalid(err) {
			t.Errorf("%s: harus ErrInvalidInput, dapat %v", name, err)
		}
	}
	if _, err := s.RecordClient(ctx, endT1, goodTransfer()); !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("ENDUSER tidak boleh mencatat: %v", err)
	}
	if _, err := s.RecordClient(ctx, domain.Actor{}, goodTransfer()); !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("tanpa identitas harus Forbidden: %v", err)
	}
}

// IDOR: tenant hanya melihat baris tenantnya; SUPERADMIN melihat semua; lintas tenant = NotFound.
func TestTransfer_TanpaIDOR(t *testing.T) {
	s, _, _ := newTransferSvc(false)
	ctx := context.Background()
	t1, _ := s.RecordClient(ctx, adminT1, goodTransfer())
	t1b, _ := s.RecordClient(ctx, nocT1, RecordInput{Kind: "EXPORT", Status: "COMPLETED", Module: "tasks", Format: "csv", TotalRows: 5, SuccessRows: 5})
	t2, _ := s.RecordClient(ctx, adminT2, goodTransfer())
	sa, _ := s.RecordClient(ctx, superA, RecordInput{Kind: "EXPORT", Status: "COMPLETED", Module: "saas_report", Format: "csv", TotalRows: 1, SuccessRows: 1})
	if sa.TenantID != nil {
		t.Fatalf("superadmin tanpa tenant harus tercatat tenant NULL: %+v", sa)
	}

	rows, total, err := s.List(ctx, adminT1, nil, "", "", domain.Pagination{})
	if err != nil || total != 2 {
		t.Fatalf("admin T1 harus melihat 2 baris tenant-nya: %d %v", total, err)
	}
	for _, r := range rows {
		if r.TenantID == nil || *r.TenantID != 1 {
			t.Fatalf("baris tenant lain bocor: %+v", r)
		}
	}
	// parameter tenant_id dari klien diabaikan untuk non-superadmin (tidak bisa mengintip tenant 2)
	if _, total, _ := s.List(ctx, adminT1, u64(2), "", "", domain.Pagination{}); total != 2 {
		t.Fatalf("tenant_id=2 dari admin T1 harus diabaikan, dapat total %d", total)
	}
	if _, total, _ := s.List(ctx, nocT1, nil, "", "", domain.Pagination{}); total != 2 {
		t.Fatalf("staf lain tenant yang sama melihat riwayat tenant: %d", total)
	}
	if _, total, _ := s.List(ctx, superA, nil, "", "", domain.Pagination{}); total != 4 {
		t.Fatalf("superadmin melihat semua (4), dapat %d", total)
	}
	if _, total, _ := s.List(ctx, superA, u64(2), "", "", domain.Pagination{}); total != 1 {
		t.Fatalf("superadmin filter tenant=2 harus 1, dapat %d", total)
	}
	if _, total, _ := s.List(ctx, adminT1, nil, "EXPORT", "", domain.Pagination{}); total != 1 {
		t.Fatalf("filter kind harus bekerja: %d", total)
	}
	// ENDUSER & tanpa tenant (bukan superadmin) ditolak
	if _, _, err := s.List(ctx, endT1, nil, "", "", domain.Pagination{}); !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("ENDUSER harus Forbidden: %v", err)
	}
	if _, _, err := s.List(ctx, domain.Actor{UserID: 99, Roles: []string{domain.RoleAdmin}}, nil, "", "", domain.Pagination{}); !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("ADMIN tanpa tenant harus Forbidden: %v", err)
	}
	// baca by uuid
	if _, err := s.Get(ctx, adminT2, t1.UUID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("T2 membaca baris T1 harus NotFound: %v", err)
	}
	if _, err := s.Get(ctx, adminT1, t2.UUID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("T1 membaca baris T2 harus NotFound: %v", err)
	}
	if _, err := s.Get(ctx, adminT1, sa.UUID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("T1 membaca baris platform (tenant NULL) harus NotFound: %v", err)
	}
	if _, err := s.Get(ctx, adminT1, "bukan-uuid'--"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("uuid ngawur harus NotFound: %v", err)
	}
	if got, err := s.Get(ctx, nocT1, t1b.UUID); err != nil || got.UUID != t1b.UUID {
		t.Errorf("tenant yang sama boleh membaca: %v", err)
	}
	if got, err := s.Get(ctx, superA, t2.UUID); err != nil || got.UUID != t2.UUID {
		t.Errorf("superadmin boleh membaca semua: %v", err)
	}
	if _, _, err := s.List(ctx, adminT1, nil, "HAPUS", "", domain.Pagination{}); !isInvalid(err) {
		t.Errorf("kind asing harus ditolak: %v", err)
	}
	if _, _, err := s.List(ctx, adminT1, nil, "", "x; DROP", domain.Pagination{}); !isInvalid(err) {
		t.Errorf("module tak sah harus ditolak: %v", err)
	}
}
