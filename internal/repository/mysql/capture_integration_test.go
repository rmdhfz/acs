package mysql

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"acs/internal/domain"
)

// Integrasi MariaDB NYATA (migrations/0037): saklar capture (nyalakan/perpanjang/matikan/kedaluwarsa), simpan pesan,
// pemangkasan (hanya N terbaru), daftar ringan tanpa body, ambil per UUID hanya milik perangkat itu, ekspor urut naik.
func TestCaptureRepository_Integration(t *testing.T) {
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
	devs := NewDeviceRepository(db)
	repo := NewCaptureRepository(db)

	var statusID uint64
	if err := db.Get(&statusID, `SELECT id FROM ref_device_status WHERE code = 'UNREGISTERED'`); err != nil {
		t.Fatal(err)
	}
	oui := "ABCDEF"
	mk := func(tag string) uint64 {
		d := &domain.Device{DeviceUUID: uuid.NewString(), DeviceStatusID: statusID, OUI: &oui, SerialNumber: "ITC-" + tag + "-" + uuid.NewString()[:6]}
		if err := devs.Create(ctx, d); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_, _ = db.Exec(`DELETE FROM device_session_captures WHERE device_id = ?`, d.ID)
			_, _ = db.Exec(`DELETE FROM device_capture_switches WHERE device_id = ?`, d.ID)
			_, _ = db.Exec(`DELETE FROM devices WHERE id = ?`, d.ID)
		})
		return d.ID
	}
	a, b := mk("a"), mk("b")
	now := time.Now()

	// saklar: kosong -> nyala -> diperpanjang -> mati
	if st, err := repo.Status(ctx, a, now); err != nil || st.Active {
		t.Fatalf("awal harus tidak aktif: %+v %v", st, err)
	}
	if err := repo.EnableUntil(ctx, a, now.Add(10*time.Minute), nil); err != nil {
		t.Fatal(err)
	}
	if err := repo.EnableUntil(ctx, a, now.Add(30*time.Minute), nil); err != nil { // upsert, bukan baris ganda
		t.Fatal(err)
	}
	ids, err := repo.ActiveDeviceIDs(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	foundA, foundB := false, false
	for _, id := range ids {
		foundA = foundA || id == a
		foundB = foundB || id == b
	}
	if !foundA || foundB {
		t.Fatalf("hanya perangkat a yang aktif: %v", ids)
	}
	if err := repo.Disable(ctx, a, now); err != nil {
		t.Fatal(err)
	}
	if st, _ := repo.Status(ctx, a, now.Add(time.Second)); st.Active {
		t.Fatal("setelah Disable harus tidak aktif")
	}
	if err := repo.Disable(ctx, b, now); err != nil { // tanpa baris: bukan error
		t.Fatal(err)
	}

	// pesan: simpan 7, pertahankan 5 terbaru
	for i := 0; i < 7; i++ {
		c := &domain.SessionCapture{DeviceID: a, FromCPE: i%2 == 0, RPCName: "Inform", Body: "<x>" + strings.Repeat("é", 10) + "</x>", BodyBytes: 30}
		if err := repo.Insert(ctx, c, 5, time.Hour); err != nil {
			t.Fatal(err)
		}
		if c.UUID == "" || c.ID == 0 {
			t.Fatal("Insert harus mengisi ID/UUID")
		}
	}
	other := &domain.SessionCapture{DeviceID: b, RPCName: "Fault", Body: "<f/>", BodyBytes: 4}
	if err := repo.Insert(ctx, other, 5, time.Hour); err != nil {
		t.Fatal(err)
	}
	rows, total, err := repo.List(ctx, a, domain.Pagination{Page: 1, PageSize: 50})
	if err != nil || total != 5 || len(rows) != 5 {
		t.Fatalf("harus tersisa 5 pesan terbaru: total=%d n=%d err=%v", total, len(rows), err)
	}
	for _, r := range rows {
		if r.Body != "" {
			t.Fatal("daftar tidak boleh membawa body")
		}
	}
	if rows[0].ID < rows[4].ID {
		t.Fatal("daftar harus terbaru dulu")
	}
	// pemangkasan satu perangkat tidak menyentuh perangkat lain
	if _, tb, _ := repo.List(ctx, b, domain.Pagination{Page: 1, PageSize: 10}); tb != 1 {
		t.Fatalf("pesan perangkat b tidak boleh ikut terpangkas: %d", tb)
	}

	// ambil per UUID: hanya milik perangkat itu, dengan body utuh (UTF-8)
	got, err := repo.GetByUUID(ctx, a, rows[0].UUID)
	if err != nil || !strings.Contains(got.Body, "éééé") {
		t.Fatalf("GetByUUID: %+v %v", got, err)
	}
	if _, err := repo.GetByUUID(ctx, b, rows[0].UUID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("UUID perangkat lain harus ErrNotFound: %v", err)
	}

	// ekspor: urut waktu naik, dengan body
	all, err := repo.ListAll(ctx, a, 100)
	if err != nil || len(all) != 5 || all[0].ID > all[4].ID || all[0].Body == "" {
		t.Fatalf("ListAll: n=%d err=%v", len(all), err)
	}
}
