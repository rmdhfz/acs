package device

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"acs/internal/domain"
)

type recNotifier struct {
	evs []domain.NotificationEvent
	err error
}

func (r *recNotifier) Notify(_ context.Context, ev domain.NotificationEvent) error {
	r.evs = append(r.evs, ev)
	return r.err
}

// Inform perangkat BARU ditolak karena batas perangkat: ADMIN tenant + SUPERADMIN diberi tahu, dedupe 24 jam per
// tenant, isi tidak memuat data dari CPE (serial/OUI), dan kegagalan notifier tidak mengubah hasil (tetap kuota).
func TestInformDitolakBatas_MemberiTahu(t *testing.T) {
	ctx := context.Background()
	tenant := ptr(uint64(7))
	devs := &fakeDevices{}
	svc := newTestService(devs, &fakeOUIs{}, &fakeModels{})
	svc.WithQuota(&fakeQuota{max: u32(3), cur: 3})
	rec := &recNotifier{err: errors.New("notifikasi rusak")}
	svc.WithNotifier(rec)

	info := informInfo()
	info.TenantID = tenant
	if _, _, err := svc.FindOrCreateFromInform(ctx, info); !errors.Is(err, domain.ErrQuotaExceeded) {
		t.Fatalf("hasil utama harus tetap ErrQuotaExceeded: %v", err)
	}
	if len(rec.evs) != 1 {
		t.Fatalf("harus 1 notifikasi, dapat %d", len(rec.evs))
	}
	ev := rec.evs[0]
	if ev.TypeCode != domain.NotifDeviceLimitReached || ev.TenantID == nil || *ev.TenantID != 7 {
		t.Fatalf("jenis/tenant salah: %+v", ev)
	}
	if ev.DedupeKey == "" || ev.DedupeWindow != 24*time.Hour {
		t.Fatalf("harus dedupe 24 jam: key=%q window=%v", ev.DedupeKey, ev.DedupeWindow)
	}
	if strings.Contains(ev.Title+ev.Body, info.SerialNumber) || (info.OUI != "" && strings.Contains(ev.Title+ev.Body, info.OUI)) {
		t.Fatal("notifikasi tidak boleh memuat serial/OUI dari CPE")
	}

	// masih ada ruang -> tidak ada notifikasi
	rec.evs = nil
	svc.WithQuota(&fakeQuota{max: u32(3), cur: 1})
	if _, _, err := svc.FindOrCreateFromInform(ctx, info); err != nil {
		t.Fatal(err)
	}
	if len(rec.evs) != 0 {
		t.Fatal("tidak boleh ada notifikasi bila belum penuh")
	}

	// tanpa notifier terpasang tetap aman
	plain := newTestService(&fakeDevices{}, &fakeOUIs{}, &fakeModels{})
	plain.WithQuota(&fakeQuota{max: u32(1), cur: 1})
	if _, _, err := plain.FindOrCreateFromInform(ctx, info); !errors.Is(err, domain.ErrQuotaExceeded) {
		t.Fatalf("tanpa notifier: %v", err)
	}
}
