// Package cryptoutil menyediakan enkripsi kredensial (connection request,
// inform auth) sesuai kebijakan keamanan di TECH.md §8 — tidak pernah
// menyimpan kredensial CPE dalam bentuk plaintext di database.
package cryptoutil

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"io"
)

type Encryptor struct {
	gcm cipher.AEAD
}

// NewEncryptor membuat Encryptor AES-256-GCM. key harus tepat 32 byte.
func NewEncryptor(key []byte) (*Encryptor, error) {
	if len(key) != 32 {
		return nil, errors.New("cryptoutil: key harus 32 byte (AES-256)")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Encryptor{gcm: gcm}, nil
}

// Encrypt mengembalikan nonce||ciphertext. String kosong menghasilkan nil
// (dipetakan ke NULL di kolom VARBINARY nullable).
func (e *Encryptor) Encrypt(plaintext string) ([]byte, error) {
	if plaintext == "" {
		return nil, nil
	}
	nonce := make([]byte, e.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return e.gcm.Seal(nonce, nonce, []byte(plaintext), nil), nil
}

func (e *Encryptor) Decrypt(ciphertext []byte) (string, error) {
	if len(ciphertext) == 0 {
		return "", nil
	}
	ns := e.gcm.NonceSize()
	if len(ciphertext) < ns {
		return "", errors.New("cryptoutil: ciphertext terlalu pendek")
	}
	nonce, data := ciphertext[:ns], ciphertext[ns:]
	plain, err := e.gcm.Open(nil, nonce, data, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}
