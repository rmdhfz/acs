package auth

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"acs/internal/domain"
	"acs/internal/usecase/notify"
	"acs/pkg/mailer"
)

type recNotifier struct {
	mu  sync.Mutex
	evs []domain.NotificationEvent
	err error
}

func (r *recNotifier) Notify(_ context.Context, ev domain.NotificationEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.evs = append(r.evs, ev)
	return r.err
}

type nopMailer struct{}

func (nopMailer) Notify(mailer.Message) {}

// Impersonate: pengguna TARGET diberi tahu (bukan SUPERADMIN), dengan email ikut dikirim, dan isinya tidak memuat token.
func TestAlert_ImpersonateMemberiTahuTarget(t *testing.T) {
	svc, _, super := impFixture(t)
	rec := &recNotifier{}
	svc.WithNotifier(rec)
	res, err := svc.Impersonate(context.Background(), super, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(rec.evs) != 1 {
		t.Fatalf("harus 1 notifikasi, dapat %d", len(rec.evs))
	}
	ev := rec.evs[0]
	if ev.TypeCode != domain.NotifSecurityAlert || ev.Audience.Kind != domain.AudienceSelf || ev.Audience.UserID != 2 {
		t.Fatalf("penerima harus target (user 2) lewat SECURITY_ALERT: %+v", ev)
	}
	if ev.NoEmail {
		t.Fatal("impersonate: email harus ikut dikirim")
	}
	if strings.Contains(ev.Body, res.Token) || strings.Contains(ev.Title+ev.Body, "root") {
		t.Fatalf("isi tidak boleh memuat token maupun identitas operator: %q", ev.Body)
	}
	if !strings.Contains(ev.Body, "SUPERADMIN") || !strings.Contains(ev.Body, "Waktu:") {
		t.Fatalf("isi harus menyebut jenis kejadian dan waktu: %q", ev.Body)
	}
	// penolakan impersonate tidak boleh menghasilkan notifikasi
	rec.evs = nil
	if _, err := svc.Impersonate(context.Background(), super, 3); !errors.Is(err, ErrImpersonationForbidden) {
		t.Fatal(err)
	}
	if len(rec.evs) != 0 {
		t.Fatal("impersonate ditolak tidak boleh menotifikasi siapa pun")
	}
}

// Akun terkunci: NoEmail mengikuti ada-tidaknya email transaksional; notifikasi hanya SEKALI (saat kunci terjadi).
func TestAlert_AkunTerkunci(t *testing.T) {
	for _, withMail := range []bool{false, true} {
		user := &domain.User{ID: 1, Username: "alice", PasswordHash: hashFor(t, "correct-password"), IsActive: true}
		users := &fakeUserRepoAuth{byID: map[uint64]*domain.User{1: user}, byUsername: map[string]*domain.User{"alice": user}}
		svc := NewService(users, &fakeTokenRepoAuth{}, &fakeTenantRepoAuthTest{}, &fakeActivityRepoAuth{}, []byte("test-secret-at-least-32-bytes!!"), time.Hour, "", "", "", "")
		rec := &recNotifier{}
		svc.WithNotifier(rec)
		if withMail {
			svc.WithPasswordReset(nil, nopMailer{}, notify.NewBuilder("ACS", "https://x.test"), time.Minute)
		}
		for i := 0; i < maxFailedLoginAttempts+2; i++ {
			_, _, _ = svc.Login(context.Background(), "alice", "salah")
		}
		if len(rec.evs) != 1 {
			t.Fatalf("withMail=%v: notifikasi kunci harus tepat 1, dapat %d", withMail, len(rec.evs))
		}
		if rec.evs[0].NoEmail != withMail {
			t.Fatalf("withMail=%v: NoEmail=%v (email transaksional sudah ada -> NoEmail)", withMail, rec.evs[0].NoEmail)
		}
		if rec.evs[0].Audience.UserID != 1 {
			t.Fatal("penerima harus pemilik akun")
		}
	}
}

// Ganti password: notifikasi tanpa memuat password lama/baru; kegagalan notifier tidak menggagalkan alur utama.
func TestAlert_GantiPasswordTanpaRahasiaDanTahanGagal(t *testing.T) {
	user := &domain.User{ID: 1, Username: "alice", PasswordHash: hashFor(t, "old-password"), IsActive: true}
	users := &fakeUserRepoAuth{byID: map[uint64]*domain.User{1: user}}
	svc := NewService(users, &fakeTokenRepoAuth{}, &fakeTenantRepoAuthTest{}, &fakeActivityRepoAuth{}, []byte("test-secret-at-least-32-bytes!!"), time.Hour, "", "", "", "")
	rec := &recNotifier{err: errors.New("notifikasi rusak")}
	svc.WithNotifier(rec)
	tok, err := svc.ChangeOwnPassword(context.Background(), domain.Actor{UserID: 1}, "old-password", "password-baru-yang-panjang")
	if err != nil || tok == "" {
		t.Fatalf("notifier gagal tidak boleh menggagalkan ganti password: %v", err)
	}
	if len(rec.evs) != 1 {
		t.Fatalf("1 notifikasi, dapat %d", len(rec.evs))
	}
	all := rec.evs[0].Title + rec.evs[0].Body
	if strings.Contains(all, "old-password") || strings.Contains(all, "password-baru-yang-panjang") {
		t.Fatal("notifikasi tidak boleh memuat password")
	}
	if rec.evs[0].NoEmail != false {
		t.Fatal("tanpa email transaksional (SMTP mati) NoEmail=false agar kanal email notifikasi bisa dipakai")
	}
	// tanpa notifier terpasang (nil) tetap aman
	svc2 := NewService(users, &fakeTokenRepoAuth{}, &fakeTenantRepoAuthTest{}, &fakeActivityRepoAuth{}, []byte("test-secret-at-least-32-bytes!!"), time.Hour, "", "", "", "")
	if _, err := svc2.ChangeOwnPassword(context.Background(), domain.Actor{UserID: 1}, "password-baru-yang-panjang", "password-lain-yang-panjang"); err != nil {
		t.Fatalf("tanpa notifier harus tetap jalan: %v", err)
	}
}
