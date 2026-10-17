// Package passwordhash menyimpan & memverifikasi password dengan Argon2id
// (RFC 9106, pemenang Password Hashing Competition) dalam format PHC string:
//
//	$argon2id$v=19$m=65536,t=3,p=2$<salt b64>$<hash b64>
//
// Parameter ikut tersimpan di setiap hash, jadi biaya bisa dinaikkan nanti
// tanpa merusak hash lama — Verify melaporkan needsRehash=true bila parameter
// hash lebih rendah dari standar saat ini.
//
// Kompatibilitas mundur: hash bcrypt lama ($2a$/$2b$/$2y$) tetap bisa
// diverifikasi (needsRehash=true), supaya akun yang dibuat sebelum migrasi ke
// Argon2id bisa login dan hash-nya otomatis di-upgrade saat login berhasil.
package passwordhash

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/bcrypt"
)

// Parameter standar saat ini. 64 MiB / 3 iterasi / 2 lane sejalan dengan
// rekomendasi RFC 9106 §4 (opsi memori-terbatas) dan jauh di atas minimum
// OWASP (19 MiB, t=2, p=1). Ubah di sini untuk menaikkan biaya; hash lama
// otomatis di-upgrade saat login (needsRehash).
const (
	timeCost  uint32 = 3
	memoryKiB uint32 = 64 * 1024
	threads   uint8  = 2
	keyLen    uint32 = 32
	saltLen          = 16
	maxParamM uint32 = 512 * 1024 // batas wajar saat mem-parse hash dari DB
	maxParamT uint32 = 12
	maxParamP uint8  = 16
	maxKeyLen uint32 = 128
	// MaxPasswordLen membatasi input agar tidak dipakai sebagai vektor DoS
	// (hashing string raksasa). 128 byte cukup untuk passphrase panjang.
	MaxPasswordLen = 128
	// maxConcurrent membatasi jumlah hashing Argon2 simultan: tiap operasi
	// memakai ~64 MiB, jadi tanpa batas ini banjir login bisa menghabiskan RAM.
	maxConcurrent = 4
)

var (
	// ErrPasswordTooLong — password melebihi MaxPasswordLen.
	ErrPasswordTooLong = errors.New("passwordhash: password terlalu panjang")
	// ErrBusy — antrean hashing penuh terlalu lama (beban tinggi / serangan).
	// Pemanggil HARUS memetakannya ke 503 dan TIDAK menganggapnya password salah.
	ErrBusy = errors.New("passwordhash: server sibuk, coba lagi sebentar")
	// ErrInvalidHash — string hash bukan format yang dikenali.
	ErrInvalidHash = errors.New("passwordhash: format hash tidak valid")

	sem = make(chan struct{}, maxConcurrent)

	// busyTimeout — batas menunggu slot hashing sebelum menyerah (ErrBusy).
	// var (bukan const) agar bisa dipendekkan di test.
	busyTimeout = 8 * time.Second
	// dummyHash dipakai DummyVerify (meratakan waktu respons untuk user tak
	// dikenal). Dibangkitkan sekali saat init; password-nya acak & dibuang.
	dummyHash string
)

func init() {
	salt := make([]byte, saltLen)
	_, _ = rand.Read(salt)
	key, _ := derive([]byte("dummy-password-never-used"), salt, timeCost, memoryKiB, threads, keyLen)
	dummyHash = encode(key, salt, timeCost, memoryKiB, threads)
}

func derive(password, salt []byte, t, m uint32, p uint8, n uint32) ([]byte, error) {
	timer := time.NewTimer(busyTimeout)
	defer timer.Stop()
	select {
	case sem <- struct{}{}:
	case <-timer.C:
		return nil, ErrBusy
	}
	defer func() { <-sem }()
	return argon2.IDKey(password, salt, t, m, p, n), nil
}

func encode(key, salt []byte, t, m uint32, p uint8) string {
	b64 := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, m, t, p, b64.EncodeToString(salt), b64.EncodeToString(key))
}

// Hash menghasilkan hash Argon2id dengan salt acak 16 byte.
func Hash(password string) (string, error) {
	if len(password) > MaxPasswordLen {
		return "", ErrPasswordTooLong
	}
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("passwordhash: gagal membangkitkan salt: %w", err)
	}
	key, err := derive([]byte(password), salt, timeCost, memoryKiB, threads, keyLen)
	if err != nil {
		return "", err
	}
	return encode(key, salt, timeCost, memoryKiB, threads), nil
}

// Verify memeriksa password terhadap hash tersimpan (Argon2id atau bcrypt lama).
// needsRehash=true berarti password BENAR tetapi hash perlu diganti dengan
// Hash() — karena masih bcrypt atau parameter Argon2id-nya lebih lemah.
// Selalu ok=false (tanpa error) untuk password salah; error hanya untuk hash
// tersimpan yang tidak bisa dibaca (ErrInvalidHash) atau antrean penuh (ErrBusy).
func Verify(password, encoded string) (ok, needsRehash bool, err error) {
	if len(password) > MaxPasswordLen {
		return false, false, nil
	}
	switch {
	case strings.HasPrefix(encoded, "$argon2id$"):
		return verifyArgon2id(password, encoded)
	case strings.HasPrefix(encoded, "$2a$"), strings.HasPrefix(encoded, "$2b$"), strings.HasPrefix(encoded, "$2y$"):
		if bcrypt.CompareHashAndPassword([]byte(encoded), []byte(password)) != nil {
			return false, false, nil
		}
		return true, true, nil
	}
	return false, false, ErrInvalidHash
}

func verifyArgon2id(password, encoded string) (bool, bool, error) {
	// ["", "argon2id", "v=19", "m=..,t=..,p=..", salt, hash]
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 {
		return false, false, ErrInvalidHash
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false, false, ErrInvalidHash
	}
	var m, t uint32
	var p uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil {
		return false, false, ErrInvalidHash
	}
	if m == 0 || t == 0 || p == 0 || m > maxParamM || t > maxParamT || p > maxParamP {
		return false, false, ErrInvalidHash
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) == 0 {
		return false, false, ErrInvalidHash
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(want) == 0 || uint32(len(want)) > maxKeyLen {
		return false, false, ErrInvalidHash
	}
	got, err := derive([]byte(password), salt, t, m, p, uint32(len(want)))
	if err != nil {
		return false, false, err // ErrBusy: BUKAN password salah
	}
	if subtle.ConstantTimeCompare(got, want) != 1 {
		return false, false, nil
	}
	return true, m < memoryKiB || t < timeCost || p < threads || uint32(len(want)) < keyLen, nil
}

// DummyVerify menjalankan satu verifikasi Argon2id palsu agar waktu respons
// login untuk email/username TAK DIKENAL mirip dengan yang dikenal (mencegah
// enumerasi akun lewat timing).
func DummyVerify(password string) {
	if len(password) > MaxPasswordLen {
		password = password[:MaxPasswordLen]
	}
	_, _, _ = verifyArgon2id(password, dummyHash)
}
