package cwmp

import (
	"errors"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"acs/internal/usecase/session"
	"acs/pkg/cwmpxml"
)

// Batas isi Inform dari CPE. Isi Inform TIDAK dipercaya: CPE (atau penyerang yang memegang rahasia Inform tenant)
// bisa mengirim apa saja hingga 4 MiB. Angka disamakan dengan kolom schema.sql (devices, device_parameters,
// device_events, device_sessions) supaya satu nilai buruk tidak membuat INSERT/UPSERT gagal (500 → CPE tak bisa
// Inform) atau menggulung-balik seluruh batch parameter.
const (
	maxInformEvents        = 64   // TR-069: Event bersifat daftar pendek; 64 jauh di atas kebutuhan nyata
	maxInformParams        = 2000 // ParameterList Inform; CPE nyata mengirim puluhan–ratusan
	maxInformParamNameLen  = 512  // device_parameters.parameter_name
	maxInformParamValueLen = 8192 // karakter; nilai lebih panjang dipotong (TEXT hanya 65535 byte)
	maxInformSerialLen     = 128  // devices.serial_number
	maxInformProductClass  = 128  // devices.product_class
	maxInformVersionLen    = 64   // devices.software_version / hardware_version
	maxInformCRURLLen      = 255  // devices.connection_request_url
	maxInformEventCodeLen  = 64   // ref_event_codes.code
	maxInformCommandKeyLen = 255  // device_events.command_key
	maxInformNamespaceLen  = 64   // device_sessions.cwmp_namespace
)

var (
	ouiRe = regexp.MustCompile(`^[0-9A-Za-z]{6}$`)
	// Namespace CWMP resmi: urn:dslforum-org:cwmp-1-0 .. cwmp-1-n. Nilai lain diganti default saat membalas.
	cwmpNamespaceRe = regexp.MustCompile(`^urn:dslforum-org:cwmp-1-[0-9]{1,2}$`)

	errInformIdentity = errors.New("identitas perangkat pada Inform tidak valid")
)

func hasControl(s string) bool {
	return strings.ContainsFunc(s, unicode.IsControl)
}

// clampRunes memotong s menjadi maksimal n karakter (rune) tanpa memecah karakter multibyte.
func clampRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n])
}

// validateInformIdentity memeriksa DeviceId. Identitas yang cacat ditolak (400) karena tanpa OUI+serial yang
// sah perangkat tidak bisa didaftarkan/dicocokkan, dan nilai itu akan diteruskan ke SQL, log, dan UI.
func validateInformIdentity(id cwmpxml.DeviceIDStruct) error {
	if !ouiRe.MatchString(id.OUI) {
		return errInformIdentity
	}
	if strings.TrimSpace(id.SerialNumber) == "" || utf8.RuneCountInString(id.SerialNumber) > maxInformSerialLen ||
		!utf8.ValidString(id.SerialNumber) || hasControl(id.SerialNumber) {
		return errInformIdentity
	}
	if !utf8.ValidString(id.ProductClass) || hasControl(id.ProductClass) {
		return errInformIdentity
	}
	return nil
}

// sanitizeNamespace mengembalikan namespace CWMP yang valid, atau "" bila cacat (pemanggil memakai default NSCWMP).
func sanitizeNamespace(ns string) string {
	if len(ns) > maxInformNamespaceLen || !cwmpNamespaceRe.MatchString(ns) {
		return ""
	}
	return ns
}

// sanitizeInformEvents membuang event cacat dan membatasi jumlahnya. Penulisan kode event (mis. "1 BOOT")
// dipertahankan apa adanya; hanya panjang/karakter kontrol yang diperiksa.
func sanitizeInformEvents(items []cwmpxml.EventStruct) []session.InformEvent {
	n := len(items)
	if n > maxInformEvents {
		n = maxInformEvents
	}
	out := make([]session.InformEvent, 0, n)
	for _, e := range items[:n] {
		if strings.TrimSpace(e.EventCode) == "" || utf8.RuneCountInString(e.EventCode) > maxInformEventCodeLen ||
			!utf8.ValidString(e.EventCode) || hasControl(e.EventCode) {
			continue
		}
		ck := e.CommandKey
		if !utf8.ValidString(ck) || hasControl(ck) {
			ck = ""
		}
		out = append(out, session.InformEvent{EventCode: e.EventCode, CommandKey: clampRunes(ck, maxInformCommandKeyLen)})
	}
	return out
}

// sanitizeInformParams membatasi jumlah parameter, membuang yang namanya cacat, dan memotong nilai yang kelewat
// panjang — per elemen, supaya satu entri buruk tidak menggagalkan seluruh UpsertBatch.
func sanitizeInformParams(values []cwmpxml.ParameterValue) []session.InformParameter {
	n := len(values)
	if n > maxInformParams {
		n = maxInformParams
	}
	out := make([]session.InformParameter, 0, n)
	for _, p := range values[:n] {
		if p.Name == "" || utf8.RuneCountInString(p.Name) > maxInformParamNameLen || !utf8.ValidString(p.Name) || hasControl(p.Name) {
			continue
		}
		v := p.Value.Value
		if !utf8.ValidString(v) {
			v = strings.ToValidUTF8(v, "�")
		}
		out = append(out, session.InformParameter{Name: p.Name, Value: clampRunes(v, maxInformParamValueLen), Type: clampRunes(p.Value.Type, 32)})
	}
	return out
}
