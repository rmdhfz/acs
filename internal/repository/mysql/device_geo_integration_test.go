package mysql

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"

	"acs/internal/domain"
)

// Test integrasi MariaDB NYATA: latitude/longitude harus tersimpan di Create,
// Update, dan jalur revive (sebelumnya repository tidak pernah menulis kolom
// itu, sehingga lokasi yang diisi operator hilang diam-diam).
// Dilewati bila ACS_TEST_DSN tidak diisi (lihat password_reset_integration_test.go).
func TestDeviceRepository_GeoPersistence_Integration(t *testing.T) {
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
	repo := NewDeviceRepository(db)

	var statusID uint64
	if err := db.Get(&statusID, `SELECT id FROM ref_device_status WHERE code = 'UNREGISTERED'`); err != nil {
		t.Fatalf("status: %v", err)
	}
	oui := "ABCDEF"
	serial := "IT-GEO-" + uuid.NewString()[:8]
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM devices WHERE oui = ? AND serial_number = ?`, oui, serial) })

	lat, lon := -6.2088, 106.8456
	d := &domain.Device{
		DeviceUUID: uuid.NewString(), DeviceStatusID: statusID, OUI: &oui, SerialNumber: serial,
		Latitude: &lat, Longitude: &lon,
	}
	if err := repo.Create(ctx, d); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetByID(ctx, d.ID)
	if err != nil || got.Latitude == nil || got.Longitude == nil || *got.Latitude != lat || *got.Longitude != lon {
		t.Fatalf("Create harus menyimpan lokasi: %+v err=%v", got, err)
	}

	lat2, lon2 := 3.5952, 98.6722
	got.Latitude, got.Longitude = &lat2, &lon2
	if err := repo.Update(ctx, got); err != nil {
		t.Fatal(err)
	}
	got, _ = repo.GetByID(ctx, d.ID)
	if got.Latitude == nil || *got.Latitude != lat2 || *got.Longitude != lon2 {
		t.Fatalf("Update harus menyimpan lokasi: lat=%v lon=%v", got.Latitude, got.Longitude)
	}

	// Revive: baris terhapus dihidupkan lagi dengan lokasi dari registrasi baru.
	if err := repo.SoftDelete(ctx, d.ID, 1); err != nil {
		t.Fatal(err)
	}
	lat3, lon3 := -7.2575, 112.7521
	again := &domain.Device{
		DeviceUUID: uuid.NewString(), DeviceStatusID: statusID, OUI: &oui, SerialNumber: serial,
		Latitude: &lat3, Longitude: &lon3,
	}
	if err := repo.Create(ctx, again); err != nil {
		t.Fatal(err)
	}
	if again.ID != d.ID {
		t.Fatalf("harus me-revive baris yang sama: %d vs %d", again.ID, d.ID)
	}
	got, _ = repo.GetByID(ctx, again.ID)
	if got.Latitude == nil || *got.Latitude != lat3 || *got.Longitude != lon3 {
		t.Fatalf("revive harus menulis lokasi baru: lat=%v lon=%v", got.Latitude, got.Longitude)
	}
}
