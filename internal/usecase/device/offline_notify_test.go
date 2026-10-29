package device

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"acs/internal/domain"
)

type recWebhook struct {
	calls []whCall
	err   error
}

type whCall struct {
	event  string
	tenant *uint64
	data   map[string]any
}

func (r *recWebhook) Enqueue(_ context.Context, code string, tenantID *uint64, payload any) error {
	m, _ := payload.(map[string]any)
	r.calls = append(r.calls, whCall{code, tenantID, m})
	return r.err
}

func stale(id uint64, tenant *uint64, serial string, seen *time.Time) domain.StaleDevice {
	return domain.StaleDevice{ID: id, DeviceUUID: fmt.Sprintf("uuid-%d", id), TenantID: tenant, SerialNumber: serial, LastInformAt: seen}
}

func TestMarkStaleOffline_WebhookPerPerangkatDanRingkasanPerTenant(t *testing.T) {
	a, b := uint64(10), uint64(20)
	seen := time.Date(2026, 10, 29, 1, 0, 0, 0, time.UTC)
	repo := &offlineRepo{moved: []domain.StaleDevice{
		stale(1, &a, "A1", &seen), stale(2, &a, "A2", &seen), stale(3, &b, "B1", nil),
		stale(4, nil, "ORPHAN", &seen), // tanpa tenant: tak ada penerima
	}}
	wh, rec := &recWebhook{}, &recNotifier{}
	svc := &Service{devices: repo, refs: fakeRefs{}}
	svc.WithWebhooks(wh).WithNotifier(rec)

	if n, err := svc.MarkStaleOffline(context.Background(), time.Minute); err != nil || n != 4 {
		t.Fatalf("n=%d err=%v", n, err)
	}

	// Webhook: satu per perangkat BERTENANT, berisi uuid publik (bukan PK) dan tenant yang benar.
	if len(wh.calls) != 3 {
		t.Fatalf("webhook = %d, mau 3 (perangkat tanpa tenant dilewati): %+v", len(wh.calls), wh.calls)
	}
	for i, c := range wh.calls {
		if c.event != domain.WebhookEventDeviceOffline {
			t.Errorf("event[%d] = %q", i, c.event)
		}
		if c.tenant == nil {
			t.Fatalf("event[%d] tanpa tenant", i)
		}
		if _, leaks := c.data["device_id"]; leaks {
			t.Errorf("payload tidak boleh memuat PK numerik: %+v", c.data)
		}
	}
	if wh.calls[0].data["device_uuid"] != "uuid-1" || wh.calls[0].data["serial_number"] != "A1" || *wh.calls[0].tenant != a {
		t.Errorf("event pertama salah: %+v tenant=%d", wh.calls[0].data, *wh.calls[0].tenant)
	}
	if wh.calls[0].data["last_inform_at"] != "2026-10-29T01:00:00Z" {
		t.Errorf("last_inform_at = %v", wh.calls[0].data["last_inform_at"])
	}
	if wh.calls[2].data["last_inform_at"] != nil || *wh.calls[2].tenant != b {
		t.Errorf("perangkat B1 (tanpa last_inform) / tenant salah: %+v", wh.calls[2])
	}

	// Notifikasi: diringkas — tepat satu per tenant (bukan per perangkat), urutan tenant stabil.
	if len(rec.evs) != 2 {
		t.Fatalf("notifikasi = %d, mau 2 (satu per tenant): %+v", len(rec.evs), rec.evs)
	}
	if *rec.evs[0].TenantID != a || *rec.evs[1].TenantID != b {
		t.Errorf("tenant notifikasi = %d,%d", *rec.evs[0].TenantID, *rec.evs[1].TenantID)
	}
	if rec.evs[0].Title != "2 perangkat offline" || !strings.Contains(rec.evs[0].Body, "A1, A2") {
		t.Errorf("ringkasan tenant A salah: %q / %q", rec.evs[0].Title, rec.evs[0].Body)
	}
	if strings.Contains(rec.evs[0].Body, "B1") || strings.Contains(rec.evs[0].Body, "ORPHAN") {
		t.Errorf("ringkasan tenant A membocorkan perangkat tenant lain: %q", rec.evs[0].Body)
	}
	// Satu perangkat: judul menyebut serial dan tautan ke perangkat itu.
	if rec.evs[1].Title != "Perangkat offline: B1" || rec.evs[1].Link != "/devices/3" || rec.evs[1].EntityID == nil || *rec.evs[1].EntityID != 3 {
		t.Errorf("notifikasi tunggal salah: %+v", rec.evs[1])
	}
	for _, e := range rec.evs {
		if e.TypeCode != domain.NotifDeviceOffline {
			t.Errorf("jenis notifikasi = %q", e.TypeCode)
		}
	}
}

func TestOfflineNotice_BanyakPerangkatDipotongTapiJumlahBenar(t *testing.T) {
	tenant := uint64(1)
	var devs []domain.StaleDevice
	for i := 1; i <= 8; i++ {
		devs = append(devs, stale(uint64(i), &tenant, fmt.Sprintf("SN%d", i), nil))
	}
	ev := offlineNotice(&tenant, devs)
	if ev.Title != "8 perangkat offline" {
		t.Errorf("judul = %q", ev.Title)
	}
	if !strings.Contains(ev.Body, "SN1, SN2, SN3, SN4, SN5 dan 3 lainnya") || strings.Contains(ev.Body, "SN6") {
		t.Errorf("isi harus menyebut 5 serial + sisa dihitung: %q", ev.Body)
	}
	if ev.EntityID != nil {
		t.Errorf("ringkasan banyak perangkat tidak boleh menunjuk satu entitas: %+v", ev.EntityID)
	}
}

func TestMarkStaleOffline_GalatWebhookDanNotifikasiTidakMenggagalkanSapuan(t *testing.T) {
	tenant := uint64(1)
	repo := &offlineRepo{moved: []domain.StaleDevice{stale(1, &tenant, "A1", nil)}}
	svc := &Service{devices: repo, refs: fakeRefs{}}
	svc.WithWebhooks(&recWebhook{err: errors.New("antrean webhook rusak")}).WithNotifier(&recNotifier{err: errors.New("notifikasi rusak")})
	if n, err := svc.MarkStaleOffline(context.Background(), time.Minute); err != nil || n != 1 {
		t.Fatalf("sapuan harus tetap berhasil: n=%d err=%v", n, err)
	}
}

func TestMarkStaleOffline_TidakAdaYangBerpindah_TidakMemberitahu(t *testing.T) {
	wh, rec := &recWebhook{}, &recNotifier{}
	svc := &Service{devices: &offlineRepo{}, refs: fakeRefs{}}
	svc.WithWebhooks(wh).WithNotifier(rec)
	if n, err := svc.MarkStaleOffline(context.Background(), time.Minute); err != nil || n != 0 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if len(wh.calls)+len(rec.evs) != 0 {
		t.Errorf("tanpa perangkat berpindah tidak boleh ada pemberitahuan: %+v %+v", wh.calls, rec.evs)
	}
}

func TestMarkStaleOffline_GalatRepoTidakMemberitahu(t *testing.T) {
	tenant := uint64(1)
	wh, rec := &recWebhook{}, &recNotifier{}
	svc := &Service{devices: &offlineRepo{err: errors.New("db mati"), moved: []domain.StaleDevice{stale(1, &tenant, "A1", nil)}}, refs: fakeRefs{}}
	svc.WithWebhooks(wh).WithNotifier(rec)
	if _, err := svc.MarkStaleOffline(context.Background(), time.Minute); err == nil {
		t.Fatal("galat repo harus diteruskan")
	}
	if len(wh.calls)+len(rec.evs) != 0 {
		t.Errorf("tidak boleh memberitahu saat sapuan gagal: %+v %+v", wh.calls, rec.evs)
	}
}
