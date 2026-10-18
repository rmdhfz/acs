package mysql

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"acs/internal/domain"
)

// Integrasi MariaDB NYATA (migrations/0033 harus sudah diterapkan): layout menu per pengguna (upsert/reset/hidup
// kembali), laporan bug, dan riwayat transfer — termasuk pemisahan data antar tenant/pengguna di level SQL.
// Dilewati tanpa ACS_TEST_DSN.

type consoleFixture struct {
	db           *sqlx.DB
	sfx          string
	t1, t2       uint64
	u1, u2, uSup uint64
}

func newConsoleFixture(t *testing.T) *consoleFixture {
	t.Helper()
	dsn := os.Getenv("ACS_TEST_DSN")
	if dsn == "" {
		t.Skip("ACS_TEST_DSN tidak diisi — test integrasi MariaDB dilewati")
	}
	db, err := Connect(dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	f := &consoleFixture{db: db, sfx: uuid.NewString()[:8]}
	ctx := context.Background()
	mkTenant := func(n string) uint64 {
		res, err := db.ExecContext(ctx, `INSERT INTO tenants (tenant_uuid, code, name) VALUES (?, ?, ?)`, uuid.NewString(), "it-c"+n+"-"+f.sfx, "IT Console "+n+" "+f.sfx)
		if err != nil {
			t.Fatalf("tenant: %v", err)
		}
		id, _ := res.LastInsertId()
		return uint64(id)
	}
	mkUser := func(n string, tenant *uint64) uint64 {
		res, err := db.ExecContext(ctx, `INSERT INTO users (user_uuid, tenant_id, username, email, password_hash, full_name) VALUES (?, ?, ?, ?, 'x', ?)`,
			uuid.NewString(), tenant, "itc-"+n+"-"+f.sfx, "itc-"+n+"-"+f.sfx+"@example.test", "IT "+n)
		if err != nil {
			t.Fatalf("user: %v", err)
		}
		id, _ := res.LastInsertId()
		return uint64(id)
	}
	f.t1, f.t2 = mkTenant("1"), mkTenant("2")
	f.u1, f.u2, f.uSup = mkUser("a", &f.t1), mkUser("b", &f.t2), mkUser("s", nil)
	t.Cleanup(func() {
		ids := []any{f.u1, f.u2, f.uSup}
		_, _ = db.Exec(`DELETE FROM user_menu_layouts WHERE user_id IN (?, ?, ?)`, ids...)
		_, _ = db.Exec(`DELETE FROM bug_reports WHERE reporter_id IN (?, ?, ?)`, ids...)
		_, _ = db.Exec(`DELETE FROM data_transfer_logs WHERE user_id IN (?, ?, ?)`, ids...)
		_, _ = db.Exec(`DELETE FROM users WHERE id IN (?, ?, ?)`, ids...)
		_, _ = db.Exec(`DELETE FROM tenants WHERE id IN (?, ?)`, f.t1, f.t2)
		_ = db.Close()
	})
	return f
}

func TestConsoleRepo_MenuLayout(t *testing.T) {
	f := newConsoleFixture(t)
	r := NewMenuLayoutRepository(f.db)
	ctx := context.Background()
	if _, err := r.Get(ctx, f.u1); err != domain.ErrNotFound {
		t.Fatalf("belum ada layout harus ErrNotFound, dapat %v", err)
	}
	if err := r.Upsert(ctx, f.u1, `{"version":1,"nodes":[],"hidden":[]}`); err != nil {
		t.Fatal(err)
	}
	if err := r.Upsert(ctx, f.u1, `{"version":1,"nodes":[{"type":"item","key":"a"}],"hidden":[]}`); err != nil {
		t.Fatalf("upsert kedua (update): %v", err)
	}
	got, err := r.Get(ctx, f.u1)
	if err != nil || got != `{"version":1,"nodes":[{"type":"item","key":"a"}],"hidden":[]}` {
		t.Fatalf("layout terakhir harus tersimpan: %q %v", got, err)
	}
	// terisolasi per pengguna
	if _, err := r.Get(ctx, f.u2); err != domain.ErrNotFound {
		t.Fatalf("pengguna lain tidak boleh melihat layout u1: %v", err)
	}
	if err := r.Reset(ctx, f.u1); err != nil {
		t.Fatal(err)
	}
	if err := r.Reset(ctx, f.u1); err != nil {
		t.Fatalf("reset harus idempoten: %v", err)
	}
	if _, err := r.Get(ctx, f.u1); err != domain.ErrNotFound {
		t.Fatalf("setelah reset harus ErrNotFound: %v", err)
	}
	// hidup kembali pada kunci unik yang sama (soft delete, bukan baris baru)
	if err := r.Upsert(ctx, f.u1, `{"version":1,"nodes":[],"hidden":["z"]}`); err != nil {
		t.Fatalf("upsert setelah reset: %v", err)
	}
	if got, err := r.Get(ctx, f.u1); err != nil || got == "" {
		t.Fatalf("layout harus hidup lagi: %v", err)
	}
	var n int
	if err := f.db.Get(&n, `SELECT COUNT(*) FROM user_menu_layouts WHERE user_id = ?`, f.u1); err != nil || n != 1 {
		t.Fatalf("harus tetap satu baris per pengguna: %d %v", n, err)
	}
}

func TestConsoleRepo_BugReports(t *testing.T) {
	f := newConsoleFixture(t)
	r := NewBugReportRepository(f.db)
	ctx := context.Background()
	mk := func(uid uint64, tid *uint64, title string, imp *uint64) *domain.BugReport {
		b, err := r.Create(ctx, domain.BugReportCreate{
			UUID: uuid.NewString(), TenantID: tid, ReporterID: uid, ImpersonatorID: imp, Title: title, Description: "deskripsi uji integrasi", Expected: "harapan uji integrasi",
			PagePath: "/x", UserAgent: "UA", AppVersion: "1.0", SeverityCode: "MAJOR",
		})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		return b
	}
	a := mk(f.u1, &f.t1, "bug A", nil)
	b := mk(f.u2, &f.t2, "bug B", nil)
	c := mk(f.uSup, nil, "bug C (platform)", nil)
	d := mk(f.u1, &f.t1, "bug D impersonate", &f.uSup)
	if a.Status != "OPEN" || a.Severity != "MAJOR" || a.TenantName == nil || a.ReporterEmail == "" {
		t.Fatalf("hasil join salah: %+v", a)
	}
	if !d.Impersonated || a.Impersonated {
		t.Fatalf("penanda impersonate salah: a=%v d=%v", a.Impersonated, d.Impersonated)
	}
	rows, total, err := r.List(ctx, domain.BugReportFilter{ReporterID: &f.u1}, domain.Pagination{})
	if err != nil || total != 2 || len(rows) != 2 {
		t.Fatalf("filter pelapor: total=%d err=%v", total, err)
	}
	for _, x := range rows {
		if x.ReporterID != f.u1 {
			t.Fatalf("baris pelapor lain bocor: %+v", x)
		}
	}
	if _, total, _ := r.List(ctx, domain.BugReportFilter{TenantID: &f.t2}, domain.Pagination{}); total != 1 {
		t.Fatalf("filter tenant 2: %d", total)
	}
	if err := r.UpdateStatus(ctx, b.UUID, "RESOLVED", strPtr("diperbaiki"), f.uSup); err != nil {
		t.Fatal(err)
	}
	got, _ := r.GetByUUID(ctx, b.UUID)
	if got.Status != "RESOLVED" || got.AdminNote == nil || *got.AdminNote != "diperbaiki" {
		t.Fatalf("status/catatan tidak tersimpan: %+v", got)
	}
	if _, total, _ := r.List(ctx, domain.BugReportFilter{ReporterID: &f.u2, Status: "RESOLVED"}, domain.Pagination{}); total != 1 {
		t.Fatalf("filter status: %d", total)
	}
	if err := r.UpdateStatus(ctx, uuid.NewString(), "RESOLVED", nil, f.uSup); err != domain.ErrNotFound {
		t.Fatalf("uuid tak ada harus ErrNotFound: %v", err)
	}
	if err := r.MarkEmailed(ctx, c.ID); err != nil {
		t.Fatal(err)
	}
	if n, _ := r.CountByReporterSince(ctx, f.u1, time.Now().Add(-time.Hour)); n != 2 {
		t.Fatalf("CountByReporterSince = %d, mau 2", n)
	}
	// dua kolom terpisah + tangkapan layar atomik dengan laporan
	if a.Expected != "harapan uji integrasi" || a.Description == a.Expected || a.HasScreenshot {
		t.Fatalf("kolom terjadi/harapan harus terpisah & tanpa gambar: %+v", a)
	}
	withShot, err := r.Create(ctx, domain.BugReportCreate{
		UUID: uuid.NewString(), TenantID: &f.t1, ReporterID: f.u1, Title: "bug bergambar", Description: "terjadi", Expected: "diharapkan", SeverityCode: "MINOR",
		Screenshot: &domain.BugScreenshot{ContentType: "image/png", Width: 10, Height: 10, SHA256: strings.Repeat("a", 64), Data: []byte("\x89PNG-data-uji")},
	})
	if err != nil || !withShot.HasScreenshot {
		t.Fatalf("laporan + gambar: %v %+v", err, withShot)
	}
	sc, err := r.GetScreenshot(ctx, withShot.ID)
	if err != nil || sc.ContentType != "image/png" || string(sc.Data) != "\x89PNG-data-uji" || sc.Width != 10 {
		t.Fatalf("GetScreenshot: %v %+v", err, sc)
	}
	if _, err := r.GetScreenshot(ctx, a.ID); err != domain.ErrNotFound {
		t.Fatalf("laporan tanpa gambar harus ErrNotFound: %v", err)
	}
	// uuid berisi payload SQL diperlakukan sebagai nilai biasa
	if _, err := r.GetByUUID(ctx, "x' OR '1'='1"); err != domain.ErrNotFound {
		t.Fatalf("payload SQL harus ErrNotFound: %v", err)
	}
	// FK: pelapor tidak ada ditolak
	if _, err := r.Create(ctx, domain.BugReportCreate{UUID: uuid.NewString(), ReporterID: 987654321, Title: "x", Description: "y", SeverityCode: "MINOR"}); err == nil {
		t.Fatal("pelapor tak ada harus gagal (FK)")
	}
}

func TestConsoleRepo_DataTransfers(t *testing.T) {
	f := newConsoleFixture(t)
	r := NewDataTransferRepository(f.db)
	ctx := context.Background()
	mk := func(uid uint64, tid *uint64, kind, status, module string, total, ok, bad uint32) *domain.DataTransferLog {
		d, err := r.Create(ctx, domain.DataTransferCreate{
			UUID: uuid.NewString(), TenantID: tid, UserID: uid, KindCode: kind, StatusCode: status, Module: module, Format: "csv",
			TotalRows: total, SuccessRows: ok, FailedRows: bad, Source: domain.TransferSourceClient,
		})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		return d
	}
	a := mk(f.u1, &f.t1, "IMPORT", "PARTIAL", "devices", 10, 7, 3)
	mk(f.u1, &f.t1, "EXPORT", "COMPLETED", "tasks", 5, 5, 0)
	b := mk(f.u2, &f.t2, "IMPORT", "FAILED", "users", 4, 0, 4)
	mk(f.uSup, nil, "EXPORT", "COMPLETED", "saas_report", 1, 1, 0)
	if a.Kind != "IMPORT" || a.Status != "PARTIAL" || a.TenantName == nil || a.UserName == "" || a.EmailedAt != nil {
		t.Fatalf("hasil join salah: %+v", a)
	}
	rows, total, err := r.List(ctx, domain.DataTransferFilter{TenantID: &f.t1}, domain.Pagination{})
	if err != nil || total != 2 {
		t.Fatalf("filter tenant 1: total=%d err=%v", total, err)
	}
	for _, x := range rows {
		if x.TenantID == nil || *x.TenantID != f.t1 {
			t.Fatalf("baris tenant lain bocor: %+v", x)
		}
	}
	if _, total, _ := r.List(ctx, domain.DataTransferFilter{TenantID: &f.t1, Kind: "EXPORT"}, domain.Pagination{}); total != 1 {
		t.Fatalf("filter kind: %d", total)
	}
	if _, total, _ := r.List(ctx, domain.DataTransferFilter{TenantID: &f.t1, Module: "devices"}, domain.Pagination{}); total != 1 {
		t.Fatalf("filter modul: %d", total)
	}
	if got, err := r.GetByUUID(ctx, b.UUID); err != nil || got.Module != "users" {
		t.Fatalf("get by uuid: %v", err)
	}
	if err := r.MarkEmailed(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if n, _ := r.CountEmailedByUserSince(ctx, f.u1, time.Now().Add(-time.Hour)); n != 1 {
		t.Fatalf("CountEmailedByUserSince = %d, mau 1", n)
	}
	if n, _ := r.CountEmailedByUserSince(ctx, f.u2, time.Now().Add(-time.Hour)); n != 0 {
		t.Fatalf("pengguna lain tidak boleh ikut terhitung: %d", n)
	}
	// kode kind/status tak dikenal -> subselect NULL -> NOT NULL gagal (bukan baris setengah jadi)
	if _, err := r.Create(ctx, domain.DataTransferCreate{UUID: uuid.NewString(), UserID: f.u1, KindCode: "NGAWUR", StatusCode: "COMPLETED", Module: "x", Format: "csv", Source: "CLIENT"}); err == nil {
		t.Fatal("kind tak dikenal harus gagal")
	}
}

func strPtr(s string) *string { return &s }
