package mysql

import (
	"context"
	"os"
	"testing"
	"time"
)

// MariaDB nyata: PruneLogs hanya memangkas baris LAMA dan tidak melanggar FK
// device_events -> device_sessions. Dilewati tanpa ACS_TEST_DSN.
func TestRetention_PruneLogs_Integration(t *testing.T) {
	dsn := os.Getenv("ACS_TEST_DSN")
	if dsn == "" {
		t.Skip("ACS_TEST_DSN tidak diisi — test integrasi MariaDB dilewati")
	}
	db, err := Connect(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()

	var devID, evCode uint64
	if err := db.GetContext(ctx, &devID, `SELECT id FROM devices LIMIT 1`); err != nil {
		t.Skip("tidak ada device untuk dipakai test")
	}
	if err := db.GetContext(ctx, &evCode, `SELECT id FROM ref_event_codes LIMIT 1`); err != nil {
		t.Fatal(err)
	}
	old := time.Now().AddDate(0, 0, -400)
	recent := time.Now().Add(-time.Hour)
	tokOld, tokNew := "ret-old-"+time.Now().Format("150405.000"), "ret-new-"+time.Now().Format("150405.000")
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE e FROM device_events e JOIN device_sessions s ON s.id = e.session_id WHERE s.session_token IN (?, ?)`, tokOld, tokNew)
		_, _ = db.ExecContext(ctx, `DELETE FROM device_sessions WHERE session_token IN (?, ?)`, tokOld, tokNew)
	})
	ins := func(tok string, at time.Time) uint64 {
		res, err := db.ExecContext(ctx, `INSERT INTO device_sessions (device_id, session_token, status, started_at) VALUES (?, ?, 'CLOSED', ?)`, devID, tok, at)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		if _, err := db.ExecContext(ctx, `INSERT INTO device_events (device_id, session_id, event_code_id, occurred_at) VALUES (?, ?, ?, ?)`, devID, id, evCode, at); err != nil {
			t.Fatal(err)
		}
		return uint64(id)
	}
	oldID, newID := ins(tokOld, old), ins(tokNew, recent)

	if _, err := NewRetentionRepository(db).PruneLogs(ctx, time.Now().AddDate(0, 0, -90)); err != nil {
		t.Fatalf("PruneLogs: %v", err)
	}
	var n int
	_ = db.GetContext(ctx, &n, `SELECT COUNT(*) FROM device_sessions WHERE id = ?`, oldID)
	if n != 0 {
		t.Error("sesi lama harus terhapus")
	}
	_ = db.GetContext(ctx, &n, `SELECT COUNT(*) FROM device_sessions WHERE id = ?`, newID)
	if n != 1 {
		t.Error("sesi baru tidak boleh terhapus")
	}
	_ = db.GetContext(ctx, &n, `SELECT COUNT(*) FROM device_events WHERE session_id = ?`, newID)
	if n != 1 {
		t.Error("event baru tidak boleh terhapus")
	}
}
