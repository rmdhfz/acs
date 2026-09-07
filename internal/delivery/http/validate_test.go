package http

import (
	"net/http"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
)

func TestCheckMaxLen(t *testing.T) {
	tests := []struct {
		name      string
		rules     []lenRule
		wantErr   bool
		wantField string // potongan yang harus muncul di pesan galat
	}{
		{
			name:    "di bawah batas lolos",
			rules:   []lenRule{{"name", "ISP Jakarta", maxTenantName}},
			wantErr: false,
		},
		{
			name:    "tepat di batas lolos (boundary)",
			rules:   []lenRule{{"code", strings.Repeat("A", maxTenantCode), maxTenantCode}},
			wantErr: false,
		},
		{
			name:      "satu karakter di atas batas ditolak",
			rules:     []lenRule{{"code", strings.Repeat("A", maxTenantCode+1), maxTenantCode}},
			wantErr:   true,
			wantField: "code",
		},
		{
			name:    "string kosong selalu lolos",
			rules:   []lenRule{{"description", "", maxDescription}},
			wantErr: false,
		},
		{
			// Ini alasan checkMaxLen memakai utf8.RuneCountInString, bukan len():
			// VARCHAR(n) MariaDB membatasi KARAKTER, sementara len() menghitung
			// byte — nama non-ASCII yang sah akan tertolak kalau salah hitung.
			name:    "karakter multibyte dihitung sebagai rune bukan byte",
			rules:   []lenRule{{"name", strings.Repeat("é", maxTenantName), maxTenantName}},
			wantErr: false,
		},
		{
			name:      "multibyte melewati batas tetap ditolak",
			rules:     []lenRule{{"name", strings.Repeat("é", maxTenantName+1), maxTenantName}},
			wantErr:   true,
			wantField: "name",
		},
		{
			name: "melaporkan pelanggaran PERTAMA saja",
			rules: []lenRule{
				{"code", "ok", maxTenantCode},
				{"name", strings.Repeat("A", maxTenantName+1), maxTenantName},
				{"cwmp_inform_username", strings.Repeat("B", maxTenantCWMPUser+1), maxTenantCWMPUser},
			},
			wantErr:   true,
			wantField: "name",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := checkMaxLen(tc.rules...)
			if !tc.wantErr {
				if err != nil {
					t.Fatalf("harusnya lolos, malah galat: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("harusnya ditolak, malah lolos")
			}
			he, ok := err.(*echo.HTTPError)
			if !ok {
				t.Fatalf("galat harus *echo.HTTPError supaya jadi 400, dapat %T", err)
			}
			// Inti perbaikannya: input kepanjangan harus 400, BUKAN 500 dari
			// error MariaDB 1406 "Data too long for column".
			if he.Code != http.StatusBadRequest {
				t.Errorf("kode = %d, mau %d", he.Code, http.StatusBadRequest)
			}
			if !strings.Contains(he.Message, tc.wantField) {
				t.Errorf("pesan %q harus menyebut field %q", he.Message, tc.wantField)
			}
		})
	}
}

func TestOptStr(t *testing.T) {
	if got := optStr(nil); got != "" {
		t.Errorf("optStr(nil) = %q, mau string kosong supaya field opsional otomatis lolos", got)
	}
	v := "isi"
	if got := optStr(&v); got != "isi" {
		t.Errorf("optStr(&v) = %q, mau %q", got, v)
	}
}

// TestLenRulesMatchSchema menjaga konstanta di validate.go tetap sinkron dengan
// lebar kolom di schema.sql. Kalau migrasi mengubah lebar kolom tapi konstanta
// di sini tidak ikut diubah, validasi jadi bohong: request lolos 400 lalu tetap
// meledak jadi 500 di MariaDB (atau sebaliknya, menolak nilai yang sebenarnya sah).
func TestLenRulesMatchSchema(t *testing.T) {
	// nilai di kanan disalin manual dari schema.sql — sengaja hardcode supaya
	// perubahan skema memaksa orang membaca dan memperbarui test ini.
	want := map[string]int{
		"tenants.code":                         32,
		"tenants.name":                         128,
		"users.username":                       64,
		"users.email":                          191,
		"users.full_name":                      128,
		"tags.name":                            255,
		"presets.name":                         255,
		"webhook_subscriptions.name":           128,
		"webhook_subscriptions.target_url":     500,
		"vendor_parameter_mappings.tr069_path": 512,
	}
	got := map[string]int{
		"tenants.code":                         maxTenantCode,
		"tenants.name":                         maxTenantName,
		"users.username":                       maxUsername,
		"users.email":                          maxEmail,
		"users.full_name":                      maxFullName,
		"tags.name":                            maxTagName,
		"presets.name":                         maxPresetName,
		"webhook_subscriptions.name":           maxWebhookName,
		"webhook_subscriptions.target_url":     maxWebhookTargetURL,
		"vendor_parameter_mappings.tr069_path": maxTR069Path,
	}
	for col, w := range want {
		if got[col] != w {
			t.Errorf("batas %s = %d, tapi schema.sql bilang %d", col, got[col], w)
		}
	}
}
