// Command seal-secrets mengenkripsi (AES-GCM, kunci ACS_CREDENTIAL_ENC_KEY) nilai rahasia yang
// MASIH plaintext di provisioning_profile_parameters dan presets.configurations — langkah sekali
// jalan setelah meng-upgrade ke versi yang mengenkripsi nilai rahasia at-rest. Idempoten: nilai yang
// sudah tersegel dilewati; aman dijalankan ulang.
//
// PERINGATAN: setelah dijalankan, versi acsd LAMA (tanpa dukungan enkripsi) tidak boleh dipakai pada
// DB yang sama (akan mengirim ciphertext ke CPE). Backup DB dulu; kunci enkripsi yang sama WAJIB
// dipakai acsd (kehilangan kunci = nilai rahasia tak terbaca).
//
// Usage (di container acsd):
//
//	docker compose run --rm --no-deps --entrypoint /app/seal-secrets acsd
package main

import (
	"context"
	"log"

	"acs/internal/config"
	"acs/internal/repository/mysql"
	"acs/pkg/cryptoutil"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	enc, err := cryptoutil.NewEncryptor(cfg.CredentialEncKey)
	if err != nil {
		log.Fatalf("cryptoutil: %v", err)
	}
	db, err := mysql.Connect(cfg.DBDSN)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer db.Close()

	profiles, presets, err := mysql.SealExistingSecrets(context.Background(), db, enc)
	if err != nil {
		log.Fatalf("seal-secrets: %v (sebagian baris mungkin sudah terenkripsi; aman dijalankan ulang)", err)
	}
	log.Printf("seal-secrets: selesai — %d parameter profil & %d preset dienkripsi", profiles, presets)
}
