package mysql

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// Integrasi MariaDB NYATA: backfill mengenkripsi nilai rahasia pada tasks.parameters lama (plaintext), menjaga
// updated_at, melewati task bertipe lain, dan idempoten. Dilewati tanpa ACS_TEST_DSN.
func TestSealExistingTaskSecrets_Integration(t *testing.T) {
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
	s := testSealer(t)

	var statusID, setTypeID, otherTypeID, taskStatusID uint64
	if err := db.GetContext(ctx, &statusID, `SELECT id FROM ref_device_status LIMIT 1`); err != nil {
		t.Fatal(err)
	}
	if err := db.GetContext(ctx, &setTypeID, `SELECT id FROM ref_task_types WHERE code = 'SET_PARAMETER_VALUES'`); err != nil {
		t.Fatal(err)
	}
	if err := db.GetContext(ctx, &otherTypeID, `SELECT id FROM ref_task_types WHERE code <> 'SET_PARAMETER_VALUES' LIMIT 1`); err != nil {
		t.Fatal(err)
	}
	if err := db.GetContext(ctx, &taskStatusID, `SELECT id FROM ref_task_status LIMIT 1`); err != nil {
		t.Fatal(err)
	}
	sfx := uuid.NewString()[:8]
	dres, err := db.ExecContext(ctx, `INSERT INTO devices (device_uuid, device_status_id, oui, serial_number) VALUES (?, ?, ?, ?)`,
		uuid.NewString(), statusID, "AABBCC", "IT-SEALTASK-"+sfx)
	if err != nil {
		t.Fatalf("device: %v", err)
	}
	devID, _ := dres.LastInsertId()
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM tasks WHERE device_id = ?`, devID)
		_, _ = db.Exec(`DELETE FROM devices WHERE id = ?`, devID)
	})

	plain := `{"values":{"` + taskPwdName + `":"wifi-lama-plain","` + taskSSIDName + `":"Rumah"}}`
	otherPlain := `{"values":{"` + taskPwdName + `":"bukan-set-param"}}`
	insert := func(typeID uint64, params string) int64 {
		res, err := db.ExecContext(ctx, `INSERT INTO tasks (task_uuid, device_id, task_type_id, task_status_id, parameters, updated_at) VALUES (?, ?, ?, ?, ?, '2026-01-02 03:04:05')`,
			uuid.NewString(), devID, typeID, taskStatusID, params)
		if err != nil {
			t.Fatalf("task: %v", err)
		}
		id, _ := res.LastInsertId()
		return id
	}
	setID := insert(setTypeID, plain)
	otherID := insert(otherTypeID, otherPlain)

	n, err := SealExistingTaskSecrets(ctx, db, s)
	if err != nil || n < 1 {
		t.Fatalf("backfill: n=%d err=%v", n, err)
	}

	var got struct {
		Params  string    `db:"parameters"`
		Updated time.Time `db:"updated_at"`
	}
	if err := db.GetContext(ctx, &got, `SELECT parameters, updated_at FROM tasks WHERE id = ?`, setID); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got.Params, "wifi-lama-plain") || !strings.Contains(got.Params, sealedPrefix) {
		t.Fatalf("task SET_PARAMETER_VALUES harus tersegel: %s", got.Params)
	}
	if !strings.Contains(got.Params, "Rumah") {
		t.Fatalf("nilai tak sensitif harus tetap plaintext: %s", got.Params)
	}
	if got.Updated.Format("2006-01-02 15:04:05") != "2026-01-02 03:04:05" {
		t.Fatalf("updated_at harus dipertahankan: %s", got.Updated)
	}
	var other string
	if err := db.GetContext(ctx, &other, `SELECT parameters FROM tasks WHERE id = ?`, otherID); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(other, "bukan-set-param") {
		t.Fatalf("task bertipe lain tidak boleh disentuh: %s", other)
	}

	// idempoten: tidak ada baris milik uji ini yang berubah lagi
	before := got.Params
	if _, err := SealExistingTaskSecrets(ctx, db, s); err != nil {
		t.Fatal(err)
	}
	_ = db.GetContext(ctx, &got.Params, `SELECT parameters FROM tasks WHERE id = ?`, setID)
	if got.Params != before {
		t.Fatalf("backfill harus idempoten: %s != %s", got.Params, before)
	}
}
