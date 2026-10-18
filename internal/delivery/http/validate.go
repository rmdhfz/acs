package http

import (
	"fmt"
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/labstack/echo/v5"
)

// Validasi panjang string di lapisan delivery.
//
// KENAPA ADA: sebelum ini tidak ada satu pun pengecekan panjang, baik di
// handler maupun di frontend. Input yang melebihi lebar kolom lolos sampai ke
// MariaDB dan meledak sebagai error 1406 "Data too long for column" — yang
// keluar ke pemanggil sebagai HTTP 500, seolah server yang rusak, padahal itu
// murni input tidak valid dan seharusnya 400. Temuan audit parity 2026-09-05.
//
// Batas di bawah HARUS sama persis dengan lebar kolom di `schema.sql`. Kalau
// kolomnya diubah lewat migrasi, ubah juga konstanta di sini — dan `maxLength`
// pada input form terkait di `frontend/src/`.
const (
	// tenants
	maxTenantCode       = 32  // tenants.code VARCHAR(32) NOT NULL
	maxTenantName       = 128 // tenants.name VARCHAR(128) NOT NULL
	maxTenantCWMPUser   = 128 // tenants.cwmp_inform_username VARCHAR(128)
	maxTenantBrandName  = 128 // tenants.brand_name VARCHAR(128)
	maxTenantLogoURL    = 512 // tenants.logo_url VARCHAR(512)
	maxTenantPrimaryHex = 7   // tenants.primary_color CHAR(7) — "#RRGGBB"
	// users
	maxUsername = 64  // users.username VARCHAR(64) NOT NULL
	maxEmail    = 191 // users.email VARCHAR(191) NOT NULL
	maxFullName = 128 // users.full_name VARCHAR(128) NOT NULL
	// api_tokens
	maxAPITokenName = 128 // api_tokens.name VARCHAR(128) NOT NULL
	// tags & presets
	maxTagName    = 255 // tags.name VARCHAR(255) NOT NULL
	maxTagColor   = 7   // tags.color VARCHAR(7)
	maxPresetName = 255 // presets.name VARCHAR(255) NOT NULL
	maxPresetChan = 64  // presets.channel VARCHAR(64)
	// katalog vendor
	maxVendorCode  = 32  // ref_vendors.code VARCHAR(32) NOT NULL
	maxVendorName  = 128 // ref_vendors.name VARCHAR(128) NOT NULL
	maxDescription = 255 // pola umum kolom description VARCHAR(255)
	// vendor_ouis.oui CHAR(6) NOT NULL. Hanya batas ATAS yang dicek di sini —
	// aturan semantik "OUI wajib tepat 6 digit heksadesimal" sengaja TIDAK
	// ditambahkan supaya tidak mengubah perilaku yang sudah ada (nilai lebih
	// pendek selama ini diterima); itu keputusan produk, bukan perbaikan bug
	// 500 yang jadi lingkup perubahan ini.
	maxOUI             = 6
	maxOUINotes        = 255 // vendor_ouis.notes VARCHAR(255)
	maxProductClass    = 128 // device_models.product_class VARCHAR(128)
	maxModelName       = 128 // device_models.model_name VARCHAR(128) NOT NULL
	maxLogicalKey      = 128 // vendor_parameter_mappings.logical_key VARCHAR(128) NOT NULL
	maxTR069Path       = 512 // vendor_parameter_mappings.tr069_path VARCHAR(512) NOT NULL
	maxSoftwareVerPatt = 255 // *.software_version_pattern VARCHAR(255)
	// provisioning & ZTP
	maxProfileName       = 128 // provisioning_profiles.name VARCHAR(128) NOT NULL
	maxSerialPattern     = 255 // zero_touch_rules.serial_pattern VARCHAR(255)
	maxMatchParamName    = 512 // zero_touch_rules.match_parameter_name VARCHAR(512)
	maxMatchParamValPatt = 255 // zero_touch_rules.match_parameter_value_pattern VARCHAR(255)
	// notifikasi (batas isi/judul/link: usecase/notification, = lebar kolom notifications)
	maxNotificationTypeCode = 48 // ref_notification_types.code VARCHAR(48)
	// webhooks
	maxWebhookName      = 128 // webhook_subscriptions.name VARCHAR(128) NOT NULL
	maxWebhookTargetURL = 500 // webhook_subscriptions.target_url VARCHAR(500) NOT NULL
)

// lenRule — satu field yang mau dicek panjangnya.
type lenRule struct {
	Field string // nama field seperti yang dikirim klien (JSON), bukan nama kolom
	Value string
	Max   int
}

// checkMaxLen mengembalikan 400 pada pelanggaran PERTAMA yang ditemukan.
//
// Menghitung RUNE, bukan byte: kolom VARCHAR(n) MariaDB dengan
// utf8mb4_unicode_ci membatasi jumlah KARAKTER, jadi len() byte akan menolak
// nama yang sebenarnya sah (mis. huruf beraksen atau emoji di nama tenant).
func checkMaxLen(rules ...lenRule) error {
	for _, r := range rules {
		if utf8.RuneCountInString(r.Value) > r.Max {
			return echo.NewHTTPError(http.StatusBadRequest,
				fmt.Sprintf("%s maksimal %d karakter", r.Field, r.Max))
		}
	}
	return nil
}

// optStr membaca field opsional (*string) menjadi string biasa untuk dicek.
// nil dianggap string kosong sehingga otomatis lolos pengecekan panjang.
func optStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// Batas masukan billing (langganan SaaS, migrations/0032). Panjang dihitung RUNE (checkMaxLen).
const (
	maxBillingBody      = 16 << 10 // body JSON billing biasa
	maxBillingWebhook   = 64 << 10 // body callback Xendit
	maxBillingPlanCode  = 64       // sebelum normalisasi; usecase menegakkan 2..32 setelah trim
	maxBillingPlanName  = 64       // billing_plans.name VARCHAR(64)
	maxBillingInvoiceID = 36       // UUID kanonik
	maxCallbackToken    = 512
)

// rejectControl menolak karakter kontrol (termasuk newline/tab) dan UTF-8 tidak valid pada string dari klien.
func rejectControl(field, v string) error {
	if !utf8.ValidString(v) {
		return echo.NewHTTPError(http.StatusBadRequest, field+" bukan teks UTF-8 yang valid")
	}
	for _, r := range v {
		if unicode.IsControl(r) {
			return echo.NewHTTPError(http.StatusBadRequest, field+" tidak boleh memuat karakter kontrol")
		}
	}
	return nil
}

// validateUploadName menolak nama berkas yang memuat pemisah path, karakter kontrol, atau nama relatif ("."/".."):
// nama ini kelak dipakai sebagai target_file_name RPC Download ke CPE.
func validateUploadName(name string) error {
	if name == "" {
		return nil
	}
	if err := rejectControl("file_name", name); err != nil {
		return err
	}
	if name == "." || name == ".." || strings.ContainsAny(name, "/\\") {
		return echo.NewHTTPError(http.StatusBadRequest, "file_name tidak boleh memuat pemisah path")
	}
	return nil
}
