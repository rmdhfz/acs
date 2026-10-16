package mysql

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"

	"acs/internal/domain"
)

// Enkripsi at-rest nilai rahasia konfigurasi (PPPoE/WiFi password, PSK, token…) pada
// provisioning_profile_parameters.parameter_value dan presets.configurations[].value.
//
// Ini detail PENYIMPANAN (transparan bagi usecase): repository menyegel saat menulis dan
// membuka saat membaca, jadi seluruh jalur baca (kirim ke CPE, drift check, API, ekspor)
// selalu melihat plaintext dan tidak ada yang bisa lupa mendekripsi. Format: "enc:v1:" +
// base64(AES-GCM). Baris lama tanpa awalan tetap terbaca (kompatibel mundur) dan dienkripsi
// oleh `seal-secrets`. Yang disegel hanya nilai berparameter bernama sensitif
// (domain.IsSensitiveParamName).

const sealedPrefix = "enc:v1:"

// ValueSealer — pkg/cryptoutil.Encryptor.
type ValueSealer interface {
	Encrypt(plaintext string) ([]byte, error)
	Decrypt(ciphertext []byte) (string, error)
}

// sealValue menyegel v bila nama sensitif. Nilai yang kebetulan berawalan "enc:v1:" tetap
// disegel (dan dibuka satu kali saat dibaca) sehingga tidak ambigu.
func sealValue(s ValueSealer, name, v string) (string, error) {
	if s == nil || v == "" || !domain.IsSensitiveParamName(name) {
		return v, nil
	}
	b, err := s.Encrypt(v)
	if err != nil {
		return "", fmt.Errorf("mysql: gagal mengenkripsi nilai rahasia: %w", err)
	}
	return sealedPrefix + base64.StdEncoding.EncodeToString(b), nil
}

// openValue membuka nilai tersegel; nilai tanpa awalan (data lama) dikembalikan apa adanya.
// Gagal buka = error (fail closed): ciphertext tidak boleh sampai ke CPE/API.
func openValue(s ValueSealer, v string) (string, error) {
	if !strings.HasPrefix(v, sealedPrefix) {
		return v, nil
	}
	if s == nil {
		return "", errors.New("mysql: nilai terenkripsi ditemukan tetapi kunci enkripsi tidak dikonfigurasi")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(v, sealedPrefix))
	if err != nil {
		return "", fmt.Errorf("mysql: nilai terenkripsi rusak: %w", err)
	}
	plain, err := s.Decrypt(raw)
	if err != nil {
		return "", fmt.Errorf("mysql: gagal membuka nilai terenkripsi (kunci berubah?): %w", err)
	}
	return plain, nil
}

// transformPresetConfigs menerapkan fn pada "value" tiap op set_parameter ber-key sensitif.
// Field lain dipertahankan apa adanya. Konfigurasi bukan array JSON valid dibiarkan (validasi
// ada di usecase), supaya penyimpanan tidak menolak data yang sebelumnya diterima.
func transformPresetConfigs(configs string, fn func(key, value string) (string, error)) (string, error) {
	trimmed := strings.TrimSpace(configs)
	if trimmed == "" || trimmed == "null" {
		return configs, nil
	}
	var ops []map[string]json.RawMessage
	if err := json.Unmarshal([]byte(trimmed), &ops); err != nil {
		return configs, nil
	}
	changed := false
	for _, op := range ops {
		var key, value string
		if json.Unmarshal(op["key"], &key) != nil || json.Unmarshal(op["value"], &value) != nil {
			continue
		}
		if !domain.IsSensitiveParamName(key) || value == "" {
			continue
		}
		nv, err := fn(key, value)
		if err != nil {
			return "", err
		}
		if nv != value {
			b, _ := json.Marshal(nv)
			op["value"] = b
			changed = true
		}
	}
	if !changed {
		return configs, nil
	}
	out, err := json.Marshal(ops)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func sealPresetConfigs(s ValueSealer, configs string) (string, error) {
	return transformPresetConfigs(configs, func(k, v string) (string, error) { return sealValue(s, k, v) })
}

func openPresetConfigs(s ValueSealer, configs string) (string, error) {
	return transformPresetConfigs(configs, func(_, v string) (string, error) { return openValue(s, v) })
}

// SealExistingSecrets mengenkripsi nilai rahasia plaintext yang sudah ada (backfill sekali jalan,
// idempoten: nilai berawalan enc:v1: dilewati). Mengembalikan jumlah baris profil & preset yang diubah.
func SealExistingSecrets(ctx context.Context, db *sqlx.DB, s ValueSealer) (profileRows, presetRows int, err error) {
	var params []struct {
		ID    uint64  `db:"id"`
		Name  string  `db:"parameter_name"`
		Value *string `db:"parameter_value"`
	}
	if err := db.SelectContext(ctx, &params, `SELECT id, parameter_name, parameter_value FROM provisioning_profile_parameters WHERE parameter_value IS NOT NULL AND parameter_value <> ''`); err != nil {
		return 0, 0, err
	}
	for _, p := range params {
		if p.Value == nil || strings.HasPrefix(*p.Value, sealedPrefix) {
			continue
		}
		sealed, err := sealValue(s, p.Name, *p.Value)
		if err != nil {
			return profileRows, presetRows, err
		}
		if sealed == *p.Value {
			continue // nama tidak sensitif
		}
		if _, err := db.ExecContext(ctx, `UPDATE provisioning_profile_parameters SET parameter_value = ? WHERE id = ?`, sealed, p.ID); err != nil {
			return profileRows, presetRows, err
		}
		profileRows++
	}

	var presets []struct {
		ID      uint64 `db:"id"`
		Configs string `db:"configurations"`
	}
	if err := db.SelectContext(ctx, &presets, `SELECT id, configurations FROM presets`); err != nil {
		return profileRows, 0, err
	}
	for _, p := range presets {
		// Hanya nilai yang belum bersegel yang disegel (hindari segel ganda pada nilai berawalan enc:v1:).
		out, err := transformPresetConfigs(p.Configs, func(k, v string) (string, error) {
			if strings.HasPrefix(v, sealedPrefix) {
				return v, nil
			}
			return sealValue(s, k, v)
		})
		if err != nil {
			return profileRows, presetRows, err
		}
		if out == p.Configs {
			continue
		}
		if _, err := db.ExecContext(ctx, `UPDATE presets SET configurations = ? WHERE id = ?`, out, p.ID); err != nil {
			return profileRows, presetRows, err
		}
		presetRows++
	}
	return profileRows, presetRows, nil
}
