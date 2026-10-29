package domain

import (
	"context"
	"sort"
	"strconv"
	"strings"
)

// ---- Pola nama parameter dengan wildcard instance ('*'), dipakai aturan otomasi (set_parameter) ----
//
// Tabel multi-instance TR-069 (WLANConfiguration.{i}, WANPPPConnection.{i}, ...) beranggotakan instance bernomor. Pola
// "InternetGatewayDevice.LANDevice.1.WLANConfiguration.*.SSID" berarti "SSID pada SETIAP instance WLANConfiguration yang pernah dilaporkan
// perangkat". '*' selalu SATU segmen instance numerik utuh (bukan teks bebas, bukan beberapa segmen), sehingga ekspansinya tertutup dan terbatas.

const (
	// AutomationMaxWildcards = jumlah segmen '*' maksimum pada satu nama target (dua tingkat tabel: mis. LANDevice.*.WLANConfiguration.*).
	AutomationMaxWildcards = 2
	// AutomationMaxWildcardInstances = instance maksimum yang boleh cocok untuk SATU aksi. Lebih dari ini aksi DILEWATI seluruhnya (bukan
	// diterapkan sebagian), dan aksi berwildcard dihitung sebanyak ini pada anggaran set_parameter per tenant.
	AutomationMaxWildcardInstances = 8
)

// ParamWildcardCount: jumlah segmen '*' pada nama/pola (segmen dipisah titik).
func ParamWildcardCount(name string) int {
	n := 0
	for _, seg := range strings.Split(name, ".") {
		if seg == "*" {
			n++
		}
	}
	return n
}

// HasParamWildcard: apakah nama memuat karakter '*' sama sekali (termasuk bentuk tak sah seperti "SSID*").
func HasParamWildcard(name string) bool { return strings.Contains(name, "*") }

// ValidParamPattern: pola sah = raw path (TR-098 InternetGatewayDevice. / TR-181 Device.) yang tiap segmennya huruf/angka/'_'/'-' ATAU persis
// '*'; '*' hanya sebagai segmen utuh, tidak pada segmen pertama/terakhir (parameter daun harus eksplisit), dan paling banyak
// AutomationMaxWildcards. Pola tanpa '*' sah bila ValidParamName.
func ValidParamPattern(name string) bool {
	if !HasParamWildcard(name) {
		return ValidParamName(name)
	}
	if len(name) > MaxParamNameLen || !(strings.HasPrefix(name, "InternetGatewayDevice.") || strings.HasPrefix(name, "Device.")) {
		return false
	}
	segs := strings.Split(name, ".")
	if len(segs) < 3 || ParamWildcardCount(name) > AutomationMaxWildcards {
		return false
	}
	for i, seg := range segs {
		switch {
		case seg == "*":
			if i == 0 || i == len(segs)-1 {
				return false
			}
		case seg == "" || !ValidParamName(seg):
			return false
		}
	}
	return true
}

// MatchParamPattern: bila path cocok dengan pola, mengembalikan nomor instance yang ditangkap tiap '*' (berurutan). '*' hanya cocok dengan
// segmen angka (instance), tidak pernah dengan nama objek atau beberapa segmen.
func MatchParamPattern(pattern, path string) ([]string, bool) {
	ps, xs := strings.Split(pattern, "."), strings.Split(path, ".")
	if len(ps) != len(xs) {
		return nil, false
	}
	var caps []string
	for i, p := range ps {
		if p == "*" {
			if !allDigits(xs[i]) {
				return nil, false
			}
			caps = append(caps, xs[i])
			continue
		}
		if p != xs[i] {
			return nil, false
		}
	}
	return caps, true
}

func allDigits(s string) bool {
	if s == "" || len(s) > 9 { // 9 digit cukup untuk instance TR-069 dan menjaga perbandingan numerik tetap aman
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// SubstituteParamWildcards mengganti segmen '*' pada s dengan nomor instance dari caps (berurutan). Bila '*' lebih banyak daripada caps,
// sisanya dibiarkan apa adanya.
func SubstituteParamWildcards(s string, caps []string) string {
	segs := strings.Split(s, ".")
	k := 0
	for i, seg := range segs {
		if seg == "*" && k < len(caps) {
			segs[i] = caps[k]
			k++
		}
	}
	return strings.Join(segs, ".")
}

// MatchedParamInstance = satu path yang cocok dengan pola beserta instance yang ditangkap.
type MatchedParamInstance struct {
	Path string
	Caps []string
}

// FilterParamPattern menyaring kandidat ke yang benar-benar cocok pola, unik, terurut menurut nomor instance (numerik: 2 sebelum 10).
func FilterParamPattern(pattern string, candidates []string) []MatchedParamInstance {
	seen := map[string]bool{}
	var out []MatchedParamInstance
	for _, c := range candidates {
		if seen[c] {
			continue
		}
		if caps, ok := MatchParamPattern(pattern, c); ok {
			seen[c] = true
			out = append(out, MatchedParamInstance{Path: c, Caps: caps})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		for k := range out[i].Caps {
			a, _ := strconv.Atoi(out[i].Caps[k])
			b, _ := strconv.Atoi(out[j].Caps[k])
			if a != b {
				return a < b
			}
		}
		return out[i].Path < out[j].Path
	})
	return out
}

// DeviceParameterPathLister = kemampuan OPSIONAL repository parameter perangkat: mencari NAMA parameter yang cocok pola kasar. Dicek lewat
// type assertion supaya implementasi/fake lain tidak dipaksa mengimplementasikannya.
type DeviceParameterPathLister interface {
	// CandidatePaths: nama parameter milik perangkat yang cocok pola dengan tiap '*' dibaca sebagai "apa saja" (LIKE), urut nama, maks limit.
	// Hasilnya SUPERSET: pemanggil menyaring ketat dengan FilterParamPattern (yang menegakkan '*' = satu segmen angka).
	CandidatePaths(ctx context.Context, deviceID uint64, pattern string, limit int) ([]string, error)
}
