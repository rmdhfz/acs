package auth

import (
	"errors"
	"testing"
)

func TestValidateOIDCIdentity(t *testing.T) {
	yes, no := true, false
	tests := []struct {
		name          string
		tokenNonce    string
		expectedNonce string
		verified      *bool
		email         string
		wantErr       error // nil = lolos; sentinel = harus errors.Is; errAny = error apa pun
	}{
		{"semua benar, verified=true", "n1", "n1", &yes, "a@x.id", nil},
		{"verified tidak dikirim IdP (Azure AD) tetap lolos", "n1", "n1", nil, "a@x.id", nil},
		{"nonce beda (replay/injeksi)", "n2", "n1", &yes, "a@x.id", ErrOIDCNonceMismatch},
		{"nonce di token kosong", "", "n1", &yes, "a@x.id", ErrOIDCNonceMismatch},
		{"nonce yang diharapkan kosong (cookie hilang) ditolak walau token kosong", "", "", &yes, "a@x.id", ErrOIDCNonceMismatch},
		{"email belum terverifikasi eksplisit", "n1", "n1", &no, "admin@x.id", errAny},
		{"email kosong", "n1", "n1", &yes, "", errAny},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateOIDCIdentity(tc.tokenNonce, tc.expectedNonce, tc.verified, tc.email)
			switch {
			case tc.wantErr == nil && err != nil:
				t.Fatalf("seharusnya lolos, dapat %v", err)
			case tc.wantErr == errAny && err == nil:
				t.Fatal("seharusnya ditolak")
			case tc.wantErr != nil && tc.wantErr != errAny && !errors.Is(err, tc.wantErr):
				t.Fatalf("want %v, got %v", tc.wantErr, err)
			}
		})
	}
}

var errAny = errors.New("any")
