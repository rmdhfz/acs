package mysql

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"

	"acs/internal/domain"
)

// Test integrasi MariaDB NYATA untuk aturan otomasi (migrations/0040): aturan, state pagar, riwayat, serta bolak-balik JSON actions.
// Dilewati bila ACS_TEST_DSN tidak diisi.
func TestAutomationRepositories_Integration(t *testing.T) {
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
	rules, states, runs := NewAutomationRuleRepository(db), NewAutomationStateRepository(db), NewAutomationRunRepository(db)

	sfx := uuid.NewString()[:6]
	mkTenant := func(code string) uint64 {
		res, err := db.Exec(`INSERT INTO tenants (tenant_uuid, code, name, is_active) VALUES (?, ?, ?, 1)`, uuid.NewString(), code, code)
		if err != nil {
			t.Fatalf("tenant: %v", err)
		}
		id, _ := res.LastInsertId()
		return uint64(id)
	}
	ta, tb := mkTenant("aua-"+sfx), mkTenant("aub-"+sfx)
	var statusID uint64
	if err := db.Get(&statusID, `SELECT id FROM ref_device_status WHERE code = 'UNREGISTERED'`); err != nil {
		t.Fatal(err)
	}
	oui := "ABCDEF"
	devRepo := NewDeviceRepository(db)
	d1 := &domain.Device{DeviceUUID: uuid.NewString(), TenantID: &ta, DeviceStatusID: statusID, OUI: &oui, SerialNumber: "AU1-" + sfx}
	d2 := &domain.Device{DeviceUUID: uuid.NewString(), TenantID: &ta, DeviceStatusID: statusID, OUI: &oui, SerialNumber: "AU2-" + sfx}
	for _, d := range []*domain.Device{d1, d2} {
		if err := devRepo.Create(ctx, d); err != nil {
			t.Fatal(err)
		}
	}
	var ruleIDs []uint64
	t.Cleanup(func() {
		for _, id := range ruleIDs {
			_, _ = db.Exec(`DELETE FROM automation_runs WHERE rule_id = ?`, id)
			_, _ = db.Exec(`DELETE FROM automation_states WHERE rule_id = ?`, id)
			_, _ = db.Exec(`DELETE FROM automation_rules WHERE id = ?`, id)
		}
		_, _ = db.Exec(`DELETE FROM devices WHERE id IN (?, ?)`, d1.ID, d2.ID)
		_, _ = db.Exec(`DELETE FROM tenants WHERE id IN (?, ?)`, ta, tb)
	})

	tag := uint64(9)
	actions := domain.AutomationActions{
		{Type: domain.AutomationActionSetParameter, Name: "wifi.ssid", Value: "concat('ACS-', {@serial_number})"},
		{Type: domain.AutomationActionGetParameterValues, Names: []string{"A.B", "C.D"}},
		{Type: domain.AutomationActionAddTag, TagID: &tag}, {Type: domain.AutomationActionReboot},
	}
	mk := func(tenant uint64, name string, active bool, prio uint32) *domain.AutomationRule {
		r := &domain.AutomationRule{TenantID: tenant, Name: name, TriggerEvent: domain.ZtpTriggerEventEveryInform,
			Condition: "{X} < 1", Actions: actions, CooldownSeconds: 600, MaxConsecutiveFires: 3, Priority: prio, IsActive: active}
		if err := rules.Create(ctx, r, nil); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		ruleIDs = append(ruleIDs, r.ID)
		return r
	}
	r1 := mk(ta, "r1", true, 200)
	r2 := mk(ta, "r2", true, 100)
	mk(ta, "mati", false, 50)
	mk(tb, "lain", true, 10)

	// bolak-balik JSON actions + kode trigger dari ref
	got, err := rules.GetByUUID(ctx, r1.UUID)
	if err != nil || got.TriggerEvent != domain.ZtpTriggerEventEveryInform || len(got.Actions) != 4 ||
		got.Actions[0].Value != actions[0].Value || len(got.Actions[1].Names) != 2 || got.Actions[2].TagID == nil || *got.Actions[2].TagID != 9 {
		t.Fatalf("round-trip actions: %+v %v", got, err)
	}
	// ListActive: aktif saja, tenant sendiri, urut priority
	act, err := rules.ListActive(ctx, ta)
	if err != nil || len(act) != 2 || act[0].UUID != r2.UUID || act[1].UUID != r1.UUID {
		t.Fatalf("ListActive: %+v %v", act, err)
	}
	if n, _ := rules.CountInTenant(ctx, ta); n != 3 {
		t.Errorf("CountInTenant = %d, want 3", n)
	}
	if _, err := rules.FindByName(ctx, tb, "r1"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("FindByName lintas tenant: %v", err)
	}
	// trigger tak dikenal ditolak sebagai ErrInvalidInput (bukan 500)
	bad := &domain.AutomationRule{TenantID: ta, Name: "x", TriggerEvent: "NGAWUR", Actions: actions, CooldownSeconds: 600, MaxConsecutiveFires: 3}
	if err := rules.Create(ctx, bad, nil); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("trigger tak dikenal: %v", err)
	}
	// update + soft delete
	r1.Condition, r1.TriggerEvent = "{Y} > 2", domain.ZtpTriggerEventBootstrapOnly
	if err := rules.Update(ctx, r1, nil); err != nil {
		t.Fatal(err)
	}
	if g, _ := rules.GetByUUID(ctx, r1.UUID); g.Condition != "{Y} > 2" || g.TriggerEvent != domain.ZtpTriggerEventBootstrapOnly {
		t.Errorf("update: %+v", g)
	}
	// ---- state ----
	if err := states.Save(ctx, &domain.AutomationState{RuleID: r1.ID, DeviceID: d1.ID, ConsecutiveFires: 2}); err != nil {
		t.Fatal(err)
	}
	if err := states.Save(ctx, &domain.AutomationState{RuleID: r1.ID, DeviceID: d1.ID, ConsecutiveFires: 3, Suspended: true}); err != nil { // upsert
		t.Fatal(err)
	}
	_ = states.Save(ctx, &domain.AutomationState{RuleID: r1.ID, DeviceID: d2.ID, ConsecutiveFires: 1, Suspended: true})
	m, err := states.ListByDevice(ctx, d1.ID)
	if err != nil || len(m) != 1 || m[r1.ID].ConsecutiveFires != 3 || !m[r1.ID].Suspended {
		t.Fatalf("ListByDevice: %+v %v", m, err)
	}
	if err := states.ResetConsecutive(ctx, r1.ID, d1.ID); err != nil {
		t.Fatal(err)
	}
	if m, _ = states.ListByDevice(ctx, d1.ID); m[r1.ID].ConsecutiveFires != 0 || !m[r1.ID].Suspended {
		t.Errorf("ResetConsecutive tidak boleh membuka suspend: %+v", m[r1.ID])
	}
	one := d1.ID
	if n, err := states.ClearSuspension(ctx, r1.ID, &one); err != nil || n != 1 {
		t.Errorf("ClearSuspension(1 perangkat): %d %v", n, err)
	}
	if m, _ = states.ListByDevice(ctx, d2.ID); !m[r1.ID].Suspended {
		t.Error("perangkat lain tidak boleh ikut terbuka")
	}
	if n, _ := states.ClearSuspension(ctx, r1.ID, nil); n != 1 {
		t.Errorf("ClearSuspension(semua): %d", n)
	}

	// ---- riwayat ----
	msg := "gagal"
	for i, fired := range []bool{true, true, false} {
		run := &domain.AutomationRun{RuleID: r1.ID, DeviceID: d1.ID, Fired: fired, Summary: "reboot"}
		if !fired {
			run.Error = &msg
		}
		if i == 2 {
			run.DeviceID = d2.ID
		}
		if err := runs.Insert(ctx, run); err != nil || run.UUID == "" {
			t.Fatalf("insert run: %v", err)
		}
	}
	rows, total, err := runs.List(ctx, domain.AutomationRunFilter{TenantID: &ta, RuleUUID: r1.UUID}, domain.Pagination{Page: 1, PageSize: 50})
	if err != nil || total != 3 || rows[0].Fired || rows[0].RuleName != "r1" || rows[0].DeviceSerial != d2.SerialNumber || rows[0].Error == nil {
		t.Fatalf("List runs: total=%d %+v %v", total, rows, err)
	}
	if _, total, _ := runs.List(ctx, domain.AutomationRunFilter{TenantID: &tb}, domain.Pagination{Page: 1, PageSize: 50}); total != 0 {
		t.Errorf("tenant lain tidak boleh melihat riwayat: %d", total)
	}
	dev1 := d1.ID
	if _, total, _ := runs.List(ctx, domain.AutomationRunFilter{DeviceID: &dev1, RuleUUID: r1.UUID}, domain.Pagination{Page: 1, PageSize: 50}); total != 2 {
		t.Errorf("filter device: %d", total)
	}

	if err := rules.SoftDelete(ctx, r2.ID, 1); err != nil {
		t.Fatal(err)
	}
	if act, _ = rules.ListActive(ctx, ta); len(act) != 1 || act[0].UUID != r1.UUID {
		t.Errorf("setelah hapus: %+v", act)
	}
	if err := rules.SoftDelete(ctx, r2.ID, 1); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("hapus dua kali: %v", err)
	}
	if _, err := rules.GetByUUID(ctx, r2.UUID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("GetByUUID terhapus: %v", err)
	}
}
