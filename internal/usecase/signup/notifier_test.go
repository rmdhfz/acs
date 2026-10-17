package signup

import (
	"context"
	"errors"
	"strings"
	"testing"

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

// Setelah tenant hasil pendaftaran dibuat, SUPERADMIN diberi tahu (NEW_TENANT_SIGNUP); kegagalan notifier tidak
// menggagalkan verifikasi; isi tidak memuat email/password pendaftar.
func TestVerify_MemberiTahuSuperadmin(t *testing.T) {
	r := newRig(cfgOn)
	rec := &recNotifier{err: errors.New("notifikasi rusak")}
	r.svc.WithNotifier(rec)
	ctx := context.Background()
	if err := r.svc.Request(ctx, goodInput()); err != nil {
		t.Fatal(err)
	}
	if len(rec.evs) != 0 {
		t.Fatal("belum boleh ada notifikasi sebelum email diverifikasi")
	}
	if _, err := r.svc.Verify(ctx, r.token(t)); err != nil {
		t.Fatalf("notifier gagal tidak boleh menggagalkan verifikasi: %v", err)
	}
	if len(rec.evs) != 1 {
		t.Fatalf("harus 1 notifikasi, dapat %d", len(rec.evs))
	}
	ev := rec.evs[0]
	if ev.TypeCode != domain.NotifNewTenantSignup || ev.TenantID == nil || *ev.TenantID != 1 {
		t.Fatalf("jenis/tenant salah: %+v", ev)
	}
	if !strings.Contains(ev.Body, "Rumah Net Sejahtera") {
		t.Fatalf("isi harus menyebut nama tenant: %q", ev.Body)
	}
	if all := ev.Title + ev.Body; strings.Contains(all, "rumahnet.id") || strings.Contains(all, "Kuda-Laut") {
		t.Fatal("notifikasi tidak boleh memuat email/password pendaftar")
	}
}

// Verifikasi yang gagal (token ngawur) tidak menotifikasi.
func TestVerify_GagalTidakMenotifikasi(t *testing.T) {
	r := newRig(cfgOn)
	rec := &recNotifier{}
	r.svc.WithNotifier(rec)
	if _, err := r.svc.Verify(context.Background(), strings.Repeat("x", 30)); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	if len(rec.evs) != 0 {
		t.Fatal("tidak boleh ada notifikasi")
	}
}
