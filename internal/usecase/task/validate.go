package task

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"acs/internal/domain"
)

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
		for _, n := range p.Names {
			if strings.TrimSpace(n) == "" || len(n) > 256 {
				return bad("nama parameter kosong atau lebih dari 256 karakter")
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
		for n := range p.Values {
			if strings.TrimSpace(n) == "" || len(n) > 256 {
				return bad("nama parameter kosong atau lebih dari 256 karakter")
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
		if len(p.Path) > 256 {
			return bad("parameters.path lebih dari 256 karakter")
		}
	case domain.TaskTypeAddObject, domain.TaskTypeDeleteObject:
		var p struct {
			ObjectName string `json:"object_name"`
		}
		if err := json.Unmarshal(raw, &p); err != nil {
			return bad("parameters harus {object_name: string}")
		}
		if strings.TrimSpace(p.ObjectName) == "" || len(p.ObjectName) > 256 {
			return bad("parameters.object_name wajib diisi (maks. 256 karakter)")
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
	case domain.TaskTypeScheduleInform:
		var p struct {
			DelaySeconds int `json:"delay_seconds"`
		}
		if err := json.Unmarshal(raw, &p); err != nil {
			return bad("parameters.delay_seconds harus bilangan bulat")
		}
		if p.DelaySeconds < 1 {
			return bad("parameters.delay_seconds minimal 1")
		}
	case domain.TaskTypeGetParameterAttributes:
		var p struct {
			Names []string `json:"names"`
		}
		if err := json.Unmarshal(raw, &p); err != nil || len(p.Names) == 0 {
			return bad("parameters.names wajib berisi minimal 1 nama parameter")
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
		for _, a := range p.Attributes {
			if strings.TrimSpace(a.Name) == "" || len(a.Name) > 256 {
				return bad("attributes[].name wajib diisi (maks. 256 karakter)")
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
