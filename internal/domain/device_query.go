package domain

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ---- Query perangkat bebas: kondisi pada nilai parameter, urutan, proyeksi ----
//
// Padanan query/projection/sort pada /devices GenieACS, tetapi bentuknya tertutup: operator dari daftar tetap, nama parameter
// divalidasi, semua nilai di-bind (tidak ada SQL/ekspresi bebas dari klien) dan tetap dibatasi tenant.

// Batas query bebas (satu sumber untuk handler, usecase, dan dokumentasi API).
const (
	MaxParamConds     = 5   // kondisi param= per permintaan
	MaxProjectParams  = 20  // nama parameter pada params=
	MaxParamNameLen   = 256 // device_parameters.parameter_name
	MaxParamCondValue = 255
)

// Operator kondisi parameter.
const (
	ParamOpEq      = "eq"      // nilai sama persis (tidak peka huruf besar/kecil, mengikuti collation tabel)
	ParamOpNe      = "ne"      // parameter ada DAN nilainya berbeda
	ParamOpLike    = "like"    // mengandung teks
	ParamOpPrefix  = "prefix"  // berawalan teks
	ParamOpGt      = "gt"      // numerik >
	ParamOpGe      = "ge"      // numerik >=
	ParamOpLt      = "lt"      // numerik <
	ParamOpLe      = "le"      // numerik <=
	ParamOpExists  = "exists"  // parameter pernah dilaporkan (bernilai tidak kosong)
	ParamOpMissing = "missing" // parameter belum ada / kosong
)

// ParamCond = satu kondisi: parameter Name dengan operator Op terhadap Value (kosong untuk exists/missing).
type ParamCond struct {
	Name  string
	Op    string
	Value string
}

// DeviceSortFields = kolom yang boleh dipakai ?sort= (nama JSON == nama kolom).
var DeviceSortFields = []string{"id", "serial_number", "last_inform_at", "created_at", "ip_address", "software_version", "product_class"}

var paramNameRe = regexp.MustCompile(`^[A-Za-z0-9_.\-]+$`)

// ValidParamName: nama parameter TR-069 yang aman dipakai sebagai kunci (huruf, angka, '.', '_', '-'), maks MaxParamNameLen.
func ValidParamName(name string) bool {
	return name != "" && len(name) <= MaxParamNameLen && paramNameRe.MatchString(name)
}

// ParseParamCond mengurai "nama~op[~nilai]" (nilai boleh memuat '~'). Op numerik mensyaratkan nilai angka.
func ParseParamCond(raw string) (ParamCond, error) {
	parts := strings.SplitN(raw, "~", 3)
	if len(parts) < 2 {
		return ParamCond{}, fmt.Errorf("%w: param harus berbentuk nama~op[~nilai], dapat %q", ErrInvalidInput, truncateForMsg(raw))
	}
	c := ParamCond{Name: parts[0], Op: parts[1]}
	if len(parts) == 3 {
		c.Value = parts[2]
	}
	if !ValidParamName(c.Name) {
		return ParamCond{}, fmt.Errorf("%w: nama parameter tidak valid (huruf, angka, '.', '_', '-'; maks %d karakter)", ErrInvalidInput, MaxParamNameLen)
	}
	switch c.Op {
	case ParamOpExists, ParamOpMissing:
		if c.Value != "" {
			return ParamCond{}, fmt.Errorf("%w: op %s tidak memakai nilai", ErrInvalidInput, c.Op)
		}
	case ParamOpEq, ParamOpNe, ParamOpLike, ParamOpPrefix:
		if c.Value == "" {
			return ParamCond{}, fmt.Errorf("%w: op %s membutuhkan nilai", ErrInvalidInput, c.Op)
		}
	case ParamOpGt, ParamOpGe, ParamOpLt, ParamOpLe:
		if _, err := strconv.ParseFloat(c.Value, 64); err != nil {
			return ParamCond{}, fmt.Errorf("%w: op %s membutuhkan nilai angka", ErrInvalidInput, c.Op)
		}
	default:
		return ParamCond{}, fmt.Errorf("%w: op %q tidak dikenal (eq, ne, like, prefix, gt, ge, lt, le, exists, missing)", ErrInvalidInput, c.Op)
	}
	if len(c.Value) > MaxParamCondValue {
		return ParamCond{}, fmt.Errorf("%w: nilai kondisi maksimal %d karakter", ErrInvalidInput, MaxParamCondValue)
	}
	return c, nil
}

// ParseSort mengurai "field" atau "-field" (menurun); field di luar DeviceSortFields ditolak. Kosong = default.
func ParseSort(raw string) (field string, desc bool, err error) {
	if raw == "" {
		return "", false, nil
	}
	field = raw
	if strings.HasPrefix(raw, "-") {
		desc, field = true, raw[1:]
	}
	for _, f := range DeviceSortFields {
		if f == field {
			return field, desc, nil
		}
	}
	return "", false, fmt.Errorf("%w: sort harus salah satu dari %s (awali '-' untuk menurun)", ErrInvalidInput, strings.Join(DeviceSortFields, ", "))
}

func truncateForMsg(s string) string {
	if r := []rune(s); len(r) > 60 {
		return string(r[:60]) + "…"
	}
	return s
}
