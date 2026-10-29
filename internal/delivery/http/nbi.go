package http

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"time"

	"acs/internal/domain"
)

// NBI kompatibel-sebagian dengan GenieACS (GET /nbi/devices): integrator yang sudah memakai query/projection/sort/skip/limit
// GenieACS bisa memindahkan klien tanpa menulis ulang. Ini PENERJEMAH tipis ke query perangkat yang sudah ada (domain.DeviceFilter),
// bukan mesin query Mongo: operator dari daftar tetap, nama parameter divalidasi, nilai di-bind, dan semuanya tetap dibatasi
// tenant/RBAC oleh usecase. Apa pun di luar subset di bawah ditolak 400 dengan pesan yang menyebut yang didukung — tidak diabaikan
// diam-diam, supaya klien tidak mengira filternya berlaku.
//
// Subset query (objek JSON, kunci-kunci digabung AND, maks domain.MaxParamConds kondisi parameter):
//
//	"DeviceID.SerialNumber": "<serial>"                    cocok persis
//	"_lastInform": {"$gte": "<RFC3339>", "$lt": "<RFC3339>"}
//	"<path TR-069>": <nilai> | {"$eq|$ne|$gt|$gte|$lt|$lte": <nilai>, "$exists": true|false}   (akhiran ._value boleh)

const nbiMaxQueryBytes = 4096

// nbiRoots = awalan root data model; kunci query berawalan ini dianggap path parameter.
var nbiRoots = []string{"InternetGatewayDevice.", "Device."}

// nbiSortFields memetakan kunci sort GenieACS ke kolom yang diizinkan domain.DeviceSortFields.
var nbiSortFields = map[string]string{
	"_lastInform":           "last_inform_at",
	"_registered":           "created_at",
	"DeviceID.SerialNumber": "serial_number",
	"DeviceID.ProductClass": "product_class",
}

// nbiAlwaysFields = kunci projection yang selalu ada pada dokumen sehingga tidak perlu diminta (diterima, tidak berefek).
var nbiAlwaysFields = map[string]bool{"_id": true, "_lastInform": true, "_registered": true, "DeviceID": true}

func nbiBadRequest(format string, a ...any) error { return badRequest("nbi: "+format, a...) }

// decodeNBIObject mengurai satu objek JSON (angka dijaga sebagai json.Number) dan menolak data sisa.
func decodeNBIObject(what, raw string) (map[string]any, error) {
	if len(raw) > nbiMaxQueryBytes {
		return nil, nbiBadRequest("%s maksimal %d byte", what, nbiMaxQueryBytes)
	}
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil || m == nil {
		return nil, nbiBadRequest("%s harus berupa objek JSON", what)
	}
	if dec.More() {
		return nil, nbiBadRequest("%s memuat data setelah objek JSON", what)
	}
	return m, nil
}

// nbiScalar mengubah nilai literal query menjadi teks pembanding. Boolean/null/larik/objek ditolak: nilai parameter TR-069 tersimpan
// sebagai teks, jadi pembandingan boolean tidak punya arti yang tunggal.
func nbiScalar(key string, v any) (string, error) {
	switch t := v.(type) {
	case string:
		return t, nil
	case json.Number:
		return t.String(), nil
	}
	return "", nbiBadRequest("nilai untuk %q harus teks atau angka", key)
}

// parseNBIQuery mengisi filter dari parameter query= (kosong = tanpa filter).
func parseNBIQuery(raw string, f *domain.DeviceFilter) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	q, err := decodeNBIObject("query", raw)
	if err != nil {
		return err
	}
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sort.Strings(keys) // urutan deterministik: pesan galat & kondisi SQL stabil

	for _, key := range keys {
		val := q[key]
		switch {
		case key == "DeviceID.SerialNumber":
			s, err := nbiEqOnly(key, val)
			if err != nil {
				return err
			}
			if s == "" {
				return nbiBadRequest("DeviceID.SerialNumber tidak boleh kosong")
			}
			f.SerialNumber = s
		case key == "_lastInform":
			if err := nbiLastInform(val, f); err != nil {
				return err
			}
		case nbiIsParamKey(key):
			if err := nbiParamConds(key, val, f); err != nil {
				return err
			}
		default:
			return nbiBadRequest("kunci query %q tidak didukung (didukung: DeviceID.SerialNumber, _lastInform, dan path parameter berawalan %s)",
				nbiTrunc(key), strings.Join(nbiRoots, " / "))
		}
	}
	return nil
}

// nbiEqOnly menerima nilai literal atau {"$eq": nilai}.
func nbiEqOnly(key string, val any) (string, error) {
	if m, ok := val.(map[string]any); ok {
		if len(m) != 1 {
			return "", nbiBadRequest("%q hanya mendukung pencocokan persis", key)
		}
		inner, ok := m["$eq"]
		if !ok {
			return "", nbiBadRequest("%q hanya mendukung pencocokan persis ($eq)", key)
		}
		val = inner
	}
	return nbiScalar(key, val)
}

func nbiLastInform(val any, f *domain.DeviceFilter) error {
	ops, ok := val.(map[string]any)
	if !ok || len(ops) == 0 {
		return nbiBadRequest("_lastInform harus berupa objek operator ($gte, $lt)")
	}
	for _, op := range sortedKeys(ops) {
		switch op {
		case "$gte", "$lt":
		case "$gt", "$lte":
			// Repositori memakai last_inform_at >= After dan < Before; $gt/$lte tidak bisa dipetakan persis, jadi ditolak.
			return nbiBadRequest("_lastInform mendukung $gte dan $lt saja (bukan %s)", op)
		default:
			return nbiBadRequest("operator %q pada _lastInform tidak didukung (didukung: $gte, $lt)", nbiTrunc(op))
		}
		s, ok := ops[op].(string)
		if !ok {
			return nbiBadRequest("nilai _lastInform harus teks RFC3339")
		}
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return nbiBadRequest("_lastInform harus berformat RFC3339 (mis. 2026-10-22T03:00:00Z)")
		}
		if op == "$gte" {
			f.InformAfter = &t
		} else {
			f.InformBefore = &t
		}
	}
	return nil
}

func nbiIsParamKey(key string) bool {
	for _, p := range nbiRoots {
		if strings.HasPrefix(key, p) {
			return true
		}
	}
	return false
}

// nbiParamConds menerjemahkan satu kunci path parameter menjadi kondisi domain.ParamCond. Validasi nama/operator/nilai dipakai ulang dari
// domain.ParseParamCond (satu sumber kebenaran dengan param= pada GET /devices).
func nbiParamConds(key string, val any, f *domain.DeviceFilter) error {
	name := strings.TrimSuffix(key, "._value")
	add := func(op, value string) error {
		raw := name + "~" + op
		if op != domain.ParamOpExists && op != domain.ParamOpMissing {
			raw += "~" + value
		}
		cond, err := domain.ParseParamCond(raw)
		if err != nil {
			return nbiBadRequest("%s", strings.TrimPrefix(err.Error(), domain.ErrInvalidInput.Error()+": "))
		}
		if len(f.Params) >= domain.MaxParamConds {
			return nbiBadRequest("maksimal %d kondisi parameter per permintaan", domain.MaxParamConds)
		}
		f.Params = append(f.Params, cond)
		return nil
	}

	ops, isOps := val.(map[string]any)
	if !isOps {
		s, err := nbiScalar(key, val)
		if err != nil {
			return err
		}
		return add(domain.ParamOpEq, s)
	}
	if len(ops) == 0 {
		return nbiBadRequest("objek operator untuk %q kosong", nbiTrunc(key))
	}
	byOp := map[string]string{"$eq": domain.ParamOpEq, "$ne": domain.ParamOpNe, "$gt": domain.ParamOpGt, "$gte": domain.ParamOpGe, "$lt": domain.ParamOpLt, "$lte": domain.ParamOpLe}
	for _, op := range sortedKeys(ops) {
		if op == "$exists" {
			b, ok := ops[op].(bool)
			if !ok {
				return nbiBadRequest("$exists harus true atau false")
			}
			kind := domain.ParamOpMissing
			if b {
				kind = domain.ParamOpExists
			}
			if err := add(kind, ""); err != nil {
				return err
			}
			continue
		}
		mapped, ok := byOp[op]
		if !ok {
			return nbiBadRequest("operator %q tidak didukung (didukung: $eq $ne $gt $gte $lt $lte $exists)", nbiTrunc(op))
		}
		s, err := nbiScalar(key, ops[op])
		if err != nil {
			return err
		}
		if err := add(mapped, s); err != nil {
			return err
		}
	}
	return nil
}

func sortedKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// parseNBISort menerima {"<kunci>": 1|-1} satu kunci (kosong = urutan bawaan).
func parseNBISort(raw string, f *domain.DeviceFilter) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	m, err := decodeNBIObject("sort", raw)
	if err != nil {
		return err
	}
	if len(m) != 1 {
		return nbiBadRequest("sort hanya mendukung satu kunci")
	}
	for key, dir := range m {
		col, ok := nbiSortFields[key]
		if !ok {
			return nbiBadRequest("sort pada %q tidak didukung (didukung: %s)", nbiTrunc(key), strings.Join(nbiSortKeyList(), ", "))
		}
		n, ok := dir.(json.Number)
		if !ok || (n.String() != "1" && n.String() != "-1") {
			return nbiBadRequest("arah sort harus 1 atau -1")
		}
		if n.String() == "-1" {
			col = "-" + col
		}
		if f.Sort, f.SortDesc, err = domain.ParseSort(col); err != nil {
			return nbiBadRequest("%s", strings.TrimPrefix(err.Error(), domain.ErrInvalidInput.Error()+": "))
		}
	}
	return nil
}

func nbiSortKeyList() []string {
	out := make([]string, 0, len(nbiSortFields))
	for k := range nbiSortFields {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// parseNBIProjection menerima daftar dipisah koma. Kunci dokumen tetap (_id, _lastInform, ...) selalu ada; sisanya harus path parameter
// PERSIS (bukan subtree) dan menjadi domain.DeviceFilter.ProjectParams.
func parseNBIProjection(raw string) ([]string, error) {
	var params []string
	seen := map[string]bool{}
	for _, name := range splitList(raw) {
		if nbiAlwaysFields[name] || strings.HasPrefix(name, "DeviceID.") {
			continue
		}
		name = strings.TrimSuffix(name, "._value")
		if !nbiIsParamKey(name) || !domain.ValidParamName(name) {
			return nil, nbiBadRequest("projection %q tidak dikenal: gunakan _id, _lastInform, _registered, DeviceID atau path parameter lengkap berawalan %s",
				nbiTrunc(name), strings.Join(nbiRoots, " / "))
		}
		if !seen[name] {
			seen[name] = true
			params = append(params, name)
		}
	}
	if len(params) > domain.MaxProjectParams {
		return nil, nbiBadRequest("maksimal %d path parameter pada projection", domain.MaxProjectParams)
	}
	return params, nil
}

// nbiPagination memetakan skip/limit ke halaman. Offset repositori = (page-1)*limit, jadi skip harus kelipatan limit; selain itu ditolak
// (bukan dibulatkan) agar tidak ada baris yang terlewat atau terulang. limit kosong = domain.DefaultPageSize; maks domain.MaxPageSize.
func nbiPagination(skipRaw, limitRaw string) (domain.Pagination, error) {
	limit, skip := domain.DefaultPageSize, 0
	var err error
	if limitRaw != "" {
		if limit, err = strconv.Atoi(limitRaw); err != nil || limit < 1 || limit > domain.MaxPageSize {
			return domain.Pagination{}, nbiBadRequest("limit harus bilangan 1..%d", domain.MaxPageSize)
		}
	}
	if skipRaw != "" {
		if skip, err = strconv.Atoi(skipRaw); err != nil || skip < 0 {
			return domain.Pagination{}, nbiBadRequest("skip harus bilangan >= 0")
		}
	}
	if skip%limit != 0 {
		return domain.Pagination{}, nbiBadRequest("skip harus kelipatan limit (skip=%d, limit=%d)", skip, limit)
	}
	page := skip/limit + 1
	if page > domain.MaxPage {
		return domain.Pagination{}, nbiBadRequest("skip terlalu besar")
	}
	return domain.Pagination{Page: page, PageSize: limit}, nil
}

// nbiDeviceDoc membentuk dokumen perangkat bergaya GenieACS: _id (= device_uuid, bukan PK internal), _lastInform, _registered,
// DeviceID, dan parameter terproyeksi bersarang {"A": {"B": {"_value": "v"}}}. Manufacturer, _tags, _type/_writable/_timestamp tidak
// diisi (tidak tersimpan pada baris daftar); _value selalu teks seperti yang tersimpan.
func nbiDeviceDoc(it domain.DeviceListItem, params []string) map[string]any {
	leaf := func(v *string) map[string]any {
		if v == nil {
			return map[string]any{"_value": ""}
		}
		return map[string]any{"_value": *v}
	}
	serial := it.SerialNumber
	doc := map[string]any{
		"_id":         it.DeviceUUID,
		"_registered": it.CreatedAt.UTC().Format(time.RFC3339),
		"DeviceID": map[string]any{
			"OUI":          leaf(it.OUI),
			"ProductClass": leaf(it.ProductClass),
			"SerialNumber": leaf(&serial),
		},
	}
	if it.LastInformAt != nil {
		doc["_lastInform"] = it.LastInformAt.UTC().Format(time.RFC3339)
	}
	for _, name := range params {
		if v, ok := it.Parameters[name]; ok {
			nbiSetNested(doc, name, v)
		}
	}
	return doc
}

// nbiSetNested menulis {"_value": v} pada path bertitik. Bila jalur menabrak simpul non-objek (mis. kunci _id), parameter dilewati.
func nbiSetNested(doc map[string]any, path, value string) {
	node := doc
	parts := strings.Split(path, ".")
	for i, p := range parts {
		if p == "" {
			return
		}
		if i == len(parts)-1 {
			if next, ok := node[p].(map[string]any); ok {
				next["_value"] = value
			} else if _, exists := node[p]; !exists {
				node[p] = map[string]any{"_value": value}
			}
			return
		}
		next, ok := node[p].(map[string]any)
		if !ok {
			if _, exists := node[p]; exists {
				return
			}
			next = map[string]any{}
			node[p] = next
		}
		node = next
	}
}

// nbiTrunc memendekkan teks dari klien sebelum dimasukkan ke pesan galat (hitung rune, bukan byte).
func nbiTrunc(s string) string {
	if r := []rune(s); len(r) > 60 {
		return string(r[:60]) + "…"
	}
	return s
}
