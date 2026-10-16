// Package totp mengimplementasikan TOTP (RFC 6238, HMAC-SHA1, 30 detik, 6 digit) — kompatibel
// dengan Google Authenticator, Microsoft Authenticator, Authy, dsb. Tanpa dependensi eksternal.
package totp

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	Period = 30
	Digits = 6
)

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// GenerateSecret membuat rahasia acak 160 bit (base32, 32 karakter).
func GenerateSecret() (string, error) {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return b32.EncodeToString(b), nil
}

func decode(secret string) ([]byte, error) {
	return b32.DecodeString(strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(secret), " ", "")))
}

// codeAt menghitung kode untuk langkah waktu (counter) tertentu.
func codeAt(key []byte, counter uint64, digits int) string {
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], counter)
	mac := hmac.New(sha1.New, key)
	mac.Write(msg[:])
	sum := mac.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	bin := (uint32(sum[off])&0x7f)<<24 | uint32(sum[off+1])<<16 | uint32(sum[off+2])<<8 | uint32(sum[off+3])
	mod := uint32(1)
	for i := 0; i < digits; i++ {
		mod *= 10
	}
	return fmt.Sprintf("%0*d", digits, bin%mod)
}

// Code — kode TOTP untuk waktu t.
func Code(secret string, t time.Time) (string, error) {
	key, err := decode(secret)
	if err != nil {
		return "", err
	}
	return codeAt(key, uint64(t.Unix()/Period), Digits), nil
}

// Verify memeriksa kode terhadap langkah waktu t±window (window=1 → toleransi ±30 detik untuk
// selisih jam). Mengembalikan langkah (counter) yang cocok; pemanggil WAJIB menolak langkah
// yang sama/lebih lama dari langkah terakhir yang sudah dipakai (anti-replay).
func Verify(secret, code string, t time.Time, window int) (uint64, bool) {
	code = strings.TrimSpace(code)
	if len(code) != Digits {
		return 0, false
	}
	key, err := decode(secret)
	if err != nil {
		return 0, false
	}
	cur := uint64(t.Unix() / Period)
	ok := false
	var matched uint64
	for d := -window; d <= window; d++ {
		step := cur + uint64(int64(d))
		// konstan-waktu: jangan berhenti di kecocokan pertama agar tak membocorkan posisi
		if subtle.ConstantTimeCompare([]byte(codeAt(key, step, Digits)), []byte(code)) == 1 {
			ok, matched = true, step
		}
	}
	return matched, ok
}

// ProvisioningURI — otpauth:// untuk QR/aplikasi autentikator.
func ProvisioningURI(issuer, account, secret string) string {
	label := url.PathEscape(issuer) + ":" + url.PathEscape(account)
	q := url.Values{}
	q.Set("secret", secret)
	q.Set("issuer", issuer)
	q.Set("algorithm", "SHA1")
	q.Set("digits", fmt.Sprint(Digits))
	q.Set("period", fmt.Sprint(Period))
	return "otpauth://totp/" + label + "?" + q.Encode()
}
