package task

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"acs/internal/domain"
	"acs/pkg/cryptoutil"
)

// Nilai rahasia pada task SET_PARAMETER_VALUES (password WiFi, PPPoE, PSK, ...) tidak disimpan plaintext di
// tasks.parameters: saat task DIBUAT nilainya dienkripsi (cryptoutil, AES-GCM, kunci ACS_CREDENTIAL_ENC_KEY) menjadi
// penanda "enc:v1:<base64>"; saat task DIKIRIM ke CPE penanda dibuka lagi (OpenSecrets). API tak pernah mengembalikan
// nilai maupun penandanya — selalu "[redacted]" (lihat redactSensitiveParams). Retry memakai penanda yang sama.
const sealedPrefix = "enc:v1:"

// ErrSealedNoKey — ada nilai terenkripsi tetapi kunci enkripsi tidak tersedia / tidak cocok.
var ErrSealedNoKey = errors.New("task: nilai rahasia terenkripsi tidak dapat dibuka (kunci enkripsi tidak ada atau berubah)")

// IsSealedValue melaporkan apakah v adalah penanda nilai terenkripsi.
func IsSealedValue(v string) bool { return strings.HasPrefix(v, sealedPrefix) }

// SealSecrets mengenkripsi nilai bernama sensitif pada params["values"] (bentuk SET_PARAMETER_VALUES). Tanpa kunci
// (enc nil), tanpa nilai sensitif, atau nilai kosong: params dikembalikan apa adanya. Tidak memutasi params asli.
func SealSecrets(enc *cryptoutil.Encryptor, params map[string]interface{}) (map[string]interface{}, error) {
	if enc == nil || params == nil {
		return params, nil
	}
	raw, ok := params["values"]
	if !ok {
		return params, nil
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return params, nil
	}
	var vals map[string]string
	if json.Unmarshal(b, &vals) != nil || len(vals) == 0 {
		return params, nil
	}
	changed := false
	out := make(map[string]string, len(vals))
	for k, v := range vals {
		if v != "" && !IsSealedValue(v) && domain.IsSensitiveParamName(k) {
			ct, err := enc.Encrypt(v)
			if err != nil {
				return nil, fmt.Errorf("task: enkripsi nilai rahasia: %w", err)
			}
			out[k] = sealedPrefix + base64.StdEncoding.EncodeToString(ct)
			changed = true
			continue
		}
		out[k] = v
	}
	if !changed {
		return params, nil
	}
	cp := make(map[string]interface{}, len(params))
	for k, v := range params {
		cp[k] = v
	}
	cp["values"] = out
	return cp, nil
}

// OpenSecrets membuka semua penanda terenkripsi pada JSON task.Parameters (dipakai tepat sebelum dikirim ke CPE atau
// dicatat ke device_parameters). Tanpa penanda: raw dikembalikan apa adanya (jalur cepat, tanpa parse).
func OpenSecrets(enc *cryptoutil.Encryptor, raw []byte) ([]byte, error) {
	if !bytes.Contains(raw, []byte(sealedPrefix)) {
		return raw, nil
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		return raw, nil
	}
	vraw, ok := top["values"]
	if !ok {
		return raw, nil
	}
	var vals map[string]string
	if err := json.Unmarshal(vraw, &vals); err != nil {
		return raw, nil
	}
	for k, v := range vals {
		if !IsSealedValue(v) {
			continue
		}
		if enc == nil {
			return nil, ErrSealedNoKey
		}
		ct, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(v, sealedPrefix))
		if err != nil {
			return nil, ErrSealedNoKey
		}
		plain, err := enc.Decrypt(ct)
		if err != nil {
			return nil, ErrSealedNoKey
		}
		vals[k] = plain
	}
	nv, err := json.Marshal(vals)
	if err != nil {
		return nil, err
	}
	top["values"] = nv
	return json.Marshal(top)
}

// maskSealed mengganti penanda terenkripsi pada JSON task.Parameters dengan "[redacted]" untuk SEMUA role (termasuk
// ADMIN): nilai yang sudah dienkripsi tidak boleh dibaca kembali lewat API.
func maskSealed(params *domain.JSONRawMessage) {
	if params == nil || !bytes.Contains(*params, []byte(sealedPrefix)) {
		return
	}
	var top map[string]json.RawMessage
	if json.Unmarshal(*params, &top) != nil {
		return
	}
	var vals map[string]string
	if json.Unmarshal(top["values"], &vals) != nil {
		return
	}
	for k, v := range vals {
		if IsSealedValue(v) {
			vals[k] = domain.RedactedValue
		}
	}
	nv, err := json.Marshal(vals)
	if err != nil {
		return
	}
	top["values"] = nv
	if out, err := json.Marshal(top); err == nil {
		*params = out
	}
}
