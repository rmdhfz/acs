package http

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"acs/internal/domain"
)

func TestParseNBIQuery_Diterjemahkan(t *testing.T) {
	var f domain.DeviceFilter
	q := `{"DeviceID.SerialNumber":"ZTEG1234",
	      "InternetGatewayDevice.DeviceInfo.SoftwareVersion":"V1.0",
	      "InternetGatewayDevice.WANDevice.1.WANDeviceNumberOfEntries._value":{"$gte":2,"$lt":9},
	      "Device.X.Y":{"$exists":false},
	      "_lastInform":{"$gte":"2026-10-01T00:00:00Z","$lt":"2026-10-29T00:00:00Z"}}`
	if err := parseNBIQuery(q, &f); err != nil {
		t.Fatalf("galat tak terduga: %v", err)
	}
	if f.SerialNumber != "ZTEG1234" {
		t.Errorf("serial = %q", f.SerialNumber)
	}
	want := map[string]bool{
		"Device.X.Y~missing": true,
		"InternetGatewayDevice.DeviceInfo.SoftwareVersion~eq~V1.0":        true,
		"InternetGatewayDevice.WANDevice.1.WANDeviceNumberOfEntries~ge~2": true,
		"InternetGatewayDevice.WANDevice.1.WANDeviceNumberOfEntries~lt~9": true,
	}
	if len(f.Params) != len(want) {
		t.Fatalf("jumlah kondisi = %d (%v), mau %d", len(f.Params), f.Params, len(want))
	}
	for _, c := range f.Params {
		key := c.Name + "~" + c.Op
		if c.Value != "" {
			key += "~" + c.Value
		}
		if !want[key] {
			t.Errorf("kondisi tak terduga %q", key)
		}
	}
	if f.InformAfter == nil || !f.InformAfter.Equal(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("InformAfter = %v", f.InformAfter)
	}
	if f.InformBefore == nil || !f.InformBefore.Equal(time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("InformBefore = %v", f.InformBefore)
	}
}

func TestParseNBIQuery_Kosong(t *testing.T) {
	for _, raw := range []string{"", "   "} {
		var f domain.DeviceFilter
		if err := parseNBIQuery(raw, &f); err != nil || len(f.Params) != 0 || f.SerialNumber != "" {
			t.Errorf("query %q: err=%v filter=%+v", raw, err, f)
		}
	}
}

func TestParseNBIQuery_Ditolak(t *testing.T) {
	long := `{"InternetGatewayDevice.A":"` + strings.Repeat("x", nbiMaxQueryBytes) + `"}`
	cases := map[string]struct{ raw, wantMsg string }{
		"bukan objek":              {`[1,2]`, "objek JSON"},
		"json rusak":               {`{"a":`, "objek JSON"},
		"data sisa":                {`{} {}`, "setelah objek"},
		"null":                     {`null`, "objek JSON"},
		"terlalu panjang":          {long, "maksimal"},
		"kunci tak didukung":       {`{"_id":"x"}`, "tidak didukung"},
		"operator logika":          {`{"$or":[{}]}`, "tidak didukung"},
		"regex":                    {`{"InternetGatewayDevice.A":{"$regex":"x"}}`, "tidak didukung"},
		"in":                       {`{"InternetGatewayDevice.A":{"$in":["x"]}}`, "tidak didukung"},
		"boolean sebagai nilai":    {`{"InternetGatewayDevice.A":true}`, "teks atau angka"},
		"null sebagai nilai":       {`{"InternetGatewayDevice.A":null}`, "teks atau angka"},
		"larik sebagai nilai":      {`{"InternetGatewayDevice.A":["x"]}`, "teks atau angka"},
		"exists bukan boolean":     {`{"InternetGatewayDevice.A":{"$exists":"ya"}}`, "$exists"},
		"op numerik nilai teks":    {`{"InternetGatewayDevice.A":{"$gt":"abc"}}`, "angka"},
		"objek operator kosong":    {`{"InternetGatewayDevice.A":{}}`, "kosong"},
		"nama parameter berbahaya": {`{"InternetGatewayDevice.A'; DROP TABLE x;--":"v"}`, "nama parameter"},
		"serial kosong":            {`{"DeviceID.SerialNumber":""}`, "tidak boleh kosong"},
		"serial dengan operator":   {`{"DeviceID.SerialNumber":{"$ne":"x"}}`, "pencocokan persis"},
		"lastInform $gt":           {`{"_lastInform":{"$gt":"2026-10-01T00:00:00Z"}}`, "$gte dan $lt"},
		"lastInform $lte":          {`{"_lastInform":{"$lte":"2026-10-01T00:00:00Z"}}`, "$gte dan $lt"},
		"lastInform bukan RFC3339": {`{"_lastInform":{"$gte":"kemarin"}}`, "RFC3339"},
		"lastInform skalar":        {`{"_lastInform":"2026-10-01T00:00:00Z"}`, "objek operator"},
		"nilai eq kosong":          {`{"InternetGatewayDevice.A":""}`, "membutuhkan nilai"},
		"terlalu banyak kondisi": {
			`{"Device.A":"1","Device.B":"1","Device.C":"1","Device.D":"1","Device.E":"1","Device.F":"1"}`, "maksimal",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var f domain.DeviceFilter
			err := parseNBIQuery(tc.raw, &f)
			if err == nil {
				t.Fatalf("seharusnya ditolak: %s", tc.raw)
			}
			if !strings.Contains(err.Error(), tc.wantMsg) {
				t.Errorf("pesan %q tidak memuat %q", err.Error(), tc.wantMsg)
			}
		})
	}
}

func TestParseNBISort(t *testing.T) {
	var f domain.DeviceFilter
	if err := parseNBISort(`{"_lastInform":-1}`, &f); err != nil || f.Sort != "last_inform_at" || !f.SortDesc {
		t.Errorf("desc: err=%v sort=%q desc=%v", err, f.Sort, f.SortDesc)
	}
	f = domain.DeviceFilter{}
	if err := parseNBISort(`{"DeviceID.SerialNumber":1}`, &f); err != nil || f.Sort != "serial_number" || f.SortDesc {
		t.Errorf("asc: err=%v sort=%q desc=%v", err, f.Sort, f.SortDesc)
	}
	f = domain.DeviceFilter{}
	if err := parseNBISort("", &f); err != nil || f.Sort != "" {
		t.Errorf("kosong: err=%v sort=%q", err, f.Sort)
	}
	for _, bad := range []string{`{"_lastInform":-1,"_registered":1}`, `{"foo":1}`, `{"_lastInform":2}`, `{"_lastInform":"1"}`, `[]`, `{}`} {
		if err := parseNBISort(bad, &domain.DeviceFilter{}); err == nil {
			t.Errorf("sort %s seharusnya ditolak", bad)
		}
	}
}

func TestParseNBIProjection(t *testing.T) {
	got, err := parseNBIProjection("_id, _lastInform,DeviceID.SerialNumber,InternetGatewayDevice.DeviceInfo.SoftwareVersion._value,InternetGatewayDevice.DeviceInfo.SoftwareVersion,Device.X")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"InternetGatewayDevice.DeviceInfo.SoftwareVersion", "Device.X"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("params = %v, mau %v (duplikat harus dibuang, kunci tetap diabaikan)", got, want)
	}
	if got, err := parseNBIProjection(""); err != nil || len(got) != 0 {
		t.Errorf("kosong: %v %v", got, err)
	}
	if _, err := parseNBIProjection("VirtualParameters.foo"); err == nil {
		t.Error("kunci di luar root data model seharusnya ditolak")
	}
	var many []string
	for i := 0; i <= domain.MaxProjectParams; i++ {
		many = append(many, "Device.P"+strings.Repeat("x", i))
	}
	if _, err := parseNBIProjection(strings.Join(many, ",")); err == nil {
		t.Error("melebihi MaxProjectParams seharusnya ditolak")
	}
}

func TestNBIPagination(t *testing.T) {
	ok := []struct {
		skip, limit string
		page, size  int
	}{
		{"", "", 1, domain.DefaultPageSize},
		{"0", "10", 1, 10},
		{"20", "10", 3, 10},
		{"", "200", 1, 200},
	}
	for _, c := range ok {
		pg, err := nbiPagination(c.skip, c.limit)
		if err != nil || pg.Page != c.page || pg.PageSize != c.size {
			t.Errorf("skip=%q limit=%q: %+v err=%v", c.skip, c.limit, pg, err)
		}
	}
	for _, c := range [][2]string{{"5", "10"}, {"", "0"}, {"", "201"}, {"-1", "10"}, {"abc", ""}, {"", "x"}, {"5", ""}} {
		if _, err := nbiPagination(c[0], c[1]); err == nil {
			t.Errorf("skip=%q limit=%q seharusnya ditolak", c[0], c[1])
		}
	}
	// skip yang membuat halaman melewati batas tidak boleh overflow offset.
	if _, err := nbiPagination("9999999999", "1"); err == nil {
		t.Error("skip raksasa seharusnya ditolak")
	}
}

func TestNBIDeviceDoc(t *testing.T) {
	oui := "ABCDEF"
	last := time.Date(2026, 10, 29, 3, 0, 0, 0, time.FixedZone("WIB", 7*3600))
	it := domain.DeviceListItem{
		Device: domain.Device{
			DeviceUUID: "uuid-1", SerialNumber: "SN1", OUI: &oui, LastInformAt: &last,
			Audit: domain.Audit{CreatedAt: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)},
		},
		Parameters: map[string]string{
			"InternetGatewayDevice.DeviceInfo.SoftwareVersion": "V1",
			"InternetGatewayDevice.DeviceInfo":                 "tabrakan-dengan-cabang",
			"InternetGatewayDevice.LANDevice.1.X":              "a",
			"InternetGatewayDevice.LANDevice.1.Y":              "b",
		},
	}
	doc := nbiDeviceDoc(it, []string{
		"InternetGatewayDevice.DeviceInfo.SoftwareVersion", "InternetGatewayDevice.DeviceInfo",
		"InternetGatewayDevice.LANDevice.1.X", "InternetGatewayDevice.LANDevice.1.Y", "InternetGatewayDevice.TidakAda",
	})
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	_ = json.Unmarshal(b, &got)

	if got["_id"] != "uuid-1" {
		t.Errorf("_id harus device_uuid, bukan PK: %v", got["_id"])
	}
	if got["_lastInform"] != "2026-10-28T20:00:00Z" {
		t.Errorf("_lastInform harus UTC RFC3339: %v", got["_lastInform"])
	}
	if got["_registered"] != "2026-10-01T00:00:00Z" {
		t.Errorf("_registered = %v", got["_registered"])
	}
	did := got["DeviceID"].(map[string]any)
	if did["SerialNumber"].(map[string]any)["_value"] != "SN1" || did["OUI"].(map[string]any)["_value"] != "ABCDEF" || did["ProductClass"].(map[string]any)["_value"] != "" {
		t.Errorf("DeviceID = %v", did)
	}
	igd := got["InternetGatewayDevice"].(map[string]any)
	di := igd["DeviceInfo"].(map[string]any)
	if di["SoftwareVersion"].(map[string]any)["_value"] != "V1" {
		t.Errorf("SoftwareVersion = %v", di)
	}
	if di["_value"] != "tabrakan-dengan-cabang" {
		t.Errorf("simpul yang juga cabang harus memuat _value: %v", di)
	}
	lan := igd["LANDevice"].(map[string]any)["1"].(map[string]any)
	if lan["X"].(map[string]any)["_value"] != "a" || lan["Y"].(map[string]any)["_value"] != "b" {
		t.Errorf("LANDevice.1 = %v", lan)
	}
	if _, ok := igd["TidakAda"]; ok {
		t.Error("parameter yang tak ada nilainya tidak boleh muncul")
	}
	for _, leaked := range []string{"tenant_id", "connection_request", "inform_password"} {
		if strings.Contains(string(b), leaked) {
			t.Errorf("dokumen NBI membocorkan %q", leaked)
		}
	}

	noLast := nbiDeviceDoc(domain.DeviceListItem{Device: domain.Device{DeviceUUID: "u2", SerialNumber: "S"}}, nil)
	if _, ok := noLast["_lastInform"]; ok {
		t.Error("perangkat belum pernah Inform tidak boleh punya _lastInform")
	}
}
