package passwordhash

import (
	"errors"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func TestHashVerifyRoundTrip(t *testing.T) {
	h, err := Hash("administrator")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "$argon2id$v=19$m=65536,t=3,p=2$") {
		t.Fatalf("format PHC tidak sesuai: %s", h)
	}
	ok, rehash, err := Verify("administrator", h)
	if err != nil || !ok || rehash {
		t.Fatalf("password benar: ok=%v rehash=%v err=%v", ok, rehash, err)
	}
	if ok, _, err := Verify("Administrator", h); err != nil || ok {
		t.Fatalf("password salah harus ok=false tanpa error: ok=%v err=%v", ok, err)
	}
	// Salt acak: dua hash dari password sama harus berbeda.
	h2, _ := Hash("administrator")
	if h == h2 {
		t.Fatal("salt harus acak per hash")
	}
}

func TestVerifyLegacyBcryptNeedsRehash(t *testing.T) {
	b, err := bcrypt.GenerateFromPassword([]byte("lama-sekali"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	ok, rehash, err := Verify("lama-sekali", string(b))
	if err != nil || !ok || !rehash {
		t.Fatalf("bcrypt benar: ok=%v rehash=%v err=%v", ok, rehash, err)
	}
	if ok, _, _ := Verify("salah", string(b)); ok {
		t.Fatal("bcrypt dengan password salah harus gagal")
	}
}

func TestVerifyWeakerArgonParamsNeedsRehash(t *testing.T) {
	salt := []byte("0123456789abcdef")
	key, _ := derive([]byte("pw-lemah"), salt, 1, 8*1024, 1, keyLen)
	weak := encode(key, salt, 1, 8*1024, 1)
	ok, rehash, err := Verify("pw-lemah", weak)
	if err != nil || !ok || !rehash {
		t.Fatalf("parameter lemah: ok=%v rehash=%v err=%v", ok, rehash, err)
	}
}

func TestVerifyInvalidHash(t *testing.T) {
	for _, bad := range []string{
		"", "plaintext", "$argon2id$", "$argon2id$v=19$m=1,t=1,p=1$c2FsdA$",
		"$argon2id$v=18$m=65536,t=3,p=2$c2FsdA$aGFzaA",
		"$argon2id$v=19$m=999999999,t=3,p=2$c2FsdA$aGFzaA", // m melebihi batas
		"$argon2id$v=19$m=65536,t=3,p=2$!!!$aGFzaA",        // salt bukan base64
	} {
		if _, _, err := Verify("x", bad); err == nil {
			t.Errorf("hash %q seharusnya error", bad)
		}
	}
}

func TestPasswordTooLong(t *testing.T) {
	long := strings.Repeat("a", MaxPasswordLen+1)
	if _, err := Hash(long); err != ErrPasswordTooLong {
		t.Fatalf("Hash harus ErrPasswordTooLong, dapat %v", err)
	}
	h, _ := Hash("ok-password")
	if ok, _, err := Verify(long, h); ok || err != nil {
		t.Fatalf("Verify password terlalu panjang: ok=%v err=%v", ok, err)
	}
	DummyVerify(long) // tidak boleh panic
}

// Antrean penuh → ErrBusy (bukan menggantung selamanya, bukan "password salah").
func TestBusyWhenSemaphoreExhausted(t *testing.T) {
	h, _ := Hash("password-ok-ok")
	old := busyTimeout
	busyTimeout = 50 * time.Millisecond
	defer func() { busyTimeout = old }()
	for i := 0; i < maxConcurrent; i++ {
		sem <- struct{}{}
	}
	defer func() {
		for i := 0; i < maxConcurrent; i++ {
			<-sem
		}
	}()
	if _, err := Hash("x"); !errors.Is(err, ErrBusy) {
		t.Fatalf("Hash saat penuh: %v", err)
	}
	ok, _, err := Verify("password-ok-ok", h)
	if ok || !errors.Is(err, ErrBusy) {
		t.Fatalf("Verify saat penuh harus ErrBusy (bukan salah-password): ok=%v err=%v", ok, err)
	}
}
