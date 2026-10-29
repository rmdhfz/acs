package mysql

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"acs/internal/domain"
)

// Integrasi MariaDB NYATA untuk inventaris ONU terdaftar (migrations/0047): upsert per (olt, serial) dengan uuid/first_seen_at dipertahankan,
// last_state_change_at hanya maju saat keadaan berubah, tautan ke perangkat ACS (serial tak peka huruf besar/kecil, tenant sama, tanpa
// penggandaan baris), ringkasan port, hapus-lunak + hidup kembali, dan isolasi tenant. Dilewati tanpa ACS_TEST_DSN.
func TestOLTInventoryRepository_Integration(t *testing.T) {
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
	oltRepo := NewOLTRepository(db)
	inv := NewOLTInventoryRepository(db)
	refs := NewRefRepository(db)
	devRepo := NewDeviceRepository(db)

	stateID := func(code string) uint64 {
		r, err := refs.GetByCode(ctx, domain.RefTableOLTONUState, code)
		if err != nil {
			t.Fatalf("ref state %s: %v", code, err)
		}
		return r.ID
	}
	pollUnknown, err := refs.GetByCode(ctx, domain.RefTableOLTPollStatus, domain.OLTPollUnknown)
	if err != nil {
		t.Fatal(err)
	}
	devStatus, err := refs.GetByCode(ctx, domain.RefTableDeviceStatus, "UNREGISTERED")
	if err != nil {
		t.Fatal(err)
	}

	sfx := uuid.NewString()[:8]
	mkTenant := func(tag string) *domain.Tenant {
		tn := &domain.Tenant{TenantUUID: uuid.NewString(), Code: tag + sfx, Name: "it-inv-" + tag, IsActive: true}
		if err := NewTenantRepository(db).Create(ctx, tn); err != nil {
			t.Fatal(err)
		}
		return tn
	}
	tnA, tnB := mkTenant("A"), mkTenant("B")
	t.Cleanup(func() {
		for _, id := range []uint64{tnA.ID, tnB.ID} {
			_, _ = db.Exec(`DELETE n FROM olt_registered_onus n JOIN olts o ON o.id = n.olt_id WHERE o.tenant_id = ?`, id)
			_, _ = db.Exec(`DELETE FROM olts WHERE tenant_id = ?`, id)
			_, _ = db.Exec(`DELETE FROM devices WHERE tenant_id = ?`, id)
			_, _ = db.Exec(`DELETE FROM tenants WHERE id = ?`, id)
		}
	})
	mkOLT := func(tn *domain.Tenant, name string, active bool) *domain.OLT {
		o := &domain.OLT{UUID: uuid.NewString(), TenantID: tn.ID, Name: name, DriverCode: "sim", Host: "10.1.1.1", SNMPPort: 161, SSHPort: 22, IsActive: active, PollStatusID: pollUnknown.ID}
		if err := oltRepo.Create(ctx, o); err != nil {
			t.Fatal(err)
		}
		return o
	}
	oltA1, oltA2, oltB := mkOLT(tnA, "A1", true), mkOLT(tnA, "A2-nonaktif", false), mkOLT(tnB, "B1", true)

	mkDev := func(tn *domain.Tenant, oui, serial string, lastInform *time.Time) uint64 {
		o := oui
		d := &domain.Device{DeviceUUID: uuid.NewString(), TenantID: &tn.ID, DeviceStatusID: devStatus.ID, OUI: &o, SerialNumber: serial, LastInformAt: lastInform}
		if err := devRepo.Create(ctx, d); err != nil {
			t.Fatal(err)
		}
		return d.ID
	}

	sp := func(s string) *string { return &s }
	fp := func(f float64) *float64 { return &f }
	now := time.Now().Truncate(time.Millisecond)
	row := func(port, idx, serial, state string, rx *float64, at time.Time) domain.RegisteredONU {
		return domain.RegisteredONU{
			UUID: uuid.NewString(), PonPort: port, ONUIndex: sp(idx), SerialNumber: serial, ONUType: sp("SIM-ONU"),
			StateID: stateID(state), VendorState: sp("v-" + state), RxPowerDBM: rx, FirstSeenAt: at, LastSeenAt: at, LastStateChangeAt: at,
		}
	}
	serials := func(rows []domain.RegisteredONU) []string {
		out := make([]string, len(rows))
		for i, r := range rows {
			out[i] = r.SerialNumber
		}
		return out
	}
	same := func(a, b []string) bool {
		if len(a) != len(b) {
			return false
		}
		for i := range a {
			if a[i] != b[i] {
				return false
			}
		}
		return true
	}

	// ---- upsert awal: urutan tampil = port, nomor ONU numerik (2 sebelum 10), serial ----
	first := []domain.RegisteredONU{
		row("p/1", "10", "ZZ-S10", domain.OLTStateOnline, fp(-20.5), now),
		row("p/1", "2", "ZZ-S02", domain.OLTStateLOS, nil, now),
		row("p/2", "1", "zz-s21", domain.OLTStateOffline, fp(-27.0), now),
		row("p/2", "2", "ZZ-S22", domain.OLTStateOnline, fp(-22.0), now),
	}
	if err := inv.UpsertRegisteredONUs(ctx, oltA1.ID, first); err != nil {
		t.Fatal(err)
	}
	if err := inv.UpsertRegisteredONUs(ctx, oltB.ID, []domain.RegisteredONU{row("p/1", "1", "ZZ-S10", domain.OLTStateOnline, nil, now)}); err != nil {
		t.Fatal(err)
	}
	listed, total, err := inv.ListRegisteredONUs(ctx, domain.RegisteredONUFilter{OLTID: &oltA1.ID}, domain.Pagination{})
	if err != nil || total != 4 {
		t.Fatalf("daftar: total=%d err=%v", total, err)
	}
	if want := []string{"ZZ-S02", "ZZ-S10", "zz-s21", "ZZ-S22"}; !same(serials(listed), want) {
		t.Fatalf("urutan = %v, mau %v (port, lalu nomor ONU numerik)", serials(listed), want)
	}
	if listed[0].State != domain.OLTStateLOS || listed[0].OLTName != "A1" || listed[0].TenantID != tnA.ID || listed[0].OLTUUID != oltA1.UUID {
		t.Fatalf("join keadaan/OLT salah: %+v", listed[0])
	}

	// ---- filter + isolasi tenant ----
	los := stateID(domain.OLTStateLOS)
	for name, c := range map[string]struct {
		f    domain.RegisteredONUFilter
		want []string
	}{
		"per port":                   {domain.RegisteredONUFilter{OLTID: &oltA1.ID, PonPort: "p/2"}, []string{"zz-s21", "ZZ-S22"}},
		"per keadaan":                {domain.RegisteredONUFilter{OLTID: &oltA1.ID, StateID: &los}, []string{"ZZ-S02"}},
		"cari serial":                {domain.RegisteredONUFilter{OLTID: &oltA1.ID, Search: "s2"}, []string{"zz-s21", "ZZ-S22"}},
		"tenant A tak lihat":         {domain.RegisteredONUFilter{TenantID: &tnA.ID, Search: "ZZ-S10"}, []string{"ZZ-S10"}},
		"tenant B hanya OLT sendiri": {domain.RegisteredONUFilter{TenantID: &tnB.ID, Search: "ZZ-S"}, []string{"ZZ-S10"}},
	} {
		got, _, err := inv.ListRegisteredONUs(ctx, c.f, domain.Pagination{})
		if err != nil || !same(serials(got), c.want) {
			t.Errorf("%s: %v err=%v, mau %v", name, serials(got), err, c.want)
		}
	}
	if got, _, _ := inv.ListRegisteredONUs(ctx, domain.RegisteredONUFilter{TenantID: &tnB.ID, Search: "ZZ-S10"}, domain.Pagination{}); len(got) != 1 || got[0].OLTUUID != oltB.UUID {
		t.Errorf("serial sama di dua tenant: tenant B harus melihat barisnya sendiri saja: %+v", got)
	}

	// ---- tautan ke perangkat ACS ----
	recent, older := now.Add(-time.Minute), now.Add(-time.Hour)
	devLower := mkDev(tnA, "AAAAAA", "zz-s10", &older)  // huruf kecil = tetap cocok (collation tak peka)
	devNewer := mkDev(tnA, "BBBBBB", "ZZ-S10", &recent) // serial sama beda OUI: yang paling baru Inform menang
	_ = devLower
	devOtherTenant := mkDev(tnB, "AAAAAA", "ZZ-S22", &recent) // tenant lain: TIDAK boleh tertaut
	_ = devOtherTenant
	listed, _, _ = inv.ListRegisteredONUs(ctx, domain.RegisteredONUFilter{OLTID: &oltA1.ID}, domain.Pagination{})
	if len(listed) != 4 {
		t.Fatalf("serial ganda pada perangkat TIDAK boleh menggandakan baris ONU: %d baris", len(listed))
	}
	byS := map[string]domain.RegisteredONU{}
	for _, r := range listed {
		byS[r.SerialNumber] = r
	}
	if r := byS["ZZ-S10"]; r.DeviceID == nil || *r.DeviceID != devNewer || r.DeviceStatus == nil || *r.DeviceStatus != "UNREGISTERED" {
		t.Errorf("tautan ZZ-S10 = %+v, mau perangkat %d (paling baru Inform) berstatus UNREGISTERED", r, devNewer)
	}
	if r := byS["ZZ-S22"]; r.DeviceID != nil {
		t.Errorf("perangkat tenant lain tidak boleh tertaut: %+v", r.DeviceID)
	}
	if r := byS["ZZ-S02"]; r.DeviceID != nil {
		t.Errorf("tanpa perangkat = tanpa tautan: %+v", r.DeviceID)
	}

	// ---- ringkasan port ----
	ports, err := inv.PortSummaries(ctx, oltA1.ID)
	if err != nil || len(ports) != 2 {
		t.Fatalf("port: %+v err=%v", ports, err)
	}
	p1, p2 := ports[0], ports[1]
	if p1.PonPort != "p/1" || p1.Total != 2 || p1.Online != 1 || p1.LOS != 1 || p1.Offline != 0 || p1.Linked != 1 || p1.MinRxDBM == nil || *p1.MinRxDBM != -20.5 {
		t.Errorf("port p/1 = %+v", p1)
	}
	if p2.PonPort != "p/2" || p2.Total != 2 || p2.Online != 1 || p2.Offline != 1 || p2.Linked != 0 || p2.MinRxDBM == nil || *p2.MinRxDBM != -27 || p2.AvgRxDBM == nil || *p2.AvgRxDBM != -24.5 {
		t.Errorf("port p/2 = %+v", p2)
	}

	// ---- upsert ulang: uuid + first_seen dipertahankan; last_state_change hanya maju pada ONU yang berubah ----
	time.Sleep(30 * time.Millisecond)
	later := time.Now().Truncate(time.Millisecond)
	second := []domain.RegisteredONU{
		row("p/1", "10", "ZZ-S10", domain.OLTStateOnline, fp(-20.5), later), // tetap ONLINE
		row("p/1", "2", "ZZ-S02", domain.OLTStateOnline, fp(-21.0), later),  // LOS -> ONLINE
		row("p/2", "1", "zz-s21", domain.OLTStateOffline, fp(-27.0), later), // tetap OFFLINE
		row("p/2", "2", "ZZ-S22", domain.OLTStateOnline, fp(-22.0), later),
	}
	if err := inv.UpsertRegisteredONUs(ctx, oltA1.ID, second); err != nil {
		t.Fatal(err)
	}
	after, _, _ := inv.ListRegisteredONUs(ctx, domain.RegisteredONUFilter{OLTID: &oltA1.ID}, domain.Pagination{})
	byS = map[string]domain.RegisteredONU{}
	for _, r := range after {
		byS[r.SerialNumber] = r
	}
	prev := map[string]domain.RegisteredONU{}
	for _, r := range listed {
		prev[r.SerialNumber] = r
	}
	for s, r := range byS {
		if r.UUID != prev[s].UUID || !r.FirstSeenAt.Equal(prev[s].FirstSeenAt) {
			t.Errorf("%s: uuid/first_seen_at harus dipertahankan", s)
		}
		if !r.LastSeenAt.After(prev[s].LastSeenAt) {
			t.Errorf("%s: last_seen_at harus maju", s)
		}
		changed := s == "ZZ-S02"
		if changed != r.LastStateChangeAt.After(prev[s].LastStateChangeAt) {
			t.Errorf("%s: last_state_change_at maju=%v, mau %v", s, r.LastStateChangeAt.After(prev[s].LastStateChangeAt), changed)
		}
	}
	if byS["ZZ-S02"].State != domain.OLTStateOnline || byS["ZZ-S02"].RxPowerDBM == nil || *byS["ZZ-S02"].RxPowerDBM != -21 {
		t.Errorf("ZZ-S02 setelah perubahan: %+v", byS["ZZ-S02"])
	}

	// ---- hapus-lunak + hidup kembali ----
	gone := byS["zz-s21"]
	if err := inv.RemoveRegisteredONUs(ctx, []uint64{gone.ID}, nil); err != nil {
		t.Fatal(err)
	}
	if got, total, _ := inv.ListRegisteredONUs(ctx, domain.RegisteredONUFilter{OLTID: &oltA1.ID}, domain.Pagination{}); total != 3 || len(got) != 3 {
		t.Fatalf("setelah dihapus-lunak: total=%d", total)
	}
	if in, _ := inv.ListRegisteredByOLT(ctx, oltA1.ID); len(in) != 3 {
		t.Fatalf("ListRegisteredByOLT harus tanpa yang dihapus-lunak: %d", len(in))
	}
	time.Sleep(30 * time.Millisecond)
	revive := time.Now().Truncate(time.Millisecond)
	if err := inv.UpsertRegisteredONUs(ctx, oltA1.ID, []domain.RegisteredONU{row("p/2", "1", "zz-s21", domain.OLTStateOffline, nil, revive)}); err != nil {
		t.Fatal(err)
	}
	got, _, _ := inv.ListRegisteredONUs(ctx, domain.RegisteredONUFilter{OLTID: &oltA1.ID, Search: "s21"}, domain.Pagination{})
	if len(got) != 1 || got[0].UUID != gone.UUID || !got[0].FirstSeenAt.After(gone.FirstSeenAt) || !got[0].LastStateChangeAt.After(gone.LastStateChangeAt) {
		t.Fatalf("hidup kembali: uuid dipertahankan, first_seen & last_state_change diatur ulang: %+v", got)
	}

	// ---- FindRegisteredBySerial: tenant, OLT aktif, yang terakhir terlihat menang ----
	if err := inv.UpsertRegisteredONUs(ctx, oltA2.ID, []domain.RegisteredONU{row("x/1", "1", "ZZ-S22", domain.OLTStateDyingGasp, nil, time.Now().Add(time.Hour).Truncate(time.Millisecond))}); err != nil {
		t.Fatal(err)
	}
	f, err := inv.FindRegisteredBySerial(ctx, tnA.ID, "zz-s22")
	if err != nil || f.OLTUUID != oltA1.UUID || f.State != domain.OLTStateOnline {
		t.Fatalf("OLT nonaktif tidak boleh dipakai walau lebih baru: %+v err=%v", f, err)
	}
	if _, err := inv.FindRegisteredBySerial(ctx, tnB.ID, "ZZ-S22"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("tenant B tak boleh menemukan ONU milik OLT tenant A: %v", err)
	}
	if f, err := inv.FindRegisteredBySerial(ctx, tnA.ID, "ZZ-S10"); err != nil || f.DeviceID == nil || *f.DeviceID != devNewer || f.OLTLastPollAt != nil {
		t.Fatalf("temukan + tautan + OLTLastPollAt NULL (belum pernah dipoll): %+v err=%v", f, err)
	}

	// ---- jumlah OLT aktif & OLT yang perlu dipoll ----
	if n, _ := inv.CountActiveOLTs(ctx, tnA.ID); n != 1 {
		t.Errorf("OLT aktif tenant A = %d, mau 1 (yang nonaktif tak dihitung)", n)
	}
	due, err := inv.ListOLTsDueForPoll(ctx, time.Now(), 1000)
	if err != nil {
		t.Fatal(err)
	}
	has := func(id uint64) bool {
		for _, o := range due {
			if o.ID == id {
				return true
			}
		}
		return false
	}
	if !has(oltA1.ID) || !has(oltB.ID) || has(oltA2.ID) {
		t.Errorf("OLT jatuh tempo: harus memuat A1 dan B (belum pernah dipoll), tidak memuat yang nonaktif")
	}
	if err := oltRepo.RecordPoll(ctx, oltA1.ID, domain.OLTPollResult{StatusID: pollUnknown.ID}); err != nil {
		t.Fatal(err)
	}
	due, _ = inv.ListOLTsDueForPoll(ctx, time.Now().Add(-time.Hour), 1000)
	if has(oltA1.ID) {
		t.Errorf("OLT yang baru dipoll tidak boleh jatuh tempo untuk jendela 1 jam")
	}
}
