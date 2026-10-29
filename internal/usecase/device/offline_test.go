package device

import (
	"context"
	"errors"
	"testing"
	"time"

	"acs/internal/domain"
)

type offlineRepo struct {
	domain.DeviceRepository
	moved    []domain.StaleDevice
	err      error
	gotMin   time.Duration
	gotNow   time.Time
	gotID    uint64
	gotSec   *uint32
	setCalls int
}

func (r *offlineRepo) MarkStaleOffline(_ context.Context, _ uint64, min time.Duration, now time.Time) ([]domain.StaleDevice, error) {
	r.gotMin, r.gotNow = min, now
	return r.moved, r.err
}
func (r *offlineRepo) SetInformInterval(_ context.Context, id uint64, sec *uint32) error {
	r.setCalls++
	r.gotID, r.gotSec = id, sec
	return nil
}

type recordedEvent struct {
	tenant  uint64
	typ     string
	payload map[string]interface{}
}

type fakePublisher struct{ events []recordedEvent }

func (p *fakePublisher) BroadcastEvent(string, interface{}) {}
func (p *fakePublisher) BroadcastToTenant(tenantID uint64, typ string, payload interface{}) {
	m, _ := payload.(map[string]interface{})
	p.events = append(p.events, recordedEvent{tenantID, typ, m})
}

func TestMarkStaleOffline_MenyiarkanHanyaYangBerpindahDanPunyaTenant(t *testing.T) {
	tenantA, tenantB := uint64(10), uint64(20)
	seen := time.Date(2026, 10, 29, 1, 0, 0, 0, time.UTC)
	repo := &offlineRepo{moved: []domain.StaleDevice{
		{ID: 1, TenantID: &tenantA, SerialNumber: "A1", LastInformAt: &seen},
		{ID: 2, TenantID: &tenantB, SerialNumber: "B1", LastInformAt: &seen},
		{ID: 3, TenantID: nil, SerialNumber: "ORPHAN", LastInformAt: &seen}, // tanpa tenant: tak ada kanal tujuan
	}}
	pub := &fakePublisher{}
	svc := &Service{devices: repo, refs: fakeRefs{}}
	svc.WithPublisher(pub)

	n, err := svc.MarkStaleOffline(context.Background(), 15*time.Minute)
	if err != nil || n != 3 {
		t.Fatalf("n=%d err=%v, mau 3 perangkat berpindah", n, err)
	}
	if repo.gotMin != 15*time.Minute || repo.gotNow.IsZero() {
		t.Errorf("repo menerima min=%v now=%v", repo.gotMin, repo.gotNow)
	}
	if len(pub.events) != 2 {
		t.Fatalf("siaran = %d, mau 2 (perangkat tanpa tenant tidak disiarkan): %+v", len(pub.events), pub.events)
	}
	for _, e := range pub.events {
		if e.typ != "DEVICE_OFFLINE" {
			t.Errorf("tipe event = %q", e.typ)
		}
	}
	if pub.events[0].tenant != tenantA || pub.events[0].payload["device_id"] != uint64(1) || pub.events[0].payload["serial_number"] != "A1" {
		t.Errorf("event pertama salah tenant/payload: %+v", pub.events[0])
	}
	if pub.events[1].tenant != tenantB {
		t.Errorf("event kedua harus ke tenant B (tidak bocor lintas tenant): %+v", pub.events[1])
	}
}

func TestMarkStaleOffline_GalatRepoTidakMenyiarkan(t *testing.T) {
	boom := errors.New("db mati")
	tenant := uint64(1)
	repo := &offlineRepo{err: boom, moved: []domain.StaleDevice{{ID: 1, TenantID: &tenant}}}
	pub := &fakePublisher{}
	svc := &Service{devices: repo, refs: fakeRefs{}}
	svc.WithPublisher(pub)

	if n, err := svc.MarkStaleOffline(context.Background(), time.Minute); !errors.Is(err, boom) || n != 0 {
		t.Fatalf("n=%d err=%v, mau galat repo diteruskan", n, err)
	}
	if len(pub.events) != 0 {
		t.Errorf("tidak boleh menyiarkan saat gagal: %+v", pub.events)
	}
}

func TestMarkStaleOffline_TanpaPublisherTetapBerjalan(t *testing.T) {
	tenant := uint64(1)
	svc := &Service{devices: &offlineRepo{moved: []domain.StaleDevice{{ID: 1, TenantID: &tenant}}}, refs: fakeRefs{}}
	if n, err := svc.MarkStaleOffline(context.Background(), time.Minute); err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
}

func TestSetInformInterval_NolDisimpanSebagaiTidakDiketahui(t *testing.T) {
	repo := &offlineRepo{}
	svc := &Service{devices: repo}
	if err := svc.SetInformInterval(context.Background(), 9, 0); err != nil || repo.gotSec != nil || repo.gotID != 9 {
		t.Fatalf("0 harus nil: id=%d sec=%v err=%v", repo.gotID, repo.gotSec, err)
	}
	if err := svc.SetInformInterval(context.Background(), 9, 43200); err != nil || repo.gotSec == nil || *repo.gotSec != 43200 {
		t.Fatalf("43200 harus tersimpan: %v err=%v", repo.gotSec, err)
	}
}
