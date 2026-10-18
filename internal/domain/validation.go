package domain

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Validator teks bersama untuk usecase. SATU implementasi dipakai jalur Create DAN Update supaya aturan tidak
// bisa lolos lewat PATCH/PUT (temuan audit validasi S3/S4). Semua error membungkus ErrInvalidInput (→ HTTP 400).

// MaxUserPasswordLen = batas atas password akun (karakter/byte) -- sama dengan passwordhash.MaxPasswordLen, agar
// input raksasa tak dipakai sebagai vektor DoS di Argon2id. Satu sumber untuk semua jalur ubah-password.
const MaxUserPasswordLen = 128

var (
	usernameRe   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._@+-]{2,63}$`)
	tenantCodeRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{1,31}$`)
)

// CleanText memangkas spasi tepi lalu memeriksa panjang (dalam KARAKTER, sesuai kolom VARCHAR), UTF-8 valid, dan
// ketiadaan karakter kontrol. allowNewline=true mengizinkan \n dan \t (catatan multi-baris).
func CleanText(field, v string, minRunes, maxRunes int, allowNewline bool) (string, error) {
	v = strings.TrimSpace(v)
	if !utf8.ValidString(v) {
		return "", fmt.Errorf("%w: %s bukan teks UTF-8 yang valid", ErrInvalidInput, field)
	}
	n := utf8.RuneCountInString(v)
	if n < minRunes {
		if minRunes == 1 {
			return "", fmt.Errorf("%w: %s wajib diisi", ErrInvalidInput, field)
		}
		return "", fmt.Errorf("%w: %s minimal %d karakter", ErrInvalidInput, field, minRunes)
	}
	if n > maxRunes {
		return "", fmt.Errorf("%w: %s maksimal %d karakter", ErrInvalidInput, field, maxRunes)
	}
	for _, r := range v {
		if unicode.IsControl(r) && !(allowNewline && (r == '\n' || r == '\t')) {
			return "", fmt.Errorf("%w: %s tidak boleh memuat karakter kontrol", ErrInvalidInput, field)
		}
	}
	return v, nil
}

// ValidateUsername: 3-64 karakter [A-Za-z0-9._@+-], diawali huruf/angka (tanpa spasi/kontrol/unicode mirip).
func ValidateUsername(v string) (string, error) {
	v = strings.TrimSpace(v)
	if !usernameRe.MatchString(v) {
		return "", fmt.Errorf("%w: username 3-64 karakter, hanya huruf, angka, titik, garis bawah, strip, @ dan +", ErrInvalidInput)
	}
	return v, nil
}

// ValidateTenantCode: 2-32 karakter [A-Za-z0-9._-], diawali huruf/angka, tanpa spasi.
func ValidateTenantCode(v string) (string, error) {
	v = strings.TrimSpace(v)
	if !tenantCodeRe.MatchString(v) {
		return "", fmt.Errorf("%w: code tenant 2-32 karakter, hanya huruf, angka, titik, garis bawah dan strip", ErrInvalidInput)
	}
	return v, nil
}

// ValidatePassword menegakkan kebijakan password akun SATU kali untuk semua jalur (buat user, reset admin, ganti
// sendiri, reset via email): minimal MinUserPasswordLen, maksimal MaxUserPasswordLen byte, dan tidak sama dengan
// username/email akun (perbandingan tanpa peduli huruf besar-kecil). identifiers boleh kosong.
func ValidatePassword(pw string, identifiers ...string) error {
	if len(pw) < MinUserPasswordLen {
		return fmt.Errorf("%w: password minimal %d karakter", ErrInvalidInput, MinUserPasswordLen)
	}
	if len(pw) > MaxUserPasswordLen {
		return fmt.Errorf("%w: password maksimal %d karakter", ErrInvalidInput, MaxUserPasswordLen)
	}
	for _, id := range identifiers {
		if id != "" && strings.EqualFold(pw, id) {
			return fmt.Errorf("%w: password tidak boleh sama dengan username atau email", ErrInvalidInput)
		}
	}
	return nil
}
