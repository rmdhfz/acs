package mysql

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"acs/internal/domain"
)

// Integrasi MariaDB NYATA (migrations/0030 harus sudah diterapkan): seluruh query notifikasi valid terhadap skema,
// isolasi per-penerima, dedupe, throttle email, preferensi, agregat evaluator, dan retensi. Dilewati tanpa ACS_TEST_DSN.
func TestNotificationRepository_Integration(t *testing.T) {
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
	repo := NewNotificationRepository(db)
	users := NewUserRepository(db)
	sfx := uuid.NewString()[:8]

	// ---- fixture: satu tenant + staf + enduser + admin nonaktif ----
	res, err := db.ExecContext(ctx, `INSERT INTO tenants (tenant_uuid, code, name) VALUES (?, ?, ?)`, uuid.NewString(), "it-n-"+sfx, "IT Notif "+sfx)
	if err != nil {
		t.Fatalf("tenant: %v", err)
	}
	tid64, _ := res.LastInsertId()
	tid := uint64(tid64)
	var userIDs []uint64
	mk := func(name string, active bool, role string) uint64 {
		u := &domain.User{UserUUID: uuid.NewString(), TenantID: &tid, Username: "it-n-" + name + "-" + sfx,
			Email: "it-n-" + name + "-" + sfx + "@example.test", PasswordHash: "x", FullName: name, IsActive: active}
		if err := users.Create(ctx, u); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		var rid uint64
		if err := db.GetContext(ctx, &rid, `SELECT id FROM ref_roles WHERE code = ?`, role); err != nil {
			t.Fatal(err)
		}
		if err := users.AssignRole(ctx, u.ID, rid, nil); err != nil {
			t.Fatal(err)
		}
		userIDs = append(userIDs, u.ID)
		return u.ID
	}
	admin := mk("admin", true, domain.RoleAdmin)
	noc := mk("noc", true, domain.RoleNOC)
	enduser := mk("enduser", true, domain.RoleEndUser)
	offAdmin := mk("offadmin", false, domain.RoleAdmin)
	var devID, taskID uint64
	t.Cleanup(func() {
		for _, id := range userIDs {
			_, _ = db.Exec(`DELETE FROM notifications WHERE user_id = ?`, id)
			_, _ = db.Exec(`DELETE FROM notification_preferences WHERE user_id = ?`, id)
			_, _ = db.Exec(`DELETE FROM user_roles WHERE user_id = ?`, id)
			_, _ = db.Exec(`DELETE FROM users WHERE id = ?`, id)
		}
		if taskID != 0 {
			_, _ = db.Exec(`DELETE FROM tasks WHERE id = ?`, taskID)
		}
		if devID != 0 {
			_, _ = db.Exec(`DELETE FROM devices WHERE id = ?`, devID)
		}
		_, _ = db.Exec(`DELETE FROM tenants WHERE id = ?`, tid)
	})

	// ---- penerima ----
	rc, err := repo.ListRecipients(ctx, &tid, []string{domain.RoleAdmin, domain.RoleNOC})
	if err != nil {
		t.Fatalf("ListRecipients: %v", err)
	}
	got := map[uint64]bool{}
	for _, r := range rc {
		got[r.UserID] = true
	}
	if len(got) != 2 || !got[admin] || !got[noc] || got[enduser] || got[offAdmin] {
		t.Fatalf("penerima ADMIN+NOC salah: %+v (enduser/nonaktif tidak boleh ikut)", rc)
	}
	one, err := repo.GetRecipient(ctx, admin)
	if err != nil || len(one.Roles) != 1 || one.Roles[0] != domain.RoleAdmin {
		t.Fatalf("GetRecipient: %+v %v", one, err)
	}
	if _, err := repo.GetRecipient(ctx, offAdmin); err != domain.ErrNotFound {
		t.Fatalf("pengguna nonaktif harus ErrNotFound, dapat %v", err)
	}

	// ---- tipe ----
	types, err := repo.ListTypes(ctx)
	if err != nil || len(types) < 7 {
		t.Fatalf("ListTypes: %d %v", len(types), err)
	}
	sec, err := repo.GetTypeByCode(ctx, domain.NotifSecurityAlert)
	if err != nil || !sec.IsMandatory || sec.Severity != domain.NotificationSeverityInfo || !sec.DefaultEmail {
		t.Fatalf("GetTypeByCode SECURITY_ALERT: %+v %v", sec, err)
	}
	ann, _ := repo.GetTypeByCode(ctx, domain.NotifAnnouncement)
	if id, err := repo.SeverityIDByCode(ctx, domain.NotificationSeverityCritical); err != nil || id == 0 || id == ann.SeverityID {
		t.Fatalf("SeverityIDByCode CRITICAL: %d %v", id, err)
	}
	if _, err := repo.SeverityIDByCode(ctx, "AKU_BUKAN_KEPARAHAN"); err != domain.ErrNotFound {
		t.Fatalf("keparahan tak dikenal harus ErrNotFound: %v", err)
	}
	if _, err := repo.GetTypeByCode(ctx, "TIDAK_ADA"); err != domain.ErrNotFound {
		t.Fatalf("jenis tak ada harus ErrNotFound: %v", err)
	}

	// ---- insert / list / mark read ----
	now := time.Now()
	key := "k-" + sfx
	link := "/devices/1"
	mkN := func(user uint64, title string, emailed bool) *domain.Notification {
		n := &domain.Notification{NotificationUUID: uuid.NewString(), TenantID: &tid, UserID: user, TypeID: ann.ID, SeverityID: ann.SeverityID,
			Title: title, Body: "isi", Link: &link, DedupeKey: &key}
		if emailed {
			n.EmailedAt = &now
		}
		return n
	}
	n1, n2, n3 := mkN(admin, "satu", true), mkN(admin, "dua", false), mkN(noc, "milik noc", false)
	if err := repo.InsertBatch(ctx, []*domain.Notification{n1, n2, n3}); err != nil {
		t.Fatalf("InsertBatch: %v", err)
	}
	if n1.ID == 0 || n2.ID <= n1.ID {
		t.Fatalf("ID belum diisi berurutan: %d %d", n1.ID, n2.ID)
	}
	items, total, unread, err := repo.List(ctx, admin, false, domain.Pagination{Page: 1, PageSize: 10})
	if err != nil || total != 2 || unread != 2 || len(items) != 2 || items[0].Title != "dua" {
		t.Fatalf("List admin: total=%d unread=%d items=%+v err=%v", total, unread, items, err)
	}
	if items[0].Type != domain.NotifAnnouncement || items[0].Severity != domain.NotificationSeverityInfo || items[0].TypeName == "" || items[0].ID != n2.NotificationUUID {
		t.Fatalf("bentuk item salah: %+v", items[0])
	}
	// admin TIDAK boleh menandai notifikasi milik noc
	if n, err := repo.MarkRead(ctx, admin, []string{n3.NotificationUUID}, now); err != nil || n != 0 {
		t.Fatalf("mark read milik orang lain harus 0 baris, dapat %d (%v)", n, err)
	}
	if c, _ := repo.CountUnread(ctx, noc); c != 1 {
		t.Fatalf("notifikasi noc tidak boleh berubah: unread=%d", c)
	}
	if n, err := repo.MarkRead(ctx, admin, []string{n1.NotificationUUID}, now); err != nil || n != 1 {
		t.Fatalf("MarkRead: %d %v", n, err)
	}
	if _, total, unread, _ = repo.List(ctx, admin, true, domain.Pagination{Page: 1, PageSize: 10}); total != 1 || unread != 1 {
		t.Fatalf("filter unread: total=%d unread=%d", total, unread)
	}
	if n, _ := repo.MarkAllRead(ctx, admin, now); n != 1 {
		t.Fatalf("MarkAllRead harus 1 baris tersisa, dapat %d", n)
	}
	if c, _ := repo.CountUnread(ctx, admin); c != 0 {
		t.Fatalf("unread admin harus 0: %d", c)
	}

	// ---- dedupe & throttle email ----
	if ok, err := repo.ExistsDedupe(ctx, ann.ID, &tid, key, now.Add(-time.Hour)); err != nil || !ok {
		t.Fatalf("dedupe harus ada: %v %v", ok, err)
	}
	if ok, _ := repo.ExistsDedupe(ctx, ann.ID, &tid, key, now.Add(time.Hour)); ok {
		t.Fatal("jendela di masa depan tidak boleh cocok")
	}
	if ok, _ := repo.ExistsDedupe(ctx, ann.ID, &tid, "lain", now.Add(-time.Hour)); ok {
		t.Fatal("kunci lain tidak boleh cocok")
	}
	if ok, _ := repo.ExistsDedupe(ctx, ann.ID, nil, key, now.Add(-time.Hour)); ok {
		t.Fatal("tenant NULL tidak boleh cocok dengan tenant terisi")
	}
	em, err := repo.EmailedUsersSince(ctx, ann.ID, []uint64{admin, noc}, now.Add(-time.Hour))
	if err != nil || !em[admin] || em[noc] {
		t.Fatalf("EmailedUsersSince: %v %v", em, err)
	}

	// ---- preferensi ----
	if err := repo.UpsertPreferences(ctx, admin, []domain.NotificationPreference{{TypeID: ann.ID, InApp: true, Email: false}}); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertPreferences(ctx, admin, []domain.NotificationPreference{{TypeID: ann.ID, InApp: false, Email: true}}); err != nil {
		t.Fatalf("upsert kedua (update): %v", err)
	}
	prefs, err := repo.ListPreferences(ctx, admin)
	if err != nil || len(prefs) != 1 || prefs[0].InApp || !prefs[0].Email {
		t.Fatalf("ListPreferences: %+v %v", prefs, err)
	}
	forUsers, err := repo.ListPreferencesForUsers(ctx, ann.ID, []uint64{admin, noc})
	if err != nil || len(forUsers) != 1 || forUsers[0].UserID != admin {
		t.Fatalf("ListPreferencesForUsers: %+v %v", forUsers, err)
	}

	// ---- agregat evaluator ----
	var statusID, taskTypeID, failedID uint64
	if err := db.GetContext(ctx, &statusID, `SELECT id FROM ref_device_status WHERE code = ?`, domain.DeviceStatusOffline); err != nil {
		t.Fatal(err)
	}
	dres, err := db.ExecContext(ctx, `INSERT INTO devices (device_uuid, tenant_id, device_status_id, oui, serial_number, last_inform_at) VALUES (?, ?, ?, ?, ?, NOW())`,
		uuid.NewString(), tid, statusID, "AABBCC", "IT-N-"+sfx)
	if err != nil {
		t.Fatalf("device: %v", err)
	}
	d64, _ := dres.LastInsertId()
	devID = uint64(d64)
	if err := db.GetContext(ctx, &taskTypeID, `SELECT id FROM ref_task_types LIMIT 1`); err != nil {
		t.Fatal(err)
	}
	if err := db.GetContext(ctx, &failedID, `SELECT id FROM ref_task_status WHERE code = ?`, domain.TaskStatusFailed); err != nil {
		t.Fatal(err)
	}
	tres, err := db.ExecContext(ctx, `INSERT INTO tasks (task_uuid, device_id, task_type_id, task_status_id) VALUES (?, ?, ?, ?)`, uuid.NewString(), devID, taskTypeID, failedID)
	if err != nil {
		t.Fatalf("task: %v", err)
	}
	t64, _ := tres.LastInsertId()
	taskID = uint64(t64)
	health, err := repo.TenantDeviceHealth(ctx)
	if err != nil {
		t.Fatalf("TenantDeviceHealth: %v", err)
	}
	foundH := false
	for _, h := range health {
		if h.TenantID == tid {
			foundH = h.Total == 1 && h.Offline == 1
		}
	}
	if !foundH {
		t.Fatalf("health tenant uji tidak sesuai: %+v", health)
	}
	failed, err := repo.FailedTasksSince(ctx, now.Add(-30*time.Minute))
	if err != nil {
		t.Fatalf("FailedTasksSince: %v", err)
	}
	foundF := false
	for _, f := range failed {
		if f.TenantID == tid {
			foundF = f.Count == 1
		}
	}
	if !foundF {
		t.Fatalf("task gagal tenant uji tidak terhitung: %+v", failed)
	}

	// ---- retensi: notifikasi lama terhapus, yang baru tetap ----
	old := mkN(noc, "lama", false)
	if err := repo.InsertBatch(ctx, []*domain.Notification{old}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE notifications SET created_at = ? WHERE id = ?`, now.AddDate(0, 0, -400), old.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := NewRetentionRepository(db).PruneLogs(ctx, now.AddDate(0, 0, -90)); err != nil {
		t.Fatalf("PruneLogs: %v", err)
	}
	var cnt int
	_ = db.GetContext(ctx, &cnt, `SELECT COUNT(*) FROM notifications WHERE id = ?`, old.ID)
	if cnt != 0 {
		t.Fatal("notifikasi lama harus dipangkas retensi")
	}
	_ = db.GetContext(ctx, &cnt, `SELECT COUNT(*) FROM notifications WHERE id = ?`, n3.ID)
	if cnt != 1 {
		t.Fatal("notifikasi baru tidak boleh dipangkas")
	}

	// id item di API harus UUID, bukan PK numerik
	if _, err := uuid.Parse(items[0].ID); err != nil {
		t.Fatalf("id item harus uuid: %v", err)
	}
}
