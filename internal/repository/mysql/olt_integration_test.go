package mysql

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"

	"acs/internal/domain"
)

// Integrasi MariaDB NYATA untuk OLT Controller (migrations/0044): join status, kredensial biner, upsert per (olt, serial) dengan
// first_seen_at/uuid dipertahankan, pemotongan batch, revive baris yang dipangkas, prune RESOLVED, dan cascade soft delete.
// Dilewati tanpa ACS_TEST_DSN.
func TestOLTRepository_Integration(t *testing.T) {
	dsn := os.Getenv("ACS_TEST_DSN")
	if dsn == "" {
		t.Skip("ACS_TEST_DSN tidak diisi — test integrasi MariaDB dilewati")
	}
	db, err := Connect(dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()
	repo := NewOLTRepository(db)
	refs := NewRefRepository(db)

	pollID := func(code string) uint64 {
		r, err := refs.GetByCode(ctx, domain.RefTableOLTPollStatus, code)
		if err != nil {
			t.Fatalf("ref poll %s: %v", code, err)
		}
		return r.ID
	}
	onuID := func(code string) uint64 {
		r, err := refs.GetByCode(ctx, domain.RefTableONUDiscoveryStatus, code)
		if err != nil {
			t.Fatalf("ref onu %s: %v", code, err)
		}
		return r.ID
	}
	pending, ignored, resolved := onuID(domain.ONUStatusPending), onuID(domain.ONUStatusIgnored), onuID(domain.ONUStatusResolved)

	// tenant uji (+ pembersihan fisik data uji, seperti tes integrasi lain)
	sfx := uuid.NewString()[:8]
	tn := &domain.Tenant{TenantUUID: uuid.NewString(), Code: "OLT" + sfx, Name: "it-olt", IsActive: true}
	if err := NewTenantRepository(db).Create(ctx, tn); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE n FROM olt_unregistered_onus n JOIN olts o ON o.id = n.olt_id WHERE o.tenant_id = ?`, tn.ID)
		_, _ = db.Exec(`DELETE FROM olts WHERE tenant_id = ?`, tn.ID)
		_, _ = db.Exec(`DELETE FROM tenants WHERE id = ?`, tn.ID)
	})

	// ---- OLT: create / get / join status / kredensial biner ----
	user := "operator"
	notes := "gudang"
	o := &domain.OLT{
		UUID: uuid.NewString(), TenantID: tn.ID, Name: "OLT 100%_A", DriverCode: "sim", Host: "10.1.1.1", SNMPPort: 161, SSHPort: 22,
		SNMPCommunityEnc: []byte{0x00, 0xff, 0x10, 0x80}, SSHUsername: &user, SSHPasswordEnc: []byte{0x01, 0x02, 0x00}, Notes: &notes,
		IsActive: true, PollStatusID: pollID(domain.OLTPollUnknown),
	}
	if err := repo.Create(ctx, o); err != nil || o.ID == 0 {
		t.Fatalf("Create: %v", err)
	}
	got, err := repo.GetByUUID(ctx, o.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if got.PollStatus != domain.OLTPollUnknown || got.TenantID != tn.ID || got.Name != o.Name || got.SNMPPort != 161 || !got.IsActive ||
		string(got.SNMPCommunityEnc) != string(o.SNMPCommunityEnc) || string(got.SSHPasswordEnc) != string(o.SSHPasswordEnc) ||
		got.SSHUsername == nil || *got.SSHUsername != "operator" || got.LastPollAt != nil {
		t.Fatalf("GetByUUID: %+v", got)
	}
	if _, err := repo.GetByUUID(ctx, uuid.NewString()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("uuid tak ada: %v", err)
	}
	// uuid ganda ditolak (unik)
	dup := *o
	dup.ID = 0
	if err := repo.Create(ctx, &dup); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("uuid ganda harus ErrConflict: %v", err)
	}

	// ---- List: filter tenant, pencarian dengan karakter LIKE ----
	other := &domain.OLT{UUID: uuid.NewString(), TenantID: tn.ID, Name: "Zeta", DriverCode: "sim", Host: "10.1.1.2", SNMPPort: 161, SSHPort: 22, IsActive: true, PollStatusID: pollID(domain.OLTPollUnknown)}
	if err := repo.Create(ctx, other); err != nil {
		t.Fatal(err)
	}
	rows, total, err := repo.List(ctx, domain.OLTFilter{TenantID: &tn.ID}, domain.Pagination{})
	if err != nil || total != 2 || len(rows) != 2 || rows[0].Name != "OLT 100%_A" {
		t.Fatalf("List: %d %+v %v", total, rows, err)
	}
	if rows, total, _ := repo.List(ctx, domain.OLTFilter{TenantID: &tn.ID, Search: "100%"}, domain.Pagination{}); total != 1 || rows[0].UUID != o.UUID {
		t.Fatalf("'%%' harus dicari literal, bukan wildcard: %d", total)
	}
	if _, total, _ := repo.List(ctx, domain.OLTFilter{TenantID: &tn.ID, Search: "10.1.1.2"}, domain.Pagination{}); total != 1 {
		t.Fatalf("cari host: %d", total)
	}
	otherTenant := tn.ID + 1_000_000
	if _, total, _ := repo.List(ctx, domain.OLTFilter{TenantID: &otherTenant}, domain.Pagination{}); total != 0 {
		t.Fatalf("tenant lain tidak boleh bocor: %d", total)
	}

	// ---- Update: nilai sama tetap nil (RowsAffected 0 bukan "tidak ada"), nilai baru tersimpan, id tak ada → NotFound ----
	if err := repo.Update(ctx, got); err != nil {
		t.Fatalf("Update tanpa perubahan: %v", err)
	}
	got.Name, got.Host, got.IsActive, got.SNMPCommunityEnc, got.Notes = "Baru", "10.1.1.9", false, nil, nil
	if err := repo.Update(ctx, got); err != nil {
		t.Fatal(err)
	}
	if g, _ := repo.GetByUUID(ctx, o.UUID); g.Name != "Baru" || g.Host != "10.1.1.9" || g.IsActive || len(g.SNMPCommunityEnc) != 0 || g.Notes != nil {
		t.Fatalf("setelah Update: %+v", g)
	}
	ghost := *got
	ghost.ID = 987654321
	if err := repo.Update(ctx, &ghost); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Update id tak ada: %v", err)
	}

	// ---- RecordPoll: identitas hanya diganti bila dibawa (COALESCE) ----
	if err := repo.RecordPoll(ctx, o.ID, domain.OLTPollResult{StatusID: pollID(domain.OLTPollOK), Info: &domain.OLTInfo{Name: "OLT-X", Version: "v9"}}); err != nil {
		t.Fatal(err)
	}
	msg := "OLT tidak dapat dijangkau"
	if err := repo.RecordPoll(ctx, o.ID, domain.OLTPollResult{StatusID: pollID(domain.OLTPollUnreachable), Error: &msg}); err != nil {
		t.Fatal(err)
	}
	g, _ := repo.GetByUUID(ctx, o.UUID)
	if g.PollStatus != domain.OLTPollUnreachable || g.LastPollError == nil || *g.LastPollError != msg || g.LastPollAt == nil ||
		g.ReportedName == nil || *g.ReportedName != "OLT-X" || g.ReportedVersion == nil || *g.ReportedVersion != "v9" {
		t.Fatalf("RecordPoll: %+v", g)
	}
	if err := repo.RecordPoll(ctx, o.ID, domain.OLTPollResult{StatusID: pollID(domain.OLTPollOK)}); err != nil {
		t.Fatal(err)
	}
	if g, _ := repo.GetByUUID(ctx, o.UUID); g.LastPollError != nil || g.ReportedName == nil {
		t.Fatalf("galat dibersihkan saat sukses, identitas tetap: %+v", g)
	}

	// ---- ONU: upsert baru, lalu upsert ulang mempertahankan uuid & first_seen_at ----
	t0 := time.Now().Add(-48 * time.Hour).Truncate(time.Millisecond)
	t1 := time.Now().Truncate(time.Millisecond)
	typ := "TIPE-A"
	first := []domain.DiscoveredONU{
		{UUID: uuid.NewString(), PonPort: "1/1", SerialNumber: "S-A", ONUType: &typ, StatusID: pending, FirstSeenAt: t0, LastSeenAt: t0},
		{UUID: uuid.NewString(), PonPort: "1/2", SerialNumber: "S-B", StatusID: pending, FirstSeenAt: t0, LastSeenAt: t0},
	}
	if err := repo.UpsertONUs(ctx, o.ID, first); err != nil {
		t.Fatal(err)
	}
	second := []domain.DiscoveredONU{
		{UUID: uuid.NewString(), PonPort: "1/9", SerialNumber: "S-A", StatusID: ignored, FirstSeenAt: t1, LastSeenAt: t1}, // ada: uuid & first_seen lama dipertahankan
	}
	if err := repo.UpsertONUs(ctx, o.ID, second); err != nil {
		t.Fatalf("upsert ulang: %v", err)
	}
	onus, err := repo.ListONUsByOLT(ctx, o.ID)
	if err != nil || len(onus) != 2 {
		t.Fatalf("ListONUsByOLT: %d %v", len(onus), err)
	}
	var a domain.DiscoveredONU
	for _, n := range onus {
		if n.SerialNumber == "S-A" {
			a = n
		}
	}
	near := func(x, y time.Time) bool { d := x.Sub(y); return d < 2*time.Millisecond && d > -2*time.Millisecond }
	if a.UUID != first[0].UUID || !near(a.FirstSeenAt, t0) || !near(a.LastSeenAt, t1) || a.PonPort != "1/9" || a.Status != domain.ONUStatusIgnored ||
		a.OLTUUID != o.UUID || a.OLTName != "Baru" || a.TenantID != tn.ID || a.ONUType != nil {
		t.Fatalf("upsert ulang S-A: %+v", a)
	}

	// ---- batch besar melintasi batas chunk (200) ----
	big := make([]domain.DiscoveredONU, 450)
	for i := range big {
		big[i] = domain.DiscoveredONU{UUID: uuid.NewString(), PonPort: "2/1", SerialNumber: fmt.Sprintf("BIG-%04d", i), StatusID: pending, FirstSeenAt: t1, LastSeenAt: t1}
	}
	if err := repo.UpsertONUs(ctx, o.ID, big); err != nil {
		t.Fatalf("upsert 450 baris: %v", err)
	}
	if all, _ := repo.ListONUsByOLT(ctx, o.ID); len(all) != 452 {
		t.Fatalf("setelah batch: %d, harapan 452", len(all))
	}

	// ---- ListONUs: filter + join + pencarian + paginasi ----
	if _, total, _ := repo.ListONUs(ctx, domain.ONUFilter{TenantID: &tn.ID}, domain.Pagination{}); total != 452 {
		t.Fatalf("ListONUs tenant: %d", total)
	}
	if _, total, _ := repo.ListONUs(ctx, domain.ONUFilter{TenantID: &otherTenant}, domain.Pagination{}); total != 0 {
		t.Fatalf("tenant lain tidak boleh bocor: %d", total)
	}
	if rows, total, _ := repo.ListONUs(ctx, domain.ONUFilter{OLTID: &o.ID, StatusID: &ignored}, domain.Pagination{}); total != 1 || rows[0].SerialNumber != "S-A" {
		t.Fatalf("filter status: %d", total)
	}
	if _, total, _ := repo.ListONUs(ctx, domain.ONUFilter{OLTID: &o.ID, Search: "BIG-"}, domain.Pagination{}); total != 450 {
		t.Fatalf("cari serial: %d", total)
	}
	if _, total, _ := repo.ListONUs(ctx, domain.ONUFilter{OLTID: &o.ID, Search: "BIG-00"}, domain.Pagination{}); total != 100 {
		t.Fatalf("cari serial berawalan BIG-00: %d", total)
	}
	if _, total, _ := repo.ListONUs(ctx, domain.ONUFilter{OLTID: &o.ID, Search: "1/9"}, domain.Pagination{}); total != 1 {
		t.Fatalf("cari port: %d", total)
	}
	if byUUID, err := repo.GetONUByUUID(ctx, a.UUID); err != nil || byUUID.SerialNumber != "S-A" || byUUID.OLTUUID != o.UUID {
		t.Fatalf("GetONUByUUID: %+v %v", byUUID, err)
	}
	if _, err := repo.GetONUByUUID(ctx, uuid.NewString()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("onu tak ada: %v", err)
	}

	// ---- SetONUsStatus: resolved_at/updated_by terisi; nil mengosongkan; > satu chunk ----
	bigIDs := []uint64{}
	for _, n := range func() []domain.DiscoveredONU { all, _ := repo.ListONUsByOLT(ctx, o.ID); return all }() {
		if n.SerialNumber != "S-A" && n.SerialNumber != "S-B" {
			bigIDs = append(bigIDs, n.ID)
		}
	}
	sort.Slice(bigIDs, func(i, j int) bool { return bigIDs[i] < bigIDs[j] }) // urutan id = urutan penyisipan
	by := uint64(777)
	if err := repo.SetONUsStatus(ctx, bigIDs, resolved, &t1, &by); err != nil {
		t.Fatal(err)
	}
	var rcount int
	_ = db.Get(&rcount, `SELECT COUNT(*) FROM olt_unregistered_onus WHERE olt_id = ? AND status_id = ? AND resolved_at IS NOT NULL AND updated_by = ?`, o.ID, resolved, by)
	if rcount != 450 {
		t.Fatalf("SetONUsStatus 450 baris: %d", rcount)
	}
	if err := repo.SetONUsStatus(ctx, nil, resolved, &t1, nil); err != nil {
		t.Fatalf("daftar kosong harus aman: %v", err)
	}

	// ---- Prune: sisakan 2 RESOLVED terbaru (berdasarkan resolved_at, lalu id); yang lain dihapus-lunak ----
	for i, id := range bigIDs {
		ts := t0.Add(time.Duration(i) * time.Second) // id terakhir paling baru
		if _, err := db.Exec(`UPDATE olt_unregistered_onus SET resolved_at = ? WHERE id = ?`, ts, id); err != nil {
			t.Fatal(err)
		}
	}
	removed, err := repo.PruneResolvedONUs(ctx, o.ID, resolved, 2)
	if err != nil || removed != 448 {
		t.Fatalf("Prune: removed=%d err=%v", removed, err)
	}
	var keptIDs []uint64
	_ = db.Select(&keptIDs, `SELECT id FROM olt_unregistered_onus WHERE olt_id = ? AND status_id = ? AND is_deleted = 0 ORDER BY id`, o.ID, resolved)
	if len(keptIDs) != 2 || keptIDs[0] != bigIDs[448] || keptIDs[1] != bigIDs[449] {
		t.Fatalf("yang tersisa harus 2 terbaru: %v", keptIDs)
	}
	// PENDING/IGNORED tidak pernah ikut terpangkas
	if rows, _ := repo.ListONUsByOLT(ctx, o.ID); len(rows) != 4 {
		t.Fatalf("setelah prune: %d (S-A, S-B + 2 RESOLVED)", len(rows))
	}

	// ---- revive: serial yang sudah dihapus-lunak muncul lagi → baris yang sama hidup kembali, first_seen_at dihitung baru ----
	prunedSerial := fmt.Sprintf("BIG-%04d", 0)
	var prunedUUID string
	_ = db.Get(&prunedUUID, `SELECT onu_uuid FROM olt_unregistered_onus WHERE olt_id = ? AND serial_number = ?`, o.ID, prunedSerial)
	t2 := time.Now().Truncate(time.Millisecond)
	if err := repo.UpsertONUs(ctx, o.ID, []domain.DiscoveredONU{{UUID: uuid.NewString(), PonPort: "3/3", SerialNumber: prunedSerial, StatusID: pending, FirstSeenAt: t2, LastSeenAt: t2}}); err != nil {
		t.Fatalf("revive: %v", err)
	}
	var rv struct {
		UUID   string    `db:"onu_uuid"`
		First  time.Time `db:"first_seen_at"`
		Del    bool      `db:"is_deleted"`
		Status uint64    `db:"status_id"`
		Res    *string   `db:"resolved_at"`
	}
	if err := db.Get(&rv, `SELECT onu_uuid, first_seen_at, is_deleted, status_id, resolved_at FROM olt_unregistered_onus WHERE olt_id = ? AND serial_number = ?`, o.ID, prunedSerial); err != nil {
		t.Fatal(err)
	}
	if rv.UUID != prunedUUID || rv.Del || rv.Status != pending || rv.Res != nil || !near(rv.First, t2) {
		t.Fatalf("revive: %+v (uuid lama %s, first_seen harus ~sekarang)", rv, prunedUUID)
	}

	// ---- SoftDelete: OLT + antreannya hilang dari semua query; hapus ulang → NotFound ----
	if err := repo.SoftDelete(ctx, o.ID, 777); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetByUUID(ctx, o.UUID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("OLT terhapus: %v", err)
	}
	if rows, _ := repo.ListONUsByOLT(ctx, o.ID); len(rows) != 0 {
		t.Fatalf("antrean OLT terhapus harus ikut hilang: %d", len(rows))
	}
	if _, total, _ := repo.ListONUs(ctx, domain.ONUFilter{TenantID: &tn.ID}, domain.Pagination{}); total != 0 {
		t.Fatalf("ListONUs setelah hapus OLT: %d", total)
	}
	if _, total, _ := repo.List(ctx, domain.OLTFilter{TenantID: &tn.ID}, domain.Pagination{}); total != 1 {
		t.Fatalf("OLT lain tetap ada: %d", total)
	}
	if err := repo.SoftDelete(ctx, o.ID, 777); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("hapus ulang: %v", err)
	}
	var delBy *uint64
	_ = db.Get(&delBy, `SELECT deleted_by FROM olts WHERE id = ?`, o.ID)
	if delBy == nil || *delBy != 777 {
		t.Fatalf("deleted_by: %v", delBy)
	}
}
