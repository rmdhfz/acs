package task

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"

	"acs/internal/domain"
)

// Batas jumlah & ukuran parameter task. Tanpa batas ini satu request (maks. 2 MiB) bisa memuat ratusan ribu nama
// (menghasilkan ribuan task chunk) atau SOAP raksasa ke CPE. Panjang nama dihitung rune sesuai kolom schema.sql.
const (
	MaxParameterNames    = 200  // GetParameterValues/GetParameterAttributes per request
	MaxSetValues         = 100  // SetParameterValues per request
	MaxSetAttributes     = 200  // SetParameterAttributes per request
	MaxAccessListEntries = 16   // access_list per atribut
	MaxParameterNameLen  = 256  // karakter
	MaxParameterValueLen = 8192 // karakter; sama dengan provisioning_profile_parameters
	MaxTransferURLLen    = 2048
	MaxScheduleInformSec = 7 * 24 * 3600
)

// checkParamName menolak nama parameter kosong, terlalu panjang, bukan UTF-8, atau memuat karakter kontrol.
func checkParamName(n string) bool {
	if strings.TrimSpace(n) == "" || utf8.RuneCountInString(n) > MaxParameterNameLen || !utf8.ValidString(n) {
		return false
	}
	for _, r := range n {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

// ValidateTaskInput memeriksa tipe & bentuk parameters SEBELUM task disimpan,
// agar task yang pasti gagal dibangun menjadi RPC (builder CWMP) ditolak di API
// dengan 400 — bukan masuk antrean lalu membuat respons CWMP 500 dan task
// tersangkut berstatus SENT. Bentuk JSON harus sama dengan yang dibaca
// internal/delivery/cwmp/builder.go BuildRequestBody (kontrak internal).
func ValidateTaskInput(taskType string, params map[string]interface{}) error {
	bad := func(format string, a ...any) error {
		return fmt.Errorf("%w: %s", domain.ErrInvalidInput, fmt.Sprintf(format, a...))
	}
	raw, err := json.Marshal(params)
	if err != nil {
		return bad("parameters bukan JSON valid")
	}
	if params == nil {
		raw = []byte("{}")
	}

	switch taskType {
	case domain.TaskTypeGetParameterValues:
		var p struct {
			Names []string `json:"names"`
		}
		if err := json.Unmarshal(raw, &p); err != nil {
			return bad("parameters.names harus array string")
		}
		if len(p.Names) == 0 {
			return bad("parameters.names wajib berisi minimal 1 nama parameter")
		}
		if len(p.Names) > MaxParameterNames {
			return bad("parameters.names maksimal %d nama per task", MaxParameterNames)
		}
		for _, n := range p.Names {
			if !checkParamName(n) {
				return bad("nama parameter kosong, lebih dari %d karakter, atau memuat karakter kontrol", MaxParameterNameLen)
			}
		}
	case domain.TaskTypeSetParameterValues:
		var p struct {
			Values map[string]string `json:"values"`
			Types  map[string]string `json:"types"`
		}
		if err := json.Unmarshal(raw, &p); err != nil {
			return bad("parameters.values harus objek {nama: nilai-string}; parameters.types objek {nama: tipe-xsd}")
		}
		for n, ty := range p.Types {
			if domain.NormalizeXSDType(ty) == "" {
				return bad("tipe XSD %q untuk %q tidak dikenal (string, int, unsignedInt, boolean, dateTime, base64)", ty, n)
			}
		}
		if len(p.Values) == 0 {
			return bad("parameters.values wajib berisi minimal 1 parameter")
		}
		if len(p.Values) > MaxSetValues {
			return bad("parameters.values maksimal %d parameter per task", MaxSetValues)
		}
		for n, v := range p.Values {
			if !checkParamName(n) {
				return bad("nama parameter kosong, lebih dari %d karakter, atau memuat karakter kontrol", MaxParameterNameLen)
			}
			if utf8.RuneCountInString(v) > MaxParameterValueLen || !utf8.ValidString(v) {
				return bad("nilai parameter %q lebih dari %d karakter atau bukan UTF-8", n, MaxParameterValueLen)
			}
		}
	case domain.TaskTypeGetParameterNames:
		var p struct {
			Path      string `json:"path"`
			NextLevel bool   `json:"next_level"`
			Discover  *struct {
				Depth  int `json:"depth"`
				Budget int `json:"budget"`
			} `json:"discover"`
		}
		if err := json.Unmarshal(raw, &p); err != nil {
			return bad("parameters harus {path: string, next_level: bool}")
		}
		if p.Discover != nil && (p.Discover.Depth < 1 || p.Discover.Depth > 8 || p.Discover.Budget < 0 || p.Discover.Budget > 300) {
			return bad("parameters.discover: depth 1-8, budget 0-300")
		}
		if utf8.RuneCountInString(p.Path) > MaxParameterNameLen || strings.ContainsFunc(p.Path, unicode.IsControl) {
			return bad("parameters.path lebih dari %d karakter atau memuat karakter kontrol", MaxParameterNameLen)
		}
	case domain.TaskTypeAddObject, domain.TaskTypeDeleteObject:
		var p struct {
			ObjectName string `json:"object_name"`
		}
		if err := json.Unmarshal(raw, &p); err != nil {
			return bad("parameters harus {object_name: string}")
		}
		if !checkParamName(p.ObjectName) {
			return bad("parameters.object_name wajib diisi (maks. %d karakter, tanpa karakter kontrol)", MaxParameterNameLen)
		}
	case domain.TaskTypeReboot, domain.TaskTypeFactoryReset:
		// tanpa parameter
	case domain.TaskTypeDownload, domain.TaskTypeUpload:
		var p struct {
			URL        string `json:"url"`
			StorageKey string `json:"storage_key"`
		}
		if err := json.Unmarshal(raw, &p); err != nil {
			return bad("parameters tidak valid")
		}
		if taskType == domain.TaskTypeDownload && p.URL == "" && p.StorageKey != "" {
			break // URL presigned dibuat saat RPC dikirim
		}
		u, err := url.Parse(p.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return bad("parameters.url wajib URL http(s) yang valid")
		}
		if len(p.URL) > MaxTransferURLLen || u.User != nil || strings.ContainsFunc(p.URL, unicode.IsControl) {
			return bad("parameters.url maksimal %d karakter, tanpa kredensial di URL dan tanpa karakter kontrol", MaxTransferURLLen)
		}
	case domain.TaskTypeScheduleInform:
		var p struct {
			DelaySeconds int `json:"delay_seconds"`
		}
		if err := json.Unmarshal(raw, &p); err != nil {
			return bad("parameters.delay_seconds harus bilangan bulat")
		}
		if p.DelaySeconds < 1 || p.DelaySeconds > MaxScheduleInformSec {
			return bad("parameters.delay_seconds harus 1-%d", MaxScheduleInformSec)
		}
	case domain.TaskTypeGetParameterAttributes:
		var p struct {
			Names []string `json:"names"`
		}
		if err := json.Unmarshal(raw, &p); err != nil || len(p.Names) == 0 {
			return bad("parameters.names wajib berisi minimal 1 nama parameter")
		}
		if len(p.Names) > MaxParameterNames {
			return bad("parameters.names maksimal %d nama per task", MaxParameterNames)
		}
		for _, n := range p.Names {
			if !checkParamName(n) {
				return bad("nama parameter kosong, lebih dari %d karakter, atau memuat karakter kontrol", MaxParameterNameLen)
			}
		}
	case domain.TaskTypeSetParameterAttributes:
		var p struct {
			Attributes []struct {
				Name               string   `json:"name"`
				NotificationChange bool     `json:"notification_change"`
				Notification       int      `json:"notification"`
				AccessListChange   bool     `json:"access_list_change"`
				AccessList         []string `json:"access_list"`
			} `json:"attributes"`
		}
		if err := json.Unmarshal(raw, &p); err != nil || len(p.Attributes) == 0 {
			return bad("parameters.attributes wajib berisi minimal 1 item {name, notification_change, notification, ...}")
		}
		if len(p.Attributes) > MaxSetAttributes {
			return bad("parameters.attributes maksimal %d item per task", MaxSetAttributes)
		}
		for _, a := range p.Attributes {
			if !checkParamName(a.Name) {
				return bad("attributes[].name wajib diisi (maks. %d karakter, tanpa karakter kontrol)", MaxParameterNameLen)
			}
			if len(a.AccessList) > MaxAccessListEntries {
				return bad("attributes[].access_list maksimal %d entri", MaxAccessListEntries)
			}
			for _, e := range a.AccessList {
				if !checkParamName(e) {
					return bad("attributes[].access_list berisi entri kosong/terlalu panjang")
				}
			}
			if a.Notification < 0 || a.Notification > 2 {
				return bad("attributes[].notification harus 0 (off), 1 (passive) atau 2 (active)")
			}
		}
	default:
		return bad("tipe task %q tidak dikenal", taskType)
	}
	return nil
}
