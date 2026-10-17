package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"acs/internal/domain"
)

func oidcSvc(t *testing.T, issuer string, u *domain.User) (*Service, *fakeUserRepoAuth) {
	t.Helper()
	users := &fakeUserRepoAuth{byID: map[uint64]*domain.User{u.ID: u}, byUsername: map[string]*domain.User{u.Username: u}}
	svc := NewService(users, &fakeTokenRepoAuth{}, &fakeTenantRepoAuthTest{}, &fakeActivityRepoAuth{}, []byte("test-secret-at-least-32-bytes!!"), time.Hour, "", "", "", "")
	// konfigurasi OIDC diset langsung (tanpa discovery jaringan) — yang diuji hanya logika lokal.
	svc.oidcCfg = &oidcSettings{issuer: issuer, clientID: "cid"}
	return svc, users
}

func TestOIDCProvider_GoogleDikenali(t *testing.T) {
	cases := map[string]string{
		"https://accounts.google.com":              "google",
		"https://ACCOUNTS.google.com/":             "google",
		"https://login.microsoftonline.com":        "oidc",
		"https://keycloak.local/realms/x":          "oidc",
		"https://accounts.google.com.evil.example": "oidc", // bukan host Google
	}
	for issuer, want := range cases {
		svc, _ := oidcSvc(t, issuer, &domain.User{ID: 1, Username: "a"})
		if got := svc.OIDCProvider(); got != want {
			t.Errorf("%s: got %q, want %q", issuer, got, want)
		}
	}
	if (&Service{}).OIDCProvider() != "" {
		t.Error("tanpa konfigurasi OIDC harus kosong")
	}
}

func TestEmailDomainAllowed(t *testing.T) {
	allowed := []string{"perusahaan.co.id", "anak.co.id"}
	for email, want := range map[string]bool{
		"budi@perusahaan.co.id":     true,
		"budi@Anak.CO.id":           true,
		"budi@gmail.com":            false,
		"budi@sub.perusahaan.co.id": false,
		"budi@perusahaan.co.id.x":   false,
		"tanpa-at":                  false,
	} {
		if got := emailDomainAllowed(email, allowed); got != want {
			t.Errorf("%q: got %v want %v", email, got, want)
		}
	}
	if !emailDomainAllowed("siapa@saja.com", nil) {
		t.Error("tanpa daftar = tanpa batas")
	}
}

// Login SSO pertama mengikat identitas IdP; berikutnya harus sama. Sub berbeda (email berpindah tangan) ditolak.
func TestVerifyOIDCBinding_IkatLaluCocokkan(t *testing.T) {
	u := &domain.User{ID: 1, Username: "budi", IsActive: true}
	svc, _ := oidcSvc(t, "https://accounts.google.com", u)
	ctx := context.Background()
	const sub = "https://accounts.google.com|1234567890"

	if err := svc.verifyOIDCBinding(ctx, u, sub); err != nil {
		t.Fatalf("login pertama harus lolos & mengikat: %v", err)
	}
	if u.OIDCSubject == nil || *u.OIDCSubject != sub {
		t.Fatalf("ikatan tidak tersimpan: %v", u.OIDCSubject)
	}
	if err := svc.verifyOIDCBinding(ctx, u, sub); err != nil {
		t.Fatalf("login berikutnya dengan sub sama harus lolos: %v", err)
	}
	if err := svc.verifyOIDCBinding(ctx, u, "https://accounts.google.com|9999"); !errors.Is(err, ErrOIDCSubjectMismatch) {
		t.Fatalf("sub berbeda harus ditolak, dapat %v", err)
	}
	// issuer berbeda dengan sub sama juga berbeda identitas
	if err := svc.verifyOIDCBinding(ctx, u, "https://idp-lain.example|1234567890"); !errors.Is(err, ErrOIDCSubjectMismatch) {
		t.Fatalf("issuer berbeda harus ditolak, dapat %v", err)
	}
}

// Kalah balapan: BindOIDCSubject mengembalikan false karena login lain sudah mengikat; hanya sub yang sama yang lolos.
func TestVerifyOIDCBinding_KalahBalapan(t *testing.T) {
	u := &domain.User{ID: 1, Username: "budi", IsActive: true}
	svc, users := oidcSvc(t, "https://accounts.google.com", u)
	winner := "https://accounts.google.com|AAA"
	// pemenang mengikat di repo SETELAH u dimuat (u.OIDCSubject di memori masih nil)
	stored := winner
	users.byID[1] = &domain.User{ID: 1, Username: "budi", IsActive: true, OIDCSubject: &stored}
	if err := svc.verifyOIDCBinding(context.Background(), u, "https://accounts.google.com|BBB"); !errors.Is(err, ErrOIDCSubjectMismatch) {
		t.Fatalf("pecundang dengan sub berbeda harus ditolak: %v", err)
	}
	if err := svc.verifyOIDCBinding(context.Background(), u, winner); err != nil {
		t.Fatalf("sub yang sama dengan pemenang harus lolos: %v", err)
	}
}
