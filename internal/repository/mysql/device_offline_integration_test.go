package mysql

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"acs/internal/domain"
)

// Test integrasi MariaDB NYATA: ambang offline per perangkat (migrations/0046). SQL penyapu dan domain.OfflineThreshold adalah dua
// salinan rumus yang sama — test ini memastikan keduanya sepakat di setiap kasus, termasuk batas 7 hari dan interval raksasa.
// MarkStaleOffline bersifat global (semua perangkat basi di DB ikut berpindah), jadi jalankan hanya pada DB sekali pakai.
// Dilewati bila ACS_TEST_DSN tidak diisi.
func TestDeviceRepository_MarkStaleOffline_Integration(t *testing.T) {
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

	var onlineID, offlineID uint64
	if err := db.Get(&onlineID, `SELECT id FROM ref_device_status WHERE code = 'ONLINE'`); err != nil {
		t.Fatalf("status ONLINE: %v", err)
	}
	if err := db.Get(&offlineID, `SELECT id FROM ref_device_status WHERE code = 'OFFLINE'`); err != nil {
		t.Fatalf("status OFFLINE: %v", err)
	}

	u := func(v uint32) *uint32 { return &v }
	const minThreshold = 15 * time.Minute
	cases := []struct {
		name      string
		ago       time.Duration // sejak Inform terakhir
		interval  *uint32
		offline   bool // sudah OFFLINE sebelum sapuan
		neverSeen bool
		wantStale bool
	}{
		{name: "interval tak diketahui, diam 20 mnt", ago: 20 * time.Minute, wantStale: true},
		{name: "interval tak diketahui, diam 10 mnt", ago: 10 * time.Minute},
		{name: "interval 12 jam, diam 20 mnt (sehat)", ago: 20 * time.Minute, interval: u(43200)},
		{name: "interval 12 jam, diam 29 jam (masih dalam toleransi 30 jam)", ago: 29 * time.Hour, interval: u(43200)},
		{name: "interval 12 jam, diam 31 jam", ago: 31 * time.Hour, interval: u(43200), wantStale: true},
		{name: "interval 10 mnt, diam 26 mnt (ambang 25 mnt)", ago: 26 * time.Minute, interval: u(600), wantStale: true},
		{name: "interval 10 mnt, diam 24 mnt", ago: 24 * time.Minute, interval: u(600)},
		{name: "interval 1 mnt, diam 16 mnt (minimum 15 mnt menang)", ago: 16 * time.Minute, interval: u(60), wantStale: true},
		{name: "interval raksasa, diam 6 hari (dibatasi 7 hari)", ago: 6 * 24 * time.Hour, interval: u(4294967295)},
		{name: "interval raksasa, diam 8 hari", ago: 8 * 24 * time.Hour, interval: u(4294967295), wantStale: true},
		{name: "sudah OFFLINE tidak disiarkan ulang", ago: 3 * time.Hour, offline: true},
		{name: "belum pernah Inform", neverSeen: true},
	}

	now := time.Now()
	ids := make([]uint64, len(cases))
	for i, c := range cases {
		oui, serial := "ABCDEF", "IT-OFF-"+uuid.NewString()[:8]
		d := &domain.Device{DeviceUUID: uuid.NewString(), DeviceStatusID: onlineID, OUI: &oui, SerialNumber: serial}
		if err := repo.Create(ctx, d); err != nil {
			t.Fatalf("%s: create: %v", c.name, err)
		}
		ids[i] = d.ID
		t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM devices WHERE oui = ? AND serial_number = ?`, oui, serial) })

		status := onlineID
		if c.offline {
			status = offlineID
		}
		var lastInform any
		if !c.neverSeen {
			lastInform = now.Add(-c.ago)
		}
		if _, err := db.Exec(`UPDATE devices SET last_inform_at = ?, inform_interval_seconds = ?, device_status_id = ? WHERE id = ?`,
			lastInform, c.interval, status, d.ID); err != nil {
			t.Fatalf("%s: siapkan baris: %v", c.name, err)
		}

		// Rumus Go dan ekspektasi tertulis harus sepakat (menjaga tabel ini tetap jujur).
		if !c.neverSeen && !c.offline {
			if goStale := c.ago > domain.OfflineThreshold(c.interval, minThreshold); goStale != c.wantStale {
				t.Fatalf("%s: domain.OfflineThreshold berkata stale=%v, tabel kasus berkata %v", c.name, goStale, c.wantStale)
			}
		}
	}

	moved, err := repo.MarkStaleOffline(ctx, offlineID, minThreshold, now)
	if err != nil {
		t.Fatalf("MarkStaleOffline: %v", err)
	}
	movedSet := map[uint64]domain.StaleDevice{}
	for _, m := range moved {
		movedSet[m.ID] = m
	}
	for i, c := range cases {
		_, got := movedSet[ids[i]]
		if got != c.wantStale {
			t.Errorf("%s: dikembalikan sebagai berpindah=%v, mau %v", c.name, got, c.wantStale)
		}
		var status uint64
		if err := db.Get(&status, `SELECT device_status_id FROM devices WHERE id = ?`, ids[i]); err != nil {
			t.Fatal(err)
		}
		wantStatus := onlineID
		if c.wantStale || c.offline {
			wantStatus = offlineID
		}
		if status != wantStatus {
			t.Errorf("%s: status di DB = %d, mau %d", c.name, status, wantStatus)
		}
	}
	if m, ok := movedSet[ids[0]]; ok && (m.SerialNumber == "" || m.LastInformAt == nil) {
		t.Errorf("StaleDevice harus membawa serial & last_inform_at untuk siaran: %+v", m)
	}

	// Sapuan kedua: tidak ada yang berpindah lagi dari baris uji (idempoten, tidak ada siaran ganda).
	again, err := repo.MarkStaleOffline(ctx, offlineID, minThreshold, now)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range again {
		for _, id := range ids {
			if m.ID == id {
				t.Errorf("perangkat %d berpindah dua kali", id)
			}
		}
	}
}

func TestDeviceRepository_SetInformInterval_Integration(t *testing.T) {
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
		t.Fatal(err)
	}
	oui, serial := "ABCDEF", "IT-IVL-"+uuid.NewString()[:8]
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM devices WHERE oui = ? AND serial_number = ?`, oui, serial) })
	d := &domain.Device{DeviceUUID: uuid.NewString(), DeviceStatusID: statusID, OUI: &oui, SerialNumber: serial}
	if err := repo.Create(ctx, d); err != nil {
		t.Fatal(err)
	}

	read := func() *uint32 {
		got, err := repo.GetByID(ctx, d.ID)
		if err != nil {
			t.Fatal(err)
		}
		return got.InformIntervalSeconds
	}
	if read() != nil {
		t.Fatal("perangkat baru harus tanpa interval (NULL)")
	}
	v := uint32(43200)
	if err := repo.SetInformInterval(ctx, d.ID, &v); err != nil || read() == nil || *read() != 43200 {
		t.Fatalf("set 43200: %v (%v)", err, read())
	}
	if err := repo.SetInformInterval(ctx, d.ID, &v); err != nil { // nilai sama: tanpa galat
		t.Fatal(err)
	}
	if err := repo.SetInformInterval(ctx, d.ID, nil); err != nil || read() != nil {
		t.Fatalf("set NULL: %v (%v)", err, read())
	}
}
