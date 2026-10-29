package mysql

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"acs/internal/domain"
)

// Integrasi MariaDB NYATA: riwayat perubahan keadaan ONU (migrations/0049): sisip batch (> satu chunk), urutan terbaru dulu, paginasi, join kode
// keadaan (NULL = baru/hilang), serial tak peka huruf besar/kecil, hitung perubahan sejati sejak waktu tertentu, pencarian ONU lewat UUID, isolasi
// antar OLT, dan pemangkasan retensi. Dilewati tanpa ACS_TEST_DSN.
func TestOLTStateEvents_Integration(t *testing.T) {
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
	oltRepo, inv, refs := NewOLTRepository(db), NewOLTInventoryRepository(db), NewRefRepository(db)

	sid := func(code string) *uint64 {
		r, err := refs.GetByCode(ctx, domain.RefTableOLTONUState, code)
		if err != nil {
			t.Fatalf("ref %s: %v", code, err)
		}
		id := r.ID
		return &id
	}
	pollUnknown, err := refs.GetByCode(ctx, domain.RefTableOLTPollStatus, domain.OLTPollUnknown)
	if err != nil {
		t.Fatal(err)
	}
	tn := &domain.Tenant{TenantUUID: uuid.NewString(), Code: "EV" + uuid.NewString()[:8], Name: "it-events", IsActive: true}
	if err := NewTenantRepository(db).Create(ctx, tn); err != nil {
		t.Fatal(err)
	}
	mkOLT := func(name string) *domain.OLT {
		o := &domain.OLT{UUID: uuid.NewString(), TenantID: tn.ID, Name: name, DriverCode: "sim", Host: "10.1.1.1", SNMPPort: 161, SSHPort: 22, IsActive: true, PollStatusID: pollUnknown.ID}
		if err := oltRepo.Create(ctx, o); err != nil {
			t.Fatal(err)
		}
		return o
	}
	o1, o2 := mkOLT("E1"), mkOLT("E2")
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM olt_onu_state_events WHERE olt_id IN (?, ?)`, o1.ID, o2.ID)
		_, _ = db.Exec(`DELETE FROM olt_registered_onus WHERE olt_id IN (?, ?)`, o1.ID, o2.ID)
		_, _ = db.Exec(`DELETE FROM olts WHERE tenant_id = ?`, tn.ID)
		_, _ = db.Exec(`DELETE FROM tenants WHERE id = ?`, tn.ID)
	})

	now := time.Now().Truncate(time.Millisecond)
	ev := func(olt uint64, serial string, from, to *uint64, ago time.Duration) domain.ONUStateEvent {
		return domain.ONUStateEvent{OLTID: olt, SerialNumber: serial, PonPort: "p/1", FromStateID: from, ToStateID: to, OccurredAt: now.Add(-ago)}
	}
	online, los, off := sid(domain.OLTStateOnline), sid(domain.OLTStateLOS), sid(domain.OLTStateOffline)
	rx := -23.5

	// urutan sisip (id naik): baru (400 hari lalu), ONLINE>OFFLINE (3 jam lalu: di luar jendela 1 jam), LOS>ONLINE, ONLINE>LOS, lalu hilang.
	first := []domain.ONUStateEvent{
		ev(o1.ID, "EVT-0001", nil, online, 400*24*time.Hour),
		ev(o1.ID, "EVT-0001", online, off, 3*time.Hour),
		ev(o1.ID, "EVT-0001", off, online, 30*time.Minute),
		ev(o1.ID, "EVT-0001", online, los, 10*time.Minute),
	}
	first[3].RxPowerDBM = &rx
	if err := inv.InsertStateEvents(ctx, first); err != nil {
		t.Fatal(err)
	}
	if err := inv.InsertStateEvents(ctx, []domain.ONUStateEvent{ev(o1.ID, "EVT-0001", los, nil, time.Minute), ev(o2.ID, "EVT-0001", nil, online, time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if err := inv.InsertStateEvents(ctx, nil); err != nil {
		t.Fatalf("batch kosong harus aman: %v", err)
	}

	got, total, err := inv.ListStateEvents(ctx, o1.ID, "EVT-0001", domain.Pagination{})
	if err != nil || total != 5 || len(got) != 5 {
		t.Fatalf("daftar: total=%d n=%d err=%v", total, len(got), err)
	}
	str := func(p *string) string {
		if p == nil {
			return "-"
		}
		return *p
	}
	var trail []string
	for _, e := range got {
		trail = append(trail, str(e.FromState)+">"+str(e.ToState))
	}
	if want := "LOS>-|ONLINE>LOS|OFFLINE>ONLINE|ONLINE>OFFLINE|->ONLINE"; strings.Join(trail, "|") != strings.ReplaceAll(want, "->ONLINE", "-"+">ONLINE") {
		t.Fatalf("urutan/join = %v (terbaru dulu; NULL = '-')", trail)
	}
	if got[1].RxPowerDBM == nil || *got[1].RxPowerDBM != -23.5 || got[1].PonPort != "p/1" {
		t.Errorf("RX dan port harus tersimpan: %+v", got[1])
	}
	// isolasi antar OLT: serial sama pada OLT lain tidak bercampur
	if g, tot, _ := inv.ListStateEvents(ctx, o2.ID, "EVT-0001", domain.Pagination{}); tot != 1 || len(g) != 1 {
		t.Errorf("OLT lain: total=%d", tot)
	}
	// serial tak peka huruf besar/kecil (collation), sama dengan tautan perangkat
	if _, tot, _ := inv.ListStateEvents(ctx, o1.ID, "evt-0001", domain.Pagination{}); tot != 5 {
		t.Errorf("serial huruf kecil harus cocok: total=%d", tot)
	}
	// paginasi
	if g, tot, _ := inv.ListStateEvents(ctx, o1.ID, "EVT-0001", domain.Pagination{Page: 2, PageSize: 2}); tot != 5 || len(g) != 2 || str(g[0].ToState) != "ONLINE" || str(g[0].FromState) != "OFFLINE" {
		t.Errorf("halaman 2 = %d baris", len(g))
	}

	// perubahan SEJATI saja (dari dan ke keduanya ada): baru/hilang tidak dihitung
	if n, err := inv.CountStateChanges(ctx, o1.ID, "EVT-0001", now.Add(-time.Hour)); err != nil || n != 2 {
		t.Errorf("1 jam: %d, mau 2 (LOS>ONLINE 30 mnt, ONLINE>LOS 10 mnt)", n)
	}
	if n, _ := inv.CountStateChanges(ctx, o1.ID, "EVT-0001", now.Add(-24*time.Hour)); n != 3 {
		t.Errorf("24 jam: %d, mau 3", n)
	}
	if n, _ := inv.CountStateChanges(ctx, o2.ID, "EVT-0001", now.Add(-24*time.Hour)); n != 0 {
		t.Errorf("OLT lain hanya punya peristiwa baru: %d", n)
	}

	// batch besar melewati satu chunk (500)
	var big []domain.ONUStateEvent
	for i := 0; i < 1200; i++ {
		big = append(big, ev(o2.ID, "BIG", online, los, time.Duration(i)*time.Second))
	}
	if err := inv.InsertStateEvents(ctx, big); err != nil {
		t.Fatal(err)
	}
	if _, tot, _ := inv.ListStateEvents(ctx, o2.ID, "BIG", domain.Pagination{PageSize: 10}); tot != 1200 {
		t.Errorf("1200 baris lintas chunk: total=%d", tot)
	}

	// FindRegisteredByUUID: ketemu pada OLT-nya saja, termasuk yang dihapus-lunak; tak ketemu di OLT lain
	row := domain.RegisteredONU{UUID: uuid.NewString(), PonPort: "p/1", SerialNumber: "EVT-0001", StateID: *online, FirstSeenAt: now, LastSeenAt: now, LastStateChangeAt: now}
	if err := inv.UpsertRegisteredONUs(ctx, o1.ID, []domain.RegisteredONU{row}); err != nil {
		t.Fatal(err)
	}
	if f, err := inv.FindRegisteredByUUID(ctx, o1.ID, row.UUID); err != nil || f.SerialNumber != "EVT-0001" || f.OLTUUID != o1.UUID {
		t.Fatalf("find: %+v err=%v", f, err)
	}
	if _, err := inv.FindRegisteredByUUID(ctx, o2.ID, row.UUID); err == nil {
		t.Error("UUID ONU dari OLT lain tidak boleh ditemukan")
	}
	list, _, _ := inv.ListRegisteredONUs(ctx, domain.RegisteredONUFilter{OLTID: &o1.ID}, domain.Pagination{})
	if len(list) != 1 {
		t.Fatalf("baris ONU: %d", len(list))
	}
	if err := inv.RemoveRegisteredONUs(ctx, []uint64{list[0].ID}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := inv.FindRegisteredByUUID(ctx, o1.ID, row.UUID); err != nil {
		t.Errorf("ONU yang dihapus-lunak tetap dapat dicari (riwayat 'kapan hilang'): %v", err)
	}

	// retensi: peristiwa lebih tua dari cutoff dipangkas fisik, yang lebih baru utuh. Cutoff sengaja SANGAT lama (365 hari): PruneLogs memangkas
	// seluruh tabel log di database uji, jadi cutoff dekat akan menghapus log milik data lain.
	res, err := NewRetentionRepository(db).PruneLogs(ctx, now.AddDate(0, 0, -365))
	if err != nil {
		t.Fatal(err)
	}
	if res["olt_onu_state_events"] < 1 {
		t.Errorf("retensi tidak memangkas olt_onu_state_events: %v", res)
	}
	if _, tot, _ := inv.ListStateEvents(ctx, o1.ID, "EVT-0001", domain.Pagination{}); tot != 4 {
		t.Errorf("setelah retensi 365 hari: total=%d, mau 4 (yang 400 hari lalu terpangkas)", tot)
	}
}
