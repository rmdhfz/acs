package mysql

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/google/uuid"

	"acs/internal/domain"
)

// Test integrasi MariaDB NYATA untuk faults (migrations/0038): dedup, batas fault aktif per perangkat, penutupan.
// Dilewati bila ACS_TEST_DSN tidak diisi.
func TestFaultRepository_Integration(t *testing.T) {
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
	repo := NewFaultRepository(db)
	devs := NewDeviceRepository(db)

	var statusID uint64
	if err := db.Get(&statusID, `SELECT id FROM ref_device_status WHERE code = 'UNREGISTERED'`); err != nil {
		t.Fatal(err)
	}
	oui := "ABCDEF"
	d := &domain.Device{DeviceUUID: uuid.NewString(), DeviceStatusID: statusID, OUI: &oui, SerialNumber: "FLT-" + uuid.NewString()[:8]}
	if err := devs.Create(ctx, d); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM device_faults WHERE device_id = ?`, d.ID)
		_, _ = db.Exec(`DELETE FROM devices WHERE id = ?`, d.ID)
	})

	// dedup: kejadian berulang = satu baris, count naik
	for i := 0; i < 3; i++ {
		if err := repo.Upsert(ctx, &domain.DeviceFault{DeviceID: d.ID, RPCMethod: "SET_PARAMETER_VALUES", FaultCode: "9005", FaultString: "x"}); err != nil {
			t.Fatal(err)
		}
	}
	rows, total, err := repo.List(ctx, domain.FaultFilter{DeviceID: &d.ID}, domain.Pagination{Page: 1, PageSize: 10})
	if err != nil || total != 1 || rows[0].OccurrenceCount != 3 {
		t.Fatalf("dedup: total=%d %+v %v", total, rows, err)
	}

	// batas per perangkat: kode berbeda-beda tidak boleh menumbuhkan tabel tanpa batas
	for i := 1; i < domain.FaultMaxActivePerDevice; i++ {
		if err := repo.Upsert(ctx, &domain.DeviceFault{DeviceID: d.ID, RPCMethod: "REBOOT", FaultCode: fmt.Sprint(9100 + i)}); err != nil {
			t.Fatalf("fault #%d: %v", i, err)
		}
	}
	if err := repo.Upsert(ctx, &domain.DeviceFault{DeviceID: d.ID, RPCMethod: "REBOOT", FaultCode: "9999"}); !errors.Is(err, domain.ErrQuotaExceeded) {
		t.Fatalf("fault ke-%d harus ditolak ErrQuotaExceeded, got %v", domain.FaultMaxActivePerDevice+1, err)
	}
	// kejadian ulang untuk kode yang SUDAH ada tetap diterima (tidak menambah baris)
	if err := repo.Upsert(ctx, &domain.DeviceFault{DeviceID: d.ID, RPCMethod: "REBOOT", FaultCode: "9101"}); err != nil {
		t.Fatalf("kejadian ulang harus diterima walau batas tercapai: %v", err)
	}
	// menutup membuka ruang lagi
	if n, err := repo.ResolveActive(ctx, d.ID, "REBOOT"); err != nil || n != int64(domain.FaultMaxActivePerDevice-1) {
		t.Fatalf("ResolveActive: %d %v", n, err)
	}
	if err := repo.Upsert(ctx, &domain.DeviceFault{DeviceID: d.ID, RPCMethod: "REBOOT", FaultCode: "9999"}); err != nil {
		t.Fatalf("setelah ditutup harus bisa lagi: %v", err)
	}
}
