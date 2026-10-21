package cwmpxml

import (
	"regexp"
	"strings"
)

// RedactedValue menggantikan nilai parameter rahasia pada hasil Redact.
const RedactedValue = "***REDACTED***"

// secretName mengenali NAMA parameter yang nilainya rahasia: kredensial & kunci WiFi/PPP/ACS. Sengaja dicocokkan pada
// seluruh path parameter (mis. ...PreSharedKey.1.KeyPassphrase, ...ManagementServer.ConnectionRequestPassword,
// X_..._WEBPassword), huruf besar/kecil diabaikan.
var secretName = regexp.MustCompile(`(?i)(pass(word|wd|phrase)?|secret|psk|presharedkey|sharedkey|wpakey|wepkey|authkey|privatekey|credential|token|community)`)

// paramPair = <Name>…</Name> diikuti <Value …>…</Value> (ParameterValueStruct pada Inform, GetParameterValuesResponse,
// SetParameterValues). Nilai kosong/self-closing tidak perlu disamarkan.
var paramPair = regexp.MustCompile(`(?s)(<(?:[\w.-]+:)?Name>)([^<]*)(</(?:[\w.-]+:)?Name>\s*<(?:[\w.-]+:)?Value\b[^>]*>)([^<]*)(</(?:[\w.-]+:)?Value>)`)

// Redact menyamarkan nilai parameter bernama rahasia pada pesan SOAP CWMP, SEBELUM pesan disimpan/ditampilkan di mana pun
// (capture sesi, fixture uji). Struktur XML tidak berubah. Pesan yang bukan XML dikembalikan apa adanya.
func Redact(b []byte) []byte {
	return paramPair.ReplaceAllFunc(b, func(m []byte) []byte {
		sub := paramPair.FindSubmatch(m)
		if sub == nil || strings.TrimSpace(string(sub[4])) == "" || !secretName.Match(sub[2]) {
			return m
		}
		out := make([]byte, 0, len(m))
		out = append(out, sub[1]...)
		out = append(out, sub[2]...)
		out = append(out, sub[3]...)
		out = append(out, RedactedValue...)
		out = append(out, sub[5]...)
		return out
	})
}

// rpcRoot menangkap elemen pertama di dalam <soap:Body>.
var rpcRoot = regexp.MustCompile(`(?s)<(?:[\w.-]+:)?Body\b[^>]*>\s*<(?:[\w.-]+:)?([\w]+)`)

// BodyRPCName mengembalikan nama elemen RPC pada Body (Inform, GetParameterValuesResponse, Fault, ...); "" bila tak ada
// (mis. POST kosong dari CPE).
func BodyRPCName(raw []byte) string {
	if m := rpcRoot.FindSubmatch(raw); m != nil {
		return string(m[1])
	}
	return ""
}
